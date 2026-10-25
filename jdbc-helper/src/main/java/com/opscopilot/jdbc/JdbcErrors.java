package com.opscopilot.jdbc;

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.sql.SQLException;
import java.util.*;
import java.util.regex.Pattern;

/** Bounded driver diagnostics, with connection credentials removed before truncation. */
final class JdbcErrors {
    private static final Pattern JDBC_URL = Pattern.compile("(?i)jdbc:[^\\s\"'<>]+", Pattern.UNICODE_CASE);
    private static final Pattern ASSIGNMENT = Pattern.compile("(?i)(password|passwd|pwd|secret|token|api[_-]?key)\\s*[:=]\\s*(?:\"[^\"]*\"|'[^']*'|[^\\s,;)\\]}]+)");
    private static final Pattern DIRECT_MEMORY = Pattern.compile("Cannot reserve ([0-9]{1,19}) bytes of direct buffer memory \\(allocated: ([0-9]{1,19}), limit: ([0-9]{1,19})\\)");

    static String format(String phase, Throwable error, String password, String user, String url) {
        List<Throwable> chain = new ArrayList<>();
        Set<Throwable> visited = Collections.newSetFromMap(new IdentityHashMap<>());
        collect(error, chain, visited);
        String state = "", code = "";
        List<String> details = new ArrayList<>();
        StringBuilder matching = new StringBuilder();
        for (Throwable cause : chain) {
            matching.append(cause.getClass().getName()).append(' ').append(cause.getMessage()).append('\n');
            if (cause instanceof SQLException sql && state.isEmpty() && sql.getSQLState() != null) {
                state = sql.getSQLState().matches("[A-Za-z0-9]{5}") ? sql.getSQLState() : "unknown";
                code = Integer.toString(sql.getErrorCode());
            }
            String message = redact(String.valueOf(cause.getMessage()), password, user, url);
            // SQL errors can echo arbitrary literals from statements; connection
            // diagnostics remain detailed, query literals remain private.
            if (phase.equals("执行 SQL")) message = message.replaceAll("'(?:''|[^'])*'|\"(?:\"\"|[^\"])*\"", "[literal]");
            if (message.length() > 800) message = message.substring(0, 800) + "…";
            details.add(cause.getClass().getSimpleName() + ": " + message);
        }
        String lower = matching.toString().toLowerCase(Locale.ROOT);
        boolean memoryFailure = isMemoryFailure(error);
        String hint;
        if (memoryFailure && (lower.contains("direct buffer memory") || lower.contains("directbuffer"))) hint = "JDBC 驱动的直接内存分配失败。这是本地 Java 运行组件的报错，不代表数据库服务器内存不足。握手阶段需对照可用客户端的驱动 JAR、URL、目标端口和缓冲配置；仅凭此错误不能判断应当增大内存，程序不会自动重试。";
        else if (memoryFailure) hint = "JDBC 运行组件发生内存分配失败，请检查驱动及查询范围；已停止使用该辅助进程，操作不自动重试。";
        else if (lower.contains("public key retrieval is not allowed")) hint = "服务器要求完整密码验证。请启用可信 TLS，或在受信任网络／SSH 通道中显式允许获取服务器 RSA 公钥。";
        else if (lower.contains("inaccessibleobjectexception") || lower.contains("illegalaccesserror")) hint = "驱动与当前 Java 运行组件的模块访问规则不兼容。请更新程序和 JDBC 运行组件，并保留以下异常详情。";
        else if (lower.contains("javax/xml/bind") || lower.contains("javax.xml.bind")) hint = "驱动依赖旧版 JAXB。请更新随程序分发的 JDBC 运行组件，或导入对应的 JAXB 2.x 依赖 JAR。";
        else if (lower.contains("unsupportedclassversionerror")) hint = "驱动要求的 Java 版本高于当前运行组件，请使用兼容驱动或更新组件。";
        else if (lower.contains("classnotfoundexception") || lower.contains("noclassdeffounderror")) hint = "驱动缺少依赖类，请一并导入厂商依赖 JAR。";
        else if (lower.contains("unknownhostexception")) hint = "无法解析数据库地址，请检查主机名和当前网络。";
        else if (lower.contains("connectexception") || lower.contains("connection refused")) hint = "数据库端口拒绝连接，请检查地址、端口、服务和 SSH 通道。";
        else if (lower.contains("sockettimeoutexception") || lower.contains("connect timed out")) hint = "连接或握手超时，请检查内网可达性、防火墙及数据库服务状态。";
        else if (lower.contains("sslhandshakeexception") || lower.contains("certificateexception") || lower.contains("pkix")) hint = "TLS 握手或证书验证失败，请检查服务端 TLS 配置及 Java 信任库。";
        else if (state.startsWith("28") || lower.contains("authentication failed") || lower.contains("invalid username") || lower.contains("access denied")) hint = "数据库拒绝身份验证，请核对账号、密码及账号登录权限。";
        else hint = "请根据以下驱动详情排查；驱动导入成功不代表数据库连接成功。";
        String context = "JDBC 操作失败 · " + phase;
        if (!state.isEmpty()) context += "（SQLState " + state + "，代码 " + code + "）";
        context += "\n" + hint;
        if (memoryFailure) context += memoryDetails(chain, matching.toString(), password, user, url);
        else if (!phase.equals("执行 SQL")) {
            Throwable deepest = chain.isEmpty() ? error : chain.get(chain.size() - 1);
            context += "\n异常调用位置（采样，仅类、方法和行号）：" + stackDetails(deepest, password, user, url);
        }
        context += "\n" + String.join("\n原因：", details);
        return context.length() > 5000 ? context.substring(0, 5000) + "…" : context;
    }

