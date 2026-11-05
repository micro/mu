package mu.client.android;

import android.content.Context;
import org.json.JSONObject;
import org.vosk.Model;
import org.vosk.Recognizer;
import org.vosk.android.RecognitionListener;
import org.vosk.android.SpeechService;
import org.vosk.android.StorageService;

// All audio stays on the device. Foreground only: the activity stops this on pause.
final class VoiceInput {
    interface Listener { void ready(); void detected(); void partial(String text); void heard(String text); void wake(String command); void error(String message); }
    private Model model;
    private SpeechService speech;
    private Recognizer recognizer;
    private int generation;
    private boolean closed;
    final Listener listener;
    VoiceInput(Context context, Listener listener) {
        this.listener = listener;
        StorageService.unpack(context, "model-en-us", "speech", m -> {
            if (closed) { m.close(); return; }
            model = m;
            listener.ready();
        }, e -> { if (!closed) listener.error("Voice model could not load. You can still type."); });
    }
    boolean ready() { return model != null && !closed; }
    void listen(boolean wakeOnly) {
        stop();
        if (!ready()) { listener.error("Voice is still loading. You can type meanwhile."); return; }
        int current = generation;
        try {
            recognizer = new Recognizer(model, 16000);
            speech = new SpeechService(recognizer, 16000);
            RecognitionListener callbacks = new RecognitionListener() {
                private boolean detected;
                private void acknowledge() {
                    if (!detected) { detected = true; listener.detected(); }
                }
                private void result(String json) {
                    if (current != generation || closed) return;
                    try {
                        String text = new JSONObject(json).optString("text").trim();
                        if (text.isEmpty()) {
                            if (!wakeOnly || detected) { stop(); listener.error("I didn’t catch that. Tap Speak to try again."); }
                            return;
                        }
                        if (wakeOnly) {
                            String command = VoiceCommands.afterWake(text);
                            if (command != null) { acknowledge(); stop(); listener.wake(command); }
                            else if (detected) { stop(); listener.error("I didn’t catch that. Tap Speak to try again."); }
                        } else { stop(); listener.heard(text); }
                    } catch (Exception e) { stop(); listener.error("Could not understand that. Please try again."); }
                }
                public void onPartialResult(String json) {
                    if (current != generation || closed) return;
                    try {
                        String text = new JSONObject(json).optString("partial").trim();
                        if (wakeOnly) {
                            String command = VoiceCommands.afterWake(text);
                            if (command == null) return;
                            acknowledge(); listener.partial(command);
                        } else listener.partial(text);
                    } catch (Exception ignored) { /* Wait for a complete recognizer result. */ }
                }
                public void onResult(String json) { result(json); }
                public void onFinalResult(String json) { result(json); }
                public void onError(Exception e) {
                    if (current != generation) return;
                    stop(); listener.error("Microphone unavailable. Please try again.");
                }
                public void onTimeout() {
                    if (current != generation) return;
                    stop(); listener.error("I didn’t catch that. Tap Speak or toggle hands-free to try again.");
                }
            };
            if (wakeOnly) speech.startListening(callbacks); else speech.startListening(callbacks, 30000);
        } catch (Exception e) { stop(); listener.error("Microphone unavailable. You can still type."); }
    }
    void stop() {
        generation++;
        if (speech != null) { speech.cancel(); speech.shutdown(); speech = null; }
        if (recognizer != null) { recognizer.close(); recognizer = null; }
    }
    void close() {
        closed = true; stop();
        if (model != null) { model.close(); model = null; }
    }
}
