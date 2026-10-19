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
        String hint;
        if (lower.contains("public key retrieval is not allowed")) hint = "服务器要求完整密码验证。请启用可信 TLS，或在受信任网络／SSH 通道中显式允许获取服务器 RSA 公钥。";
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
        context += "\n" + hint + "\n" + String.join("\n原因：", details);
        return context.length() > 5000 ? context.substring(0, 5000) + "…" : context;
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
