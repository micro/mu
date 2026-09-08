(function () {
  var root = document.getElementById('home-status');
  if (!root) return;
  var label = root.querySelector('[data-status-label]');
  var input = root.querySelector('[data-status-input]');
  var feedback = root.querySelector('[data-status-feedback]');
  if (root.dataset.statusSaved === undefined) root.dataset.statusSaved = input.value;
  var saved = root.dataset.statusSaved, editing = !input.hidden, saving = false;
  // Soft navigation serializes this DOM, including an in-flight editor.
  if (input.readOnly) feedback.textContent = 'Save interrupted. Press Enter or tap away to retry.';
  input.readOnly = false;
  input.addEventListener('input', function () { input.defaultValue = input.value; });
  function close() {
    editing = false;
    input.hidden = true;
    label.hidden = false;
    label.textContent = saved || 'What are you up to?';
  }
  label.addEventListener('click', function () {
    if (saving) return;
    input.value = input.defaultValue = saved;
    label.hidden = true;
    input.hidden = false;
    editing = true;
    feedback.textContent = '';
    input.focus();
  });
  async function save() {
    if (!editing || saving) return;
    if (input.value === saved) { close(); return; }
    saving = true;
    input.readOnly = true;
    feedback.textContent = 'Saving…';
    var controller = new AbortController();
    var timeout = setTimeout(function () { controller.abort(); }, 15000);
    try {
      var response = await fetch('/home', {
        method: 'POST', credentials: 'same-origin', signal: controller.signal,
        headers: {'Accept': 'application/json', 'Content-Type': 'application/x-www-form-urlencoded'},
        body: new URLSearchParams({action: 'status', status: input.value, _csrf: root.dataset.csrf})
      });
      if (!response.ok) throw new Error('save failed');
      var result = await response.json();
      if (typeof result.status !== 'string') throw new Error('invalid response');
      saved = result.status;
      root.dataset.statusSaved = saved;
      input.value = input.defaultValue = saved;
      var hadFocus = document.activeElement === input;
      close();
      feedback.textContent = '';
      if (hadFocus) label.focus();
    } catch (err) {
      feedback.textContent = 'Could not save. Your text is kept here; press Enter or tap away to retry.';
    } finally {
      clearTimeout(timeout);
      input.readOnly = false;
      saving = false;
    }
  }
  input.addEventListener('blur', save);
  input.addEventListener('keydown', function (event) {
    if (event.isComposing || saving) return;
    if (event.key === 'Enter') { event.preventDefault(); save(); }
    if (event.key === 'Escape') {
      event.preventDefault();
      input.value = input.defaultValue = saved;
      feedback.textContent = '';
      close();
      label.focus();
    }
  });
})();
