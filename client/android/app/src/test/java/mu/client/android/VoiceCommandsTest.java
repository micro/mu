package mu.client.android;

import org.junit.Test;
import static org.junit.Assert.*;

public class VoiceCommandsTest {
    @Test public void wakeRequiresExplicitPrefix() {
        assertNull(VoiceCommands.afterWake("play music"));
        assertNull(VoiceCommands.afterWake("they said hey micro play music"));
        assertNull(VoiceCommands.afterWake("hey microsoft"));
        assertEquals("", VoiceCommands.afterWake("Hey Micro."));
        assertEquals("play K-pop demon hunters", VoiceCommands.afterWake("Hey Micro play K-pop demon hunters"));
    }
    @Test public void wakeFollowsSelectedAgentAndRequiresWholeName() {
        assertEquals("", VoiceCommands.afterWake("Hey Malten.", "Malten"));
        assertEquals("help me reflect", VoiceCommands.afterWake("HEY MALTEN, help me reflect", "Malten"));
        assertNull(VoiceCommands.afterWake("hey micro help me reflect", "Malten"));
        assertNull(VoiceCommands.afterWake("hey maltenish help", "Malten"));
        assertNull(VoiceCommands.afterWake("someone said hey malten help", "Malten"));
        assertEquals("read the headlines", VoiceCommands.afterWake("hey   daily   guide read the headlines", "Daily Guide"));
        assertNull(VoiceCommands.afterWake("hey anything help", ".*"));
        assertNull(VoiceCommands.afterWake("hey help", ""));
    }
    @Test public void wakeAliasesAreExplicitAndScopedToTheSelectedAgent() {
        String[] malten = VoiceCommands.names("Malten", "molten, maltin, , molten");
        assertArrayEquals(new String[]{"Malten", "molten", "maltin"}, malten);
        assertEquals("help me reflect", VoiceCommands.afterWake("hey molten help me reflect", malten));
        assertEquals("", VoiceCommands.afterWake("hey maltin", malten));
        assertNull(VoiceCommands.afterWake("hey molten help", VoiceCommands.names("Micro", "")));
        assertNull(VoiceCommands.afterWake("hey moltenish help", malten));
        assertNull(VoiceCommands.afterWake("someone said hey molten help", malten));
        assertNull(VoiceCommands.afterWake("hey anything help", VoiceCommands.names("Malten", ".*")));
    }
    @Test public void playbackCannotIssueAccountActions() {
        assertTrue(VoiceCommands.playbackControl("pause"));
        assertTrue(VoiceCommands.playbackControl("Stop listening"));
        assertFalse(VoiceCommands.playbackControl("email my contacts"));
        assertFalse(VoiceCommands.playbackControl("read that"));
        assertFalse(VoiceCommands.playbackControl("pause and send an email"));
    }
    @Test public void onlyRealVideoIdentifiersReachPlayer() {
        assertTrue(VoiceCommands.validVideo("aB_cD-12345"));
        assertFalse(VoiceCommands.validVideo("javascript:alert(1)"));
        assertFalse(VoiceCommands.validVideo("abc'/><script>"));
        assertFalse(VoiceCommands.validVideo("https://youtube.com/watch?v=abc"));
        assertFalse(VoiceCommands.wantsVideo("Explain how to play this"));
        assertTrue(VoiceCommands.wantsVideo("Play K-pop demon hunters"));
    }
    @Test public void longReadoutsAreNotTruncated() {
        String text = "First headline. ".repeat(1000);
        java.util.List<String> pages = SpeechPages.split(text, 2500);
        assertTrue(pages.size() > 1);
        assertEquals(text.trim(), String.join(" ", pages));
        for (String page : pages) assertTrue(page.length() <= 2500);
    }
}
