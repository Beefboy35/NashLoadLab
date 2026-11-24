import { formatExact } from '../lib/format'
import { downloadCsv, downloadJson } from '../lib/export'
import type { ExperimentPoint, NodeLoad, TraceStep } from '../api/types'

interface PotentialChartProps {
  initial?: string
  steps: TraceStep[]
  visibleCount?: number
}

/**
 * Converts an exact API value only for SVG coordinates. Tables, tooltips and
 * exports keep the original string, so a rational such as "4/3" is never
 * presented as a rounded result.
 */
function finite(value: string | number | undefined): number | null {
  if (value === undefined) return null
  if (typeof value === 'number') return Number.isFinite(value) ? value : null

  const text = value.trim()
  const fraction = /^([+-]?\d+)\/([+-]?\d+)$/.exec(text)
  if (fraction) {
    const numerator = Number(fraction[1])
    const denominator = Number(fraction[2])
    if (!Number.isFinite(numerator) || !Number.isFinite(denominator) || denominator === 0) return null
    const quotient = numerator / denominator
    return Number.isFinite(quotient) ? quotient : null
  }

  const parsed = Number(text)
  return Number.isFinite(parsed) ? parsed : null
}

export function PotentialChart({ initial, steps, visibleCount = steps.length }: PotentialChartProps) {
  const values = [initial, ...steps.slice(0, visibleCount).map((step) => step.potential)]
    .map(finite)
    .filter((value): value is number => value !== null)

  if (values.length === 0) return <p className="muted">Данные потенциальной функции пока не получены.</p>

  const maximum = Math.max(...values)
  const minimum = Math.min(...values)
  const spread = maximum - minimum || 1
  const width = 640
  const height = 220
  const pad = 34
  const points = values.map((value, index) => {
    const x = pad + (index * (width - pad * 2)) / Math.max(values.length - 1, 1)
    const y = height - pad - ((value - minimum) * (height - pad * 2)) / spread
    return `${x},${y}`
  }).join(' ')
  const fullTableData = [
    ...(initial === undefined ? [] : [{ step: 0, potential: initial }]),
    ...steps.map((step) => ({ step: step.step, potential: step.potential })),
  ]

  return (
    <figure className="chart" aria-labelledby="potential-caption">
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label="График потенциальной функции по номеру шага">
        <line x1={pad} y1={height - pad} x2={width - pad} y2={height - pad} className="chart__axis" />
        <line x1={pad} y1={pad} x2={pad} y2={height - pad} className="chart__axis" />
        <polyline points={points} className="chart__line" />
        {points.split(' ').map((point, index) => {
          const [cx, cy] = point.split(',')
          return <circle key={index} cx={cx} cy={cy} r="3.5" className="chart__point" />
        })}
        <text x={pad} y={height - 8} className="chart__label">0</text>
        <text x={width - pad} y={height - 8} textAnchor="end" className="chart__label">шаг</text>
        <text x={pad + 4} y={pad - 8} className="chart__label">F</text>
      </svg>
      <figcaption id="potential-caption">Потенциальная функция F: только реальные точки сервера, включая шаг 0.</figcaption>
      <details>
        <summary>Полная таблица данных графика F</summary>
        <table>
          <thead><tr><th scope="col">Шаг</th><th scope="col">F</th></tr></thead>
          <tbody>
            {initial !== undefined ? <tr><th scope="row">0</th><td>{formatExact(initial)}</td></tr> : null}
            {steps.map((step) => <tr key={step.step}><th scope="row">{step.step}</th><td>{formatExact(step.potential)}</td></tr>)}
          </tbody>
        </table>
      </details>
      <div className="chart-actions">
        <button type="button" className="button button--secondary" onClick={() => downloadCsv('potential-F.csv', ['шаг', 'F'], fullTableData.map((row) => [row.step, row.potential]))}>Экспорт F в CSV</button>
        <button type="button" className="button button--secondary" onClick={() => downloadJson('potential-F.json', fullTableData)}>Экспорт F в JSON</button>
      </div>
    </figure>
  )
}

interface LoadBarsProps {
  title: string
  loads?: NodeLoad[]
  sharedMaximum: number
}

