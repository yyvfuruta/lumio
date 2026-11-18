package io.github.yyvfuruta.lumio;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URI;

/** Streams authenticated originals/ZIPs to a destination chosen by the user. */
public final class MediaDownload {
    private MediaDownload() { }
    public interface Progress { void received(long bytes, long total); }
    public static void save(String server, String target, String cookies, OutputStream destination, Progress progress) throws IOException {
        if (!WebPolicy.downloadAllowed(server, target)) throw new IOException("Download fora do servidor Lumio não permitido.");
        String token = WebPolicy.sessionToken(cookies);
        if (token.isEmpty()) throw new IOException("Entre novamente para baixar os arquivos.");
        HttpURLConnection c = (HttpURLConnection) URI.create(target).toURL().openConnection();
        c.setInstanceFollowRedirects(false); c.setConnectTimeout(20000); c.setReadTimeout(120000);
        c.setRequestProperty("Cookie", "lumio_session=" + token);
        try {
            int status = c.getResponseCode();
            if (status != 200) throw new IOException("Não foi possível baixar o arquivo (HTTP " + status + ").");
            long total = c.getContentLengthLong(), received = 0;
            try (InputStream in = c.getInputStream()) {
                byte[] buffer = new byte[65536]; int n;
                while ((n = in.read(buffer)) != -1) {
                    destination.write(buffer, 0, n); received += n; progress.received(received, total);
                }
            }
            if (total >= 0 && total != received) throw new IOException("Download interrompido. Tente novamente.");
            destination.flush();
        } finally { c.disconnect(); }
    }
}
