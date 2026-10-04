import { useEffect } from 'react'
import { api } from './api'
import { normalizeBoard, normalizeStoredCompares } from './normalize'
import Canvas from './Canvas'
import Sidebar from './Sidebar'
import { useStore } from './store'

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
    // Recover the compare reports frozen with the project: the latest
    // one is shown right away, flagged stale if the board has changed
    // since it was computed.
    api.listCompares()
      .then((raw) => {
        const { reports, warnings } = normalizeStoredCompares(raw)
        const st = useStore.getState()
        st.setCompares(reports)
        const last = reports[reports.length - 1]
        if (last) st.restoreCompare(last)
        if (warnings.length) setStatus({ status: '比较报告部分缺失, 已恢复其余报告: ' + warnings.join('; ') })
      })
      .catch(() => { /* no stored reports yet */ })
  }, [setBoard, setStatus])
  return (
    <div className="app">
      <Sidebar />
      <Canvas />
    </div>
  )
}
