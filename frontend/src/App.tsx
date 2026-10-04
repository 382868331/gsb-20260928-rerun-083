import { useEffect } from 'react'
import { api } from './api'
import { normalizeBoard, normalizeCompare } from './normalize'
import Canvas from './Canvas'
import Sidebar from './Sidebar'
import { useStore } from './store'
import type { StoredCompare } from './types'

export default function App() {
  const setBoard = useStore((s) => s.setBoard)
  const setStatus = useStore((s) => s.set)
  useEffect(() => {
    api.getBoard()
      .then((raw) => {
        const { board, warnings } = normalizeBoard(raw)
        setBoard(board)
        if (warnings.length) setStatus({ status: '板资料部分缺失, 已保留其余内容: ' + warnings.join('; ') })
      })
      .catch((e) => setStatus({ status: '后端连接失败: ' + e.message }))
    // Recover compare reports frozen with the project. Each entry is
    // normalized on its own, so one damaged record never costs the rest.
    api.listCompares()
      .then(({ compares, warnings }) => {
        const ok: StoredCompare[] = []
        let dropped = 0
        for (const sc of compares ?? []) {
          try {
            const { board } = normalizeBoard(sc.basis)
            ok.push({ ...sc, basis: board, report: normalizeCompare(sc.report) })
          } catch {
            dropped++
          }
        }
        const st = useStore.getState()
        st.set({ savedCompares: ok })
        const last = ok[ok.length - 1]
        if (last) st.restoreCompare(last)
        const notes = [...(warnings ?? [])]
        if (dropped) notes.push(dropped + ' 份比较报告无法恢复, 已跳过')
        if (notes.length) st.set({ status: '比较报告恢复: ' + notes.join('; ') })
      })
      .catch(() => { /* 无历史报告或后端较旧, 忽略 */ })
  }, [setBoard, setStatus])
  return (
    <div className="app">
      <Sidebar />
      <Canvas />
    </div>
  )
}
