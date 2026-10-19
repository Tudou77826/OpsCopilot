package fixtures;

import java.nio.ByteBuffer;
import java.sql.*;
import java.util.Properties;
import java.util.logging.Logger;

/** Exercises legacy driver initialization without opening a database connection. */
public final class LegacyNioDriver implements Driver {
    private static final class Buffers {
        static final java.lang.reflect.Method GET;
        static final java.lang.reflect.Method RELEASE;
        static {
            try {
                Class<?> util = Class.forName("sun.nio.ch.Util");
                GET = util.getDeclaredMethod("getTemporaryDirectBuffer", int.class);
                GET.setAccessible(true);
                RELEASE = util.getDeclaredMethod("releaseTemporaryDirectBuffer", ByteBuffer.class);
                RELEASE.setAccessible(true);
            } catch (ReflectiveOperationException error) {
                throw new ExceptionInInitializerError(error);
            }
        }
    }

    @Override public Connection connect(String url, Properties properties) throws SQLException {
        if (!acceptsURL(url)) return null;
        try {
            ByteBuffer buffer = (ByteBuffer) Buffers.GET.invoke(null, 16);
            try {
                buffer.putInt(42);
                if (buffer.getInt(0) != 42) throw new SQLException("NIO buffer read failed");
            } finally {
                Buffers.RELEASE.invoke(null, buffer);
            }
        } catch (ReflectiveOperationException | LinkageError error) {
            throw new SQLException("Legacy driver initialization failed", "08001", error);
        }
        // The test stops here: no real server is involved.
        throw new SQLException("legacy-nio-reflection-ok", "08001");
    }
    @Override public boolean acceptsURL(String url) { return url != null && url.startsWith("jdbc:legacy-nio:"); }
    @Override public DriverPropertyInfo[] getPropertyInfo(String url, Properties properties) { return new DriverPropertyInfo[0]; }
    @Override public int getMajorVersion() { return 1; }
    @Override public int getMinorVersion() { return 0; }
    @Override public boolean jdbcCompliant() { return false; }
    @Override public Logger getParentLogger() { return Logger.getGlobal(); }
}
