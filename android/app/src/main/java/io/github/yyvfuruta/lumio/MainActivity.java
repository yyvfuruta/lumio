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
import android.widget.CheckBox;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.ScrollView;
import android.widget.TextView;

public final class MainActivity extends Activity implements UploadModel.Observer {
    private static final int PICK_MEDIA = 10;
    private static final int INK = 0xff163042, TEAL = 0xff087f8c, MUTED = 0xff536b7b;
    private UploadModel model;
    private EditText url, password;
    private CheckBox allowHTTP;
    private Button login, logout, select, upload, clear;
    private TextView status, queue, detail;
    private ProgressBar progress;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        Object retained = getLastNonConfigurationInstance();
        model = retained instanceof UploadModel ? (UploadModel) retained : new UploadModel(getApplicationContext());
        model.observer = this;
        ScrollView scroll = new ScrollView(this); scroll.setFillViewport(true);
        LinearLayout page = new LinearLayout(this); page.setOrientation(LinearLayout.VERTICAL);
        page.setPadding(dp(24), dp(36), dp(24), dp(28));
        scroll.addView(page); setContentView(scroll);
        scroll.setOnApplyWindowInsetsListener((v, insets) -> {
            page.setPadding(dp(24), dp(24) + insets.getSystemWindowInsetTop(), dp(24), dp(28) + insets.getSystemWindowInsetBottom());
            return insets;
        });
        scroll.requestApplyInsets();
        TextView brand = text("LUMIO", 14, TEAL); brand.setLetterSpacing(.2f); brand.setTypeface(null, Typeface.BOLD); page.addView(brand);
        TextView title = text("Suas memórias,\nno seu servidor.", 32, INK); title.setTypeface(null, Typeface.BOLD); add(page, title, 12);
        add(page, text("Envie fotos e vídeos do celular sem alterar os arquivos originais.", 16, MUTED), 12);

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
    @Override public Object onRetainNonConfigurationInstance() { return model; }
    @Override protected void onDestroy() {
        model.observer = null;
        if (!isChangingConfigurations() && !model.busy) model.worker.shutdown();
        super.onDestroy();
    }
}
