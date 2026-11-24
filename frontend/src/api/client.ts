import type {
  ExperimentListResponse,
  ExperimentRequest,
  ExperimentSummary,
  HealthResponse,
  NodeInput,
  OptimizationRequest,
  OptimizationResult,
  PresentationResponse,
  PreviewRequest,
  PreviewResponse,
  PresetsResponse,
  SimulationRequest,
  SimulationResult,
} from './types'

const API_ROOT = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

export class ApiError extends Error {
  readonly status: number
  readonly details: unknown

  constructor(message: string, status: number, details: unknown = undefined) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.details = details
  }
}

interface ApiErrorPayload {
  message?: unknown
  error?: unknown
  details?: unknown
}

function errorReason(details: unknown): string | undefined {
  if (typeof details !== 'object' || details === null) return undefined
  const reason = (details as { reason?: unknown }).reason
  return typeof reason === 'string' && reason.trim() !== '' ? reason.trim() : undefined
}

function errorMessage(payload: unknown, fallback: string): string {
  if (typeof payload === 'object' && payload !== null) {
    const candidate = payload as ApiErrorPayload
    const message = typeof candidate.message === 'string'
      ? candidate.message
      : typeof candidate.error === 'string'
        ? candidate.error
        : fallback
    const reason = errorReason(candidate.details)
    return reason === undefined ? message : `${message} Причина: ${reason}`
  }
  return fallback
}

async function request<T>(
  path: string,
  init: RequestInit = {},
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch(`${API_ROOT}${path}`, {
    ...init,
    signal,
    headers: {
      Accept: 'application/json',
      ...init.headers,
    },
  })

  const isJson = response.headers.get('content-type')?.includes('application/json') ?? false
  const body: unknown = isJson ? await response.json() : await response.text()

  if (!response.ok) {
    throw new ApiError(
      errorMessage(body, `Сервер вернул ошибку ${response.status}.`),
      response.status,
      body,
    )
  }

  return body as T
}

function jsonInit(method: 'POST' | 'DELETE', body?: unknown): RequestInit {
  return {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

function apiDecimal(value: string): string {
  return value.trim().replace(',', '.')
}

function apiNodes<T extends { nodes: NodeInput[] }>(payload: T): T {
  const nodes = payload.nodes.map((node) => {
    const normalized = { ...node, a: apiDecimal(node.a), b: apiDecimal(node.b) }
    if (node.kind === 'table') {
      return { ...normalized, table_values: (node.table_values ?? []).map(apiDecimal) }
    }

    // Таблица относится только к виду `table`. Не передаём старые черновые
    // значения, если пользователь переключился обратно на формулу.
    const { table_values: _tableValues, ...formulaNode } = normalized
    return formulaNode
  })

  return {
    ...payload,
    nodes,
  } as T
}

function apiExperiment(payload: ExperimentRequest): ExperimentRequest {
  return {
    ...payload,
    a: { ...payload.a, from: apiDecimal(payload.a.from), to: apiDecimal(payload.a.to), step: apiDecimal(payload.a.step) },
    b: { ...payload.b, from: apiDecimal(payload.b.from), to: apiDecimal(payload.b.to), step: apiDecimal(payload.b.step) },
  }
}

export const api = {
  health: (signal?: AbortSignal) => request<HealthResponse>('/health', {}, signal),
  presentation: (signal?: AbortSignal) => request<PresentationResponse>('/presentation', {}, signal),
  presets: (signal?: AbortSignal) => request<PresetsResponse>('/presets', {}, signal),
  preview: (payload: PreviewRequest, signal?: AbortSignal) =>
    request<PreviewResponse>('/preview', jsonInit('POST', apiNodes(payload)), signal),
  createSimulation: (payload: SimulationRequest, signal?: AbortSignal) =>
    request<SimulationResult>('/simulations', jsonInit('POST', apiNodes(payload)), signal),
  getSimulation: (id: string, signal?: AbortSignal) =>
    request<SimulationResult>(`/simulations/${encodeURIComponent(id)}`, {}, signal),
  optimize: (payload: OptimizationRequest, signal?: AbortSignal) =>
    request<OptimizationResult>('/optimize', jsonInit('POST', apiNodes(payload)), signal),
  createExperiment: (payload: ExperimentRequest, signal?: AbortSignal) =>
    request<ExperimentSummary>('/experiments', jsonInit('POST', apiExperiment(payload)), signal),
  experiments: (signal?: AbortSignal) =>
    request<ExperimentListResponse>('/experiments', {}, signal),
  experiment: (id: string, signal?: AbortSignal) =>
    request<ExperimentSummary>(`/experiments/${encodeURIComponent(id)}`, {}, signal),
  deleteExperiment: (id: string, signal?: AbortSignal) =>
    request<void>(`/experiments/${encodeURIComponent(id)}`, jsonInit('DELETE'), signal),
  exportUrl: (id: string, format: 'csv' | 'json') =>
    `${API_ROOT}/experiments/${encodeURIComponent(id)}/export?format=${format}`,
}
