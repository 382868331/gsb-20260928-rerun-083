import type { Board, CompareReport, Jumper, Pad, Point, StoredCompare, Trace, Variant, Via } from './types'

const isNum = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v)
const isStr = (v: unknown): v is string => typeof v === 'string'

// normalizeBoard repairs a partially missing board document instead of
// rejecting it wholesale: every absent section falls back to a safe
// default and every broken element is skipped with a warning that names
// its position, so the rest of the board stays editable.
export function normalizeBoard(raw: unknown): { board: Board; warnings: string[] } {
  const warnings: string[] = []
  if (!raw || typeof raw !== 'object') throw new Error('板资料不是有效的 JSON 对象')
  const r = raw as Record<string, unknown>

  const arr = (field: string): unknown[] => {
    const v = r[field]
    if (v == null) {
      warnings.push('字段 ' + field + ' 缺失, 已按空列表处理')
      return []
    }
    if (!Array.isArray(v)) {
      warnings.push('字段 ' + field + ' 不是列表, 已忽略')
      return []
    }
    return v
  }

  const points = (v: unknown, where: string): Point[] => {
    if (!Array.isArray(v)) return []
    const out: Point[] = []
    v.forEach((p, i) => {
      const o = p as Record<string, unknown>
      if (o && isNum(o.x) && isNum(o.y)) out.push({ x: Math.round(o.x), y: Math.round(o.y) })
      else warnings.push(where + ' 第 ' + (i + 1) + ' 个坐标缺失或非法, 已跳过')
    })
    return out
  }

  const layers = arr('layers').filter(isStr)
  if (layers.length === 0) {
    warnings.push('字段 layers 为空, 已使用默认层 top/bottom')
    layers.push('top', 'bottom')
  }

  const nets = arr('nets')
    .filter((n) => {
      const o = n as Record<string, unknown>
      if (o && isStr(o.id)) return true
      warnings.push('网络条目缺少 id, 已跳过')
      return false
    })
    .map((n) => {
      const o = n as Record<string, unknown>
      return { id: o.id as string, name: isStr(o.name) ? o.name : (o.id as string) }
    })

  const pads: Pad[] = []
  arr('pads').forEach((p, i) => {
    const o = p as Record<string, unknown>
    const where = '焊盘[' + i + ']' + (o && isStr(o.id) ? ' (' + o.id + ')' : '')
    if (!o || !isNum(o.x) || !isNum(o.y)) {
      warnings.push(where + ' 坐标缺失或非法, 已跳过')
      return
    }
    pads.push({
      id: isStr(o.id) ? o.id : 'pad-' + i,
      netId: isStr(o.netId) ? o.netId : '',
      layer: isStr(o.layer) ? o.layer : layers[0],
      shape: o.shape === 'rect' ? 'rect' : 'circle',
      x: Math.round(o.x), y: Math.round(o.y),
      diameter: isNum(o.diameter) ? o.diameter : 1_000_000,
      w: isNum(o.w) ? o.w : 1_000_000,
      h: isNum(o.h) ? o.h : 1_000_000,
      drill: isNum(o.drill) ? o.drill : 0,
    })
  })

  const lineLike = <T extends { id: string }>(field: string, label: string, make: (o: Record<string, unknown>, id: string, pts: Point[]) => T): T[] => {
    const out: T[] = []
    arr(field).forEach((t, i) => {
      const o = t as Record<string, unknown>
      const id = o && isStr(o.id) ? o.id : label + '-' + i
      if (!o || !Array.isArray(o.points) || o.points.length === 0) {
        warnings.push(label + '[' + i + '] (' + id + ') 缺少点列, 已跳过')
        return
      }
      const pts = points(o.points, label + ' ' + id)
      if (pts.length === 0) {
        warnings.push(label + '[' + i + '] (' + id + ') 没有有效坐标, 已跳过')
        return
      }
      out.push(make(o, id, pts))
    })
    return out
  }

  const traces = lineLike<Trace & object>('traces', '铜线', (o, id, pts) => ({
    id, netId: isStr(o.netId) ? o.netId : '', layer: isStr(o.layer) ? o.layer : layers[0],
    width: isNum(o.width) ? o.width : 500_000, points: pts,
  }))
  const jumpers = lineLike<Jumper & object>('jumpers', '跳线', (o, id, pts) => ({
    id, netId: isStr(o.netId) ? o.netId : '', layer: isStr(o.layer) ? o.layer : layers[0],
    width: isNum(o.width) ? o.width : 500_000, points: pts,
  }))

  const vias: Via[] = []
  arr('vias').forEach((v, i) => {
    const o = v as Record<string, unknown>
    const where = '过孔[' + i + ']' + (o && isStr(o.id) ? ' (' + o.id + ')' : '')
    if (!o || !isNum(o.x) || !isNum(o.y)) {
      warnings.push(where + ' 坐标缺失或非法, 已跳过')
      return
    }
    vias.push({
      id: isStr(o.id) ? o.id : 'via-' + i,
      netId: isStr(o.netId) ? o.netId : '',
      x: Math.round(o.x), y: Math.round(o.y),
      diameter: isNum(o.diameter) ? o.diameter : 1_200_000,
      drill: isNum(o.drill) ? o.drill : 600_000,
      layers: Array.isArray(o.layers) ? o.layers.filter(isStr) : [],
    })
  })

  const variants: Variant[] = []
  arr('variants').forEach((v, i) => {
    const o = v as Record<string, unknown>
    if (!o || !isStr(o.id)) {
      warnings.push('装配方案[' + i + '] 缺少 id, 已跳过')
      return
    }
    variants.push({
      id: o.id,
      name: isStr(o.name) ? o.name : o.id,
      dnp: Array.isArray(o.dnp) ? o.dnp.filter(isStr) : [],
      jumpers: Array.isArray(o.jumpers) ? o.jumpers.filter(isStr) : [],
    })
  })

  const board: Board = {
    id: isStr(r.id) ? r.id : 'board',
    name: isStr(r.name) ? r.name : '未命名板',
    revision: isNum(r.revision) ? r.revision : 0,
    outline: points(r.outline, '板框'),
    layers, nets, pads, traces, vias, jumpers, variants,
  }
  return { board, warnings }
}

