export class ProtocolError extends Error {
    status;
    thread;
    submission;
    operation;
    constructor(message, status = 0, thread = "") {
        super(message);
        this.status = status;
        this.thread = thread;
    }
}
export function runtimeOrigin(value) {
    const u = new URL(value);
    if (u.username ||
        u.password ||
        u.search ||
        u.hash ||
        !["", "/"].includes(u.pathname) ||
        (u.protocol !== "https:" &&
            !(u.protocol === "http:" &&
                ["localhost", "127.0.0.1", "[::1]"].includes(u.hostname))))
        throw new ProtocolError("Use an HTTPS origin, or HTTP on loopback, without credentials or a path.");
    return u.origin;
}
// Generic MUCP transport. No assistant contracts, cookies by default, or retries.
export class Client {
    options;
    server;
    constructor(options) {
        this.options = options;
        this.server = runtimeOrigin(options.server);
        if (options.session &&
            (options.token || runtimeOrigin(options.session.origin) !== this.server))
            throw new ProtocolError("Session credentials require the same origin and no bearer token.");
        if (options.timeout !== undefined &&
            (!Number.isFinite(options.timeout) || options.timeout <= 0))
            throw new ProtocolError("Timeout must be positive.");
    }
    discover() {
        return this.exchange("GET");
    }
    call(service, endpoint, body = {}) {
        return this.exchange("POST", service, endpoint, body);
    }
    async exchange(method, service = "", endpoint = "", body) {
        const id = crypto.randomUUID(), headers = new Headers({ "Micro-Protocol": "mucp/1" });
        if (method === "POST") {
            headers.set("Micro-Service", service);
            headers.set("Micro-Endpoint", endpoint);
            headers.set("Micro-Id", id);
            headers.set("Content-Type", "application/json");
        }
        const token = typeof this.options.token === "function"
            ? await this.options.token()
            : this.options.token;
        if (token) {
            if (!/^[\x21-\x7e]+$/.test(token))
                throw new ProtocolError("Invalid token characters.");
            headers.set("Authorization", "Bearer " + token);
        }
        if (this.options.session)
            headers.set("X-CSRF-Token", await this.options.session.csrf());
        const payload = method === "POST" ? JSON.stringify(body) : undefined;
        if (payload && new TextEncoder().encode(payload).length > 65536)
            throw new ProtocolError("Request exceeds 65536 bytes.");
        let response;
        try {
            response = await (this.options.fetch || globalThis.fetch)(this.server + "/", {
                method,
                headers,
                body: payload,
                credentials: this.options.session ? "same-origin" : "omit",
                redirect: "error",
                cache: "no-store",
                signal: AbortSignal.timeout(this.options.timeout ?? 20000),
            });
        }
        catch {
            throw new ProtocolError("Connection failed; the outcome may be unknown.");
        }
        const reader = response.body?.getReader();
        const chunks = [];
        let size = 0;
        if (reader)
            try {
                for (;;) {
                    const part = await reader.read();
                    if (part.done)
                        break;
                    size += part.value.length;
                    if (size > 4 * 1024 * 1024) {
                        await reader.cancel();
                        throw new ProtocolError("Response exceeds client limit.");
                    }
                    chunks.push(part.value);
                }
            }
            finally {
                reader.releaseLock();
            }
        const bytes = new Uint8Array(size);
        let offset = 0;
        for (const chunk of chunks) {
            bytes.set(chunk, offset);
            offset += chunk.length;
        }
        let data;
        try {
            data = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
        }
        catch {
            throw new ProtocolError("Invalid JSON response.", response.status);
        }
        if (!response.ok)
            throw new ProtocolError(typeof data?.error === "string"
                ? data.error
                : data?.error?.message || "Request failed.", response.status, response.headers.get("Micro-Thread") || "");
        if (response.headers.get("Micro-Protocol") !== "mucp/1" ||
            (method === "POST" &&
                ["Micro-Id", "Micro-Service", "Micro-Endpoint"].some((key) => response.headers.get(key) !== headers.get(key))) ||
            response.headers.get("Content-Type")?.split(";")[0].trim() !==
                "application/json" ||
            !data ||
            typeof data !== "object" ||
            Array.isArray(data))
            throw new ProtocolError("Invalid MUCP response.");
        return data;
    }
}
export class Mu {
    client;
    constructor(options) {
        this.client = options instanceof Client ? options : new Client(options);
    }
    call(service, endpoint, body = {}) {
        return this.client.call(service, endpoint, body);
    }
    discover() {
        return this.client.discover();
    }
    agents() {
        return this.call("assistant", "Assistant.ListAgents");
    }
    threads(request = {}) {
        return this.call("assistant", "Assistant.ListThreads", request);
    }
    read(thread, pageToken = "") {
        return this.call("assistant", "Assistant.ReadThread", { thread, pageToken });
    }
    home() {
        return this.call("assistant", "Home.Overview");
    }
    inbox(pageToken = "") {
        return this.call("assistant", "Inbox.List", { pageToken });
    }
    services(pageToken = "") {
        return this.call("assistant", "Services.List", { pageToken });
    }
    prepare(prompt, options = {}) {
        if (!prompt.trim() || new TextEncoder().encode(prompt).length > 8000)
            throw new ProtocolError("Prompt must contain 1–8000 UTF-8 bytes.");
        return {
            messageId: crypto.randomUUID(),
            request: {
                agentId: "",
                thread: "",
                ...options,
                prompt,
                mode: "ACCESS_MODE_PERSONAL",
            },
        };
    }
    async submit(submission) {
        try {
            return await this.call("assistant", "Assistant.Submit", submission);
        }
        catch (error) {
            const e = error instanceof ProtocolError
                ? error
                : new ProtocolError("Submission failed; the outcome may be unknown.");
            e.submission = submission;
            throw e;
        }
    }
    status(id) {
        return this.call("assistant", "Assistant.ReadOperation", { id });
    }
    async wait(operation, timeout = 120000) {
        if (!Number.isFinite(timeout) || timeout <= 0)
            throw new ProtocolError("Timeout must be positive.");
        const deadline = Date.now() + timeout, id = operation.id, thread = operation.thread;
        let delay = 1000;
        try {
            for (;;) {
                if (!id ||
                    !thread ||
                    operation.id !== id ||
                    operation.thread !== thread)
                    throw new ProtocolError("Operation identity changed.");
                if (operation.state === "OPERATION_STATE_SUCCEEDED" && operation.result)
                    return operation.result;
                if (!["OPERATION_STATE_QUEUED", "OPERATION_STATE_RUNNING"].includes(operation.state))
                    throw new ProtocolError(operation.error ||
                        "Operation stopped or returned an unknown state.", 500, thread);
                const remaining = deadline - Date.now();
                if (remaining <= 0)
                    throw new ProtocolError("Request is still running; recover it using its operation ID.", 504, thread);
                await new Promise((resolve) => setTimeout(resolve, Math.min(delay, remaining)));
                if (Date.now() >= deadline)
                    throw new ProtocolError("Request is still running; recover it using its operation ID.", 504, thread);
                const next = await this.status(id);
                if (next.id !== id || next.thread !== thread)
                    throw new ProtocolError("Operation identity changed.");
                operation = next;
                delay = Math.min(delay * 2, 30000);
            }
        }
        catch (error) {
            const e = error instanceof ProtocolError
                ? error
                : new ProtocolError("Recovery interrupted.");
            e.thread = thread;
            e.operation = operation;
            throw e;
        }
    }
    async ask(prompt, options = {}) {
        const submission = this.prepare(prompt, options);
        try {
            return await this.wait(await this.submit(submission));
        }
        catch (error) {
            if (error instanceof ProtocolError)
                error.submission = submission;
            throw error;
        }
    }
}
