import { useRef, useState, useCallback } from "react";
import type { Board, Point } from "./types";
import { useStore, type Layer } from "./store";

interface View {
  scale: number;
  // screen pixel offset of board origin (mm 0,0)
  ox: number;
  oy: number;
}

const LAYER_COLOR: Record<Layer, string> = {
  top: "#2563eb",
  mid: "#7c3aed",
  bottom: "#059669",
};

function polyPoints(b: Board, layer: Layer): Point[] {
  return b.traces.filter((t) => t.layer === layer).flatMap((t) => t.points);
}

export default function Canvas() {
  const svgRef = useRef<SVGSVGElement>(null);
  const [view, setView] = useState<View>({ scale: 10, ox: 40, oy: 40 });
  const [panning, setPanning] = useState<{ x: number; y: number; ox: number; oy: number } | null>(null);
  const [dragMoved, setDragMoved] = useState(false);
  const [hover, setHover] = useState<Point | null>(null);

  const board = useStore((s) => s.board);
  const tool = useStore((s) => s.tool);
  const layer = useStore((s) => s.layer);
  const draftTrace = useStore((s) => s.draftTrace);
  const selectedId = useStore((s) => s.selectedId);
  const report = useStore((s) => s.report);
  const focusedIssue = useStore((s) => s.focusedIssue);
  const addPad = useStore((s) => s.addPad);
  const addVia = useStore((s) => s.addVia);
  const pushDraftPoint = useStore((s) => s.pushDraftPoint);
  const finishTrace = useStore((s) => s.finishTrace);
  const cancelDraft = useStore((s) => s.cancelDraft);
  const selectFeature = useStore((s) => s.selectFeature);

  // Geometry always works in board millimetres. Zoom only changes this
  // mm->pixel factor; contact checks live on the Go side in mm.
  const toBoard = useCallback(
    (clientX: number, clientY: number): Point => {
      const rect = svgRef.current!.getBoundingClientRect();
      return {
        x: (clientX - rect.left - view.ox) / view.scale,
        y: (clientY - rect.top - view.oy) / view.scale,
      };
    },
    [view]
  );

  if (!board) {
    return (
      <div className="canvas-empty">
        <p>还没有打开的板子。</p>
        <p>点击左上角「新建板子」开始，或导入 .pcb.json 文件。</p>
      </div>
    );
  }

  const onWheel = (e: React.WheelEvent) => {
    const rect = svgRef.current!.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    const factor = e.deltaY < 0 ? 1.12 : 1 / 1.12;
    const scale = Math.min(40, Math.max(2, view.scale * factor));
    // keep the board point under the cursor stationary
    const bx = (mx - view.ox) / view.scale;
    const by = (my - view.oy) / view.scale;
    setView({ scale, ox: mx - bx * scale, oy: my - by * scale });
  };

  const onMouseDown = (e: React.MouseEvent) => {
    if (e.button === 1 || (e.button === 0 && tool === "select" && targetIsBackground(e))) {
      setPanning({ x: e.clientX, y: e.clientY, ox: view.ox, oy: view.oy });
      setDragMoved(false);
    }
  };

  const targetIsBackground = (e: React.MouseEvent) =>
    (e.target as HTMLElement | SVGElement).getAttribute?.("data-bg") === "1";

  const onMouseMove = (e: React.MouseEvent) => {
    if (panning) {
      const dx = e.clientX - panning.x;
      const dy = e.clientY - panning.y;
      if (Math.abs(dx) + Math.abs(dy) > 3) setDragMoved(true);
      setView({
        ...view,
        ox: panning.ox + dx,
        oy: panning.oy + dy,
      });
      return;
    }
    setHover(toBoard(e.clientX, e.clientY));
  };

  const onMouseUp = () => setPanning(null);

  const onClick = (e: React.MouseEvent) => {
    if (dragMoved) return;
    const p = toBoard(e.clientX, e.clientY);
    switch (tool) {
      case "pad":
        addPad(p);
        break;
      case "via":
        addVia(p);
        break;
      case "trace":
        pushDraftPoint(p);
        break;
      default:
        if (targetIsBackground(e)) selectFeature(null);
    }
  };

  const onContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    if (tool === "trace") finishTrace();
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") cancelDraft();
    if (e.key === "Enter" && tool === "trace") finishTrace();
  };

  const o = board.outline;
  const padPath = (x: number, y: number, r: number) =>
    `M ${x - r} ${y} A ${r} ${r} 0 1 0 ${x + r} ${y} A ${r} ${r} 0 1 0 ${x - r} ${y}`;
  const traceD = (pts: Point[]) =>
    pts.map((q, i) => `${i === 0 ? "M" : "L"} ${q.x} ${q.y}`).join(" ");
  const allPts = polyPoints(board, layer);
  const gridStep = 5;
  const focused = focusedIssue != null ? report?.issues[focusedIssue] : null;

  return (
    <svg
      ref={svgRef}
      className="canvas"
      tabIndex={0}
      onWheel={onWheel}
      onMouseDown={onMouseDown}
      onMouseMove={onMouseMove}
      onMouseUp={onMouseUp}
      onClick={onClick}
      onContextMenu={onContextMenu}
      onKeyDown={onKeyDown}
    >
      <g transform={`translate(${view.ox} ${view.oy}) scale(${view.scale})`}>
        <rect
          data-bg="1"
          x={-2000}
          y={-2000}
          width={4000}
          height={4000}
          fill="#0f172a"
        />
        {Array.from({ length: Math.ceil(o.width / gridStep) + 1 }, (_, i) => (
          <line
            key={`v${i}`}
            x1={i * gridStep}
            y1={0}
            x2={i * gridStep}
            y2={o.height}
            stroke="#1e293b"
            strokeWidth={1 / view.scale}
          />
        ))}
        {Array.from({ length: Math.ceil(o.height / gridStep) + 1 }, (_, i) => (
          <line
            key={`h${i}`}
            x1={0}
            y1={i * gridStep}
            x2={o.width}
            y2={i * gridStep}
            stroke="#1e293b"
            strokeWidth={1 / view.scale}
          />
        ))}

        <rect
          x={o.x}
          y={o.y}
          width={o.width}
          height={o.height}
          fill="#111827"
          stroke="#e2e8f0"
          strokeWidth={2 / view.scale}
        />

        {/* inactive layers rendered faint below */}
        {(["bottom", "mid", "top"] as Layer[])
          .filter((l) => l !== layer)
          .map((l) => (
            <g key={l} opacity={0.22}>
              {board.traces
                .filter((t) => t.layer === l)
                .map((t) => (
                  <path
                    key={t.id}
                    d={traceD(t.points)}
                    fill="none"
                    stroke={LAYER_COLOR[l]}
                    strokeWidth={t.width}
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  />
                ))}
            </g>
          ))}

        {board.traces
          .filter((t) => t.layer === layer)
          .map((t) => (
            <path
              key={t.id}
              d={traceD(t.points)}
              fill="none"
              stroke={LAYER_COLOR[layer]}
              strokeWidth={t.width}
              strokeLinecap="round"
              strokeLinejoin="round"
              className={selectedId === t.id ? "selected" : ""}
              onMouseDown={(e) => {
                e.stopPropagation();
                selectFeature(t.id);
              }}
            />
          ))}

        {board.vias.map((v) => (
          <g
            key={v.id}
            className={selectedId === v.id ? "selected" : ""}
            onMouseDown={(e) => {
              e.stopPropagation();
              selectFeature(v.id);
            }}
          >
            <circle cx={v.x} cy={v.y} r={v.radius} fill="#fbbf24" opacity={0.35} />
            <circle
              cx={v.x}
              cy={v.y}
              r={v.radius}
              fill="none"
              stroke="#f59e0b"
              strokeWidth={0.18}
            />
            <circle cx={v.x} cy={v.y} r={v.radius * 0.35} fill="#f59e0b" />
            <text x={v.x} y={v.y - v.radius - 0.4} fontSize={1.4} textAnchor="middle" fill="#fde68a">
              {v.layers.join("/")}
            </text>
          </g>
        ))}

        {board.pads
          .filter((p) => p.layers.includes(layer))
          .map((p) => (
            <g
              key={p.id}
              className={selectedId === p.id ? "selected" : ""}
              onMouseDown={(e) => {
                e.stopPropagation();
                selectFeature(p.id);
              }}
            >
              <circle cx={p.x} cy={p.y} r={p.radius} fill="#e2e8f0" />
              {p.holeRadius ? (
                <circle cx={p.x} cy={p.y} r={p.holeRadius} fill="#0f172a" />
              ) : null}
              <text x={p.x} y={p.y + 0.5} fontSize={1.6} textAnchor="middle" fill="#0f172a">
                {p.name}
              </text>
              {p.net ? (
                <text x={p.x} y={p.y - p.radius - 0.5} fontSize={1.5} textAnchor="middle" fill="#7dd3fc">
                  {p.net}
                </text>
              ) : null}
            </g>
          ))}

        {draftTrace.length > 0 && (
          <path
            d={traceD(draftTrace)}
            fill="none"
            stroke="#f472b6"
            strokeWidth={useStore.getState().traceWidth}
            strokeDasharray="1 0.6"
            strokeLinecap="round"
          />
        )}
        {draftTrace.length > 0 && hover && (
          <line
            x1={draftTrace[draftTrace.length - 1].x}
            y1={draftTrace[draftTrace.length - 1].y}
            x2={hover.x}
            y2={hover.y}
            stroke="#f472b6"
            strokeWidth={useStore.getState().traceWidth}
            strokeDasharray="1 0.6"
          />
        )}

        {/* DRC markers point at exact board coordinates */}
        {report?.issues
          .map((iss, i) => ({ iss, i }))
          .filter(({ iss }) => iss.at)
          .map(({ iss, i }) => (
            <g
              key={`issue-${i}`}
              className={`issue ${iss.severity}`}
              opacity={focusedIssue == null || focusedIssue === i ? 1 : 0.25}
            >
              <circle
                cx={iss.at!.x}
                cy={iss.at!.y}
                r={1.6 / Math.sqrt(view.scale / 10)}
                fill="none"
                stroke={iss.severity === "error" ? "#ef4444" : "#f59e0b"}
                strokeWidth={0.3}
              />
              <line
                x1={iss.at!.x - 1.2}
                y1={iss.at!.y}
                x2={iss.at!.x + 1.2}
                y2={iss.at!.y}
                stroke={iss.severity === "error" ? "#ef4444" : "#f59e0b"}
                strokeWidth={0.2}
              />
              <line
                x1={iss.at!.x}
                y1={iss.at!.y - 1.2}
                x2={iss.at!.x}
                y2={iss.at!.y + 1.2}
                stroke={iss.severity === "error" ? "#ef4444" : "#f59e0b"}
                strokeWidth={0.2}
              />
            </g>
          ))}
        {focused?.at ? (
          <circle
            cx={focused.at.x}
            cy={focused.at.y}
            r={3}
            fill="none"
            stroke="#fca5a5"
            strokeWidth={0.25}
          />
        ) : null}

        {hover && tool !== "select" ? (
          <text x={hover.x + 1} y={hover.y - 1} fontSize={1.6} fill="#94a3b8">
            {hover.x.toFixed(2)}, {hover.y.toFixed(2)} mm
          </text>
        ) : null}
      </g>
    </svg>
  );
}
