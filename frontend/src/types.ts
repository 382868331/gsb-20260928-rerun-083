export interface Point {
  x: number;
  y: number;
}

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface Component {
  id: string;
  reference: string;
  footprint: string;
}

export interface Pad {
  id: string;
  componentId: string;
  name: string;
  net?: string;
  x: number;
  y: number;
  radius: number;
  holeRadius?: number;
  layers: string[];
}

export interface Trace {
  id: string;
  net?: string;
  layer: string;
  width: number;
  points: Point[];
}

export interface Via {
  id: string;
  net?: string;
  x: number;
  y: number;
  radius: number;
  layers: string[];
}

export interface Board {
  id: string;
  name: string;
  outline: Rect;
  components: Component[];
  pads: Pad[];
  traces: Trace[];
  vias: Via[];
}

export type IssueSeverity = "error" | "warning";

export interface Issue {
  severity: IssueSeverity;
  code: string;
  message: string;
  at?: Point;
  layer?: string;
  features?: string[];
}

export interface Contact {
  kind: string;
  at: Point;
  layer?: string;
  a: string;
  b: string;
  netA?: string;
  netB?: string;
}

export interface NetStatus {
  net: string;
  connected: boolean;
  padCount: number;
  groups: string[][];
  missingPads?: string[];
}

export interface DRCReport {
  netStatuses: NetStatus[];
  contacts: Contact[];
  issues: Issue[];
}

