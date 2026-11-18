package io.github.yyvfuruta.lumio;

import android.app.Activity;
import android.content.ClipData;
import android.content.Intent;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Bundle;
import android.text.InputType;
import android.view.View;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.Toast;
import android.widget.CheckBox;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.ScrollView;
import android.widget.TextView;

public final class MainActivity extends Activity implements UploadModel.Observer {
    private static final int PICK_MEDIA = 10, SAVE_DOWNLOAD = 12;
    private static final int INK = 0xff171717, TEAL = 0xff444444, MUTED = 0xff777777;
    private UploadModel model;
    private EditText url, password;
    private CheckBox allowHTTP;
    private Button login, logout, select, upload, clear;
    private TextView status, queue, detail;
    private ProgressBar progress;
    private FrameLayout panels;
    private View uploadPanel;
    private LinearLayout shell, tabs;
    private GalleryView gallery;
    private Button libraryTab, uploadsTab;
    private TextView transferStatus;
    private boolean galleryVisible, previousSignedIn, previousBusy, uploadFromLibrary;
    private int loginRevision, libraryRevision;
    private String downloadURL, downloadServer;


    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        Object retained = getLastNonConfigurationInstance();
        model = retained instanceof UploadModel ? (UploadModel) retained : new UploadModel(getApplicationContext());
        model.observer = this;
        ScrollView scroll = new ScrollView(this); scroll.setFillViewport(true);
        LinearLayout page = new LinearLayout(this); page.setOrientation(LinearLayout.VERTICAL);
        page.setPadding(dp(24), dp(36), dp(24), dp(28));
        scroll.addView(page); uploadPanel = scroll;
        shell = new LinearLayout(this); shell.setOrientation(LinearLayout.VERTICAL);
        panels = new FrameLayout(this); shell.addView(panels, new LinearLayout.LayoutParams(-1, 0, 1));
        panels.addView(scroll, new FrameLayout.LayoutParams(-1, -1));
        transferStatus = text("", 13, INK); transferStatus.setPadding(dp(16), dp(8), dp(16), dp(8)); transferStatus.setVisibility(View.GONE); shell.addView(transferStatus);
        tabs = new LinearLayout(this);
        libraryTab = new Button(this); libraryTab.setText("Biblioteca"); libraryTab.setAllCaps(false); libraryTab.setOnClickListener(v -> showLibrary());
        uploadsTab = new Button(this); uploadsTab.setText("Envios"); uploadsTab.setAllCaps(false); uploadsTab.setOnClickListener(v -> showUploads());
        tabs.addView(libraryTab, new LinearLayout.LayoutParams(0, dp(48), 1)); tabs.addView(uploadsTab, new LinearLayout.LayoutParams(0, dp(48), 1)); shell.addView(tabs);
        setContentView(shell);
        shell.setOnApplyWindowInsetsListener((v, insets) -> { shell.setPadding(0, insets.getSystemWindowInsetTop(), 0, insets.getSystemWindowInsetBottom()); return insets; });
        shell.requestApplyInsets(); loginRevision = model.loginRevision; libraryRevision = model.libraryRevision;
        TextView brand = text("LUMIO", 14, TEAL); brand.setLetterSpacing(.2f); brand.setTypeface(null, Typeface.BOLD); page.addView(brand);
        TextView title = text("Sua biblioteca de fotos.", 28, INK); title.setTypeface(null, Typeface.BOLD); add(page, title, 12);
        add(page, text("Conecte seu servidor para abrir a biblioteca. Gerencie os envios deste celular aqui.", 16, MUTED), 12);

