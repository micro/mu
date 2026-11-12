package mu.client.android;

import org.json.JSONObject;
import org.junit.Test;
import static org.junit.Assert.*;

public class ClientTest {
    @Test public void sharedWireFixturePreservesUnknownContentFallback() throws Exception {
        String raw;
        try (java.io.InputStream in=getClass().getResourceAsStream("/ask-response.json")) {
            assertNotNull(in); raw=new String(in.readAllBytes(), java.nio.charset.StandardCharsets.UTF_8);
        }
        mu.micro.client.proto.assistant.v1.AskResponse.Builder response=mu.micro.client.proto.assistant.v1.AskResponse.newBuilder();
        com.google.protobuf.util.JsonFormat.parser().ignoringUnknownFields().merge(raw,response);
        assertEquals("thread-example",response.getThread());
        JSONObject mapped=new JSONObject(com.google.protobuf.util.JsonFormat.printer().print(response.build()));
        org.json.JSONArray items=AgentApi.results(mapped.getJSONArray("content"));
        assertEquals("article",items.getJSONObject(0).getString("kind"));
        assertEquals("A future widget can still be read",items.getJSONObject(1).getString("title"));
        assertEquals("text",items.getJSONObject(1).getString("kind"));
    }

    @Test public void sharedExchangesMatchGeneratedContracts() throws Exception {
        org.json.JSONArray cases;
        try (java.io.InputStream in=getClass().getResourceAsStream("/exchanges.json")) {
            assertNotNull(in); cases=new org.json.JSONArray(new String(in.readAllBytes(),java.nio.charset.StandardCharsets.UTF_8));
        }
        for(int i=0;i<cases.length();i++) {
            JSONObject item=cases.getJSONObject(i);
            com.google.protobuf.Message.Builder request,response;
            switch(item.getString("operation")) {
            case "ListAgents":request=mu.micro.client.proto.assistant.v1.ListAgentsRequest.newBuilder();response=mu.micro.client.proto.assistant.v1.ListAgentsResponse.newBuilder();break;
            case "ListThreads":request=mu.micro.client.proto.assistant.v1.ListThreadsRequest.newBuilder();response=mu.micro.client.proto.assistant.v1.ListThreadsResponse.newBuilder();break;
            case "ReadThread":request=mu.micro.client.proto.assistant.v1.ReadThreadRequest.newBuilder();response=mu.micro.client.proto.assistant.v1.ReadThreadResponse.newBuilder();break;
            case "Ask":request=mu.micro.client.proto.assistant.v1.AskRequest.newBuilder();response=mu.micro.client.proto.assistant.v1.AskResponse.newBuilder();break;
            default:throw new AssertionError("Unknown operation");
            }
            com.google.protobuf.util.JsonFormat.parser().merge(item.getJSONObject("request").toString(),request);
            com.google.protobuf.util.JsonFormat.parser().ignoringUnknownFields().merge(item.getJSONObject("response").toString(),response);
            assertNotNull(request.build());assertNotNull(response.build());
        }
    }

    @Test public void compositionFixturesMatchGeneratedTypes() throws Exception {
        String[] names={"home","inbox","services","contacts","operation"};
        com.google.protobuf.Message.Builder[] builders={mu.micro.client.proto.assistant.v1.HomeResponse.newBuilder(),mu.micro.client.proto.assistant.v1.ListInboxResponse.newBuilder(),mu.micro.client.proto.assistant.v1.ListServicesResponse.newBuilder(),mu.micro.client.proto.service.contacts.v1.ListResponse.newBuilder(),mu.micro.client.proto.assistant.v1.Operation.newBuilder()};
        for(int i=0;i<names.length;i++)try(java.io.InputStream in=getClass().getResourceAsStream("/"+names[i]+"-response.json")){
            assertNotNull(in);com.google.protobuf.util.JsonFormat.parser().merge(new String(in.readAllBytes(),java.nio.charset.StandardCharsets.UTF_8),builders[i]);assertNotNull(builders[i].build());
        }
    }
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
