package fixtures;

import java.lang.reflect.Proxy;
import java.sql.*;
import java.util.Properties;
import java.util.logging.Logger;

/** Test only: connection succeeds but metadata fails without any network traffic. */
public final class MetadataFailureDriver implements Driver {
    @Override public Connection connect(String url, Properties properties) {
        if (!acceptsURL(url)) return null;
        return (Connection) Proxy.newProxyInstance(getClass().getClassLoader(), new Class<?>[]{Connection.class}, (proxy, method, args) -> {
            if (method.getName().equals("getMetaData")) throw new SQLException("fixture metadata denied", "HY000", 777);
            if (method.getName().equals("close")) return null;
            if (method.getName().equals("isClosed")) return false;
            throw new SQLFeatureNotSupportedException("unused fixture method");
        });
    }
    @Override public boolean acceptsURL(String url) { return url != null && url.startsWith("jdbc:metadata-fixture://"); }
    @Override public DriverPropertyInfo[] getPropertyInfo(String url, Properties info) { return new DriverPropertyInfo[0]; }
    @Override public int getMajorVersion() { return 1; }
    @Override public int getMinorVersion() { return 0; }
    @Override public boolean jdbcCompliant() { return false; }
    @Override public Logger getParentLogger() { return Logger.getGlobal(); }
}
