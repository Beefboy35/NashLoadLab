import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import type { NodeInput, PresentationResponse } from '../api/types'
import { App } from './App'

const presetsResponse = {
  presets: [{
    id: 'A',
    name: 'А. Два игрока',
    description: 'Эталонный пример.',
    request: {
      players: 2,
      nodes: [
        { id: 'A', name: 'Узел А', a: '0', b: '1', kind: 'linear' },
        { id: 'B', name: 'Узел Б', a: '0', b: '2', kind: 'linear' },
      ],
      initial_assignment: ['B', 'B'],
      algorithm: 'first_improvement',
      max_steps: 100,
    },
  }],
}

const twoNodeModel = [
  { id: 'A', name: 'Узел А', a: '0', b: '1', kind: 'linear' as const },
  { id: 'B', name: 'Узел Б', a: '0', b: '2', kind: 'linear' as const },
]

const scenarioBNodes = [
  { id: 'A', name: 'Узел А', a: '0', b: '1', kind: 'linear' as const },
  { id: 'B', name: 'Узел Б', a: '1', b: '0.5', kind: 'linear' as const },
]

const complexNodes = [
  { id: 'У₁', name: 'Узел 1', a: '0', b: '1', kind: 'linear' as const },
  { id: 'У₂', name: 'Узел 2', a: '0.4', b: '0.8', kind: 'linear' as const },
  { id: 'У₃', name: 'Узел 3', a: '0.1', b: '0.18', kind: 'quadratic' as const },
  { id: 'У₄', name: 'Узел 4', a: '0', b: '0', kind: 'table' as const, table_values: ['0.7', '1.1', '1.6', '2.3', '3.5', '5.2', '7.5', '10.5'] },
]

const scalingNodes = [
  { id: 'У₁', name: 'Узел 1 — линейный', a: '0', b: '1', kind: 'linear' as const },
  { id: 'У₂', name: 'Узел 2 — линейный', a: '0.4', b: '0.8', kind: 'linear' as const },
  { id: 'У₃', name: 'Узел 3 — квадратичный', a: '0.1', b: '0.18', kind: 'quadratic' as const },
  { id: 'У₄', name: 'Узел 4 — квадратичный', a: '0.2', b: '0.12', kind: 'quadratic' as const },
]

function summary(nodes: NodeInput[], assignment: string[], potential: string, totalTime: string, latencies: Record<string, string> = {}) {
  return {
    assignment,
    loads: nodes.map((node) => ({
      node_id: node.id,
      node_name: node.name,
      load: assignment.filter((value) => value === node.id).length,
      latency: latencies[node.id] ?? '0',
    })),
    potential,
    total_time: totalTime,
  }
}

function traceStep(
  step: number,
  from: string,
  to: string,
  previous: string,
  next: string,
  delta: string,
  potential: string,
  totalTime: string,
  snapshot: ReturnType<typeof summary>,
) {
  return {
    step,
    player: step,
    from,
    to,
    previous,
    next,
    delta,
    potential,
    total_time: totalTime,
    summary: snapshot,
  }
}

const aInitial = summary(twoNodeModel, ['B', 'B'], '6', '8', { A: '0', B: '4' })
const aFinal = summary(twoNodeModel, ['A', 'B'], '3', '3', { A: '1', B: '2' })
const bInitial = summary(scenarioBNodes, ['A', 'A', 'A', 'A'], '10', '16', { A: '4', B: '0' })
const bAfterFirst = summary(scenarioBNodes, ['B', 'A', 'A', 'A'], '7.5', '10.5', { A: '3', B: '1.5' })
const bFinal = summary(scenarioBNodes, ['B', 'B', 'A', 'A'], '6.5', '8', { A: '2', B: '2' })
const cFinal = summary(twoNodeModel, ['A', 'A'], '3', '4', { A: '2', B: '0' })
const cOptimum = summary(twoNodeModel, ['A', 'B'], '3', '3', { A: '1', B: '2' })

