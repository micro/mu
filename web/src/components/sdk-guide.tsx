import { Button } from "./ui/button";
import { PageHeading } from "./layout";
const example = `<h1>My notes</h1>
<button id="load">Load notes</button>
<pre id="result"></pre>
<script>
  document.getElementById('load').onclick = async () => {
    try {
      const notes = await mu.service('notes', 'list', {});
      document.getElementById('result').textContent =
        JSON.stringify(notes, null, 2);
    } catch (error) {
      document.getElementById('result').textContent = error.message;
    }
  };
</script>`;
export function SDKGuide() {
 return <article className="max-w-3xl space-y-6">
  <PageHeading title="App SDK" />
  <p>Build a small HTML app that uses Micro’s services. The platform provides the SDK when the app runs.</p>
  <section className="space-y-3"><h2 className="text-lg font-medium">Get started</h2><ol className="list-decimal space-y-2 pl-5"><li>Create an app and open its HTML editor.</li><li>Add your HTML, styles and JavaScript. Use the provided <code>mu</code> object to call services.</li><li>Save and open the app to try it with your account.</li></ol><Button asChild><a href="/apps/new">Create app</a></Button></section>
  <section className="space-y-3"><h2 className="text-lg font-medium">Example: load your notes</h2><pre className="overflow-x-auto rounded-md bg-muted p-4 text-sm"><code>{example}</code></pre><p>Use <code>textContent</code> to display untrusted results as text. Handle rejected requests so people can see when an action fails.</p></section>
  <section className="space-y-3"><h2 className="text-lg font-medium">Discover services</h2><p><code>await mu.services()</code> lists the services available to the app. Call a method with <code>await mu.service(name, method, arguments)</code>.</p><p>The service reference lists method names and arguments. Access checks and usage charges still apply. A successful SDK call does not grant unrestricted access to the server.</p><Button asChild><a href="/services">Service reference</a></Button></section>
  <section className="space-y-3"><h2 className="text-lg font-medium">App storage</h2><p>Use <code>mu.store.set(key, value)</code>, <code>mu.store.get(key)</code> and <code>mu.store.del(key)</code> for the app’s key/value storage. Await each call and handle failures.</p></section>
  <section className="space-y-3"><h2 className="text-lg font-medium">How it runs</h2><p>Apps run in an isolated frame. The injected SDK sends supported requests through a controlled bridge. Do not embed account tokens or add a script import for the SDK; it is supplied automatically.</p><p><a className="underline underline-offset-4" href="/apps/sdk.js">View the JavaScript asset</a> if you need to inspect the standalone runtime.</p></section>
 </article>;
}
