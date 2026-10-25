package com.opscopilot.jdbc;

import com.google.gson.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.sql.*;
import java.util.*;
import java.util.concurrent.*;

/** Private, length-prefixed stdio RPC. One isolated driver/connection per process. */
public final class Main {
    private static final Gson JSON = new GsonBuilder().serializeNulls().create();
    private static final int FRAME_LIMIT = 64 << 20;
    private static final DataOutputStream OUTPUT = new DataOutputStream(System.out);
    private static final ExecutorService WORK = Executors.newSingleThreadExecutor();
    private static volatile Statement active;
    private static Connection connection;
    private static URLClassLoader loader;
    private static Driver driver;
    private static String dialect;
    private static volatile String password = "", user = "", connectionURL = "";
    private static volatile boolean memoryFailed;
    private static final ThreadLocal<String> PHASE = ThreadLocal.withInitial(() -> "处理请求");
    private static final ThreadLocal<List<Map<String, Object>>> EVENTS = ThreadLocal.withInitial(ArrayList::new);
    private static final ThreadLocal<Long> STARTED = ThreadLocal.withInitial(System::nanoTime);
    private static Map<String, Object> runtimeDetails;

    public static void main(String[] args) throws Exception {
        // Driver diagnostics must never corrupt the protocol or reach application logs.
        System.setOut(new PrintStream(OutputStream.nullOutputStream()));
        System.setErr(new PrintStream(OutputStream.nullOutputStream()));
        runtimeDetails = RuntimeDetails.capture();
        try (DataInputStream input = new DataInputStream(System.in)) {
            while (true) {
                int size;
                try { size = input.readInt(); } catch (EOFException e) { break; }
                if (size < 1 || size > FRAME_LIMIT) break;
                byte[] bytes = input.readNBytes(size);
                if (bytes.length != size) break;
                JsonObject request = JsonParser.parseString(new String(bytes, StandardCharsets.UTF_8)).getAsJsonObject();
                if (text(request, "op").equals("cancel")) {
                    // Independent from the single connection worker, even while execute blocks.
                    Thread.ofVirtual().start(() -> respond(request, () -> {
                        Statement statement = active;
                        if (statement != null) statement.cancel();
                        return Map.of();
                    }));
                } else {
                    WORK.execute(() -> respond(request, () -> dispatch(request)));
                }
            }
        } finally {
            // EOF is the parent's lifetime signal. A driver blocked in close()
            // must not keep an orphan JVM alive after the application exits.
            WORK.shutdownNow();
            Runtime.getRuntime().halt(0);
        }
    }

    private interface Operation { Object run() throws Exception; }
    private static void respond(JsonObject request, Operation operation) {
        Map<String, Object> response = new LinkedHashMap<>();
        response.put("id", request.get("id"));
        PHASE.set(text(request, "op").equals("query") ? "执行 SQL" : "处理请求");
        EVENTS.get().clear(); STARTED.set(System.nanoTime());
        try {
            if (memoryFailed) throw new SQLException("JDBC 运行组件发生过内存分配失败，请重新连接；未继续执行操作");
            response.put("data", operation.run());
        }
        catch (Exception | LinkageError | OutOfMemoryError error) {
            memoryFailed |= JdbcErrors.isMemoryFailure(error);
            response.put("error", JdbcErrors.format(PHASE.get(), error, password, user, connectionURL));
            Map<String, Object> failure = new LinkedHashMap<>();
            failure.put("phase", PHASE.get()); failure.put("category", JdbcErrors.category(error));
            Map<String, Long> memory = JdbcErrors.memoryValues(error);
            if (!memory.isEmpty()) failure.put("directMemory", memory);
            response.put("failure", failure);
            if (memoryFailed) response.put("fatal", true);
        }
        String op = text(request, "op");
        if (op.equals("load") || op.equals("open") || op.equals("version")) response.put("runtime", runtimeDetails);
        if (!EVENTS.get().isEmpty()) response.put("events", new ArrayList<>(EVENTS.get()));
        byte[] bytes = JSON.toJson(response).getBytes(StandardCharsets.UTF_8);
        try {
            synchronized (OUTPUT) {
                if (bytes.length > FRAME_LIMIT) throw new IOException("response too large");
                OUTPUT.writeInt(bytes.length); OUTPUT.write(bytes); OUTPUT.flush();
            }
        } catch (IOException error) { Runtime.getRuntime().halt(1); }
    }

    // Fixed phase names only. Never emit driver stdout/stderr, credentials or SQL.
    private static void mark(String stage) {
        EVENTS.get().add(Map.of("stage", stage, "elapsedMS", (System.nanoTime() - STARTED.get()) / 1_000_000));
    }

