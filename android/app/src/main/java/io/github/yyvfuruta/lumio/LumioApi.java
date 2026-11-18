package io.github.yyvfuruta.lumio;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpCookie;
import java.net.HttpURLConnection;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.UUID;

/** HTTP transport shared by the Android app and JVM smoke tests. */
public final class LumioApi {
    public interface Progress { void sent(long bytes); }
    public static final class Reply {
        public final String body;
        public final String token;
        public final long expiresAt;
        Reply(String body, String token, long expiresAt) {
            this.body = body; this.token = token; this.expiresAt = expiresAt;
        }
    }
    public static final class ApiException extends IOException {
        public final int status;
        ApiException(int status, String body) { super(body); this.status = status; }
    }
    public static String normalize(String raw, boolean allowHTTP) {
        URI uri;
        try { uri = URI.create(raw.trim()); } catch (IllegalArgumentException e) {
            throw new IllegalArgumentException("Endereço do servidor inválido.");
        }
        if (uri.getHost() == null || uri.getRawUserInfo() != null
                || uri.getRawQuery() != null || uri.getRawFragment() != null
                || !("https".equals(uri.getScheme()) || (allowHTTP && "http".equals(uri.getScheme())))) {
            throw new IllegalArgumentException("Use https://seu-servidor. Para HTTP local, habilite a opção abaixo.");
        }
        String result = uri.toASCIIString();
        while (result.endsWith("/")) result = result.substring(0, result.length() - 1);
        return result;
    }
    private final String base;
    private final String token;
    public LumioApi(String base, String token) { this.base = base; this.token = token; }

    private HttpURLConnection connect(String path, String method) throws IOException {
        HttpURLConnection c = (HttpURLConnection) URI.create(base + path).toURL().openConnection();
        c.setInstanceFollowRedirects(false);
        c.setConnectTimeout(20000); c.setReadTimeout(120000);
        c.setRequestMethod(method);
        c.setRequestProperty("X-Lumio-Request", "1");
        c.setRequestProperty("Accept", "application/json");
        if (token != null && !token.isEmpty()) c.setRequestProperty("Cookie", "lumio_session=" + token);
        return c;
    }
    public Reply json(String path, String method, String json) throws IOException {
        HttpURLConnection c = connect(path, method);
        try {
            if (json != null) {
                c.setDoOutput(true);
                c.setRequestProperty("Content-Type", "application/json");
                byte[] data = json.getBytes(StandardCharsets.UTF_8);
                c.setFixedLengthStreamingMode(data.length);
                try (OutputStream out = c.getOutputStream()) { out.write(data); }
            }
            return finish(c);
        } finally { c.disconnect(); }
    }
    public Reply upload(String filename, long size, InputStream source, Progress progress) throws IOException {
        String boundary = "Lumio" + UUID.randomUUID().toString().replace("-", "");
        // Escape provider-controlled names before placing them in MIME headers.
        filename = filename.replace("\\", "_").replace("\"", "_").replace("\r", "_").replace("\n", "_");
        byte[] head = ("--" + boundary + "\r\nContent-Disposition: form-data; name=\"photo\"; filename=\""
                + filename + "\"\r\nContent-Type: application/octet-stream\r\n\r\n").getBytes(StandardCharsets.UTF_8);
        byte[] tail = ("\r\n--" + boundary + "--\r\n").getBytes(StandardCharsets.UTF_8);
        HttpURLConnection c = connect("/api/v1/photos", "POST");
        try {
            c.setDoOutput(true);
            c.setRequestProperty("Content-Type", "multipart/form-data; boundary=" + boundary);
            if (size >= 0) c.setFixedLengthStreamingMode(size + head.length + tail.length);
            else c.setChunkedStreamingMode(65536);
            try (OutputStream out = c.getOutputStream()) {
                out.write(head);
                byte[] buffer = new byte[65536]; int n; long sent = 0;
                while ((n = source.read(buffer)) != -1) {
                    if (Thread.currentThread().isInterrupted()) throw new IOException("Envio interrompido.");
                    out.write(buffer, 0, n); sent += n; progress.sent(sent);
                }
                out.write(tail);
            } catch (IOException failure) {
                // A full server rejection (e.g. 413) may arrive before all bytes are sent.
                try { if (c.getResponseCode() >= 400) return finish(c); }
                catch (ApiException rejected) { throw rejected; }
                catch (IOException ignored) { /* Preserve the original transport error. */ }
                throw failure;
            }
            return finish(c);
        } finally { c.disconnect(); }
    }
    private Reply finish(HttpURLConnection c) throws IOException {
        int status = c.getResponseCode();
        InputStream stream = status >= 200 && status < 300 ? c.getInputStream() : c.getErrorStream();
        String body = "";
        if (stream != null) try (InputStream in = stream; ByteArrayOutputStream out = new ByteArrayOutputStream()) {
            byte[] buffer = new byte[4096]; int n;
            while ((n = in.read(buffer)) != -1) {
                if (out.size() + n > 1024 * 1024) throw new IOException("Resposta do servidor muito grande.");
                out.write(buffer, 0, n);
            }
            body = out.toString("UTF-8");
        }
        if (status < 200 || status >= 300) throw new ApiException(status, body);
        String session = null; long expires = 0;
        for (java.util.Map.Entry<String, List<String>> header : c.getHeaderFields().entrySet()) {
            if (!"Set-Cookie".equalsIgnoreCase(header.getKey())) continue;
            for (String value : header.getValue()) for (HttpCookie cookie : HttpCookie.parse(value)) {
                if (!"lumio_session".equals(cookie.getName())) continue;
                if (cookie.getSecure() && !base.startsWith("https://"))
                    throw new IOException("O servidor exige HTTPS. Use HTTPS ou LUMIO_SECURE_COOKIE=false no servidor HTTP local.");
                session = cookie.getValue();
                expires = System.currentTimeMillis() + Math.max(0, cookie.getMaxAge()) * 1000;
            }
        }
        return new Reply(body, session, expires);
    }
}
