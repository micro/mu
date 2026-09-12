package home

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCollapsePreservesConversation(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	raw, err := os.ReadFile("views.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	start, end := strings.Index(src, "  const personal="), strings.Index(src, "  const upcoming")
	if start < 0 || end <= start {
		t.Fatal("conversation controller missing")
	}
	script := `
const assert=require('assert');
const events={},classes=new Set(),attrs={};
const panel={classList:{toggle(k,on){on?classes.add(k):classes.delete(k)}}};
const transcript={textContent:'Existing answer'};
const button={addEventListener(k,fn){this[k]=fn},setAttribute(k,v){attrs[k]=v}};
const home={querySelector(s){return s==='#home-personal'?panel:s==='#home-conversation-toggle'?button:transcript}};
const window={addEventListener(k,fn){events[k]=fn}};
` + src[start:end] + `
assert(classes.has('is-conversing'));
button.click();
assert(classes.has('conversation-collapsed'));
assert.equal(transcript.textContent,'Existing answer');
assert.equal(button.textContent,'⌄ Resume conversation');
button.click();
assert(classes.has('is-conversing'));
assert.equal(transcript.textContent,'Existing answer');
events['mu-chat-active']({detail:false});
assert(button.hidden);
assert(classes.has('conversation-collapsed'));
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
