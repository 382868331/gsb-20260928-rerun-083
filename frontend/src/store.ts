import { create } from "zustand";
import type { Board, DRCReport, Pad, Point, Trace, Via } from "./types";
import { api } from "./api";

export type Tool = "select" | "pad" | "trace" | "via" | "outline";

export const LAYERS = ["top", "mid", "bottom"] as const;
export type Layer = (typeof LAYERS)[number];

interface State {
  board: Board | null;
  boards: Board[];
  report: DRCReport | null;
  tool: Tool;
  layer: Layer;
  traceWidth: number;
  padRadius: number;
  viaRadius: number;
  minClearance: number;
  draftTrace: Point[];
  selectedId: string | null;
  focusedIssue: number | null;
  status: string;
  dirty: boolean;
  canUndo: boolean;
  canRedo: boolean;
  history: LocalHistory;

  setTool: (t: Tool) => void;
  setLayer: (l: Layer) => void;
  setParam: (p: { traceWidth?: number; padRadius?: number; viaRadius?: number; minClearance?: number }) => void;
  refreshList: () => Promise<void>;
  load: (id: string) => Promise<void>;
  create: () => Promise<void>;
  mutate: (fn: (b: Board) => Board, opts?: { select?: string | null; pushDraft?: Point }) => void;
  addPad: (p: Point) => void;
  addVia: (p: Point) => void;
  pushDraftPoint: (p: Point) => void;
  cancelDraft: () => void;
  finishTrace: () => void;
  selectFeature: (id: string | null) => void;
  setSelectedNet: (net: string) => void;
  deleteSelected: () => void;
  setOutline: (patch: Partial<Board["outline"]>) => void;
  save: () => Promise<void>;
  undo: () => Promise<void>;
  redo: () => Promise<void>;
  runDrc: () => Promise<void>;
  focusIssue: (i: number | null) => void;
  exportFile: () => void;
  importFile: (file: File, overwrite: boolean) => Promise<void>;
}

let idCounter = 1;
const nextId = (kind: string) =>
  `${kind}-${Date.now().toString(36)}-${(idCounter++).toString(36)}`;

// Local edit snapshots cover unsaved work. The server keeps its own stack for
// saved revisions, so undo works whether or not the board has been saved.
interface LocalHistory {
  past: Board[];
  future: Board[];
}

export const emptyBoard = (): Board => ({
  id: "",
  name: "未命名电路板",
  outline: { x: 0, y: 0, width: 80, height: 60 },
  components: [
    { id: "comp-1", reference: "REF1", footprint: "GENERIC" },
  ],
  pads: [],
  traces: [],
  vias: [],
});

