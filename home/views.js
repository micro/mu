(() => {
  const home = document.getElementById('home-cards');
  if (!home) return;
  const actions=home.querySelector('#home-conversation-actions');
  function conversation(active){
    if(actions)actions.hidden=!active;
  }
  window.addEventListener('mu-chat-active',event=>conversation(event.detail===true));
  const close=home.querySelector('#home-conversation-close');
  if(close)close.addEventListener('click',event=>{event.preventDefault();event.stopPropagation();window.muChatNew();});
  const initial=home.querySelector('#mu-chat-conv');
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
})();
