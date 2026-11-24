import { useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { api } from '../../api/client'
import type {
  AllocationSummary,
  NodeInput,
  PresentationResponse,
  PresentationScalingPoint,
  PresentationScenario,
  PresentationSeriesPoint,
} from '../../api/types'
import { Loading, Notice } from '../../components/ui'
import { formatExact } from '../../lib/format'

const fullSlides = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9] as const
const shortSlides = [4, 6, 7, 8] as const

type PresentationMode = 'full' | 'short'

/** IDs remain stable in the engine; the mathematical slides use Latin/numeric notation. */
const nodeSymbols: Record<string, string> = {
  A: 'A',
  B: 'B',
  C: 'C',
  D: 'D',
  'У₁': '1',
  'У₂': '2',
  'У₃': '3',
  'У₄': '4',
}

function nodeSymbol(id: string): string {
  return nodeSymbols[id] ?? id
}

function profileText(assignment: string[] | undefined): string {
  return assignment?.length ? '(' + assignment.map(nodeSymbol).join(',') + ')' : '—'
}

function summaryValue(summary: AllocationSummary | undefined, key: 'potential' | 'total_time'): string {
  return formatExact(summary?.[key])
}

function numberValue(value: string | number | undefined): number {
  const text = String(value ?? '0').replace(',', '.')
  const fraction = /^(-?\d+)\/(\d+)$/.exec(text)
  const parsed = fraction ? Number(fraction[1]) / Number(fraction[2]) : Number(text)
  return Number.isFinite(parsed) ? parsed : 0
}

function approximate(value: string | number | undefined, digits = 3): string {
  return numberValue(value).toFixed(digits).replace('.', ',')
}

function loadOf(summary: AllocationSummary | undefined, nodeID: string): number {
  return summary?.loads?.find((load) => load.node_id === nodeID)?.load ?? 0
}

function latencyOf(summary: AllocationSummary | undefined, nodeID: string): string {
  return formatExact(summary?.loads?.find((load) => load.node_id === nodeID)?.latency)
}

function loadVector(nodes: NodeInput[], summary: AllocationSummary | undefined): string {
  return '(' + nodes.map((node) => loadOf(summary, node.id)).join(',') + ')'
}

function totalTimeFormula(nodes: NodeInput[], summary: AllocationSummary | undefined): ReactNode {
  const terms = nodes
    .map((node) => ({ load: loadOf(summary, node.id), latency: latencyOf(summary, node.id) }))
    .filter((term) => term.load > 0)
  return terms.map((term, index) => (
    <span key={term.load + '-' + term.latency + '-' + index}>{index ? ' + ' : ''}{term.load} · {term.latency}</span>
  ))
}

function Metric({ label, value, note, accent = false }: { label: ReactNode; value: ReactNode; note?: ReactNode; accent?: boolean }) {
  return (
    <div className={'prepared-metric' + (accent ? ' prepared-metric--accent' : '')}>
      <span>{label}</span><strong>{value}</strong>{note ? <small>{note}</small> : null}
    </div>
  )
}

function SlideTitle({ number, eyebrow, title, children }: { number: number; eyebrow: string; title: string; children?: ReactNode }) {
  return (
    <header className="prepared-slide__header">
      <p className="presentation-kicker">Слайд {number} · {eyebrow}</p>
      <h1>{title}</h1>
      {children ? <div className="prepared-slide__lead">{children}</div> : null}
    </header>
  )
}

function NodeFormula({ node }: { node: NodeInput }) {
  const symbol = nodeSymbol(node.id)
  if (node.kind === 'table') {
    const values = node.table_values?.map(formatExact) ?? []
    const preview = values.length > 5 ? [...values.slice(0, 4), '…', values.at(-1) ?? ''] : values
    return <span className="prepared-formula">T<sub>{symbol}</sub>(k): [{preview.join('; ')}]</span>
  }
  return (
    <span className="prepared-formula">
      T<sub>{symbol}</sub>(k) = {formatExact(node.a)} + {formatExact(node.b)} · k{node.kind === 'quadratic' ? <sup>2</sup> : null}
    </span>
  )
}

/** Numbered task chips make a load a concrete allocation rather than an abstract bar. */
function TaskAllocation({ nodes, summary, label, compact = false }: {
  nodes: NodeInput[]
  summary: AllocationSummary | undefined
  label: string
  compact?: boolean
}) {
  const assignment = summary?.assignment ?? []
  return (
    <section className={'prepared-task-allocation' + (compact ? ' prepared-task-allocation--compact' : '')} aria-label={label}>
      <header><strong>{label}</strong><span>{profileText(assignment)}</span></header>
      <div className="prepared-task-allocation__nodes">
        {nodes.map((node) => {
          const tasks = assignment.map((nodeID, index) => nodeID === node.id ? index + 1 : null).filter((task): task is number => task !== null)
          return (
            <article key={node.id}>
              <div className="prepared-task-allocation__node-head">
                <b>{nodeSymbol(node.id)}</b><span>n<sub>{nodeSymbol(node.id)}</sub>(s) = {tasks.length}</span>
              </div>
              <div className="prepared-task-allocation__chips">
                {tasks.length ? tasks.map((task) => <i key={task}>{task}</i>) : <em>—</em>}
              </div>
              <small>T<sub>{nodeSymbol(node.id)}</sub> = {latencyOf(summary, node.id)}</small>
            </article>
          )
        })}
      </div>
    </section>
  )
}

