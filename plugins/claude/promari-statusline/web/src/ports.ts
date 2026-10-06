import type { Snapshot } from "./model.ts";

// 利用側が必要とする小さな契約だけを定義し、ブラウザの実装には依存しない。
export type Dispose = () => void;

export interface SnapshotRepository {
  load(signal: AbortSignal): Promise<Snapshot>;
}

export interface Scheduler {
  after(milliseconds: number, action: () => void): Dispose;
}

export interface Navigation {
  current(): string;
  go(hash: string): void;
  subscribe(listener: (hash: string) => void): Dispose;
}

export interface Visibility {
  current(): boolean;
  subscribe(listener: (visible: boolean) => void): Dispose;
}

export interface Fullscreen {
  current(): boolean;
  toggle(): Promise<void>;
  subscribe(listener: (active: boolean) => void): Dispose;
}
