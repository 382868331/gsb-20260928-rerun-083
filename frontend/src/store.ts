import { create } from 'zustand'
import type { Board, CheckError, CompareReport, NetStatus, Point, Sel, Tool } from './types'

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
  netStatus: NetStatus[]
  lastStatus: NetStatus[]
  variantId: string
  reportFor: { variantId: string; revision: number } | null
  compare: CompareReport | null
  status: string
  view: { zoom: number; panX: number; panY: number }
  pathError: CheckError | null

  setBoard: (b: Board) => void
  setTool: (t: Tool) => void
  set: (p: Partial<State>) => void
  commit: (fn: (b: Board) => void) => void
  undo: () => void
  redo: () => void
  setView: (v: { zoom: number; panX: number; panY: number }) => void
  setReport: (r: { errors: CheckError[]; netStatus: NetStatus[] }, remember: boolean) => void
  setVariant: (id: string) => void
}

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
  netStatus: [],
  lastStatus: [],
  variantId: '',
  reportFor: null,
  compare: null,
  status: '',
  pathError: null,
  view: { zoom: 1, panX: 60, panY: 40 },

  setBoard: (b) =>
    set({
      board: b,
      past: [],
      future: [],
      errors: [],
      netStatus: [],
      lastStatus: [],
      variantId: '',
      reportFor: null,
      compare: null,
      pathError: null,
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
    set({ board: next, past: [...past, board], future: [], errors: [], pathError: null })
  },
  undo: () => {
    const { board, past, future } = get()
    if (!board || past.length === 0) return
    set({
      board: past[past.length - 1],
      past: past.slice(0, -1),
      future: [board, ...future],
      errors: [], pathError: null,
      selected: null,
      draft: [],
    })
  },
  redo: () => {
    const { board, past, future } = get()
    if (!board || future.length === 0) return
    set({
      board: future[0],
      past: [...past, board],
      future: future.slice(1),
      errors: [], pathError: null,
      selected: null,
      draft: [],
    })
  },
  setView: (v) => set({ view: v }),
  // remember=true snapshots the previous per-net state so the sidebar can
  // show which nets just broke / shorted / recovered after a local edit.
  setReport: (r, remember) =>
    set((st) => ({
      errors: r.errors,
      netStatus: r.netStatus,
      lastStatus: remember ? st.netStatus : st.lastStatus,
      pathError: null,
    })),
  // Switching variants clears the previous verdict so the next render
  // can never show one variant's conclusions under another's name; the
  // sidebar effect immediately re-checks the newly selected variant.
  setVariant: (id) =>
    set({ variantId: id, errors: [], netStatus: [], lastStatus: [], reportFor: null, compare: null, pathError: null }),
}))
