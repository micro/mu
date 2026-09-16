import { Button } from "./ui/button";
import { PageHeading } from "./layout";
const example = `<h1>News headlines</h1>
<button id="load">Load news</button>
<pre id="result"></pre>
<script>
  const mu = window.mu.createClient();
  document.getElementById('load').onclick = async () => {
    try {
      const news = await mu.news.headlines({});
      document.getElementById('result').textContent =
        JSON.stringify(news, null, 2);
    } catch (error) {
      document.getElementById('result').textContent = error.message;
    }
  };
</script>`;
export function SDKGuide() {
 return <article className="space-y-6">
  <PageHeading title="App SDK" />
  <p>Build a small HTML app that uses Micro’s services. The platform provides the SDK when the app runs.</p>
  <section className="space-y-3"><h2 className="text-lg font-medium">Get started</h2><ol className="list-decimal space-y-2 pl-5"><li>Create an app and open its HTML editor.</li><li>Add your HTML, styles and JavaScript. Create a service client with <code>window.mu.createClient()</code>.</li><li>Save and open the app to try it with your account.</li></ol><Button asChild><a href="/apps/new">Create app</a></Button></section>
  <section className="space-y-3"><h2 className="text-lg font-medium">Example: load news headlines</h2><pre className="overflow-x-auto rounded-md bg-muted p-4 text-sm"><code>{example}</code></pre><p>Use <code>textContent</code> to display untrusted results as text. Handle rejected requests so people can see when an action fails.</p></section>
  <section className="space-y-3"><h2 className="text-lg font-medium">Service methods</h2><p><code>await mu.services()</code> lists the services available to the app. On your service client, call a method with <code>{"await mu.news.headlines({})"}</code>.</p><p><a className="underline" href="/apps/services.js">JavaScript SDK</a> · <a className="underline" href="/apps/services.d.ts">TypeScript definitions</a></p><p>The service reference lists method names and arguments. Access checks and usage charges still apply. A successful SDK call does not grant unrestricted access to the server.</p><Button asChild><a href="/services">Service reference</a></Button></section>
  <section className="space-y-3"><h2 className="text-lg font-medium">App storage</h2><p>Use <code>mu.store.set(key, value)</code>, <code>mu.store.get(key)</code> and <code>mu.store.del(key)</code> for the app’s key/value storage. Await each call and handle failures.</p></section>
  <section className="space-y-3"><h2 className="text-lg font-medium">Use the SDK outside Mu</h2><p>Import the service client and supply a Services token with access to the services you need. Keep the token in your server’s environment, outside browser code.</p><pre className="overflow-x-auto rounded-md bg-muted p-4 text-sm"><code>{`import { createClient } from './services.js';

const mu = createClient({
  baseURL: '${location.origin}',
  token: process.env.MU_SERVICES_TOKEN,
});

const headlines = await mu.news.headlines({});`}</code></pre><p>Download the JavaScript SDK and its TypeScript definitions together as <code>services.js</code> and <code>services.d.ts</code>. Named methods return the service’s structured response and reject failed requests.</p></section>
  <section className="space-y-3"><h2 className="text-lg font-medium">How it runs</h2><p>Apps run in an isolated frame. The injected SDK sends supported requests through a controlled bridge. Do not embed account tokens or add a script import for the SDK; it is supplied automatically.</p><p><a className="underline underline-offset-4" href="/apps/services.js">View the service SDK</a> if you need to inspect the standalone runtime.</p></section>
 </article>;
}
