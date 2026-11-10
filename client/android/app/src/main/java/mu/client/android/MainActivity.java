package mu.client.android;

import android.Manifest;
import android.app.*;
import android.content.*;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Bundle;
import android.speech.tts.*;
import android.view.*;
import android.view.inputmethod.EditorInfo;
import android.widget.*;
import io.noties.markwon.Markwon;
import org.json.JSONObject;
import org.json.JSONArray;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class MainActivity extends Activity {
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private Vault vault;
    private JSONObject account = new JSONObject();
    private String thread = "", agent = "micro";
    private final java.util.Set<String> displayedMessages = new java.util.HashSet<>();
    private boolean busy, foreground, listening, ttsReady, destroyed, speaking;
    private LinearLayout messages;
    private ScrollView scroll;
    private EditText prompt;
    private TextView status;
    private Button send, speak, settings, newChat;
    private Switch wake, spoken;
    private VoiceInput voice;
    private int pendingVoice = -1; // -1 cancelled, 0 dictate, 1 wake
    private TextToSpeech tts;
    private Markwon markdown;
    private VideoPlayer video;
    private JSONArray results = new JSONArray();
    private int resultIndex = -1, pageIndex;
    private List<String> pages = new ArrayList<>();
    private String lastReply = "", utterance = "";
    private long speechEpoch;
    private boolean voiceRequest, controlsOnly;
    private JSONArray agents = new JSONArray();
    private boolean agentAvailable, agentLinkPending, launchHandled;
    private String agentName = "Micro", agentDescription = "A personal assistant";
    private Button agentPicker;
    private TextView description;

    @Override public void onCreate(Bundle saved) {
        super.onCreate(saved);
        launchHandled = saved != null && saved.getBoolean("launch_handled", false);
        setContentView(R.layout.main);
        View root = findViewById(R.id.root);
        int pad = (int) (16 * getResources().getDisplayMetrics().density);
        root.setOnApplyWindowInsetsListener((v, insets) -> {
            if (android.os.Build.VERSION.SDK_INT >= 30) {
                android.graphics.Insets bars = insets.getInsets(
                        WindowInsets.Type.systemBars() | WindowInsets.Type.ime());
                v.setPadding(pad + bars.left, pad + bars.top, pad + bars.right, pad + bars.bottom);
            } else {
                v.setPadding(pad + insets.getSystemWindowInsetLeft(), pad + insets.getSystemWindowInsetTop(),
                        pad + insets.getSystemWindowInsetRight(), pad + insets.getSystemWindowInsetBottom());
            }
            return insets;
        });
        vault = new Vault(this);
        try { account = vault.read(); }
        catch (Exception e) { vault.clear(); }
        agent = account.optString("agent", "micro");
        agentName = account.optString("agent_name", agent.equals("micro") ? "Micro" : agent);
        agentDescription = account.optString("agent_description", agent.equals("micro") ? "A personal assistant" : "");
        thread = account.optJSONObject("threads") == null ? "" : account.optJSONObject("threads").optString(agent);
        markdown = Markwon.create(this);
        video = new VideoPlayer(this, () -> { controlsOnly = true; startVoice(false); });
        messages = findViewById(R.id.messages);
        scroll = findViewById(R.id.scroll);
        prompt = findViewById(R.id.prompt);
        status = findViewById(R.id.status);
        send = findViewById(R.id.send); speak = findViewById(R.id.speak);
        agentPicker = findViewById(R.id.agent_picker);
        description = findViewById(R.id.agent_description);
        agentPicker.setOnClickListener(v -> { if (signedIn()) loadAgents(true); else connection(); });
        settings = findViewById(R.id.settings); newChat = findViewById(R.id.new_chat);
        wake = findViewById(R.id.wake); spoken = findViewById(R.id.spoken);
        send.setOnClickListener(v -> send());
        prompt.setOnEditorActionListener((v, action, event) -> {
            if (action == EditorInfo.IME_ACTION_SEND ||
                    (event != null && event.getKeyCode() == KeyEvent.KEYCODE_ENTER && event.getAction() == KeyEvent.ACTION_DOWN)) {
                send(); return true;
            }
            return false;
        });
        speak.setOnClickListener(v -> { if (listening || speaking) stopAudio(); else startVoice(false); });
        settings.setOnClickListener(v -> connection());
        newChat.setText("Chats");
        newChat.setOnClickListener(v -> { if (signedIn()) chooseChat(""); else connection(); });
        wake.setOnCheckedChangeListener((v, enabled) -> { if (enabled) startVoice(true); else stopAudio(); });
        spoken.setOnCheckedChangeListener((v, enabled) -> { if (!enabled) stopAudio(); });
        tts = new TextToSpeech(this, result -> {
            if (destroyed || result != TextToSpeech.SUCCESS) return;
            ttsReady = OfflineSpeech.select(tts);
        });
        tts.setOnUtteranceProgressListener(new UtteranceProgressListener() {
            public void onStart(String id) {}
            public void onDone(String id) { runOnUiThread(() -> {
                if (!id.equals(utterance) || destroyed) return;
                if (++pageIndex < pages.size()) speakPage();
                else { speaking = false; update(); resumeWake(); }
            }); }
            public void onError(String id) { runOnUiThread(() -> { if (id.equals(utterance)) { speaking = false; update(); resumeWake(); } }); }
        });
        status.setText(signedIn() ? "Type a message or tap Speak." : "Sign in to start talking.");
        if (saved != null && signedIn()) {
            if (agent.equals(saved.getString("agent", "micro"))) thread = saved.getString("thread", thread);

        }
        update();
        // Process launch data after creation. Keep the original intent stable;
        // remember consumption separately across lifecycle recreation.
        root.post(() -> {
            if (destroyed) return;
            callback(getIntent());
            if (signedIn() && !agentLinkPending) loadAgents(false);
        });
    }

    @Override public boolean dispatchTouchEvent(MotionEvent event) {
        if (event.getActionMasked() == MotionEvent.ACTION_DOWN && prompt != null && prompt.hasFocus()) {
            android.graphics.Rect bounds = new android.graphics.Rect();
            prompt.getGlobalVisibleRect(bounds);
            if (!bounds.contains((int) event.getRawX(), (int) event.getRawY())) dismissKeyboard();
        }
        return super.dispatchTouchEvent(event);
    }
    private void dismissKeyboard() {
        prompt.clearFocus();
        android.view.inputmethod.InputMethodManager keyboard =
                (android.view.inputmethod.InputMethodManager) getSystemService(INPUT_METHOD_SERVICE);
        if (keyboard != null) keyboard.hideSoftInputFromWindow(prompt.getWindowToken(), 0);
    }

    private boolean signedIn() {
        return !account.optString("token").isEmpty() && account.optLong("expires") > System.currentTimeMillis();
    }
    private void update() {
        if (destroyed) return;
        if (agentPicker != null) {
            agentPicker.setText(agentName); agentPicker.setEnabled(!busy);
            agentPicker.setContentDescription("Change agent. Current agent: " + agentName);
            description.setText(agentDescription);
            prompt.setHint("Message " + agentName);
        }
        if (foreground && speaking) getWindow().addFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        else getWindow().clearFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        send.setEnabled(!busy && signedIn() && agentAvailable);
        speak.setEnabled(!busy && signedIn() && agentAvailable);
        settings.setText(signedIn() ? "Account" : "Sign in");
        settings.setEnabled(!busy); newChat.setEnabled(!busy);
        wake.setEnabled(!busy && signedIn() && agentAvailable);
        wake.setText(getString(R.string.wake, agentName));
        speak.setText(listening || speaking ? R.string.stop : R.string.speak);
    }
    private void message(String who, String text) {
        TextView view = new TextView(this);
        view.setTextSize(16); view.setTextIsSelectable(true);
        int gap = (int) (12 * getResources().getDisplayMetrics().density);
        view.setPadding(0, gap, 0, gap);
        LinearLayout.LayoutParams spacing = new LinearLayout.LayoutParams(-1, -2);
        spacing.bottomMargin = gap;
        view.setLayoutParams(spacing);
        markdown.setMarkdown(view, "**" + who + "**\n\n" + text);
        messages.addView(view);
        scroll.post(() -> scroll.fullScroll(View.FOCUS_DOWN));
    }
    private void send() {
        if (busy || !signedIn()) { if (!signedIn()) connection(); return; }
        if (!agentAvailable) { status.setText("Choose an available agent first."); return; }
        String text = prompt.getText().toString().trim();
        if (text.isEmpty()) return;
        dismissKeyboard();
        boolean spokenRequest = voiceRequest;
        voiceRequest = false;
        if (localCommand(text)) { prompt.setText(""); resumeWake(); return; }
        stopAudio(); video.pause();
        busy = true; update(); prompt.setText("");
        message("You", text); status.setText(agentName + " is working…");
        final String server = account.optString("server"), token = account.optString("token");
        final String currentThread = thread, currentAgent = agent;
        worker.execute(() -> {
            try {
                JSONObject reply = AgentApi.ask(server, token, currentAgent, currentThread, text, spokenRequest);
                String response = reply.getString("text");
                String next = reply.getString("thread");
                runOnUiThread(() -> {
                    if (destroyed) return;
                    thread = next; busy = false;
                    lastReply = response; results = reply.optJSONArray("results");
                    if (results == null) results = new JSONArray();
                    resultIndex = -1;
                    persistSelection(); message(agentName, response); showResults(); status.setText(""); update();
                    boolean played = false;
                    if (foreground && VoiceCommands.wantsVideo(text)) {
                        for (int i = 0; i < results.length(); i++) {
                            JSONObject item = results.optJSONObject(i);
                            if (item != null && item.optString("kind").equals("video")
                                    && VoiceCommands.validVideo(item.optString("id"))) {
                                openResult(i, true); played = true; break;
                            }
                        }
                    }
                    if (!played && foreground && spoken.isChecked()) readAloud(response);
                    else resumeWake();
                });
            } catch (Exception e) {
                runOnUiThread(() -> {
                    if (destroyed) return;
                    busy = false; update();
                    status.setText(e.getMessage() + " Nothing was retried. Open Chats to check for your reply before sending again.");
                });
            }
        });
    }
    private void startVoice(boolean wakeOnly) {
        if (!foreground || busy || !signedIn() || !agentAvailable) return;
        pendingVoice = wakeOnly ? 1 : 0;
        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.RECORD_AUDIO}, 1); return;
        }
        if (!wakeOnly) {
            if (tts != null) tts.stop();
            utterance = ""; speaking = false; video.pause();
        }
        if (voice == null) {
            status.setText("Preparing offline voice…");
            voice = new VoiceInput(getApplicationContext(), new VoiceInput.Listener() {
                public void ready() {
                    if (foreground && pendingVoice >= 0) startVoice(pendingVoice == 1);
                }
                public void detected() {
                    controlsOnly = speaking || video.playing();
                    interruptReply();
                    speak.performHapticFeedback(android.view.HapticFeedbackConstants.VIRTUAL_KEY);
                    status.setText("Heard ‘Hey " + agentName + "’. Listening…");
                }
                public void partial(String text) {
                    status.setText(text.isEmpty() ? "Listening… Say your request." : "Hearing: “" + text + "”");
                }
                public void wake(String command) {
                    controlsOnly = controlsOnly || speaking || video.playing();
                    if (command.isEmpty()) { controlsOnly = false; startVoice(false); }
                    else handleVoice(command);
                }
                public void heard(String text) {
                    handleVoice(text);
                }
                public void error(String text) { listening = false; update(); status.setText(text); }
            });
            return;
        }
        if (!voice.ready()) { status.setText("Preparing offline voice…"); return; }
        pendingVoice = -1;
        listening = true; update();
        status.setText(wakeOnly ? "Listening locally for “Hey " + agentName + "”…" : "Listening… Pause when finished.");
        voice.listen(wakeOnly, wakeNames());
    }
    // Keep recognition alive so the rest of the request can still be captured.
    void interruptReply() {
        utterance = ""; speechEpoch++;
        if (tts != null) tts.stop();
        speaking = false; video.pause(); update();
    }
    private String[] wakeNames() {
        JSONObject aliases = account.optJSONObject("wake_aliases");
        return VoiceCommands.names(agentName, aliases == null ? "" : aliases.optString(agent));
    }
    private void wakeAliases() {
        LinearLayout form = new LinearLayout(this); form.setOrientation(LinearLayout.VERTICAL);
        int pad = getResources().getDimensionPixelSize(R.dimen.control_gap);
        form.setPadding(pad * 2, pad, pad * 2, pad);
        TextView label = new TextView(this);
        label.setText("Other spellings of " + agentName + " heard by voice recognition (comma separated)");
        EditText input = new EditText(this); input.setSingleLine(true); input.setHint("e.g. molten, maltin");
        JSONObject aliases = account.optJSONObject("wake_aliases");
        input.setText(aliases == null ? "" : aliases.optString(agent));
        input.setFilters(new android.text.InputFilter[]{new android.text.InputFilter.LengthFilter(160)});
        form.addView(label); form.addView(input);
        new AlertDialog.Builder(this).setTitle("Wake aliases for " + agentName).setView(form)
            .setPositiveButton("Save", (d, w) -> {
                try {
                    JSONObject saved = account.optJSONObject("wake_aliases");
                    if (saved == null) saved = new JSONObject();
                    saved.put(agent, input.getText().toString().trim()); account.put("wake_aliases", saved);
                    vault.write(account); stopAudio(); resumeWake();
                } catch (Exception e) { status.setText("Could not save wake aliases."); }
            }).setNegativeButton("Cancel", null).show();
    }
    private void resumeWake() {
        if (foreground && !busy && wake.isChecked() && prompt.length() == 0) startVoice(true);
    }
    private void stopAudio() {
        pendingVoice = -1; utterance = ""; speechEpoch++;
        if (voice != null) voice.stop();
        if (tts != null) tts.stop();
        listening = false; speaking = false; update();
    }
    @Override public void onRequestPermissionsResult(int request, String[] permissions, int[] results) {
        super.onRequestPermissionsResult(request, permissions, results);
        if (request == 1 && results.length > 0 && results[0] == PackageManager.PERMISSION_GRANTED) {
            if (pendingVoice >= 0) startVoice(pendingVoice == 1);
            else status.setText("Microphone enabled. Tap Speak when ready.");
        }
        else { wake.setChecked(false); status.setText("Microphone permission denied. You can still type."); }
    }
    private void handleVoice(String text) {
        listening = false; update();
        boolean restricted = controlsOnly; controlsOnly = false;
        if ((!restricted || VoiceCommands.playbackControl(text)) && localCommand(text)) { resumeWake(); return; }
        prompt.setText(text); prompt.setSelection(text.length()); voiceRequest = true;
        if (wake.isChecked() && !restricted) { voiceRequest = true; send(); }
        else status.setText(restricted ? "Playback cannot issue account requests. Check the message and tap Send." : "Check your message, then tap Send.");
    }
    boolean localCommand(String text) {
        switch (VoiceCommands.normalize(text)) {
            case "stop listening":
                if (!wake.isChecked() && !listening && pendingVoice < 0) return false;
                wake.setChecked(false); stopAudio(); status.setText("Listening stopped."); return true;
            case "stop": case "pause":
                if (!speaking && !listening && !video.active() && pageIndex >= pages.size()) return false;
                stopAudio(); video.pause(); status.setText("Paused."); resumeWake(); return true;
            case "resume": case "continue":
                if (video.active()) { video.resumeView(); video.command("play"); }
                else if (pageIndex < pages.size()) speakPage();
                else return false;
                resumeWake(); return true;
            case "back": case "close video":
                if (!video.active()) return false;
                video.close(); status.setText("Back to chat."); resumeWake(); return true;
            case "next": case "next headline":
                if (resultIndex + 1 < results.length()) openResult(++resultIndex, true);
                else if (pageIndex + 1 < pages.size()) { pageIndex++; speakPage(); }
                else return false;
                return true;
            case "previous": case "previous headline":
                if (resultIndex > 0) openResult(--resultIndex, true);
                else if (pageIndex > 0 && !pages.isEmpty()) { pageIndex--; speakPage(); }
                else return false;
                return true;
            case "play first": case "open first": case "read first":
                if (results.length() == 0) return false;
                openResult(0, true); return true;
            case "read that": case "read aloud": case "read it":
                if (lastReply.isEmpty()) return false;
                readAloud(lastReply); return true;
            case "audio only":
                if (video.active()) {
                    String explanation = "YouTube requires a visible player. Audio-only is unavailable for this source.";
                    video.notice(explanation); readAloud(explanation);
                }
                else if (!lastReply.isEmpty()) readAloud(lastReply);
                else return false;
                return true;
            case "show video":
                if (resultIndex < 0 || results.optJSONObject(resultIndex) == null ||
                        !results.optJSONObject(resultIndex).optString("kind").equals("video")) return false;
                openResult(resultIndex, false); return true;
            case "show text":
                if (!speaking && !video.active() && pageIndex >= pages.size()) return false;
                stopAudio(); video.close(); resumeWake(); return true;
            default: return false;
        }
    }
    private void showResults() {
        final JSONArray shownResults = results;
        boolean embedded = false;
        for (int i = 0; i < shownResults.length(); i++) {
            JSONObject item = shownResults.optJSONObject(i);
            if (item == null || item.optString("title").isEmpty()) continue;
            if (!embedded && item.optString("kind").equals("video") && VoiceCommands.validVideo(item.optString("id"))) {
                video.show(item.optString("id"), item.optString("title"), false);
                resultIndex = i; embedded = true;
            }
            final int index = i;
            Button button = new Button(this, null, 0, R.style.CompactButton);
            button.setText((i + 1) + ". " + item.optString("title"));
            button.setOnClickListener(v -> { results = shownResults; openResult(index, true); });
            LinearLayout.LayoutParams spacing = new LinearLayout.LayoutParams(-1, -2);
            spacing.bottomMargin = getResources().getDimensionPixelSize(R.dimen.control_gap);
            messages.addView(button, spacing);
        }
    }
    private void openResult(int index, boolean play) {
        JSONObject item = results.optJSONObject(index);
        if (item == null) return;
        resultIndex = index;
        if (item.optString("kind").equals("video")) {
            stopAudio(); video.show(item.optString("id"), item.optString("title"), play); resumeWake();
        } else {
            video.close();
            String text = item.optString("body", item.optString("summary"));
            if (text.isEmpty()) text = "Ask " + agentName + " to read this item for more detail.";
            readAloud(item.optString("title") + ". " + text);
        }
    }
    private void readAloud(String text) {
        if (text.isEmpty()) return;
        stopAudio(); video.pause();
        if (!ttsReady) { status.setText("Install an offline English voice in Android settings for spoken replies."); resumeWake(); return; }
        pages = SpeechPages.split(markdown.toMarkdown(text).toString(), Math.min(2500, TextToSpeech.getMaxSpeechInputLength()));
        pageIndex = 0; speakPage();
    }
    private void speakPage() {
        if (!foreground || !ttsReady || pageIndex < 0 || pageIndex >= pages.size()) return;
        speechEpoch++; utterance = "reply-" + speechEpoch; speaking = true; update();
        if (tts.speak(pages.get(pageIndex), TextToSpeech.QUEUE_FLUSH, null, utterance) == TextToSpeech.ERROR) {
            speaking = false; update();
        }
        resumeWake();
    }
    private void persistSelection() {
        try {
            JSONObject threads = account.optJSONObject("threads");
            if (threads == null) threads = new JSONObject();
            threads.put(agent, thread);
            account.put("threads", threads).put("agent", agent).put("agent_name", agentName).put("agent_description", agentDescription);
            vault.write(account);
        } catch (Exception e) { status.setText("Could not save your selected agent on this device."); }
    }
    void selectAgent(String slug, String name, String about) {
        if (busy) return;
        persistSelection(); stopAudio(); video.close(); wake.setChecked(false);
        agent = AgentApi.agent(slug); agentAvailable = true; agentName = name; agentDescription = about;
        JSONObject threads = account.optJSONObject("threads");
        thread = threads == null ? "" : threads.optString(agent);
        messages.removeAllViews(); prompt.setText(""); results = new JSONArray(); pages.clear(); lastReply = "";
        persistSelection(); update();
        status.setText("Start talking: type a message or tap Speak.");
        if (signedIn() && !thread.isEmpty()) restoreChat("", true);
    }
    private void startChat() {
        stopAudio(); video.close(); results = new JSONArray(); lastReply = ""; pages.clear();
        thread = ""; persistSelection(); messages.removeAllViews(); prompt.setText("");
        status.setText("New conversation with " + agentName); update();
    }
    private void chooseChat(String page) {
        if (busy) return;
        dismissKeyboard(); stopAudio(); busy=true; update();
        final String server=account.optString("server"), token=account.optString("token"), selected=agent;
        worker.execute(() -> {
            try {
                JSONObject response=AgentApi.threads(server,token,selected,page);
                JSONArray chats=response.getJSONArray("threads");
                runOnUiThread(() -> {
                    if (destroyed) return;
                    busy=false; update();
                    String[] labels=new String[chats.length()];
                    for(int i=0;i<chats.length();i++) labels[i]=chats.optJSONObject(i).optString("subject", "Chat");
                    AlertDialog.Builder dialog=new AlertDialog.Builder(this).setTitle(agentName+" chats")
                        .setItems(labels,(d,index)->{
                            thread=chats.optJSONObject(index).optString("id");persistSelection();restoreChat("",true);
                        }).setPositiveButton("New chat",(d,w)->startChat()).setNegativeButton("Cancel",null);
                    if (!response.optString("nextPageToken").isEmpty())
                        dialog.setNeutralButton("More",(d,w)->chooseChat(response.optString("nextPageToken")));
                    dialog.show();
                });
            } catch(Exception e) {runOnUiThread(()->{if(!destroyed){busy=false;update();status.setText(e.getMessage());}});}
        });
    }
    private void restoreChat(String page, boolean clear) {
        if (busy || thread.isEmpty()) return;
        stopAudio(); busy=true; update();status.setText("Loading chat…");
        final String server=account.optString("server"),token=account.optString("token"),target=thread,selected=agent;
        worker.execute(()->{
            try {
                JSONObject response=AgentApi.history(server,token,target,page);
                if(!response.getJSONObject("thread").optString("agentId").equals(selected)) throw new java.io.IOException("This chat belongs to a different agent. Select it from Chats.");
                runOnUiThread(()->{
                    if (destroyed) return;
                    busy=false;
                    if(!thread.equals(target)){update();return;}
                    if(clear){video.close();messages.removeAllViews();displayedMessages.clear();results=new JSONArray();lastReply="";}
                    JSONArray history=response.optJSONArray("messages");
                    try {
                        for(int i=0;history!=null && i<history.length();i++){
                            JSONObject m=history.getJSONObject(i);
                            if(!displayedMessages.add(m.getString("id"))) continue;
                            boolean assistant=m.optString("role").equals("ROLE_AGENT");
                            message(assistant?agentName:"You",m.optString("text"));
                            if(assistant){lastReply=m.optString("text");results=AgentApi.results(m.optJSONArray("content"));showResults();}
                        }
                    } catch(Exception e){status.setText("Some chat content could not be displayed.");}
                    String next=response.optString("nextPageToken");
                    if(!next.isEmpty()){
                        Button more=new Button(this,null,0,R.style.CompactButton);more.setText("Load more messages");
                        more.setOnClickListener(v->{messages.removeView(more);restoreChat(next,false);});messages.addView(more);
                    }
                    boolean pending=response.optBoolean("pending");
                    status.setText(pending?"Working… Your reply will appear here.":"");update();
                    if(pending && next.isEmpty()) scroll.postDelayed(()->{
                        if(!destroyed && foreground && !busy && thread.equals(target)) restoreChat(page,false);
                    },3000);
                });
            }catch(Exception e){runOnUiThread(()->{if(!destroyed){busy=false;update();status.setText(e.getMessage()+" Open Chats to try again.");}});}
        });
    }
    private void loadAgents(boolean choose) {
        if (!signedIn() || busy) return;
        busy = true; update();
        String server = account.optString("server"), token = account.optString("token");
        worker.execute(() -> {
            try {
                JSONArray available = AgentApi.catalog(server, token).getJSONArray("agents");
                runOnUiThread(() -> {
                    if (destroyed) return;
                    busy = false; agents = available;
                    boolean found = false;
                    for (int i = 0; i < agents.length(); i++) {
                        JSONObject item = agents.optJSONObject(i);
                        if (item != null && (agent.equals(item.optString("slug")) || agent.equals(item.optString("id")))) {
                            String previous = agent;
                            agent = item.optString("id", item.optString("slug"));
                            if (!previous.equals(agent) && account.optJSONObject("threads") != null && thread.isEmpty()) thread = account.optJSONObject("threads").optString(agent);
                            agentName = item.optString("name", agent); agentDescription = item.optString("description"); found = true; break;
                        }
                    }
                    agentAvailable = found;
                    if (found) persistSelection();
                    else status.setText("This agent is not available to this account. Tap its name to choose another.");
                    update();
                    if (found && !choose && !thread.isEmpty()) restoreChat("", true);
                    if (choose) {
                        String[] labels = new String[agents.length()];
                        for (int i = 0; i < agents.length(); i++) {
                            JSONObject item = agents.optJSONObject(i);
                            labels[i] = item.optString("name") + (item.optString("description").isEmpty() ? "" : "\n" + item.optString("description"));
                        }
                        new AlertDialog.Builder(this).setTitle("Choose an agent").setItems(labels, (d, index) -> {
                            JSONObject item = agents.optJSONObject(index);
                            selectAgent(item.optString("id", item.optString("slug")), item.optString("name"), item.optString("description"));
                        }).setNeutralButton("Create agent", (d, w) -> startActivity(new Intent(Intent.ACTION_VIEW,
                            Uri.parse(AgentApi.origin(account.optString("server")) + "/agent/new"))))
                            .setNegativeButton("Cancel", null).show();
                    }
                });
            } catch (Exception e) { runOnUiThread(() -> { if (!destroyed) { busy = false; update(); status.setText(e.getMessage()); } }); }
        });
    }

    private void overview(String view, String page) {
        if (busy || !signedIn()) return;
        dismissKeyboard(); stopAudio(); busy=true; update();
        final String server=account.optString("server"), token=account.optString("token");
        worker.execute(() -> {
            try {
                JSONObject response=view.equals("home")?AgentApi.home(server,token):view.equals("inbox")?AgentApi.inbox(server,token,page):AgentApi.services(server,token,page);
                runOnUiThread(() -> {
                    if(destroyed) return;
                    busy=false;update();
                    AlertDialog.Builder dialog=new AlertDialog.Builder(this).setTitle(view.equals("home")?"Home":view.equals("inbox")?"Inbox":"Services").setNegativeButton("Close",null);
                    if(view.equals("home")) {
                        JSONArray events=response.optJSONArray("events");
                        StringBuilder summary=new StringBuilder();
                        if(events!=null) for(int i=0;i<events.length();i++){JSONObject e=events.optJSONObject(i);summary.append(e.optString("title")).append(" · ").append(e.optString("start")).append("\n");}
                        if(summary.length()==0) summary.append("No cached events.\n");
                        if(response.optBoolean("calendarPending")) summary.append("Calendar updates are pending.\n");
                        summary.append("\n").append(response.optString("brief"));
                        dialog.setMessage(summary.toString()).setPositiveButton("Inbox",(d,w)->overview("inbox","")).setNeutralButton("Services",(d,w)->overview("services",""));
                    } else {
                        JSONArray rows=response.optJSONArray(view.equals("inbox")?"items":"services");
                        if(rows==null) rows=new JSONArray();
                        final JSONArray entries=rows;
                        String[] labels=new String[entries.length()];
                        for(int i=0;i<labels.length;i++){JSONObject item=entries.optJSONObject(i);labels[i]=view.equals("inbox")?item.optString("subject","Untitled")+(item.optBoolean("unread")?" · Unread":""):item.optString("name")+" — "+item.optString("description");}
                        if(labels.length==0)dialog.setMessage("Nothing here yet.");
                        else dialog.setItems(labels,(d,index)->{JSONObject item=entries.optJSONObject(index);if(view.equals("inbox"))readInbox(item.optString("id"),"");else new AlertDialog.Builder(this).setTitle(item.optString("name")).setMessage(item.optString("description")).setPositiveButton("Close",null).show();});
                        if(!response.optString("nextPageToken").isEmpty())dialog.setNeutralButton("More",(d,w)->overview(view,response.optString("nextPageToken")));
                    }
                    dialog.show();
                });
            } catch(Exception e){runOnUiThread(()->{if(!destroyed){busy=false;update();status.setText(e.getMessage());}});}
        });
    }
    private void readInbox(String id,String page) {
        if(busy)return;
        busy=true;update();final String server=account.optString("server"),token=account.optString("token");
        worker.execute(()->{
            try{
                JSONObject response=AgentApi.readInbox(server,token,id,page),item=response.getJSONObject("item");
                StringBuilder text=new StringBuilder();JSONArray entries=response.getJSONArray("messages");
                for(int i=0;i<entries.length();i++)text.append(entries.getJSONObject(i).optString("text")).append("\n\n");
                runOnUiThread(()->{
                    if(destroyed)return;busy=false;update();
                    AlertDialog.Builder dialog=new AlertDialog.Builder(this).setTitle(item.optString("subject","Inbox")).setMessage(text.toString()).setNegativeButton("Close",null)
                        .setPositiveButton(item.optBoolean("unread")?"Mark read":"Mark unread",(d,w)->{
                            busy=true;update();worker.execute(()->{try{AgentApi.markInbox(server,token,id,!item.optBoolean("unread"));runOnUiThread(()->{if(!destroyed){busy=false;update();overview("inbox","");}});}catch(Exception e){runOnUiThread(()->{if(!destroyed){busy=false;update();status.setText(e.getMessage());}});}});
                        });
                    if(!response.optString("nextPageToken").isEmpty())dialog.setNeutralButton("More",(d,w)->readInbox(id,response.optString("nextPageToken")));
                    dialog.show();
                });
            }catch(Exception e){runOnUiThread(()->{if(!destroyed){busy=false;update();status.setText(e.getMessage());}});}
        });
    }
    private Uri agentLink() {
        return new Uri.Builder().scheme("micro-agent").authority("open").appendQueryParameter("server", account.optString("server", "https://micro.mu")).appendQueryParameter("agent", agent).build();
    }
    private void connection() {
        if (busy) return;
        stopAudio();
        AlertDialog.Builder dialog = new AlertDialog.Builder(this).setTitle(signedIn() ? "Account" : "Start talking to " + agentName);
        if (!signedIn()) {
            dialog.setMessage(agentDescription + "\n\nSign in with your existing account, or create one in your browser.")
                .setPositiveButton("Sign in", (d, w) -> beginSignIn(account.optString("server", "https://micro.mu")))
                .setNeutralButton("Settings", (d, w) -> advancedConnection()).setNegativeButton("Cancel", null).show();
            return;
        }
        dialog.setItems(new String[]{"Choose an agent", "Add agent to home screen", "Share agent link", "Open account", "Settings", "Sign out", "Use Mu as phone assistant", "Wake aliases", "Home", "Inbox", "Services"}, (d, index) -> {
            switch (index) {
                case 8: overview("home", ""); break;
                case 9: overview("inbox", ""); break;
                case 10: overview("services", ""); break;
                case 7: wakeAliases(); break;
                case 6:
                    if (android.os.Build.VERSION.SDK_INT >= 29) {
                        android.app.role.RoleManager roles = getSystemService(android.app.role.RoleManager.class);
                        if (roles != null && roles.isRoleAvailable(android.app.role.RoleManager.ROLE_ASSISTANT) && !roles.isRoleHeld(android.app.role.RoleManager.ROLE_ASSISTANT)) {
                            startActivityForResult(roles.createRequestRoleIntent(android.app.role.RoleManager.ROLE_ASSISTANT), 7); break;
                        }
                    }
                    Intent assistantSettings = new Intent(android.provider.Settings.ACTION_VOICE_INPUT_SETTINGS);
                    if (assistantSettings.resolveActivity(getPackageManager()) != null) startActivity(assistantSettings);
                    else status.setText("Open Android Settings → Default apps → Digital assistant and select Mu.");
                    break;
                case 0: loadAgents(true); break;
                case 1:
                    if (!agentAvailable) { status.setText("Choose an available agent first."); break; }
                    android.content.pm.ShortcutManager shortcuts = getSystemService(android.content.pm.ShortcutManager.class);
                    if (shortcuts != null && shortcuts.isRequestPinShortcutSupported()) {
                        Intent open = new Intent(this, MainActivity.class).setAction(Intent.ACTION_VIEW).setData(agentLink());
                        android.content.pm.ShortcutInfo shortcut = new android.content.pm.ShortcutInfo.Builder(this, "agent-" + account.optString("server").hashCode() + "-" + agent)
                            .setShortLabel(agentName).setIntent(open).build();
                        shortcuts.requestPinShortcut(shortcut, null);
                    } else status.setText("This launcher does not support pinned shortcuts.");
                    break;
                case 2:
                    if (!agentAvailable) { status.setText("Choose an available agent first."); break; }
                    Intent share = new Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, agentLink().toString());
                    startActivity(Intent.createChooser(share, "Share agent")); break;
                case 3: startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(AgentApi.origin(account.optString("server")) + "/account"))); break;
                case 4: advancedConnection(); break;
                case 5: signOut(); break;
            }
        }).setNegativeButton("Close", null).show();
    }
    private boolean openAgentLink(Intent intent) {
        Uri uri = intent.getData();
        if (uri == null || !"micro-agent".equals(uri.getScheme())) return false;
        launchHandled = true;
        if (busy) { status.setText("Finish the current request before opening another agent."); return true; }
        try {
            if (!"open".equals(uri.getHost()) || uri.getUserInfo() != null || uri.getPort() != -1 || uri.getFragment() != null || (uri.getPath() != null && !uri.getPath().isEmpty())) throw new IllegalArgumentException();
            String server = AgentApi.origin(uri.getQueryParameter("server"));
            String slug = AgentApi.agent(uri.getQueryParameter("agent"));

            Runnable open = () -> {
                agentLinkPending = false;
                if (!server.equals(account.optString("server", "https://micro.mu"))) {
                    signOut(); account = new JSONObject();
                }
                try { account.put("server", server); } catch (Exception ignored) {}
                selectAgent(slug, slug.equals("micro") ? "Micro" : "Agent", "Sign in to load this agent’s details.");
                agentAvailable = false; update();
                if (signedIn()) loadAgents(false); else connection();
            };
            if (!server.equals(account.optString("server", "https://micro.mu"))) {
                agentLinkPending = true;
                new AlertDialog.Builder(this).setTitle("Change server?").setMessage("Open this agent on " + server + "? You will need to sign in there. Your current account credentials will not be sent.")
                    .setPositiveButton("Continue", (d,w) -> open.run()).setNegativeButton("Cancel", (d,w) -> { agentLinkPending = false; if (signedIn()) loadAgents(false); }).setOnCancelListener(d -> { agentLinkPending = false; if (signedIn()) loadAgents(false); }).show();
            } else open.run();
        } catch (Exception e) { status.setText("That agent link is invalid."); }
        return true;
    }
    private void advancedConnection() {
        if (busy) return;
        stopAudio();
        LinearLayout form = new LinearLayout(this); form.setOrientation(LinearLayout.VERTICAL);
        int pad = (int) (20 * getResources().getDisplayMetrics().density); form.setPadding(pad, pad, pad, pad);
        EditText server = new EditText(this); server.setSingleLine(true); server.setHint("Server");
        server.setId(View.generateViewId());
        TextView serverLabel = new TextView(this); serverLabel.setText("Server address"); serverLabel.setLabelFor(server.getId());
        form.addView(serverLabel);
        server.setText(account.optString("server", "https://micro.mu")); form.addView(server);
        EditText name = new EditText(this); name.setSingleLine(true); name.setHint("Agent"); name.setText(agent);
        name.setId(View.generateViewId());
        TextView agentLabel = new TextView(this); agentLabel.setText("Agent name"); agentLabel.setLabelFor(name.getId());
        agentLabel.setPadding(0, pad / 2, 0, 0); form.addView(agentLabel); form.addView(name);
        TextView info = new TextView(this);
        info.setText("Sign in or create an account in your browser. Your existing plan and credits apply. Voice is English and runs on this device.");
        form.addView(info);
        Button manage = new Button(this); manage.setText("Open account"); form.addView(manage);
        manage.setOnClickListener(v -> {
            try { startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(AgentApi.origin(server.getText().toString()) + "/account"))); }
            catch (Exception e) { status.setText("Enter a valid HTTPS server address."); }
        });
        AlertDialog dialog = new AlertDialog.Builder(this).setTitle("Mu account").setView(form)
                .setPositiveButton("Sign in", null).setNegativeButton("Cancel", null)
                .setNeutralButton("Sign out", (d, w) -> signOut()).create();
        dialog.setOnShowListener(d -> dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener(v -> {
            server.setError(null); name.setError(null);
            String origin, selected;
            try { origin = AgentApi.origin(server.getText().toString()); }
            catch (IllegalArgumentException e) { server.setError(e.getMessage()); return; }
            try { selected = AgentApi.agent(name.getText().toString().trim()); }
            catch (IllegalArgumentException e) { name.setError(e.getMessage()); return; }
            dialog.dismiss(); signOut(); agent = selected; agentName = selected; agentDescription = ""; beginSignIn(origin);
        }));
        dialog.show();
    }
    private void signOut() {
        stopAudio(); video.close(); results = new JSONArray(); pages.clear(); lastReply = ""; wake.setChecked(false); vault.clear(); account = new JSONObject();
        thread = ""; messages.removeAllViews(); prompt.setText(""); update();
        status.setText("Signed out on this device. You can revoke access in Account → Tokens.");
    }
    private void beginSignIn(String server) {
        agentAvailable = false;
        busy = true; update(); status.setText("Opening sign-in…");
        worker.execute(() -> {
            try {
                JSONObject pending = OAuth.begin(server, BuildConfig.OAUTH_REDIRECT);
                JSONObject stored = new JSONObject().put("pending", pending).put("server", server).put("agent", agent).put("agent_name", agentName).put("agent_description", agentDescription);
                String url = OAuth.authorize(pending);
                runOnUiThread(() -> {
                    if (destroyed) return;
                    try { vault.write(stored); }
                    catch (Exception e) { busy = false; update(); status.setText("Could not save sign-in securely."); return; }
                    account = stored; busy = false; update();
                    try { startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(url))); status.setText("Complete sign-in in your browser."); }
                    catch (Exception e) { status.setText("No browser available to sign in."); }
                });
            } catch (Exception e) {
                runOnUiThread(() -> { if (!destroyed) { busy = false; update(); status.setText("Could not start sign-in. Check the server and try again."); } });
            }
        });
    }
    @Override protected void onNewIntent(Intent intent) { super.onNewIntent(intent); setIntent(intent); launchHandled = false; callback(intent); }
    private void callback(Intent intent) {
        if (launchHandled) return;
        if (openAgentLink(intent)) return;
        Uri uri = intent.getData();
        if (uri == null || busy) return;
        Uri expected = Uri.parse(BuildConfig.OAUTH_REDIRECT);
        if (!expected.equals(uri.buildUpon().clearQuery().fragment(null).build()) || uri.getFragment() != null) return;
        launchHandled = true;
        JSONObject pending = account.optJSONObject("pending");
        String code = uri.getQueryParameter("code");
        if (pending == null || code == null || !OAuth.validState(pending, uri.getQueryParameter("state"), System.currentTimeMillis())) {
            status.setText("Sign-in expired or did not match. Please sign in again."); return;
        }
        busy = true; update();
        vault.clear();
        worker.execute(() -> {
            try {
                // Callback is single-use even if the process stops during exchange.
                JSONObject token = OAuth.exchange(pending, code);
                JSONObject connected = new JSONObject().put("server", pending.getString("server"))
                        .put("token", token.getString("access_token"))
                        .put("expires", System.currentTimeMillis() + token.getLong("expires_in") * 1000);
                runOnUiThread(() -> {
                    if (destroyed) return;
                    try { connected.put("agent", agent); vault.write(connected); }
                    catch (Exception e) { busy = false; update(); status.setText("Could not save sign-in securely. Please reconnect."); return; }
                    account = connected; thread = ""; messages.removeAllViews(); busy = false; update();
                    status.setText("Type a message or tap Speak."); loadAgents(false);
                });
            } catch (Exception e) {
                runOnUiThread(() -> { if (!destroyed) { busy = false; account = new JSONObject(); update(); status.setText("Sign-in failed. Please try again."); } });
            }
        });
    }
    @Override protected void onResume() { super.onResume(); foreground = true; if (video != null) video.resumeView(); update(); }
    @Override protected void onPause() {
        foreground = false; stopAudio(); if (video != null) video.pause();
        if (wake != null) wake.setChecked(false); // Never silently resume ambient listening.
        super.onPause();
    }
    @Override protected void onSaveInstanceState(Bundle out) {
        out.putBoolean("launch_handled", launchHandled);
        out.putString("thread", thread); out.putString("agent", agent); super.onSaveInstanceState(out);
    }
    @Override protected void onDestroy() {
        destroyed = true; if (video != null) video.close(); if (voice != null) voice.close();
        if (tts != null) { tts.stop(); tts.shutdown(); }
        worker.shutdown(); super.onDestroy();
    }
}
