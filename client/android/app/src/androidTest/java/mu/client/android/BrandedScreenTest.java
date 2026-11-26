package mu.client.android;

import android.content.Intent;
import android.net.Uri;
import android.widget.Button;
import androidx.test.core.app.ActivityScenario;
import androidx.test.platform.app.InstrumentationRegistry;
import org.junit.Test;
import static org.junit.Assert.*;
import static org.junit.Assume.assumeTrue;

public class BrandedScreenTest {
    @Test public void fixedIdentitySurvivesForeignLinkAndRecreation() throws Exception {
        assumeTrue(BuildConfig.BRANDED);
        android.content.Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        Vault vault = new Vault(context); vault.clear();
        Intent link = new Intent(context, MainActivity.class).setAction(Intent.ACTION_VIEW)
            .setData(Uri.parse("micro-agent://open?server=https%3A%2F%2Fforeign.example&agent=other"));
        try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(link)) {
            scenario.onActivity(activity -> {
                activity.selectAgent("other", "Other", "Should be ignored");
                assertEquals(BuildConfig.APP_NAME, ((Button) activity.findViewById(R.id.agent_picker)).getText().toString());
                assertFalse(activity.findViewById(R.id.agent_picker).isEnabled());
                assertFalse(activity.findViewById(R.id.send).isEnabled());
            });
            scenario.recreate();
            scenario.onActivity(activity -> assertEquals(BuildConfig.APP_NAME,
                ((Button) activity.findViewById(R.id.agent_picker)).getText().toString()));
        } finally { vault.clear(); }
        Intent callback = Intent.parseUri("intent://oauth/callback?code=test&state=test#Intent;scheme="
            + BuildConfig.APPLICATION_ID + ";package=" + BuildConfig.APPLICATION_ID + ";end", Intent.URI_INTENT_SCHEME);
        assertNotNull(context.getPackageManager().resolveActivity(callback, android.content.pm.PackageManager.MATCH_DEFAULT_ONLY));
    }
}
