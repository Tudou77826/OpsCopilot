package com.huawei.gauss.jdbc;

import java.lang.reflect.Proxy;
import java.sql.*;
import java.util.Properties;
import java.util.logging.Logger;

/** In-process protocol fixture; not a GaussDB implementation. */
public final class ZenithDriver implements Driver {
    private static Object proxy(Class<?> type, java.lang.reflect.InvocationHandler handler) {
        return Proxy.newProxyInstance(ZenithDriver.class.getClassLoader(), new Class<?>[]{type}, handler);
    }
    private static ResultSet rows(String column, String value) {
        final boolean[] next = {true};
        return (ResultSet) proxy(ResultSet.class, (p, m, a) -> {
            switch (m.getName()) {
                case "getMetaData": return proxy(ResultSetMetaData.class, (p2, m2, a2) -> {
                    switch (m2.getName()) {
                        case "getColumnCount": return 1;
                        case "getColumnLabel": return column;
                        case "getColumnTypeName": return "INTEGER";
                        case "getColumnType": return Types.INTEGER;
                        default: throw new SQLFeatureNotSupportedException(m2.getName());
                    }
                });
                case "next": boolean result = next[0]; next[0] = false; return result;
                case "getString": return value;
                case "close": return null;
                default: throw new SQLFeatureNotSupportedException(m.getName());
            }
        });
    }
    @Override public boolean acceptsURL(String url) { return url != null && url.startsWith("jdbc:zenith:@"); }
    @Override public Connection connect(String url, Properties properties) {
        if (!acceptsURL(url)) return null;
        return (Connection) proxy(Connection.class, (p, m, a) -> {
            switch (m.getName()) {
                case "getMetaData": return proxy(DatabaseMetaData.class, (p2, m2, a2) -> {
                    if (m2.getName().equals("getDatabaseProductName")) return "Fixture";
                    if (m2.getName().equals("getDatabaseProductVersion")) return "1";
                    if (m2.getName().equals("getSchemas")) return rows("TABLE_SCHEM", "APP");
                    if (m2.getName().equals("getTables")) return rows("TABLE_NAME", "ORDERS");
                    if (m2.getName().equals("getColumns")) return rows("COLUMN_NAME", "ID");
                    if (m2.getName().equals("getIndexInfo")) return rows("INDEX_NAME", "IDX_ID");
                    throw new SQLFeatureNotSupportedException(m2.getName());
                });
                case "prepareStatement":
                    if (!((String)a[0]).startsWith("SELECT ")) throw new SQLException("fixture only accepts SELECT");
                    return proxy(PreparedStatement.class, (p2, m2, a2) -> {
                        switch (m2.getName()) {
                            case "setQueryTimeout": case "setMaxRows": case "close": return null;
                            case "execute": return true;
                            case "getResultSet": return rows("VALUE", "42");
                            default: throw new SQLFeatureNotSupportedException(m2.getName());
                        }
                    });
                case "close": return null;
                case "isClosed": return false;
                default: throw new SQLFeatureNotSupportedException("unexpected connection operation: " + m.getName());
            }
        });
    }
    @Override public DriverPropertyInfo[] getPropertyInfo(String url, Properties properties) { return new DriverPropertyInfo[0]; }
    @Override public int getMajorVersion() { return 1; }
    @Override public int getMinorVersion() { return 0; }
    @Override public boolean jdbcCompliant() { return false; }
    @Override public Logger getParentLogger() { return Logger.getGlobal(); }
}
