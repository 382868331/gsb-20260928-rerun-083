import { useEffect, useRef } from 'react'
import { api } from './api'
import { normalizeBoard } from './normalize'
import { MM, uid, useStore } from './store'
import type { CheckReport, NetStatus, Tool } from './types'

const TOOLS: { id: Tool; label: string }[] = [
  { id: 'select', label: '选择/移动' },
  { id: 'trace', label: '铜线' },
  { id: 'pad', label: '焊盘' },
  { id: 'via', label: '过孔' },
  { id: 'outline', label: '板框' },
]

export default function Sidebar() {
  const s = useStore()
  const b = s.board
  const fileRef = useRef<HTMLInputElement>(null)

  const run = async (fn: () => Promise<void>, ok: string) => {
    try { await fn() } catch (e) { s.set({ status: '错误: ' + (e as Error).message }) ; return }
    s.set({ status: ok })
  }

  // Reads the live store at call time so it stays valid regardless of
  // which render scheduled it (and never closes over a null board).
  const doCheck = async (remember = false) => {
    const st = useStore.getState()
    const bd = st.board
    if (!bd) return
    try {
      const r: CheckReport = await api.checkBoard(bd, st.asmId)
      st.setReport(r, remember)
      const n = r.errors.length
      st.set({ status: n === 0 ? '即时复核: 无开路/短路' : `即时复核: 发现 ${n} 个问题` })
    } catch (e) {
      st.set({ status: '复核失败: ' + (e as Error).message })
    }
  }

  // Instant re-verification after every local edit (place/move/delete,
  // undo/redo, layer or net change) and after every variant switch, so
  // each assembly's verdict reflects its own mounted set and board
  // revision. Geometry-only checks: zoom never affects the verdict.
  const firstRun = useRef(true)
  useEffect(() => {
    if (!useStore.getState().board) return
    if (firstRun.current) { firstRun.current = false; return }
    const h = setTimeout(() => { void doCheck(true) }, 200)
    return () => clearTimeout(h)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [b, s.asmId])

  if (!b) return <aside className="sidebar" />

  const save = () => run(async () => { const nb = await api.saveBoard(b); useStore.setState({ board: nb }) }, '已保存 (修订 ' + (b.revision + 1) + ')')
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
      const parsed = JSON.parse(await f.text())
      const nb = await api.importBoard(parsed)
      const { board, warnings } = normalizeBoard(nb)
      s.setBoard(board)
      if (warnings.length > 0) s.set({ warnings })
    }, '导入完成, 元件身份已保留')
  const reset = () => run(async () => s.setBoard(await api.resetBoard()), '已重置为验证板')

  const addAssembly = () => {
    const name = prompt('新装配方案名称')
    if (name) s.commit((bb) => { bb.assemblies.push({ id: uid('asm'), name, exclude: [] }) })
  }

  // Re-check every variant against the same shared geometry and surface
  // the nets that short or open only under some stuffing options.
  const doCompare = async () => {
    try {
      const cr = await api.compareAssemblies(b)
      useStore.setState((st) => ({ compare: cr, reports: { ...st.reports, ...cr.reports } }))
      s.set({
        status: cr.diffs.length === 0
          ? '方案比较: 各装配方案连通结论一致'
          : `方案比较: ${cr.diffs.length} 个网络只在部分方案中短接/断开`,
      })
    } catch (e) {
      s.set({ status: '比较失败: ' + (e as Error).message })
    }
  }

  const focusError = (x: number, y: number) => {
    const zoom = Math.max(s.view.zoom, 4)
    const k = (30 / MM) * zoom
    s.setView({ zoom, panX: window.innerWidth * 0.35 - x * k, panY: window.innerHeight / 2 - y * k })
  }

  // Diff the latest per-net states against those remembered before the
  // last edit: surfaces exactly which nets opened, shorted or recovered.
  const stateLabel: Record<string, string> = { connected: '连通', open: '开路', short: '短路' }
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

  const mmInput = (v: number, cb: (n: number) => void) => (
    <input type="number" step={0.1} value={+(v / MM).toFixed(3)}
      onChange={(e) => cb(Math.round(parseFloat(e.target.value || '0') * MM))} />
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
          <button className="primary" onClick={() => doCheck()}>检查连通/短路</button>
        </div>
        <div className="row">
          <button onClick={exportJson}>导出</button>
          <button onClick={() => fileRef.current?.click()}>导入</button>
          <button onClick={reset}>重置示例</button>
          <input ref={fileRef} type="file" accept=".json" hidden
            onChange={(e) => { const f = e.target.files?.[0]; if (f) importJson(f); e.target.value = '' }} />
        </div>
        {s.status && <div className="row small status">{s.status}</div>}
        {s.warnings.length > 0 && (
          <div className="warnings">
            {s.warnings.map((w, i) => <div key={i} className="row small">⚠ {w}</div>)}
          </div>
        )}
      </div>

      <div className="group">
        <h2>装配方案</h2>
        <label>当前方案
          <select value={s.asmId} onChange={(e) => s.setAssembly(e.target.value)}>
            <option value="">基板 (全部安装)</option>
            {b.assemblies.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
          </select>
        </label>
        {s.asmId && (
          <div className="row small">
            未安装 {b.assemblies.find((a) => a.id === s.asmId)?.exclude.length ?? 0} 个元件;
            选中元件后可按方案勾选是否安装
          </div>
        )}
        <div className="row">
          <button onClick={addAssembly}>+ 方案</button>
          <button onClick={doCompare}>比较所有方案</button>
        </div>
        {s.compare && (
          <div>
            {s.compare.diffs.length === 0 && <div className="row small">各方案连通结论一致</div>}
            {s.compare.diffs.map((d) => (
              <div key={d.netId + d.kind} className={'diff ' + d.kind}>
                <b>{d.netName}</b> {d.kind === 'short' ? '短路' : '开路'}仅出现在: {d.onlyIn.join('、')}
                <div className="row wrap">
                  {d.states.map((st) => (
                    <button key={st.assemblyId} className="linkbtn"
                      onClick={() => s.setAssembly(st.assemblyId)}>
                      {st.assemblyName}: {stateLabel[st.state] ?? st.state}
                    </button>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

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

      {sel && (selPad || selTrace || selVia) && (
        <div className="group">
          <h2>选中: {selPad ? '焊盘' : selTrace ? '铜线' : '过孔'} {(selPad ?? selTrace ?? selVia)!.id}</h2>
          <label>网络
            <select value={(selPad ?? selTrace ?? selVia)!.netId}
              onChange={(e) => s.commit((bb) => {
                const it = sel.kind === 'pad' ? bb.pads.find((x) => x.id === sel.id)
                  : sel.kind === 'trace' ? bb.traces.find((x) => x.id === sel.id)
                  : bb.vias.find((x) => x.id === sel.id)
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
          {b.assemblies.length > 0 && (
            <>
              <div className="row small">各方案安装:</div>
              <div className="row wrap">
                {b.assemblies.map((a) => (
                  <label key={a.id} className="chk">
                    <input type="checkbox" checked={!a.exclude.includes(sel.id)}
                      onChange={(e) => s.commit((bb) => {
                        const asm = bb.assemblies.find((x) => x.id === a.id)
                        if (!asm) return
                        asm.exclude = e.target.checked
                          ? asm.exclude.filter((id) => id !== sel.id)
                          : [...asm.exclude, sel.id]
                      })} />
                    {a.name}
                  </label>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      {s.netStatus.length > 0 && (
        <div className="group">
          <h2>网络连通复核 ({s.netStatus.length}){s.asmId ? ' · ' + (b.assemblies.find((a) => a.id === s.asmId)?.name ?? s.asmId) : ''}</h2>
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
