import { useEffect } from 'react'
import { api } from './api'
import { normalizeBoard } from './normalize'
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
  }, [setBoard, setStatus])
  return (
    <div className="app">
      <Sidebar />
      <Canvas />
    </div>
  )
}