function PotentialLine({ points, title }: { points: Array<{ step: number; potential: string }>; title: string }) {
  if (!points.length) return null
  const values = points.map((point) => numberValue(point.potential))
  const min = Math.min(...values)
  const max = Math.max(...values)
  const range = Math.max(max - min, 1)
  const width = 700
  const height = 290
  const left = 72
  const right = 38
  const top = 40
  const bottom = 58
  const x = (index: number) => points.length === 1 ? width / 2 : left + index * ((width - left - right) / (points.length - 1))
  const y = (value: number) => top + (max - value) / range * (height - top - bottom)
  const path = values.map((value, index) => (index === 0 ? 'M' : 'L') + ' ' + x(index) + ' ' + y(value)).join(' ')
  const maxPoint = points.reduce((best, point) => numberValue(point.potential) > numberValue(best.potential) ? point : best)
  const minPoint = points.reduce((best, point) => numberValue(point.potential) < numberValue(best.potential) ? point : best)
  return (
    <figure className="prepared-line-chart">
      <figcaption>{title}</figcaption>
      <svg viewBox={'0 0 ' + width + ' ' + height} role="img" aria-label={title + ': ' + points.map((point) => 'шаг ' + point.step + ', Φ ' + formatExact(point.potential)).join('; ')}>
        <line x1={left} y1={height - bottom} x2={width - right} y2={height - bottom} className="prepared-line-chart__axis" />
        <line x1={left} y1={top} x2={left} y2={height - bottom} className="prepared-line-chart__axis" />
        <text x={left - 52} y={top + 10} className="prepared-line-chart__axis-label">Φ(s)</text>
        <text x={width - right - 2} y={height - 18} textAnchor="end" className="prepared-line-chart__axis-label">шаг</text>
        <text x={left - 10} y={top + 20} textAnchor="end" className="prepared-line-chart__tick">{formatExact(maxPoint.potential)}</text>
        <text x={left - 10} y={height - bottom} textAnchor="end" className="prepared-line-chart__tick">{formatExact(minPoint.potential)}</text>
        <path d={path} className="prepared-line-chart__line" />
        {points.map((point, index) => (
          <g key={point.step + '-' + point.potential}>
            <circle cx={x(index)} cy={y(values[index])} r="6" className="prepared-line-chart__point" />
            <text x={x(index)} y={y(values[index]) - 15} textAnchor="middle" className="prepared-line-chart__value">{formatExact(point.potential)}</text>
            <text x={x(index)} y={height - bottom + 28} textAnchor="middle" className="prepared-line-chart__tick">{point.step}</text>
          </g>
        ))}
      </svg>
    </figure>
  )
}

function ModelLines({ data }: { data: PresentationResponse }) {
  const valuesA = data.linear_preview.map((point) => numberValue(point.a))
  const valuesB = data.linear_preview.map((point) => numberValue(point.b))
  const maximum = Math.max(1, ...valuesA, ...valuesB)
  const width = 650
  const height = 310
  const left = 70
  const right = 34
  const top = 34
  const bottom = 62
  const x = (index: number) => data.linear_preview.length === 1 ? width / 2 : left + index * ((width - left - right) / (data.linear_preview.length - 1))
  const y = (value: number) => top + (maximum - value) / maximum * (height - top - bottom)
  const path = (values: number[]) => values.map((value, index) => (index === 0 ? 'M' : 'L') + ' ' + x(index) + ' ' + y(value)).join(' ')
  const ticks = [0, maximum / 4, maximum / 2, maximum * 3 / 4, maximum]
  return (
    <figure className="prepared-model-chart">
      <div className="prepared-chart-key" aria-label="Обозначения графика">
        <span><i className="prepared-chart-key__a" />T<sub>A</sub>(k) = k</span>
        <span><i className="prepared-chart-key__b" />T<sub>B</sub>(k) = 2k</span>
      </div>
      <svg viewBox={'0 0 ' + width + ' ' + height} role="img" aria-label="График функций T A от k и T B от k">
        {ticks.map((tick) => (
          <g key={tick}>
            <line x1={left} y1={y(tick)} x2={width - right} y2={y(tick)} className="prepared-model-chart__grid" />
            <text x={left - 11} y={y(tick) + 5} textAnchor="end" className="prepared-line-chart__tick">{formatExact(tick)}</text>
          </g>
        ))}
        <line x1={left} y1={height - bottom} x2={width - right} y2={height - bottom} className="prepared-line-chart__axis" />
        <line x1={left} y1={top} x2={left} y2={height - bottom} className="prepared-line-chart__axis" />
        <path d={path(valuesA)} className="prepared-model-chart__a" />
        <path d={path(valuesB)} className="prepared-model-chart__b" />
        {data.linear_preview.map((point, index) => (
          <g key={point.load}>
            <circle cx={x(index)} cy={y(valuesA[index])} r="5" className="prepared-model-chart__point-a" />
            <circle cx={x(index)} cy={y(valuesB[index])} r="5" className="prepared-model-chart__point-b" />
            <text x={x(index)} y={height - bottom + 28} textAnchor="middle" className="prepared-line-chart__tick">{point.load}</text>
          </g>
        ))}
        <text x={left - 58} y={top + 7} className="prepared-line-chart__axis-label">Tⱼ(k)</text>
        <text x={width - right} y={height - 16} textAnchor="end" className="prepared-line-chart__axis-label">k — число задач</text>
      </svg>
      <figcaption>Числовые точки: k = 1, 2, 3, 4. Линейная функция — лишь частный случай модели.</figcaption>
    </figure>
  )
}

