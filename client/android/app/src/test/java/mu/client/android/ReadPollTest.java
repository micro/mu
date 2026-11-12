package mu.client.android;
import org.junit.Test;
import static org.junit.Assert.*;
public class ReadPollTest {
    @Test public void pendingReadsStayBelow150PerHour() {
        ReadPoll poll = new ReadPoll();
        long elapsed = 0; int calls = 0;
        while (elapsed < 3600000) { elapsed += poll.next(); calls++; }
        assertTrue(calls < 150);
        poll.reset(); assertEquals(3000L, poll.next());
    }
}
