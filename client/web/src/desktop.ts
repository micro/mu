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
      redirect(): Promise<string>;
      authorize(url: string): Promise<void>;
      loadSession(): Promise<Session | null>;
      saveSession(session: Session | null): Promise<void>;
      onCallback(callback: (result: Callback) => void): () => void;
    };
  }
}
