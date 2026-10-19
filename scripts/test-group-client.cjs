const fs=require('fs'),vm=require('vm'),assert=require('assert');
const source=fs.readFileSync('internal/app/html/mu.js','utf8');
const start=source.indexOf('var topic="";');const end=source.indexOf('\n})();',start);
let timers=[],sockets=[],data={id:'group_test',authenticated:true},input={value:'hello'},status={},button={};
class Socket {static OPEN=1; constructor(){this.readyState=0;sockets.push(this)} close(){this.readyState=3;if(this.onclose)this.onclose({code:1000})} send(s){this.sent=s}}
const nodes={'room-data':{get textContent(){return JSON.stringify(data)}},messages:{innerHTML:'',replaceChildren(){}},'room-status':status};
const context={WebSocket:Socket,console,setTimeout:f=>timers.push(f),window:{location:{protocol:'https:',host:'micro.test'},addEventListener(){}},document:{readyState:'loading',addEventListener(){},getElementById:id=>nodes[id],querySelector:()=>button},showToast(){}};
vm.createContext(context);vm.runInContext(source.slice(start,end),context);
context.connectRoomWebSocket(data.id);assert.equal(button.disabled,true);
sockets[0].readyState=1;sockets[0].onopen();context.sendRoomMessage({querySelector:()=>input});assert.equal(JSON.parse(sockets[0].sent).content,'hello');assert.equal(input.value,'');
sockets[0].close();assert.equal(timers.length,1);input.value='keep this';context.sendRoomMessage({querySelector:()=>input});assert.equal(input.value,'keep this');timers.shift()();assert.equal(sockets.length,2);
sockets[1].close();data=null;timers.shift()();assert.equal(sockets.length,2);
console.log('PASS: send, disconnect, reconnect, unsent draft and navigation guards');
