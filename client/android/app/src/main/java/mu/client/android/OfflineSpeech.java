package mu.client.android;

import android.speech.tts.TextToSpeech;
import android.speech.tts.Voice;
import java.util.Comparator;
import java.util.Locale;

final class OfflineSpeech {
    static boolean select(TextToSpeech speech) {
        if (speech.getVoices() == null) return false;
        // Prefer the device locale and higher-quality installed voices. Never
        // silently send reply text to a network speech provider.
        Voice preferred = speech.getDefaultVoice();
        return speech.getVoices().stream()
            .filter(v -> !v.isNetworkConnectionRequired()
                && v.getLocale().getLanguage().equals(Locale.ENGLISH.getLanguage())
                && (v.getFeatures() == null || !v.getFeatures().contains(TextToSpeech.Engine.KEY_FEATURE_NOT_INSTALLED)))
            .sorted(Comparator.comparingInt((Voice v) -> v.getLocale().equals(Locale.getDefault()) ? 1 : 0).reversed()
                .thenComparing(Comparator.comparingInt(Voice::getQuality).reversed())
                .thenComparing(v -> !v.equals(preferred)).thenComparing(Voice::getName))
            .anyMatch(v -> speech.setVoice(v) == TextToSpeech.SUCCESS);
    }
}
