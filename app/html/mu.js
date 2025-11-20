var APP_PREFIX = 'mu_';
var VERSION = 'v5';
var CACHE_NAME = APP_PREFIX + VERSION;

// Static assets to cache on install
var STATIC_CACHE = [
  '/mu.css',
  '/mu.js',
  '/mu.png',
  '/home.png',
  '/chat.png',
  '/blog.png',
  '/news.png',
  '/video.png',
  '/mail.png',
  '/icon-192.png',
  '/icon-512.png',
  '/manifest.webmanifest'
];

// Pages to cache on first visit
var PAGE_CACHE = [
  '/home',
  '/chat',
  '/blog',
  '/news',
  '/video'
];

let context = [];

// Cache-first strategy with network fallback
async function cacheFirst(request) {
  const cachedResponse = await caches.match(request);
  if (cachedResponse) {
    // Update cache in background if online
    if (navigator.onLine) {
      fetch(request).then(networkResponse => {
        if (networkResponse.ok && request.method === 'GET') {
          caches.open(CACHE_NAME).then(cache => {
            cache.put(request, networkResponse.clone());
          });
        }
      }).catch(() => {
        // Silently fail - we already have cached version
      });
    }
    return cachedResponse;
  }
  
  // Not in cache, try network
  try {
    const networkResponse = await fetch(request);
    if (networkResponse.ok && request.method === 'GET') {
      const cache = await caches.open(CACHE_NAME);
      cache.put(request, networkResponse.clone());
    }
    return networkResponse;
  } catch (error) {
    // Offline and not cached
    return new Response('Offline - content not available', {
      status: 503,
      statusText: 'Service Unavailable',
      headers: new Headers({
        'Content-Type': 'text/plain'
      })
    });
  }
}

// Network-first for API calls
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
    return cachedResponse || new Response('Offline', {
      status: 503,
      statusText: 'Service Unavailable'
    });
  }
}

self.addEventListener('fetch', function (e) {
  const url = new URL(e.request.url);
  
  console.log('Fetch request : ' + e.request.url);
  
  // Skip non-GET requests
  if (e.request.method !== 'GET') {
    e.respondWith(fetch(e.request));
    return;
  }
  
  // For root path, use network-first to allow redirects
  if (url.pathname === '/' || url.pathname === '') {
    e.respondWith(fetch(e.request));
    return;
  }
  
  // Use cache-first for static assets and pages
  if (
    url.pathname.match(/\.(css|js|png|jpg|jpeg|gif|svg|ico|webmanifest)$/) ||
    PAGE_CACHE.includes(url.pathname)
  ) {
    e.respondWith(cacheFirst(e.request));
  } else {
    // Network-first for API calls
    e.respondWith(networkFirst(e.request));
  }
})

self.addEventListener('install', function (e) {
  console.log('Installing service worker version: ' + VERSION);
  e.waitUntil(
    caches.open(CACHE_NAME).then(function (cache) {
      console.log('Caching static assets');
      return cache.addAll(STATIC_CACHE);
    }).then(() => {
      // Skip waiting to activate immediately
      return self.skipWaiting();
    })
  );
})

self.addEventListener('activate', function (e) {
  console.log('Activating service worker version: ' + VERSION);
  e.waitUntil(
    caches.keys().then(function (keyList) {
      return Promise.all(keyList.map(function (key) {
        if (key.startsWith(APP_PREFIX) && key !== CACHE_NAME) {
          console.log('Deleting old cache: ' + key);
          return caches.delete(key);
        }
      }));
    }).then(() => {
      // Take control of all clients immediately
      return self.clients.claim();
    })
  );
})

function loadMessages() {
	console.log("loading messages"); 

	var d = document.getElementById("messages");

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

	var p = document.getElementById("prompt");

	if (p.value == "") {
		return false
	}

	// reset prompt
	p.value = '';

	console.log("sending", data);
	d.innerHTML += `<div class="message"><span class="you">you</span><p>${data["prompt"]}</p></div>`;
	d.scrollTop = d.scrollHeight;

	var prompt = data["prompt"];

        data["context"] = context;

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
	    setContext();
	})
	.catch(error => {
	    console.error('Error:', error);
	    // Handle errors
	});

	return false;
}

function loadContext() {
	var ctx = sessionStorage.getItem("context");
	if (ctx == null || ctx == undefined || ctx == "") {
		return
	}
	context = JSON.parse(ctx);
}

function setContext() {
	sessionStorage.setItem("context", JSON.stringify(context));
}

function getCookie(name) {
  var match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  if (match) return match[2];
}

function setSession() {
	fetch("/session", {
	  method: "POST",
	  headers: {
	      'Content-Type': 'application/json'
	  },
	}).then(response => response.json())
	.then(sess => {
	    console.log('Success:', sess);
	    var acc = document.getElementById("account");
	    if (sess.type == "account") {
	      acc.innerHTML = "<a href='/logout'>Logout</a>";
	    } else {
	      acc.innerHTML = "<a href='/login'>Login</a>";
	      // If we're on a protected page but not logged in, redirect
	      const protectedPaths = ['/home', '/chat', '/blog', '/news', '/video'];
	      if (protectedPaths.includes(window.location.pathname)) {
	        window.location.href = '/';
	      }
	    }
	})
	.catch(error => {
	    console.error('Error:', error);
	    // On error, redirect to home
	    window.location.href = '/';
	});
}

function loadChat() {
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
                    messages.style.height = viewportHeight - 225;
		    container.style.height = viewportHeight - 135;
                } else {
                    // Keyboard closed, revert changes
                    // document.body.style.paddingBottom = '0';
                    messages.style.height = viewportHeight - 225;
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

	// Check session status on page load
	setSession();
});
