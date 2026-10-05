import { useEffect, useState } from 'react'
import { api } from './api'
import { useStore } from './store'
import type { Board, RepairMod, RepairResult, RepairPreviewReport, StoredRepair } from './types'

const stateLabel: Record<string, string> = { connected: '连通', open: '开路', short: '短路' }

let modSeq = 1

// RepairPanel is the constrained minimum-cost repair workflow: declare
// the electrical requirements and the allowed modifications, solve for
// the cheapest combination, preview each mod's effect, then apply the
// chosen combination atomically — with a staleness re-check before
// anything is written.
export default function RepairPanel() {
  const s = useStore()
  const b = s.board
  const [reqNets, setReqNets] = useState<string[]>([])
  const [forbidNets, setForbidNets] = useState<string[]>([])
  const [mods, setMods] = useState<RepairMod[]>([])
  const [result, setResult] = useState<RepairResult | null>(null)
  const [preview, setPreview] = useState<RepairPreviewReport | null>(null)
  const [repairs, setRepairs] = useState<StoredRepair[]>([])
  const [choice, setChoice] = useState(0)
  const [msg, setMsg] = useState('')
  // The exact board the solve ran against: applying is allowed only
  // while neither the revision nor the in-memory board has moved on.
  const [solvedBoard, setSolvedBoard] = useState<Board | null>(null)

  useEffect(() => {
    api.listRepairs().then((r) => setRepairs(r.repairs ?? [])).catch(() => {})
  }, [result])

  if (!b) return null

  const toggle = (list: string[], id: string, set: (v: string[]) => void) =>
    set(list.includes(id) ? list.filter((x) => x !== id) : [...list, id])

  const addMod = (kind: RepairMod['kind']) => {
    const id = 'm' + modSeq++
    const m: RepairMod = { id, kind, cost: 1, excludes: [] }
    if (kind === 'jumper') m.jumperId = b.jumpers[0]?.id ?? ''
    else m.padId = b.pads[0]?.id ?? ''
    setMods([...mods, m])
  }
  const updMod = (id: string, p: Partial<RepairMod>) =>
    setMods(mods.map((m) => (m.id === id ? { ...m, ...p } : m)))

  const solve = async () => {
    setMsg('')
    try {
      const res = await api.solveRepair({
        board: b, variantId: s.variantId,
        requireConnected: reqNets, forbidShort: forbidNets, mods,
      })
      setResult(res)
      setChoice(0)
      setSolvedBoard(res.status === 'ok' ? b : null)
      setMsg(res.status === 'ok'
        ? '找到 ' + res.solutions.length + ' 个最低成本组合 (成本 ' + res.solutions[0]?.cost + ')'
        : res.status === 'no-solution' ? '无解: 请复查冲突要求' : '资料不完整, 无法求解')
    } catch (e) { setMsg('求解失败: ' + (e as Error).message) }
  }

  const doPreview = async () => {
    setMsg('')
    try {
      setPreview(await api.previewRepair({ board: b, variantId: s.variantId, mods }))
    } catch (e) { setMsg('预览失败: ' + (e as Error).message) }
  }

  const stale = result !== null && (solvedBoard === null || b !== solvedBoard || b.revision !== result.revision)

  const apply = async () => {
    if (!result || !solvedBoard) return
    const sol = result.solutions[choice]
    if (!sol) return
    setMsg('')
    try {
      const chosen = mods.filter((m) => sol.modIds.includes(m.id))
      const r = await api.applyRepair({
        variantId: s.variantId, mods: chosen,
        requireConnected: reqNets, forbidShort: forbidNets,
        basisRevision: result.revision,
      })
      s.setBoard(r.board)
      s.setVariant(r.variantId)
      setResult(null)
      setPreview(null)
      setSolvedBoard(null)
      setMsg('已应用组合并保存修复记录 (修订 ' + r.board.revision + ')')
    } catch (e) {
      setMsg('应用被拒绝: ' + (e as Error).message + ' — 请重新求解')
      setSolvedBoard(null)
    }
  }

  const netToggle = (list: string[], set: (v: string[]) => void, label: string) => (
    <>
      <div className="row small">{label}</div>
      <div className="row wrap">
        {b.nets.map((n) => (
          <label key={n.id} className="chk">
            <input type="checkbox" checked={list.includes(n.id)} onChange={() => toggle(list, n.id, set)} />
            {n.name}
          </label>
        ))}
      </div>
    </>
  )

  return (
    <div className="group">
      <h2>修复求解 (最低成本组合)</h2>
      {netToggle(reqNets, setReqNets, '必须连通的网络:')}
      {netToggle(forbidNets, setForbidNets, '禁止短接的网络:')}
      <div className="row small">允许的改装 (成本 / 互斥):</div>
      {mods.map((m) => (
        <div key={m.id} className="modrow">
          <div className="row">
            <b>{m.id}</b>
            <select value={m.kind}
              onChange={(e) => {
                const kind = e.target.value as RepairMod['kind']
                updMod(m.id, kind === 'jumper'
                  ? { kind, jumperId: b.jumpers[0]?.id ?? '', padId: undefined }
                  : { kind, padId: b.pads[0]?.id ?? '', jumperId: undefined })
              }}>
              <option value="jumper">装跳线</option>
              <option value="dnp">不装器件</option>
              <option value="populate">补装器件</option>
            </select>
            {m.kind === 'jumper' ? (
              <select value={m.jumperId} onChange={(e) => updMod(m.id, { jumperId: e.target.value })}>
                {b.jumpers.map((j) => <option key={j.id} value={j.id}>{j.id}</option>)}
              </select>
            ) : (
              <select value={m.padId} onChange={(e) => updMod(m.id, { padId: e.target.value })}>
                {b.pads.map((p) => <option key={p.id} value={p.id}>{p.id}</option>)}
              </select>
            )}
            <button onClick={() => setMods(mods.filter((x) => x.id !== m.id))}>删</button>
          </div>
          <div className="row">
            <label>成本
              <input type="number" min={0} value={m.cost}
                onChange={(e) => updMod(m.id, { cost: Math.max(0, parseInt(e.target.value || '0', 10) || 0) })} />
            </label>
            <label>互斥
              <input type="text" placeholder="如 m2,m3" value={(m.excludes ?? []).join(',')}
                onChange={(e) => updMod(m.id, { excludes: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} />
            </label>
          </div>
        </div>
      ))}
      <div className="row">
        <button onClick={() => addMod('jumper')}>+ 跳线改装</button>
        <button onClick={() => addMod('dnp')}>+ 器件改装</button>
      </div>
      <div className="row">
        <button onClick={doPreview} disabled={mods.length === 0}>预览各项影响</button>
        <button className="primary" onClick={solve}>求解最低成本组合</button>
      </div>
      {msg && <div className="row small status">{msg}</div>}

      {preview && (
        <div className="small">
          <div className="row small">各项改装对网络的影响 (基于修订 {preview.revision}):</div>
          {preview.mods.map((p) => (
            <div key={p.modId} className="netst">
              <b>{p.modId}</b> {p.label}
              {p.changes.length === 0 && <div className="small">对网络无影响</div>}
              {p.changes.map((c) => (
                <div key={c.netId} className="small">
                  {c.netName}: {stateLabel[c.before] ?? c.before} → {stateLabel[c.after] ?? c.after}
                </div>
              ))}
            </div>
          ))}
        </div>
      )}

      {result && (
        <div className="small">
          {result.warnings.length > 0 && (
            <div className="row small status">资料缺失: {result.warnings.join('; ')}</div>
          )}
          {result.status === 'no-solution' && (
            <div>
              <div className="row small">相互冲突的要求 (可复查):</div>
              {result.conflicts.map((c, i) => <div key={i} className="err open">{c}</div>)}
            </div>
          )}
          {result.status === 'ok' && result.solutions.map((sol, i) => (
            <label key={i} className={'netst chk' + (i === choice ? ' connected' : '')}>
              <input type="radio" checked={i === choice} onChange={() => setChoice(i)} />
              组合 {i + 1} (成本 {sol.cost}): {sol.modIds.length ? sol.modIds.join(' + ') : '(无需改装)'}
            </label>
          ))}
          {result.status === 'ok' && (
            <>
              {stale && (
                <div className="row small status">板在求解后已变化, 需重新求解才能应用</div>
              )}
              <button className="primary" disabled={stale} onClick={apply}>
                确认并应用所选组合
              </button>
            </>
          )}
        </div>
      )}

      {repairs.length > 0 && (
        <div className="small">
          已保存的修复记录 (独立存档, 不影响冻结比较报告):
          {repairs.map((r, i) => (
            <div key={r.id} className="row small">
              #{i + 1} · 依据修订 {r.basisRevision} → 修订 {r.appliedRevision} · {r.modIds.join(' + ') || '(无改装)'} · 成本 {r.cost}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