    static String category(Throwable error) {
        List<Throwable> chain = new ArrayList<>();
        collect(error, chain, Collections.newSetFromMap(new IdentityHashMap<>()));
        String text = chain.stream().map(cause -> cause.getClass().getName() + " " + cause.getMessage()).reduce("", (a, b) -> a + "\n" + b).toLowerCase(Locale.ROOT);
        if (isMemoryFailure(error)) return text.contains("direct buffer memory") || text.contains("directbuffer") ? "memory.direct" : "memory.other";
        if (text.contains("inaccessibleobjectexception") || text.contains("illegalaccesserror") || text.contains("unsupportedclassversionerror") || text.contains("classnotfoundexception") || text.contains("noclassdeffounderror")) return "driver.compatibility";
        if (text.contains("sslhandshakeexception") || text.contains("certificateexception") || text.contains("pkix")) return "tls";
        if (text.contains("unknownhostexception")) return "network.dns";
        if (text.contains("sockettimeoutexception") || text.contains("connect timed out")) return "network.timeout";
        if (text.contains("connectexception") || text.contains("connection refused")) return "network.refused";
        if (chain.stream().anyMatch(cause -> cause instanceof SQLException sql && sql.getSQLState() != null && sql.getSQLState().startsWith("28"))) return "authentication";
        return "driver.other";
    }

    static boolean isMemoryFailure(Throwable error) {
        List<Throwable> chain = new ArrayList<>();
        collect(error, chain, Collections.newSetFromMap(new IdentityHashMap<>()));
        // Some vendor drivers keep only Error.toString() in their SQLException.
        return chain.stream().anyMatch(cause -> cause instanceof OutOfMemoryError ||
            String.valueOf(cause.getMessage()).contains("java.lang.OutOfMemoryError:"));
    }

    static Map<String, Long> memoryValues(Throwable error) {
        List<Throwable> chain = new ArrayList<>();
        collect(error, chain, Collections.newSetFromMap(new IdentityHashMap<>()));
        for (Throwable cause : chain) {
            var match = DIRECT_MEMORY.matcher(String.valueOf(cause.getMessage()));
            if (!match.find()) continue;
            try {
                return Map.of("requested", Long.parseLong(match.group(1)), "allocated", Long.parseLong(match.group(2)), "limit", Long.parseLong(match.group(3)));
            } catch (NumberFormatException ignored) { /* An incomplete number is not evidence. */ }
        }
        return Map.of();
    }

