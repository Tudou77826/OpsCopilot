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
    @Test public void identifiesDirectMemoryLimitAndBoundsAllocationFrames() {
        OutOfMemoryError allocation = new OutOfMemoryError("Cannot reserve 758132258 bytes of direct buffer memory (allocated: 0, limit: 100663296)");
        StackTraceElement[] frames = new StackTraceElement[30];
        Arrays.fill(frames, new StackTraceElement("vendor.PacketReader", "readPacket", "private-password.java", 42));
        allocation.setStackTrace(frames);
        SQLException wrapper = new SQLException("driver failed password=private-password", "08006", allocation);
        String result = JdbcErrors.format("建立连接／握手认证", wrapper, "private-password", "private-user", "");
        assertTrue(JdbcErrors.isMemoryFailure(wrapper));
        assertTrue(result.contains("723.0 MiB")); assertTrue(result.contains("上限 96.0 MiB"));
        assertTrue(result.contains("不能判断应当增大内存"));
        assertEquals(8, result.split("vendor.PacketReader.readPacket:42", -1).length - 1);
        assertFalse(result.contains("private-password")); assertFalse(result.contains(".java"));
        assertTrue(result.length() <= 5001);
    }
    @Test public void reportsMissingMemoryCauseWithoutInventingStack() {
        SQLException wrapper = new SQLException("java.lang.OutOfMemoryError: Cannot reserve 758132258 bytes of direct buffer memory (allocated: 0, limit: 100663296)");
        String result = JdbcErrors.format("连接", wrapper, "", "", "");
        assertTrue(JdbcErrors.isMemoryFailure(wrapper));
        assertTrue(result.contains("未保留原始内存分配调用栈"));
        assertTrue(result.contains("外层异常调用位置，不代表分配栈"));
    }
    @Test public void distinguishesHeapFromDirectMemoryFailure() {
        String result = JdbcErrors.format("执行 SQL", new OutOfMemoryError("Java heap space"), "", "", "");
        assertTrue(result.contains("内存分配失败"));
        assertFalse(result.contains("直接内存分配失败"));
        assertFalse(JdbcErrors.isMemoryFailure(new SQLException("ordinary failure")));
    }
    @Test public void retainsVendorAllocationFrameUnderDeepNioStack() {
        OutOfMemoryError allocation = new OutOfMemoryError("direct buffer memory");
        StackTraceElement[] frames = new StackTraceElement[35];
        Arrays.fill(frames, new StackTraceElement("java.nio.Bits", "reserveMemory", "private-path.java", 123));
        frames[30] = new StackTraceElement("vendor.PacketReader", "readPacket", "private-path.java", 42);
        allocation.setStackTrace(frames);
        String text = JdbcErrors.format("建立连接／握手认证", allocation, "", "", "");
        assertTrue(text.contains("vendor.PacketReader.readPacket:42"));
        assertEquals("memory.direct", JdbcErrors.category(allocation));
        assertFalse(text.contains("private-path"));
    }
}
