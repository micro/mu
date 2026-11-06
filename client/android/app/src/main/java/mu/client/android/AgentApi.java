package mu.client.android;

import org.json.JSONObject;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;

final class AgentApi {
    static String origin(String value) {
        try {
            URI u = new URI(value.trim());
            if (!"https".equals(u.getScheme()) || u.getHost() == null || u.getUserInfo() != null
                    || u.getQuery() != null || u.getFragment() != null
                    || !(u.getPath().isEmpty() || u.getPath().equals("/"))) {
                throw new IllegalArgumentException();
            }
            return new URI("https", null, u.getHost(), u.getPort(), null, null, null).toString();
        } catch (Exception e) {
            throw new IllegalArgumentException("Use an HTTPS server address without a path.");
        }
    }

    static String agent(String name) {
        if (!name.matches("[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}"))
            throw new IllegalArgumentException("Enter an agent name, such as micro.");
        return name;
    }

    static JSONObject ask(String server, String token, String agent, String thread, String prompt, boolean voice) throws Exception {
        if (prompt.getBytes(StandardCharsets.UTF_8).length > 8000)
            throw new IOException("Keep the message below 8,000 bytes.");
        JSONObject body = new JSONObject().put("prompt", prompt).put("thread", thread)
                .put("context", new JSONObject().put("voice", voice));
        return post(origin(server) + "/agent/" + agent(agent), token, body.toString(), "application/json");
    }

    static JSONObject askPublic(String server, String token, String prompt) throws Exception {
        if (prompt.getBytes(StandardCharsets.UTF_8).length > 8000) throw new IOException("Keep the message below 8,000 bytes.");
        return post(origin(server) + "/agent", token,
            new JSONObject().put("prompt", prompt).put("public_only", true).toString(), "application/json");
    }

    static JSONObject catalog(String server, String token) throws Exception {
        HttpURLConnection c = (HttpURLConnection) new URL(origin(server) + "/agent?view=agents").openConnection();
        c.setInstanceFollowRedirects(false); c.setConnectTimeout(15000); c.setReadTimeout(15000);
        c.setRequestProperty("Accept", "application/json");
        c.setRequestProperty("Authorization", "Bearer " + token);
        try {
            if (c.getResponseCode() != 200) throw new IOException("Could not load your agents. Check your connection or sign in again.");
            try (InputStream in = c.getInputStream(); ByteArrayOutputStream out = new ByteArrayOutputStream()) {
                byte[] buffer = new byte[8192]; int n;
                while ((n = in.read(buffer)) != -1) {
                    if (out.size() + n > 1024 * 1024) throw new IOException("Agent list too large.");
                    out.write(buffer, 0, n);
                }
                return new JSONObject(out.toString("UTF-8"));
            }
        } finally { c.disconnect(); }
    }

    static JSONObject post(String endpoint, String token, String body, String contentType) throws Exception {
        HttpURLConnection c = (HttpURLConnection) new URL(endpoint).openConnection();
        c.setInstanceFollowRedirects(false); // Never forward credentials or replay an action.
        c.setConnectTimeout(15000);
        c.setReadTimeout(180000);
        c.setRequestMethod("POST");
        c.setRequestProperty("Accept", "application/json");
        c.setRequestProperty("Content-Type", contentType);
        if (!token.isEmpty()) c.setRequestProperty("Authorization", "Bearer " + token);
        c.setDoOutput(true);
        try {
            try (OutputStream out = c.getOutputStream()) { out.write(body.getBytes(StandardCharsets.UTF_8)); }
            int code = c.getResponseCode();
            if (code == 401) throw new IOException("Your session has expired. Sign in again.");
            if (code == 402) throw new IOException("Your account needs credit. Open Account to manage your plan.");
            if (code == 403) throw new IOException("Access denied. Reconnect and allow Assistant API access and actions.");
            if (code < 200 || code >= 300) throw new IOException("Request failed (HTTP " + code + ").");
            try (InputStream in = c.getInputStream(); ByteArrayOutputStream out = new ByteArrayOutputStream()) {
                byte[] buffer = new byte[8192];
                int n;
                while ((n = in.read(buffer)) != -1) {
                    if (out.size() + n > 4 * 1024 * 1024) throw new IOException("Response too large.");
                    out.write(buffer, 0, n);
                }
                return new JSONObject(out.toString(StandardCharsets.UTF_8.name()));
            }
        } finally { c.disconnect(); }
    }
}
