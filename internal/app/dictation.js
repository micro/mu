(function () {
  var mic = document.getElementById('mu-chat-mic');
  var input = document.getElementById('mu-chat-input');
  var status = document.getElementById('mu-chat-voice-status');
  var Recognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (!mic || !input || !Recognition) return;
  mic.hidden = false;
  var recognition = null, before = '', after = '', inserted = '', active = false, accepting = false;
  function stop() { accepting = false; if (recognition) recognition.stop(); }
  function reset() {
    active = false; accepting = false; mic.setAttribute('aria-pressed', 'false');
    mic.setAttribute('aria-label', 'Dictate');
    if (status.textContent === 'Listening…') status.textContent = '';
  }
  mic.onclick = function () {
    if (active) { stop(); return; }
    before = input.value.slice(0, input.selectionStart);
    after = input.value.slice(input.selectionEnd); inserted = '';
    recognition = new Recognition(); accepting = true; active = true;
    recognition.lang = navigator.language || 'en-GB';
    recognition.continuous = false;
    recognition.interimResults = true;
    recognition.onstart = function () {
      active = true; mic.setAttribute('aria-pressed', 'true');
      mic.setAttribute('aria-label', 'Stop dictation'); status.textContent = 'Listening…';
    };
    recognition.onresult = function (event) {
      if (!accepting) return;
      var text = '';
      for (var i = 0; i < event.results.length; i++) text += event.results[i][0].transcript;
      inserted = (before && !/\s$/.test(before) ? ' ' : '') + text;
      input.value = (before + inserted + after).slice(0, input.maxLength);
      input.dispatchEvent(new Event('input', {bubbles:true}));
    };
    recognition.onerror = function (event) {
      status.textContent = event.error === 'not-allowed' ? 'Microphone permission was denied. You can still type.' : 'Dictation stopped. Try again or continue typing.';
      reset();
    };
    recognition.onend = reset;
    try { recognition.start(); } catch (e) { status.textContent = 'Dictation is unavailable. You can still type.'; reset(); }
  };
  // Typing or sending ends capture, and dictation never sends a message itself.
  input.addEventListener('beforeinput', function () { if (active) stop(); });
  input.addEventListener('keydown', function () { if (active) stop(); });
  document.getElementById('mu-chat-form').addEventListener('submit', stop);
  window.addEventListener('pagehide', stop);
  document.addEventListener('visibilitychange', function () { if (document.hidden) stop(); });
})();
