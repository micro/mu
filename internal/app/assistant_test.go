package app

import (
	"os/exec"
	"testing"
)

func TestAssistantPanelRetainsConversationOnClose(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	script := `
const assert=require('assert');
let click,close,created=0,opened=0,appended=0;
const panel={setAttribute(){},querySelector(){return {addEventListener(_,fn){close=fn}}},showModal(){opened++},close(){this.closed=true}};
const document={addEventListener(_,fn,capture){assert(capture);click=fn},createElement(tag){assert.equal(tag,'dialog');created++;return panel},body:{classList:{remove(){}},appendChild(){appended++}}};
const location={pathname:'/home'},window={HTMLDialogElement:function(){}};
` + assistantJS + `
const event=()=>({target:{closest(){return {}}},button:0,preventDefault(){this.prevented=true},stopImmediatePropagation(){this.stopped=true}});
let e=event();click(e);assert(e.prevented && e.stopped);assert.equal(created,1);assert.equal(appended,1);
assert(panel.innerHTML.includes('/assistant?panel=1'));
const frame=panel.innerHTML;
close({preventDefault(){}});assert(panel.closed);
click(event());assert.equal(created,1);assert.equal(opened,2);assert.equal(panel.innerHTML,frame);
e=event();e.ctrlKey=true;click(e);assert(!e.prevented);
location.pathname='/assistant';e=event();click(e);assert(!e.prevented);
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
