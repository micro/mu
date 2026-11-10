import { createRoot } from "react-dom/client";
import App from "./App";
import { finishSignIn } from "./auth";
import "../../../internal/app/html/mu.css";
const root = createRoot(document.getElementById("root")!);
// Exchange each authorization code once, before mounting the client.
import type { Callback } from "./desktop";
function start(callback?: Callback) {
  finishSignIn(callback)
    .then((session) =>
      root.render(<App key={callback?.state || "initial"} initial={session} />),
    )
    .catch((error) =>
      root.render(
        <App
          key="error"
          initial={null}
          error={error instanceof Error ? error.message : "Sign-in failed."}
        />,
      ),
    );
}
window.muDesktop?.onCallback(start);
start();
