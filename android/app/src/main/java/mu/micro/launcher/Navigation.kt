package mu.micro.launcher

import java.net.URI

internal object Navigation {
    const val HOME = "https://micro.mu/home"
    enum class Destination { MICRO, BROWSER_LOGIN, EXTERNAL, BLOCKED }

    fun destination(raw: String): Destination {
        val uri = try { URI(raw) } catch (_: Exception) { return Destination.BLOCKED }
        if (uri.scheme.equals("https", true) && uri.host.equals("micro.mu", true) &&
            uri.userInfo == null && (uri.port == -1 || uri.port == 443)) {
            // OAuth must start in the browser so its state cookie stays there.
            if (uri.path.orEmpty().startsWith("/oauth2/")) return Destination.BROWSER_LOGIN
            return Destination.MICRO
        }
        return if (uri.scheme?.lowercase() in setOf("https", "http", "mailto", "tel", "sms", "geo")) {
            Destination.EXTERNAL
        } else Destination.BLOCKED
    }
}
