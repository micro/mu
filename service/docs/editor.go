package docs

const editorTools = `<div class="doc-toolbar" role="group" aria-label="Text formatting">
<button type="button" data-before="**" data-after="**" title="Bold"><strong>B</strong></button>
<button type="button" data-before="_" data-after="_" title="Italic"><em>I</em></button>
<button type="button" data-before="## " data-line="1">Heading</button>
<button type="button" data-before="- " data-line="1">List</button>
<button type="button" data-before="1. " data-line="1">Numbered list</button>
<button type="button" data-before="&gt; " data-line="1">Quote</button>
<button type="button" data-before="[" data-after="](https://)">Link</button>
<button type="button" data-before="&#96;" data-after="&#96;">Code</button>
</div>`

const editorScript = `<script>(function(){
var body=document.getElementById('doc-body');if(!body||body.dataset.editorWired)return;body.dataset.editorWired='1';
document.querySelectorAll('.doc-toolbar button').forEach(function(b){b.addEventListener('click',function(){var start=body.selectionStart,end=body.selectionEnd,text=body.value.slice(start,end),before=b.dataset.before||'',after=b.dataset.after||'';if(b.dataset.line){start=body.value.lastIndexOf('\n',start-1)+1;text=body.value.slice(start,end).split('\n').map(function(line){return before+line}).join('\n');before='';}body.setRangeText(before+text+after,start,end,'select');body.focus();body.dispatchEvent(new Event('input',{bubbles:true}));});});
})();</script>`
