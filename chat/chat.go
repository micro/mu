package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/micro/mu/app"
	"github.com/micro/mu/llm"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// render chat
		res := app.RenderHTML("Chat", "Chat with AI", `
<div id="messages"></div>
<form id="chat-form" action="/chat" method="POST" onsubmit="event.preventDefault(); askLLM('/chat', this, 'messages');">
<input id="context" name="context" type="hidden">
<input id="prompt" name="prompt" type="text" autofocus autocomplete=off>
<button>-></button>
<script>
document.addEventListener('DOMContentLoaded', function() {
	onLoad("messages");

	// scroll to bottom of prompt
	const prompt = document.getElementById('prompt'); // Your input element

	const messages = document.getElementById('messages');

	if (window.visualViewport) {
	    window.visualViewport.addEventListener('resize', () => {
		const viewportHeight = window.visualViewport.height;
		const documentHeight = document.documentElement.clientHeight;

		// If the viewport height has significantly decreased, the keyboard is likely open
		if (viewportHeight < documentHeight) {
		    // Adjust your layout. For example, you might set the height of your
		    // messages container or add a class to shift content up.
		    // This is a more advanced approach and requires careful calculation
		    // of your layout.
		    // Example: document.body.style.paddingBottom = (documentHeight - viewportHeight) + 'px';
		    // Or: Make sure your input container stays at the bottom of the *visual* viewport.
		    // You'd typically make your chat messages div fill the available height
		    // and the input box positioned relative to the bottom of that.

		    messages.style.height = viewportHeight - 200;
		} else {
		    // Keyboard closed, revert changes
		    // document.body.style.paddingBottom = '0';
		    messages.style.height = viewportHeight - 200;
		}

		// After adjusting, you might still want to call scrollIntoView
		// to ensure the input is exactly where you want it.
		messages.scrollTop = messages.scrollHeight;
		//prompt.scrollIntoView({ behavior: 'smooth', block: 'end' });
		window.scrollTo(0, document.body.scrollHeight);
	    });
	} else {
	    // Fallback for browsers not supporting visualViewport (e.g., older Android)
	    window.addEventListener('resize', () => {
		// Similar logic as above, but window.innerHeight might behave differently
		// depending on the browser.
		//prompt.scrollIntoView({ behavior: 'smooth', block: 'end' });
		window.scrollTo(0, document.body.scrollHeight);
	    });
	}
});
</script>
</form>`)

		w.Write([]byte(res))
		return
	}

	if r.Method == "POST" {
		data := make(map[string]interface{})

		if ct := r.Header.Get("Content-Type"); ct == "application/json" {
			b, _ := ioutil.ReadAll(r.Body)
			if len(b) == 0 {
				return
			}

			json.Unmarshal(b, &data)

			if data["prompt"] == nil {
				return
			}
		} else {
			// save the response
			r.ParseForm()

			// get the message
			msg := r.Form.Get("prompt")
			ctx := r.Form.Get("context")

			if len(msg) == 0 {
				return
			}

			var ictx interface{}
			json.Unmarshal([]byte(ctx), &ictx)
			data["prompt"] = msg
			data["context"] = ictx
		}

		var ctx []string
		if vals := data["context"].([]interface{}); vals != nil {
			for _, val := range vals {
				b, _ := json.Marshal(val)
				ctx = append(ctx, string(b))
			}
		}

		// query the llm
		resp := llm.Query(context.TODO(), ctx, fmt.Sprintf("%v", data["prompt"]))

		if len(resp) == 0 {
			return
		}

		// save the response
		data["answer"] = resp
		b, _ := json.Marshal(data)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}
}
