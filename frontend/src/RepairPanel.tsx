import { useEffect, useState } from 'react'
import { api } from './api'
import { uid, useStore } from './store'
import type { NetChange, RepairMod, RepairResult, RepairSpec, StoredRepair } from './types'

const stateLabel: Record<string, string> = { connected: '连通', open: '开路', short: '短路' }

const kindLabel: Record<RepairMod['kind'], string> = {
  jumper: '安装跳线',
  populate: '补装器件',
  dnp: '移除器件',
}

function changesList({ changes }: { changes: NetChange[] }) {
  if (changes.length === 0) return <div className="row small">对网络状态无影响</div>
  return (
    <div className="small">
      {changes.map((c) => (
        <div key={c.netId}>
          {c.netName}: {stateLabel[c.from] ?? c.from} → {stateLabel[c.to] ?? c.to}
        </div>
      ))}
    </div>
  )
}

// RepairPanel drives the constrained minimal-cost repair solver: the
// user states the electrical requirements and the allowed modifications
// (with cost and mutual exclusion), previews each mod's net impact, and
// applies a whole confirmed combination at once. If the board changed
// since the solve, the backend refuses the apply and a re-solve is
// required.
export default function RepairPanel() {
  const s = useStore()
  const b = s.board
  const [mustConnect, setMustConnect] = useState<string[]>([])
  const [noShort, setNoShort] = useState<string[]>([])
  const [mods, setMods] = useState<RepairMod[]>([])
  const [result, setResult] = useState<RepairResult | null>(null)
  const [stale, setStale] = useState(false)
  const [saved, setSaved] = useState<StoredRepair[]>([])
  const [msg, setMsg] = useState('')

  const refreshSaved = () =>
    api.listRepairs().then(({ repairs }) => setSaved(repairs ?? [])).catch(() => {})
  useEffect(() => { refreshSaved() }, [])

  if (!b) return null

  const spec: RepairSpec = { mustConnect, noShort, mods }
  const variantName = (id: string) =>
    id === '' ? '基准 (全部装配, 无跳线)' : (b.variants.find((v) => v.id === id)?.name ?? id)

  const toggle = (list: string[], set: (v: string[]) => void, id: string, on: boolean) =>
    set(on ? [...list, id] : list.filter((x) => x !== id))

  const updateMod = (id: string, patch: Partial<RepairMod>) =>
    setMods(mods.map((m) => (m.id === id ? { ...m, ...patch } : m)))

  const solve = async () => {
    setMsg('')
    try {
      const { result: res } = await api.solveRepair(b, s.variantId, spec)
      setResult(res)
      setStale(false)
      refreshSaved()
      setMsg(res.solutions.length > 0
        ? '找到 ' + res.solutions.length + ' 个最低成本组合 (成本 ' + res.solutions[0].cost + ')'
        : '无解: 请复查相互冲突的要求')
    } catch (e) {
      setMsg('求解失败: ' + (e as Error).message)
    }
  }

  const apply = async (modIds: string[]) => {
    if (!result) return
    if (!confirm('确认按当前板修订与装配应用整个组合 ' + modIds.join('+') + '?')) return
    try {
      const out = await api.applyRepair(b, s.variantId, spec, modIds, result.key)
      s.setBoard(out.board)
      s.setVariant(out.variantId)
      setResult(null)
      setMsg('已应用为新装配方案 (成本 ' + out.cost + '), 修订 ' + out.board.revision)
    } catch (e) {
      // The basis moved on (edit/undo/assembly change): the old solve
      // no longer stands in for the current board.
      setStale(true)
      setMsg((e as Error).message)
    }
  }

  const netBoxes = (list: string[], set: (v: string[]) => void) =>
    b.nets.map((n) => (
      <label key={n.id} className="chk">
        <input type="checkbox" checked={list.includes(n.id)}
          onChange={(e) => toggle(list, set, n.id, e.target.checked)} />
        {n.name}
      </label>
    ))

  return (
    <div className="group">
      <h2>最小修复方案求解 · {variantName(s.variantId)}</h2>
      <div className="row small">必须连通的网络:</div>
      <div className="row wrap">{netBoxes(mustConnect, setMustConnect)}</div>
      <div className="row small">禁止短接的网络:</div>
      <div className="row wrap">{netBoxes(noShort, setNoShort)}</div>

      <div className="row small">允许的改装 (成本 / 互斥条件):</div>
      {mods.map((m) => (
        <div key={m.id} className="netst">
          <div className="row">
            <b>{m.id}</b>
            <select value={m.kind} onChange={(e) => updateMod(m.id, { kind: e.target.value as RepairMod['kind'], target: '' })}>
              {Object.entries(kindLabel).map(([k, l]) => <option key={k} value={k}>{l}</option>)}
            </select>
            <select value={m.target} onChange={(e) => updateMod(m.id, { target: e.target.value })}>
              <option value="">(选择对象)</option>
              {(m.kind === 'jumper' ? b.jumpers : b.pads).map((it) => (
                <option key={it.id} value={it.id}>{it.id}</option>
              ))}
            </select>
            <label className="small">成本
              <input type="number" min={0} style={{ width: '4em' }} value={m.cost}
                onChange={(e) => updateMod(m.id, { cost: Math.max(0, parseInt(e.target.value || '0', 10) || 0) })} />
            </label>
            <button className="linkbtn" onClick={() => setMods(mods.filter((x) => x.id !== m.id))}>删除</button>
          </div>
          {mods.length > 1 && (
            <div className="row wrap small">
              互斥:
              {mods.filter((x) => x.id !== m.id).map((x) => (
                <label key={x.id} className="chk">
                  <input type="checkbox" checked={m.excludes.includes(x.id)}
                    onChange={(e) => updateMod(m.id, {
                      excludes: e.target.checked ? [...m.excludes, x.id] : m.excludes.filter((y) => y !== x.id),
                    })} />
                  {x.id}
                </label>
              ))}
            </div>
          )}
        </div>
      ))}
      <div className="row">
        <button onClick={() => setMods([...mods, { id: 'm' + (mods.length + 1) + '-' + uid('').slice(-4), kind: 'jumper', target: '', cost: 1, excludes: [] }])}>
          + 改装项
        </button>
        <button className="primary" disabled={mods.length === 0 || mustConnect.length + noShort.length === 0} onClick={solve}>
          求解最低成本组合
        </button>
      </div>
      {msg && <div className="row small status">{msg}</div>}

      {result && (
        <div className="netst">
          <b>求解结果 (修订 {result.revision} · {variantName(result.variantId)})</b>
          {stale && <div className="row small status">依据已过期: 板在求解后被修改, 请重新求解后再应用</div>}
          {result.warnings.length > 0 && (
            <div className="row small status">资料缺失: {result.warnings.join('; ')}</div>
          )}
          <div className="row small">每项改装单独预览:</div>
          {result.modEffects.map((eff) => (
            <div key={eff.modId} className="small">
              <b>{eff.modId}</b> (成本 {eff.cost})
              {changesList(eff)}
              {eff.conflicts.map((c, i) => <div key={i} className="status">冲突: {c}</div>)}
            </div>
          ))}
          {result.solutions.length > 0 && (
            <>
              <div className="row small">最低成本组合 ({result.solutions.length} 个, 成本 {result.solutions[0].cost}):</div>
              {result.solutions.map((sol, i) => (
                <div key={i} className="netst">
                  <div className="row">
                    <b>组合 {i + 1}: {sol.mods.length === 0 ? '(无需改装)' : sol.mods.join(' + ')}</b>
                    <button className="primary" disabled={stale} onClick={() => apply(sol.mods)}>应用该组合</button>
                  </div>
                  {changesList(sol)}
                </div>
              ))}
            </>
          )}
          {result.solutions.length === 0 && (
            <>
              <div className="row small status">无可行组合, 相互冲突的要求:</div>
              {result.conflicts.map((c, i) => <div key={i} className="small">· {c}</div>)}
            </>
          )}
        </div>
      )}

      {saved.length > 0 && (
        <div className="small">
          已保存的求解报告 (旧报告仍代表原修订):
          {saved.map((sr, i) => (
            <div key={sr.id} className="row small">
              #{i + 1} · 修订 {sr.revision} · {sr.result.solutions.length} 个组合
              {sr.revision !== b.revision && <span> · 依据已过期</span>}
              <button className="linkbtn" onClick={() => {
                setMustConnect(sr.spec.mustConnect ?? [])
                setNoShort(sr.spec.noShort ?? [])
                setMods(sr.spec.mods ?? [])
                setResult(sr.result)
                setStale(sr.revision !== b.revision)
              }}>查看</button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

