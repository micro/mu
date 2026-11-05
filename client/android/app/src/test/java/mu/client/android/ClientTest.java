package mu.client.android;

import org.json.JSONObject;
import org.junit.Test;
import static org.junit.Assert.*;

public class ClientTest {
    @Test public void acceptsOnlyServerOrigins() {
        assertEquals("https://micro.mu", AgentApi.origin("https://micro.mu/"));
        assertEquals("https://example.org:8443", AgentApi.origin("https://example.org:8443"));
        for (String bad : new String[]{"http://micro.mu", "https://user:password@micro.mu", "https://micro.mu/agent",
                "https://micro.mu?token=x", "https://micro.mu#fragment", "file:///tmp"}) {
            try { AgentApi.origin(bad); fail(bad); } catch (IllegalArgumentException expected) {}
        }
    }
    @Test public void agentCannotChangeRequestPath() {
        for (String bad : new String[]{"../account", "micro?x", "micro#x", "/micro", "micro%2Fadmin"}) {
            try { AgentApi.agent(bad); fail(bad); } catch (IllegalArgumentException expected) {}
        }
    }
    @Test public void pkceMatchesRFC7636() throws Exception {
        assertEquals("E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
                OAuth.challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"));
    }
    @Test public void callbackRequiresFreshMatchingState() throws Exception {
        JSONObject pending = new JSONObject().put("state", "expected").put("created", 1000);
        assertTrue(OAuth.validState(pending, "expected", 2000));
        assertFalse(OAuth.validState(pending, "wrong", 2000));
        assertFalse(OAuth.validState(pending, null, 2000));
        assertFalse(OAuth.validState(pending, "expected", 1000 + 600000));
        assertFalse(OAuth.validState(pending, "expected", 999));
    }
}
