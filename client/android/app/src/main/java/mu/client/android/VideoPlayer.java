package mu.client.android;

import android.annotation.SuppressLint;
import android.app.Dialog;
import android.content.Context;
import android.view.Window;
import android.view.WindowManager;
import android.content.Intent;
import android.net.Uri;
import android.view.View;
import android.webkit.*;
import android.widget.*;
import androidx.webkit.WebViewAssetLoader;

// A visible provider player. Never receives Micro credentials or arbitrary HTML.
final class VideoPlayer {
    private WebView web;
    private LinearLayout container;
    private Dialog dialog;
    private final Runnable talk;
    private final Context context;
    private final TextView title;
    private String id = "";

    VideoPlayer(Context context, Runnable talk) {
        this.context = context; this.talk = talk;
        title = new TextView(context);
    }
    void notice(String message) { title.setText(message); }
    boolean active() { return web != null; }
    @SuppressLint("SetJavaScriptEnabled")
    void show(String video, String label, boolean autoplay) {
        if (!VoiceCommands.validVideo(video)) return;
        close(); id = video;
        dialog = new Dialog(context);
        dialog.requestWindowFeature(Window.FEATURE_NO_TITLE);
        container = new LinearLayout(context);
        container.setOrientation(LinearLayout.VERTICAL);
        title.setText(label); title.setMaxLines(2); container.addView(title);
        dialog.setContentView(container);
        dialog.setOnDismissListener(d -> close());
        dialog.show();
        dialog.getWindow().setLayout(WindowManager.LayoutParams.MATCH_PARENT, WindowManager.LayoutParams.MATCH_PARENT);
        web = new WebView(context);
        web.getSettings().setJavaScriptEnabled(true);
        web.getSettings().setDomStorageEnabled(true);
        web.getSettings().setAllowFileAccess(false);
        web.getSettings().setAllowContentAccess(false);
        web.getSettings().setMediaPlaybackRequiresUserGesture(false);
        web.getSettings().setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        WebViewAssetLoader assets = new WebViewAssetLoader.Builder()
                .setDomain(BuildConfig.APPLICATION_ID)
                .addPathHandler("/assets/", new WebViewAssetLoader.AssetsPathHandler(context)).build();
        web.setWebViewClient(new WebViewClient() {
            @Override public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                return assets.shouldInterceptRequest(request.getUrl());
            }
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                // Keep provider navigation inside the frame. Never navigate the top-level
                // privileged player document to a third-party page.
                return request.isForMainFrame();
            }
        });
        web.setWebChromeClient(new WebChromeClient());
        container.addView(web, new LinearLayout.LayoutParams(-1, 0, 1));
        LinearLayout controls = new LinearLayout(context);
        button(controls, "Play", () -> command("play"));
        button(controls, "Pause", () -> command("pause"));
        button(controls, "Speak", talk);
        button(controls, "Close", this::close);
        Button original = new Button(context, null, 0, R.style.CompactButton);
        original.setText("Open in YouTube");
        original.setOnClickListener(v -> context.startActivity(new Intent(Intent.ACTION_VIEW,
                Uri.parse("https://www.youtube.com/watch?v=" + id))));
        container.addView(original);
        container.addView(controls);
        container.setVisibility(View.VISIBLE);
        // This origin is the installed application identifier, as required by YouTube's WebView integration.
        web.loadUrl("https://" + BuildConfig.APPLICATION_ID + "/assets/player.html?id=" + id + "&autoplay=" + (autoplay ? "1" : "0"));
    }
    private void button(LinearLayout parent, String text, Runnable action) {
        Button button = new Button(context, null, 0, R.style.CompactButton);
        button.setText(text); button.setOnClickListener(v -> action.run());
        parent.addView(button);
    }
    void command(String action) {
        if (web != null && (action.equals("play") || action.equals("pause")))
            web.evaluateJavascript("window.microPlayerCommand && window.microPlayerCommand('" + action + "')", null);
    }
    void pause() { command("pause"); if (web != null) web.onPause(); }
    void resumeView() { if (web != null) web.onResume(); }
    void close() {
        if (web != null) {
            command("pause"); container.removeView(web); web.destroy(); web = null;
        }
        if (container != null) { container.removeAllViews(); container = null; }
        if (dialog != null) {
            Dialog old = dialog; dialog = null; old.setOnDismissListener(null); old.dismiss();
        }
        id = "";
    }
}
