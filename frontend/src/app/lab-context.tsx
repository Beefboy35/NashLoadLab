import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import { api } from '../api/client'
import type {
  OptimizationResult,
  Preset,
  SimulationRequest,
  SimulationResult,
} from '../api/types'

const initialDraft: SimulationRequest = {
  players: 2,
  nodes: [
    { id: 'A', name: 'Узел A', a: '0', b: '1', kind: 'linear' },
    { id: 'B', name: 'Узел B', a: '0', b: '2', kind: 'linear' },
  ],
  initial_assignment: ['B', 'B'],
  algorithm: 'first_improvement',
  max_steps: 100,
}

interface LabContextValue {
  draft: SimulationRequest
  replaceDraft: (draft: SimulationRequest) => void
  presets: Preset[]
  presetsLoading: boolean
  presetsError: string | null
  initialMode: 'manual' | 'random'
  setInitialMode: (mode: 'manual' | 'random') => void
  applyPreset: (preset: Preset) => void
  simulation: SimulationResult | null
  setSimulation: (simulation: SimulationResult | null) => void
  optimization: OptimizationResult | null
  setOptimization: (optimization: OptimizationResult | null) => void
}

const LabContext = createContext<LabContextValue | null>(null)

function cloneRequest(request: SimulationRequest): SimulationRequest {
  return {
    ...request,
    nodes: request.nodes.map((node) => ({
      ...node,
      ...(node.table_values === undefined ? {} : { table_values: [...node.table_values] }),
    })),
    initial_assignment: [...request.initial_assignment],
  }
}

export function LabProvider({ children }: { children: React.ReactNode }) {
  const [draft, setDraft] = useState<SimulationRequest>(() => cloneRequest(initialDraft))
  const [presets, setPresets] = useState<Preset[]>([])
  const [presetsLoading, setPresetsLoading] = useState(true)
  const [presetsError, setPresetsError] = useState<string | null>(null)
  const [initialMode, setInitialMode] = useState<'manual' | 'random'>('manual')
  const [simulation, setSimulation] = useState<SimulationResult | null>(null)
  const [optimization, setOptimization] = useState<OptimizationResult | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    void api
      .presets(controller.signal)
      .then((response) => setPresets(response.presets))
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        setPresetsError(error instanceof Error ? error.message : 'Не удалось загрузить пресеты.')
      })
      .finally(() => setPresetsLoading(false))
    return () => controller.abort()
  }, [])

  const value = useMemo<LabContextValue>(
    () => ({
      draft,
      replaceDraft: (nextDraft) => {
        setDraft(cloneRequest(nextDraft))
        setSimulation(null)
        setOptimization(null)
      },
      presets,
      presetsLoading,
      presetsError,
      initialMode,
      setInitialMode,
      applyPreset: (preset) => {
        setDraft(cloneRequest(preset.request))
        setSimulation(null)
        setOptimization(null)
        setInitialMode('manual')
      },
      simulation,
      setSimulation,
      optimization,
      setOptimization,
    }),
    [draft, initialMode, optimization, presets, presetsError, presetsLoading, simulation],
  )

  return <LabContext.Provider value={value}>{children}</LabContext.Provider>
}

export function useLab(): LabContextValue {
  const context = useContext(LabContext)
  if (context === null) throw new Error('useLab должен использоваться внутри LabProvider.')
  return context
}
