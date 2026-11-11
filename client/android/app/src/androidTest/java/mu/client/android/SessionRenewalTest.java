package mu.client.android;

import android.content.Context;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.JSONObject;
import org.junit.Test;
import org.junit.runner.RunWith;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;
import static org.junit.Assert.*;

@RunWith(AndroidJUnit4.class)
public class SessionRenewalTest {
    @Test public void expiredStoredSessionRenewsOnceAndPreservesAccountState() throws Exception {
        Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        Vault vault = new Vault(context);
        vault.clear();
        ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            String origin = "https://runtime.example";
            JSONObject old = new JSONObject().put("server", origin).put("client", "connection")
                .put("token", "old").put("refresh", "refresh").put("expires", 1).put("agent", "malten");
            vault.write(old);
            AtomicInteger calls = new AtomicInteger();
            Vault.Renewal renewal = (server, account) -> {
                assertEquals(origin,server); assertEquals("refresh",account.getString("refresh"));
                calls.incrementAndGet();
                return new JSONObject().put("access_token","new").put("refresh_token","rotated")
                    .put("token_type","Bearer").put("expires_in",86400);
            };
            Future<String> first = pool.submit(() -> vault.accessToken(origin,"connection",renewal));
            Future<String> second = pool.submit(() -> new Vault(context).accessToken(origin,"connection",renewal));
            assertEquals("new",first.get(15,TimeUnit.SECONDS));
            assertEquals("new",second.get(15,TimeUnit.SECONDS));
            assertEquals(1,calls.get());
            assertEquals("rotated",new Vault(context).read().getString("refresh"));
            old.put("agent","micro"); vault.write(old);
            assertEquals("new",vault.read().getString("token"));
            assertEquals("micro",vault.read().getString("agent"));
            vault.clear();
            try { vault.accessToken(origin,"connection",renewal); fail("Disconnected credential reused"); }
            catch(java.io.IOException expected) { }
        } finally {pool.shutdownNow(); vault.clear();}
    }
    @Test public void disconnectDuringRenewalDoesNotRestoreCredentials() throws Exception {
        Vault vault = new Vault(InstrumentationRegistry.getInstrumentation().getTargetContext());
        vault.clear();
        try {
            vault.write(new JSONObject().put("server","https://runtime.example").put("client","connection")
                .put("token","old").put("refresh","refresh").put("expires",1));
            try {
                vault.accessToken("https://runtime.example","connection",(server,account)->{
                    vault.clear();
                    return new JSONObject().put("access_token","new").put("refresh_token","rotated")
                        .put("token_type","Bearer").put("expires_in",86400);
                });
                fail("Renewal survived disconnect");
            } catch(java.io.IOException expected) { }
            assertFalse(vault.read().has("token"));
        } finally {vault.clear();}
    }
}