        LinearLayout connection = card(page, 28);
        add(connection, text("01  /  CONECTAR", 13, TEAL), 0);
        url = new EditText(this); url.setId(101); url.setSingleLine(true); url.setHint("https://fotos.exemplo.com");
        url.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        url.setText(model.prefs.getString("url", model.sessions.url())); add(connection, url, 12);
        password = new EditText(this); password.setId(102); password.setSingleLine(true); password.setHint("Senha do Lumio");
        password.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD);
        password.setSaveEnabled(false); password.setImportantForAutofill(View.IMPORTANT_FOR_AUTOFILL_NO);
        add(connection, password, 6);
        allowHTTP = new CheckBox(this); allowHTTP.setText("Permitir HTTP na rede local (sem criptografia)");
        allowHTTP.setTextSize(12); allowHTTP.setChecked(model.prefs.getBoolean("http", false)); add(connection, allowHTTP, 4);
        login = button("Conectar ao servidor", connection, v -> {
            try {
                String endpoint = LumioApi.normalize(url.getText().toString(), allowHTTP.isChecked());
                String secret = password.getText().toString();
                if (secret.isEmpty()) { password.setError("Informe a senha do servidor"); return; }
                model.prefs.edit().putBoolean("http", allowHTTP.isChecked()).apply();
                password.setText(""); model.login(endpoint, secret);
            } catch (IllegalArgumentException e) { url.setError(e.getMessage()); }
        });
        logout = button("Sair da conta", connection, v -> model.logout());

        LinearLayout files = card(page, 18);
        add(files, text("02  /  ENVIAR", 13, TEAL), 0);
        queue = text("", 22, INK); queue.setTypeface(null, Typeface.BOLD); add(files, queue, 14);
        add(files, text("Selecione apenas os arquivos que deseja enviar. Fotos duplicadas não são armazenadas novamente.", 14, MUTED), 8);
        select = button("Selecionar fotos e vídeos", files, v -> pickFiles(false));
        upload = button("Enviar para o Lumio", files, v -> model.upload());
        clear = button("Limpar seleção", files, v -> model.clearQueue());
        progress = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal); progress.setMax(100); add(files, progress, 16);
        status = text("", 15, INK); status.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE); add(files, status, 12);
        detail = text("", 13, MUTED); detail.setTextIsSelectable(true); add(files, detail, 12);
        add(page, text("Mantenha o aplicativo aberto durante o envio. Se ele for encerrado, a seleção pendente fica salva para tentar novamente. O limite e os formatos aceitos são definidos pelo servidor.", 12, MUTED), 22);
        if (state != null) { downloadURL = state.getString("download-url"); downloadServer = state.getString("download-server"); uploadFromLibrary = state.getBoolean("upload-library"); }
        changed();
    }
    private int dp(int n) { return Math.round(n * getResources().getDisplayMetrics().density); }
    private TextView text(String value, int sp, int color) {
        TextView v = new TextView(this); v.setText(value); v.setTextSize(sp); v.setTextColor(color); return v;
    }
    private void add(LinearLayout parent, View child, int top) {
        LinearLayout.LayoutParams p = new LinearLayout.LayoutParams(-1, -2); p.topMargin = dp(top); parent.addView(child, p);
    }
    private LinearLayout card(LinearLayout page, int top) {
        LinearLayout c = new LinearLayout(this); c.setOrientation(LinearLayout.VERTICAL); c.setPadding(dp(20), dp(20), dp(20), dp(20));
        GradientDrawable bg = new GradientDrawable(); bg.setColor(Color.WHITE); bg.setCornerRadius(dp(18)); c.setBackground(bg); c.setElevation(dp(1));
        add(page, c, top); return c;
    }
    private Button button(String label, LinearLayout parent, View.OnClickListener action) {
        Button b = new Button(this); b.setText(label); b.setAllCaps(false); b.setOnClickListener(action); add(parent, b, 8); return b;
    }
    @Override public void changed() {
        runOnUiThread(() -> {
            if (isFinishing() || isDestroyed()) return;
            boolean idle = !model.busy;
            if (model.signedIn && (!previousSignedIn || loginRevision != model.loginRevision)) { loginRevision = model.loginRevision; closeGallery(); showLibrary(); }
            else if (!model.signedIn && previousSignedIn) { closeGallery(); showUploads(); }
            previousSignedIn = model.signedIn;
            if (model.libraryRevision != libraryRevision) { libraryRevision = model.libraryRevision; if (gallery != null) gallery.refresh(); }
            libraryTab.setEnabled(model.signedIn); uploadsTab.setText(model.count() > 0 ? "Envios (" + model.count() + ")" : "Envios");
            if (gallery != null) gallery.transfers(model.busy);
            transferStatus.setText(model.message); transferStatus.setVisibility(galleryVisible && model.busy ? View.VISIBLE : View.GONE);
            if (previousBusy && !model.busy && galleryVisible) Toast.makeText(this, model.message, Toast.LENGTH_LONG).show();
            previousBusy = model.busy;
            login.setEnabled(idle); url.setEnabled(idle); password.setEnabled(idle); allowHTTP.setEnabled(idle);
            logout.setVisibility(model.signedIn ? View.VISIBLE : View.GONE); logout.setEnabled(idle);
            select.setEnabled(idle && model.signedIn);
            upload.setEnabled(idle && model.signedIn && model.count() > 0); clear.setEnabled(idle && model.count() > 0);
            queue.setText(model.count() + " arquivo(s) selecionado(s)");
            status.setText(model.message); detail.setText(model.detail); progress.setProgress(model.progress);
            progress.setVisibility(model.busy ? View.VISIBLE : View.GONE);
            if (model.busy) getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
            else getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        });
    }
    @Override protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request == SAVE_DOWNLOAD) {
            if (result == RESULT_OK && data != null && data.getData() != null && downloadURL != null && downloadServer.equals(model.endpoint())) {
                model.download(downloadServer, downloadURL, "lumio_session=" + model.sessionToken(), data.getData());
            }
            downloadURL = null; downloadServer = null; return;
        }
        if (request != PICK_MEDIA || result != RESULT_OK || data == null) { uploadFromLibrary = false; return; }
        ClipData clips = data.getClipData();
        if (clips != null) for (int i = 0; i < clips.getItemCount(); i++) addUri(clips.getItemAt(i).getUri());
        else if (data.getData() != null) addUri(data.getData());
        model.message = "Seleção pronta. Toque em Enviar para o Lumio.";
        if (uploadFromLibrary) model.upload(); uploadFromLibrary = false; changed();
    }
    private void addUri(Uri uri) {
        if (!"content".equals(uri.getScheme())) return;
        try { getContentResolver().takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION); }
        catch (SecurityException e) { model.message = "O provedor não permite manter acesso a este arquivo."; return; }
        model.add(uri);
    }
    private void pickFiles(boolean send) {
        if (model.busy) return; uploadFromLibrary = send;
        Intent choose = new Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("*/*");
        choose.putExtra(Intent.EXTRA_MIME_TYPES, new String[]{"image/*", "video/*"}); choose.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
        choose.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION);
        try { startActivityForResult(choose, PICK_MEDIA); }
        catch (android.content.ActivityNotFoundException e) { Toast.makeText(this, "Nenhum seletor de arquivos disponível.", Toast.LENGTH_LONG).show(); }
    }
    private void showLibrary() {
        if (!model.signedIn) { showUploads(); return; }
        if (gallery == null) {
            gallery = new GalleryView(this, model, new GalleryView.Host() {
                @Override public void upload() { pickFiles(true); }
                @Override public void download(String target, String name, String mime) {
                    if (model.busy || downloadURL != null) { Toast.makeText(MainActivity.this, "Aguarde a operação atual terminar.", Toast.LENGTH_SHORT).show(); return; }
                    if (!MediaPolicy.downloadAllowed(model.endpoint(), target)) return;
                    downloadURL = target; downloadServer = model.endpoint();
                    Intent save = new Intent(Intent.ACTION_CREATE_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType(mime).putExtra(Intent.EXTRA_TITLE, MediaPolicy.filename(name));
                    try { startActivityForResult(save, SAVE_DOWNLOAD); }
                    catch (android.content.ActivityNotFoundException e) { downloadURL = null; downloadServer = null; Toast.makeText(MainActivity.this, "Nenhum local de salvamento disponível.", Toast.LENGTH_LONG).show(); }
                }
                @Override public void logout() { if (!model.busy) model.logout(); }
                @Override public void expired() { model.invalidateSession(); }
                @Override public void theme(GalleryTheme theme) { applyChromeTheme(theme); }
            });
            panels.addView(gallery, new FrameLayout.LayoutParams(-1, -1));
        }
        galleryVisible = true; uploadPanel.setVisibility(View.GONE); gallery.setVisibility(View.VISIBLE); gallery.shown();
        transferStatus.setVisibility(model.busy ? View.VISIBLE : View.GONE);
    }
    private void showUploads() {
        galleryVisible = false; if (gallery != null) { gallery.pause(); gallery.setVisibility(View.GONE); }
        uploadPanel.setVisibility(View.VISIBLE); transferStatus.setVisibility(View.GONE); applyChromeTheme(new GalleryTheme(this, "light"));
    }
    private void applyChromeTheme(GalleryTheme theme) {
        shell.setBackgroundColor(theme.background); tabs.setBackgroundColor(theme.background); transferStatus.setTextColor(theme.text);
        for (Button b : new Button[]{libraryTab, uploadsTab}) { b.setTextColor(theme.text); b.setBackgroundTintList(android.content.res.ColorStateList.valueOf(theme.background)); }
        getWindow().setStatusBarColor(theme.background); getWindow().setNavigationBarColor(theme.background);
        int flags = getWindow().getDecorView().getSystemUiVisibility(), bars = View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR;
        getWindow().getDecorView().setSystemUiVisibility(theme.dark ? flags & ~bars : flags | bars);
    }
    private void closeGallery() { if (gallery != null) { gallery.close(); panels.removeView(gallery); gallery = null; } }
    @Override protected void onSaveInstanceState(Bundle state) {
        super.onSaveInstanceState(state); state.putString("download-url", downloadURL); state.putString("download-server", downloadServer); state.putBoolean("upload-library", uploadFromLibrary);
    }
    @Override protected void onStop() { if (gallery != null) gallery.pause(); super.onStop(); }
    @Override public void onBackPressed() {
        if (galleryVisible && gallery != null && gallery.back()) return;
        if (!galleryVisible && model.signedIn) showLibrary(); else super.onBackPressed();
    }
    @Override public Object onRetainNonConfigurationInstance() { return model; }
    @Override protected void onDestroy() {
        closeGallery(); model.observer = null;
        if (!isChangingConfigurations()) model.worker.shutdown();
        super.onDestroy();
    }
}
