# Browser voice

The microphone beside the Home/agent composer opens a local voice dialog.
The user must enable voice; no microphone is opened or model downloaded on page
load. Recording produces an editable transcript. Send uses the conversation's
existing authenticated agent flow, context and usage accounting.

The optional “Hey Micro” mode recognises an English wake phrase locally with
Whisper tiny.en. Say “Hey Micro” and pause, then say a request; a request following
the phrase in the same utterance also works. This is an experimental speech-based
wake listener, not a specialised low-power keyword model. It can be slow or
misrecognise speech. It runs only in the foreground and stops on hiding the page,
closing the dialog or selecting Stop voice. It never resumes across reloads.

Whisper speech recognition and Kokoro spoken replies run in a Web Worker using
WASM. Runtimes are pinned (Transformers.js 3.8.1, Kokoro.js 1.2.1); models are
Xenova/whisper-tiny.en and onnx-community/Kokoro-82M-v1.0-ONNX (q8).
Initial downloads exceed 100 MB and are cached by the runtime/browser when
available. Model hosting is Hugging Face; runtime hosting is jsDelivr. Neither
receives recorded audio or response text. Only the resulting request is sent to
Micro; the configured agent may send that text to its model provider as usual.
There are no speech API keys or speech-provider charges.

Speak replies is optional and loads Kokoro only when needed. Replies are limited
to 1,500 characters for playback; the full response stays in the conversation.
Recording pauses during playback to avoid speaking to itself. Stop speaking
interrupts playback; Stop voice terminates inference and releases the microphone.

The main application's CSP is unchanged. Only the dedicated worker permits
WASM compilation and the named runtime/model origins. Parent/iframe messages
check both origin and source; they cannot overwrite an existing composer draft.

Requires HTTPS (or localhost), microphone permission, WebAssembly, AudioContext
and sufficient memory. This is not a background, locked-screen or native client.
