package mu.client.android;

import org.json.JSONObject;
import org.json.JSONArray;
import java.util.UUID;
import com.google.protobuf.Message;
import com.google.protobuf.util.JsonFormat;
import mu.client.proto.assistant.v1.*;
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
        AskRequest q = AskRequest.newBuilder().setAgentId(agent(agent)).setThread(thread).setPrompt(prompt)
            .setVoice(voice).setMode(AccessMode.ACCESS_MODE_PERSONAL).build();
        JSONObject reply = call(server, token, "Ask", q, AskResponse.newBuilder());
        reply.put("results", results(reply.optJSONArray("content")));
        return reply;
    }

    static JSONObject askPublic(String server, String token, String prompt) throws Exception {
        if (!catalog(server, token).optBoolean("publicQuestions", false))
            throw new IOException("This server does not support locked-phone questions. Unlock to continue.");
        if (prompt.getBytes(StandardCharsets.UTF_8).length > 8000) throw new IOException("Keep the message below 8,000 bytes.");
        return call(server, token, "Ask", AskRequest.newBuilder().setPrompt(prompt)
            .setMode(AccessMode.ACCESS_MODE_PUBLIC).setVoice(true).build(), AskResponse.newBuilder());
    }

    static JSONObject catalog(String server, String token) throws Exception {
        return call(server, token, "ListAgents", ListAgentsRequest.getDefaultInstance(), ListAgentsResponse.newBuilder());
    }
    static JSONObject threads(String server, String token, String agent, String page) throws Exception {
        return call(server, token, "ListThreads", ListThreadsRequest.newBuilder().setAgentId(agent)
            .setPageToken(page).setPageSize(25).build(), ListThreadsResponse.newBuilder());
    }
    static JSONObject history(String server, String token, String thread, String page) throws Exception {
        return call(server, token, "ReadThread", ReadThreadRequest.newBuilder().setThread(thread)
            .setPageToken(page).setPageSize(100).build(), ReadThreadResponse.newBuilder());
    }
    private static JSONObject call(String server, String token, String operation, Message request, Message.Builder response) throws Exception {
        JSONObject raw = post(origin(server)+"/", token, JsonFormat.printer().print(request), "application/json", operation);
        // Ignore additive fields from a newer runtime; always retain content fallbacks.
        JsonFormat.parser().ignoringUnknownFields().merge(raw.toString(), response);
        return new JSONObject(JsonFormat.printer().includingDefaultValueFields().print(response.build()));
    }
    static JSONArray results(JSONArray content) throws Exception {
        JSONArray items = new JSONArray();
        if (content == null) return items;
        for (int i=0; i<content.length(); i++) {
            JSONObject block=content.getJSONObject(i), item;
            if (block.has("source")) {item=block.getJSONObject("source");item.put("kind","article");}
            else if (block.has("video") && block.getJSONObject("video").optString("provider").equals("youtube")) {item=block.getJSONObject("video");item.put("kind","video");}
            else if (block.has("image")) {item=block.getJSONObject("image");item.put("kind","image").put("title",item.optString("description"));}
            else if (block.has("resource")) {item=block.getJSONObject("resource");}
            else {item=new JSONObject().put("kind","text");}
            if (item.optString("title").isEmpty()) item.put("title",block.optString("fallback"));
            if (item.optString("body").isEmpty()) item.put("body",block.optString("fallback"));
            items.put(item);
        }
        return items;
    }

    static JSONObject post(String endpoint, String token, String body, String contentType) throws Exception {
        return post(endpoint, token, body, contentType, "");
    }
    private static JSONObject post(String endpoint, String token, String body, String contentType, String operation) throws Exception {
        HttpURLConnection c = (HttpURLConnection) new URL(endpoint).openConnection();
        c.setInstanceFollowRedirects(false); // Never forward credentials or replay an action.
        c.setConnectTimeout(15000);
        c.setReadTimeout(180000);
        c.setRequestMethod("POST");
        c.setRequestProperty("Accept", "application/json");
        c.setRequestProperty("Content-Type", contentType);
        if (!token.isEmpty()) c.setRequestProperty("Authorization", "Bearer " + token);
        String requestId = UUID.randomUUID().toString();
        if (!operation.isEmpty()) {
            c.setRequestProperty("Micro-Protocol", "mucp/1");
            c.setRequestProperty("Micro-Service", "assistant");
            c.setRequestProperty("Micro-Endpoint", "Assistant."+operation);
            c.setRequestProperty("Micro-Id", requestId);
        }
        c.setDoOutput(true);
        try {
            try (OutputStream out = c.getOutputStream()) { out.write(body.getBytes(StandardCharsets.UTF_8)); }
            int code = c.getResponseCode();
            if (code == 401) throw new IOException("Your session has expired. Sign in again.");
            if (code == 402) throw new IOException("Your account needs credit. Open Account to manage your plan.");
            if (code == 403) throw new IOException("Access denied. Reconnect and allow Assistant API access and actions.");
            if (!operation.isEmpty() && code == 504) throw new IOException("Your request was accepted and is still running. Open Chats to recover the reply.");
            if (!operation.isEmpty() && code >= 200 && code < 300 &&
                (!"mucp/1".equals(c.getHeaderField("Micro-Protocol")) || !requestId.equals(c.getHeaderField("Micro-Id"))))
                throw new IOException("This server does not support this assistant protocol. Update the runtime.");
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
