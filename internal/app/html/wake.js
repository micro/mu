// Optional wake-activated dictation. It fills the ordinary input; it never
// sends a request or changes the reader's Speak preference.
(function(){
  if(window.muWake)window.muWake.destroy();
  var panel=document.getElementById('mu-chat-wake');
  var button=document.getElementById('mu-chat-wake-toggle');
  var status=document.getElementById('mu-chat-wake-status');
  var box=document.getElementById('mu-chat-input');
  var SR=window.SpeechRecognition||window.webkitSpeechRecognition;
  if(!panel||!button||!status||!box||!SR)return;
  panel.hidden=false;
  var enabled=false,rec=null,retry=null,capture=null,dictating=false,base='',failures=0,monitor=null;
  function report(text){status.textContent=text;}
  function abort(){
    var old=rec;rec=null;
    if(old){old.onend=null;old.onresult=null;try{old.abort();}catch(e){}}
  }
  function stop(message){
    enabled=false;dictating=false;clearTimeout(retry);clearTimeout(capture);clearInterval(monitor);monitor=null;abort();
    button.textContent='Start listening';button.setAttribute('aria-pressed','false');
    report(message||'Listening is off.');
  }
  function speaking(){return window.speechSynthesis&&(window.speechSynthesis.speaking||window.speechSynthesis.pending);}
  function later(){clearTimeout(retry);if(enabled)retry=setTimeout(start,1000);}
  function finish(){dictating=false;clearTimeout(capture);abort();report('Check the words, then press Send.');later();}
  function fill(text){
    var value=base+text;
    if(box.maxLength>0&&value.length>box.maxLength){stop('That was too long. Please finish in the text box.');return false;}
    box.value=value;box.dispatchEvent(new Event('input',{bubbles:true}));return true;
  }
  function start(){
    if(!enabled||rec)return;
    if(document.hidden||!panel.isConnected||!panel.open){stop();return;}
    if(speaking()){later();return;}
    var current=new SR();rec=current;
    current.lang=document.documentElement.lang||'en-GB';
    current.continuous=true;current.interimResults=true;
    current.onstart=function(){if(rec===current)report(dictating?'Listening. Say your request.':'Listening for “Hey Micro”.');};
    current.onresult=function(e){
      if(!enabled||rec!==current||document.hidden||!panel.isConnected||speaking())return;
      for(var i=e.resultIndex;i<e.results.length;i++){
        var result=e.results[i],text=String(result[0].transcript||'').trim();
        if(!dictating){
          if(!result.isFinal)continue;
          var wake=/^hey[\s,]+micro\b[\s,.!?:-]*(.*)$/i.exec(text);
          if(!wake)continue;
          failures=0;dictating=true;base=box.value?box.value.replace(/\s*$/,'')+' ':'';
          report('Listening. Say your request.');
          clearTimeout(capture);capture=setTimeout(finish,10000);
          text=wake[1].trim();
          if(!text)continue;
        }
        if(!fill(text))return;
        if(result.isFinal&&text){finish();return;}
      }
    };
    current.onerror=function(e){
      if(rec!==current)return;
      if(e.error==='no-speech')return;
      stop(e.error==='not-allowed'||e.error==='service-not-allowed'?'Microphone access was denied. You can still type.':'Voice recognition stopped. Try starting again or use the text box.');
    };
    current.onend=function(){
      if(rec!==current)return;rec=null;
      if(dictating){later();return;}
      if(++failures>20){stop('Voice recognition stopped. Start listening to try again.');return;}
      later();
    };
    try{current.start();}catch(e){stop('Voice recognition is unavailable. You can still type.');}
  }
  function toggle(){
    if(enabled){stop();return;}
    if(window.muStopDictation)window.muStopDictation();
    enabled=true;failures=0;
    monitor=setInterval(function(){if(speaking()&&rec){dictating=false;clearTimeout(capture);abort();later();}},250);
    button.textContent='Stop listening';button.setAttribute('aria-pressed','true');
    report('Starting microphone…');start();
  }
  function hidden(){if(document.hidden)stop('Listening stopped because you left this page.');}
  function navigated(){if(!panel.isConnected)destroy();}
  function collapsed(){if(!panel.open)stop();}
  function leave(){stop();}
  function destroy(){stop();panel.removeEventListener('toggle',collapsed);button.removeEventListener('click',toggle);document.removeEventListener('visibilitychange',hidden);document.removeEventListener('mu:navigated',navigated);window.removeEventListener('pagehide',leave);}
  button.addEventListener('click',toggle);
  document.addEventListener('visibilitychange',hidden);
  document.addEventListener('mu:navigated',navigated);
  window.addEventListener('pagehide',leave);
  panel.addEventListener('toggle',collapsed);
  window.muWake={stop:stop,destroy:destroy};
})();
