package mu.micro.launcher

import org.junit.Assert.assertEquals
import org.junit.Test

class NavigationTest {
    @Test fun onlyExactHttpsOriginStaysInside() {
        assertEquals(Navigation.Destination.MICRO, Navigation.destination(Navigation.HOME))
        assertEquals(Navigation.Destination.MICRO, Navigation.destination("https://micro.mu:443/inbox"))
        for (url in listOf("https://micro.mu.evil.test/home", "https://micro.mu@evil.test", "http://micro.mu", "https://micro.mu:8443")) {
            assertEquals(url, Navigation.Destination.EXTERNAL, Navigation.destination(url))
        }
    }
    @Test fun oauthStartsInBrowserAndUnsafeSchemesAreBlocked() {
        assertEquals(Navigation.Destination.BROWSER_LOGIN, Navigation.destination("https://micro.mu/oauth2/google"))
        for (url in listOf("javascript:alert(1)", "file:///etc/passwd", "content://secret", "intent://app", "data:text/html,hello", "not a uri")) {
            assertEquals(url, Navigation.Destination.BLOCKED, Navigation.destination(url))
        }
        assertEquals(Navigation.Destination.EXTERNAL, Navigation.destination("tel:+441234567890"))
    }
}
