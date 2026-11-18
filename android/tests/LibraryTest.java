import io.github.yyvfuruta.lumio.MediaPolicy;
import io.github.yyvfuruta.lumio.MediaDownload;
import com.sun.net.httpserver.HttpServer;
import java.io.ByteArrayOutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.atomic.AtomicInteger;

public final class LibraryTest {
    public static void main(String[] args) throws Exception {
        String origin = "https://photos.example.com";
        for (String url : new String[]{origin + "/", origin + "/api/v1/photos", "https://PHOTOS.example.com:443/preview"}) {
            if (!MediaPolicy.sameOrigin(origin, url)) throw new AssertionError("same origin denied: " + url);
        }
        for (String url : new String[]{"http://photos.example.com/", "https://photos.example.com.evil/", "https://photos.example.com:444/", "https://user@photos.example.com/", "file:///tmp/photo", "content://photos.example.com/x", "javascript:alert(1)", "not a URL"}) {
            if (MediaPolicy.sameOrigin(origin, url)) throw new AssertionError("foreign origin accepted: " + url);
        }
        String id = "b".repeat(64), token = "a".repeat(64);
        if (!MediaPolicy.sessionToken("other=x; lumio_session=" + token + "; more=x").equals(token)) throw new AssertionError("cookie parsing");
        if (!MediaPolicy.sessionToken("lumio_session=bad\r\nCookie: injected").isEmpty()) throw new AssertionError("cookie injection");
        if (!MediaPolicy.filename("../../phone\n.jpg").equals("phone_.jpg")) throw new AssertionError("unsafe filename");
        HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        AtomicInteger downloaded = new AtomicInteger();
        server.createContext("/api/v1/photos/" + id + "/original", exchange -> {
            if (!("lumio_session=" + token).equals(exchange.getRequestHeaders().getFirst("Cookie"))) {
                exchange.sendResponseHeaders(401, -1); exchange.close(); return;
            }
            byte[] original = "original photo bytes".getBytes(StandardCharsets.UTF_8);
            downloaded.incrementAndGet(); exchange.sendResponseHeaders(200, original.length);
            exchange.getResponseBody().write(original); exchange.close();
        });
        server.createContext("/api/v1/photo-archives/" + "c".repeat(32), exchange -> {
            byte[] zip = "streamed archive".getBytes(StandardCharsets.UTF_8);
            exchange.sendResponseHeaders(200, 0); exchange.getResponseBody().write(zip); exchange.close();
        });
        server.createContext("/api/v1/photo-archives/" + "d".repeat(32), exchange -> {
            exchange.getResponseHeaders().set("Location", "/api/v1/photos/" + id + "/original");
            exchange.sendResponseHeaders(302, -1); exchange.close();
        });
        server.createContext("/api/v1/photo-archives/" + "e".repeat(32), exchange -> {
            exchange.sendResponseHeaders(200, 100); exchange.getResponseBody().write(new byte[]{1, 2}); exchange.close();
        });
        server.start();
        try {
            String base = "http://127.0.0.1:" + server.getAddress().getPort();
            String path = "/api/v1/photos/" + id + "/original?download=1";
            ByteArrayOutputStream out = new ByteArrayOutputStream();
            MediaDownload.save(base, base + path, "lumio_session=" + token, out, (bytes, total) -> {});
            if (!out.toString("UTF-8").equals("original photo bytes")) throw new AssertionError("changed original");
            out.reset();
            MediaDownload.save(base, base + "/api/v1/photo-archives/" + "c".repeat(32), "lumio_session=" + token, out, (bytes, total) -> {});
            if (!out.toString("UTF-8").equals("streamed archive")) throw new AssertionError("unknown-length stream");
            for (String target : new String[]{"http://localhost:" + server.getAddress().getPort() + path,
                    base + "/api/v1/photos", base + "/api/v1/photo-archives/" + "d".repeat(32), base + "/api/v1/photo-archives/" + "e".repeat(32)}) {
                try {
                    MediaDownload.save(base, target, "lumio_session=" + token, new ByteArrayOutputStream(), (bytes, total) -> {});
                    throw new AssertionError("unsafe or incomplete download accepted: " + target);
                } catch (java.io.IOException expected) { }
            }
            if (downloaded.get() != 1) throw new AssertionError("followed redirect or leaked credential");
            System.out.println("Android library/download tests passed");
        } finally { server.stop(0); }
    }
}
