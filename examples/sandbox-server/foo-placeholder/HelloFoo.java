// Placeholder foo JAR. Lives at /home/tomcat/jar-services/foo.jar until
// orascout pulls and deploys the real one. Listens on :8090 and returns
// a string that makes it obvious it's the placeholder, not your real service.
import java.io.OutputStream;
import java.net.ServerSocket;
import java.net.Socket;

public class HelloFoo {
    public static void main(String[] args) throws Exception {
        int port = 8090;
        ServerSocket srv = new ServerSocket(port);
        System.out.println("foo PLACEHOLDER listening on :" + port);
        String body =
            "foo placeholder — orascout has not deployed a real version yet.\n" +
            "Push your real foo.jar to the configured registry and orascout\n" +
            "will replace this on its next polling cycle.\n";
        byte[] resp = (
            "HTTP/1.0 200 OK\r\n" +
            "Content-Type: text/plain\r\n" +
            "Content-Length: " + body.length() + "\r\n" +
            "Connection: close\r\n\r\n" + body
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
