package docs

const editorTools = `<div class="doc-toolbar d-flex gap-2 mb-3" role="group" aria-label="Text formatting" style="flex-wrap:wrap">
<button type="button" data-before="**" data-after="**" title="Bold"><strong>B</strong></button>
<button type="button" data-before="_" data-after="_" title="Italic"><em>I</em></button>
<button type="button" data-before="## " data-line="1">Heading</button>
<button type="button" data-before="- " data-line="1">List</button>
<button type="button" data-before="1. " data-line="1">Numbered list</button>
<button type="button" data-before="&gt; " data-line="1">Quote</button>
<button type="button" data-before="[" data-after="](https://)">Link</button>
<button type="button" data-before="&#96;" data-after="&#96;">Code</button>
<label class="btn">Import text<input id="doc-import" type="file" accept=".txt,.md,.markdown,text/plain,text/markdown" class="sr-only"></label>
</div><p id="doc-import-status" class="text-sm text-muted" role="status">Import a Markdown or plain-text file, then save it as a document.</p>`

const editorScript = `<script>(function(){
var body=document.getElementById('doc-body'),pick=document.getElementById('doc-import');if(!body||body.dataset.editorWired)return;body.dataset.editorWired='1';
document.querySelectorAll('.doc-toolbar button').forEach(function(b){b.addEventListener('click',function(){var start=body.selectionStart,end=body.selectionEnd,text=body.value.slice(start,end),before=b.dataset.before||'',after=b.dataset.after||'';if(b.dataset.line){start=body.value.lastIndexOf('\n',start-1)+1;text=body.value.slice(start,end).split('\n').map(function(line){return before+line}).join('\n');before='';}body.setRangeText(before+text+after,start,end,'select');body.focus();body.dispatchEvent(new Event('input',{bubbles:true}));});});
pick.addEventListener('change',async function(){var file=pick.files[0],status=document.getElementById('doc-import-status');if(!file)return;if(file.size>1048576){status.textContent='Choose a file up to 1 MB.';return;}if(!/\.(txt|md|markdown)$/i.test(file.name)){status.textContent='Choose a Markdown or plain-text file.';return;}if(body.value.trim()&&!confirm('Replace the current draft with this file?'))return;try{var text=new TextDecoder('utf-8',{fatal:true}).decode(await file.arrayBuffer());if(text.includes('\u0000'))throw new Error();body.value=text;var title=document.getElementById('doc-title');if(!title.value)title.value=file.name.replace(/\.[^.]+$/,'');status.textContent='Imported into this draft. Save to keep it.';body.dispatchEvent(new Event('input',{bubbles:true}));}catch(e){status.textContent='Could not import this file. Use UTF-8 plain text or Markdown.';}pick.value='';});
})();</script>`
