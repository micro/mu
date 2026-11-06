package mu.client.android;

import android.content.Intent;
import android.service.voice.VoiceInteractionService;
import android.service.voice.VoiceInteractionSession;

/** System activation only. No continuous microphone or screen-content capture. */
public final class AssistantService extends VoiceInteractionService {
    @Override public void onReady() {
        super.onReady();
        setDisabledShowContext(VoiceInteractionSession.SHOW_WITH_ASSIST | VoiceInteractionSession.SHOW_WITH_SCREENSHOT);
    }
    @Override public void onLaunchVoiceAssistFromKeyguard() {
        startActivity(new Intent(this, AssistantActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
    }
}
