'use strict';
let player;
let ready = false;
window.microPlayerPlaying = false;
let desired = new URLSearchParams(location.search).get('autoplay') === '1' ? 'play' : 'pause';
window.microPlayerCommand = function(command) {
  if (command !== 'play' && command !== 'pause') return;
  desired = command;
  if (ready) {
    if (command === 'play') player.playVideo();
    else player.pauseVideo();
  }
};
window.onYouTubeIframeAPIReady = function() {
  const id = new URLSearchParams(location.search).get('id') || '';
  if (!/^[a-zA-Z0-9_-]{11}$/.test(id)) return;
  player = new YT.Player('player', {
    videoId: id,
    playerVars: {playsinline:1, origin:location.origin},
    events: {
      onStateChange: function(event) { window.microPlayerPlaying = event.data === YT.PlayerState.PLAYING; },
      onReady: function() { ready = true; window.microPlayerCommand(desired); },
      onError: function() {
        document.getElementById('status').textContent = 'Playback unavailable here. Use Open in YouTube.';
      },
      onAutoplayBlocked: function() {
        document.getElementById('status').textContent = 'Tap the player to begin playback.';
      }
    }
  });
};
