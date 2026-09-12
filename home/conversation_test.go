package home

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestClosePreservesHomeExchange(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	raw, err := os.ReadFile("views.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	start, end := strings.Index(src, "  const actions="), strings.Index(src, "  const upcoming")
	if start < 0 || end <= start {
		t.Fatal("conversation controller missing")
	}
	script := `
const assert=require('assert');
const events={},actionBar={},transcript={textContent:'Existing answer'};
const button={addEventListener(k,fn){this[k]=fn}},prompt={addEventListener(k,fn){this[k]=fn}};
const home={querySelector(s){return s==='#home-conversation-actions'?actionBar:s==='#home-conversation-close'?button:s==='#mu-chat-input'?prompt:transcript}};
let resets=0;
const window={addEventListener(k,fn){events[k]=fn},muChatClose(){resets++}};
` + src[start:end] + `
assert.equal(actionBar.hidden,false);
button.click({preventDefault(){},stopPropagation(){}});
assert.equal(resets,0);
assert.equal(transcript.hidden,true);
assert.equal(transcript.textContent,'Existing answer');
assert.equal(actionBar.hidden,true);
prompt.focus();assert.equal(transcript.hidden,false);
events['mu-chat-active']({detail:true});
assert.equal(actionBar.hidden,false);
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
