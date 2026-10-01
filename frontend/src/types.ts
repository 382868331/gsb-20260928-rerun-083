export interface Point { x: number; y: number }
export interface Net { id: string; name: string }
export interface Pad {
  id: string; netId: string; layer: string; shape: 'circle' | 'rect'
  x: number; y: number; diameter: number; w: number; h: number; drill: number
}
export interface Trace {
  id: string; netId: string; layer: string; width: number; points: Point[]
}
export interface Via {
  id: string; netId: string; x: number; y: number
  diameter: number; drill: number; layers: string[]
}
export interface Board {
  id: string; name: string; revision: number
  outline: Point[]; layers: string[]
  nets: Net[]; pads: Pad[]; traces: Trace[]; vias: Via[]
}
export interface CheckError {
  type: 'open' | 'short'; message: string
  nets: string[]; elements: string[]
  steps?: PathStep[]
  x: number; y: number; x2: number; y2: number
}
export interface PathStep {
  kind: 'element' | 'hop'
  id: string
  netId: string
  netName: string
  layer: string
  x: number; y: number
}
export interface NetStatus {
  netId: string
  netName: string
  state: 'connected' | 'open' | 'short'
  components: number
  shortedWith: string[]
  elements: string[]
}
export interface CheckReport { errors: CheckError[]; netStatus: NetStatus[] }
export type Tool = 'select' | 'trace' | 'pad' | 'via' | 'outline'
export interface Sel { kind: 'pad' | 'trace' | 'via'; id: string }
