import { useEffect, useRef, useState } from "react";
import { useStore, LAYERS } from "./store";
import Canvas from "./Canvas";
import type { Board } from "./types";

function ToolButton({ tool, label, hint }: { tool: string; label: string; hint: string }) {
  const active = useStore((st) => st.tool === tool);
  const setTool = useStore((st) => st.setTool);
  return (
    <button
      className={active ? "tool active" : "tool"}
      title={hint}
      onClick={() => setTool(tool as never)}
    >
      {label}
    </button>
  );
}

export default function App() {
  const st = useStore();
  const fileRef = useRef<HTMLInputElement>(null);
  const [overwrite, setOverwrite] = useState(false);

  useEffect(() => {
    st.refreshList();
  }, []);

  const selectedPad = st.board?.pads.find((p) => p.id === st.selectedId);
  const selectedTrace = st.board?.traces.find((t) => t.id === st.selectedId);
  const selectedVia = st.board?.vias.find((v) => v.id === st.selectedId);
  const selectedNet = selectedPad?.net ?? selectedTrace?.net ?? selectedVia?.net ?? "";

  const toggleViaLayer = (layer: string) => {
    if (!selectedVia || !st.board) return;
    st.mutate((b: Board) => {
      const v = b.vias.find((q) => q.id === selectedVia.id);
      if (!v) return b;
      v.layers = v.layers.includes(layer)
        ? v.layers.filter((x) => x !== layer)
        : [...v.layers, layer];
      return b;
    });
  };

  const errorCount = st.report?.issues.filter((i) => i.severity === "error").length ?? 0;

  return (
    <div className="app">
      <header className="topbar">
        <strong>电路板设计工作台</strong>
        <select
          value={st.board?.id ?? ""}
          onChange={(e) => e.target.value && st.load(e.target.value)}
        >
          <option value="">选择已保存的板子…</option>
          {st.boards.map((b) => (
            <option key={b.id} value={b.id}>
              {b.name} ({b.id.slice(0, 6)})
            </option>
          ))}
        </select>
        <button onClick={st.create}>新建板子</button>
        <button disabled={!st.board} onClick={st.save}>保存</button>
        <button disabled={!st.board} onClick={st.undo}>撤销</button>
        <button disabled={!st.board} onClick={st.redo}>重做</button>
        <button disabled={!st.board} onClick={() => st.runDrc()}>检查连通/短路</button>
        <button disabled={!st.board} onClick={st.exportFile}>导出</button>
        <button onClick={() => fileRef.current?.click()}>导入</button>
        <label className="inline">
          <input type="checkbox" checked={overwrite} onChange={(e) => setOverwrite(e.target.checked)} />
          覆盖同 ID
        </label>
        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          style={{ display: "none" }}
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) st.importFile(f, overwrite);
            e.target.value = "";
          }}
        />
        <span className={st.dirty ? "status dirty" : "status"}>{st.status}</span>
      </header>

      <div className="main">
        <aside className="left">
          <section>
            <h3>工具</h3>
            <div className="tools">
              <ToolButton tool="select" label="选择/平移" hint="点击选中，拖拽空白平移，滚轮缩放" />
              <ToolButton tool="pad" label="放置焊盘" hint="在当前层放置圆形焊盘" />
              <ToolButton tool="trace" label="手工连线" hint="逐点单击走线，右键/回车完成，Esc 取消" />
              <ToolButton tool="via" label="放置过孔" hint="过孔按声明层连接，不声明就不跨层" />
            </div>
            <h3>工作层</h3>
            <div className="tools">
              {LAYERS.map((l) => (
                <button
                  key={l}
                  className={st.layer === l ? "tool layer active " + l : "tool layer " + l}
                  onClick={() => {
                    st.setTool("select");
                    st.setLayer(l);
                  }}
                >
                  {l === "top" ? "顶层 top" : l === "mid" ? "中间层 mid" : "底层 bottom"}
                </button>
              ))}
            </div>
            <p className="hint">不同层铜线几何交叉不会相连；只有过孔把其声明的层短接。</p>
          </section>

          <section>
            <h3>参数 (mm)</h3>
            <label>线宽
              <input type="number" step="0.1" value={st.traceWidth}
                onChange={(e) => st.setParam({ traceWidth: Number(e.target.value) })} />
            </label>
            <label>焊盘半径
              <input type="number" step="0.1" value={st.padRadius}
                onChange={(e) => st.setParam({ padRadius: Number(e.target.value) })} />
            </label>
            <label>过孔半径
              <input type="number" step="0.1" value={st.viaRadius}
                onChange={(e) => st.setParam({ viaRadius: Number(e.target.value) })} />
            </label>
            <label>最小间隙
              <input type="number" step="0.05" value={st.minClearance}
                onChange={(e) => st.setParam({ minClearance: Number(e.target.value) })} />
            </label>
          </section>

          {st.board && (
            <section>
              <h3>板框</h3>
              <label>宽
                <input type="number" value={st.board.outline.width}
                  onChange={(e) => st.setOutline({ width: Number(e.target.value) })} />
              </label>
              <label>高
                <input type="number" value={st.board.outline.height}
                  onChange={(e) => st.setOutline({ height: Number(e.target.value) })} />
              </label>
              <label>板名
                <input value={st.board.name}
                  onChange={(e) => st.mutate((b) => ({ ...b, name: e.target.value }))} />
              </label>
            </section>
          )}

          {st.selectedId && (
            <section>
              <h3>选中对象 {st.selectedId.slice(0, 10)}</h3>
              <label>网络名称
                <input
                  value={selectedNet}
                  placeholder="（无网络）"
                  onChange={(e) => st.setSelectedNet(e.target.value)}
                />
              </label>
              {selectedVia && (
                <div className="via-layers">
                  <p>过孔声明连接层（至少两层才跨层）：</p>
                  {LAYERS.map((l) => (
                    <label key={l} className="inline">
                      <input
                        type="checkbox"
                        checked={selectedVia.layers.includes(l)}
                        onChange={() => toggleViaLayer(l)}
                      />
                      {l}
                    </label>
                  ))}
                </div>
              )}
              {selectedPad && <p className="hint">焊盘层：{selectedPad.layers.join(", ")}</p>}
              {selectedTrace && (
                <p className="hint">导线层：{selectedTrace.layer}，{selectedTrace.points.length} 个节点</p>
              )}
              <button className="danger" onClick={st.deleteSelected}>删除</button>
            </section>
          )}
        </aside>

        <Canvas />

        <aside className="right">
          <section>
            <h3>实际网络连通</h3>
            {!st.report && <p className="hint">保存或点击检查后显示。连通只按几何接触计算。</p>}
            {st.report?.netStatuses.map((ns) => (
              <div key={ns.net} className={ns.connected ? "net ok" : "net bad"}>
                <strong>{ns.net}</strong>
                <span>
                  {ns.connected
                    ? "已实际连通"
                    : "未连通：" + ns.groups.length + " 个独立铜片段"}
                </span>
                {!ns.connected && (
                  <ul>
                    {ns.groups.map((g, i) => (
                      <li key={i}>[{g.join(", ")}]</li>
                    ))}
                  </ul>
                )}
              </div>
            ))}
          </section>

          <section>
            <h3>
              DRC 问题
              {errorCount > 0 ? <span className="badge bad">{errorCount}</span> : <span className="badge ok">0</span>}
            </h3>
            <div className="issues">
              {st.report?.issues.map((iss, i) => (
                <button
                  key={i}
                  className={"issue-row " + iss.severity + (st.focusedIssue === i ? " focus" : "")}
                  onClick={() => st.focusIssue(st.focusedIssue === i ? null : i)}
                >
                  <span className="code">{iss.code}</span>
                  <span className="msg">{iss.message}</span>
                </button>
              ))}
              {st.report && st.report.issues.length === 0 && (
                <p className="hint">无短路、无细间隙，声明网络全部实际接通。</p>
              )}
            </div>
          </section>

          <section>
            <h3>物理接触点 ({st.report?.contacts.length ?? 0})</h3>
            <div className="contacts">
              {st.report?.contacts.map((c, i) => (
                <div key={i} className="contact">
                  {c.kind} · {c.layer || "-"} · ({c.at.x.toFixed(2)}, {c.at.y.toFixed(2)})
                  <br />
                  <small>
                    {c.a} ↔ {c.b}
                    {c.netA || c.netB ? " [" + (c.netA || "-") + "/" + (c.netB || "-") + "]" : ""}
                  </small>
                </div>
              ))}
            </div>
          </section>
        </aside>
      </div>
    </div>
  );
}