function assignmentForLoads(nodes: NodeInput[], loads: number[]): string[] {
  return nodes.reduce<string[]>((assignment, node, index) => (
    assignment.concat(Array.from({ length: loads[index] ?? 0 }, () => node.id))
  ), [])
}

function complexLatencies(loads: number[]): Record<string, string> {
  const u1 = ['0', '1', '2', '3', '4', '5', '6', '7', '8']
  const u2 = ['0', '1.2', '2', '2.8', '3.6', '4.4', '5.2', '6', '6.8']
  const u3 = ['0', '0.28', '0.82', '1.72', '2.98', '4.6', '6.58', '8.92', '11.62']
  const u4 = ['0', '0.7', '1.1', '1.6', '2.3', '3.5', '5.2', '7.5', '10.5']
  return { 'У₁': u1[loads[0]], 'У₂': u2[loads[1]], 'У₃': u3[loads[2]], 'У₄': u4[loads[3]] }
}

function complexSummary(loads: number[], potential: string, totalTime: string) {
  return summary(complexNodes, assignmentForLoads(complexNodes, loads), potential, totalTime, complexLatencies(loads))
}

const complexInitial = complexSummary([8, 0, 0, 0], '36', '64')
const complexFinal = complexSummary([1, 1, 3, 3], '8.42', '12.16')
const complexOptimum = complexSummary([1, 2, 2, 3], '8.7', '11.44')

const complexTraceData = [
  { from: 'У₁', to: 'У₂', previous: '8', next: '1.2', delta: '-6.8', loads: [7, 1, 0, 0], potential: '29.2', totalTime: '50.2' },
  { from: 'У₂', to: 'У₃', previous: '1.2', next: '0.28', delta: '-0.92', loads: [7, 0, 1, 0], potential: '28.28', totalTime: '49.28' },
  { from: 'У₁', to: 'У₂', previous: '7', next: '1.2', delta: '-5.8', loads: [6, 1, 1, 0], potential: '22.48', totalTime: '37.48' },
  { from: 'У₂', to: 'У₃', previous: '1.2', next: '0.82', delta: '-0.38', loads: [6, 0, 2, 0], potential: '22.1', totalTime: '37.64' },
  { from: 'У₃', to: 'У₄', previous: '0.82', next: '0.7', delta: '-0.12', loads: [6, 0, 1, 1], potential: '21.98', totalTime: '36.98' },
  { from: 'У₁', to: 'У₂', previous: '6', next: '1.2', delta: '-4.8', loads: [5, 1, 1, 1], potential: '17.18', totalTime: '27.18' },
  { from: 'У₂', to: 'У₃', previous: '1.2', next: '0.82', delta: '-0.38', loads: [5, 0, 2, 1], potential: '16.8', totalTime: '27.34' },
  { from: 'У₁', to: 'У₂', previous: '5', next: '1.2', delta: '-3.8', loads: [4, 1, 2, 1], potential: '13', totalTime: '19.54' },
  { from: 'У₂', to: 'У₄', previous: '1.2', next: '1.1', delta: '-0.1', loads: [4, 0, 2, 2], potential: '12.9', totalTime: '19.84' },
  { from: 'У₁', to: 'У₂', previous: '4', next: '1.2', delta: '-2.8', loads: [3, 1, 2, 2], potential: '10.1', totalTime: '14.04' },
  { from: 'У₁', to: 'У₂', previous: '3', next: '2', delta: '-1', loads: [2, 2, 2, 2], potential: '9.1', totalTime: '11.84' },
  { from: 'У₂', to: 'У₃', previous: '2', next: '1.72', delta: '-0.28', loads: [2, 1, 3, 2], potential: '8.82', totalTime: '12.56' },
  { from: 'У₃', to: 'У₄', previous: '1.72', next: '1.6', delta: '-0.12', loads: [2, 1, 2, 3], potential: '8.7', totalTime: '11.64' },
  { from: 'У₁', to: 'У₃', previous: '2', next: '1.72', delta: '-0.28', loads: [1, 1, 3, 3], potential: '8.42', totalTime: '12.16' },
]

