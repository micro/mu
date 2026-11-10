package mu.client.android;

import org.json.JSONArray;
import org.json.JSONObject;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.util.Base64;

final class OAuth {
    static String random() {
        byte[] bytes = new byte[32];
        new SecureRandom().nextBytes(bytes);
        return Base64.getUrlEncoder().withoutPadding().encodeToString(bytes);
    }
    static String challenge(String verifier) throws Exception {
        return Base64.getUrlEncoder().withoutPadding().encodeToString(
                MessageDigest.getInstance("SHA-256").digest(verifier.getBytes(StandardCharsets.US_ASCII)));
    }
    static String encode(String value) {
        try { return URLEncoder.encode(value, "UTF-8"); }
        catch (Exception e) { throw new IllegalArgumentException(e); }
    }
    static JSONObject begin(String server, String redirect) throws Exception {
        server = AgentApi.origin(server);
        JSONObject registration = AgentApi.post(server + "/oauth/register", "",
                new JSONObject().put("client_name", "Mu for Android")
                        .put("redirect_uris", new JSONArray().put(redirect)).toString(), "application/json");
        return new JSONObject().put("server", server).put("client", registration.getString("client_id"))
                .put("verifier", random()).put("state", random()).put("created", System.currentTimeMillis())
                .put("redirect", redirect);
    }
    static String authorize(JSONObject pending) throws Exception {
        return pending.getString("server") + "/oauth/authorize?response_type=code"
                + "&client_id=" + encode(pending.getString("client"))
                + "&redirect_uri=" + encode(pending.getString("redirect"))
                + "&scope=" + encode("read write api:agent api:home api:inbox api:services")
                + "&state=" + encode(pending.getString("state"))
                + "&code_challenge_method=S256&code_challenge=" + encode(challenge(pending.getString("verifier")));
    }
    static boolean validState(JSONObject pending, String state, long now) {
        long created = pending.optLong("created");
        return state != null && created > 0 && now >= created && now - created < 10 * 60 * 1000
                && MessageDigest.isEqual(state.getBytes(StandardCharsets.UTF_8),
                pending.optString("state").getBytes(StandardCharsets.UTF_8));
    }
    static JSONObject exchange(JSONObject pending, String code) throws Exception {
        return AgentApi.post(pending.getString("server") + "/oauth/token", "",
                "grant_type=authorization_code&client_id=" + encode(pending.getString("client"))
                        + "&redirect_uri=" + encode(pending.getString("redirect"))
                        + "&code_verifier=" + encode(pending.getString("verifier"))
                        + "&code=" + encode(code), "application/x-www-form-urlencoded");
    }
}
