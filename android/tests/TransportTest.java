import io.github.yyvfuruta.lumio.LumioApi;
import com.sun.net.httpserver.HttpServer;
import java.io.ByteArrayInputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.atomic.AtomicInteger;

public final class TransportTest {
    public static void main(String[] args) throws Exception {
        HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        AtomicInteger uploads = new AtomicInteger();
        server.createContext("/api/v1/sessions", exchange -> {
            byte[] body = exchange.getRequestBody().readAllBytes();
            if (!new String(body, StandardCharsets.UTF_8).equals("{\"password\":\"test\"}")
                    || !"1".equals(exchange.getRequestHeaders().getFirst("X-Lumio-Request"))) {
                exchange.sendResponseHeaders(400, -1); exchange.close(); return;
            }
            exchange.getResponseHeaders().add("Set-Cookie", "lumio_session=" + "a".repeat(64) + "; Path=/; Max-Age=604800; HttpOnly");
            byte[] result = "{\"authenticated\":true}".getBytes(StandardCharsets.UTF_8);
            exchange.sendResponseHeaders(201, result.length); exchange.getResponseBody().write(result); exchange.close();
        });
        server.createContext("/api/v1/photos", exchange -> {
            String body = new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
            if (!body.contains("name=\"photo\"") || !body.contains("original bytes")
                    || !body.contains("filename=\"photo__evil.jpg\"")
                    || !exchange.getRequestHeaders().getFirst("Cookie").equals("lumio_session=" + "a".repeat(64))) {
                exchange.sendResponseHeaders(400, -1); exchange.close(); return;
            }
            uploads.incrementAndGet();
            byte[] result = "{\"photo\":{},\"duplicate\":false}".getBytes(StandardCharsets.UTF_8);
            exchange.sendResponseHeaders(201, result.length); exchange.getResponseBody().write(result); exchange.close();
        });
        server.createContext("/redirect", exchange -> {
            exchange.getResponseHeaders().set("Location", "/api/v1/sessions");
            exchange.sendResponseHeaders(302, -1); exchange.close();
        });
        server.start();
        try {
            String base = "http://127.0.0.1:" + server.getAddress().getPort();
            if (!LumioApi.normalize(base + "/", true).equals(base)) throw new AssertionError("URL normalization");
            for (String url : new String[]{base, "https://user:pass@example.com", "file:///tmp", "https://example.com?token=x"}) {
                try { LumioApi.normalize(url, false); throw new AssertionError("unsafe URL accepted"); }
                catch (IllegalArgumentException expected) { }
            }
            LumioApi.Reply login = new LumioApi(base, "").json("/api/v1/sessions", "POST", "{\"password\":\"test\"}");
            if (!login.token.equals("a".repeat(64)) || login.expiresAt < System.currentTimeMillis()) throw new AssertionError("session");
            LumioApi api = new LumioApi(base, login.token);
            for (int size : new int[]{14, -1}) {
                api.upload("photo\r\nevil.jpg", size, new ByteArrayInputStream("original bytes".getBytes(StandardCharsets.UTF_8)), n -> {});
            }
            if (uploads.get() != 2) throw new AssertionError("uploads");
            try { api.json("/redirect", "GET", null); throw new AssertionError("followed redirect"); }
            catch (LumioApi.ApiException expected) { if (expected.status != 302) throw expected; }
            System.out.println("Android transport tests passed");
        } finally { server.stop(0); }
    }
}
