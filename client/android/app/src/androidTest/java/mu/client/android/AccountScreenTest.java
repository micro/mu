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
    @Test public void selectedAgentAndSeparateThreadsSurviveRecreation() throws Exception {
        android.content.Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        Vault vault = new Vault(context); vault.clear();
        vault.write(new org.json.JSONObject().put("agent", "micro").put("threads", new org.json.JSONObject().put("micro", "micro-thread").put("malten", "malten-thread")));
        try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(MainActivity.class)) {
            scenario.onActivity(activity -> {
                activity.selectAgent("malten", "Malten", "For reflection");
                assertEquals("Hands-free “Hey Malten” while open", ((android.widget.Switch) activity.findViewById(R.id.wake)).getText().toString());
            });
            scenario.recreate();
            scenario.onActivity(activity -> {
                assertEquals("Malten", ((Button) activity.findViewById(R.id.agent_picker)).getText().toString());
                assertEquals("Hands-free “Hey Malten” while open", ((android.widget.Switch) activity.findViewById(R.id.wake)).getText().toString());
                assertEquals("For reflection", ((android.widget.TextView) activity.findViewById(R.id.agent_description)).getText().toString());
            });
            org.json.JSONObject saved = vault.read();
            assertEquals("malten", saved.getString("agent"));
            assertEquals("micro-thread", saved.getJSONObject("threads").getString("micro"));
            assertEquals("malten-thread", saved.getJSONObject("threads").getString("malten"));
        } finally { vault.clear(); }
    }
    @Test public void tappingOutsideComposerClearsFocusWithoutLosingDraft() {
        try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(MainActivity.class)) {
            scenario.onActivity(activity -> {
                EditText input = activity.findViewById(R.id.prompt);
                input.setText("Unsent draft"); input.requestFocus();
                assertTrue(input.hasFocus());
                android.graphics.Rect bounds = new android.graphics.Rect();
                input.getGlobalVisibleRect(bounds);
                long now = android.os.SystemClock.uptimeMillis();
                android.view.MotionEvent inside = android.view.MotionEvent.obtain(now, now, android.view.MotionEvent.ACTION_DOWN, bounds.centerX(), bounds.centerY(), 0);
                activity.dispatchTouchEvent(inside); inside.recycle();
                assertTrue(input.hasFocus());
                android.view.MotionEvent outside = android.view.MotionEvent.obtain(now, now, android.view.MotionEvent.ACTION_DOWN, 1, 1, 0);
                activity.dispatchTouchEvent(outside); outside.recycle();
                assertFalse(input.hasFocus());
                assertEquals("Unsent draft", input.getText().toString());
            });
        }
    }

    @Test public void invalidAgentLinkCannotSwitchServer() {
        android.content.Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        new Vault(context).clear();
        android.content.Intent link = new android.content.Intent(context, MainActivity.class).setAction(android.content.Intent.ACTION_VIEW)
            .setData(android.net.Uri.parse("micro-agent://open?server=http%3A%2F%2Fevil.example&agent=malten"));
        try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(link)) {
            scenario.onActivity(activity -> assertEquals("That agent link is invalid.", ((android.widget.TextView) activity.findViewById(R.id.status)).getText().toString()));
        } finally { new Vault(context).clear(); }
    }

    @Test public void browserReturnIntentResolvesToInstalledApp() throws Exception {
        android.content.Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        String app = context.getPackageName();
        android.content.Intent intent = android.content.Intent.parseUri(
                "intent://oauth/callback?code=test&state=test#Intent;scheme=" + app + ";package=" + app + ";end",
                android.content.Intent.URI_INTENT_SCHEME);
        intent.addCategory(android.content.Intent.CATEGORY_BROWSABLE);
        android.content.pm.ResolveInfo resolved = context.getPackageManager().resolveActivity(intent, PackageManager.MATCH_DEFAULT_ONLY);
        assertNotNull(resolved);
        assertEquals(app, resolved.activityInfo.packageName);
        assertEquals("mu.client.android.MainActivity", resolved.activityInfo.name);
        assertEquals("test", intent.getData().getQueryParameter("state"));
    }

    @Test public void visiblePlayerOpensAndClosesWithoutAnAccountCredential() {
        try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(MainActivity.class)) {
            scenario.onActivity(activity -> {
                VideoPlayer player = new VideoPlayer(activity, () -> {});
                android.widget.LinearLayout messages = activity.findViewById(R.id.messages);
                int before = messages.getChildCount();
                activity.findViewById(R.id.prompt).requestFocus();
                player.show("abcdefghijk", "Playback test", false);
                assertEquals(before + 1, messages.getChildCount());
                assertFalse(activity.findViewById(R.id.prompt).hasFocus());
                assertTrue(player.active());
                assertFalse(player.playing());
                player.pause();
                player.close();
                assertFalse(player.active());
                assertEquals(before, messages.getChildCount());
                player.show("javascript:alert(1)", "Invalid", true);
                assertFalse(player.active());
            });
        }
    }
    @Test public void conversationWordsAreNotConsumedWithoutPlayback() {
        try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(MainActivity.class)) {
            scenario.onActivity(activity -> {
                assertFalse(activity.localCommand("continue"));
                assertFalse(activity.localCommand("next"));
                assertFalse(activity.localCommand("previous"));
                assertFalse(activity.localCommand("back"));
            });
        }
    }
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
