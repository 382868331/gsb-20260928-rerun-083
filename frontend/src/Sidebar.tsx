import { useRef } from 'react'
import { api } from './api'
import { MM, uid, useStore } from './store'
import type { Tool } from './types'

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
  if (!b) return <aside className="sidebar" />

  const run = async (fn: () => Promise<void>, ok: string) => {
    try { await fn() } catch (e) { s.set({ status: '错误: ' + (e as Error).message }) ; return }
    s.set({ status: ok })
  }

  const save = () => run(async () => { const nb = await api.saveBoard(b); useStore.setState({ board: nb }) }, '已保存 (修订 ' + (b.revision + 1) + ')')
  const check = () => run(async () => {
    const r = await api.checkBoard(b)
    s.set({ errors: r.errors })
  }, '')
  const doCheck = async () => {
    await check()
    const n = useStore.getState().errors.length
    s.set({ status: n === 0 ? '检查通过: 无开路/短路' : `发现 ${n} 个问题` })
  }
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
      s.setBoard(nb)
    }, '导入完成, 元件身份已保留')
  const reset = () => run(async () => s.setBoard(await api.resetBoard()), '已重置为验证板')

  const focusError = (x: number, y: number) => {
    const zoom = Math.max(s.view.zoom, 4)
    const k = (30 / MM) * zoom
    s.setView({ zoom, panX: window.innerWidth * 0.35 - x * k, panY: window.innerHeight / 2 - y * k })
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
          <button className="primary" onClick={doCheck}>检查连通/短路</button>
        </div>
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

      <div className="group">
        <h2>检查结果 ({s.errors.length})</h2>
        {s.errors.length === 0 && <div className="row small">无问题, 或尚未运行检查</div>}
        {s.errors.map((er, i) => (
          <div key={i} className={'err ' + er.type} onClick={() => focusError((er.x + er.x2) / 2, (er.y + er.y2) / 2)}>
            <b>{er.type === 'short' ? '短路' : '开路'}</b> {er.message}
            <div className="small">位置 ({(er.x / MM).toFixed(3)}, {(er.y / MM).toFixed(3)}) mm · 元件: {er.elements.join(', ')}</div>
          </div>
        ))}
      </div>
    </aside>
  )
}
