function toggleMenu() {
  if (matchMedia('(min-width:901px)').matches) {
    var collapsed = document.body.classList.toggle('nav-collapsed');
    try { localStorage.setItem('mu_nav_collapsed', collapsed ? '1' : '0'); } catch (_) {}
  } else document.body.classList.toggle('menu-open');
}
document.addEventListener('keydown', function (e) {
  if (e.key === 'Escape') document.body.classList.remove('menu-open');
});
document.addEventListener('DOMContentLoaded', function () {
  var session = new URL(location.href).searchParams.get('session');
  document.querySelectorAll('#nav a').forEach(function (a) {
    var u = new URL(a.href);
    if (session ? u.searchParams.get('session') === session : u.pathname === location.pathname && !u.search) { a.classList.add('active'); a.setAttribute('aria-current', 'page'); }
  });
});
if (navigator.serviceWorker) navigator.serviceWorker.register('/mu.js', {scope:'/', updateViaCache:'none'}).then(function (r) {r.update();}).catch(function () {});

document.addEventListener('click', function(e) {
  document.querySelectorAll('.action-menu[open]').forEach(function(menu) {
    if(!menu.contains(e.target))menu.open=false;
  });
});
document.addEventListener('keydown', function(e) {
  if(e.key==='Escape')document.querySelectorAll('.action-menu[open]').forEach(function(menu){menu.open=false;menu.querySelector('summary').focus();});
});

// Keep shared action menus inside the viewport, wherever their trigger appears.
document.addEventListener('toggle', function(e) {
  var menu=e.target;
  if(!menu.matches('.action-menu')||!menu.open)return;
  var items=menu.querySelector('.action-menu-items');
  items.style.marginLeft='0';items.style.marginTop='0';
  var box=items.getBoundingClientRect();
  if(box.right>innerWidth-16)items.style.marginLeft=(innerWidth-16-box.right)+'px';
  var viewport=window.visualViewport,top=viewport?viewport.offsetTop:0,bottom=top+(viewport?viewport.height:innerHeight);
  if(box.bottom>bottom-8){var trigger=menu.querySelector('summary').getBoundingClientRect();items.style.marginTop=(Math.max(top+8,trigger.top-8-box.height)-box.top)+'px';}
},true);

// One accessible recent-search component. Each form keeps its own transport.
(function () {
  function wire() {
    document.querySelectorAll('[data-scroll-end]:not([data-scroll-ready])').forEach(function (e) {
      e.dataset.scrollReady = '1'; requestAnimationFrame(function () { e.scrollTop = e.scrollHeight; });
    });
    document.querySelectorAll('[data-recent-searches]:not([data-recent-ready])').forEach(function (root) {
      var form = document.getElementById(root.dataset.recentSearches);
      var input = form && form.querySelector('input[name=q],input[name=query]');
      if (!input) return;
      root.dataset.recentReady = '1';
      var key = root.dataset.storageKey;
      function normalized(value) { return value.trim().replace(/\s+/g, ' ').toLowerCase(); }
      function load() {
        try { var rows = JSON.parse(localStorage.getItem(key) || '[]'); return Array.isArray(rows) ? rows.filter(function (x) { return typeof x === 'string' && x.trim(); }).filter(function (x, i, all) { return all.findIndex(function (y) { return normalized(y) === normalized(x); }) === i; }).slice(0, 10) : []; }
        catch (_) { return []; }
      }
      function save(rows) { try { localStorage.setItem(key, JSON.stringify(rows.slice(0, 10))); } catch (_) {} }
      function remember(query) { if (query) save([query].concat(load().filter(function (x) { return normalized(x) !== normalized(query); }))); }
      function render() {
        root.replaceChildren();
        var rows = load(); if (!rows.length) return;
        var heading = document.createElement('h3'); heading.textContent = 'Recent searches'; root.append(heading);
        var list = document.createElement('div'); list.className = 'form-actions'; root.append(list);
        rows.forEach(function (query) {
          var group = document.createElement('span'); group.className = 'form-actions no-wrap';
          var search = document.createElement('button'); search.type = 'button'; search.textContent = query;
          search.addEventListener('click', function () { input.value = query; remember(query); form.requestSubmit(); });
          var remove = document.createElement('button'); remove.type = 'button'; remove.textContent = '×'; remove.setAttribute('aria-label', 'Remove recent search: ' + query);
          remove.addEventListener('click', function () { save(load().filter(function (x) { return normalized(x) !== normalized(query); })); render(); });
          group.append(search, remove); list.append(group);
        });
      }
      form.addEventListener('submit', function () { remember(input.value.trim()); });
      render();
    });
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', wire); else wire();
  new MutationObserver(wire).observe(document.documentElement, {childList: true, subtree: true});
})();
