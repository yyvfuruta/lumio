package io.github.yyvfuruta.lumio;

import android.app.Activity;
import android.app.AlertDialog;
import android.app.Dialog;
import android.graphics.Color;
import android.net.Uri;
import android.view.Gravity;
import android.view.View;
import android.view.Window;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.ImageButton;
import android.widget.LinearLayout;
import android.widget.MediaController;
import android.widget.TextView;
import android.widget.VideoView;
import java.util.Collections;

final class PhotoViewer extends Dialog {
    private final Activity activity;
    private final GalleryModel model;
    private final PhotoLoader loader;
    private final GalleryTheme theme;
    private final GalleryModel.Download download;
    private final ZoomImageView image;
    private final VideoView video;
    private final TextView title, metadata, position, error;
    private final Button favorite, delete;
    private final LinearLayout actions;
    private String id;
    PhotoViewer(Activity activity, GalleryModel model, PhotoLoader loader, GalleryTheme theme, GalleryModel.Download download, String id) {
        super(activity, android.R.style.Theme_Material_NoActionBar_Fullscreen);
        this.activity = activity; this.model = model; this.loader = loader; this.theme = theme; this.download = download;
        requestWindowFeature(Window.FEATURE_NO_TITLE);
        FrameLayout root = new FrameLayout(activity); root.setBackgroundColor(Color.rgb(17, 17, 17)); setContentView(root);
        image = new ZoomImageView(activity); root.addView(image, new FrameLayout.LayoutParams(-1, -1));
        video = new VideoView(activity); root.addView(video, new FrameLayout.LayoutParams(-1, -1, Gravity.CENTER));
        MediaController controls = new MediaController(activity); controls.setAnchorView(video); video.setMediaController(controls);
        video.setOnPreparedListener(player -> { video.seekTo(1); controls.show(3000); });
        ImageButton close = icon("close", "Fechar foto"); FrameLayout.LayoutParams closeParams = new FrameLayout.LayoutParams(dp(40), dp(40), Gravity.TOP | Gravity.RIGHT); closeParams.setMargins(0, dp(12), dp(16), 0); root.addView(close, closeParams); close.setOnClickListener(v -> dismiss());
        ImageButton previous = icon("left", "Foto anterior"), next = icon("right", "Próxima foto");
        root.addView(previous, new FrameLayout.LayoutParams(dp(40), dp(48), Gravity.CENTER_VERTICAL | Gravity.LEFT));
        root.addView(next, new FrameLayout.LayoutParams(dp(40), dp(48), Gravity.CENTER_VERTICAL | Gravity.RIGHT));
        previous.setOnClickListener(v -> move(-1)); next.setOnClickListener(v -> move(1));
        actions = new LinearLayout(activity); actions.setOrientation(LinearLayout.VERTICAL); actions.setPadding(dp(20), dp(18), dp(20), dp(18)); actions.setBackgroundColor(theme.background);
        FrameLayout.LayoutParams menuParams = new FrameLayout.LayoutParams(Math.min(activity.getResources().getDisplayMetrics().widthPixels - dp(32), dp(350)), -2, Gravity.BOTTOM | Gravity.RIGHT); menuParams.setMargins(dp(16), 0, dp(16), dp(64)); root.addView(actions, menuParams);
        title = label(16, theme.text); title.setTypeface(null, android.graphics.Typeface.BOLD); actions.addView(title);
        metadata = label(13, theme.muted); actions.addView(metadata); position = label(12, theme.muted); actions.addView(position);
        favorite = button("", actions); favorite.setOnClickListener(v -> { Photo p = model.find(this.id); if (p != null) model.favorite(p); });
        button("Baixar original", actions).setOnClickListener(v -> { Photo p = model.find(this.id); if (p != null) download.ready(model.server + "/api/v1/photos/" + p.id + "/original?download=1", p.name, p.type); });
        delete = button("Excluir", actions); delete.setTextColor(0xffc73535); delete.setOnClickListener(v -> {
            Photo p = model.find(this.id); if (p == null) return;
            new AlertDialog.Builder(activity, theme.dialogTheme()).setMessage("Excluir permanentemente “" + p.name + "”?")
                    .setPositiveButton("Excluir", (dialog, which) -> model.delete(Collections.singletonList(p.id))).setNegativeButton("Cancelar", null).show();
        });
        error = label(13, Color.WHITE); error.setPadding(dp(16), dp(8), dp(16), dp(8)); FrameLayout.LayoutParams errorParams = new FrameLayout.LayoutParams(-1, -2, Gravity.BOTTOM); errorParams.bottomMargin = dp(16); root.addView(error, errorParams);
        video.setOnErrorListener((player, what, extra) -> { error.setText("Este vídeo não pôde ser reproduzido. Baixe o original."); error.setVisibility(View.VISIBLE); actions.setVisibility(View.VISIBLE); return true; });
        ImageButton more = icon("more", "Ações da foto"); FrameLayout.LayoutParams moreParams = new FrameLayout.LayoutParams(dp(40), dp(40), Gravity.RIGHT | Gravity.BOTTOM); moreParams.setMargins(0, 0, dp(16), dp(14)); root.addView(more, moreParams); more.setOnClickListener(v -> actions.setVisibility(actions.getVisibility() == View.VISIBLE ? View.GONE : View.VISIBLE));
        actions.setVisibility(View.GONE);
        root.setOnApplyWindowInsetsListener((v, insets) -> { closeParams.topMargin = dp(12) + insets.getSystemWindowInsetTop(); close.setLayoutParams(closeParams); moreParams.bottomMargin = dp(14) + insets.getSystemWindowInsetBottom(); more.setLayoutParams(moreParams); menuParams.bottomMargin = dp(64) + insets.getSystemWindowInsetBottom(); actions.setLayoutParams(menuParams); return insets; });
        setOnDismissListener(dialog -> { video.stopPlayback(); image.setImageDrawable(null); });
        showPhoto(id);
    }
    @Override public void show() { super.show(); getWindow().setLayout(-1, -1); }
    private int dp(int value) { return Math.round(value * activity.getResources().getDisplayMetrics().density); }
    private TextView label(int size, int color) { TextView v = new TextView(activity); v.setTextSize(size); v.setTextColor(color); return v; }
    private ImageButton icon(String kind, String description) { ImageButton b = new ImageButton(activity); b.setImageDrawable(new GalleryIcon(kind, Color.WHITE)); b.setPadding(dp(10), dp(10), dp(10), dp(10)); b.setBackgroundColor(0x88222222); b.setContentDescription(description); return b; }
    private Button button(String label, LinearLayout parent) { Button b = new Button(activity); b.setText(label); b.setAllCaps(false); b.setTextColor(theme.text); b.setBackgroundTintList(android.content.res.ColorStateList.valueOf(theme.surface)); parent.addView(b, new LinearLayout.LayoutParams(-1, -2)); return b; }
    void refresh() {
        Photo p = model.find(id); if (p == null) { dismiss(); return; }
        title.setText(p.name); metadata.setText(p.width + " × " + p.height + " · " + android.text.format.Formatter.formatFileSize(activity, p.size) + "\n" + LibraryDates.label(LibraryDates.day(p.addedAt, model.zone), model.zone));
        favorite.setText(p.favorite ? "♥ Remover dos favoritos" : "♡ Adicionar aos favoritos"); favorite.setEnabled(!model.favoriteBusy.contains(id) && !model.mutating); delete.setEnabled(!model.mutating && !model.transferring);
        position.setText((model.photos.indexOf(p) + 1) + " / " + model.photos.size());
    }
    private void showPhoto(String nextId) {
        Photo p = model.find(nextId); if (p == null) return; id = nextId; video.stopPlayback(); image.setImageDrawable(null); error.setVisibility(View.GONE);
        image.setVisibility(p.video() ? View.GONE : View.VISIBLE); video.setVisibility(p.video() ? View.VISIBLE : View.GONE);
        if (p.video()) {
            video.setVideoURI(Uri.parse(model.server + "/api/v1/photos/" + p.id + "/original"), Collections.singletonMap("Cookie", "lumio_session=" + model.token));
        } else { error.setText("Carregando…"); error.setVisibility(View.VISIBLE); loader.bind(p, true, image, error); }
        refresh();
    }
    private void move(int offset) { Photo p = model.find(id); if (p == null) return; int index = model.photos.indexOf(p) + offset; if (index >= 0 && index < model.photos.size()) showPhoto(model.photos.get(index).id); }
    void pause() { if (video.isPlaying()) video.pause(); }
}
