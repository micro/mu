package mu.client.android;

import java.util.ArrayList;
import java.util.List;

final class SpeechPages {
    static List<String> split(String text, int limit) {
        List<String> pages = new ArrayList<>();
        String remaining = text.trim();
        while (!remaining.isEmpty()) {
            int end = Math.min(limit, remaining.length());
            if (end < remaining.length()) {
                int boundary = remaining.lastIndexOf(' ', end);
                if (boundary > end / 2) end = boundary;
                if (Character.isHighSurrogate(remaining.charAt(end - 1))) end--;
            }
            pages.add(remaining.substring(0, end));
            remaining = remaining.substring(end).trim();
        }
        return pages;
    }
}
