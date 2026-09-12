(() => {
  const home = document.getElementById('home-cards');
  if (!home) return;
  const actions=home.querySelector('#home-conversation-actions');
  const initial=home.querySelector('#mu-chat-conv');
  const input=home.querySelector('#mu-chat-input');
  function conversation(active){
    if(actions)actions.hidden=!active;
    if(initial)initial.hidden=!active;
  }
  window.addEventListener('mu-chat-active',event=>conversation(event.detail===true));
  const close=home.querySelector('#home-conversation-close');
  if(close)close.addEventListener('click',event=>{
    event.preventDefault();event.stopPropagation();conversation(false);
  });
  if(input)input.addEventListener('focus',()=>{
    if(initial&&initial.textContent.trim())conversation(true);
  });
  conversation(!!initial && !!initial.textContent.trim());

  const upcoming = home.querySelector('[data-home-upcoming]');
  if (upcoming && upcoming.dataset.fresh !== 'true') {
    fetch('/home?section=upcoming', {credentials: 'same-origin'})
      .then(response => { if (!response.ok) throw new Error('events'); return response.json(); })
      .then(data => {
        upcoming.innerHTML = data.upcoming;
        const brief = home.querySelector("#home-brief");
        if (brief) brief.innerHTML = data.brief;
        const todo = home.querySelector("#home-todo");
        if (todo) todo.innerHTML = data.todo;
        upcoming.querySelectorAll('[data-event-time]').forEach(node => {
          node.textContent = new Date(node.dateTime).toLocaleString(undefined, {weekday:'short', day:'numeric', month:'short', hour:'2-digit', minute:'2-digit'});
        });
      })
      .catch(() => { upcoming.innerHTML = '<p class="text-muted">Could not load events. <a href="/events">Open events</a></p>'; });
  }
  const tabs = [...home.querySelectorAll('.view-switch [role="tab"]')];
  const positions = new Map();
  let selected = tabs.find(tab => tab.getAttribute('aria-selected') === 'true');
  function select(tab) {
    if (tab === selected) {
      return;
    }
    positions.set(selected.id, window.scrollY);
    tabs.forEach(item => {
      const active = item === tab;
      item.setAttribute('aria-selected', String(active));
      item.tabIndex = active ? 0 : -1;
      document.getElementById(item.getAttribute('aria-controls')).hidden = !active;
    });
    selected = tab;
    if (tab.id === 'home-view-feed') {
      document.getElementById('home-feed').dispatchEvent(new Event('home-feed-shown'));
    }
    const url = new URL(location.href);
    if (tab.id === 'home-view-feed') url.searchParams.set('view', 'feed');
    else url.searchParams.delete('view');
    history.replaceState(history.state, '', url);
    window.scrollTo({top: positions.get(tab.id) || 0, behavior: 'instant'});
  }
  tabs.forEach((tab, index) => {
    tab.addEventListener('click', event => {
      if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      event.preventDefault();
      event.stopPropagation();
      select(tab);
    });
    tab.addEventListener('keydown', event => {
      let next;
      if (event.key === 'ArrowRight') next = tabs[(index + 1) % tabs.length];
      if (event.key === 'ArrowLeft') next = tabs[(index + tabs.length - 1) % tabs.length];
      if (event.key === 'Home') next = tabs[0];
      if (event.key === 'End') next = tabs[tabs.length - 1];
      if (!next) return;
      event.preventDefault();
      next.focus();
      select(next);
    });
  });
})();
