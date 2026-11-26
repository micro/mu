package mu.client.android;

import org.junit.Test;
import static org.junit.Assert.*;
import org.json.JSONObject;
import com.google.protobuf.util.JsonFormat;
import mu.micro.client.proto.assistant.v1.*;

public class AppClientTest {
    @Test public void customerContractsAcceptBothJsonSpellings() throws Exception {
        PaidSession.Builder receipt = PaidSession.newBuilder();
        JsonFormat.parser().merge("{\"id\":\"session\",\"started_at\":123,\"ends_at\":456,\"status\":\"active\"}", receipt);
        assertEquals(456, receipt.getEndsAt());
        JSONObject mapped = new JSONObject(JsonFormat.printer().print(receipt));
        assertEquals(456, mapped.getLong("endsAt"));
        AppSessions.Builder sessions = AppSessions.newBuilder();
        JsonFormat.parser().merge("{\"sessions\":[" + mapped + "]}", sessions);
        assertEquals("session", sessions.getSessions(0).getId());
    }
    @Test public void appCannotOverrideItsServer() throws Exception {
        for (String server : new String[]{"https://foreign.example", "https://micro.mu/agent/malten"}) {
            try { AppApi.endpoint(server, "profile"); fail("accepted " + server); }
            catch (java.io.IOException | IllegalArgumentException expected) {}
        }
        if (BuildConfig.BRANDED) {
            assertEquals(BuildConfig.FIXED_SERVER + "/agent/" + BuildConfig.FIXED_AGENT + "/app/profile",
                AppApi.endpoint(BuildConfig.FIXED_SERVER, "profile"));
            assertEquals(BuildConfig.APPLICATION_ID + "://oauth/callback", BuildConfig.OAUTH_REDIRECT);
        }
    }
    @Test public void brandedConsentOnlyRequestsAgentAccess() throws Exception {
        JSONObject pending = new JSONObject().put("server", "https://micro.mu").put("client", "test")
            .put("redirect", BuildConfig.OAUTH_REDIRECT).put("state", "test").put("verifier", "verifier");
        String url = java.net.URLDecoder.decode(OAuth.authorize(pending), "UTF-8");
        assertTrue(url.contains("scope=read write api:agent"));
        assertEquals(!BuildConfig.BRANDED, url.contains("api:inbox"));
        assertTrue(url.contains("code_challenge_method=S256"));
    }
}