export function LoadBars({ title, loads = [], sharedMaximum }: LoadBarsProps) {
  return (
    <section className="load-bars" aria-label={title}>
      <h3>{title}</h3>
      {loads.length === 0 ? <p className="muted">Нагрузка не вычислена.</p> : (
        <>
          <div className="load-bars__plot" role="img" aria-label={`${title}. Масштаб от нуля до ${sharedMaximum} заданий`}>
            {loads.map((entry) => {
              const width = sharedMaximum === 0 ? 0 : Math.max(0, (entry.load / sharedMaximum) * 100)
              return (
                <div className="load-bars__row" key={entry.node_id}>
                  <span className="load-bars__label">{entry.node_name ?? entry.node_id}</span>
                  <div className="load-bars__track"><span className="load-bars__bar" style={{ width: `${width}%` }} /></div>
                  <strong>{entry.load}</strong>
                </div>
              )
            })}
          </div>
          <table>
            <thead><tr><th scope="col">Узел</th><th scope="col">Нагрузка</th><th scope="col">Время</th></tr></thead>
            <tbody>{loads.map((entry) => <tr key={entry.node_id}><th scope="row">{entry.node_name ?? entry.node_id}</th><td>{entry.load}</td><td>{formatExact(entry.latency)}</td></tr>)}</tbody>
          </table>
          <div className="chart-actions">
            <button type="button" className="button button--secondary" onClick={() => downloadCsv('node-loads.csv', ['узел', 'нагрузка', 'время'], loads.map((entry) => [entry.node_name ?? entry.node_id, entry.load, entry.latency ?? null]))}>Экспорт загрузок в CSV</button>
            <button type="button" className="button button--secondary" onClick={() => downloadJson('node-loads.json', loads)}>Экспорт загрузок в JSON</button>
          </div>
        </>
      )}
    </section>
  )
}

interface SeriesDefinition {
  key: string
  label: string
  color: string
  value: (point: ExperimentPoint) => string | number | undefined
}

function ExperimentSeriesChart({ title, series, points }: { title: string; series: SeriesDefinition[]; points: ExperimentPoint[] }) {
  const plotted = series.map((definition) => ({
    ...definition,
    values: points.map((point, index) => ({ index, value: finite(definition.value(point)) })).filter((entry): entry is { index: number; value: number } => entry.value !== null),
  }))
  const values = plotted.flatMap((definition) => definition.values.map((entry) => entry.value))
  if (values.length === 0) return <p className="muted">{title}: нет числовых точек для построения.</p>

  const width = 360
  const height = 190
  const pad = 30
  const maximum = Math.max(...values, 0)
  const spread = maximum || 1
  const xAt = (index: number) => pad + (index * (width - pad * 2)) / Math.max(points.length - 1, 1)
  const yAt = (value: number) => height - pad - (value * (height - pad * 2)) / spread

  return (
    <figure className="chart experiment-series" aria-label={title}>
      <h3>{title}</h3>
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${title}; показаны только точки, возвращённые сервером`}>
        <line x1={pad} y1={height - pad} x2={width - pad} y2={height - pad} className="chart__axis" />
        <line x1={pad} y1={pad} x2={pad} y2={height - pad} className="chart__axis" />
        {plotted.flatMap((definition) => definition.values.map((entry) => (
          <circle key={`${definition.key}-${entry.index}`} cx={xAt(entry.index)} cy={yAt(entry.value)} r="4" fill={definition.color}>
            <title>{`${definition.label}, точка ${entry.index + 1}: ${entry.value}`}</title>
          </circle>
        )))}
        <text x={pad} y={height - 8} className="chart__label">1</text>
        <text x={width - pad} y={height - 8} textAnchor="end" className="chart__label">точка серии</text>
        <text x={pad + 4} y={pad - 8} className="chart__label">0 — {formatExact(String(maximum))}</text>
      </svg>
      <figcaption className="chart__legend">{series.map((definition) => <span key={definition.key}><i style={{ background: definition.color }} />{definition.label}</span>)}</figcaption>
    </figure>
  )
}

export function ExperimentChart({ points }: { points: ExperimentPoint[] }) {
  if (points.length === 0) return <p className="muted">Точки серии ещё не получены.</p>
  return (
    <div className="experiment-chart" aria-label="Серия экспериментов">
      <div className="experiment-chart__grid">
        <ExperimentSeriesChart title="Число шагов" points={points} series={[{ key: 'steps', label: 'шаги', color: '#146c94', value: (point) => point.steps }]} />
        <ExperimentSeriesChart title="Отношение найденного равновесия к оптимуму K_run" points={points} series={[{ key: 'k-run', label: 'K_run', color: '#6d4c9b', value: (point) => point.k_run }]} />
        <ExperimentSeriesChart title="Суммарное время" points={points} series={[
          { key: 'ne', label: 'S_NE', color: '#0f766e', value: (point) => point.total_time_ne },
          { key: 'opt', label: 'S_opt', color: '#b45309', value: (point) => point.total_time_opt },
        ]} />
      </div>
      <div className="table-scroll">
        <table>
          <thead><tr><th scope="col">n</th><th scope="col">m</th><th scope="col">a</th><th scope="col">b</th><th scope="col">Шаги</th><th scope="col">K_run</th><th scope="col">S_NE</th><th scope="col">S_opt</th></tr></thead>
          <tbody>
            {points.map((point, index) => (
              <tr key={`${point.players}-${point.nodes}-${index}`}>
                <td>{point.players}</td><td>{point.nodes}</td><td>{formatExact(point.a)}</td><td>{formatExact(point.b)}</td><td>{point.steps ?? '—'}</td>
                <td>{formatExact(point.k_run)}</td><td>{formatExact(point.total_time_ne)}</td><td>{formatExact(point.total_time_opt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
