package mu.client.android;

import java.util.Locale;

final class VoiceCommands {
    static String normalize(String text) {
        return text.trim().toLowerCase(Locale.ROOT).replaceAll("[.!?]+$", "").trim();
    }
    static String afterWake(String text) {
        String normalized = normalize(text);
        if (normalized.equals("hey micro")) return "";
        if (!normalized.startsWith("hey micro ")) return null;
        return text.trim().substring(10).trim();
    }
    static boolean wantsVideo(String text) {
        return normalize(text).matches("^(play|watch|listen to) .+");
    }
    static boolean playbackControl(String text) {
        switch (normalize(text)) {
            case "stop": case "pause": case "resume": case "continue": case "stop listening":
            case "next": case "previous": case "next headline": case "previous headline":
            case "back": case "close video": case "show text": case "show video": case "audio only":
                return true;
            default: return false;
        }
    }
    static boolean validVideo(String id) { return id.matches("[a-zA-Z0-9_-]{11}"); }
}
