package mu.client.android;

import android.Manifest;
import android.app.Activity;
import android.app.KeyguardManager;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.os.Bundle;
import android.speech.tts.TextToSpeech;
import android.speech.tts.UtteranceProgressListener;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.TextView;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.JSONObject;

/** A short, explicitly invoked voice interaction, not an always-on listener. */
public final class AssistantActivity extends Activity {
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private VoiceInput voice;
    private TextToSpeech tts;
    private io.noties.markwon.Markwon markdown;
    private TextView name, status, answer;
    private Button speak, open;
    private JSONObject account = new JSONObject();
    private boolean active, listening, working, speaking, ttsReady, publicMode, invoked, wantListen;
    private int generation;
    private String agent = "micro", label = "Micro", thread = "";

    boolean locked() { return getSystemService(KeyguardManager.class).isDeviceLocked(); }

    @Override public void onCreate(Bundle saved) {
        super.onCreate(saved);
        if (android.os.Build.VERSION.SDK_INT >= 27) { setShowWhenLocked(true); setTurnScreenOn(true); }
        else getWindow().addFlags(WindowManager.LayoutParams.FLAG_SHOW_WHEN_LOCKED | WindowManager.LayoutParams.FLAG_TURN_SCREEN_ON);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_SECURE);
        setContentView(R.layout.assistant);
        markdown = io.noties.markwon.Markwon.create(this);
        name = findViewById(R.id.assistant_name); status = findViewById(R.id.assistant_status);
        answer = findViewById(R.id.assistant_answer); speak = findViewById(R.id.assistant_speak); open = findViewById(R.id.assistant_open);
        findViewById(R.id.assistant_close).setOnClickListener(v -> finish());
        speak.setOnClickListener(v -> { if (listening || speaking || working) cancel(); else listen(); });
        open.setOnClickListener(v -> unlockAndOpen());
        tts = new TextToSpeech(this, result -> {
            if (!isDestroyed() && result == TextToSpeech.SUCCESS) ttsReady = OfflineSpeech.select(tts);
        });
        tts.setOnUtteranceProgressListener(new UtteranceProgressListener() {
            public void onStart(String id) {}
            public void onDone(String id) { runOnUiThread(() -> { if (id.equals("reply-" + generation)) { speaking = false; update(); } }); }
            public void onError(String id) { onDone(id); }
        });
    }
    @Override public void onResume() {
        super.onResume(); active = true; publicMode = locked(); answer.setText("");
        try { account = new Vault(this).read(); } catch (Exception e) { account = new JSONObject(); }
        agent = account.optString("agent", "micro"); label = account.optString("agent_name", "Micro");
        JSONObject threads = account.optJSONObject("threads");
        thread = threads == null ? "" : threads.optString(agent);
        name.setText(publicMode ? "Mu · Public questions" : label);
        open.setText(publicMode ? "Unlock" : "Open app");
        status.setText(publicMode ? "Ask public questions, such as weather in a town. Unlock for your agent." : "Speak to " + label);
        update();
        // Begin only after the UI is visible, never from an unsolicited broadcast.
        if (!invoked) { invoked = true; speak.post(() -> { if (active) listen(); }); }
    }
    private boolean signedIn() { return !account.optString("token").isEmpty() && account.optLong("expires") > System.currentTimeMillis(); }
    private void update() {
        speak.setText(listening || speaking || working ? "Stop" : "Speak");
        if (active && (listening || speaking || working)) getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        else getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
    }
    private void cancel() {
        generation++; wantListen = false; listening = false; working = false; speaking = false;
        if (voice != null) voice.stop();
        if (tts != null) tts.stop();
        status.setText("Stopped. A request already sent may still complete; it will not be retried."); update();
    }
    private void listen() {
        if (!active || working) return;
        if (!publicMode && locked()) { finish(); return; }
        if (!signedIn()) { status.setText("Unlock and open Mu to sign in."); return; }
        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            if (locked()) { status.setText("Unlock and open Mu to allow microphone access."); return; }
            requestPermissions(new String[]{Manifest.permission.RECORD_AUDIO}, 1); return;
        }
        wantListen = true;
        if (voice == null) {
            status.setText("Preparing offline voice…");
            voice = new VoiceInput(getApplicationContext(), new VoiceInput.Listener() {
                public void ready() { if (active && wantListen) listen(); }
                public void detected() {}
                public void partial(String text) { if (active) status.setText(text.isEmpty() ? "Listening…" : "Hearing: “" + text + "”"); }
                public void heard(String text) { submit(text); }
                public void wake(String text) { submit(text); }
                public void error(String text) { if (active) { listening = false; status.setText(text); update(); } }
            });
            return;
        }
        if (!voice.ready()) return;
        if (tts != null) tts.stop(); speaking = false;
        listening = true; status.setText(publicMode ? "Mu is listening · public questions only" : label + " is listening…");
        speak.performHapticFeedback(android.view.HapticFeedbackConstants.VIRTUAL_KEY);
        voice.listen(false); update();
    }
    private void submit(String text) {
        if (!active || working || (!publicMode && locked())) return;
        wantListen = false; listening = false; working = true; answer.setText(""); status.setText("“" + text + "”\nWorking…"); update();
        final int request = ++generation;
        final boolean restricted = publicMode;
        final String server = account.optString("server"), token = account.optString("token"), selected = agent, previous = thread;
        worker.execute(() -> {
            try {
                JSONObject reply = restricted ? AgentApi.askPublic(server, token, text) : AgentApi.ask(server, token, selected, previous, text, true);
                runOnUiThread(() -> {
                    if (!active || request != generation || (!restricted && locked())) return;
                    working = false;
                    if (!restricted) {
                        thread = reply.optString("thread");
                        try {
                            Vault vault = new Vault(this); JSONObject latest = vault.read();
                            if (!token.equals(latest.optString("token")) || !server.equals(latest.optString("server"))) { finish(); return; }
                            JSONObject threads = latest.optJSONObject("threads"); if (threads == null) threads = new JSONObject();
                            threads.put(selected, thread); latest.put("threads", threads); vault.write(latest);
                        } catch (Exception e) { status.setText("Answer received, but could not save the conversation on this device."); }
                    }
                    String response = reply.optString("text"); markdown.setMarkdown(answer, response); status.setText(restricted ? "Public answer · unlock for personal requests" : label);
                    if (ttsReady) {
                        java.util.List<String> pages = SpeechPages.split(answer.getText().toString(), TextToSpeech.getMaxSpeechInputLength() - 100);
                        speaking = !pages.isEmpty();
                        for (int i = 0; i < pages.size(); i++) tts.speak(pages.get(i), i == 0 ? TextToSpeech.QUEUE_FLUSH : TextToSpeech.QUEUE_ADD, null,
                            i == pages.size() - 1 ? "reply-" + generation : "part-" + generation + "-" + i);
                    }
                    if (!ttsReady) status.append(" · Install an offline English voice for spoken replies.");
                    update();
                });
            } catch (Exception e) { runOnUiThread(() -> { if (active && request == generation) { working = false; status.setText(e.getMessage()); update(); } }); }
        });
    }
    private void unlockAndOpen() {
        cancel();
        Runnable go = () -> { if (!locked()) { startActivity(new Intent(this, MainActivity.class)); finish(); } };
        if (locked()) getSystemService(KeyguardManager.class).requestDismissKeyguard(this, new KeyguardManager.KeyguardDismissCallback() {
            @Override public void onDismissSucceeded() { go.run(); }
        }); else go.run();
    }
    @Override public void onRequestPermissionsResult(int code, String[] permissions, int[] grants) {
        super.onRequestPermissionsResult(code, permissions, grants);
        if (code == 1 && grants.length > 0 && grants[0] == PackageManager.PERMISSION_GRANTED) listen();
        else status.setText("Microphone permission is required to speak. You can use the main app to type.");
    }
    @Override public void onPause() { active = false; cancel(); answer.setText(""); super.onPause(); }
    @Override public void onDestroy() { if (voice != null) voice.close(); if (tts != null) tts.shutdown(); worker.shutdownNow(); super.onDestroy(); }
}
