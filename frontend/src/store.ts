import { create } from 'zustand'
import type { Board, CheckError, CompareReport, NetStatus, Point, Sel, Tool } from './types'

export const MM = 1_000_000

export const uid = (p: string) =>
  p + '-' + (crypto.randomUUID ? crypto.randomUUID().slice(0, 8) : Math.random().toString(36).slice(2, 10))

const clone = (b: Board): Board => JSON.parse(JSON.stringify(b))

// compareKeyOf binds a compare report to its exact basis: the board
// revision plus every variant's own assembly selection. Moving a via,
// toggling a jumper or undoing an edit changes the board identity, so
// a stored report can be flagged stale instead of silently standing in
// for the new geometry.
export const compareKeyOf = (b: Board): string =>
  b.revision + '|' + b.variants.map((v) => v.id + ':' + [...v.dnp].sort().join(',') + '+' + [...v.jumpers].sort().join(',')).join(';') + '|' + fingerprint(b)

// Geometry is not revision-tracked client-side (unsaved edits keep the
// same revision number), so fingerprint the conductive geometry too.
const fingerprint = (b: Board): string =>
  JSON.stringify([b.pads, b.traces, b.vias, b.jumpers])

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
  compareBoard: Board | null
  compareKey: string | null
  compareFrozen: boolean
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
  setCompare: (rep: CompareReport, basis: Board) => void
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
  compareBoard: null,
  compareKey: null,
  compareFrozen: false,
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
      compareBoard: null,
      compareKey: null,
      compareFrozen: false,
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
      errors: r.errors ?? [],
      netStatus: r.netStatus ?? [],
      lastStatus: remember ? st.netStatus : st.lastStatus,
      pathError: null,
    })),
  // Switching variants clears the previous single-variant verdict so the
  // next render can never show one variant's conclusions under another's
  // name; the compare report stays — it names every variant explicitly
  // and is flagged stale by compareKey once the board itself changes.
  setVariant: (id) =>
    set({ variantId: id, errors: [], netStatus: [], lastStatus: [], reportFor: null, pathError: null }),
  // Freezes the exact board the compare ran against, so later edits can
  // be detected and fault paths can still be traced on the old revision.
  setCompare: (rep, basis) =>
    set({ compare: rep, compareBoard: clone(basis), compareKey: compareKeyOf(basis), compareFrozen: false }),
}))
