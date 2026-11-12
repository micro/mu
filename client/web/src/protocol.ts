import { renewSession } from "./auth";
import {
  Client,
  Mu,
  ProtocolError,
  runtimeOrigin,
  type Operations,
} from "../../typescript/src/index";
export { ProtocolError, runtimeOrigin };
export type {
  Agent,
  Thread,
  Content,
  Message,
} from "../../typescript/src/index";
export interface Session {
  client?: string;
  refresh?: string;
  server: string;
  token: string;
  expires: number;
  cookie?: boolean;
}

function client(session: Session): Client {
  return new Client({
    server: session.server,
    ...(session.cookie
      ? {
          session: {
            origin: location.origin,
            csrf: async () => {
              await renewSession(session);
              return decodeURIComponent(
                document.cookie
                  .split("; ")
                  .find((c) => c.startsWith("csrf_token="))
                  ?.slice(11) || "",
              );
            },
          },
        }
      : {
          token: async () => {
            await renewSession(session);
            return session.token;
          },
        }),
  });
}
export async function call<K extends keyof Operations>(
  session: Session,
  operation: K,
  body: Operations[K]["request"],
): Promise<Operations[K]["response"]> {
  if (operation === "Ask") {
    const request = body as Operations["Ask"]["request"];
    return new Mu(client(session)).ask(request.prompt, request) as Promise<
      Operations[K]["response"]
    >;
  }
  return client(session).call("assistant", "Assistant." + operation, body);
}
export async function invoke<T>(
  session: Session,
  service: string,
  endpoint: string,
  body: unknown,
): Promise<T> {
  return client(session).call<T>(service, endpoint, body);
}