    private static String memoryDetails(List<Throwable> chain, String messages, String password, String user, String url) {
        StringBuilder result = new StringBuilder();
        var match = DIRECT_MEMORY.matcher(messages);
        if (match.find()) {
            try {
                long requested = Long.parseLong(match.group(1)), allocated = Long.parseLong(match.group(2)), limit = Long.parseLong(match.group(3));
                result.append(String.format(Locale.ROOT, "\n直接内存：本次申请 %.1f MiB；已分配 %.1f MiB；上限 %.1f MiB。", requested / 1048576.0, allocated / 1048576.0, limit / 1048576.0));
            } catch (NumberFormatException ignored) { /* Preserve the bounded original error below. */ }
        }
        Throwable allocation = chain.stream().filter(cause -> cause instanceof OutOfMemoryError).findFirst().orElse(null);
        if (allocation == null) return result + "\n驱动未保留原始内存分配调用栈，尚不能定位分配来源。\n以下为外层异常调用位置，不代表分配栈：" + stackDetails(chain.get(0), password, user, url);
        result.append("\n内存分配调用位置（采样，仅类、方法和行号）：");
        result.append(stackDetails(allocation, password, user, url));
        return result.toString();
    }

    private static String stackDetails(Throwable error, String password, String user, String url) {
        StringBuilder result = new StringBuilder();
        StackTraceElement[] frames = error.getStackTrace();
        SortedSet<Integer> selected = new TreeSet<>();
        for (int i = 0; i < Math.min(4, frames.length); i++) selected.add(i);
        // Preserve a vendor frame even when NIO/reflection fills the stack top.
        for (int i = 4; i < Math.min(128, frames.length) && selected.size() < 8; i++) {
            String name = frames[i].getClassName();
            if (!name.startsWith("java.") && !name.startsWith("javax.") && !name.startsWith("jdk.") && !name.startsWith("sun.") && !name.startsWith("com.opscopilot.jdbc.")) selected.add(i);
        }
        for (int i = 4; i < frames.length && selected.size() < 8; i++) selected.add(i);
        for (int i : selected) {
            String owner = frames[i].getClassName(), method = frames[i].getMethodName();
            if (!owner.matches("[A-Za-z0-9_$./]{1,180}") || !method.matches("[A-Za-z0-9_$<>]{1,120}")) continue;
            if (result.length() > 1300) { result.append("\n  [调用位置已截断]"); break; }
            result.append("\n  ").append(redact(owner + "." + method, password, user, url));
            if (frames[i].getLineNumber() >= 0) result.append(":").append(frames[i].getLineNumber());
        }
        return result.length() == 0 ? "\n  [驱动未提供栈帧]" : result.toString();
    }

    private static void collect(Throwable error, List<Throwable> out, Set<Throwable> seen) {
        if (error == null || out.size() >= 6 || !seen.add(error)) return;
        out.add(error);
        collect(error.getCause(), out, seen);
        if (error instanceof SQLException sql) collect(sql.getNextException(), out, seen);
    }

    private static String redact(String text, String password, String user, String url) {
        if (url != null && !url.isEmpty()) text = text.replace(url, "[JDBC URL]");
        text = JDBC_URL.matcher(text).replaceAll("[JDBC URL]");
        text = ASSIGNMENT.matcher(text).replaceAll("$1=[redacted]");
        for (String secret : new String[]{password, user}) {
            if (secret == null || secret.isEmpty()) continue;
            text = text.replace(URLEncoder.encode(secret, StandardCharsets.UTF_8), "[redacted]")
                .replace(secret.replace("'", "''"), "[redacted]").replace(secret, "[redacted]");
        }
        return text.replaceAll("[\\p{Cntrl}]", " ");
    }
}
