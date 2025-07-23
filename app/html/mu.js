var APP_PREFIX = 'mu_';
var VERSION = 'v1';
var URLS = [    
  `/`,
  `/index.html`,
  `/mu.png`,
  `/mu.js`
]

var CACHE_NAME = APP_PREFIX + VERSION

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

function askLLM(url, el, div) {
	var d = document.getElementById(div);

	const formData = new FormData(el);
	const data = {};

	// Iterate over formData and populate the data object
	for (let [key, value] of formData.entries()) {
		data[key] = value;
	}

	console.log("sending", data);
	document.getElementById("message").value = '';
	d.innerHTML += `<div>You: ${data["message"]}</div>`;

	fetch(url, {
	  method: "POST",
	  headers: {
	      'Content-Type': 'application/json'
	  },
	  body: JSON.stringify(data)
	}).then(response => response.json())
	.then(result => {
	    console.log('Success:', result);
	    // Handle success, e.g., show a success message
            d.innerHTML += `<div>LLM: ${result.answer}</div>`
	})
	.catch(error => {
	    console.error('Error:', error);
	    // Handle errors
	});

	return false;
}
