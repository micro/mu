package mu.client.android;

import com.google.protobuf.Message;
import com.google.protobuf.util.JsonFormat;
import mu.micro.client.proto.assistant.v1.*;
import org.json.JSONObject;
import java.io.IOException;

/** Customer facade shared with the agent's PWA. No creator credentials. */
final class AppApi {
    static String endpoint(String server, String route) throws IOException {
        if (!BuildConfig.BRANDED || !AgentApi.origin(server).equals(BuildConfig.FIXED_SERVER))
            throw new IOException("This app cannot connect to another server.");
        return BuildConfig.FIXED_SERVER + "/agent/" + BuildConfig.FIXED_AGENT + "/app/" + route;
    }
    static JSONObject call(String server, String connection, String route, Message request, Message.Builder response) throws Exception {
        String url = endpoint(server, route);
        if (AgentApi.vault == null) throw new IOException("Please sign in again.");
        String token = AgentApi.vault.accessToken(server, connection);
        JSONObject raw = request == null ? AgentApi.get(url, token) : AgentApi.post(url, token,
                JsonFormat.printer().preservingProtoFieldNames().print(request), "application/json");
        JsonFormat.parser().ignoringUnknownFields().merge(raw.toString(), response);
        return new JSONObject(JsonFormat.printer().includingDefaultValueFields().print(response.build()));
    }
    static JSONObject profile(String server, String token) throws Exception {
        return call(server, token, "profile", null, AppProfile.newBuilder());
    }
    static JSONObject ask(String server, String token, String thread, String prompt) throws Exception {
        // Resolve the credential before reading its session, so stale connections fail closed.
        endpoint(server, "chat");
        if (AgentApi.vault == null) throw new IOException("Please sign in again.");
        AgentApi.vault.accessToken(server, token);
        JSONObject account = AgentApi.vault.read();
        return call(server, token, "chat", AppPrompt.newBuilder().setPrompt(prompt).setThread(thread)
                .setSession(account.optString("paid_session")).build(), AppAnswer.newBuilder());
    }
    static JSONObject sessions(String server, String token) throws Exception {
        return call(server, token, "sessions", null, AppSessions.newBuilder());
    }
    static JSONObject start(String server, String token, String id) throws Exception {
        if (!id.matches("[a-zA-Z0-9_-]{1,128}")) throw new IOException("Invalid session.");
        return call(server, token, "session/" + id, PaidSessionRequest.getDefaultInstance(), PaidSession.newBuilder());
    }
}
