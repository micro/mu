package mu.client.android;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.content.Context;
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
    private final LinearLayout messages;
    private boolean paused;
    private boolean playing;
    private final Runnable talk;
    private final Context context;
    private final TextView title;
    private String id = "";

    VideoPlayer(Context context, Runnable talk) {
        this.context = context; this.talk = talk;
        title = new TextView(context);
        messages = ((Activity) context).findViewById(R.id.messages);
    }
    void notice(String message) { title.setText(message); }
    boolean active() { return web != null; }
    boolean playing() { return web != null && !paused && playing; }
    @SuppressLint("SetJavaScriptEnabled")
    void show(String video, String label, boolean autoplay) {
        if (!VoiceCommands.validVideo(video)) return;
        close(); id = video;
        paused = false;
        playing = autoplay;
        container = new LinearLayout(context);
        container.setOrientation(LinearLayout.VERTICAL);
        int gap = (int) (12 * context.getResources().getDisplayMetrics().density);
        LinearLayout.LayoutParams mediaSpacing = new LinearLayout.LayoutParams(-1, -2);
        mediaSpacing.topMargin = gap; mediaSpacing.bottomMargin = gap * 2;
        container.setLayoutParams(mediaSpacing);
        title.setPadding(0, 0, 0, gap);
        title.setText(label); title.setMaxLines(2); container.addView(title);
        messages.addView(container);
        dismissKeyboard();
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
        container.addView(web, new LinearLayout.LayoutParams(-1, (int) (260 * context.getResources().getDisplayMetrics().density)));
        web.addOnLayoutChangeListener((v, l, t, r, b, ol, ot, or, ob) -> {
            int height = Math.max((int) (232 * context.getResources().getDisplayMetrics().density), (r - l) * 9 / 16 + (int) (32 * context.getResources().getDisplayMetrics().density));
            if (web != null && web.getLayoutParams().height != height) {
                web.getLayoutParams().height = height; web.requestLayout();
            }
        });
        web.post(pollPlayback);
        LinearLayout controls = new LinearLayout(context);
        button(controls, "Play", () -> { resumeView(); command("play"); });
        button(controls, "Pause", () -> command("pause"));
        button(controls, "Speak", talk);
        button(controls, "Close", this::close);
        Button original = new Button(context, null, 0, R.style.CompactButton);
        original.setText("Open in YouTube");
        original.setOnClickListener(v -> context.startActivity(new Intent(Intent.ACTION_VIEW,
                Uri.parse("https://www.youtube.com/watch?v=" + id))));
        container.addView(original);
        controls.setPadding(0, gap, 0, 0);
        container.addView(controls);
        container.setVisibility(View.VISIBLE);
        LinearLayout shown = container;
        shown.post(() -> {
            if (container == shown) {
                ScrollView scroll = ((Activity) context).findViewById(R.id.scroll);
                scroll.smoothScrollTo(0, shown.getTop());
            }
        });
        // This origin is the installed application identifier, as required by YouTube's WebView integration.
        web.loadUrl("https://" + BuildConfig.APPLICATION_ID + "/assets/player.html?id=" + id + "&autoplay=" + (autoplay ? "1" : "0"));
    }
    private void dismissKeyboard() {
        Activity activity = (Activity) context;
        View prompt = activity.findViewById(R.id.prompt);
        prompt.clearFocus();
        android.view.inputmethod.InputMethodManager keyboard = (android.view.inputmethod.InputMethodManager) context.getSystemService(Context.INPUT_METHOD_SERVICE);
        keyboard.hideSoftInputFromWindow(prompt.getWindowToken(), 0);
    }
    private void button(LinearLayout parent, String text, Runnable action) {
        Button button = new Button(context, null, 0, R.style.CompactButton);
        button.setText(text); button.setOnClickListener(v -> action.run());
        LinearLayout.LayoutParams layout = new LinearLayout.LayoutParams(0, -2, 1);
        int gap = (int) (4 * context.getResources().getDisplayMetrics().density);
        layout.setMargins(gap, 0, gap, 0);
        parent.addView(button, layout);
    }
    // Read only the local player's state; no JavaScript bridge or account credentials.
    private final Runnable pollPlayback = new Runnable() {
        @Override public void run() {
            WebView current = web;
            if (current == null) return;
            current.evaluateJavascript("window.microPlayerPlaying === true", value -> {
                if (web == current) {
                    playing = "true".equals(value);
                    current.setKeepScreenOn(!paused && playing);
                }
            });
            current.postDelayed(this, 500);
        }
    };
    void command(String action) {
        if (action.equals("play")) dismissKeyboard();
        if (web != null && (action.equals("play") || action.equals("pause")))
            web.evaluateJavascript("window.microPlayerCommand && window.microPlayerCommand('" + action + "')", null);
    }
    void pause() { playing = false; paused = true; command("pause"); if (web != null) { web.setKeepScreenOn(false); } }
    void resumeView() { paused = false; if (web != null) web.onResume(); }
    void close() {
        if (web != null) {
            command("pause"); web.removeCallbacks(pollPlayback); web.setKeepScreenOn(false); container.removeView(web); web.destroy(); web = null;
        }
        if (container != null) { container.removeAllViews(); messages.removeView(container); container = null; }
        playing = false;
        id = "";
    }
}