// normalizeCompare repairs a compare report whose lists may arrive as
// null (empty Go slices) so the sidebar can always read .length; one
// broken variant entry never takes the whole report — or the page —
// down with it.
export function normalizeCompare(raw: unknown): CompareReport {
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : [])
  const strs = (v: unknown): string[] => arr(v).filter((x): x is string => typeof x === 'string')
  const variants = arr(r.variants).map((v) => {
    const o = (v ?? {}) as Record<string, unknown>
    return {
      variantId: typeof o.variantId === 'string' ? o.variantId : '',
      name: typeof o.name === 'string' ? o.name : String(o.variantId ?? ''),
      revision: typeof o.revision === 'number' ? o.revision : 0,
      status: o.status === 'empty' || o.status === 'incomplete' ? o.status : 'ok' as const,
      warnings: strs(o.warnings),
      errors: arr(o.errors),
      netStatus: arr(o.netStatus),
    }
  }) as CompareReport['variants']
  const diff = arr(r.diff).map((d) => {
    const o = (d ?? {}) as Record<string, unknown>
    return {
      netId: typeof o.netId === 'string' ? o.netId : '',
      netName: typeof o.netName === 'string' ? o.netName : '',
      states: (o.states && typeof o.states === 'object' ? o.states : {}) as Record<string, string>,
      onlyOpen: strs(o.onlyOpen),
      onlyShort: strs(o.onlyShort),
    }
  })
  return {
    id: typeof r.id === 'string' ? r.id : undefined,
    key: typeof r.key === 'string' ? r.key : undefined,
    revision: typeof r.revision === 'number' ? r.revision : 0,
    variants, diff, warnings: strs(r.warnings),
  }
}

// normalizeStoredCompares repairs the persisted compare-report history
// one entry at a time: a single damaged entry (missing report, broken
// basis snapshot) is skipped or degraded with a warning, while every
// other report is recovered fully.
export function normalizeStoredCompares(raw: unknown): { reports: StoredCompare[]; warnings: string[] } {
  const warnings: string[] = []
  const r = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const list = Array.isArray(r.reports) ? r.reports : []
  const reports: StoredCompare[] = []
  list.forEach((e, i) => {
    if (!e || typeof e !== 'object') {
      warnings.push('比较报告[' + i + '] 资料缺失, 已跳过')
      return
    }
    const o = e as Record<string, unknown>
    let basis: Board | null = null
    if (o.basis && typeof o.basis === 'object') {
      try {
        basis = normalizeBoard(o.basis).board
      } catch {
        warnings.push('比较报告[' + i + '] 的板依据无法恢复, 仅保留报告结果')
      }
    } else {
      warnings.push('比较报告[' + i + '] 缺少板依据快照, 导通路径将按当前几何查看')
    }
    reports.push({
      id: typeof o.id === 'string' ? o.id : 'cmp-' + i,
      revision: typeof o.revision === 'number' ? o.revision : 0,
      key: typeof o.key === 'string' ? o.key : '',
      current: o.current === true,
      report: normalizeCompare(o.report),
      basis,
    })
  })
  return { reports, warnings }
}
