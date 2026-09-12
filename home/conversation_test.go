package home

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestOverviewTogglePreservesConversation(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	raw, err := os.ReadFile("views.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	start, end := strings.Index(src, "  const actions="), strings.Index(src, "  function refreshOverview")
	if start < 0 || end <= start {
		t.Fatal("conversation controller missing")
	}
	script := `
const assert=require('assert');
let refreshes=0;
function refreshOverview(){refreshes++;}
const events={},actionBar={},transcript={textContent:'Existing answer'},overview={};
const button={addEventListener(k,fn){this[k]=fn},setAttribute(k,v){this[k]=v}};
const home={querySelector(s){return {'#home-conversation-actions':actionBar,'#home-conversation-toggle':button,'#mu-chat-conv':transcript,'#home-overview':overview}[s]}};
const window={addEventListener(k,fn){events[k]=fn},muChatNew(){throw Error('must not reset conversation')}};
` + "{\n" + src[start:end] + `
assert.equal(transcript.hidden,true);
assert.equal(overview.hidden,false);
assert.equal(actionBar.hidden,false);
assert.equal(button.textContent,'Resume conversation');
button.click();
assert.equal(transcript.hidden,false);
assert.equal(overview.hidden,true);
assert.equal(button['aria-expanded'],'true');
button.click();
assert.equal(transcript.textContent,'Existing answer');
assert.equal(refreshes,1);
assert.equal(overview.hidden,false);
events['mu-chat-active']({detail:true});
assert.equal(transcript.hidden,false);
assert.equal(overview.hidden,true);
transcript.textContent='';
events['mu-chat-active']({detail:false});
assert.equal(actionBar.hidden,true);
assert.equal(overview.hidden,false);
}
`

	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
