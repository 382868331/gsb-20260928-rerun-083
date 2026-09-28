import type { Board, DRCReport } from "./types";

async function req<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    throw new Error(`${res.status} ${await res.text()}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  listBoards: () => req<Board[]>("/api/boards"),
  getBoard: (id: string) => req<Board>(`/api/boards/${id}`),
  createBoard: (b: Board) =>
    req<Board>("/api/boards", { method: "POST", body: JSON.stringify(b) }),
  saveBoard: (id: string, b: Board) =>
    req<Board>(`/api/boards/${id}`, { method: "PUT", body: JSON.stringify(b) }),
  undo: (id: string) => req<Board>(`/api/boards/${id}/undo`, { method: "POST" }),
  redo: (id: string) => req<Board>(`/api/boards/${id}/redo`, { method: "POST" }),
  drc: (id: string, minClearanceMm: number) =>
    req<DRCReport>(`/api/boards/${id}/drc`, {
      method: "POST",
      body: JSON.stringify({ minClearanceMm }),
    }),
  importBoard: (b: Board, overwrite: boolean) =>
    req<Board>(`/api/boards/${b.id}/import?overwrite=${overwrite}`, {
      method: "POST",
      body: JSON.stringify(b),
    }),
};

