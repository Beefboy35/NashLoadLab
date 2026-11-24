export type ExactValue = string

export type LatencyKind = 'linear' | 'quadratic' | 'table'
export type ImprovementPolicy = 'first_improvement' | 'best_response'
export type SimulationStatus = 'equilibrium' | 'step_limit' | 'cancelled' | 'error'
export type CalculationStatus = 'exact' | 'not_computed' | 'cancelled' | 'error'

export interface NodeInput {
  id: string
  name: string
  a: ExactValue
  b: ExactValue
  kind: LatencyKind
  /**
   * Значения T(1), …, T(n) для табличного вида функции. Для линейного и
   * квадратичного вида поле не передаётся в API.
   */
  table_values?: ExactValue[]
}

export interface SimulationRequest {
  players: number
  nodes: NodeInput[]
  initial_assignment: string[]
  algorithm: ImprovementPolicy
  max_steps: number
  seed?: number
}

export interface Preset {
  id: string
  name: string
  description: string
  request: SimulationRequest
}

export interface PresetsResponse {
  presets: Preset[]
}

export interface PreviewRequest {
  players: number
  nodes: NodeInput[]
}

export interface PreviewCell {
  node_id: string
  value: ExactValue
}

export interface PreviewRow {
  load: number
  values: PreviewCell[]
}

export interface PreviewResponse {
  rows: PreviewRow[]
}

export interface NodeLoad {
  node_id: string
  node_name?: string
  load: number
  latency?: ExactValue
}

export interface TraceStep {
  step: number
  player: number
  from: string
  to: string
  previous: ExactValue
  next: ExactValue
  delta: ExactValue
  potential: ExactValue
  total_time: ExactValue
  /** Complete server-calculated state after this move. */
  summary: AllocationSummary
}

export interface NashCheck {
  is_equilibrium: boolean
  checked_moves?: number
  explanation?: string
  violations?: ImprovingMove[]
}

export interface ImprovingMove {
  player: number
  from: string
  to: string
  current: ExactValue
  candidate: ExactValue
}

export interface AllocationSummary {
  assignment?: string[]
  loads?: NodeLoad[]
  potential?: ExactValue
  total_time?: ExactValue
}

export interface SimulationResult {
  id?: string
  request: SimulationRequest
  initial?: AllocationSummary
  trace: TraceStep[]
  final: AllocationSummary
  nash_check: NashCheck
  status?: SimulationStatus
  message?: string
  created_at?: string
}

export interface OptimizationRequest {
  players: number
  nodes: NodeInput[]
  initial_assignment?: string[]
  algorithm?: ImprovementPolicy
  max_steps?: number
  max_states: number
}

export interface OptimizationValue extends AllocationSummary {
  status: CalculationStatus
  message?: string
}

export interface OptimizationResult {
  optimum: OptimizationValue
  equilibrium?: AllocationSummary
  k_run?: ExactValue
  poa: OptimizationValue & { value?: ExactValue; worst_equilibrium?: AllocationSummary }
}

export interface NumberRange {
  from: number
  to: number
  step: number
}

export interface DecimalRange {
  from: ExactValue
  to: ExactValue
  step: ExactValue
}

export interface ExperimentRequest {
  players: NumberRange
  nodes: NumberRange
  a: DecimalRange
  b: DecimalRange
  algorithm: ImprovementPolicy
  seed: number
  max_steps: number
}

export interface ExperimentPoint {
  players: number
  nodes: number
  a: ExactValue
  b: ExactValue
  steps?: number
  k_run?: ExactValue
  total_time_ne?: ExactValue
  total_time_opt?: ExactValue
  status?: CalculationStatus
}

export interface ExperimentSummary {
  id: string
  status: 'queued' | 'running' | 'complete' | 'cancelled' | 'error'
  created_at: string
  updated_at?: string
  request?: ExperimentRequest
  points?: ExperimentPoint[]
  message?: string
}

export interface ExperimentListResponse {
  experiments: ExperimentSummary[]
}

export interface HealthResponse {
  status: string
  version?: string
}

/** Данные готовой демонстрации защиты, полностью рассчитанные сервером. */
export interface PresentationLinearPreviewPoint {
  load: number
  a: ExactValue
  b: ExactValue
}

export interface PresentationEquilibriumComparisonPoint {
  profile: string
  total_time: ExactValue
}

export interface PresentationPotentialTracePoint {
  step: number
  potential: ExactValue
}

/**
 * A fixed, server-verified scenario used by the defence presentation.  It is
 * deliberately a snapshot: the presentation never sends a calculation
 * request or lets a viewer change its model.
 */
export interface PresentationScenario {
  id: string
  title: string
  players: number
  nodes: NodeInput[]
  algorithm: ImprovementPolicy
  initial: AllocationSummary
  trace: TraceStep[]
  final: AllocationSummary
  nash: NashCheck
  optimum: AllocationSummary
  k_run: ExactValue
  poa: ExactValue
  worst_equilibrium: AllocationSummary
}

export interface PresentationSeriesPoint {
  players: number
  nodes: number
  steps: number
  k_run: ExactValue
  total_time_ne: ExactValue
  total_time_opt: ExactValue
}

/** Fixed conditions of the controlled scale experiment in the defence deck. */
export interface PresentationScalingSetup {
  nodes: NodeInput[]
  initial_node_id: string
  algorithm: ImprovementPolicy
}

/**
 * One server-calculated point of the controlled scale experiment.
 * Metrics are omitted by the server whenever their calculation was not
 * completed, so the presentation cannot mistake an interrupted run for a
 * confirmed equilibrium or an exact optimum.
 */
export interface PresentationScalingPoint {
  n: number
  m: number
  h: number
  equilibrium_status: SimulationStatus
  equilibrium_note?: string
  s_equilibrium?: ExactValue
  s_optimum?: ExactValue
  k_run?: ExactValue
  exact_status: CalculationStatus
  exact_note?: string
}

export interface PresentationResponse {
  linear_preview: PresentationLinearPreviewPoint[]
  equilibrium_comparison: PresentationEquilibriumComparisonPoint[]
  potential_trace: PresentationPotentialTracePoint[]
  scenarios: PresentationScenario[]
  complex_scenario: PresentationScenario
  series: PresentationSeriesPoint[]
  scaling_setup: PresentationScalingSetup
  scaling_series: PresentationScalingPoint[]
}