function ResearchQuestion({ index, label, statement, method, result }: {
  index: 1 | 2 | 3
  label: string
  statement: ReactNode
  method: ReactNode
  result: ReactNode
}) {
  return (
    <article className={'prepared-research-question prepared-research-question--' + index}>
      <div className="prepared-research-question__top"><b>Р<sub>{index}</sub></b><span>{label}</span></div>
      <div className="prepared-research-question__route">
        <div><small>утверждение</small><strong>{statement}</strong></div><i aria-hidden="true">→</i>
        <div><small>метод проверки</small><strong>{method}</strong></div><i aria-hidden="true">→</i>
        <div><small>результат</small><strong>{result}</strong></div>
      </div>
    </article>
  )
}

function PotentialShiftDiagram() {
  return (
    <figure className="prepared-potential-shift" aria-label="Изменение слагаемых потенциала при переходе с узла A на узел B">
      <figcaption>При переходе A → B изменяются только два крайних слагаемых</figcaption>
      <div className="prepared-potential-shift__sums">
        <div>
          <span>узел A: до перехода</span>
          <strong>T<sub>A</sub>(1) + … + <mark className="prepared-potential-shift__removed">T<sub>A</sub>(n<sub>A</sub>)</mark></strong>
          <small>это слагаемое исчезает</small>
        </div>
        <div>
          <span>узел B: после перехода</span>
          <strong>T<sub>B</sub>(1) + … + <mark className="prepared-potential-shift__added">T<sub>B</sub>(n<sub>B</sub> + 1)</mark></strong>
          <small>это слагаемое добавляется</small>
        </div>
      </div>
      <div className="prepared-potential-shift__delta">ΔΦ = T<sub>B</sub>(n<sub>B</sub> + 1) − T<sub>A</sub>(n<sub>A</sub>) = Δt<sub>i</sub> &lt; 0</div>
    </figure>
  )
}

function SeriesGrid({ points }: { points: PresentationSeriesPoint[] }) {
  const players = [...new Set(points.map((point) => point.players))].sort((a, b) => a - b)
  const nodes = [...new Set(points.map((point) => point.nodes))].sort((a, b) => a - b)
  const values = points.map((point) => numberValue(point.k_run))
  const min = Math.min(...values)
  const max = Math.max(...values)
  const intensity = (value: string) => 0.14 + (numberValue(value) - min) / Math.max(max - min, 0.000001) * 0.72
  const gridStyle = { gridTemplateColumns: 'minmax(6rem, 0.86fr) repeat(' + nodes.length + ', minmax(0, 1fr))' }
  return (
    <section className="prepared-series-grid" style={gridStyle} aria-label="Шесть исходных конфигураций">
      <div className="prepared-series-grid__corner">задачи ↓<br />узлы →</div>
      {nodes.map((count) => <div className="prepared-series-grid__head" key={count}>{count} узла</div>)}
      {players.flatMap((playerCount) => [
        <div className="prepared-series-grid__side" key={'side-' + playerCount}>{playerCount} задач</div>,
        ...nodes.map((nodeCount) => {
          const point = points.find((candidate) => candidate.players === playerCount && candidate.nodes === nodeCount)
          if (!point) return <div className="prepared-series-cell prepared-series-cell--empty" key={playerCount + '-' + nodeCount}>—</div>
          const style = { '--series-intensity': intensity(point.k_run) } as CSSProperties
          return <div className="prepared-series-cell" style={style} key={playerCount + '-' + nodeCount}><strong>K<sub>найд</sub> {formatExact(point.k_run)}</strong><span>H = {point.steps}</span><small>S<sub>равн</sub> {formatExact(point.total_time_ne)} · S<sub>опт</sub> {formatExact(point.total_time_opt)}</small></div>
        }),
      ])}
    </section>
  )
}

