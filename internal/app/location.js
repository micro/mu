var requestClientContext=(function(){
if(!document.getElementById('mu-chat-location'))return function(){return {};};
// Only the opt-out flag persists; coordinates remain in this page's memory.
var deviceLocation=null, locationBusy=false, locationEpoch=0, locationOff=false;
try { locationOff=sessionStorage.getItem('mu-location-off')==='1'; } catch(e) {}
var locationButton=document.getElementById('mu-chat-location');
function locationLabel(text){if(locationButton){
 var active=text.indexOf('Stop')!==-1;
 locationButton.setAttribute('aria-pressed',String(active));
 locationButton.setAttribute('aria-label',active?'Stop sharing approximate location':text==='Finding location…'?text:'Share approximate location');
 locationButton.title=active?'Approximate location shared with Micro and its model. Click to stop.':'Share approximate location with Micro and its model';
}}
function refreshDeviceLocation(explicit){
 if(locationOff || locationBusy || !navigator.geolocation || document.hidden)return;
 var epoch=locationEpoch;
 function locate(){
  if(locationOff || epoch!==locationEpoch)return;
  locationBusy=true;
  navigator.geolocation.getCurrentPosition(function(p){
   locationBusy=false;
   if(locationOff || epoch!==locationEpoch)return;
   deviceLocation={latitude:Math.round(p.coords.latitude*100)/100,longitude:Math.round(p.coords.longitude*100)/100,accuracy_m:Math.max(1600,p.coords.accuracy),captured_at:new Date(p.timestamp).toISOString(),source:'device'};
   locationLabel('Location shared · Stop');
  },function(){
   locationBusy=false;
   if(epoch!==locationEpoch)return;
   deviceLocation=null;locationLabel('Share location');
  },{maximumAge:60000,timeout:5000,enableHighAccuracy:false});
 }
 if(explicit){locate();return;}
 if(navigator.permissions && navigator.permissions.query){
  navigator.permissions.query({name:'geolocation'}).then(function(p){
   if(p.state==='granted')locate();
   else {deviceLocation=null;locationLabel('Share location');}
   p.onchange=function(){deviceLocation=null;locationEpoch++;locationLabel('Share location');};
  }).catch(function(){});
 }
}
if(locationButton){
 locationButton.hidden=!navigator.geolocation;
 locationButton.onclick=function(){
  if(deviceLocation || locationBusy){
   locationOff=true;locationEpoch++;deviceLocation=null;locationBusy=false;
   try{sessionStorage.setItem('mu-location-off','1');}catch(e){}
   locationLabel('Share location');
  }else{
   locationOff=false;
   try{sessionStorage.removeItem('mu-location-off');}catch(e){}
   locationLabel('Finding location…');refreshDeviceLocation(true);
  }
 };
}
function requestClientContext(){
 var context={};
 try{context.timezone=Intl.DateTimeFormat().resolvedOptions().timeZone;}catch(e){}
 if(!locationOff && deviceLocation && Date.now()-Date.parse(deviceLocation.captured_at)<=300000)context.location=deviceLocation;
 refreshDeviceLocation(false);
 return context;
}
refreshDeviceLocation(false);
document.addEventListener('visibilitychange',function(){if(!document.hidden)refreshDeviceLocation(false);});
window.addEventListener('focus',function(){refreshDeviceLocation(false);});
setInterval(function(){refreshDeviceLocation(false);},60000);

return requestClientContext;
})();