const complexTrace = complexTraceData.map((point, index) => traceStep(
  index + 1,
  point.from,
  point.to,
  point.previous,
  point.next,
  point.delta,
  point.potential,
  point.totalTime,
  complexSummary(point.loads, point.potential, point.totalTime),
))

const presentationResponse = {
  linear_preview: [
    { load: 1, a: '1', b: '2' },
    { load: 2, a: '2', b: '4' },
    { load: 3, a: '3', b: '6' },
    { load: 4, a: '4', b: '8' },
  ],
  equilibrium_comparison: [
    { profile: '(A,A)', total_time: '4' },
    { profile: '(A,B)', total_time: '3' },
  ],
  potential_trace: [
    { step: 0, potential: '10' },
    { step: 1, potential: '7.5' },
    { step: 2, potential: '6.5' },
  ],
  scenarios: [
    {
      id: 'A', title: 'Сценарий А', players: 2, nodes: twoNodeModel, algorithm: 'first_improvement',
      initial: aInitial, trace: [traceStep(1, 'B', 'A', '4', '1', '-3', '3', '3', aFinal)], final: aFinal,
      nash: { is_equilibrium: true, checked_moves: 2 }, optimum: aFinal, k_run: '1', poa: '4/3', worst_equilibrium: cFinal,
    },
    {
      id: 'B', title: 'Сценарий Б', players: 4, nodes: scenarioBNodes, algorithm: 'first_improvement',
      initial: bInitial,
      trace: [
        traceStep(1, 'A', 'B', '4', '1.5', '-2.5', '7.5', '10.5', bAfterFirst),
        traceStep(2, 'A', 'B', '3', '2', '-1', '6.5', '8', bFinal),
      ],
      final: bFinal,
      nash: { is_equilibrium: true, checked_moves: 8 }, optimum: bFinal, k_run: '1', poa: '1', worst_equilibrium: bFinal,
    },
    {
      id: 'C', title: 'Сценарий В', players: 2, nodes: twoNodeModel, algorithm: 'first_improvement',
      initial: cFinal, trace: [], final: cFinal,
      nash: { is_equilibrium: true, checked_moves: 2 }, optimum: cOptimum, k_run: '4/3', poa: '4/3', worst_equilibrium: cFinal,
    },
  ],
  complex_scenario: {
    id: 'сложный', title: 'Сложный случай', players: 8, nodes: complexNodes, algorithm: 'first_improvement',
    initial: complexInitial,
    trace: complexTrace,
    final: complexFinal, nash: { is_equilibrium: true, checked_moves: 32 }, optimum: complexOptimum,
    k_run: '152/143', poa: '152/143', worst_equilibrium: complexFinal,
  },
  series: [
    { players: 4, nodes: 2, steps: 2, k_run: '1', total_time_ne: '8', total_time_opt: '8' },
    { players: 6, nodes: 2, steps: 3, k_run: '1', total_time_ne: '17.4', total_time_opt: '17.4' },
    { players: 6, nodes: 3, steps: 7, k_run: '259/241', total_time_ne: '10.36', total_time_opt: '9.64' },
    { players: 8, nodes: 3, steps: 9, k_run: '1', total_time_ne: '17.56', total_time_opt: '17.56' },
    { players: 8, nodes: 4, steps: 14, k_run: '152/143', total_time_ne: '12.16', total_time_opt: '11.44' },
    { players: 10, nodes: 4, steps: 16, k_run: '1', total_time_ne: '17.96', total_time_opt: '17.96' },
  ],
  scaling_setup: {
    nodes: scalingNodes,
    initial_node_id: 'У₁',
    algorithm: 'first_improvement',
  },
  scaling_series: [
    { n: 8, m: 4, h: 15, equilibrium_status: 'equilibrium', s_equilibrium: '11.2', s_optimum: '10.48', k_run: '140/131', exact_status: 'exact' },
    { n: 16, m: 4, h: 25, equilibrium_status: 'equilibrium', s_equilibrium: '51.32', s_optimum: '50.8', k_run: '1283/1270', exact_status: 'exact' },
    { n: 32, m: 4, h: 43, equilibrium_status: 'equilibrium', s_equilibrium: '250.52', s_optimum: '247.04', k_run: '6263/6176', exact_status: 'exact' },
  ],
} satisfies PresentationResponse

