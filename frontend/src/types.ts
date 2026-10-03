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
export interface Jumper {
  id: string; netId: string; layer: string; width: number; points: Point[]
}
export interface Variant {
  id: string; name: string; dnp: string[]; jumpers: string[]
}
export interface Board {
  id: string; name: string; revision: number
  outline: Point[]; layers: string[]
  nets: Net[]; pads: Pad[]; traces: Trace[]; vias: Via[]
  jumpers: Jumper[]; variants: Variant[]
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
export interface CheckReport {
  errors: CheckError[]; netStatus: NetStatus[]
  variantId?: string; revision?: number
}
export interface VariantSelection { dnp: string[]; jumpers: string[] }
export interface VariantReport {
  variantId: string; name: string; revision: number
  status?: 'ok' | 'empty' | 'missing'
  notes?: string[]
  selection?: VariantSelection
  errors: CheckError[]; netStatus: NetStatus[]
}
export interface NetDiff {
  netId: string; netName: string
  states: Record<string, string>
  onlyOpen: string[]; onlyShort: string[]
  paths?: Record<string, PathStep[]>
}
export interface CompareReport {
  revision: number; variants: VariantReport[]; diff: NetDiff[]
}
export type Tool = 'select' | 'trace' | 'pad' | 'via' | 'outline' | 'jumper'
export interface Sel { kind: 'pad' | 'trace' | 'via' | 'jumper'; id: string }
