import type { Answer, Operation, Operations, Request, Submission } from "./types.js";
export type * from "./types.js";
export declare class ProtocolError extends Error {
    status: number;
    thread: string;
    submission?: Submission;
    operation?: Operation;
    constructor(message: string, status?: number, thread?: string);
}
export declare function runtimeOrigin(value: string): string;
export interface Options {
    server: string;
    token?: string | (() => string | Promise<string>);
    timeout?: number;
    session?: {
        origin: string;
        csrf: () => string | Promise<string>;
    };
    fetch?: typeof globalThis.fetch;
}
export declare class Client {
    private options;
    readonly server: string;
    constructor(options: Options);
    discover(): Promise<Record<string, unknown>>;
    call<T = Record<string, unknown>>(service: string, endpoint: string, body?: unknown): Promise<T>;
    private exchange;
}
export declare class Mu {
    private readonly client;
    constructor(options: Options | Client);
    call<T = Record<string, unknown>>(service: string, endpoint: string, body?: unknown): Promise<T>;
    discover(): Promise<Record<string, unknown>>;
    agents(): Promise<{
        agents: import("./types.js").Agent[];
    }>;
    threads(request?: Operations["ListThreads"]["request"]): Promise<{
        threads: import("./types.js").Thread[];
        nextPageToken: string;
    }>;
    read(thread: string, pageToken?: string): Promise<{
        thread: import("./types.js").Thread;
        messages: import("./types.js").Message[];
        nextPageToken: string;
        pending: boolean;
    }>;
    home(): Promise<Record<string, unknown>>;
    inbox(pageToken?: string): Promise<Record<string, unknown>>;
    services(pageToken?: string): Promise<Record<string, unknown>>;
    private prepare;
    private submit;
    status(id: string): Promise<Operation>;
    wait(operation: Operation, timeout?: number): Promise<Answer>;
    ask(prompt: string, options?: Partial<Omit<Request, "prompt" | "mode">>): Promise<Answer>;
}