function json(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }))
}

function pathOf(input: string | URL | Request): string {
  return new URL(String(input), 'http://test').pathname
}

describe('App', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn((input: string | URL | Request, init?: RequestInit) => {
      const path = pathOf(input)
      const method = init?.method ?? 'GET'
      if (path === '/api/v1/presets' && method === 'GET') return json(presetsResponse)
      if (path === '/api/v1/presentation' && method === 'GET') return json(presentationResponse)
      return Promise.reject(new Error(`Неожиданный API: ${method} ${path}`))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('показывает русскую модель и пресет, полученный с сервера', async () => {
    render(<MemoryRouter initialEntries={['/model']}><App /></MemoryRouter>)

    expect(screen.getByRole('heading', { name: 'Модель' })).toBeInTheDocument()
    expect(screen.getByLabelText(/Число игроков/)).toHaveValue(2)
    expect(await screen.findByText('А. Два игрока')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Перейти к поиску равновесия' })).toHaveAttribute('href', '/search')
  })

  it('даёт заполнить табличную функцию и меняет её длину вместе с числом игроков', () => {
    render(<MemoryRouter initialEntries={['/model']}><App /></MemoryRouter>)

    fireEvent.change(screen.getByLabelText('Вид функции узла Узел A'), { target: { value: 'table' } })

    expect(screen.getByRole('heading', { name: 'Значения табличных функций' })).toBeInTheDocument()
    const firstValue = screen.getByLabelText('Время T(1) для узла Узел A')
    expect(firstValue).toHaveValue('')
    expect(screen.getByLabelText('Время T(2) для узла Узел A')).toHaveValue('')

    fireEvent.change(firstValue, { target: { value: '1,5' } })
    fireEvent.change(screen.getByLabelText(/Число игроков/), { target: { value: '3' } })

    expect(screen.getByLabelText('Время T(1) для узла Узел A')).toHaveValue('1,5')
    expect(screen.getByLabelText('Время T(3) для узла Узел A')).toHaveValue('')

    fireEvent.change(screen.getByLabelText(/Число игроков/), { target: { value: '1' } })
    expect(screen.queryByLabelText('Время T(2) для узла Узел A')).not.toBeInTheDocument()
  })

  it('показывает подготовленную десятислайдовую защиту без запуска расчёта', async () => {
    render(<MemoryRouter initialEntries={['/presentation']}><App /></MemoryRouter>)

    expect(await screen.findByRole('heading', { name: 'Равновесие Нэша при распределении вычислительной нагрузки' })).toBeInTheDocument()
    expect(screen.getByText('Слайд 1 из 10')).toBeInTheDocument()

    fireEvent.keyDown(window, { key: 'ArrowRight' })
    expect(screen.getByText('Слайд 2 из 10')).toBeInTheDocument()
    expect(screen.getByTestId('presentation-slide')).toHaveTextContent(/T/)

    for (let index = 0; index < 8; index += 1) {
      fireEvent.keyDown(window, { key: 'ArrowRight' })
    }
    expect(screen.getByText('Слайд 10 из 10')).toBeInTheDocument()
    expect(screen.getByText('Построенная модель позволяет математически исследовать достижение равновесий Нэша и оценивать, насколько устойчивое распределение вычислительной нагрузки отличается от оптимального.')).toBeInTheDocument()

    fireEvent.keyDown(window, { key: 'Home' })
    expect(screen.getByText('Слайд 1 из 10')).toBeInTheDocument()

    const calls = fetchMock.mock.calls.map(([input, init]) => ({
      path: pathOf(input as string | URL | Request),
      method: (init as RequestInit | undefined)?.method ?? 'GET',
    }))
    expect(calls).toEqual(expect.arrayContaining([
      { path: '/api/v1/presets', method: 'GET' },
      { path: '/api/v1/presentation', method: 'GET' },
    ]))
    expect(calls.every((call) => call.method === 'GET')).toBe(true)
  })

  it('показывает краткий read-only маршрут защиты 5 → 7 → 8 → 9 и переключает масштабную конфигурацию одной кнопкой', async () => {
    render(<MemoryRouter initialEntries={['/presentation']}><App /></MemoryRouter>)

    fireEvent.click(await screen.findByRole('button', { name: 'Краткий показ · до 3 минут' }))

    expect(screen.getByText('Маршрут защиты: 5 → 7 → 8 → 9 · до 3 минут')).toBeInTheDocument()
    expect(screen.getByText('Экран 1 из 4 · слайд 5')).toBeInTheDocument()

    fireEvent.keyDown(window, { key: 'ArrowRight' })
    expect(screen.getByText('Экран 2 из 4 · слайд 7')).toBeInTheDocument()

    fireEvent.keyDown(window, { key: 'ArrowRight' })
    expect(screen.getByText('Экран 3 из 4 · слайд 8')).toBeInTheDocument()

    fireEvent.keyDown(window, { key: 'ArrowRight' })
    expect(screen.getByText('Экран 4 из 4 · слайд 9')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Контролируемая серия: меняется только n' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Следующий →' })).toBeDisabled()

    const scaleButton = screen.getByRole('button', { name: 'Следующая конфигурация масштабной серии' })
    expect(scaleButton).toHaveTextContent('Следующая конфигурация: 8 → 16 → 32')
    expect(screen.getByTestId('scaling-active-config')).toHaveTextContent('n = 8')
    expect(screen.getByTestId('scaling-baseline-k')).toBeInTheDocument()
    expect(screen.getByText('K = 1 — совпадение с оптимумом')).toBeInTheDocument()

    const x8 = Number(screen.getByTestId('scaling-point-h-8').getAttribute('data-x'))
    const x16 = Number(screen.getByTestId('scaling-point-h-16').getAttribute('data-x'))
    const x32 = Number(screen.getByTestId('scaling-point-h-32').getAttribute('data-x'))
    expect(x16 - x8).toBeCloseTo((x32 - x16) / 2, 2)
    expect(screen.queryByTestId('legacy-series-details')).not.toBeInTheDocument()

    fireEvent.click(scaleButton)
    expect(screen.getByTestId('scaling-active-config')).toHaveTextContent('n = 16')

    fireEvent.click(scaleButton)
    expect(screen.getByTestId('scaling-active-config')).toHaveTextContent('n = 32')

    const calls = fetchMock.mock.calls.map(([input, init]) => ({
      path: pathOf(input as string | URL | Request),
      method: (init as RequestInit | undefined)?.method ?? 'GET',
    }))
    expect(calls).toEqual(expect.arrayContaining([
      { path: '/api/v1/presets', method: 'GET' },
      { path: '/api/v1/presentation', method: 'GET' },
    ]))
    expect(calls.every((call) => call.method === 'GET')).toBe(true)
  })

  it('скрывает подробную таблицу серии в полном показе до явного раскрытия', async () => {
    render(<MemoryRouter initialEntries={['/presentation']}><App /></MemoryRouter>)

    await screen.findByRole('heading', { name: 'Равновесие Нэша при распределении вычислительной нагрузки' })
    for (let index = 0; index < 8; index += 1) fireEvent.keyDown(window, { key: 'ArrowRight' })

    const details = screen.getByTestId('legacy-series-details')
    expect(details).not.toHaveAttribute('open')
    expect(details).toHaveTextContent('Шесть исходных конфигураций — раскрыть в полном режиме')
  })
})
