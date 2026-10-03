import { useEffect, useRef, useState } from 'react'
import { api } from './api'
import { normalizeBoard, normalizeCompare } from './normalize'
import { compareKeyOf, MM, uid, useStore } from './store'
import type { NetPathReport, NetStatus, PathStep, Tool } from './types'

const TOOLS: { id: Tool; label: string }[] = [
  { id: 'select', label: '选择/移动' },
  { id: 'trace', label: '铜线' },
  { id: 'pad', label: '焊盘' },
  { id: 'via', label: '过孔' },
  { id: 'jumper', label: '跳线' },
  { id: 'outline', label: '板框' },
]

export default function Sidebar() {
  const s = useStore()
  const b = s.board
  const fileRef = useRef<HTMLInputElement>(null)

  // Hooks must run in the same order on every render, including while
  // the board is still loading — returning early above them breaks the
  // hook sequence and kills the whole canvas on first load.
  const firstRun = useRef(true)
  const [netPathView, setNetPathView] = useState<{ rep: NetPathReport; frozen: boolean } | null>(null)
  useEffect(() => {
    if (!b) return
    if (firstRun.current) { firstRun.current = false; return }
    const h = setTimeout(() => { doCheck(true) }, 200)
    return () => clearTimeout(h)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [b, s.variantId])

  if (!b) {
    return (
      <aside className="sidebar">
        <h1>电路板工作台</h1>
        {s.status && <div className="row small status">{s.status}</div>}
      </aside>
    )
  }

  const run = async (fn: () => Promise<void>, ok: string) => {
    try { await fn() } catch (e) { s.set({ status: '错误: ' + (e as Error).message }) ; return }
    s.set({ status: ok })
  }

  const save = () => run(async () => { const nb = await api.saveBoard(b); useStore.setState({ board: nb }) }, '已保存 (修订 ' + (b.revision + 1) + ')')

  const variantName = (id: string) =>
    id === '' ? '基准 (全部装配, 无跳线)' : (b.variants.find((v) => v.id === id)?.name ?? id)

  // Re-checks the CURRENT board revision against the CURRENT variant.
  // The response is applied only when both are still current, so a slow
  // reply from a previous variant or pre-edit revision is discarded
  // instead of overwriting fresh conclusions.
  const doCheck = async (remember = false) => {
    const board = useStore.getState().board
    if (!board) return
    const variantId = useStore.getState().variantId
    try {
      const r = await api.checkVariant(board, variantId)
      const st = useStore.getState()
      if (st.board !== board || st.variantId !== variantId) return
      s.setReport(r, remember)
      s.set({ reportFor: { variantId, revision: board.revision } })
      const n = r.errors.length
      s.set({ status: `${variantName(variantId)}: ${n === 0 ? '无开路/短路' : `发现 ${n} 个问题`} (修订 ${board.revision})` })
    } catch (e) {
      s.set({ status: '复核失败: ' + (e as Error).message })
    }
  }

  const compareAll = () =>
    run(async () => {
      const rep = normalizeCompare(await api.compareVariants(b))
      s.setCompare(rep, b)
      setNetPathView(null)
    }, '已比较 ' + (b.variants.length + 1) + ' 个装配方案 (修订 ' + b.revision + ')')
  const exportJson = () => {
    const blob = new Blob([JSON.stringify(b, null, 2)], { type: 'application/json' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = (b.name || 'board') + '.json'
    a.click()
    URL.revokeObjectURL(a.href)
  }
  const importJson = (f: File) =>
    run(async () => {
      const { board: parsed, warnings } = normalizeBoard(JSON.parse(await f.text()))
      const nb = await api.importBoard(parsed)
      s.setBoard(nb)
      if (warnings.length) s.set({ status: '导入文件部分缺失, 已保留其余内容: ' + warnings.join('; ') })
    }, '导入完成, 元件身份已保留')
  const reset = () => run(async () => s.setBoard(await api.resetBoard()), '已重置为验证板')

  const focusError = (x: number, y: number) => {
    const zoom = Math.max(s.view.zoom, 4)
    const k = (30 / MM) * zoom
    s.setView({ zoom, panX: window.innerWidth * 0.35 - x * k, panY: window.innerHeight / 2 - y * k })
  }

  // Diff the latest per-net states against those remembered before the
  // last edit: surfaces exactly which nets opened, shorted or recovered.
  const stateLabel: Record<string, string> = { connected: '连通', open: '开路', short: '短路', empty: '没有可用路径' }
  const prevMap = new Map(s.lastStatus.map((n) => [n.netId, n.state]))
  const transitionOf = (n: NetStatus): string => {
    const prev = prevMap.get(n.netId)
    if (!prev || prev === n.state) return ''
    const dir =
      n.state === 'connected' ? '已恢复连通'
      : n.state === 'short' ? '新发生短路'
      : '已断开'
    return ` (${stateLabel[prev] ?? prev} → ${stateLabel[n.state] ?? n.state}, ${dir})`
  }

  const sel = s.selected
  const selPad = sel?.kind === 'pad' ? b.pads.find((p) => p.id === sel.id) : undefined
  const selTrace = sel?.kind === 'trace' ? b.traces.find((t) => t.id === sel.id) : undefined
  const selVia = sel?.kind === 'via' ? b.vias.find((v) => v.id === sel.id) : undefined
  const selJumper = sel?.kind === 'jumper' ? b.jumpers.find((j) => j.id === sel.id) : undefined

  const activeVariant = b.variants.find((v) => v.id === s.variantId)
  const toggleVariantList = (field: 'dnp' | 'jumpers', id: string, on: boolean) =>
    s.commit((bb) => {
      const v = bb.variants.find((x) => x.id === s.variantId)
      if (!v) return
      const list = v[field]
      v[field] = on ? [...list, id] : list.filter((x) => x !== id)
    })

  const mmInput = (v: number, cb: (n: number) => void) => (
    <input type="number" step={0.1} value={+(v / MM).toFixed(3)}
      onChange={(e) => cb(Math.round(parseFloat(e.target.value || '0') * MM))} />
  )

  // A stored compare is stale once the board (geometry, revision or any
  // variant's assembly selection) no longer matches the frozen basis.
  const compareStale = s.compare !== null && s.compareKey !== null && s.compareKey !== compareKeyOf(b)

  const variantVerdict = (v: { status?: string; warnings?: string[]; errors: unknown[] }) => {
    if (v.status === 'incomplete') return '资料缺失: ' + (v.warnings ?? []).join('; ')
    if (v.status === 'empty') return '没有可用路径 (该方案无导电元素)'
    return v.errors.length === 0 ? '无故障' : v.errors.length + ' 个问题'
  }

  // Fault/connection paths are always traced on a stated basis: the
  // frozen report board or the current one — never silently swapped.
  const showNetPath = (netId: string, variantId: string, useCurrent: boolean) =>
    run(async () => {
      const basis = !useCurrent && s.compareBoard ? s.compareBoard : b
      const rep = await api.netPath(basis, variantId, netId)
      setNetPathView({ rep, frozen: basis !== b })
    }, '已获取导通路径')

  const stepsList = (steps: PathStep[]) => (
    <ol className="steps">
      {steps.map((st, j) => (
        <li key={j} className={st.kind} onClick={() => focusError(st.x, st.y)}>
          {st.kind === 'element' ? (
            <>{j === 0 ? '' : '└ '}<b>{st.id}</b> [{st.netName}] @ {st.layer} <span className="small">({(st.x / MM).toFixed(2)}, {(st.y / MM).toFixed(2)}) mm</span></>
          ) : (
            <span className="hop">↳ {st.layer} 层接触/间隙 @ ({(st.x / MM).toFixed(2)}, {(st.y / MM).toFixed(2)}) mm</span>
          )}
        </li>
      ))}
    </ol>
  )

  return (
    <aside className="sidebar">
      <h1>电路板工作台</h1>
      <div className="row small">{b.name} · 修订 {b.revision}</div>

      <div className="group">
        <div className="row">
          <button onClick={save}>保存</button>
          <button onClick={s.undo} disabled={!s.past.length}>撤销</button>
          <button onClick={s.redo} disabled={!s.future.length}>重做</button>
        </div>
        <div className="row">
          <button className="primary" onClick={() => doCheck()}>检查当前方案</button>
          <button onClick={compareAll}>比较各方案</button>
        </div>
        {s.reportFor && (
          <div className="row small">结果对应: {variantName(s.reportFor.variantId)} · 修订 {s.reportFor.revision}</div>
        )}
        <div className="row">
          <button onClick={exportJson}>导出</button>
          <button onClick={() => fileRef.current?.click()}>导入</button>
          <button onClick={reset}>重置示例</button>
          <input ref={fileRef} type="file" accept=".json" hidden
            onChange={(e) => { const f = e.target.files?.[0]; if (f) importJson(f); e.target.value = '' }} />
        </div>
        {s.status && <div className="row small status">{s.status}</div>}
      </div>

      <div className="group">
        <h2>装配方案</h2>
        <label>当前方案
          <select value={s.variantId} onChange={(e) => s.setVariant(e.target.value)}>
            <option value="">基准 (全部装配, 无跳线)</option>
            {b.variants.map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}
          </select>
        </label>
        <div className="row">
          <button onClick={() => {
            const name = prompt('新方案名称')
            if (name) s.commit((bb) => { bb.variants.push({ id: uid('var'), name, dnp: [], jumpers: [] }) })
          }}>+ 方案</button>
          {activeVariant && (
            <button onClick={() => {
              if (confirm('删除方案 ' + activeVariant.name + '?')) {
                s.commit((bb) => { bb.variants = bb.variants.filter((v) => v.id !== activeVariant.id) })
                s.setVariant('')
              }
            }}>删除方案</button>
          )}
        </div>
        {activeVariant ? (
          <>
            {b.jumpers.length > 0 && <div className="row small">跳线 (安装后改变导电路径):</div>}
            {b.jumpers.map((j) => (
              <label key={j.id} className="chk">
                <input type="checkbox" checked={activeVariant.jumpers.includes(j.id)}
                  onChange={(e) => toggleVariantList('jumpers', j.id, e.target.checked)} />
                安装 {j.id}
              </label>
            ))}
            {b.pads.length > 0 && <div className="row small">器件 (不安装则从导电中移除):</div>}
            {b.pads.map((p) => (
              <label key={p.id} className="chk">
                <input type="checkbox" checked={activeVariant.dnp.includes(p.id)}
                  onChange={(e) => toggleVariantList('dnp', p.id, e.target.checked)} />
                不装 {p.id}
              </label>
            ))}
          </>
        ) : (
          <div className="row small">基准视图: 全部器件已装, 跳线不导通; 选择方案后可配置跳线与未安装器件</div>
        )}
      </div>

      {s.compare && (
        <div className="group">
          <h2>方案间差异 (修订 {s.compare.revision})</h2>
          {compareStale && !s.compareFrozen && (
            <div className="row small status">
              报告依据已过期: 板在比较后被修改 (当前修订 {b.revision})。
              <div className="row">
                <button onClick={() => s.set({ compareFrozen: true })}>保留冻结结果</button>
                <button className="primary" onClick={compareAll}>对当前板重新比较</button>
              </div>
            </div>
          )}
          {compareStale && s.compareFrozen && (
            <div className="row small status">
              已保留冻结结果, 依据修订 {s.compare.revision} (旧修订), 与当前板 (修订 {b.revision}) 不同。
              <button className="linkbtn" onClick={compareAll}>对当前板重新比较</button>
            </div>
          )}
          {(s.compare.warnings ?? []).length > 0 && (
            <div className="row small status">板资料部分缺失: {(s.compare.warnings ?? []).join('; ')}</div>
          )}
          <div className="small">
            {s.compare.variants.map((v) => (
              <div key={v.variantId} className="row small">
                {v.name}: {variantVerdict(v)} (修订 {v.revision})
              </div>
            ))}
          </div>
          {s.compare.diff.length === 0 && <div className="row small">各方案网络状态一致</div>}
          {s.compare.diff.map((d) => (
            <div key={d.netId} className="netst">
              <b>{d.netName}</b>
              {d.onlyShort.length > 0 && <div className="small">仅在 {d.onlyShort.map(variantName).join('、')} 短路</div>}
              {d.onlyOpen.length > 0 && <div className="small">仅在 {d.onlyOpen.map(variantName).join('、')} 断开</div>}
              <div className="small">
                {s.compare!.variants.map((v) => (
                  <div key={v.variantId}>{v.name}: {stateLabel[d.states[v.variantId]] ?? d.states[v.variantId]}</div>
                ))}
              </div>
              {(() => {
                const onlyConnected = s.compare!.variants.filter((v) => d.states[v.variantId] === 'connected')
                if (onlyConnected.length !== 1) return null
                const v = onlyConnected[0]
                return (
                  <button className="linkbtn"
                    onClick={() => showNetPath(d.netId, v.variantId, !(compareStale || s.compareFrozen))}>
                    查看仅在 {v.name} 导通的路径
                  </button>
                )
              })()}
            </div>
          ))}
          {netPathView && (
            <div className="netst">
              <b>导通路径: {netPathView.rep.netId} · {variantName(netPathView.rep.variantId)}</b>
              <div className="row small">
                {netPathView.frozen
                  ? '该路径来自旧修订 (修订 ' + netPathView.rep.revision + '), 非当前几何'
                  : '依据当前几何 (修订 ' + netPathView.rep.revision + ')'}
                {netPathView.frozen && (
                  <button className="linkbtn"
                    onClick={() => showNetPath(netPathView.rep.netId, netPathView.rep.variantId, true)}>
                    对当前几何重新查看
                  </button>
                )}
                <button className="linkbtn" onClick={() => setNetPathView(null)}>收起</button>
              </div>
              {netPathView.rep.state !== 'connected' && (
                <div className="row small">该网络在此方案中为 {stateLabel[netPathView.rep.state] ?? netPathView.rep.state}, 无完整导通路径</div>
              )}
              {stepsList(netPathView.rep.steps ?? [])}
            </div>
          )}
        </div>
      )}

      <div className="group">
        <h2>工具</h2>
        <div className="row wrap">
          {TOOLS.map((t) => (
            <button key={t.id} className={s.tool === t.id ? 'active' : ''} onClick={() => s.setTool(t.id)}>{t.label}</button>
          ))}
        </div>
        <div className="row small">画线/板框: 单击加点, 双击或回车完成, Esc 取消; 中键拖动平移, 滚轮缩放</div>
      </div>

      <div className="group">
        <h2>放置参数</h2>
        <label>层
          <select value={s.layer} onChange={(e) => s.set({ layer: e.target.value })}>
            {b.layers.map((l) => <option key={l} value={l}>{l}</option>)}
          </select>
        </label>
        <label>网络
          <select value={s.netId} onChange={(e) => s.set({ netId: e.target.value })}>
            <option value="">(未命名铜皮)</option>
            {b.nets.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
          </select>
        </label>
        <label>线宽 (mm) {mmInput(s.traceWidth, (n) => s.set({ traceWidth: n }))}</label>
        <label>焊盘形状
          <select value={s.padShape} onChange={(e) => s.set({ padShape: e.target.value as 'circle' | 'rect' })}>
            <option value="circle">圆形</option><option value="rect">矩形</option>
          </select>
        </label>
        <label>焊盘尺寸 (mm) {mmInput(s.padSize, (n) => s.set({ padSize: n }))}</label>
        <div className="row small">过孔连接层:</div>
        <div className="row wrap">
          {b.layers.map((l) => (
            <label key={l} className="chk">
              <input type="checkbox" checked={s.viaLayers.includes(l)}
                onChange={(e) => s.set({ viaLayers: e.target.checked ? [...s.viaLayers, l] : s.viaLayers.filter((x) => x !== l) })} />
              {l}
            </label>
          ))}
        </div>
        <div className="row">
          <button onClick={() => {
            const name = prompt('新网络名称')
            if (name) s.commit((bb) => { bb.nets.push({ id: uid('net'), name }) })
          }}>+ 网络</button>
        </div>
      </div>

      {sel && (selPad || selTrace || selVia || selJumper) && (
        <div className="group">
          <h2>选中: {selPad ? '焊盘' : selTrace ? '铜线' : selVia ? '过孔' : '跳线'} {(selPad ?? selTrace ?? selVia ?? selJumper)!.id}</h2>
          <label>网络
            <select value={(selPad ?? selTrace ?? selVia ?? selJumper)!.netId}
              onChange={(e) => s.commit((bb) => {
                const it = sel.kind === 'pad' ? bb.pads.find((x) => x.id === sel.id)
                  : sel.kind === 'trace' ? bb.traces.find((x) => x.id === sel.id)
                  : sel.kind === 'via' ? bb.vias.find((x) => x.id === sel.id)
                  : bb.jumpers.find((x) => x.id === sel.id)
                if (it) it.netId = e.target.value
              })}>
              <option value="">(未命名铜皮)</option>
              {b.nets.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
            </select>
          </label>
          {selTrace && (
            <>
              <label>所在层
                <select value={selTrace.layer} onChange={(e) => s.commit((bb) => { bb.traces.find((x) => x.id === sel.id)!.layer = e.target.value })}>
                  {b.layers.map((l) => <option key={l} value={l}>{l}</option>)}
                </select>
              </label>
              <label>线宽 (mm) {mmInput(selTrace.width, (n) => s.commit((bb) => { bb.traces.find((x) => x.id === sel.id)!.width = n }))}</label>
            </>
          )}
          {selPad && (
            <label>所在层
              <select value={selPad.layer} onChange={(e) => s.commit((bb) => { bb.pads.find((x) => x.id === sel.id)!.layer = e.target.value })}>
                {b.layers.map((l) => <option key={l} value={l}>{l}</option>)}
                <option value="through">through (全部层)</option>
              </select>
            </label>
          )}
          {selVia && (
            <div className="row wrap">
              {b.layers.map((l) => (
                <label key={l} className="chk">
                  <input type="checkbox" checked={selVia.layers.includes(l)}
                    onChange={(e) => s.commit((bb) => {
                      const v = bb.vias.find((x) => x.id === sel.id)!
                      v.layers = e.target.checked ? [...v.layers, l] : v.layers.filter((x) => x !== l)
                    })} />
                  {l}
                </label>
              ))}
            </div>
          )}
        </div>
      )}

      {s.netStatus.length > 0 && (
        <div className="group">
          <h2>网络连通复核 · {variantName(s.reportFor?.variantId ?? s.variantId)} ({s.netStatus.length})</h2>
          {s.netStatus.map((n) => (
            <div key={n.netId} className={'netst ' + n.state}>
              <b>{n.netName}</b>: {stateLabel[n.state] ?? n.state}
              {n.state === 'short' && <> · 与 {n.shortedWith.map((id) => b.nets.find((x) => x.id === id)?.name ?? id).join(', ')} 短接</>}
              {n.state === 'open' && <> · {n.components} 个独立铜皮域</>}
              <span className="small">{transitionOf(n)}</span>
            </div>
          ))}
          <div className="row small">移动/删除过孔或铜线后自动复核; 仅在存在有效跨层连接(过孔/通孔焊盘)时不同层才导通</div>
        </div>
      )}

      <div className="group">
        <h2>故障路径 ({s.errors.length})</h2>
        {s.errors.length === 0 && <div className="row small">无问题, 或尚未运行检查</div>}
        {s.errors.map((er, i) => (
          <div key={i} className={'err ' + er.type}>
            <div onClick={() => focusError((er.x + er.x2) / 2, (er.y + er.y2) / 2)}>
              <b>{er.type === 'short' ? '短路' : '开路'}</b> {er.message}
            </div>
            {er.steps && er.steps.length > 0 && (
              <ol className="steps">
                {er.steps.map((st, j) => (
                  <li key={j} className={st.kind} onClick={() => focusError(st.x, st.y)}>
                    {st.kind === 'element' ? (
                      <>{j === 0 ? '' : '└ '}<b>{st.id}</b> [{st.netName}] @ {st.layer} <span className="small">({(st.x / MM).toFixed(2)}, {(st.y / MM).toFixed(2)}) mm</span></>
                    ) : (
                      <span className="hop">↳ {st.layer} 层接触/间隙 @ ({(st.x / MM).toFixed(2)}, {(st.y / MM).toFixed(2)}) mm</span>
                    )}
                  </li>
                ))}
              </ol>
            )}
            <button className="linkbtn" onClick={() => s.set({ pathError: s.pathError === er ? null : er })}>
              {s.pathError === er ? '隐藏画布路径' : '在画布高亮完整导电路径'}
            </button>
          </div>
        ))}
      </div>
    </aside>
  )
}
