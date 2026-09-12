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
	body := ChatComponent(ChatConfig{Ask: true, ContinueNS: "assistant:account:alice:home"})
	start, end := strings.Index(body, "// Open the saved conversation itself"), strings.Index(body, "// Start a fresh session")
	if start < 0 || end <= start {
		t.Fatal("missing transfer controller")
	}
	script := `
const assert=require('assert');
let click,url,stored;
const button={disabled:false,getAttribute(){return String(this.disabled)},addEventListener(_,fn){click=fn}},status={};
const document={getElementById(id){return id==='mu-chat-continue'?button:status}};
const CONTINUE_NS='assistant:account:alice:home',conv={innerHTML:'<div>Private answer</div>'},history=[{prompt:'Private question',answer:'Private answer'}],contextId='original-thread',input={value:'Follow up'};
const sessionStorage={setItem(key,value){stored={key,value}}};
const window={location:{assign(value){url=value}}};
` + body[start:end] + `
click({preventDefault(){},stopPropagation(){}});assert.equal(url,'/assistant?session=original-thread');assert.equal(stored.key,'mu_chat_continue_draft:original-thread');assert.equal(stored.value,'Follow up');
url=undefined;sessionStorage.setItem=()=>{throw Error('storage unavailable')};click({preventDefault(){},stopPropagation(){}});assert.equal(url,'/assistant?session=original-thread');
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
