export interface Agent {
    id: string;
    name: string;
    slug: string;
    description: string;
}
export interface Thread {
    id: string;
    agentId: string;
    subject: string;
    started?: string;
    updated?: string;
}
export interface Content {
    fallback: string;
    source?: {
        title: string;
        url: string;
        summary: string;
    };
    resource?: {
        kind: string;
        id: string;
        title: string;
        url: string;
        summary: string;
    };
    video?: {
        provider: string;
        id: string;
        url: string;
        title: string;
        audioOnlyAllowed: boolean;
    };
    audio?: {
        url: string;
        title: string;
        mimeType: string;
    };
    image?: {
        url: string;
        description: string;
    };
    route?: {
        shape: {
            latitude: number;
            longitude: number;
        }[];
        steps: string[];
    };
}
export interface Message {
    id?: string;
    role: string;
    text: string;
    content?: Content[];
    at?: string;
}
export interface Operations {
    ListAgents: {
        request: Record<string, never>;
        response: {
            agents: Agent[];
        };
    };
    ListThreads: {
        request: {
            agentId?: string;
            pageSize?: number;
            pageToken?: string;
        };
        response: {
            threads: Thread[];
            nextPageToken: string;
        };
    };
    ReadThread: {
        request: {
            thread: string;
            pageSize?: number;
            pageToken?: string;
        };
        response: {
            thread: Thread;
            messages: Message[];
            nextPageToken: string;
            pending: boolean;
        };
    };
    Ask: {
        request: {
            agentId: string;
            thread: string;
            prompt: string;
            mode: "ACCESS_MODE_PERSONAL";
            voice?: boolean;
        };
        response: {
            thread: string;
            agentId: string;
            text: string;
            content: Content[];
        };
    };
}
export type Answer = Operations["Ask"]["response"];
export type Request = Operations["Ask"]["request"];
export interface Submission {
    messageId: string;
    request: Request;
}
export interface Operation {
    id: string;
    thread: string;
    agentId?: string;
    state: string;
    result?: Answer;
    error?: string;
}
