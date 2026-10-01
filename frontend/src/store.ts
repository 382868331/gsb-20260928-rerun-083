import { create } from 'zustand'
import type { Board, CheckError, NetStatus, Point, Sel, Tool } from './types'
import { api } from './api'

export const MM = 1_000_000

export const uid = (p: string) =>
  p + '-' + (crypto.randomUUID ? crypto.randomUUID().slice(0, 8) : Math.random().toString(36).slice(2, 10))

const clone = (b: Board): Board => JSON.parse(JSON.stringify(b))

interface State {
  board: Board | null
  past: Board[]
  future: Board[]
  tool: Tool
  layer: string
  netId: string
  traceWidth: number
  padShape: 'circle' | 'rect'
  padSize: number
  viaLayers: string[]
  selected: Sel | null
  draft: Point[]
  errors: CheckError[]
  nets: NetStatus[]
  status: string
  view: { zoom: number; panX: number; panY: number }

  setBoard: (b: Board) => void
  setTool: (t: Tool) => void
  set: (p: Partial<State>) => void
  commit: (fn: (b: Board) => void) => void
  undo: () => void
  redo: () => void
  setView: (v: { zoom: number; panX: number; panY: number }) => void
  recheck: (reason?: string) => Promise<void>
}

const NET_LABEL: Record<NetStatus['status'], string> = {
  connected: '连通',
  open: '断开',
  short: '短接',
}

// diffNets summarizes which nets changed state (opened / shorted /
// reconnected) and which stayed connected, for the instant-recheck
// status line shown after local edits like moving or deleting a via.
function diffNets(prev: NetStatus[], next: NetStatus[]): string {
  const before = new Map(prev.map((n) => [n.id, n]))
  const changed: string[] = []
  const stillOk: string[] = []
  for (const n of next) {
    const p = before.get(n.id)
    if (!p || p.status === n.status) {
      if (n.status === 'connected') stillOk.push(n.name)
      continue
    }
    changed.push(n.name + ' ' + NET_LABEL[p.status] + '→' + NET_LABEL[n.status])
  }
  const parts: string[] = []
  if (changed.length) parts.push(changed.join('; '))
  if (stillOk.length) parts.push(stillOk.join('、') + ' 仍连通')
  return parts.join('; ') || '无网络'
}

let recheckSeq = 0

export const useStore = create<State>((set, get) => ({
  board: null,
  past: [],
  future: [],
  tool: 'select',
  layer: 'top',
  netId: '',
  traceWidth: 0.5 * MM,
  padShape: 'circle',
  padSize: 1.5 * MM,
  viaLayers: ['top', 'bottom'],
  selected: null,
  draft: [],
  errors: [],
  nets: [],
  status: '',
  view: { zoom: 1, panX: 60, panY: 40 },

  setBoard: (b) =>
    set({
      board: b,
      past: [],
      future: [],
      errors: [],
      nets: [],
      selected: null,
      draft: [],
      netId: b.nets[0]?.id ?? '',
      layer: b.layers[0] ?? 'top',
      viaLayers: b.layers.slice(),
    }),
  setTool: (t) => set({ tool: t, draft: [], selected: null }),
  set: (p) => set(p),
  commit: (fn) => {
    const { board, past } = get()
    if (!board) return
    const next = clone(board)
    fn(next)
    set({ board: next, past: [...past, board], future: [], errors: [] })
    void get().recheck()
  },
  undo: () => {
    const { board, past, future } = get()
    if (!board || past.length === 0) return
    set({
      board: past[past.length - 1],
      past: past.slice(0, -1),
      future: [board, ...future],
      errors: [],
      selected: null,
      draft: [],
    })
    void get().recheck('撤销')
  },
  redo: () => {
    const { board, past, future } = get()
    if (!board || future.length === 0) return
    set({
      board: future[0],
      past: [...past, board],
      future: future.slice(1),
      errors: [],
      selected: null,
      draft: [],
    })
    void get().recheck('重做')
  },
  setView: (v) => set({ view: v }),
  // Instant recheck after a local edit: posts the current (possibly
  // unsaved) board and reports which nets opened, shorted, or stayed
  // connected relative to the previous known state.
  recheck: async (reason) => {
    const { board, nets } = get()
    if (!board) return
    const seq = ++recheckSeq
    try {
      const r = await api.checkBoard(board)
      if (seq !== recheckSeq) return // a newer edit superseded this one
      const diff = diffNets(nets, r.nets)
      const head = reason ? reason + '后复核: ' : '即时复核: '
      set({
        errors: r.errors,
        nets: r.nets,
        status: head + diff + (r.errors.length ? ' (' + r.errors.length + ' 个问题)' : ' (无问题)'),
      })
    } catch (e) {
      set({ status: '复核失败: ' + (e as Error).message })
    }
  },
}))
