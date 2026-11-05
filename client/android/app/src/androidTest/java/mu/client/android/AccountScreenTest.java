package mu.client.android;

import android.Manifest;
import android.content.pm.PackageManager;
import android.widget.Button;
import android.widget.EditText;
import androidx.test.core.app.ActivityScenario;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.junit.Test;
import org.junit.runner.RunWith;
import static org.junit.Assert.*;

@RunWith(AndroidJUnit4.class)
public class AccountScreenTest {
    @Test public void signedOutAppStartsWithoutMicrophoneOrAccount() {
        try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(MainActivity.class)) {
            scenario.onActivity(activity -> {
                assertEquals(PackageManager.PERMISSION_DENIED,
                        activity.checkSelfPermission(Manifest.permission.RECORD_AUDIO));
                assertFalse(activity.findViewById(R.id.send).isEnabled());
                assertFalse(activity.findViewById(R.id.speak).isEnabled());
                assertEquals("Sign in", ((Button) activity.findViewById(R.id.settings)).getText().toString());
                assertTrue(activity.findViewById(R.id.settings).performClick());
            });
            InstrumentationRegistry.getInstrumentation().waitForIdleSync();
        }
    }
    @Test public void draftSurvivesRotationAndAccountIsStillRequired() {
        try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(MainActivity.class)) {
            scenario.onActivity(activity -> ((EditText) activity.findViewById(R.id.prompt)).setText("Hello Micro"));
            scenario.recreate();
            scenario.onActivity(activity -> {
                assertEquals("Hello Micro", ((EditText) activity.findViewById(R.id.prompt)).getText().toString());
                assertFalse(activity.findViewById(R.id.send).isEnabled());
            });
        }
    }
}
