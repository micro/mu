import type { Session } from "./protocol";
export interface Callback {
  code?: string;
  error?: string;
  state: string;
  callback: string;
}
declare global {
  interface Window {
    muDesktop?: {
      loadRuntime(): Promise<string | null>;
      saveRuntime(server: string): Promise<void>;
      redirect(): Promise<string>;
      authorize(url: string): Promise<void>;
      loadSession(): Promise<Session | null>;
      sessionPersistent(): Promise<boolean>;
      saveSession(session: Session | null): Promise<void>;
      onCallback(callback: (result: Callback) => void): () => void;
    };
  }
}
