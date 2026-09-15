import { renderToString } from "react-dom/server";
import { Layout } from "./components/layout";
import { Conversation } from "../../app/assistant";
import { PublicPage, publicTitles } from "./components/public-page";
export function render(path = "/") {
  if (publicTitles[path])
    return renderToString(
      <Layout account={null} title={publicTitles[path]} publicPage>
        <PublicPage path={path} />
      </Layout>,
    );
  return renderToString(
    <Layout account={null} title="Home" conversation>
      <Conversation state={{ account: null, csrf: "" }} />
    </Layout>,
  );
}
