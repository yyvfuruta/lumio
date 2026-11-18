package io.github.yyvfuruta.lumio;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.app.AlertDialog;
import android.graphics.Color;
import android.net.http.SslError;
import android.os.Handler;
import android.os.Looper;
import android.webkit.CookieManager;
import android.webkit.JsResult;
import android.webkit.SslErrorHandler;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import java.io.ByteArrayInputStream;
import java.util.Collections;
import org.json.JSONObject;

/** Delivery adapter for the server's real responsive web frontend. No JS bridge. */
final class LibraryBrowser {
    interface Host {
        void loading(boolean loading);
        void failed(String message);
        void chooseFiles(ValueCallback<android.net.Uri[]> callback, boolean multiple);
        void download(String url, String disposition, String mime);
        void session(String server, String cookies);
        void theme(String background, String foreground);
    }
    final WebView view;
    private final Activity activity;
    private final Host host;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private volatile String server = "";
    private String injectedToken = "";
    private String lastBackground = "", lastForeground = "";
    private volatile boolean destroyed;
    private volatile boolean sessionReady;
    private boolean failed, visible;
    private android.view.View fullscreen;
    private WebChromeClient.CustomViewCallback fullscreenCallback;
    private int previousSystemUI;
    private final Runnable checkUpload = new Runnable() {
        @Override public void run() {
            if (destroyed || !visible) return;
            if (WebPolicy.sameOrigin(server, view.getUrl())) {
                syncSession();
                view.evaluateJavascript("(function(){var b=document.getElementById('upload-button'),s=getComputedStyle(document.documentElement);return {busy:!!(b&&b.disabled),background:s.getPropertyValue('--background').trim(),foreground:s.getPropertyValue('--text').trim()};})()", state -> {
                    if (destroyed) return;
                    try {
                        JSONObject value = new JSONObject(state);
                        view.setKeepScreenOn(value.optBoolean("busy"));
                        String background = value.optString("background"), foreground = value.optString("foreground");
                        if (!background.equals(lastBackground) || !foreground.equals(lastForeground)) {
                            host.theme(background, foreground); lastBackground = background; lastForeground = foreground;
                        }
                    } catch (Exception ignored) { }
                });
            }
            handler.postDelayed(this, 1000);
        }
    };
    @SuppressLint("SetJavaScriptEnabled")
    LibraryBrowser(Activity activity, Host host) {
        this.activity = activity; this.host = host; view = new WebView(activity);
        view.setBackgroundColor(Color.rgb(248, 248, 248));
        WebSettings settings = view.getSettings();
        settings.setJavaScriptEnabled(true); settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false); settings.setAllowContentAccess(true);
        settings.setAllowFileAccessFromFileURLs(false); settings.setAllowUniversalAccessFromFileURLs(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setSafeBrowsingEnabled(true); settings.setSupportMultipleWindows(false);
        settings.setJavaScriptCanOpenWindowsAutomatically(false);
        settings.setUseWideViewPort(true); settings.setLoadWithOverviewMode(true);
        settings.setMediaPlaybackRequiresUserGesture(true);
        settings.setUserAgentString(settings.getUserAgentString() + " LumioAndroid");
        CookieManager.getInstance().setAcceptCookie(true);
        CookieManager.getInstance().setAcceptThirdPartyCookies(view, false);
        view.setWebViewClient(new WebViewClient() {
            @Override public boolean shouldOverrideUrlLoading(WebView v, WebResourceRequest request) {
                if (WebPolicy.sameOrigin(server, request.getUrl().toString())) return false;
                host.failed("O aplicativo abre apenas o servidor Lumio configurado."); return true;
            }
            @Override public WebResourceResponse shouldInterceptRequest(WebView v, WebResourceRequest request) {
                String target = request.getUrl().toString();
                if (!WebPolicy.sameOrigin(server, target)) {
                    return new WebResourceResponse("text/plain", "UTF-8", 403, "Forbidden", Collections.emptyMap(), new ByteArrayInputStream(new byte[0]));
                }
                if (request.getMethod().equals("GET") && (request.getUrl().getPath().equals("/api/v1/photos")
                        || request.getUrl().getPath().equals("/api/v1/sessions/current"))) {
                    String endpoint = server, cookies = CookieManager.getInstance().getCookie(server);
                    handler.post(() -> { if (!destroyed && sessionReady && endpoint.equals(server)) host.session(endpoint, cookies); });
                }
                return null;
            }
            @Override public void onPageStarted(WebView v, String url, android.graphics.Bitmap favicon) {
                failed = false; host.loading(true);
            }
            @Override public void onPageFinished(WebView v, String url) {
                if (destroyed || !WebPolicy.sameOrigin(server, url)) return;
                if (!failed) host.loading(false);
                syncSession();
            }
            @Override public void onReceivedError(WebView v, WebResourceRequest request, WebResourceError error) {
                if (request.isForMainFrame()) fail("Não foi possível abrir o Lumio. Verifique o endereço e sua conexão.");
            }
            @Override public void onReceivedHttpError(WebView v, WebResourceRequest request, WebResourceResponse response) {
                if (request.isForMainFrame()) fail("O servidor retornou HTTP " + response.getStatusCode() + ".");
            }
            @Override public void onReceivedSslError(WebView v, SslErrorHandler decision, SslError error) {
                decision.cancel(); fail("Não foi possível validar o certificado HTTPS do servidor.");
            }
        });
        view.setWebChromeClient(new WebChromeClient() {
            @Override public boolean onShowFileChooser(WebView v, ValueCallback<android.net.Uri[]> result, FileChooserParams params) {
                if (!WebPolicy.sameOrigin(server, v.getUrl())) { result.onReceiveValue(null); return true; }
                host.chooseFiles(result, params.getMode() == FileChooserParams.MODE_OPEN_MULTIPLE); return true;
            }
            @Override public void onShowCustomView(android.view.View custom, CustomViewCallback callback) {
                if (fullscreen != null) { callback.onCustomViewHidden(); return; }
                fullscreen = custom; fullscreenCallback = callback;
                android.widget.FrameLayout decor = (android.widget.FrameLayout) activity.getWindow().getDecorView();
                previousSystemUI = decor.getSystemUiVisibility();
                decor.addView(custom, new android.widget.FrameLayout.LayoutParams(-1, -1));
                decor.setSystemUiVisibility(android.view.View.SYSTEM_UI_FLAG_FULLSCREEN | android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION | android.view.View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY);
            }
            @Override public void onHideCustomView() { hideFullscreen(); }
            @Override public boolean onJsConfirm(WebView v, String url, String message, JsResult result) {
                if (!WebPolicy.sameOrigin(server, url)) { result.cancel(); return true; }
                new AlertDialog.Builder(activity).setMessage(message)
                        .setPositiveButton("Confirmar", (d, which) -> result.confirm())
                        .setNegativeButton("Cancelar", (d, which) -> result.cancel())
                        .setOnCancelListener(d -> result.cancel()).show();
                return true;
            }
        });
        view.setDownloadListener((url, agent, disposition, mime, length) -> {
            if (WebPolicy.downloadAllowed(server, url)) host.download(url, disposition, mime);
            else host.failed("Este link não é um download do Lumio.");
        });
    }
    void show(String endpoint, String token) {
        if (endpoint.equals(server) && token.equals(injectedToken)) return;
        boolean changedServer = !server.equals(endpoint);
        sessionReady = false; server = endpoint; injectedToken = token;
        Runnable load = () -> {
            if (destroyed || !server.equals(endpoint)) return;
            String cookie = "lumio_session=" + token + "; Path=/; HttpOnly; SameSite=Strict" + (endpoint.startsWith("https://") ? "; Secure" : "");
            CookieManager.getInstance().setCookie(endpoint, cookie, accepted -> {
                if (!destroyed && server.equals(endpoint)) {
                    if (!accepted) { fail("Não foi possível abrir a sessão da biblioteca."); return; }
                    sessionReady = true; view.clearHistory(); view.loadUrl(endpoint + "/");
                }
            });
        };
        if (changedServer) {
            view.stopLoading(); view.clearCache(true);
            CookieManager.getInstance().removeAllCookies(ignored -> load.run());
        } else load.run();
    }
    void syncSession() {
        if (!destroyed && sessionReady && !server.isEmpty()) host.session(server, CookieManager.getInstance().getCookie(server));
    }
    void reload() { if (!destroyed) view.reload(); }
    void visible(boolean value) {
        visible = value; handler.removeCallbacks(checkUpload);
        if (value) { lastBackground = ""; lastForeground = ""; view.onResume(); handler.post(checkUpload); } else { view.onPause(); view.setKeepScreenOn(false); }
    }
    void back(Runnable leave) {
        if (fullscreen != null) { hideFullscreen(); return; }
        if (!WebPolicy.sameOrigin(server, view.getUrl())) { leave.run(); return; }
        view.evaluateJavascript("(function(){for(var id of ['viewer','help']){var d=document.getElementById(id);if(d&&d.open){d.close();return true;}}var m=document.getElementById('library-menu');if(m&&m.open){m.open=false;return true;}var s=document.getElementById('selection-bar');if(s&&!s.hidden){document.getElementById('cancel-selection').click();return true;}return false;})()", handled -> {
            if (destroyed || "true".equals(handled)) return;
            if (view.canGoBack()) view.goBack(); else leave.run();
        });
    }
    private void hideFullscreen() {
        if (fullscreen == null) return;
        android.widget.FrameLayout decor = (android.widget.FrameLayout) activity.getWindow().getDecorView();
        decor.removeView(fullscreen); fullscreen = null; decor.setSystemUiVisibility(previousSystemUI);
        if (fullscreenCallback != null) { fullscreenCallback.onCustomViewHidden(); fullscreenCallback = null; }
    }
    void destroy() { hideFullscreen(); destroyed = true; handler.removeCallbacksAndMessages(null); view.stopLoading(); view.destroy(); }
    private void fail(String message) { failed = true; host.failed(message); }
}
