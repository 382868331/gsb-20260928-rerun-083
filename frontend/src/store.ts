import { create } from 'zustand'
import type { Board, CheckError, Point, Sel, Tool } from './types'

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
  status: string
  view: { zoom: number; panX: number; panY: number }

  setBoard: (b: Board) => void
  setTool: (t: Tool) => void
  set: (p: Partial<State>) => void
  commit: (fn: (b: Board) => void) => void
  undo: () => void
  redo: () => void
  setView: (v: { zoom: number; panX: number; panY: number }) => void
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
  status: '',
  view: { zoom: 1, panX: 60, panY: 40 },

  setBoard: (b) =>
    set({
      board: b,
      past: [],
      future: [],
      errors: [],
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
  },
  setView: (v) => set({ view: v }),
}))
