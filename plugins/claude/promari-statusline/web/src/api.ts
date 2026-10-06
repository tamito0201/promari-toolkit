import type { Snapshot } from "./model.ts";
import type { SnapshotRepository } from "./ports.ts";
import { parseSnapshot } from "./snapshot.ts";

const REQUEST_TIMEOUT_MS = 25_000;

// HTTP の詳細を ViewModel から隔離する。テストでは fetch 自体も差し替えられる。
export function createSnapshotRepository(
  request: typeof fetch,
): SnapshotRepository {
  return {
    async load(signal): Promise<Snapshot> {
      const response = await request("/api/snapshot", {
        cache: "no-store",
        signal: AbortSignal.any([
          signal,
          AbortSignal.timeout(REQUEST_TIMEOUT_MS),
        ]),
      });
      if (!response.ok)
        throw new Error(`/api/snapshot answered ${response.status}`);
      const body: unknown = await response.json();
      return parseSnapshot(body);
    },
  };
}
