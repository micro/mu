package app

import (
	"os/exec"
	"strings"
	"testing"
)

func TestInlineQuestionScrollClearsStickyInput(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	html := ChatComponent(ChatConfig{Ask: true, Stationary: true})
	start, end := strings.Index(html, "function revealQuestion(node){"), strings.Index(html, "function toBottom(force,smooth){")
	if start < 0 || end <= start {
		t.Fatal("missing question scroll controller")
	}
	script := `
const assert=require('assert');
const stationary=true,transcript=false,contained=false;
const form={getBoundingClientRect(){return {height:42}}};
const window={getComputedStyle(){return {top:'58px'}},matchMedia(){return {matches:false}}};
const requestAnimationFrame=fn=>fn();
let calls=0;
const question={isConnected:true,style:{},scrollIntoView(options){calls++;assert.equal(options.block,'start')}};
` + html[start:end] + `
revealQuestion(question);
assert.equal(calls,1);
assert.equal(question.style.scrollMarginTop,'108px');
question.isConnected=false;revealQuestion(question);assert.equal(calls,1);
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
