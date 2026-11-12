// One deferred read at a time. Hiding a view or going offline suspends it;
// returning resumes it once, without resetting its backoff.
export class ReadPoll {
  private timer: ReturnType<typeof setTimeout> | undefined;
  private job: (() => void) | undefined;
  private attempt = 0;
  private failures = 0;
  constructor(private visible: () => boolean) {}
  next(failed = false, pages = 1) {
    if (failed)
      return (
        Math.min(300000, 5000 * 2 ** Math.min(this.failures++, 6)) *
        Math.max(1, pages)
      );
    this.failures = 0;
    return (
      Math.min(30000, 3000 * 2 ** Math.min(this.attempt++, 4)) *
      Math.max(1, pages)
    );
  }
  schedule(job: () => void, delay: number) {
    this.pause();
    this.job = job;
    if (this.visible()) this.timer = setTimeout(() => this.run(), delay);
  }
  private run() {
    this.pause();
    if (!this.visible()) return;
    const job = this.job;
    this.job = undefined;
    job?.();
  }
  pause() {
    clearTimeout(this.timer);
    this.timer = undefined;
  }
  resume() {
    if (this.visible()) this.run();
    else this.pause();
  }
  cancel() {
    this.pause();
    this.job = undefined;
    this.attempt = this.failures = 0;
  }
}
