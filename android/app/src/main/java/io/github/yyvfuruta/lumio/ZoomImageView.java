package io.github.yyvfuruta.lumio;

import android.content.Context;
import android.graphics.Matrix;
import android.graphics.drawable.Drawable;
import android.view.GestureDetector;
import android.view.MotionEvent;
import android.view.ScaleGestureDetector;
import android.widget.ImageView;

final class ZoomImageView extends ImageView {
    private final Matrix transform = new Matrix();
    private final ScaleGestureDetector pinch;
    private final GestureDetector taps;
    private float scale, minimum, x, y, lastX, lastY;
    ZoomImageView(Context context) {
        super(context); setScaleType(ScaleType.MATRIX);
        pinch = new ScaleGestureDetector(context, new ScaleGestureDetector.SimpleOnScaleGestureListener() {
            @Override public boolean onScale(ScaleGestureDetector event) { zoom(event.getScaleFactor(), event.getFocusX(), event.getFocusY()); return true; }
        });
        taps = new GestureDetector(context, new GestureDetector.SimpleOnGestureListener() {
            @Override public boolean onDown(MotionEvent event) { return true; }
            @Override public boolean onDoubleTap(MotionEvent event) { if (scale > minimum * 1.1f) fit(); else zoom(2.5f, event.getX(), event.getY()); return true; }
        });
    }
    @Override public void setImageDrawable(Drawable drawable) { super.setImageDrawable(drawable); fit(); }
    @Override protected void onSizeChanged(int w, int h, int oldW, int oldH) { super.onSizeChanged(w, h, oldW, oldH); fit(); }
    private void fit() {
        Drawable image = getDrawable(); if (image == null || getWidth() == 0 || image.getIntrinsicWidth() <= 0) return;
        minimum = Math.min((float) getWidth() / image.getIntrinsicWidth(), (float) getHeight() / image.getIntrinsicHeight());
        scale = minimum; x = (getWidth() - image.getIntrinsicWidth() * scale) / 2; y = (getHeight() - image.getIntrinsicHeight() * scale) / 2; apply();
    }
    private void zoom(float factor, float focusX, float focusY) {
        if (minimum <= 0 || scale <= 0) return;
        float next = Math.max(minimum, Math.min(minimum * 5, scale * factor)), ratio = next / scale;
        x = focusX - (focusX - x) * ratio; y = focusY - (focusY - y) * ratio; scale = next; apply();
    }
    private void apply() {
        Drawable image = getDrawable(); if (image == null) return;
        float w = image.getIntrinsicWidth() * scale, h = image.getIntrinsicHeight() * scale;
        x = w <= getWidth() ? (getWidth() - w) / 2 : Math.max(getWidth() - w, Math.min(0, x));
        y = h <= getHeight() ? (getHeight() - h) / 2 : Math.max(getHeight() - h, Math.min(0, y));
        transform.setScale(scale, scale); transform.postTranslate(x, y); setImageMatrix(transform);
    }
    @Override public boolean onTouchEvent(MotionEvent event) {
        pinch.onTouchEvent(event); taps.onTouchEvent(event);
        if (event.getActionMasked() == MotionEvent.ACTION_DOWN) { lastX = event.getX(); lastY = event.getY(); }
        if (event.getActionMasked() == MotionEvent.ACTION_MOVE && !pinch.isInProgress() && event.getPointerCount() == 1) {
            x += event.getX() - lastX; y += event.getY() - lastY; lastX = event.getX(); lastY = event.getY(); apply();
        }
        return true;
    }
}