function ScalingChart({ title, chartID, points, value, axis, formatter, referenceValue, referenceLabel }: {
  title: string
  chartID: 'h' | 'k'
  points: PresentationScalingPoint[]
  value: (point: PresentationScalingPoint) => number | undefined
  axis: string
  formatter: (item: number) => string
  referenceValue?: number
  referenceLabel?: string
}) {
  const usable = points.map((point) => ({ n: point.n, value: value(point) })).filter((point): point is { n: number; value: number } => point.value !== undefined)
  if (!usable.length) return null
  const values = referenceValue === undefined ? usable.map((point) => point.value) : [...usable.map((point) => point.value), referenceValue]
  const rawMin = Math.min(...values)
  const rawMax = Math.max(...values)
  const padding = Math.max((rawMax - rawMin) * 0.22, axis === 'K_найд' ? 0.005 : 1)
  const min = Math.max(0, rawMin - padding)
  const max = rawMax + padding
  const range = Math.max(max - min, 0.01)
  const width = 560
  const height = 270
  const left = 70
  const right = 26
  const top = 42
  const bottom = 56
  const maxN = Math.max(...usable.map((point) => point.n), 1)
  const x = (n: number) => left + n / maxN * (width - left - right)
  const y = (item: number) => top + (max - item) / range * (height - top - bottom)
  const path = usable.map((item, index) => (index === 0 ? 'M' : 'L') + ' ' + x(item.n) + ' ' + y(item.value)).join(' ')
  const ticks = [max, min + range / 2, min]
  const xTicks = [...new Set([0, ...usable.map((point) => point.n)])].sort((a, b) => a - b)
  return (
    <figure className="prepared-scaling-chart" data-testid={'scaling-chart-' + chartID}>
      <figcaption>{title}</figcaption>
      <svg viewBox={'0 0 ' + width + ' ' + height} role="img" aria-label={title}>
        {ticks.map((tick) => <g key={tick}><line x1={left} y1={y(tick)} x2={width - right} y2={y(tick)} className="prepared-model-chart__grid" /><text x={left - 18} y={y(tick) + 5} textAnchor="end" className="prepared-line-chart__tick">{formatter(tick)}</text></g>)}
        <line x1={left} y1={height - bottom} x2={width - right} y2={height - bottom} className="prepared-line-chart__axis" />
        <line x1={left} y1={top} x2={left} y2={height - bottom} className="prepared-line-chart__axis" />
        <text x={left - 54} y={top + 5} className="prepared-line-chart__axis-label">{axis}</text>
        <text x={width - right} y={height - 16} textAnchor="end" className="prepared-line-chart__axis-label">n, задач</text>
        {xTicks.map((tick) => <g key={tick}><line x1={x(tick)} y1={height - bottom} x2={x(tick)} y2={height - bottom + 5} className="prepared-line-chart__axis" /><text x={x(tick)} y={height - bottom + 27} textAnchor="middle" className="prepared-line-chart__tick">{tick}</text></g>)}
        {referenceValue !== undefined ? <g><line data-testid={'scaling-baseline-' + chartID} x1={left} y1={y(referenceValue)} x2={width - right} y2={y(referenceValue)} className="prepared-scaling-chart__baseline" /><text x={width - right - 4} y={y(referenceValue) - 8} textAnchor="end" className="prepared-scaling-chart__baseline-label">{referenceLabel}</text></g> : null}
        <path d={path} className="prepared-scaling-chart__line" />
        {usable.map((point) => {
          const pointY = y(point.value)
          const labelY = pointY < top + 32 ? pointY + 22 : pointY - 14
          return <g key={point.n}><circle data-testid={'scaling-point-' + chartID + '-' + point.n} data-x={x(point.n).toFixed(2)} cx={x(point.n)} cy={pointY} r="6" className="prepared-scaling-chart__point" /><text x={x(point.n)} y={labelY} textAnchor="middle" className="prepared-line-chart__value">{formatter(point.value)}</text></g>
        })}
      </svg>
    </figure>
  )
}

function statusText(point: PresentationScalingPoint): string {
  return point.equilibrium_status === 'equilibrium' ? 'равновесие подтверждено' : point.equilibrium_note || 'равновесие не подтверждено'
}

function exactText(point: PresentationScalingPoint): string {
  return point.exact_status === 'exact' ? 'точный оптимум' : point.exact_note || 'оптимум не вычислен'
}

function scenarioByID(data: PresentationResponse, id: string): PresentationScenario {
  return data.scenarios.find((scenario) => scenario.id === id) ?? data.scenarios[0]
}

