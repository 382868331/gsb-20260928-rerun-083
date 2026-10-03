import type { Board } from './types'

// Tolerantly repair a partially missing board: keep every usable element,
// default the missing pieces, and report each repair with its board
// location so a damaged file never blanks the whole canvas.
export function normalizeBoard(raw: unknown): { board: Board; warnings: string[] } {
  const warnings: string[] = []
  const src = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const arr = <T>(v: unknown): T[] => (Array.isArray(v) ? (v as T[]) : [])
  const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0)
  const str = (v: unknown) => (typeof v === 'string' ? v : '')
  const mm = (v: number) => (v / 1e6).toFixed(3)
  const loc = (kind: string, id: string, idx: number, x: number, y: number) =>
    `${kind} ${id || '第 ' + (idx + 1) + ' 项'} (${mm(x)}, ${mm(y)}) mm`

  const layers = arr<string>(src.layers).filter((l) => typeof l === 'string')
  if (layers.length === 0) {
    layers.push('top', 'bottom')
    warnings.push('板资料缺少层定义, 已按默认 top/bottom 两层处理')
  }

  const pads = arr<Record<string, unknown>>(src.pads).flatMap((p, i) => {
    const pad = {
      id: str(p.id), netId: str(p.netId), layer: str(p.layer),
      shape: (p.shape === 'rect' ? 'rect' : 'circle') as 'circle' | 'rect',
      x: num(p.x), y: num(p.y), diameter: num(p.diameter),
      w: num(p.w), h: num(p.h), drill: num(p.drill),
    }
    if (pad.shape === 'rect' ? pad.w <= 0 || pad.h <= 0 : pad.diameter <= 0) {
      warnings.push('焊盘尺寸缺失或无效, 已跳过: ' + loc('焊盘', pad.id, i, pad.x, pad.y))
      return []
    }
    if (!pad.layer) {
      pad.layer = layers[0]
      warnings.push(`焊盘缺少所在层, 已归入 ${pad.layer}: ` + loc('焊盘', pad.id, i, pad.x, pad.y))
    }
    return [pad]
  })

  const traces = arr<Record<string, unknown>>(src.traces).flatMap((t, i) => {
    const points = arr<Record<string, unknown>>(t.points)
      .map((p) => ({ x: Math.round(num(p.x)), y: Math.round(num(p.y)) }))
    const trace = {
      id: str(t.id), netId: str(t.netId), layer: str(t.layer),
      width: num(t.width), points,
    }
    if (points.length === 0) {
      warnings.push(`铜线 ${trace.id || '第 ' + (i + 1) + ' 项'} 缺少路径点, 已跳过`)
      return []
    }
    if (trace.width <= 0) {
      trace.width = 500000
      warnings.push('铜线缺少线宽, 已按 0.5 mm 处理: ' + loc('铜线', trace.id, i, points[0].x, points[0].y))
    }
    if (!trace.layer) {
      trace.layer = layers[0]
      warnings.push(`铜线缺少所在层, 已归入 ${trace.layer}: ` + loc('铜线', trace.id, i, points[0].x, points[0].y))
    }
    return [trace]
  })

  const vias = arr<Record<string, unknown>>(src.vias).flatMap((v, i) => {
    const via = {
      id: str(v.id), netId: str(v.netId), x: num(v.x), y: num(v.y),
      diameter: num(v.diameter), drill: num(v.drill),
      layers: arr<string>(v.layers).filter((l) => typeof l === 'string'),
    }
    if (via.diameter <= 0) {
      warnings.push('过孔直径缺失或无效, 已跳过: ' + loc('过孔', via.id, i, via.x, via.y))
      return []
    }
    if (via.layers.length === 0) {
      via.layers = [...layers]
      warnings.push('过孔缺少连接层声明, 已按全部层桥接: ' + loc('过孔', via.id, i, via.x, via.y))
    }
    return [via]
  })

  const present = new Set([...pads, ...traces, ...vias].map((e) => e.id))
  const assemblies = arr<Record<string, unknown>>(src.assemblies).map((a) => {
    const exclude = arr<string>(a.exclude).filter((id) => {
      if (present.has(id)) return true
      warnings.push(`装配方案 ${str(a.name) || str(a.id)} 引用了不存在的元件 ${id}, 已忽略`)
      return false
    })
    return { id: str(a.id), name: str(a.name), exclude }
  })

  const board: Board = {
    id: str(src.id), name: str(src.name), revision: num(src.revision),
    outline: arr<Record<string, unknown>>(src.outline).map((p) => ({ x: Math.round(num(p.x)), y: Math.round(num(p.y)) })),
    layers,
    nets: arr<Record<string, unknown>>(src.nets).map((n) => ({ id: str(n.id), name: str(n.name) })),
    pads, traces, vias, assemblies,
  }
  return { board, warnings }
}
