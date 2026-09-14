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
  document.querySelectorAll('#nav a, #mobile-nav a').forEach(function (a) {
    var u = new URL(a.href);
    if (a.closest('#mobile-nav') ? u.pathname === location.pathname : session ? u.searchParams.get('session') === session : u.pathname === location.pathname && !u.search) { a.classList.add('active'); a.setAttribute('aria-current', 'page'); }
  });
});
if (navigator.serviceWorker) navigator.serviceWorker.register('/mu.js', {scope:'/', updateViaCache:'none'}).then(function (r) {r.update();}).catch(function () {});

document.addEventListener('click', function(e) {
  document.querySelectorAll('.conversation-switcher[open]').forEach(function(menu) {
    if(!menu.contains(e.target))menu.open=false;
  });
});
document.addEventListener('keydown', function(e) {
  if(e.key==='Escape')document.querySelectorAll('.conversation-switcher[open]').forEach(function(menu){menu.open=false;});
});
