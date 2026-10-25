package fixtures;

import java.lang.reflect.Proxy;
import java.nio.ByteBuffer;
import java.sql.*;
import java.util.Properties;
import java.util.logging.Logger;

/** Deterministic local allocation; never opens a socket or executes SQL. */
public final class DirectMemoryDriver implements Driver {
    @Override public Connection connect(String url, Properties properties) throws SQLException {
        if (!acceptsURL(url)) return null;
        ByteBuffer buffer;
        try { buffer = ByteBuffer.allocateDirect(128 * 1024 * 1024); }
        catch (OutOfMemoryError error) {
            if (url.endsWith("wrapped")) throw new SQLException("driver wrapper", "08006", error);
            throw error;
        }
        DatabaseMetaData meta = (DatabaseMetaData) Proxy.newProxyInstance(getClass().getClassLoader(), new Class<?>[]{DatabaseMetaData.class}, (proxy, method, args) -> switch (method.getName()) {
            case "getDatabaseProductName" -> "memory-fixture";
            case "getDatabaseProductVersion" -> "1.0";
            default -> throw new SQLFeatureNotSupportedException();
        });
        return (Connection) Proxy.newProxyInstance(getClass().getClassLoader(), new Class<?>[]{Connection.class}, (proxy, method, args) -> switch (method.getName()) {
            case "getMetaData" -> meta;
            case "close" -> { buffer.clear(); yield null; }
            case "isClosed" -> false;
            default -> throw new SQLFeatureNotSupportedException();
        });
    }
    @Override public boolean acceptsURL(String url) { return url != null && url.startsWith("jdbc:direct-memory:"); }
    @Override public DriverPropertyInfo[] getPropertyInfo(String url, Properties properties) { return new DriverPropertyInfo[0]; }
    @Override public int getMajorVersion() { return 1; }
    @Override public int getMinorVersion() { return 0; }
    @Override public boolean jdbcCompliant() { return false; }
    @Override public Logger getParentLogger() { return Logger.getGlobal(); }
}
