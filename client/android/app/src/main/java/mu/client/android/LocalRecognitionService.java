package mu.client.android;

import android.content.Intent;
import android.os.Bundle;
import android.os.RemoteException;
import android.speech.RecognitionService;
import android.speech.SpeechRecognizer;
import java.util.ArrayList;
import java.util.Collections;

/** Android's recognizer contract, backed by the same local Vosk recognizer. */
public final class LocalRecognitionService extends RecognitionService {
    private VoiceInput voice;
    private Callback pending;
    @Override protected void onStartListening(Intent intent, Callback callback) {
        if (pending != null) { error(callback, SpeechRecognizer.ERROR_RECOGNIZER_BUSY); return; }
        pending = callback;
        voice = new VoiceInput(getApplicationContext(), new VoiceInput.Listener() {
            public void ready() {
                if (pending != callback) return;
                try { callback.readyForSpeech(new Bundle()); } catch (RemoteException e) { clear(); return; }
                voice.listen(false);
            }
            public void detected() {}
            public void partial(String text) {
                if (pending != callback) return;
                try { callback.partialResults(result(text)); } catch (RemoteException e) { clear(); }
            }
            public void heard(String text) {
                if (pending != callback) return;
                try { callback.endOfSpeech(); callback.results(result(text)); } catch (RemoteException ignored) { }
                clear();
            }
            public void wake(String text) { heard(text); }
            public void error(String text) { if (pending == callback) { LocalRecognitionService.this.error(callback, SpeechRecognizer.ERROR_NO_MATCH); clear(); } }
        });
    }
    private Bundle result(String text) {
        Bundle out = new Bundle();
        out.putStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION, new ArrayList<>(Collections.singletonList(text)));
        return out;
    }
    private void error(Callback callback, int code) { try { callback.error(code); } catch (RemoteException ignored) { } }
    private void clear() { pending = null; if (voice != null) { voice.close(); voice = null; } }
    @Override protected void onStopListening(Callback callback) { error(callback, SpeechRecognizer.ERROR_NO_MATCH); clear(); }
    @Override protected void onCancel(Callback callback) { clear(); }
    @Override public void onDestroy() { clear(); super.onDestroy(); }
}
