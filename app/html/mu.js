var APP_PREFIX = 'mu_';
var VERSION = 'v1';
var URLS = [    
  `/`,
  `/index.html`,
  `/mu.png`,
  `/mu.js`
]

var CACHE_NAME = APP_PREFIX + VERSION

let room = "";
let context = [];
let history = {};

async function networkFirst(request) {
  try {
    const networkResponse = await fetch(request);
    if (networkResponse.ok && request.method === 'GET') {
      const cache = await caches.open(CACHE_NAME);
      cache.put(request, networkResponse.clone());
    }
    return networkResponse;
  } catch (error) {
    const cachedResponse = await caches.match(request);
    return cachedResponse || Response.error();
  }
}

self.addEventListener('fetch', function (e) {
  console.log('Fetch request : ' + e.request.url);
  e.respondWith(networkFirst(e.request));
})

self.addEventListener('install', function (e) {
  e.waitUntil(
    caches.open(CACHE_NAME).then(function (cache) {
      console.log('Installing cache : ' + CACHE_NAME);
      return cache.addAll(URLS)
    })
  )
})

self.addEventListener('activate', function (e) {
  e.waitUntil(
    caches.keys().then(function (keyList) {
      var cacheWhitelist = keyList.filter(function (key) {
        return key.indexOf(APP_PREFIX)
      })
      cacheWhitelist.push(CACHE_NAME);
      return Promise.all(keyList.map(function (key, i) {
        if (cacheWhitelist.indexOf(key) === -1) {
          console.log('Deleting cache : ' + keyList[i] );
          return caches.delete(keyList[i])
        }
      }))
    })
  )
})

function loadContext() {
	var ctx = history[room];
	if (ctx == undefined || ctx == null || ctx == "undefined") {
		context = [];
		return
	}
	context = ctx;
}

function loadHistory() {
	var ctx = sessionStorage.getItem("history");
	console.log("Loading history", ctx)
	if (ctx == undefined) {
		console.log("History is undefined", ctx)
		return
	}
	var val = JSON.parse(ctx);
	if (val == null) {
		console.log("Failed to parse history", val)
		return
	}
	history = val;
}

function loadRoom() {
	// get the room
	room = window.location.hash.replace("#", "");

	if (room == "") {
		room = "all";
	}

	console.log("Loading room", room)
}

function setHistory() {
	sessionStorage.setItem("history", JSON.stringify(history));
}

function loadMessages() {
	console.log("loading messages for:", room);

	var d = document.getElementById("messages");

	if (room != "all") {
		d.innerHTML = '';
	}

	context.forEach(function(data) {
	  console.log(data);
	  d.innerHTML += `<div class="message"><span class="you">you</span><p>${data["prompt"]}</p></div>`;
	  d.innerHTML += `<div class="message"><span class="llm">llm</span>${data["answer"]}</div>`;
	})

	d.scrollTop = d.scrollHeight;
}

function askLLM(el) {
	var d = document.getElementById('messages');

	const formData = new FormData(el);
	const data = {};

	// Iterate over formData and populate the data object
	for (let [key, value] of formData.entries()) {
		data[key] = value;
	}

	console.log("sending", data);
	document.getElementById("prompt").value = '';
	d.innerHTML += `<div class="message"><span class="you">you</span><p>${data["prompt"]}</p></div>`;
	d.scrollTop = d.scrollHeight;

	var prompt = data["prompt"];

        data["context"] = context;
	data["room"] = room;

	fetch("/chat", {
	  method: "POST",
	  headers: {
	      'Content-Type': 'application/json'
	  },
	  body: JSON.stringify(data)
	}).then(response => response.json())
	.then(result => {
	    console.log('Success:', result);
	    // Handle success, e.g., show a success message
            d.innerHTML += `<div class="message"><span class="llm">llm</span>${result.answer}</div>`
	    d.scrollTop = d.scrollHeight;

	    // save the context
            context.push({answer: result.answer, prompt: prompt});
	    history[room] = context;
	    setHistory()
	})
	.catch(error => {
	    console.error('Error:', error);
	    // Handle errors
	});

	return false;
}

function loadChat() {
	loadRoom()
	loadHistory()
	loadContext()
        loadMessages();

        // scroll to bottom of prompt
        const prompt = document.getElementById('prompt'); // Your input element

        const messages = document.getElementById('messages');
        const container = document.getElementById('container');

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
                    messages.style.height = viewportHeight - 230;
		    container.style.height = viewportHeight - 135;
                } else {
                    // Keyboard closed, revert changes
                    // document.body.style.paddingBottom = '0';
                    messages.style.height = viewportHeight - 230;
		    container.style.height = viewportHeight - 135;
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
}

function getVideos(el) {
	const formData = new FormData(el);
	const data = {};

	// Iterate over formData and populate the data object
	for (let [key, value] of formData.entries()) {
		data[key] = value;
	}

	console.log("sending", data);

	fetch("/video", {
	  method: "POST",
	  headers: {
	      'Content-Type': 'application/json'
	  },
	  body: JSON.stringify(data)
	}).then(response => response.json())
	.then(result => {
	    console.log('Success:', result);
		var d = document.getElementById('results');

		if (d == null) {
			d = document.createElement("div");
			d.setAttribute("id", "results");

			var content = document.getElementById('content');
			content.innerHTML += "<h1>Results</h1>";
			content.appendChild(d);
		} else {
			d.innerHTML = '';
		}

	    // Handle success, e.g., show a success message
            d.innerHTML += result.html;
	    document.getElementById('query').value = data["query"];
	})
	.catch(error => {
	    console.error('Error:', error);
	    // Handle errors
	});

	return false;
};

self.addEventListener("hashchange", function(event) {
	// load chat
	if (window.location.pathname == "/chat") {
		loadRoom()
		loadHistory()
		loadContext()
		loadMessages();
	}
});

self.addEventListener('DOMContentLoaded', function() {
	// set nav
	var nav = document.getElementById("nav");
	for (const el of nav.children) {
		if (el.getAttribute("href") == window.location.pathname) {
			el.setAttribute("class", "active");
			continue
		}
		el.removeAttribute("class");
		//el.classList.remove("active");
	}

	// load chat
	if (window.location.pathname == "/chat") {
		loadChat();
	}
});
