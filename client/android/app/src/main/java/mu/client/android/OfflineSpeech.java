package mu.client.android;

import android.speech.tts.TextToSpeech;
import android.speech.tts.Voice;
import java.util.Locale;

final class OfflineSpeech {
    static boolean select(TextToSpeech speech) {
        if (speech.getVoices() != null) for (Voice voice : speech.getVoices()) {
            if (!voice.isNetworkConnectionRequired() && voice.getLocale().getLanguage().equals(Locale.ENGLISH.getLanguage()))
                return speech.setVoice(voice) == TextToSpeech.SUCCESS;
        }
        return false;
    }
}
