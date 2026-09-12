(() => {
  // A retained frame owns the stream and transcript independently of the page
  // behind it. Closing the panel does not reset or abort that conversation.
  let panel;
  document.addEventListener('click', event => {
    const link = event.target.closest && event.target.closest('#nav-assistant');
    if (!link || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    if (location.pathname === '/assistant') return;
    if (!window.HTMLDialogElement) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    document.body.classList.remove('menu-open');
    if (!panel) {
      panel = document.createElement('dialog');
      panel.id = 'assistant-panel';
      panel.setAttribute('aria-label', 'Assistant');
      panel.innerHTML = '<div class="assistant-panel-bar"><a href="/assistant">Assistant</a><a href="#" data-assistant-close>Close</a></div><iframe title="Conversation with Micro" src="/assistant?panel=1"></iframe>';
      panel.querySelector('[data-assistant-close]').addEventListener('click', e => {
        e.preventDefault();
        panel.close();
      });
      document.body.appendChild(panel);
    }
    panel.showModal();
  }, true);
})();
