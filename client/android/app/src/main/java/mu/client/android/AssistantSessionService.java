package mu.client.android;

import android.content.Intent;
import android.os.Bundle;
import android.service.voice.VoiceInteractionSession;
import android.service.voice.VoiceInteractionSessionService;

public final class AssistantSessionService extends VoiceInteractionSessionService {
    @Override public VoiceInteractionSession onNewSession(Bundle args) {
        return new VoiceInteractionSession(this) {
            @Override public void onCreate() {
                super.onCreate();
                setUiEnabled(false);
                setDisabledShowContext(SHOW_WITH_ASSIST | SHOW_WITH_SCREENSHOT);
            }
            @Override public void onShow(Bundle args, int flags) {
                super.onShow(args, flags);
                startVoiceActivity(new Intent(getContext(), AssistantActivity.class));
            }
        };
    }
}
