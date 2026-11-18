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
        if (!MediaPolicy.downloadAllowed(server, target)) throw new IOException("Download fora do servidor Lumio não permitido.");
        transfer(server, target, cookies, destination, Long.MAX_VALUE, progress);
    }
    public static void media(String server, String target, String cookies, OutputStream destination, long limit) throws IOException {
        if (!MediaPolicy.mediaAllowed(server, target)) throw new IOException("Endereço de mídia inválido.");
        transfer(server, target, cookies, destination, limit, (bytes, total) -> {});
    }
    private static void transfer(String server, String target, String cookies, OutputStream destination, long limit, Progress progress) throws IOException {
        String token = MediaPolicy.sessionToken(cookies);
        if (token.isEmpty()) throw new IOException("Entre novamente para baixar os arquivos.");
        HttpURLConnection c = (HttpURLConnection) URI.create(target).toURL().openConnection();
        c.setInstanceFollowRedirects(false); c.setConnectTimeout(20000); c.setReadTimeout(120000);
        c.setRequestProperty("Cookie", "lumio_session=" + token);
        try {
            int status = c.getResponseCode();
            if (status != 200) throw new IOException("Não foi possível baixar o arquivo (HTTP " + status + ").");
            long total = c.getContentLengthLong(), received = 0;
            if (total > limit) throw new IOException("Arquivo maior que o limite do visualizador.");
            try (InputStream in = c.getInputStream()) {
                byte[] buffer = new byte[65536]; int n;
                while ((n = in.read(buffer)) != -1) {
                    if (Thread.currentThread().isInterrupted()) throw new IOException("Transferência cancelada.");
                    if (received + n > limit) throw new IOException("Arquivo maior que o limite do visualizador.");
                    destination.write(buffer, 0, n); received += n; progress.received(received, total);
                }
            }
            if (total >= 0 && total != received) throw new IOException("Download interrompido. Tente novamente.");
            destination.flush();
        } finally { c.disconnect(); }
    }
}
