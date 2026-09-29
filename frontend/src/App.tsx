import { useEffect } from 'react'
import { api } from './api'
import Canvas from './Canvas'
import Sidebar from './Sidebar'
import { useStore } from './store'

export default function App() {
  const setBoard = useStore((s) => s.setBoard)
  const setStatus = useStore((s) => s.set)
  useEffect(() => {
    api.getBoard().then(setBoard).catch((e) => setStatus({ status: '后端连接失败: ' + e.message }))
  }, [setBoard, setStatus])
  return (
    <div className="app">
      <Sidebar />
      <Canvas />
    </div>
  )
}
