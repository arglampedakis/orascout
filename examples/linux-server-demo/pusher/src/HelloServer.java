// A minimal Java 8 HTTP server. The Dockerfile compiles this twice — once
// with VERSION_PLACEHOLDER replaced by "v1", once with "v2" — producing two
// distinct JARs with different content (and therefore different OCI digests
// when pushed, so orascout will redeploy).
import java.io.OutputStream;
import java.net.ServerSocket;
import java.net.Socket;

public class HelloServer {
    static final String VERSION = "VERSION_PLACEHOLDER";

    public static void main(String[] args) throws Exception {
        int port = 8082;
        ServerSocket srv = new ServerSocket(port);
        System.out.println("HelloServer " + VERSION + " listening on :" + port);
        String body = "Hello from JAR " + VERSION + "\n";
        byte[] resp = (
            "HTTP/1.0 200 OK\r\n" +
            "Content-Type: text/plain\r\n" +
            "Content-Length: " + body.length() + "\r\n" +
            "Connection: close\r\n" +
            "\r\n" +
            body
        ).getBytes();
        while (true) {
            Socket c = srv.accept();
            try {
                OutputStream os = c.getOutputStream();
                os.write(resp);
                os.flush();
            } catch (Exception ignored) {
            } finally {
                try { c.close(); } catch (Exception ignored) {}
            }
        }
    }
}
