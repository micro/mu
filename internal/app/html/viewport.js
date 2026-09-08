// Full-page conversations use the visible viewport, including in an installed
// app whose keyboard overlays rather than resizes the layout viewport.
(function(){
  var queued=false;
  function fit(){
    queued=false;
    var content=document.getElementById('content');
    var chat=content&&content.querySelector('.chat-layout,.room-layout');
    var bounded=!!chat&&window.matchMedia('(max-width:900px)').matches;
    document.body.classList.toggle('chat-page',bounded);
    if(!bounded){if(content)content.style.height='';return;}
    var viewport=window.visualViewport;
    var height=viewport?viewport.height:window.innerHeight;
    var offset=viewport?viewport.offsetTop:0;
    document.documentElement.style.setProperty('--visible-height',(height+offset)+'px');
    // Both values use layout viewport coordinates. Do not subtract page scroll
    // or the keyboard height a second time.
    var top=content.getBoundingClientRect().top;
    content.style.height=Math.max(0,height+offset-top)+'px';
  }
  function schedule(){if(!queued){queued=true;requestAnimationFrame(fit);}}
  window.addEventListener('resize',schedule);
  document.addEventListener('focusin',schedule);
  document.addEventListener('focusout',schedule);
  if(window.visualViewport){window.visualViewport.addEventListener('resize',schedule);window.visualViewport.addEventListener('scroll',schedule);}
  new MutationObserver(schedule).observe(document.getElementById('content')||document.body,{childList:true,subtree:true});
  schedule();
})();
