import { useEffect, useRef, useState } from 'react'
import { MM, uid, useStore } from './store'
import type { Board, Point } from './types'

const PX_PER_NM = 30 / MM // 1mm = 30px at zoom 1
const LAYER_COLORS: Record<string, string> = {
  top: 'rgba(224,62,47,0.8)',
  bottom: 'rgba(36,93,220,0.8)',
}
const layerColor = (l: string, i: number) =>
  LAYER_COLORS[l] ?? ['rgba(30,160,90,0.8)', 'rgba(180,80,200,0.8)'][i % 2]

export default function Canvas() {
  const svgRef = useRef<SVGSVGElement>(null)
  const drag = useRef<null | { kind: 'pan' | 'move'; sx: number; sy: number; snap?: Board; panX?: number; panY?: number }>(null)
  const [cursor, setCursor] = useState<Point | null>(null)
  const s = useStore()
  const b = s.board
  const k = PX_PER_NM * s.view.zoom

  const toBoard = (e: { clientX: number; clientY: number }): Point => {
    const r = svgRef.current!.getBoundingClientRect()
    return { x: (e.clientX - r.left - s.view.panX) / k, y: (e.clientY - r.top - s.view.panY) / k }
  }

  // Keyboard: undo/redo, delete, escape, enter.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const st = useStore.getState()
      if ((e.target as HTMLElement)?.tagName === 'INPUT' || (e.target as HTMLElement)?.tagName === 'SELECT') return
      if (e.key === 'Escape') st.set({ draft: [], selected: null })
      else if (e.key === 'Enter') finishDraft()
      else if ((e.key === 'Delete' || e.key === 'Backspace') && st.selected) deleteSelected()
      else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z' && !e.shiftKey) { e.preventDefault(); st.undo() }
      else if ((e.ctrlKey || e.metaKey) && (e.key.toLowerCase() === 'y' || (e.key.toLowerCase() === 'z' && e.shiftKey))) { e.preventDefault(); st.redo() }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const finishDraft = () => {
    const st = useStore.getState()
    if (!st.board) return
    if (st.tool === 'trace' && st.draft.length >= 2) {
      const pts = st.draft
      st.commit((bb) => {
        bb.traces.push({ id: uid('trace'), netId: st.netId, layer: st.layer, width: st.traceWidth, points: pts })
      })
      st.set({ draft: [] })
    } else if (st.tool === 'outline' && st.draft.length >= 3) {
      const pts = st.draft
      st.commit((bb) => { bb.outline = pts })
      st.set({ draft: [] })
    }
  }

  const deleteSelected = () => {
    const st = useStore.getState()
    const sel = st.selected
    if (!sel) return
    st.commit((bb) => {
      if (sel.kind === 'pad') bb.pads = bb.pads.filter((p) => p.id !== sel.id)
      if (sel.kind === 'trace') bb.traces = bb.traces.filter((t) => t.id !== sel.id)
      if (sel.kind === 'via') bb.vias = bb.vias.filter((v) => v.id !== sel.id)
    })
    st.set({ selected: null })
  }

  const onWheel = (e: React.WheelEvent) => {
    const r = svgRef.current!.getBoundingClientRect()
    const mx = e.clientX - r.left, my = e.clientY - r.top
    const zoom = Math.min(50, Math.max(0.05, s.view.zoom * Math.exp(-e.deltaY * 0.0012)))
    const kk = PX_PER_NM * zoom
    // Keep the board point under the cursor fixed: pure view transform,
    // board geometry (nm) is untouched.
    const bx = (mx - s.view.panX) / k, by = (my - s.view.panY) / k
    s.setView({ zoom, panX: mx - bx * kk, panY: my - by * kk })
  }

  const onPointerDown = (e: React.PointerEvent) => {
    if (e.button === 1 || e.button === 2) {
      drag.current = { kind: 'pan', sx: e.clientX, sy: e.clientY, panX: s.view.panX, panY: s.view.panY }
      ;(e.target as Element).setPointerCapture?.(e.pointerId)
      return
    }
    if (e.button !== 0 || !b) return
    const p = toBoard(e)
    if (s.tool === 'trace' || s.tool === 'outline') {
      s.set({ draft: [...s.draft, p] })
    } else if (s.tool === 'pad') {
      s.commit((bb) => {
        bb.pads.push({
          id: uid('pad'), netId: s.netId, layer: s.layer, shape: s.padShape,
          x: Math.round(p.x), y: Math.round(p.y),
          diameter: s.padSize, w: s.padSize, h: s.padSize * 0.7, drill: 0,
        })
      })
    } else if (s.tool === 'via') {
      s.commit((bb) => {
        bb.vias.push({
          id: uid('via'), netId: s.netId, x: Math.round(p.x), y: Math.round(p.y),
          diameter: 1.2 * MM, drill: 0.6 * MM, layers: [...s.viaLayers],
        })
      })
    } else if (s.tool === 'select') {
      s.set({ selected: null })
    }
  }

  const onElemPointerDown = (e: React.PointerEvent, kind: 'pad' | 'trace' | 'via', id: string) => {
    if (s.tool !== 'select' || e.button !== 0 || !b) return
    e.stopPropagation()
    s.set({ selected: { kind, id } })
    drag.current = { kind: 'move', sx: e.clientX, sy: e.clientY, snap: JSON.parse(JSON.stringify(b)) }
    ;(e.currentTarget as Element).setPointerCapture?.(e.pointerId)
  }

  const onPointerMove = (e: React.PointerEvent) => {
    if (!b) return
    setCursor(toBoard(e))
    const d = drag.current
    if (!d) return
    if (d.kind === 'pan') {
      s.setView({ zoom: s.view.zoom, panX: (d.panX ?? 0) + e.clientX - d.sx, panY: (d.panY ?? 0) + e.clientY - d.sy })
    } else if (d.kind === 'move' && d.snap && s.selected) {
      const dx = (e.clientX - d.sx) / k, dy = (e.clientY - d.sy) / k
      const sel = s.selected
      const nb: Board = JSON.parse(JSON.stringify(d.snap))
      if (sel.kind === 'pad') {
        const p = nb.pads.find((x) => x.id === sel.id); if (p) { p.x += dx; p.y += dy }
      } else if (sel.kind === 'via') {
        const v = nb.vias.find((x) => x.id === sel.id); if (v) { v.x += dx; v.y += dy }
      } else {
        const t = nb.traces.find((x) => x.id === sel.id)
        if (t) t.points = t.points.map((p) => ({ x: p.x + dx, y: p.y + dy }))
      }
      s.set({ board: nb })
    }
  }

  const onPointerUp = () => {
    const d = drag.current
    if (d?.kind === 'move' && d.snap && s.board) {
      // Commit the move as one undoable step.
      const cur = s.board
      useStore.setState({ past: [...s.past, d.snap], future: [], errors: [], board: cur })
    }
    drag.current = null
  }

  const onDoubleClick = () => finishDraft()

  if (!b) return <div className="canvas loading">加载中…</div>

  const selId = s.selected?.id
  const inactive = (l: string) => (l === s.layer ? 1 : 0.3)
  const pts = (ps: Point[]) => ps.map((p) => `${p.x},${p.y}`).join(' ')

  return (
    <div className="canvas">
      <svg
        ref={svgRef}
        onWheel={onWheel}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onDoubleClick={onDoubleClick}
        onContextMenu={(e) => e.preventDefault()}
      >
        <g transform={`translate(${s.view.panX},${s.view.panY}) scale(${k})`}>
          {b.outline.length >= 3 && (
            <polygon points={pts(b.outline)} fill="rgba(40,60,40,0.25)" stroke="#7a7" strokeWidth={0.3 * MM} />
          )}
          {b.traces.map((t, i) => (
            <polyline
              key={t.id} points={pts(t.points)} fill="none"
              stroke={layerColor(t.layer, b.layers.indexOf(t.layer))}
              strokeWidth={t.width} strokeLinecap="round" strokeLinejoin="round"
              opacity={inactive(t.layer)}
              style={{ pointerEvents: s.tool === 'select' ? 'auto' : 'none', cursor: 'move' }}
              onPointerDown={(e) => onElemPointerDown(e, 'trace', t.id)}
            />
          ))}
          {b.traces.map((t) =>
            selId === t.id ? (
              <polyline key={t.id + '-sel'} points={pts(t.points)} fill="none" stroke="#fff"
                strokeWidth={t.width + 0.4 * MM} strokeLinecap="round" opacity={0.35} pointerEvents="none" />
            ) : null,
          )}
          {b.vias.map((v) => (
            <g key={v.id} opacity={1}
              style={{ pointerEvents: s.tool === 'select' ? 'auto' : 'none', cursor: 'move' }}
              onPointerDown={(e) => onElemPointerDown(e, 'via', v.id)}>
              <circle cx={v.x} cy={v.y} r={v.diameter / 2} fill="#c9a227"
                stroke={selId === v.id ? '#fff' : '#7a5c00'} strokeWidth={selId === v.id ? 0.3 * MM : 0.1 * MM} />
              <circle cx={v.x} cy={v.y} r={v.drill / 2} fill="#111" />
            </g>
          ))}
          {b.pads.map((p) => {
            const col = p.layer === 'through' ? '#c9a227' : layerColor(p.layer, b.layers.indexOf(p.layer))
            const common = {
              fill: col, opacity: p.layer === 'through' ? 1 : inactive(p.layer),
              stroke: selId === p.id ? '#fff' : 'none', strokeWidth: 0.25 * MM,
              style: { pointerEvents: s.tool === 'select' ? ('auto' as const) : ('none' as const), cursor: 'move' },
              onPointerDown: (e: React.PointerEvent) => onElemPointerDown(e, 'pad', p.id),
            }
            return p.shape === 'rect' ? (
              <rect key={p.id} x={p.x - p.w / 2} y={p.y - p.h / 2} width={p.w} height={p.h} {...common} />
            ) : (
              <circle key={p.id} cx={p.x} cy={p.y} r={p.diameter / 2} {...common} />
            )
          })}
          {s.draft.length > 0 && (
            <polyline points={pts(cursor && (s.tool === 'trace' || s.tool === 'outline') ? [...s.draft, cursor] : s.draft)}
              fill="none" stroke="#ffca28" strokeWidth={s.tool === 'trace' ? s.traceWidth : 0.3 * MM}
              strokeLinecap="round" strokeDasharray={s.tool === 'outline' ? `${0.5 * MM} ${0.3 * MM}` : undefined} pointerEvents="none" />
          )}
          {s.errors.map((er, i) => (
            <g key={i} pointerEvents="none">
              <line x1={er.x} y1={er.y} x2={er.x2} y2={er.y2}
                stroke={er.type === 'short' ? '#ff1744' : '#ff9100'} strokeWidth={0.15 * MM} />
              <circle cx={(er.x + er.x2) / 2} cy={(er.y + er.y2) / 2} r={0.7 * MM} fill="none"
                stroke={er.type === 'short' ? '#ff1744' : '#ff9100'} strokeWidth={0.18 * MM} />
              <circle cx={(er.x + er.x2) / 2} cy={(er.y + er.y2) / 2} r={0.18 * MM}
                fill={er.type === 'short' ? '#ff1744' : '#ff9100'} />
            </g>
          ))}
        </g>
      </svg>
      <div className="cursor-readout">
        {cursor ? `x: ${(cursor.x / MM).toFixed(3)} mm  y: ${(cursor.y / MM).toFixed(3)} mm  缩放: ${(s.view.zoom * 100).toFixed(0)}%` : ''}
      </div>
    </div>
  )
}