export function PresentationPage() {
  const [data, setData] = useState<PresentationResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [mode, setMode] = useState<PresentationMode>('full')
  const [slidePosition, setSlidePosition] = useState(0)
  const [fullscreen, setFullscreen] = useState(false)
  const [scalingIndex, setScalingIndex] = useState(0)
  const stageRef = useRef<HTMLElement>(null)
  const activeSlides = mode === 'full' ? fullSlides : shortSlides
  const slide = activeSlides[slidePosition]

  useEffect(() => {
    const controller = new AbortController()
    api.presentation(controller.signal).then((response) => { setData(response); setError(null) }).catch((reason: unknown) => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : 'Не удалось открыть подготовленные материалы.')
    })
    return () => controller.abort()
  }, [])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'ArrowRight') { event.preventDefault(); setSlidePosition((current) => Math.min(activeSlides.length - 1, current + 1)) }
      if (event.key === 'ArrowLeft') { event.preventDefault(); setSlidePosition((current) => Math.max(0, current - 1)) }
      if (event.key === 'Home') { event.preventDefault(); setSlidePosition(0) }
      if (event.key === 'End') { event.preventDefault(); setSlidePosition(activeSlides.length - 1) }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [activeSlides.length])

  useEffect(() => {
    const syncFullscreen = () => setFullscreen(document.fullscreenElement === stageRef.current)
    document.addEventListener('fullscreenchange', syncFullscreen)
    return () => document.removeEventListener('fullscreenchange', syncFullscreen)
  }, [])

  const toggleFullscreen = async () => {
    try {
      if (document.fullscreenElement === stageRef.current) await document.exitFullscreen?.()
      else await stageRef.current?.requestFullscreen?.()
    } catch { setError('Браузер не разрешил полноэкранный режим.') }
  }

  const selectMode = (nextMode: PresentationMode) => { setMode(nextMode); setSlidePosition(0) }

  if (!data) {
    return <section className="presentation-page presentation-page--prepared" aria-labelledby="presentation-page-title"><header className="presentation-page__header"><div><p className="eyebrow">Защита курсовой · подготовленные результаты</p><h1 id="presentation-page-title">Равновесие Нэша при распределении вычислительной нагрузки</h1></div></header>{error ? <Notice tone="error">{error}</Notice> : <Loading label="Открываю подготовленные слайды…" />}</section>
  }

  const scenarioA = scenarioByID(data, 'A')
  const scenarioB = scenarioByID(data, 'B')
  const scenarioC = scenarioByID(data, 'C')
  const complex = data.complex_scenario
  const aMove = scenarioA.trace[0]
  const bTrace = [{ step: 0, potential: String(scenarioB.initial.potential ?? '') }, ...scenarioB.trace.map((step) => ({ step: step.step, potential: String(step.potential) }))]
  const complexTrace = [{ step: 0, potential: String(complex.initial.potential ?? '') }, ...complex.trace.map((step) => ({ step: step.step, potential: String(step.potential) }))]
  const bRows = [scenarioB.initial, ...scenarioB.trace.map((step) => step.summary)].slice(0, 3)
  const selectedScaling = data.scaling_series.length ? data.scaling_series[scalingIndex % data.scaling_series.length] : undefined
  const scalingFourthNode = data.scaling_setup.nodes.find((node) => node.id === 'У₄') ?? data.scaling_setup.nodes.at(-1)
  const seriesPlayerValues = [...new Set(data.series.map((point) => point.players))].sort((a, b) => a - b)
  const seriesNodeValues = [...new Set(data.series.map((point) => point.nodes))].sort((a, b) => a - b)
  const equalOptimalSeriesCount = data.series.filter((point) => Math.abs(numberValue(point.k_run) - 1) < 0.000001).length
  const nonOptimalSeriesCount = data.series.length - equalOptimalSeriesCount
  const shortestSeriesTrace = Math.min(...data.series.map((point) => point.steps))
  const longestSeriesTrace = Math.max(...data.series.map((point) => point.steps))
  const counterText = mode === 'short' ? 'Экран ' + (slidePosition + 1) + ' из ' + activeSlides.length + ' · слайд ' + (slide + 1) : 'Слайд ' + (slide + 1) + ' из ' + activeSlides.length
  const slideAriaLabel = mode === 'short' ? 'Краткий показ: экран ' + (slidePosition + 1) + ' из ' + activeSlides.length + ', слайд ' + (slide + 1) : 'Полный показ: слайд ' + (slide + 1) + ' из ' + activeSlides.length

  const renderSlide = () => {
    switch (slide) {
      case 0:
        return <>
          <div className="prepared-hero">
            <SlideTitle number={1} eyebrow="Тема и цель" title="Равновесие Нэша при распределении вычислительной нагрузки">Независимый выбор узла формирует устойчивое распределение. Цель — исследовать путь к нему и сравнить его с оптимумом системы.</SlideTitle>
            <div className="prepared-hero__statement" aria-label="Цель исследования"><span><b>1</b> задача выбирает узел</span><i aria-hidden="true">→</i><span><b>2</b> нагрузка меняет время</span><i aria-hidden="true">→</i><span><b>3</b> ищем устойчивое распределение</span></div>
          </div>
          <div className="prepared-model-sets"><span>N — задачи</span><span>R — узлы</span><span>s<sub>и</sub> — выбор задачи</span><p>Каждая задача выбирает один узел; число выбранных задач образует его нагрузку.</p></div>
          <TaskAllocation nodes={complex.nodes} summary={complex.initial} label="Конкретное начальное распределение: 8 задач и 4 узла" />
        </>
      case 1:
        return <>
          <SlideTitle number={2} eyebrow="Математическая модель" title="Время задачи зависит от нагрузки на выбранном узле" />
          <div className="prepared-model-layout">
            <div className="prepared-formula-card" data-testid="formula-latency">
              <div className="prepared-model-rule"><p className="prepared-model-rule__label">Нагрузка узла</p><div className="prepared-formula prepared-formula--large">n<sub>у</sub>(s)</div><p className="prepared-model-rule__hint prepared-model-rule__hint--definition">Число задач, выбравших узел у в распределении s.</p></div>
              <div className="prepared-model-rule"><p className="prepared-model-rule__label">Время конкретной задачи</p><div className="prepared-formula prepared-formula--large">t<sub>и</sub>(s) = T<sub>s<sub>и</sub></sub>(n<sub>s<sub>и</sub></sub>(s))</div><p className="prepared-model-rule__hint prepared-model-rule__hint--definition">Все задачи на одном узле получают одинаковое время, заданное его текущей нагрузкой.</p></div>
              <div className="prepared-model-rule"><p className="prepared-model-rule__label">Один частный случай</p><div className="prepared-formula prepared-formula--large">T<sub>у</sub>(k) = a<sub>у</sub> + b<sub>у</sub>k</div><p className="prepared-model-rule__hint">Линейная функция удобна для примера, но не ограничивает общую модель.</p></div>
            </div>
            <ModelLines data={data} />
          </div>
        </>
      case 2:
        return <>
          <SlideTitle number={3} eyebrow="Исследовательские вопросы" title="Теория, контрпример и вычислительные наблюдения — разные результаты">Программа не доказывает теорему: она воспроизводит её предпосылки и проверяет конкретные конечные случаи.</SlideTitle>
          <div className="prepared-research-questions" aria-label="Математическое утверждение, метод проверки, результат">
            <ResearchQuestion index={1} label="теоретический результат" statement="Строгие улучшения в конечной игре завершаются в NE" method="потенциал Φ и трассы опытов А, Б" result="траектории иллюстрируют теорему" />
            <ResearchQuestion index={2} label="математический контрпример" statement="NE не обязано минимизировать S" method="полный перебор малого опыта В" result="S_NE = 4, S_opt = 3" />
            <ResearchQuestion index={3} label="вычислительное исследование" statement="Как меняются H и K_найд при изменении n?" method="функции, старт и правило фиксированы; n = 8, 16, 32" result="три рассчитанные точки без вывода о влиянии старта или монотонности" />
          </div>
        </>
      case 3:
        return <>
          <SlideTitle number={4} eyebrow="Потенциальная функция Розенталя" title="Выгодный переход уменьшает потенциал на ту же величину" />
          <div className="prepared-potential-layout">
            <section className="prepared-proof-card"><p className="prepared-model-rule__label">Потенциал распределения</p><div className="prepared-formula prepared-formula--large">Φ(s) = Σ<sub>j=1</sub><sup>m</sup> Σ<sub>q=1</sub><sup>nⱼ(s)</sup> T<sub>j</sub>(q)</div><p>Суммируются времена всех уровней нагрузки каждого узла, а не только времена задач в текущем распределении.</p></section>
            <PotentialShiftDiagram />
          </div>
          <div className="prepared-theory-conclusion"><div className="prepared-formula prepared-formula--large">ΔΦ = Δt<sub>i</sub> &lt; 0</div><p>Число распределений конечно, поэтому в принятой игре с перегрузкой последовательность строго выгодных односторонних переходов заканчивается в равновесии Нэша.</p></div>
        </>
      case 4:
        return <>
          <SlideTitle number={5} eyebrow="Опыт А · два игрока" title="Один строго выгодный переход приводит к равновесию" />
          <div className="prepared-transition-layout">
            <TaskAllocation nodes={scenarioA.nodes} summary={scenarioA.initial} label="До перехода: s⁰ = (B,B)" compact />
            <section className="prepared-transition-proof"><span>игрок 1: B → A</span><div>t<sub>1</sub>(s⁰) = T<sub>B</sub>(2) = {formatExact(aMove?.previous ?? '4')}</div><i aria-hidden="true">↓</i><div>t<sub>1</sub>(s¹) = T<sub>A</sub>(1) = {formatExact(aMove?.next ?? '1')}</div><strong>Δt<sub>1</sub> = {formatExact(aMove?.next ?? '1')} − {formatExact(aMove?.previous ?? '4')} = {formatExact(aMove?.delta ?? '-3')} &lt; 0</strong></section>
            <TaskAllocation nodes={scenarioA.nodes} summary={scenarioA.final} label="После перехода: s¹ = (A,B)" compact />
          </div>
          <div className="prepared-metric-row"><Metric label="Φ: до → после" value={summaryValue(scenarioA.initial, 'potential') + ' → ' + summaryValue(scenarioA.final, 'potential')} accent /><Metric label="S: до → после" value={summaryValue(scenarioA.initial, 'total_time') + ' → ' + summaryValue(scenarioA.final, 'total_time')} /><Metric label="Проверка игрока 1" value="1 ≤ 4" note="переход B не улучшает время" /><Metric label="Проверка игрока 2" value="2 ≤ 2" note="переход A не даёт строгого выигрыша" /></div>
          <div className="prepared-result-strip"><b>(A,B) ∈ NE</b><span>Изменение Φ и изменение S здесь не отождествляются: показаны как разные величины.</span></div>
        </>
      case 5:
        return <>
          <SlideTitle number={6} eyebrow="Опыт Б · четыре игрока" title="Два улучшения: потенциал убывает, затем проверяется равновесие" />
          <div className="prepared-experiment-b-layout">
            <PotentialLine points={bTrace} title="Φ на траектории опыта Б" />
            <section className="prepared-state-table-wrap">
              <table className="prepared-state-table"><thead><tr><th>Распределение</th><th>Φ</th><th>S</th></tr></thead><tbody>{bRows.map((summary, index) => <tr key={profileText(summary.assignment) + '-' + index}><td>{profileText(summary.assignment)}</td><td>{summaryValue(summary, 'potential')}</td><td>{summaryValue(summary, 'total_time')}</td></tr>)}</tbody></table>
              <div className="prepared-move-cards"><div><span>1-й переход</span><b>{formatExact(scenarioB.trace[0]?.previous ?? '4')} → {formatExact(scenarioB.trace[0]?.next ?? '1.5')}</b><strong>Δt<sub>1</sub> = {formatExact(scenarioB.trace[0]?.delta ?? '-2.5')}</strong></div><div><span>2-й переход</span><b>{formatExact(scenarioB.trace[1]?.previous ?? '3')} → {formatExact(scenarioB.trace[1]?.next ?? '2')}</b><strong>Δt<sub>2</sub> = {formatExact(scenarioB.trace[1]?.delta ?? '-1')}</strong></div></div>
            </section>
          </div>
          <div className="prepared-result-strip"><b>Финальная проверка</b><span>T<sub>A</sub>(2) = 2 &lt; T<sub>B</sub>(3) = 2,5</span><span>T<sub>B</sub>(2) = 2 &lt; T<sub>A</sub>(3) = 3</span><strong>{profileText(scenarioB.final.assignment)} ∈ NE</strong></div>
        </>
      case 6:
        return <>
          <SlideTitle number={7} eyebrow="Опыт В · контрпример" title="Равновесие не всегда минимизирует суммарное время" />
          <div className="prepared-counterexample-layout">
            <section className="prepared-counterexample-proof"><span>У равновесного распределения {profileText(scenarioC.final.assignment)}:</span><strong>t<sub>1</sub>(A,A) = 2 = t<sub>1</sub>(B,A)</strong><strong>t<sub>2</sub>(A,A) = 2 = t<sub>2</sub>(A,B)</strong><b>(A,A) ∈ NE</b></section>
            <div className="prepared-stability"><article className="prepared-stability__equilibrium"><span>равновесие</span><strong>{profileText(scenarioC.final.assignment)}</strong><Metric label="S(A,A)" value={summaryValue(scenarioC.final, 'total_time')} /></article><div className="prepared-stability__gap"><b>≠</b><span>устойчивость<br />и минимум</span></div><article className="prepared-stability__optimum"><span>точный оптимум</span><strong>{profileText(scenarioC.optimum.assignment)}</strong><Metric label="S_opt = S(A,B) = S(B,A)" value={summaryValue(scenarioC.optimum, 'total_time')} accent /></article></div>
          </div>
          <div className="prepared-coefficient"><span>K<sub>найд</sub> = S<sub>равн</sub> / S<sub>опт</sub></span><strong>{formatExact(scenarioC.k_run)} ≈ {approximate(scenarioC.k_run)}</strong><small>Для этой малой игры после полного перебора цена анархии также равна 4/3; это не переносится автоматически на другие игры.</small></div>
        </>
      case 7:
        return <>
          <SlideTitle number={8} eyebrow="Сложный вычислительный случай" title={'8 задач · 4 неоднородных узла · ' + complex.trace.length + ' выгодных переходов'} />
          <div className="prepared-complex-functions">{complex.nodes.map((node) => <article key={node.id}><b>{nodeSymbol(node.id)}</b><NodeFormula node={node} /></article>)}</div>
          <p className="prepared-experiment-identity"><strong>Отдельный сложный опыт.</strong> T<sub>у</sub>(k) — время одной задачи при k задачах на узле; у узла 4 значения заданы таблицей. При n = 8, m = 4 получено H = {complex.trace.length}; это не точка серии слайда 9.</p>
          <div className="prepared-complex-main">
            <PotentialLine points={complexTrace} title={'Потенциал Φ(s) на ' + complex.trace.length + ' переходах'} />
            <section className="prepared-complex-evidence"><div><span>Старт</span><strong>S = {summaryValue(complex.initial, 'total_time')}</strong><small>n(s⁰) = {loadVector(complex.nodes, complex.initial)}</small></div><div><span>Найдено NE</span><strong>S = {summaryValue(complex.final, 'total_time')}</strong><small>n(s*) = {loadVector(complex.nodes, complex.final)} · Φ(s*) = {summaryValue(complex.final, 'potential')}</small></div><div><span>Точный оптимум</span><strong>S<sub>opt</sub> = {summaryValue(complex.optimum, 'total_time')}</strong><small>n(s<sub>opt</sub>) = {loadVector(complex.nodes, complex.optimum)}</small></div></section>
          </div>
          <div className="prepared-complex-calculation"><div><span>Конечное распределение {loadVector(complex.nodes, complex.final)}</span><strong>S = {totalTimeFormula(complex.nodes, complex.final)} = {summaryValue(complex.final, 'total_time')}</strong></div><div><span>Оптимальное распределение {loadVector(complex.nodes, complex.optimum)}</span><strong>S<sub>opt</sub> = {totalTimeFormula(complex.nodes, complex.optimum)} = {summaryValue(complex.optimum, 'total_time')}</strong></div><b>K<sub>найд</sub> = {formatExact(complex.k_run)} ≈ {approximate(complex.k_run)} · превышение ≈ 6,3 %</b></div>
          <div className="prepared-status-row"><span>✓ равновесие подтверждено: проверено {complex.nash.checked_moves ?? 0} односторонних переходов</span><span>✓ точный оптимум получен перебором</span><small>Это математическая модель нагрузки, а не измерение реального оборудования.</small></div>
        </>
      case 8:
        return <div className={'prepared-series-slide' + (mode === 'short' ? ' prepared-series-slide--short' : '')}>
          <SlideTitle number={9} eyebrow="Контролируемая серия" title="Контролируемая серия: меняется только n">{mode === 'short' ? 'Два графика и четыре показателя для трёх заранее рассчитанных конфигураций.' : 'Узлы, стартовое распределение и правило первого строгого улучшения фиксированы.'}</SlideTitle>
          {mode === 'full' ? <div className="prepared-series-design" aria-label="Условия контролируемой серии"><article><strong>Фиксированы</strong><span>4 формульных функции T<sub>у</sub>(k) · все задачи начинают на узле 1 · фиксированное правило выбора перехода</span></article><i aria-hidden="true">→</i><article><strong>Меняется</strong><span>только n: 8 → 16 → 32</span></article><i aria-hidden="true">→</i><article><strong>Измеряем</strong><span>H — число выгодных переходов · K<sub>найд</sub> = S<sub>равн</sub> / S<sub>опт</sub></span></article></div> : <p className="prepared-series-brief">Фиксировано: функции узлов, старт на узле 1 и правило выбора перехода.</p>}
          {mode === 'full' ? <div className="prepared-scaling-functions" aria-label="Фиксированные функции четырёх узлов">{data.scaling_setup.nodes.map((node) => <span key={node.id}><b>{nodeSymbol(node.id)}</b><NodeFormula node={node} /></span>)}</div> : null}
          <p className="prepared-experiment-contrast"><strong>Другой опыт, чем на слайде 8:</strong> там у узла 4 таблица и H = {complex.trace.length}; здесь у узла 4 {scalingFourthNode ? <NodeFormula node={scalingFourthNode} /> : null}, поэтому при n = 8 получено H = 15.</p>
          <div className="prepared-scaling-layout"><ScalingChart chartID="h" title="Число выгодных переходов H(n)" points={data.scaling_series} value={(point) => point.h} axis="H" formatter={(item) => String(Math.round(item))} /><ScalingChart chartID="k" title="Эффективность найденного равновесия K_найд(n)" points={data.scaling_series} value={(point) => point.k_run === undefined ? undefined : numberValue(point.k_run)} axis="K_найд" formatter={(item) => approximate(item)} referenceValue={1} referenceLabel="K = 1 — совпадение с оптимумом" /></div>
          {selectedScaling ? <section className="prepared-scaling-focus" aria-label={'Конфигурация n=' + selectedScaling.n}><div data-testid="scaling-active-config"><span>Выбрана конфигурация</span><strong>n = {selectedScaling.n}, m = {selectedScaling.m}</strong></div><Metric label="H" value={selectedScaling.h} /><Metric label="S_равн" value={formatExact(selectedScaling.s_equilibrium)} /><Metric label="S_опт" value={formatExact(selectedScaling.s_optimum)} /><Metric label="K_найд" value={selectedScaling.k_run === undefined ? 'не вычислен' : formatExact(selectedScaling.k_run) + ' ≈ ' + approximate(selectedScaling.k_run)} accent />{mode === 'full' ? <div className="prepared-scaling-focus__status"><span>{statusText(selectedScaling)}</span><span>{exactText(selectedScaling)}</span></div> : null}<button type="button" className="button button--secondary" aria-label="Следующая конфигурация масштабной серии" onClick={() => setScalingIndex((index) => data.scaling_series.length ? (index + 1) % data.scaling_series.length : 0)}>Следующая конфигурация: 8 → 16 → 32</button></section> : null}
          {mode === 'full' ? <><p className="prepared-series-note">Каждая точка получена математическим ядром. Графики описывают эти три расчёта и не утверждают обязательную монотонность H или K<sub>найд</sub>.</p><details className="prepared-legacy-series" data-testid="legacy-series-details"><summary>Шесть исходных конфигураций — раскрыть в полном режиме</summary><div className="prepared-series-lead"><span>{data.series.length} рассчитанных конфигураций</span><span>задачи: {seriesPlayerValues.join(', ')} · узлы: {seriesNodeValues.join(', ')}</span></div><SeriesGrid points={data.series} /></details></> : null}
        </div>
      default:
        return <>
          <SlideTitle number={10} eyebrow="Итоговые математические результаты" title="Три вывода из теории и подготовленных вычислительных опытов" />
          <div className="prepared-conclusions"><article><i>1</i><h2>Достижение равновесия</h2><p>Для конечной игры с перегрузкой последовательность строгих улучшений заканчивается в равновесии Нэша. Проведённые расчёты иллюстрируют этот теоретический результат.</p></article><article><i>2</i><h2>Равновесие и оптимум различаются</h2><p>Получен явный контрпример: <b>S<sub>равн</sub> = {summaryValue(scenarioC.final, 'total_time')}</b>, <b>S<sub>опт</sub> = {summaryValue(scenarioC.optimum, 'total_time')}</b>.</p></article><article><i>3</i><h2>Характеристики различаются</h2><p>В шести исходных конфигурациях H составило от {shortestSeriesTrace} до {longestSeriesTrace}. В {equalOptimalSeriesCount} случаях сумма времён равна оптимуму, в {nonOptimalSeriesCount} — превышает его.</p></article></div>
          <div className="prepared-closing">Построенная модель позволяет математически исследовать достижение равновесий Нэша и оценивать, насколько устойчивое распределение вычислительной нагрузки отличается от оптимального.</div>
        </>
    }
  }

  return (
    <section className="presentation-page presentation-page--prepared" aria-labelledby="presentation-page-title">
      <header className="presentation-page__header"><div><p className="eyebrow">Защита курсовой · подготовленные результаты</p><h1 id="presentation-page-title">Равновесие Нэша при распределении вычислительной нагрузки</h1><div className="prepared-mode-switch" role="group" aria-label="Режим показа презентации"><button type="button" className={mode === 'full' ? 'prepared-mode-switch__button prepared-mode-switch__button--active' : 'prepared-mode-switch__button'} aria-pressed={mode === 'full'} onClick={() => selectMode('full')}>Полный показ · 10 слайдов</button><button type="button" className={mode === 'short' ? 'prepared-mode-switch__button prepared-mode-switch__button--active' : 'prepared-mode-switch__button'} aria-pressed={mode === 'short'} onClick={() => selectMode('short')}>Краткий показ · до 3 минут</button></div></div><p className="presentation-page__hint">Стрелки <kbd>←</kbd><kbd>→</kbd> — слайды · <kbd>Home</kbd>/<kbd>End</kbd> — начало/итог</p></header>
      {error ? <Notice tone="error">{error}</Notice> : null}
      {mode === 'short' ? <p className="prepared-short-route">Маршрут защиты: 5 → 7 → 8 → 9 · до 3 минут</p> : null}
      <section ref={stageRef} className="presentation-stage presentation-stage--prepared" aria-live="polite" aria-atomic="true">
        <article className="presentation-slide presentation-slide--prepared" data-testid="presentation-slide" aria-label={slideAriaLabel}>{renderSlide()}</article>
        <footer className="presentation-controls"><button type="button" className="button button--secondary" onClick={() => setSlidePosition((current) => Math.max(0, current - 1))} disabled={slidePosition === 0}>← Предыдущий</button><span className="presentation-counter">{counterText}</span><button type="button" className="button button--primary" onClick={() => setSlidePosition((current) => Math.min(activeSlides.length - 1, current + 1))} disabled={slidePosition === activeSlides.length - 1}>Следующий →</button><button type="button" className="button button--secondary presentation-fullscreen" onClick={() => void toggleFullscreen()}>{fullscreen ? 'Выйти из полного экрана' : 'На весь экран'}</button></footer>
      </section>
    </section>
  )
}