export const useStore = create<State>((set, get) => ({
  board: null,
  boards: [],
  report: null,
  tool: "select",
  layer: "top",
  traceWidth: 0.4,
  padRadius: 0.8,
  viaRadius: 0.5,
  minClearance: 0.3,
  draftTrace: [],
  selectedId: null,
  focusedIssue: null,
  status: "就绪",
  dirty: false,
  canUndo: false,
  canRedo: false,
  history: { past: [], future: [] },

  setTool: (tool) => set({ tool, draftTrace: [] }),
  setLayer: (layer) => set({ layer }),
  setParam: (p) => set(p),

  refreshList: async () => {
    try {
      set({ boards: await api.listBoards() });
    } catch (e) {
      set({ status: `无法连接后端: ${String(e)}` });
    }
  },

  load: async (id) => {
    const board = await api.getBoard(id);
    set({
      board,
      report: null,
      draftTrace: [],
      selectedId: null,
      focusedIssue: null,
      dirty: false,
      canUndo: true,
      history: { past: [], future: [] },
      status: `已加载 ${board.name}`,
    });
  },

  create: async () => {
    const board = await api.createBoard(emptyBoard());
    await get().refreshList();
    await get().load(board.id);
  },

  mutate: (fn, opts) => {
    const cur = get().board;
    if (!cur) return;
    const past = [...get().history.past, structuredClone(cur)].slice(-200);
    set({
      board: fn(structuredClone(cur)),
      dirty: true,
      selectedId: opts?.select !== undefined ? opts.select : get().selectedId,
      history: { past, future: [] },
      canUndo: true,
      canRedo: false,
      status: "已修改（未保存）",
    });
  },

  addPad: (p) => {
    const { padRadius, layer, board } = get();
    if (!board) return;
    const componentId = board.components[0]?.id ?? "";
    const pad: Pad = {
      id: nextId("pad"),
      componentId,
      name: String(board.pads.length + 1),
      x: Number(p.x.toFixed(4)),
      y: Number(p.y.toFixed(4)),
      radius: padRadius,
      layers: [layer],
    };
    get().mutate((b) => {
      b.pads.push(pad);
      return b;
    }, { select: pad.id });
  },

  addVia: (p) => {
    const { viaRadius } = get();
    const via: Via = {
      id: nextId("via"),
      x: Number(p.x.toFixed(4)),
      y: Number(p.y.toFixed(4)),
      radius: viaRadius,
      layers: ["top", "bottom"],
    };
    get().mutate((b) => {
      b.vias.push(via);
      return b;
    }, { select: via.id });
  },

  pushDraftPoint: (p) =>
    set((s) => ({ draftTrace: [...s.draftTrace, p] })),
  cancelDraft: () => set({ draftTrace: [] }),

  finishTrace: () => {
    const { draftTrace, layer, traceWidth } = get();
    if (draftTrace.length < 2) {
      set({ draftTrace: [] });
      return;
    }
    const trace: Trace = {
      id: nextId("trace"),
      layer,
      width: traceWidth,
      points: draftTrace.map((q) => ({
        x: Number(q.x.toFixed(4)),
        y: Number(q.y.toFixed(4)),
      })),
    };
    get().mutate((b) => {
      b.traces.push(trace);
      return b;
    }, { select: trace.id });
    set({ draftTrace: [] });
  },

  selectFeature: (id) => set({ selectedId: id }),

  setSelectedNet: (net) => {
    const { selectedId } = get();
    if (!selectedId) return;
    get().mutate((b) => {
      const setNet = (v: { net?: string }) => {
        if (net) v.net = net;
        else delete v.net;
      };
      const pad = b.pads.find((q) => q.id === selectedId);
      if (pad) setNet(pad);
      const trace = b.traces.find((q) => q.id === selectedId);
      if (trace) setNet(trace);
      const via = b.vias.find((q) => q.id === selectedId);
      if (via) setNet(via);
      return b;
    });
  },

  deleteSelected: () => {
    const id = get().selectedId;
    if (!id) return;
    get().mutate((b) => {
      b.pads = b.pads.filter((q) => q.id !== id);
      b.traces = b.traces.filter((q) => q.id !== id);
      b.vias = b.vias.filter((q) => q.id !== id);
      return b;
    }, { select: null });
  },

  setOutline: (patch) => {
    get().mutate((b) => {
      b.outline = { ...b.outline, ...patch };
      return b;
    });
  },

  save: async () => {
    const b = get().board;
    if (!b) return;
    await api.saveBoard(b.id, b);
    // Saved revisions move to the server undo stack; local snapshots reset.
    set({
      dirty: false,
      canUndo: true,
      canRedo: false,
      history: { past: [], future: [] },
      status: `已保存 ${new Date().toLocaleTimeString()}`,
    });
    await get().runDrc();
  },

  undo: async () => {
    const b = get().board;
    if (!b) return;
    const { past, future } = get().history;
    if (past.length > 0) {
      const prev = past[past.length - 1];
      set({
        board: prev,
        history: { past: past.slice(0, -1), future: [structuredClone(b), ...future] },
        canRedo: true,
        dirty: true,
        selectedId: null,
        status: "已撤销本地修改",
      });
      return;
    }
    try {
      const prev = await api.undo(b.id);
      set({ board: prev, dirty: false, report: null, selectedId: null, canUndo: true, status: "已撤销" });
      await get().runDrc();
    } catch {
      set({ status: "没有可撤销的历史" });
    }
  },

  redo: async () => {
    const b = get().board;
    if (!b) return;
    const { past, future } = get().history;
    if (future.length > 0) {
      const next = future[0];
      set({
        board: next,
        history: { past: [...past, structuredClone(b)], future: future.slice(1) },
        canUndo: true,
        dirty: true,
        selectedId: null,
        status: "已重做本地修改",
      });
      return;
    }
    try {
      const next = await api.redo(b.id);
      set({ board: next, dirty: false, report: null, selectedId: null, canRedo: true, status: "已重做" });
      await get().runDrc();
    } catch {
      set({ status: "没有可重做的历史" });
    }
  },

  runDrc: async () => {
    const b = get().board;
    if (!b) return;
    try {
      const report = await api.drc(b.id, get().minClearance);
      const errors = report.issues.filter((i) => i.severity === "error").length;
      set({
        report,
        status: errors
          ? `DRC 发现 ${errors} 个错误`
          : "DRC 通过：实际连通正常、无短路",
      });
    } catch (e) {
      set({ status: `DRC 请求失败: ${String(e)}` });
    }
  },

  focusIssue: (i) => set({ focusedIssue: i }),

  exportFile: () => {
    const b = get().board;
    if (!b) return;
    const blob = new Blob([JSON.stringify(b, null, 2)], {
      type: "application/json",
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${b.name || "board"}.pcb.json`;
    a.click();
    URL.revokeObjectURL(url);
    set({ status: "已导出完整板子 JSON（含全部 ID）" });
  },

  importFile: async (file, overwrite) => {
    const board = JSON.parse(await file.text()) as Board;
    const saved = await api.importBoard(board, overwrite);
    await get().refreshList();
    await get().load(saved.id);
    set({ status: "已导入并保留全部元件/焊盘/导线/过孔身份" });
  },
}));
