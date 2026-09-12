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
const stationary=true,transcript=false,contained=false,overlay=false,conv={hidden:false};
let questionAnchor;
const form={getBoundingClientRect(){return {height:42}}};
const handlers={};let resize;
class ResizeObserver{constructor(fn){resize=fn}observe(){}}
const window={ResizeObserver,addEventListener(k,fn){handlers[k]=fn},getComputedStyle(){return {top:'58px'}},matchMedia(){return {matches:false}}};
const requestAnimationFrame=fn=>fn();
let calls=0;
const question={isConnected:true,style:{},scrollIntoView(options){calls++;assert.equal(options.block,'start')}};
` + html[start:end] + `
questionAnchor=question;revealQuestion(question);
assert.equal(calls,1);
assert.equal(question.style.scrollMarginTop,'108px');
resize();assert.equal(calls,2);
handlers.wheel();resize();revealQuestion(question);assert.equal(calls,2);
questionAnchor=question;conv.hidden=true;resize();assert.equal(calls,2);
conv.hidden=false;question.isConnected=false;revealQuestion(question);assert.equal(calls,2);
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
