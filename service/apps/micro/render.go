package micro

import (
	"encoding/json"
	"html"
	"strings"
)

// Render turns a validated spec into a complete, self-contained HTML app.
//
// The page embeds the spec as JSON and ships a single generic runtime that
// builds the UI and persists state to localStorage. Because the runtime is
// fixed and correct, every spec that validates renders into a working app —
// there is no model-authored markup or JS to go wrong.
func Render(s *Spec) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	specJSON, err := json.Marshal(s) // Go escapes <,>,& so this is safe inside <script>
	if err != nil {
		return "", err
	}
	page := pageTemplate
	page = strings.ReplaceAll(page, "__TITLE__", html.EscapeString(s.Title))
	page = strings.ReplaceAll(page, "__SPEC__", string(specJSON))
	return page, nil
}

const pageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>__TITLE__</title>
<style>
*{box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#fff;color:#111;margin:0;padding:20px;max-width:560px;margin:0 auto}
h1{font-size:22px;font-weight:800;margin:0 0 16px;display:flex;align-items:center;gap:8px}
.row{display:flex;gap:8px;margin-bottom:8px;align-items:center;flex-wrap:wrap}
input,select{padding:9px 11px;border:1px solid #ddd;border-radius:8px;font-size:15px;font-family:inherit;background:#fff}
input[type=text],input[type=number],input[type=date]{flex:1;min-width:90px}
button{padding:9px 14px;background:#111;color:#fff;border:none;border-radius:8px;cursor:pointer;font-size:15px;font-family:inherit}
button.ghost{background:#f2f2f2;color:#333;padding:6px 10px;font-size:13px}
button.icon{background:none;color:#bbb;padding:2px 6px;font-size:16px}
ul{list-style:none;padding:0;margin:0}
li{display:flex;align-items:center;gap:10px;padding:10px 0;border-bottom:1px solid #f0f0f0;font-size:15px}
li:last-child{border-bottom:none}
.muted{color:#888;font-size:13px}
.total{margin-top:14px;padding:12px 14px;background:#f7f7f7;border-radius:8px;font-weight:700;display:flex;justify-content:space-between}
.check{display:flex;align-items:center;gap:10px;cursor:pointer}
.check input{width:18px;height:18px}
.done{color:#aaa;text-decoration:line-through}
.cval{font-size:24px;font-weight:800;min-width:48px;text-align:center}
.empty{color:#aaa;padding:14px 0;font-size:14px}
</style>
</head>
<body>
<div id="app"></div>
<script>var SPEC=__SPEC__;</script>
<script>
(function(){
  var KEY='microapp:'+SPEC.type+':'+(SPEC.title||'app');
  var app=document.getElementById('app');
  function load(){try{return JSON.parse(localStorage.getItem(KEY))||{}}catch(e){return{}}}
  function save(){try{localStorage.setItem(KEY,JSON.stringify(state))}catch(e){}}
  var state=load();
  function esc(s){return String(s==null?'':s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')}
  function el(tag,attrs,html){var e=document.createElement(tag);if(attrs)for(var k in attrs){if(k==='class')e.className=attrs[k];else e.setAttribute(k,attrs[k]);}if(html!=null)e.innerHTML=html;return e;}

  function header(){
    var h=el('h1',null,(SPEC.emoji?esc(SPEC.emoji)+' ':'')+esc(SPEC.title));
    app.appendChild(h);
  }

  function tracker(){
    state.entries=state.entries||[];
    var form=el('div',{'class':'row'});
    var inputs={};
    SPEC.fields.forEach(function(f){
      var t=f.type==='number'?'number':(f.type==='date'?'date':'text');
      var inp=el('input',{type:t,placeholder:esc(f.name)});
      inputs[f.name]=inp;form.appendChild(inp);
    });
    var add=el('button',null,'Add');
    add.onclick=function(){
      var row={};var any=false;
      SPEC.fields.forEach(function(f){var v=inputs[f.name].value;if(v!=='')any=true;row[f.name]=v;});
      if(!any)return;
      state.entries.unshift(row);save();inputs[SPEC.fields[0].name].focus();draw();
    };
    form.appendChild(add);app.appendChild(form);

    var list=el('ul');
    if(state.entries.length===0)list.appendChild(el('li',{'class':'empty'},'No entries yet.'));
    state.entries.forEach(function(row,i){
      var parts=SPEC.fields.map(function(f){return '<b>'+esc(f.name)+':</b> '+esc(row[f.name])}).join(' &middot; ');
      var li=el('li',null,'<span style="flex:1">'+parts+'</span>');
      var del=el('button',{'class':'icon'},'×');del.onclick=function(){state.entries.splice(i,1);save();draw();};
      li.appendChild(del);list.appendChild(li);
    });
    app.appendChild(list);

    if(SPEC.sum){
      var total=0;state.entries.forEach(function(r){var n=parseFloat(r[SPEC.sum]);if(!isNaN(n))total+=n;});
      app.appendChild(el('div',{'class':'total'},'<span>Total '+esc(SPEC.sum)+'</span><span>'+(Math.round(total*100)/100)+'</span>'));
    }
  }

  function checklist(){
    state.checked=state.checked||{};
    state.extra=state.extra||[];
    var items=SPEC.items.concat(state.extra);
    var list=el('ul');
    items.forEach(function(it){
      var li=el('li');
      var lab=el('label',{'class':'check'});
      var cb=el('input',{type:'checkbox'});cb.checked=!!state.checked[it];
      cb.onchange=function(){state.checked[it]=cb.checked;save();draw();};
      var span=el('span',{'class':state.checked[it]?'done':''},esc(it));
      lab.appendChild(cb);lab.appendChild(span);li.appendChild(lab);list.appendChild(li);
    });
    app.appendChild(list);
    var form=el('div',{'class':'row'});
    var inp=el('input',{type:'text',placeholder:'Add item'});
    var add=el('button',null,'Add');
    add.onclick=function(){var v=inp.value.trim();if(!v)return;state.extra.push(v);inp.value='';save();draw();};
    form.appendChild(inp);form.appendChild(add);app.appendChild(form);
  }

  function counter(){
    state.values=state.values||{};
    SPEC.counters.forEach(function(c){
      var step=c.step||1;
      if(state.values[c.label]==null)state.values[c.label]=0;
      var row=el('div',{'class':'row'});
      row.appendChild(el('span',{style:'flex:1;font-size:16px'},esc(c.label)));
      var minus=el('button',{'class':'ghost'},'−');
      var val=el('span',{'class':'cval'},String(state.values[c.label]));
      var plus=el('button',{'class':'ghost'},'+');
      minus.onclick=function(){state.values[c.label]-=step;save();draw();};
      plus.onclick=function(){state.values[c.label]+=step;save();draw();};
      row.appendChild(minus);row.appendChild(val);row.appendChild(plus);app.appendChild(row);
    });
  }

  function draw(){
    app.innerHTML='';header();
    if(SPEC.type==='tracker')tracker();
    else if(SPEC.type==='checklist')checklist();
    else if(SPEC.type==='counter')counter();
  }
  draw();
})();
</script>
</body>
</html>`
