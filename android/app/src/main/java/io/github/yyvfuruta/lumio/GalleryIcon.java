package io.github.yyvfuruta.lumio;

import android.graphics.Canvas;
import android.graphics.ColorFilter;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PixelFormat;
import android.graphics.drawable.Drawable;

final class GalleryIcon extends Drawable {
    private final String kind;
    private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    GalleryIcon(String kind, int color) { this.kind = kind; paint.setColor(color); paint.setStrokeWidth(1.8f); paint.setStrokeCap(Paint.Cap.ROUND); paint.setStrokeJoin(Paint.Join.ROUND); }
    @Override public void draw(Canvas c) {
        c.save(); c.translate(getBounds().left, getBounds().top); c.scale(getBounds().width() / 24f, getBounds().height() / 24f); paint.setStyle(Paint.Style.STROKE);
        Path p = new Path();
        switch (kind) {
            case "menu": for (int y : new int[]{6, 12, 18}) c.drawLine(4, y, 20, y, paint); break;
            case "upload": p.moveTo(12, 16); p.lineTo(12, 3); p.moveTo(7, 8); p.lineTo(12, 3); p.lineTo(17, 8); p.moveTo(4, 16); p.lineTo(4, 21); p.lineTo(20, 21); p.lineTo(20, 16); c.drawPath(p, paint); break;
            case "close": c.drawLine(6, 6, 18, 18, paint); c.drawLine(18, 6, 6, 18, paint); break;
            case "left": p.moveTo(15, 5); p.lineTo(8, 12); p.lineTo(15, 19); c.drawPath(p, paint); break;
            case "right": p.moveTo(9, 5); p.lineTo(16, 12); p.lineTo(9, 19); c.drawPath(p, paint); break;
            case "more": paint.setStyle(Paint.Style.FILL); for (int x : new int[]{5, 12, 19}) c.drawCircle(x, 12, 1.4f, paint); break;
            default:
                p.moveTo(12, 21); p.cubicTo(9, 18, 2, 13, 2, 8); p.cubicTo(2, 2, 9, 1, 12, 6); p.cubicTo(15, 1, 22, 2, 22, 8); p.cubicTo(22, 13, 15, 18, 12, 21); p.close();
                if (kind.equals("saved")) paint.setStyle(Paint.Style.FILL); c.drawPath(p, paint);
        }
        c.restore();
    }
    @Override public void setAlpha(int alpha) { paint.setAlpha(alpha); }
    @Override public void setColorFilter(ColorFilter filter) { paint.setColorFilter(filter); }
    @Override public int getOpacity() { return PixelFormat.TRANSLUCENT; }
}
