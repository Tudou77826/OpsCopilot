package com.opscopilot.jdbc;

import org.junit.Test;
import static org.junit.Assert.*;
import java.net.*;
import java.sql.*;
import java.util.*;

public class JdbcErrorsTest {
    @Test public void preservesNestedConnectionFailureAndRedactsCredentials() {
        String password = "p@ss word!";
        SQLException failure = new SQLException("login failed password='" + password + "' jdbc:zenith:@host:1888", "08001", 0,
            new ConnectException("Connection refused user=admin password=" + URLEncoder.encode(password, java.nio.charset.StandardCharsets.UTF_8)));
        String result = JdbcErrors.format("建立连接／握手认证", failure, password, "admin", "jdbc:zenith:@host:1888");
        assertTrue(result.contains("08001")); assertTrue(result.contains("ConnectException")); assertTrue(result.contains("端口拒绝连接"));
        assertFalse(result.contains(password)); assertFalse(result.contains("p%40ss+word%21")); assertFalse(result.contains("jdbc:zenith")); assertFalse(result.contains("admin"));
    }
    @Test public void followsSQLExceptionNextExceptionsAndCycles() {
        SQLException outer = new SQLException("outer", "08001");
        SQLException auth = new SQLException("authentication failed", "28000");
        outer.setNextException(auth); auth.initCause(outer);
        assertTrue(JdbcErrors.format("建立连接／握手认证", outer, "", "", "").contains("身份验证"));
    }
    @Test public void distinguishesTlsDependencyAndTimeoutFailures() {
        assertTrue(JdbcErrors.format("连接", new javax.net.ssl.SSLHandshakeException("PKIX path failed"), "", "", "").contains("TLS 握手"));
        assertTrue(JdbcErrors.format("连接", new NoClassDefFoundError("javax/xml/bind/DatatypeConverter"), "", "", "").contains("JAXB"));
        assertTrue(JdbcErrors.format("连接", new SocketTimeoutException("Read timed out"), "", "", "").contains("握手超时"));
    }
    @Test public void boundsDetailsAndRedactsQueryLiterals() {
        SQLException failure = new SQLException("Invalid SQL 'private-value' " + "x".repeat(9000), "42000");
        String result = JdbcErrors.format("执行 SQL", failure, "", "", "");
        assertFalse(result.contains("private-value")); assertTrue(result.length() <= 5001);
    }
    @Test public void legacyDatatypeConverterAvailableOnJava21() {
        assertArrayEquals(new byte[]{1,2}, javax.xml.bind.DatatypeConverter.parseBase64Binary("AQI="));
        assertEquals("0102", javax.xml.bind.DatatypeConverter.printHexBinary(new byte[]{1,2}));
    }
    @Test public void identifiesModuleAccessFailureInsideDriverInitialization() {
        Throwable cause = new java.lang.reflect.InaccessibleObjectException("module java.base does not exports sun.nio.ch");
        SQLException failure = new SQLException("initialization failed", "08001", new ExceptionInInitializerError(cause));
        String result = JdbcErrors.format("建立连接／握手认证", failure, "", "", "");
        assertTrue(result.contains("模块访问规则不兼容"));
        assertTrue(result.contains("sun.nio.ch"));
        assertTrue(result.contains("08001"));
        assertFalse(result.contains("端口拒绝连接"));
    }
}
