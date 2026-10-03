import { create } from 'zustand'
import type { Board, CheckError, CheckReport, CompareResult, NetStatus, Point, Sel, Tool } from './types'

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
  status: string
  warnings: string[]
  view: { zoom: number; panX: number; panY: number }
  pathError: CheckError | null
  // Currently selected assembly variant ('' = base board, all mounted).
  asmId: string
  // Per-variant check results, keyed by assembly id. Each variant keeps
  // its own conclusions; a board edit invalidates all of them.
  reports: Record<string, CheckReport>
  compare: CompareResult | null

  setBoard: (b: Board) => void
  setTool: (t: Tool) => void
  set: (p: Partial<State>) => void
  commit: (fn: (b: Board) => void) => void
  undo: () => void
  redo: () => void
  setView: (v: { zoom: number; panX: number; panY: number }) => void
  setReport: (r: CheckReport, remember: boolean) => void
  setAssembly: (id: string) => void
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
  status: '',
  warnings: [],
  pathError: null,
  asmId: '',
  reports: {},
  compare: null,
  view: { zoom: 1, panX: 60, panY: 40 },

  setBoard: (b) =>
    set({
      board: b,
      past: [],
      future: [],
      errors: [],
      netStatus: [],
      lastStatus: [],
      warnings: [],
      pathError: null,
      selected: null,
      draft: [],
      asmId: '',
      reports: {},
      compare: null,
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
    set({ board: next, past: [...past, board], future: [], errors: [], pathError: null, reports: {}, compare: null })
  },
  undo: () => {
    const { board, past, future } = get()
    if (!board || past.length === 0) return
    set({
      board: past[past.length - 1],
      past: past.slice(0, -1),
      future: [board, ...future],
      errors: [], pathError: null, reports: {}, compare: null,
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
      errors: [], pathError: null, reports: {}, compare: null,
      selected: null,
      draft: [],
    })
  },
  setView: (v) => set({ view: v }),
  // remember=true snapshots the previous per-net state so the sidebar can
  // show which nets just broke / shorted / recovered after a local edit.
  setReport: (r, remember) =>
    set((st) => {
      const asmId = r.assembly ?? ''
      const next = {
        errors: r.errors,
        netStatus: r.netStatus,
        lastStatus: remember ? st.netStatus : st.lastStatus,
        pathError: null,
        reports: { ...st.reports, [asmId]: r },
      }
      // A result arriving for a non-selected variant is cached only, so
      // its conclusions never leak into the current view.
      if (asmId !== st.asmId) {
        return { reports: next.reports }
      }
      return next
    }),
  // Switching variants restores that variant's own cached verdicts (or
  // blanks the view until its check lands) — never reuses the previous
  // variant's conclusions.
  setAssembly: (id) =>
    set((st) => {
      const cached = st.reports[id]
      return {
        asmId: id,
        errors: cached?.errors ?? [],
        netStatus: cached?.netStatus ?? [],
        lastStatus: [],
        pathError: null,
      }
    }),
}))
