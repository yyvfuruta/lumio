package io.github.yyvfuruta.lumio;

import android.app.Activity;
import android.webkit.CookieManager;
import android.webkit.ValueCallback;
import android.webkit.URLUtil;
import android.widget.FrameLayout;
import android.widget.Toast;
import android.widget.PopupMenu;
import java.util.ArrayList;
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
import android.widget.CheckBox;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.ScrollView;
import android.widget.TextView;

public final class MainActivity extends Activity implements UploadModel.Observer {
    private static final int PICK_MEDIA = 10, PICK_WEB_MEDIA = 11, SAVE_DOWNLOAD = 12;
    private static final int INK = 0xff171717, TEAL = 0xff444444, MUTED = 0xff777777;
    private UploadModel model;
    private EditText url, password;
    private CheckBox allowHTTP;
    private Button login, logout, select, upload, clear;
    private TextView status, queue, detail;
    private ProgressBar progress, pageProgress;
    private LibraryBrowser browser;
    private FrameLayout panels, browserPanel;
    private View uploadPanel;
    private LinearLayout browserError;
    private TextView browserErrorText, transferStatus;
    private Button libraryTab, uploadsTab, menu;
    private LinearLayout shell, toolbar;
    private boolean galleryVisible, previousSignedIn, previousBusy, dirtyLibrary;
    private int loginRevision, libraryRevision;
    private ValueCallback<Uri[]> fileCallback;
    private String downloadURL, downloadServer;


    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        Object retained = getLastNonConfigurationInstance();
        model = retained instanceof UploadModel ? (UploadModel) retained : new UploadModel(getApplicationContext());
        model.observer = this;
        ScrollView scroll = new ScrollView(this); scroll.setFillViewport(true);
        LinearLayout page = new LinearLayout(this); page.setOrientation(LinearLayout.VERTICAL);
        page.setPadding(dp(24), dp(36), dp(24), dp(28));
        scroll.addView(page);
        uploadPanel = scroll;
        shell = new LinearLayout(this); shell.setOrientation(LinearLayout.VERTICAL);
        toolbar = new LinearLayout(this); toolbar.setPadding(dp(8), 0, dp(8), 0);
        libraryTab = new Button(this); libraryTab.setText("Biblioteca"); libraryTab.setAllCaps(false);
        libraryTab.setOnClickListener(v -> showLibrary());
        uploadsTab = new Button(this); uploadsTab.setText("Envios"); uploadsTab.setAllCaps(false);
        uploadsTab.setOnClickListener(v -> showUploads());
        toolbar.addView(libraryTab, new LinearLayout.LayoutParams(0, dp(48), 1));
        toolbar.addView(uploadsTab, new LinearLayout.LayoutParams(0, dp(48), 1));
        menu = new Button(this); menu.setText("⋮"); menu.setContentDescription("Opções do aplicativo");
        toolbar.addView(menu, new LinearLayout.LayoutParams(dp(48), dp(48)));
        menu.setOnClickListener(v -> {
            PopupMenu popup = new PopupMenu(this, menu);
            popup.getMenu().add("Atualizar biblioteca").setOnMenuItemClickListener(item -> { if (browser != null) browser.reload(); return true; });
            popup.getMenu().add("Configurar servidor").setOnMenuItemClickListener(item -> { showUploads(); scroll.smoothScrollTo(0, 0); return true; });
            popup.show();
        });
        shell.addView(toolbar);
        pageProgress = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal); pageProgress.setIndeterminate(true);
        pageProgress.setVisibility(View.GONE); shell.addView(pageProgress, new LinearLayout.LayoutParams(-1, dp(3)));
        panels = new FrameLayout(this); shell.addView(panels, new LinearLayout.LayoutParams(-1, 0, 1));
        panels.addView(scroll, new FrameLayout.LayoutParams(-1, -1));
        browserPanel = new FrameLayout(this); panels.addView(browserPanel, new FrameLayout.LayoutParams(-1, -1));
        browserPanel.setVisibility(View.GONE);
        transferStatus = text("", 13, INK); transferStatus.setPadding(dp(16), dp(8), dp(16), dp(8));
        transferStatus.setVisibility(View.GONE); shell.addView(transferStatus);
        setContentView(shell);
        shell.setOnApplyWindowInsetsListener((v, insets) -> {
            shell.setPadding(0, insets.getSystemWindowInsetTop(), 0, insets.getSystemWindowInsetBottom()); return insets;
        });
        loginRevision = model.loginRevision; libraryRevision = model.libraryRevision;

        scroll.requestApplyInsets();
        TextView brand = text("LUMIO", 14, TEAL); brand.setLetterSpacing(.2f); brand.setTypeface(null, Typeface.BOLD); page.addView(brand);
        TextView title = text("Sua biblioteca de fotos.", 28, INK); title.setTypeface(null, Typeface.BOLD); add(page, title, 12);
        add(page, text("Conecte seu servidor para abrir a biblioteca. Você também pode gerenciar os envios deste celular aqui.", 16, MUTED), 12);

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
        logout = button("Sair da conta", connection, v -> { closeBrowser(); CookieManager.getInstance().removeAllCookies(null); model.logout(); });

        LinearLayout files = card(page, 18);
        add(files, text("02  /  ENVIAR", 13, TEAL), 0);
        queue = text("", 22, INK); queue.setTypeface(null, Typeface.BOLD); add(files, queue, 14);
        add(files, text("Selecione apenas os arquivos que deseja enviar. Fotos duplicadas não são armazenadas novamente.", 14, MUTED), 8);
        select = button("Selecionar fotos e vídeos", files, v -> {
            Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
            intent.addCategory(Intent.CATEGORY_OPENABLE); intent.setType("*/*");
            intent.putExtra(Intent.EXTRA_MIME_TYPES, new String[]{"image/*", "video/*"});
            intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
            intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION);
            startActivityForResult(intent, PICK_MEDIA);
        });
        upload = button("Enviar para o Lumio", files, v -> model.upload());
        clear = button("Limpar seleção", files, v -> model.clearQueue());
        progress = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal); progress.setMax(100); add(files, progress, 16);
        status = text("", 15, INK); status.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE); add(files, status, 12);
        detail = text("", 13, MUTED); detail.setTextIsSelectable(true); add(files, detail, 12);
        add(page, text("Mantenha o aplicativo aberto durante o envio. Se ele for encerrado, a seleção pendente fica salva para tentar novamente. O limite e os formatos aceitos são definidos pelo servidor.", 12, MUTED), 22);
        if (state != null) { downloadURL = state.getString("download-url"); downloadServer = state.getString("download-server"); }
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
            if (model.libraryRevision != libraryRevision) { libraryRevision = model.libraryRevision; dirtyLibrary = true; }
            if (model.signedIn && (!previousSignedIn || loginRevision != model.loginRevision)) {
                loginRevision = model.loginRevision; showLibrary();
            } else if (!model.signedIn && previousSignedIn) { closeBrowser(); showUploads(); }
            previousSignedIn = model.signedIn;
            libraryTab.setEnabled(model.signedIn);
            uploadsTab.setText(model.count() > 0 ? "Envios (" + model.count() + ")" : "Envios");
            transferStatus.setVisibility(galleryVisible && model.busy ? View.VISIBLE : View.GONE);
            transferStatus.setText(model.message);
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
            if (result == RESULT_OK && data != null && data.getData() != null && downloadURL != null
                    && downloadServer.equals(model.endpoint())) {
                String cookies = CookieManager.getInstance().getCookie(downloadServer);
                if (WebPolicy.sessionToken(cookies).isEmpty() && model.signedIn) cookies = "lumio_session=" + model.sessionToken();
                model.download(downloadServer, downloadURL, cookies, data.getData());
            }
            downloadURL = null; downloadServer = null; return;
        }
        if (request == PICK_WEB_MEDIA) {
            ValueCallback<Uri[]> callback = fileCallback; fileCallback = null;
            if (callback == null) return;
            ArrayList<Uri> files = new ArrayList<>();
            if (result == RESULT_OK && data != null) {
                ClipData clips = data.getClipData();
                if (clips != null) for (int i = 0; i < clips.getItemCount(); i++) acceptWebUri(files, clips.getItemAt(i).getUri());
                else if (data.getData() != null) acceptWebUri(files, data.getData());
            }
            callback.onReceiveValue(files.isEmpty() ? null : files.toArray(new Uri[0])); return;
        }
        if (request != PICK_MEDIA || result != RESULT_OK || data == null) return;
        ClipData clips = data.getClipData();
        if (clips != null) for (int i = 0; i < clips.getItemCount(); i++) addUri(clips.getItemAt(i).getUri());
        else if (data.getData() != null) addUri(data.getData());
        model.message = "Seleção pronta. Toque em Enviar para o Lumio."; changed();
    }
    private void addUri(Uri uri) {
        if (!"content".equals(uri.getScheme())) return;
        try { getContentResolver().takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION); }
        catch (SecurityException e) { model.message = "O provedor não permite manter acesso a este arquivo."; return; }
        model.add(uri);
    }
    private void acceptWebUri(ArrayList<Uri> files, Uri uri) {
        if (uri != null && "content".equals(uri.getScheme()) && !files.contains(uri)) files.add(uri);
    }
    private void showUploads() {
        if (browser != null) { browser.syncSession(); browser.visible(false); }
        galleryVisible = false; applyChromeTheme("#ffffff", "#171717"); uploadPanel.setVisibility(View.VISIBLE); browserPanel.setVisibility(View.GONE);
        pageProgress.setVisibility(View.GONE); transferStatus.setVisibility(View.GONE);
    }
    private void showLibrary() {
        if (!model.signedIn) { showUploads(); return; }
        if (browser == null) {
            browser = new LibraryBrowser(this, new LibraryBrowser.Host() {
                @Override public void loading(boolean loading) {
                    browserError.setVisibility(View.GONE);
                    pageProgress.setVisibility(loading && galleryVisible ? View.VISIBLE : View.GONE);
                }
                @Override public void failed(String message) {
                    pageProgress.setVisibility(View.GONE); browserErrorText.setText(message); browserError.setVisibility(View.VISIBLE);
                }
                @Override public void chooseFiles(ValueCallback<Uri[]> callback, boolean multiple) {
                    if (fileCallback != null) fileCallback.onReceiveValue(null);
                    fileCallback = callback;
                    Intent choose = new Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("*/*");
                    choose.putExtra(Intent.EXTRA_MIME_TYPES, new String[]{"image/*", "video/*"});
                    choose.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, multiple);
                    choose.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
                    try { startActivityForResult(choose, PICK_WEB_MEDIA); }
                    catch (android.content.ActivityNotFoundException e) { fileCallback = null; callback.onReceiveValue(null); Toast.makeText(MainActivity.this, "Nenhum seletor de arquivos disponível.", Toast.LENGTH_LONG).show(); }
                }
                @Override public void download(String target, String disposition, String mime) {
                    if (model.busy || downloadURL != null) { Toast.makeText(MainActivity.this, "Aguarde a operação atual terminar.", Toast.LENGTH_SHORT).show(); return; }
                    downloadURL = target; downloadServer = model.endpoint();
                    Intent save = new Intent(Intent.ACTION_CREATE_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE)
                            .setType(mime == null || mime.isEmpty() ? "application/octet-stream" : mime)
                            .putExtra(Intent.EXTRA_TITLE, WebPolicy.filename(URLUtil.guessFileName(target, disposition, mime)));
                    try { startActivityForResult(save, SAVE_DOWNLOAD); }
                    catch (android.content.ActivityNotFoundException e) { downloadURL = null; downloadServer = null; Toast.makeText(MainActivity.this, "Nenhum local de salvamento disponível.", Toast.LENGTH_LONG).show(); }
                }
                @Override public void session(String server, String cookies) { model.adoptBrowserSession(server, cookies); }
                @Override public void theme(String background, String foreground) { if (galleryVisible) applyChromeTheme(background, foreground); }
            });
            browserPanel.addView(browser.view, new FrameLayout.LayoutParams(-1, -1));
            browserError = new LinearLayout(this); browserError.setOrientation(LinearLayout.VERTICAL);
            browserError.setPadding(dp(24), dp(40), dp(24), dp(24)); browserError.setBackgroundColor(0xfff8f8f8);
            browserErrorText = text("", 18, INK); browserError.addView(browserErrorText);
            button("Tentar novamente", browserError, v -> { browserError.setVisibility(View.GONE); browser.reload(); });
            button("Configurar servidor", browserError, v -> showUploads());
            browserError.setVisibility(View.GONE); browserPanel.addView(browserError, new FrameLayout.LayoutParams(-1, -1));
        }
        galleryVisible = true; uploadPanel.setVisibility(View.GONE); browserPanel.setVisibility(View.VISIBLE);
        browser.visible(true); browser.show(model.endpoint(), model.sessionToken());
        if (dirtyLibrary && !model.busy) { browser.reload(); dirtyLibrary = false; }
    }
    private void applyChromeTheme(String background, String foreground) {
        try {
            int bg = Color.parseColor(background), fg = Color.parseColor(foreground);
            shell.setBackgroundColor(bg); toolbar.setBackgroundColor(bg); transferStatus.setTextColor(fg);
            for (Button b : new Button[]{libraryTab, uploadsTab, menu}) { b.setTextColor(fg); b.setBackgroundTintList(android.content.res.ColorStateList.valueOf(bg)); }
            getWindow().setStatusBarColor(bg); getWindow().setNavigationBarColor(bg);
            boolean light = Color.red(bg) + Color.green(bg) + Color.blue(bg) > 384;
            int flags = getWindow().getDecorView().getSystemUiVisibility();
            int bars = View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR;
            getWindow().getDecorView().setSystemUiVisibility(light ? flags | bars : flags & ~bars);
        } catch (IllegalArgumentException ignored) { }
    }
    private void closeBrowser() {
        if (fileCallback != null) { fileCallback.onReceiveValue(null); fileCallback = null; }
        if (browser != null) { browser.destroy(); browser = null; browserPanel.removeAllViews(); }
    }
    @Override protected void onSaveInstanceState(Bundle state) {
        super.onSaveInstanceState(state); state.putString("download-url", downloadURL); state.putString("download-server", downloadServer);
    }
    @Override protected void onStop() {
        if (browser != null) { browser.syncSession(); browser.visible(false); }
        super.onStop();
    }
    @Override protected void onStart() {
        super.onStart(); if (browser != null && galleryVisible) browser.visible(true);
    }
    @Override public void onBackPressed() {
        if (galleryVisible && browser != null) browser.back(() -> MainActivity.super.onBackPressed());
        else if (model.signedIn) showLibrary(); else super.onBackPressed();
    }
    @Override public Object onRetainNonConfigurationInstance() { return model; }
    @Override protected void onDestroy() {
        if (fileCallback != null) { fileCallback.onReceiveValue(null); fileCallback = null; }
        closeBrowser(); model.observer = null;
        if (!isChangingConfigurations()) model.worker.shutdown();
        super.onDestroy();
    }
}
