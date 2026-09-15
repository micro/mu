import { renderToString } from "react-dom/server";
import { Layout } from "./components/layout";
import { Conversation } from "./components/conversation";
export function render() {
  return renderToString(
    <Layout account={null} title="Home" conversation>
      <Conversation state={{ account: null, csrf: "" }} />
    </Layout>,
  );
}
