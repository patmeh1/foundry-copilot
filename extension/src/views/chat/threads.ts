// threads.ts — persists chat threads in context.globalState. Tiny CRUD.
import * as vscode from 'vscode';

const KEY = 'foundryCopilot.threads';

export interface ChatMessage {
    role: 'user' | 'assistant' | 'system' | 'tool';
    content: string;
}

export interface ChatThread {
    id: string;
    title: string;
    createdAt: number;
    messages: ChatMessage[];
}

export class ThreadStore {
    constructor(private readonly context: vscode.ExtensionContext) {}

    list(): ChatThread[] {
        return this.context.globalState.get<ChatThread[]>(KEY, []);
    }

    get(id: string): ChatThread | undefined {
        return this.list().find((t) => t.id === id);
    }

    create(title: string): ChatThread {
        const t: ChatThread = { id: cryptoRandomId(), title, createdAt: Date.now(), messages: [] };
        const all = this.list();
        all.push(t);
        void this.context.globalState.update(KEY, all);
        return t;
    }

    update(t: ChatThread): void {
        const all = this.list();
        const i = all.findIndex((x) => x.id === t.id);
        if (i >= 0) {
            all[i] = t;
            void this.context.globalState.update(KEY, all);
        }
    }

    rename(id: string, title: string): void {
        const t = this.get(id);
        if (t) {
            t.title = title;
            this.update(t);
        }
    }

    delete(id: string): void {
        const all = this.list().filter((t) => t.id !== id);
        void this.context.globalState.update(KEY, all);
    }
}

function cryptoRandomId(): string {
    // crypto.randomUUID is in Node 14.17+ globally; engine is Node 20.
    const c = (globalThis as unknown as { crypto?: { randomUUID?: () => string } }).crypto;
    return c?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}
