package fixtures;

import java.io.IOException;
import java.net.Socket;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.sql.*;
import java.util.Properties;
import java.util.logging.Logger;

/** Test only: a client-first handshake, allowing a peer to reset after receiving it. */
public final class SocketResetDriver implements Driver {
    @Override public Connection connect(String url, Properties properties) throws SQLException {
        if (!acceptsURL(url)) return null;
        URI target = URI.create(url.substring("jdbc:".length()));
        try (Socket socket = new Socket(target.getHost(), target.getPort())) {
            socket.setSoTimeout(2000);
            socket.getOutputStream().write("HELLO".getBytes(StandardCharsets.UTF_8));
            socket.getOutputStream().flush();
            int reply = socket.getInputStream().read();
            throw new SQLException("Failed to read data from socket. detail: " + (reply < 0 ? "end of stream" : "unexpected test reply"), "08001");
        } catch (IOException error) {
            throw new SQLException("Failed to read data from socket. detail: " + error, "08001", error);
        }
    }
    @Override public boolean acceptsURL(String url) { return url != null && url.startsWith("jdbc:reset-probe://"); }
    @Override public DriverPropertyInfo[] getPropertyInfo(String url, Properties info) { return new DriverPropertyInfo[0]; }
    @Override public int getMajorVersion() { return 1; }
    @Override public int getMinorVersion() { return 0; }
    @Override public boolean jdbcCompliant() { return false; }
    @Override public Logger getParentLogger() { return Logger.getGlobal(); }
}