    private static Object dispatch(JsonObject request) throws Exception {
        String op = text(request, "op");
        switch (op) {
            case "load": {
                mark("driver.load");
                PHASE.set("加载驱动类");
                List<URL> urls = new ArrayList<>();
                for (JsonElement path : request.getAsJsonArray("jars")) urls.add(new File(path.getAsString()).toURI().toURL());
                loader = new URLClassLoader(urls.toArray(URL[]::new), Main.class.getClassLoader());
                Thread.currentThread().setContextClassLoader(loader);
                driver = (Driver) Class.forName(text(request, "className"), true, loader).getDeclaredConstructor().newInstance();
                mark("driver.loaded");
                dialect = text(request, "dialect");
                String checkURL = text(request, "checkURL");
                PHASE.set("校验 URL 模板");
                if (!checkURL.isEmpty() && !driver.acceptsURL(checkURL)) throw new SQLException("已加载的驱动不接受当前 URL 模板。请使用与此驱动匹配的 JDBC 协议和 URL 格式。", "08001");
                return Map.of("protocol", 1, "version", driver.getMajorVersion() + "." + driver.getMinorVersion(),
                    "javaVersion", System.getProperty("java.version"), "javaVendor", System.getProperty("java.vendor"), "osArch", System.getProperty("os.arch"));
            }
            case "open": {
                password = text(request, "password"); user = text(request, "user"); connectionURL = text(request, "url");
                PHASE.set("校验连接 URL");
                mark("url.check");
                if (!driver.acceptsURL(connectionURL)) throw new SQLException("驱动不接受生成的 JDBC URL，请核对驱动类型和 URL 模板。", "08001");
                Properties properties = new Properties();
                properties.setProperty("user", text(request, "user"));
                properties.setProperty("password", text(request, "password"));
                if (dialect.equals("mysql")) {
                    properties.setProperty("connectTimeout", "10000");
                    properties.setProperty("socketTimeout", "125000");
                    properties.setProperty("allowMultiQueries", "false");
                    properties.setProperty("allowLoadLocalInfile", "false");
                    properties.setProperty("allowUrlInLocalInfile", "false");
                    properties.setProperty("allowPublicKeyRetrieval", text(request, "allowPublicKeyRetrieval"));
                    properties.setProperty("sslMode", text(request, "tls").equals("required") ? "VERIFY_IDENTITY" : "DISABLED");
                }
                DriverManager.setLoginTimeout(10);
                PHASE.set("建立连接／握手认证");
                mark("driver.connect");
                connection = driver.connect(connectionURL, properties);
                if (connection == null) throw new SQLException("unsupported URL", "08001");
                mark("driver.connected");
                PHASE.set("读取数据库版本");
                mark("metadata.version");
                String version = connectionVersion();
                mark("metadata.version.ok");
                return Map.of("version", version);
            }
            case "version": {
                PHASE.set("读取数据库版本"); mark("metadata.version");
                String version = connectionVersion(); mark("metadata.version.ok");
                return Map.of("version", version);
            }
            case "runtime": return Map.of("javaVersion", System.getProperty("java.version"), "legacyJaxb", javax.xml.bind.DatatypeConverter.printHexBinary(new byte[]{1, 2}).equals("0102"));
            case "database": {
                if (dialect.equals("mysql")) connection.setCatalog(text(request, "database"));
                else connection.setSchema(text(request, "database"));
                return Map.of();
            }
            case "begin": {
                if (request.get("readOnly").getAsBoolean()) {
                    if (!dialect.equals("mysql")) throw new SQLFeatureNotSupportedException("read-only dialect not verified");
                    // Unlike setReadOnly (a hint), this is enforced by the server.
                    try (Statement statement = connection.createStatement()) { statement.execute("SET TRANSACTION READ ONLY"); }
                }
                connection.setAutoCommit(false);
                return Map.of();
            }
            case "rollback": connection.rollback(); connection.setAutoCommit(true); return Map.of();
            case "commit": connection.commit(); connection.setAutoCommit(true); return Map.of();
            case "schemas": {
                try (ResultSet rows = dialect.equals("mysql") ? connection.getMetaData().getCatalogs() : connection.getMetaData().getSchemas()) { return read(rows, 10000); }
            }
            case "tables": {
                String schema = text(request, "database");
                try (ResultSet rows = connection.getMetaData().getTables(dialect.equals("mysql") ? schema : null, dialect.equals("mysql") ? null : schema, "%", new String[]{"TABLE", "VIEW"})) { return read(rows, 10000); }
            }
            case "columns": case "indexes": {
                String schema = text(request, "database"), table = text(request, "table");
                DatabaseMetaData meta = connection.getMetaData();
                try (ResultSet rows = op.equals("columns")
                    ? meta.getColumns(dialect.equals("mysql") ? schema : null, dialect.equals("mysql") ? null : schema, table, "%")
                    : meta.getIndexInfo(dialect.equals("mysql") ? schema : null, dialect.equals("mysql") ? null : schema, table, false, false)) { return read(rows, 2000); }
            }
            case "query": {
                String sql = text(request, "sql");
                try (PreparedStatement statement = connection.prepareStatement(sql)) {
                    active = statement;
                    int seconds = request.get("timeout").getAsInt();
                    statement.setQueryTimeout(Math.max(1, Math.min(120, seconds)));
                    int limit = Math.max(1, Math.min(10001, request.get("limit").getAsInt()));
                    statement.setMaxRows(limit);
                    if (dialect.equals("mysql")) statement.setFetchSize(Integer.MIN_VALUE);
                    int index = 1;
                    for (JsonElement arg : request.getAsJsonArray("args")) {
                        if (arg.isJsonNull()) statement.setNull(index++, Types.NULL);
                        else statement.setString(index++, arg.getAsString());
                    }
                    if (statement.execute()) {
                        try (ResultSet rows = statement.getResultSet()) { return read(rows, limit); }
                    }
                    return Map.of("columns", List.of(), "rows", List.of(), "affectedRows", Math.max(0, statement.getLargeUpdateCount()));
                } finally { active = null; }
            }
            default: throw new IllegalArgumentException("unknown operation");
        }
    }

