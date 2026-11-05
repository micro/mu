package mu.client.android;

import android.Manifest;
import android.app.*;
import android.content.*;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Bundle;
import android.speech.tts.*;
import android.view.*;
import android.view.inputmethod.EditorInfo;
import android.widget.*;
import io.noties.markwon.Markwon;
import org.json.JSONObject;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class MainActivity extends Activity {
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private Vault vault;
    private JSONObject account = new JSONObject();
    private String thread = "", agent = "micro";
    private boolean busy, foreground, listening, ttsReady, destroyed, speaking;
    private LinearLayout messages;
    private ScrollView scroll;
    private EditText prompt;
    private TextView status;
    private Button send, speak, settings, newChat;
    private Switch wake, spoken;
    private VoiceInput voice;
    private TextToSpeech tts;
    private Markwon markdown;

    @Override public void onCreate(Bundle saved) {
        super.onCreate(saved);
        setContentView(R.layout.main);
        View root = findViewById(R.id.root);
        int pad = (int) (16 * getResources().getDisplayMetrics().density);
        root.setOnApplyWindowInsetsListener((v, insets) -> {
            if (android.os.Build.VERSION.SDK_INT >= 30) {
                android.graphics.Insets bars = insets.getInsets(
                        WindowInsets.Type.systemBars() | WindowInsets.Type.ime());
                v.setPadding(pad + bars.left, pad + bars.top, pad + bars.right, pad + bars.bottom);
            } else {
                v.setPadding(pad + insets.getSystemWindowInsetLeft(), pad + insets.getSystemWindowInsetTop(),
                        pad + insets.getSystemWindowInsetRight(), pad + insets.getSystemWindowInsetBottom());
            }
            return insets;
        });
        vault = new Vault(this);
        try { account = vault.read(); }
        catch (Exception e) { vault.clear(); }
        markdown = Markwon.create(this);
        messages = findViewById(R.id.messages);
        scroll = findViewById(R.id.scroll);
        prompt = findViewById(R.id.prompt);
        status = findViewById(R.id.status);
        send = findViewById(R.id.send); speak = findViewById(R.id.speak);
        settings = findViewById(R.id.settings); newChat = findViewById(R.id.new_chat);
        wake = findViewById(R.id.wake); spoken = findViewById(R.id.spoken);
        send.setOnClickListener(v -> send());
        prompt.setOnEditorActionListener((v, action, event) -> {
            if (action == EditorInfo.IME_ACTION_SEND ||
                    (event != null && event.getKeyCode() == KeyEvent.KEYCODE_ENTER && event.getAction() == KeyEvent.ACTION_DOWN)) {
                send(); return true;
            }
            return false;
        });
        speak.setOnClickListener(v -> { if (listening || speaking) stopAudio(); else startVoice(false); });
        settings.setOnClickListener(v -> connection());
        newChat.setOnClickListener(v -> { stopAudio(); thread = ""; messages.removeAllViews(); status.setText("New conversation"); });
        wake.setOnCheckedChangeListener((v, enabled) -> { if (enabled) startVoice(true); else stopAudio(); });
        spoken.setOnCheckedChangeListener((v, enabled) -> { if (!enabled) stopAudio(); });
        tts = new TextToSpeech(this, result -> {
            if (destroyed || result != TextToSpeech.SUCCESS) return;
            // Do not send replies to a cloud speech engine.
            if (tts.getVoices() != null) for (Voice v : tts.getVoices()) {
                if (!v.isNetworkConnectionRequired() && v.getLocale().getLanguage().equals(Locale.ENGLISH.getLanguage())) {
                    ttsReady = tts.setVoice(v) == TextToSpeech.SUCCESS; break;
                }
            }
        });
        tts.setOnUtteranceProgressListener(new UtteranceProgressListener() {
            public void onStart(String id) {}
            public void onDone(String id) { runOnUiThread(() -> { speaking = false; update(); resumeWake(); }); }
            public void onError(String id) { runOnUiThread(() -> { speaking = false; update(); }); }
        });
        status.setText(signedIn() ? "Ask Micro, or tap Speak." : "Sign in to your Micro account to begin.");
        if (saved != null && signedIn()) {
            thread = saved.getString("thread", "");
            agent = saved.getString("agent", "micro");
            if (!thread.isEmpty()) message("Micro", "Continuing your conversation. Earlier messages are in your web Inbox.");
        }
        update();
        callback(getIntent());
    }

    private boolean signedIn() {
        return !account.optString("token").isEmpty() && account.optLong("expires") > System.currentTimeMillis();
    }
    private void update() {
        if (destroyed) return;
        send.setEnabled(!busy && signedIn());
        speak.setEnabled(!busy && signedIn());
        settings.setEnabled(!busy); newChat.setEnabled(!busy);
        wake.setEnabled(!busy && signedIn());
        speak.setText(listening || speaking ? R.string.stop : R.string.speak);
    }
    private void message(String who, String text) {
        TextView view = new TextView(this);
        view.setTextSize(16); view.setTextIsSelectable(true);
        int gap = (int) (12 * getResources().getDisplayMetrics().density);
        view.setPadding(0, gap, 0, gap);
        markdown.setMarkdown(view, "**" + who + "**\n\n" + text);
        messages.addView(view);
        scroll.post(() -> scroll.fullScroll(View.FOCUS_DOWN));
    }
    private void send() {
        if (busy || !signedIn()) { if (!signedIn()) connection(); return; }
        String text = prompt.getText().toString().trim();
        if (text.isEmpty()) return;
        stopAudio();
        busy = true; update(); prompt.setText("");
        message("You", text); status.setText("Micro is working…");
        final String server = account.optString("server"), token = account.optString("token");
        final String currentThread = thread, currentAgent = agent;
        worker.execute(() -> {
            try {
                JSONObject reply = AgentApi.ask(server, token, currentAgent, currentThread, text);
                String response = reply.getString("text");
                String next = reply.getString("thread");
                runOnUiThread(() -> {
                    if (destroyed) return;
                    thread = next; busy = false;
                    message("Micro", response); status.setText(""); update();
                    if (foreground && spoken.isChecked() && ttsReady) {
                        speaking = true; update();
                        String plain = markdown.toMarkdown(response).toString();
                        int limit = TextToSpeech.getMaxSpeechInputLength();
                        int result = tts.speak(plain.substring(0, Math.min(plain.length(), limit)),
                                TextToSpeech.QUEUE_FLUSH, null, "reply");
                        if (result == TextToSpeech.ERROR) { speaking = false; update(); }
                    } else {
                        if (spoken.isChecked() && !ttsReady) status.setText("Install an offline English voice in Android settings for spoken replies.");
                        resumeWake();
                    }
                });
            } catch (Exception e) {
                runOnUiThread(() -> {
                    if (destroyed) return;
                    busy = false; update();
                    status.setText(e.getMessage() + " Nothing was retried. Check your web Inbox before sending again.");
                });
            }
        });
    }
    private void startVoice(boolean wakeOnly) {
        if (!foreground || busy || !signedIn()) return;
        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.RECORD_AUDIO}, 1); return;
        }
        if (tts != null) tts.stop();
        speaking = false;
        if (voice == null) {
            status.setText("Preparing offline voice…");
            voice = new VoiceInput(getApplicationContext(), new VoiceInput.Listener() {
                public void ready() { if (foreground) startVoice(wake.isChecked()); }
                public void wake() { startVoice(false); }
                public void heard(String text) {
                    listening = false; update(); prompt.setText(text); prompt.setSelection(text.length());
                    status.setText("Check your message, then tap Send.");
                }
                public void error(String text) { listening = false; update(); status.setText(text); }
            });
            return;
        }
        if (!voice.ready()) { status.setText("Preparing offline voice…"); return; }
        listening = true; update();
        status.setText(wakeOnly ? "Listening locally for “Hey Micro”…" : "Listening… Pause when finished.");
        voice.listen(wakeOnly);
    }
    private void resumeWake() {
        if (foreground && !busy && !speaking && wake.isChecked() && prompt.length() == 0) startVoice(true);
    }
    private void stopAudio() {
        if (voice != null) voice.stop();
        if (tts != null) tts.stop();
        listening = false; speaking = false; update();
    }
    @Override public void onRequestPermissionsResult(int request, String[] permissions, int[] results) {
        super.onRequestPermissionsResult(request, permissions, results);
        if (request == 1 && results.length > 0 && results[0] == PackageManager.PERMISSION_GRANTED) startVoice(wake.isChecked());
        else { wake.setChecked(false); status.setText("Microphone permission denied. You can still type."); }
    }
    private void connection() {
        if (busy) return;
        stopAudio();
        LinearLayout form = new LinearLayout(this); form.setOrientation(LinearLayout.VERTICAL);
        int pad = (int) (20 * getResources().getDisplayMetrics().density); form.setPadding(pad, pad, pad, pad);
        EditText server = new EditText(this); server.setSingleLine(true); server.setHint("Server");
        server.setText(account.optString("server", "https://micro.mu")); form.addView(server);
        EditText name = new EditText(this); name.setSingleLine(true); name.setHint("Agent"); name.setText(agent); form.addView(name);
        TextView info = new TextView(this);
        info.setText("Sign in or create an account in your browser. Your existing plan and credits apply. Voice is English and runs on this device.");
        form.addView(info);
        Button manage = new Button(this); manage.setText("Open account"); form.addView(manage);
        manage.setOnClickListener(v -> {
            try { startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(AgentApi.origin(server.getText().toString()) + "/account"))); }
            catch (Exception e) { status.setText("Enter a valid HTTPS server address."); }
        });
        AlertDialog dialog = new AlertDialog.Builder(this).setTitle("Micro account").setView(form)
                .setPositiveButton("Sign in", null).setNegativeButton("Cancel", null)
                .setNeutralButton("Sign out", (d, w) -> signOut()).create();
        dialog.setOnShowListener(d -> dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener(v -> {
            try {
                String origin = AgentApi.origin(server.getText().toString());
                String selected = AgentApi.agent(name.getText().toString().trim());
                dialog.dismiss(); signOut(); agent = selected; beginSignIn(origin);
            } catch (IllegalArgumentException e) { server.setError(e.getMessage()); }
        }));
        dialog.show();
    }
    private void signOut() {
        stopAudio(); wake.setChecked(false); vault.clear(); account = new JSONObject();
        thread = ""; messages.removeAllViews(); prompt.setText(""); update();
        status.setText("Signed out on this device. You can revoke access in Account → Tokens.");
    }
    private void beginSignIn(String server) {
        busy = true; update(); status.setText("Opening sign-in…");
        worker.execute(() -> {
            try {
                JSONObject pending = OAuth.begin(server, BuildConfig.OAUTH_REDIRECT);
                JSONObject stored = new JSONObject().put("pending", pending).put("server", server);
                String url = OAuth.authorize(pending);
                runOnUiThread(() -> {
                    if (destroyed) return;
                    try { vault.write(stored); }
                    catch (Exception e) { busy = false; update(); status.setText("Could not save sign-in securely."); return; }
                    account = stored; busy = false; update();
                    try { startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(url))); status.setText("Complete sign-in in your browser."); }
                    catch (Exception e) { status.setText("No browser available to sign in."); }
                });
            } catch (Exception e) {
                runOnUiThread(() -> { if (!destroyed) { busy = false; update(); status.setText("Could not start sign-in. Check the server and try again."); } });
            }
        });
    }
    @Override protected void onNewIntent(Intent intent) { super.onNewIntent(intent); setIntent(intent); callback(intent); }
    private void callback(Intent intent) {
        Uri uri = intent.getData();
        if (uri == null || busy) return;
        Uri expected = Uri.parse(BuildConfig.OAUTH_REDIRECT);
        if (!expected.equals(uri.buildUpon().clearQuery().fragment(null).build()) || uri.getFragment() != null) return;
        intent.setData(null);
        JSONObject pending = account.optJSONObject("pending");
        String code = uri.getQueryParameter("code");
        if (pending == null || code == null || !OAuth.validState(pending, uri.getQueryParameter("state"), System.currentTimeMillis())) {
            status.setText("Sign-in expired or did not match. Please sign in again."); return;
        }
        busy = true; update();
        vault.clear();
        worker.execute(() -> {
            try {
                // Callback is single-use even if the process stops during exchange.
                JSONObject token = OAuth.exchange(pending, code);
                JSONObject connected = new JSONObject().put("server", pending.getString("server"))
                        .put("token", token.getString("access_token"))
                        .put("expires", System.currentTimeMillis() + token.getLong("expires_in") * 1000);
                runOnUiThread(() -> {
                    if (destroyed) return;
                    try { vault.write(connected); }
                    catch (Exception e) { busy = false; update(); status.setText("Could not save sign-in securely. Please reconnect."); return; }
                    account = connected; thread = ""; messages.removeAllViews(); busy = false; update();
                    status.setText("Connected to " + connected.optString("server") + ". Ask Micro or tap Speak.");
                });
            } catch (Exception e) {
                runOnUiThread(() -> { if (!destroyed) { busy = false; account = new JSONObject(); update(); status.setText("Sign-in failed. Please try again."); } });
            }
        });
    }
    @Override protected void onResume() { super.onResume(); foreground = true; update(); }
    @Override protected void onPause() {
        foreground = false; stopAudio();
        if (wake != null) wake.setChecked(false); // Never silently resume ambient listening.
        super.onPause();
    }
    @Override protected void onSaveInstanceState(Bundle out) {
        out.putString("thread", thread); out.putString("agent", agent); super.onSaveInstanceState(out);
    }
    @Override protected void onDestroy() {
        destroyed = true; if (voice != null) voice.close();
        if (tts != null) { tts.stop(); tts.shutdown(); }
        worker.shutdown(); super.onDestroy();
    }
}
