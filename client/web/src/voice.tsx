import { useEffect, useRef, useState } from "react";
interface Recognition {
  interimResults: boolean;
  onresult:
    | ((event: {
        results: ArrayLike<ArrayLike<{ transcript: string }>>;
      }) => void)
    | null;
  onerror: (() => void) | null;
  onend: (() => void) | null;
  start(): void;
  stop(): void;
}
type SpeechWindow = Window & {
  SpeechRecognition?: new () => Recognition;
  webkitSpeechRecognition?: new () => Recognition;
};
export function Voice({
  reply,
  onDictation,
  onStatus,
  thread,
  busy,
}: {
  reply: string;
  onDictation: (text: string) => void;
  onStatus: (text: string) => void;
  thread: string;
  busy: boolean;
}) {
  const [active, setActive] = useState(false),
    recognition = useRef<Recognition | null>(null),
    lock = useRef<WakeLockSentinel | null>(null),
    consent = useRef(false),
    speechRef = useRef<SpeechSynthesisUtterance | null>(null),
    generation = useRef(0);
  const RecognitionAPI =
    (window as SpeechWindow).SpeechRecognition ||
    (window as SpeechWindow).webkitSpeechRecognition;
  function stop() {
    generation.current++;
    if (speechRef.current) {
      speechRef.current.onend = speechRef.current.onerror = null;
      speechRef.current = null;
    }
    if (recognition.current) {
      recognition.current.onend =
        recognition.current.onresult =
        recognition.current.onerror =
          null;
      recognition.current.stop();
      recognition.current = null;
    }
    window.speechSynthesis?.cancel();
    lock.current?.release();
    setActive(false);
  }
  async function awake() {
    try {
      const version = generation.current;
      const acquired = (await navigator.wakeLock?.request("screen")) || null;
      if (version !== generation.current) {
        acquired?.release();
        return;
      }
      lock.current = acquired;
    } catch {}
  }
  useEffect(() => {
    stop();
    return stop;
  }, [thread, busy]);
  return (
    <div className="form-actions voice-controls">
      {RecognitionAPI && (
        <button
          type="button"
          onClick={() => {
            if (
              !consent.current &&
              !confirm(
                "Your browser may send audio to its speech recognition provider. Continue?",
              )
            )
              return;
            consent.current = true;
            stop();
            const rec = new RecognitionAPI();
            recognition.current = rec;
            rec.interimResults = true;
            rec.onresult = (event) =>
              onDictation(
                Array.from(event.results, (r) => r[0].transcript).join(" "),
              );
            rec.onerror = () =>
              onStatus("Could not hear you. Try again or type.");
            rec.onend = () => {
              setActive(false);
              lock.current?.release();
              onStatus("Review your message, then send.");
            };
            rec.start();
            setActive(true);
            onStatus("Listening…");
            awake();
          }}
        >
          Speak
        </button>
      )}
      {"speechSynthesis" in window && (
        <button
          type="button"
          disabled={!reply || busy}
          onClick={() => {
            stop();
            const speech = new SpeechSynthesisUtterance(reply);
            speechRef.current = speech;
            speech.onend = speech.onerror = () => {
              setActive(false);
              onStatus("");
              lock.current?.release();
            };
            speechSynthesis.speak(speech);
            setActive(true);
            onStatus("Reading reply…");
            awake();
          }}
        >
          Read reply
        </button>
      )}
      {active && (
        <button
          type="button"
          onClick={() => {
            stop();
            onStatus("");
          }}
        >
          Stop
        </button>
      )}
    </div>
  );
}
