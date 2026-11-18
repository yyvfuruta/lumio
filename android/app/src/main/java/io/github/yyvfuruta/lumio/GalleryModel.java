package io.github.yyvfuruta.lumio;

import android.os.Handler;
import android.os.Looper;
import org.json.JSONArray;
import org.json.JSONObject;
import java.net.URLEncoder;
import java.time.ZoneId;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** Native library use cases/state. All view state is applied on the main thread. */
final class GalleryModel {
    interface Observer { void changed(); void expired(); }
    interface Download { void ready(String url, String name, String mime); }
    final String server, token;
    final ZoneId zone = ZoneId.systemDefault();
    final List<Photo> photos = new ArrayList<>();
    final List<String> months = new ArrayList<>();
    final LinkedHashMap<String, Photo> selection = new LinkedHashMap<>();
    final Set<String> favoriteBusy = new HashSet<>();
    String query = "", period = "", cursor = "", message = "";
    boolean favorite, loading, mutating, selecting, statusLoading, transferring;
    final Set<String> completeDays = new HashSet<>();
    JSONObject status;
    Observer observer;
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private final LumioApi api;
    private int generation;
    private boolean closed;
    GalleryModel(String server, String token, Observer observer) {
        this.server = server; this.token = token; this.observer = observer; api = new LumioApi(server, token);
    }
    void notifyUI() { if (!closed && observer != null) observer.changed(); }
    private void failed(Exception e) {
        message = UploadModel.describe(e);
        if (e instanceof LumioApi.ApiException && ((LumioApi.ApiException) e).status == 401 && observer != null) observer.expired();
    }
    void filter(String text, boolean favorites, String date) {
        if (mutating) return;
        query = text; favorite = favorites; period = date; load(true);
    }
    void load(boolean reset) {
        if (closed || (!reset && (loading || cursor.isEmpty()))) return;
        if (query.getBytes(java.nio.charset.StandardCharsets.UTF_8).length > 200) { message = "A busca pode ter até 200 bytes."; notifyUI(); return; }
        if (reset) { generation++; photos.clear(); months.clear(); selection.clear(); completeDays.clear(); selecting = false; cursor = ""; }
        final int current = generation;
        final String params = parameters(reset ? "" : cursor, period), timeline = timelineParameters();
        loading = true; message = ""; notifyUI();
        worker.execute(() -> {
            try {
                JSONObject page = new JSONObject(api.json("/api/v1/photos?" + params, "GET", null).body);
                List<Photo> incoming = parsePhotos(page.getJSONArray("photos"));
                String next = page.optString("nextCursor");
                List<String> dates = new ArrayList<>();
                if (reset) {
                    JSONObject groups = new JSONObject(api.json("/api/v1/photos/timeline?" + timeline, "GET", null).body);
                    JSONArray items = groups.getJSONArray("months");
                    for (int i = 0; i < items.length(); i++) { String month = items.getJSONObject(i).getString("month"); java.time.YearMonth.parse(month); dates.add(month); }
                }
                main.post(() -> {
                    if (closed || current != generation) return;
                    Set<String> seen = new HashSet<>(); for (Photo p : photos) seen.add(p.id);
                    for (Photo p : incoming) if (seen.add(p.id)) photos.add(p);
                    if (!next.isEmpty() && next.equals(cursor)) { cursor = ""; message = "O servidor repetiu a página. Atualize a biblioteca."; }
                    else cursor = next;
                    if (reset) { months.clear(); months.addAll(dates); }
                    loading = false; notifyUI();
                });
            } catch (Exception e) {
                main.post(() -> { if (!closed && current == generation) { loading = false; failed(e); notifyUI(); } });
            }
        });
    }
    private String parameters(String next, String date) {
        String[] range = LibraryDates.range(date, zone);
        return "q=" + encode(query) + "&favorite=" + favorite + (next.isEmpty() ? "" : "&cursor=" + encode(next))
                + "&from=" + encode(range[0]) + "&until=" + encode(range[1]);
    }
    private String timelineParameters() { return "q=" + encode(query) + "&favorite=" + favorite + "&tz=" + encode(zone.getId()); }
    private static String encode(String value) {
        try { return URLEncoder.encode(value, "UTF-8"); } catch (Exception impossible) { throw new IllegalArgumentException(impossible); }
    }
    private static List<Photo> parsePhotos(JSONArray array) throws Exception {
        List<Photo> items = new ArrayList<>(); for (int i = 0; i < array.length(); i++) items.add(new Photo(array.getJSONObject(i))); return items;
    }
    Photo find(String id) { for (Photo p : photos) if (p.id.equals(id)) return p; return null; }
    void favorite(Photo photo) {
        if (closed || mutating || !favoriteBusy.add(photo.id)) return;
        notifyUI();
        worker.execute(() -> {
            try {
                Photo updated = new Photo(new JSONObject(api.json("/api/v1/photos/" + photo.id, "PATCH", new JSONObject().put("favorite", !photo.favorite).toString()).body));
                main.post(() -> {
                    if (closed) return;
                    for (int i = 0; i < photos.size(); i++) if (photos.get(i).id.equals(updated.id)) {
                        if (favorite && !updated.favorite) { photos.remove(i); selection.remove(updated.id); }
                        else photos.set(i, updated);
                        break;
                    }
                    favoriteBusy.remove(photo.id); notifyUI();
                });
            } catch (Exception e) { main.post(() -> { if (!closed) { favoriteBusy.remove(photo.id); failed(e); notifyUI(); } }); }
        });
    }
    void toggle(Photo p, boolean checked) {
        if (mutating) return;
        if (checked && !selection.containsKey(p.id) && selection.size() >= 1000) { message = "Selecione no máximo 1000 arquivos."; notifyUI(); return; }
        selecting = true;
        if (checked) selection.put(p.id, p); else { selection.remove(p.id); completeDays.remove(LibraryDates.day(p.addedAt, zone)); }
        notifyUI();
    }
    void clearSelection() { if (!mutating) { selection.clear(); completeDays.clear(); notifyUI(); } }
    void cancelSelection() { if (!mutating) { selection.clear(); completeDays.clear(); selecting = false; notifyUI(); } }
    boolean daySelected(String day) {
        if (completeDays.contains(day)) return true;
        if (!cursor.isEmpty()) return false;
        boolean any = false;
        for (Photo p : photos) if (LibraryDates.day(p.addedAt, zone).equals(day)) { any = true; if (!selection.containsKey(p.id)) return false; }
        return any;
    }
    void selectAll(String day) {
        if (closed || mutating) return;
        if (day != null && daySelected(day)) {
            selection.values().removeIf(p -> LibraryDates.day(p.addedAt, zone).equals(day)); completeDays.remove(day); selecting = true; notifyUI(); return;
        }
        final int current = generation;
        final String params = parameters("", day == null ? period : day);
        mutating = true; selecting = true; message = "Selecionando arquivos…"; notifyUI();
        worker.execute(() -> {
            try {
                LinkedHashMap<String, Photo> selected = new LinkedHashMap<>(); String next = "";
                Set<String> cursors = new HashSet<>();
                do {
                    if (!cursors.add(next)) throw new Exception("O servidor repetiu uma página.");
                    JSONObject page = new JSONObject(api.json("/api/v1/photos?" + params + "&cursor=" + encode(next), "GET", null).body);
                    for (Photo p : parsePhotos(page.getJSONArray("photos"))) selected.put(p.id, p);
                    if (selected.size() > 1000) throw new Exception("Esta seleção excede 1000 arquivos. Use uma data ou busca mais específica.");
                    next = page.optString("nextCursor");
                } while (!next.isEmpty());
                main.post(() -> {
                    if (closed || current != generation) return;
                    if (day == null) { selection.clear(); completeDays.clear(); }
                    if (selection.size() + selected.values().stream().filter(p -> !selection.containsKey(p.id)).count() > 1000) message = "Selecione no máximo 1000 arquivos.";
                    else { selection.putAll(selected); for (Photo p : selected.values()) completeDays.add(LibraryDates.day(p.addedAt, zone)); message = ""; }
                    mutating = false; notifyUI();
                });
            } catch (Exception e) { main.post(() -> { if (!closed && current == generation) { mutating = false; failed(e); notifyUI(); } }); }
        });
    }
    void delete(List<String> ids) {
        if (closed || mutating || transferring || ids.isEmpty()) return;
        mutating = true; message = "Excluindo arquivos…"; notifyUI();
        worker.execute(() -> {
            List<String> deleted = new ArrayList<>(); Exception failure = null;
            for (String id : ids) {
                try { api.json("/api/v1/photos/" + id, "DELETE", null); deleted.add(id); }
                catch (Exception e) { failure = e; break; }
            }
            final Exception result = failure;
            main.post(() -> {
                if (closed) return;
                completeDays.clear(); photos.removeIf(p -> deleted.contains(p.id)); for (String id : deleted) selection.remove(id);
                mutating = false; message = deleted.size() + " arquivo(s) excluído(s).";
                if (result != null) failed(result);
                if (selection.isEmpty()) selecting = false;
                notifyUI();
            });
        });
    }
    void archive(Download callback) {
        if (closed || mutating || transferring || selection.isEmpty()) return;
        List<String> ids = new ArrayList<>(selection.keySet()); mutating = true; message = "Preparando ZIP…"; notifyUI();
        worker.execute(() -> {
            try {
                JSONObject result = new JSONObject(api.json("/api/v1/photo-archives", "POST", new JSONObject().put("photoIds", new JSONArray(ids)).toString()).body);
                String path = result.getString("url"), url = java.net.URI.create(server + "/").resolve(path).toString();
                if (!MediaPolicy.downloadAllowed(server, url)) throw new Exception("O servidor retornou um endereço de download inválido.");
                main.post(() -> { if (!closed) { mutating = false; message = ""; notifyUI(); callback.ready(url, "lumio-fotos.zip", "application/zip"); } });
            } catch (Exception e) { main.post(() -> { if (!closed) { mutating = false; failed(e); notifyUI(); } }); }
        });
    }
    void loadStatus() {
        if (closed || statusLoading) return;
        statusLoading = true; message = ""; notifyUI();
        worker.execute(() -> {
            try {
                JSONObject data = new JSONObject(api.json("/api/v1/status", "GET", null).body);
                main.post(() -> { if (!closed) { status = data; statusLoading = false; notifyUI(); } });
            } catch (Exception e) { main.post(() -> { if (!closed) { statusLoading = false; failed(e); notifyUI(); } }); }
        });
    }
    void close() { closed = true; observer = null; main.removeCallbacksAndMessages(null); worker.shutdownNow(); }
}
