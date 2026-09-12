package app

import (
	"os/exec"
	"strings"
	"testing"
)

func TestContinueTransfersExchangeWithoutWordsInURL(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	body := ChatComponent(ChatConfig{Ask: true, ContinueNS: "assistant:account:alice"})
	start, end := strings.Index(body, "// Move a completed exchange"), strings.Index(body, "// Start a fresh session")
	if start < 0 || end <= start {
		t.Fatal("missing transfer controller")
	}
	script := `
const assert=require('assert');
let click,url,stored;
const button={disabled:false,addEventListener(_,fn){click=fn}},status={};
const document={getElementById(id){return id==='mu-chat-continue'?button:status}};
const CONTINUE_NS='assistant:account:alice',conv={innerHTML:'<div>Private answer</div>'},history=[{prompt:'Private question',answer:'Private answer'}],contextId='original-thread',input={value:'Follow up'};
const sessionStorage={setItem(key,value){stored={key,value}}};
const window={location:{assign(value){url=value}}};
` + body[start:end] + `
click();assert.equal(url,'/assistant');assert.equal(stored.key,'mu_chat_handoff:assistant:account:alice');
const exchange=JSON.parse(stored.value);assert.equal(exchange.context,'original-thread');assert.deepEqual(exchange.history,history);assert.equal(exchange.html,conv.innerHTML);assert.equal(exchange.draft,'Follow up');
url=undefined;button.disabled=true;click();assert.equal(url,undefined);
button.disabled=false;sessionStorage.setItem=()=>{throw Error('storage unavailable')};click();assert.equal(url,undefined);assert(status.textContent.includes('Could not move'));
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
