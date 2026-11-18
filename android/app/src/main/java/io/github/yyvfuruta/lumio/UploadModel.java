package io.github.yyvfuruta.lumio;

import android.content.Context;
import android.content.SharedPreferences;
import android.database.Cursor;
import android.net.Uri;
import android.provider.OpenableColumns;
import android.provider.DocumentsContract;
import java.io.OutputStream;
import org.json.JSONArray;
import org.json.JSONObject;
import java.io.InputStream;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** Retained state; holds the application context, never a detached Activity. */
final class UploadModel {
    interface Observer { void changed(); }
    final Context context;
    final SessionStore sessions;
    final SharedPreferences prefs;
    final List<Uri> pending = new ArrayList<>();
    final ExecutorService worker = Executors.newSingleThreadExecutor();
    volatile Observer observer;
    volatile boolean busy;
    volatile boolean signedIn;
    volatile String message = "Conecte seu servidor para começar.";
    volatile String detail = "";
    volatile int progress;
    volatile long maxBytes = 32L << 20;
    volatile int libraryRevision;
    volatile int loginRevision;
    private volatile String base;
    private volatile String token;

    UploadModel(Context context) {
        this.context = context.getApplicationContext();
        sessions = new SessionStore(this.context);
        prefs = context.getSharedPreferences("settings", Context.MODE_PRIVATE);
        base = sessions.url(); token = sessions.token(); signedIn = !token.isEmpty();
        try {
            JSONArray items = new JSONArray(prefs.getString("pending", "[]"));
            for (int i = 0; i < items.length(); i++) pending.add(Uri.parse(items.getString(i)));
        } catch (Exception ignored) { /* An invalid local queue starts empty. */ }
        if (signedIn) message = "Conectado. Selecione fotos e vídeos para enviar.";
    }
    void notifyUI() { Observer current = observer; if (current != null) current.changed(); }
    private synchronized void persistQueue() {
        JSONArray items = new JSONArray(); for (Uri uri : pending) items.put(uri.toString());
        prefs.edit().putString("pending", items.toString()).apply();
    }
    synchronized int count() { return pending.size(); }
    synchronized void add(Uri uri) { if (!pending.contains(uri)) pending.add(uri); persistQueue(); }
    synchronized void clearQueue() {
        for (Uri uri : pending) release(uri);
        pending.clear(); persistQueue(); notifyUI();
    }
    private void release(Uri uri) {
        try { context.getContentResolver().releasePersistableUriPermission(uri, android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION); }
        catch (SecurityException ignored) { }
    }
    void login(String endpoint, String password) {
        if (busy) return;
        busy = true; message = "Conectando…"; detail = ""; notifyUI();
        worker.execute(() -> {
            try {
                LumioApi api = new LumioApi(endpoint, "");
                LumioApi.Reply reply = api.json("/api/v1/sessions", "POST", new JSONObject().put("password", password).toString());
                if (reply.token == null || !reply.token.matches("[0-9a-f]{64}")) throw new Exception("O servidor não retornou uma sessão válida.");
                // Revoke an old server session after the new one has been accepted.
                String oldBase = base, oldToken = token;
                sessions.save(endpoint, reply.token, reply.expiresAt);
                base = endpoint; token = reply.token; signedIn = true;
                prefs.edit().putString("url", endpoint).apply();
                if (!oldToken.isEmpty()) try { new LumioApi(oldBase, oldToken).json("/api/v1/sessions/current", "DELETE", null); } catch (Exception ignored) { }
                refreshLimit();
                loginRevision++;
                message = "Conectado. Suas fotos continuam no seu servidor.";
            } catch (Exception e) { message = describe(e); }
            finally { busy = false; notifyUI(); }
        });
    }
    private void refreshLimit() {
        try {
            JSONObject state = new JSONObject(new LumioApi(base, token).json("/api/v1/sessions/current", "GET", null).body);
            if (!state.optBoolean("authenticated")) throw new LumioApi.ApiException(401, "");
            maxBytes = state.getLong("maxUploadBytes");
        } catch (Exception e) {
            if (e instanceof LumioApi.ApiException && ((LumioApi.ApiException) e).status == 401) {
                sessions.clear(); token = ""; signedIn = false;
            }
        }
    }
    void logout() {
        if (busy) return;
        busy = true; message = "Saindo…"; notifyUI();
        worker.execute(() -> {
            try { new LumioApi(base, token).json("/api/v1/sessions/current", "DELETE", null); message = "Sessão encerrada."; }
            catch (Exception e) { message = "Sessão removida deste celular. Não foi possível revogá-la no servidor."; }
            finally { sessions.clear(); token = ""; signedIn = false; busy = false; notifyUI(); }
        });
    }
    void upload() {
        if (busy || !signedIn || count() == 0) return;
        busy = true; progress = 0; detail = ""; message = "Preparando envio…"; notifyUI();
        worker.execute(() -> {
            int sent = 0, duplicate = 0, failures = 0;
            StringBuilder log = new StringBuilder();
            List<Uri> batch;
            synchronized (this) { batch = new ArrayList<>(pending); }
            try {
                refreshLimit();
                if (!signedIn) throw new Exception("Sessão expirada. Entre novamente.");
                LumioApi api = new LumioApi(base, token);
                for (int i = 0; i < batch.size(); i++) {
                    Uri uri = batch.get(i);
                    String name = "foto"; long size = -1;
                    try (Cursor cursor = context.getContentResolver().query(uri,
                            new String[]{OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE}, null, null, null)) {
                        if (cursor != null && cursor.moveToFirst()) {
                            if (!cursor.isNull(0)) name = cursor.getString(0);
                            if (!cursor.isNull(1)) size = cursor.getLong(1);
                        }
                    } catch (Exception ignored) { }
                    final int position = i; final long total = size;
                    message = "Enviando " + (i + 1) + " de " + batch.size() + ": " + name;
                    notifyUI();
                    try {
                        if (size > maxBytes) throw new Exception("excede o limite de " + (maxBytes >> 20) + " MiB do servidor");
                        try (InputStream in = context.getContentResolver().openInputStream(uri)) {
                            if (in == null) throw new Exception("arquivo indisponível");
                            LumioApi.Reply reply = api.upload(name, size, in, bytes -> {
                                int p = (int) ((position + (total > 0 ? Math.min(1.0, (double) bytes / total) : 0)) * 100 / batch.size());
                                if (p != progress) { progress = p; notifyUI(); }
                            });
                            JSONObject result = new JSONObject(reply.body);
                            // Only remove a queue entry after a valid success response.
                            if (!result.has("photo")) throw new Exception("resposta de upload inválida");
                            boolean exists = result.optBoolean("duplicate");
                            if (exists) duplicate++; else sent++;
                            log.append(exists ? "Já existia: " : "Enviado: ").append(name).append('\n');
                        }
                        synchronized (this) { pending.remove(uri); persistQueue(); }
                        release(uri);
                    } catch (Exception e) {
                        failures++; log.append("Falhou: ").append(name).append(" — ").append(describe(e)).append('\n');
                        if (e instanceof LumioApi.ApiException && ((LumioApi.ApiException) e).status == 401) {
                            sessions.clear(); token = ""; signedIn = false; break;
                        }
                    }
                    detail = log.toString(); progress = (i + 1) * 100 / batch.size(); notifyUI();
                }
                message = sent + " enviado(s) · " + duplicate + " já existente(s) · " + failures + " falha(s).";
                if (count() > 0) message += " Os pendentes podem ser reenviados.";
                detail = log.toString();
                if (sent > 0) libraryRevision++;
            } catch (Exception e) { message = describe(e); }
            finally { busy = false; notifyUI(); }
        });
    }
    String endpoint() { return base; }
    String sessionToken() { return token; }
    void adoptBrowserSession(String endpoint, String cookies) {
        if (busy || !endpoint.equals(base) || worker.isShutdown()) return;
        String fresh = WebPolicy.sessionToken(cookies);
        if (fresh.equals(token)) return;
        String previous = token;
        worker.execute(() -> {
            if (busy || !endpoint.equals(base) || !previous.equals(token) || fresh.equals(token)) return;
            try {
                if (fresh.isEmpty()) { sessions.clear(); token = ""; signedIn = false; }
                else {
                    sessions.save(endpoint, fresh, System.currentTimeMillis() + 7L * 24 * 3600 * 1000);
                    token = fresh; signedIn = true;
                }
            } catch (Exception e) { message = "Não foi possível salvar a sessão neste celular."; }
            notifyUI();
        });
    }
    void download(String endpoint, String target, String cookies, Uri destination) {
        if (busy || !endpoint.equals(base)) return;
        busy = true; progress = 0; message = "Baixando arquivo…"; detail = ""; notifyUI();
        worker.execute(() -> {
            try (OutputStream out = context.getContentResolver().openOutputStream(destination, "w")) {
                if (out == null) throw new Exception("Não foi possível abrir o arquivo de destino.");
                final long[] lastUpdate = {0};
                MediaDownload.save(endpoint, target, cookies, out, (bytes, total) -> {
                    if (bytes - lastUpdate[0] < 256 * 1024 && bytes != total) return;
                    lastUpdate[0] = bytes;
                    progress = total > 0 ? (int) Math.min(100, bytes * 100 / total) : 0;
                    message = "Baixando arquivo… " + (bytes / 1024) + " KiB";
                    notifyUI();
                });
                // A provider close can still fail; success is set after closing below.
            } catch (Exception e) {
                message = describe(e);
                try { DocumentsContract.deleteDocument(context.getContentResolver(), destination); } catch (Exception ignored) { }
                busy = false; notifyUI(); return;
            }
            progress = 100; message = "Arquivo salvo no local escolhido."; busy = false; notifyUI();
        });
    }
    static String describe(Exception e) {
        if (e instanceof LumioApi.ApiException) {
            LumioApi.ApiException api = (LumioApi.ApiException) e;
            if (api.status == 401) return "Sessão inválida ou senha incorreta. Entre novamente.";
            if (api.status == 413) return "Arquivo maior que o limite do servidor.";
            if (api.status == 429) return "Muitas tentativas. Aguarde um minuto.";
            if (api.status == 503) return "Servidor ocupado ou processamento indisponível. Tente novamente.";
            try { return new JSONObject(api.getMessage()).optString("error", "Erro HTTP " + api.status); }
            catch (Exception ignored) { return "Erro HTTP " + api.status; }
        }
        if (e instanceof javax.net.ssl.SSLException) return "Não foi possível validar o certificado HTTPS do servidor.";
        return e.getMessage() == null ? "Não foi possível conectar ao servidor." : e.getMessage();
    }
}
