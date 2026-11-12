package mu.client.android;

// Keep the initial response quick without polling indefinitely every three seconds.
final class ReadPoll {
    private int attempt;
    long next() { return Math.min(30000L, 3000L << Math.min(attempt++, 4)); }
    void reset() { attempt = 0; }
}
