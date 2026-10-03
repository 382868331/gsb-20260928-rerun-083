import type { Board, CheckReport, CompareReport, NetPathReport } from './types'

async function req<T>(url: string, init?: RequestInit): Promise<T> {
  const r = await fetch(url, init)
  if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || r.statusText)
  return r.json()
}

export const api = {
  getBoard: () => req<Board>('/api/board'),
  saveBoard: (b: Board) =>
    req<Board>('/api/board', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(b) }),
  checkBoard: (b: Board) =>
    req<CheckReport>('/api/board/check', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(b) }),
  checkVariant: (b: Board, variantId: string) =>
    req<CheckReport>('/api/board/check', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ board: b, variantId }) }),
  compareVariants: (b: Board) =>
    req<CompareReport>('/api/board/variants/compare', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(b) }),
  netPath: (b: Board, variantId: string, netId: string) =>
    req<NetPathReport>('/api/board/netpath', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ board: b, variantId, netId }) }),
  importBoard: (b: unknown) =>
    req<Board>('/api/board/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(b) }),
  resetBoard: () => req<Board>('/api/board/reset', { method: 'POST' }),
}
