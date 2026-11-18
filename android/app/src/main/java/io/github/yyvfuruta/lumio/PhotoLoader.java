package io.github.yyvfuruta.lumio;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Matrix;
import android.media.ExifInterface;
import android.os.Handler;
import android.os.Looper;
import android.util.LruCache;
import android.view.View;
import android.widget.ImageView;
import android.widget.TextView;
import java.io.File;
import java.io.FileOutputStream;
import java.lang.ref.WeakReference;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** Bounded native bitmap cache; recycled rows never receive another photo's result. */
final class PhotoLoader {
    private final Context context;
    private final String server, cookies;
    private final ExecutorService worker = Executors.newFixedThreadPool(2);
    private final Handler main = new Handler(Looper.getMainLooper());
    private volatile boolean closed;
    private final LruCache<String, Bitmap> cache = new LruCache<String, Bitmap>((int) Math.min(24 * 1024 * 1024L, Runtime.getRuntime().maxMemory() / 12)) {
        @Override protected int sizeOf(String key, Bitmap value) { return value.getByteCount(); }
    };
    PhotoLoader(Context context, String server, String token) { this.context = context.getApplicationContext(); this.server = server; cookies = "lumio_session=" + token; }
    void bind(Photo photo, boolean preview, ImageView target, TextView error) {
        String kind = preview ? "preview" : "thumbnail", key = photo.id + "/" + kind;
        target.setTag(key); target.setImageDrawable(null);
        Bitmap present = cache.get(key);
        if (present != null) { target.setImageBitmap(present); if (error != null) error.setVisibility(View.GONE); return; }
        WeakReference<ImageView> image = new WeakReference<>(target); WeakReference<TextView> problem = new WeakReference<>(error);
        worker.execute(() -> {
            File spool = null;
            try {
                spool = File.createTempFile("lumio-view-", ".media", context.getCacheDir());
                try (FileOutputStream out = new FileOutputStream(spool)) { MediaDownload.media(server, server + "/api/v1/photos/" + key, cookies, out, preview ? 256L << 20 : 4L << 20); }
                BitmapFactory.Options options = new BitmapFactory.Options(); options.inJustDecodeBounds = true; BitmapFactory.decodeFile(spool.getPath(), options);
                int limit = preview ? 2048 : 320; options.inSampleSize = 1;
                while (options.outWidth / options.inSampleSize > limit || options.outHeight / options.inSampleSize > limit) options.inSampleSize *= 2;
                options.inJustDecodeBounds = false; Bitmap decoded = BitmapFactory.decodeFile(spool.getPath(), options);
                if (decoded == null) throw new Exception("Não foi possível abrir esta imagem.");
                Bitmap bitmap = orient(decoded, spool);
                if (closed) return;
                cache.put(key, bitmap);
                main.post(() -> {
                    ImageView view = image.get(); if (closed || view == null || !key.equals(view.getTag())) return;
                    view.setImageBitmap(bitmap); TextView label = problem.get(); if (label != null) label.setVisibility(View.GONE);
                });
            } catch (Exception e) {
                main.post(() -> {
                    ImageView view = image.get(); if (closed || view == null || !key.equals(view.getTag())) return;
                    view.setImageResource(android.R.drawable.ic_dialog_alert);
                    TextView label = problem.get(); if (label != null) { label.setText("Não foi possível carregar. Verifique sua conexão ou baixe o original."); label.setVisibility(View.VISIBLE); }
                });
            } finally { if (spool != null) spool.delete(); }
        });
    }
    private Bitmap orient(Bitmap source, File file) {
        try {
            int value = new ExifInterface(file.getPath()).getAttributeInt(ExifInterface.TAG_ORIENTATION, 1); Matrix m = new Matrix();
            switch (value) {
                case 2: m.setScale(-1, 1); break;
                case 3: m.setRotate(180); break;
                case 4: m.setScale(1, -1); break;
                case 5: m.setRotate(90); m.postScale(-1, 1); break;
                case 6: m.setRotate(90); break;
                case 7: m.setRotate(270); m.postScale(-1, 1); break;
                case 8: m.setRotate(270); break;
                default: return source;
            }
            return Bitmap.createBitmap(source, 0, 0, source.getWidth(), source.getHeight(), m, true);
        } catch (Exception ignored) { return source; }
    }
    void close() { closed = true; worker.shutdownNow(); main.removeCallbacksAndMessages(null); cache.evictAll(); }
}
