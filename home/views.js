(() => {
  const home = document.getElementById('home-cards');
  if (!home) return;
  const personal=home.querySelector('#home-personal');
  const conversationToggle=home.querySelector('#home-conversation-toggle');
  let hasConversation=false, expanded=false;
  function conversation(active){
    expanded=active;
    if(personal) {
      personal.classList.toggle('is-conversing',active);
      personal.classList.toggle('conversation-collapsed',!active);
    }
    if(conversationToggle){
      conversationToggle.hidden=!hasConversation && !active;
      conversationToggle.textContent=active?'Collapse conversation':'Resume conversation';
      conversationToggle.setAttribute('aria-expanded',String(active));
    }
  }
  window.addEventListener('mu-chat-active',event=>{
    if(event.detail===true)hasConversation=true;
    else hasConversation=false;
    conversation(event.detail===true);
  });
  if(conversationToggle)conversationToggle.addEventListener('click',()=>conversation(!expanded));
  const initial=home.querySelector('#mu-chat-conv');
  hasConversation=!!initial && !!initial.textContent.trim();
  conversation(hasConversation);

  function openAssistant(){
    const tab=document.getElementById('home-view-personal');
    if(tab)tab.click();
    conversation(true);
  }
  const composer=home.querySelector('#mu-chat-input');
  if(composer)composer.addEventListener('focus',openAssistant);
  const assistantLink=document.getElementById('nav-assistant');
  if(assistantLink)assistantLink.addEventListener('click',event=>{
    if(event.button!==0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)return;
    event.preventDefault();event.stopPropagation();
    openAssistant();
    if(composer)composer.focus({preventScroll:true});
  });
  const homeLink=document.getElementById('nav-home');
  if(homeLink)homeLink.addEventListener('click',event=>{
    if(event.button!==0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)return;
    event.preventDefault();event.stopPropagation();
    const tab=document.getElementById('home-view-personal');
    if(tab)tab.click();
    conversation(false);
  });
  if(new URL(window.location.href).searchParams.get('assistant')==='1'){
    conversation(true);
  }

  const upcoming = home.querySelector('[data-home-upcoming]');
  if (upcoming) {
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
      if(tab.id==='home-view-personal' && expanded)conversation(false);
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