    private static Object read(ResultSet rows, int limit) throws Exception {
        ResultSetMetaData metadata = rows.getMetaData();
        int count = metadata.getColumnCount();
        if (count > 2048) throw new SQLDataException("too many columns");
        List<Map<String, String>> columns = new ArrayList<>();
        for (int i = 1; i <= count; i++) columns.add(Map.of("name", metadata.getColumnLabel(i), "type", metadata.getColumnTypeName(i)));
        List<List<String>> data = new ArrayList<>();
        int budget = 8 << 20;
        while (rows.next()) {
            // Never return incomplete success: Go keeps an extra sentinel row for truncation.
            if (data.size() >= limit) throw new SQLDataException("result exceeds helper row limit; reduce query scope", "54000");
            List<String> row = new ArrayList<>();
            budget -= count * 32;
            if (budget < 0) throw new SQLDataException("result exceeds byte bound", "54000");
            for (int i = 1; i <= count; i++) {
                int type = metadata.getColumnType(i);
                if (type != Types.CHAR && type != Types.VARCHAR && type != Types.LONGVARCHAR && type != Types.NCHAR && type != Types.NVARCHAR && type != Types.LONGNVARCHAR && type != Types.CLOB && type != Types.NCLOB && type != Types.BINARY && type != Types.VARBINARY && type != Types.LONGVARBINARY && type != Types.BLOB) {
                    String value = rows.getString(i);
                    if (value != null) { budget -= value.getBytes(StandardCharsets.UTF_8).length; if (budget < 0 || value.length() > (1 << 20)) throw new SQLDataException("result exceeds bound", "54000"); }
                    row.add(value); continue;
                }
                if (type == Types.BINARY || type == Types.VARBINARY || type == Types.LONGVARBINARY || type == Types.BLOB) {
                    try (InputStream stream = rows.getBinaryStream(i)) {
                        if (stream == null) { row.add(null); continue; }
                        byte[] bytes = stream.readNBytes((1 << 19) + 1);
                        if (bytes.length > (1 << 19)) throw new SQLDataException("binary cell exceeds bound", "54000");
                        String value = "0x" + HexFormat.of().formatHex(bytes); budget -= value.length();
                        if (budget < 0) throw new SQLDataException("result exceeds bound", "54000");
                        row.add(value); continue;
                    }
                }
                try (Reader reader = rows.getCharacterStream(i)) {
                    if (reader == null) { row.add(null); continue; }
                    StringBuilder cell = new StringBuilder();
                    char[] buffer = new char[4096];
                    int n;
                    while ((n = reader.read(buffer)) != -1) {
                        if (cell.length() + n > (1 << 20)) throw new SQLDataException("cell exceeds bound", "54000");
                        cell.append(buffer, 0, n);
                    }
                    String value = cell.toString();
                    budget -= value.getBytes(StandardCharsets.UTF_8).length;
                    if (budget < 0) throw new SQLDataException("result exceeds byte bound", "54000");
                    row.add(value);
                }
            }
            data.add(row);
        }
        return Map.of("columns", columns, "rows", data, "affectedRows", 0);
    }
    private static String text(JsonObject object, String key) {
        JsonElement value = object.get(key);
        return value == null || value.isJsonNull() ? "" : value.getAsString();
    }
    private static String connectionVersion() throws SQLException {
        try { DatabaseMetaData metadata = connection.getMetaData(); return String.valueOf(metadata.getDatabaseProductName()) + " " + String.valueOf(metadata.getDatabaseProductVersion()); }
        catch (SQLFeatureNotSupportedException | AbstractMethodError unsupported) { return "已连接（驱动未提供版本信息）"; }
    }
}
