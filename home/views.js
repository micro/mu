(() => {
  const home = document.getElementById('home-cards');
  if (!home) return;
  const tabs = [...home.querySelectorAll('.view-switch [role="tab"]')];
  const positions = new Map();
  let selected = tabs.find(tab => tab.getAttribute('aria-selected') === 'true');
  function select(tab) {
    if (tab === selected) return;
    positions.set(selected.id, window.scrollY);
    tabs.forEach(item => {
      const active = item === tab;
      item.setAttribute('aria-selected', String(active));
      item.tabIndex = active ? 0 : -1;
      document.getElementById(item.getAttribute('aria-controls')).hidden = !active;
    });
    selected = tab;
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
