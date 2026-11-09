package mu.client.android;

import java.util.Locale;

final class VoiceCommands {
    static String normalize(String text) {
        return text.trim().toLowerCase(Locale.ROOT).replaceAll("[.!?]+$", "").trim();
    }
    static String afterWake(String text) { return afterWake(text, "Micro"); }
    static String[] names(String agentName, String aliases) {
        java.util.LinkedHashSet<String> names = new java.util.LinkedHashSet<>();
        names.add(agentName.trim());
        for (String alias : aliases.split(",")) {
            String name = alias.trim();
            if (!name.isEmpty() && name.length() <= 40 && names.size() < 9) names.add(name);
        }
        return names.toArray(new String[0]);
    }
    static String afterWake(String text, String... names) {
        for (String name : names) {
            String command = afterName(text, name);
            if (command != null) return command;
        }
        return null;
    }
    private static String afterName(String text, String agentName) {
        String name = agentName.trim();
        if (name.isEmpty()) return null;
        String[] words = name.split("\\s+");
        StringBuilder prefix = new StringBuilder("^\\s*hey\\s+");
        for (int i = 0; i < words.length; i++) {
            if (i > 0) prefix.append("\\s+");
            prefix.append(java.util.regex.Pattern.quote(words[i]));
        }
        prefix.append("(?=$|[\\s,.!?])");
        java.util.regex.Matcher match = java.util.regex.Pattern.compile(prefix.toString(),
                java.util.regex.Pattern.CASE_INSENSITIVE | java.util.regex.Pattern.UNICODE_CASE).matcher(text);
        if (!match.find()) return null;
        return text.substring(match.end()).replaceFirst("^[\\s,.!?]+", "").trim();
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
