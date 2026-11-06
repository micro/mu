package mu.client.android;

import android.content.ComponentName;
import android.content.Context;
import android.content.pm.PackageManager;
import android.widget.TextView;
import androidx.test.core.app.ActivityScenario;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.junit.Test;
import org.junit.runner.RunWith;
import static org.junit.Assert.*;

@RunWith(AndroidJUnit4.class)
public class AssistantScreenTest {
    @Test public void systemServicesRequireAndroidBindingPermission() throws Exception {
        Context c = InstrumentationRegistry.getInstrumentation().getTargetContext();
        PackageManager pm = c.getPackageManager();
        for (Class<?> service : new Class<?>[]{AssistantService.class, AssistantSessionService.class}) {
            assertEquals("android.permission.BIND_VOICE_INTERACTION", pm.getServiceInfo(new ComponentName(c, service), 0).permission);
        }
        assertEquals("android.permission.BIND_SPEECH_RECOGNITION_SERVICE", pm.getServiceInfo(new ComponentName(c, LocalRecognitionService.class), 0).permission);
        android.content.res.XmlResourceParser xml = c.getResources().getXml(R.xml.voice_interaction);
        try {
            while (xml.next() != org.xmlpull.v1.XmlPullParser.START_TAG) { }
            assertEquals(LocalRecognitionService.class.getName(), xml.getAttributeValue("http://schemas.android.com/apk/res/android", "recognitionService"));
        } finally { xml.close(); }
        assertFalse(pm.getActivityInfo(new ComponentName(c, AssistantActivity.class), 0).exported);
    }
    @Test public void signedOutAssistantCannotStartMicrophoneOrAccountRequest() {
        Context c = InstrumentationRegistry.getInstrumentation().getTargetContext();
        new Vault(c).clear();
        try (ActivityScenario<AssistantActivity> scenario = ActivityScenario.launch(AssistantActivity.class)) {
            scenario.onActivity(a -> {
                a.findViewById(R.id.assistant_speak).performClick();
                assertEquals("Unlock and open Mu to sign in.", ((TextView)a.findViewById(R.id.assistant_status)).getText().toString());
                assertEquals("", ((TextView)a.findViewById(R.id.assistant_answer)).getText().toString());
            });
            scenario.recreate();
        } finally { new Vault(c).clear(); }
    }
}
