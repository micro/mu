(() => {
  const home = document.getElementById('home-cards');
  if (!home) return;
  const actions=home.querySelector('#home-conversation-actions');
  const transcript=home.querySelector('#mu-chat-conv');
  const overview=home.querySelector('#home-overview');
  const toggle=home.querySelector('#home-conversation-toggle');
  let active=false;
  function conversation(show){
    active=show;
    transcript.hidden=!show;
    overview.hidden=show;
    actions.hidden=!show && !transcript.textContent.trim();
    toggle.textContent=show?'Today':'Resume conversation';
    toggle.setAttribute('aria-expanded',String(show));
  }
  window.addEventListener('mu-chat-active',event=>conversation(event.detail===true));
  toggle.addEventListener('click',()=>{
    conversation(!active);
    if(!active)refreshOverview('overview');
  });
  conversation(false);

  function refreshOverview(section) {
    // Guests have no private counts or calendars to refresh.
    if (overview.dataset.guest === 'true') return;
    fetch('/home?section='+section, {credentials: 'same-origin'})
      .then(response => { if (!response.ok) throw new Error('overview'); return response.json(); })
      .then(data => { overview.innerHTML=data.overview; })
      .catch(() => {
        if(overview.querySelector('[data-overview-error]'))return;
        const note=document.createElement('p');
        note.dataset.overviewError='';
        note.className='text-muted';
        note.textContent='The overview could not be refreshed and may be out of date.';
        overview.appendChild(note);
      });
  }
  if (overview.dataset.fresh !== 'true') refreshOverview('upcoming');
})();
