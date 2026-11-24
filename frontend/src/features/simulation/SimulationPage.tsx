import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api/client'
import type { AllocationSummary, TraceStep } from '../../api/types'
import { PotentialChart } from '../../components/charts'
import { Card, Loading, Notice, PageHeading } from '../../components/ui'
import { useLab } from '../../app/lab-context'
import { formatExact, nodeLabel } from '../../lib/format'
import { isSafeSeed } from '../../lib/seed'

function MoveExplanation({ step, names }: { step: TraceStep | undefined; names: Map<string, string> }) {
  if (step === undefined) return <p className="muted">Шаг 0: показано исходное распределение.</p>
  return (
    <p className="transition-text">
      Задание {step.player} перешло с узла {nodeLabel(step.from, names)} на узел {nodeLabel(step.to, names)}:
      новое время {formatExact(step.next)} меньше прежнего {formatExact(step.previous)};
      ΔF = Δt = {formatExact(step.delta)}.
    </p>
  )
}

function CurrentTables({ summary, names }: { summary: AllocationSummary | undefined; names: Map<string, string> }) {
  if (summary === undefined) {
    return <Notice tone="warning">Сервер не вернул состояние для выбранного шага.</Notice>
  }

  const assignment = summary.assignment ?? []
  const loads = summary.loads ?? []

  return (
    <div className="two-column">
      <section>
        <h3>Задания</h3>
        <div className="table-scroll">
          <table>
            <thead><tr><th scope="col">Задание</th><th scope="col">Выбранный узел</th></tr></thead>
            <tbody>{assignment.map((nodeId, index) => <tr key={index}><th scope="row">{index + 1}</th><td>{nodeLabel(nodeId, names)}</td></tr>)}</tbody>
          </table>
        </div>
      </section>
      <section>
        <h3>Вычислительные узлы</h3>
        <div className="table-scroll">
          <table>
            <thead><tr><th scope="col">Узел</th><th scope="col">Загрузка</th></tr></thead>
            <tbody>{loads.map((node) => <tr key={node.node_id}><th scope="row">{node.node_name ?? nodeLabel(node.node_id, names)}</th><td>{node.load}</td></tr>)}</tbody>
          </table>
        </div>
      </section>
    </div>
  )
}

export function SimulationPage() {
  const { draft, simulation, setSimulation, initialMode } = useLab()
  const [cursor, setCursor] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setCursor(0)
    setPlaying(false)
  }, [simulation])

  useEffect(() => {
    if (!playing || simulation === null) return undefined
    const timer = window.setInterval(() => {
      setCursor((current) => {
        if (current >= simulation.trace.length) {
          setPlaying(false)
          return current
        }
        return current + 1
      })
    }, 700)
    return () => window.clearInterval(timer)
  }, [playing, simulation])

  const runSimulation = async () => {
    if (initialMode === 'random' && !isSafeSeed(draft.seed)) {
      setError('Для случайного начального распределения требуется корректное целое фиксированное семя.')
      return
    }
    setLoading(true)
    setError(null)
    setPlaying(false)
    try {
      setSimulation(await api.createSimulation(draft))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось выполнить расчёт.')
    } finally {
      setLoading(false)
    }
  }

  const result = simulation
  const currentStep = result === null || cursor === 0 ? undefined : result.trace[cursor - 1]
  const currentSummary = result === null
    ? undefined
    : cursor === 0
      ? result.initial
      : currentStep?.summary
  const nodeNames = useMemo(() => new Map(result?.request.nodes.map((node) => [node.id, node.name]) ?? []), [result])
  const isAtEnd = result !== null && cursor >= result.trace.length
  const randomSeedInvalid = initialMode === 'random' && !isSafeSeed(draft.seed)
  const equilibriumText = result?.nash_check.is_equilibrium ? 'Равновесие Нэша подтверждено независимой проверкой.' : 'Равновесие Нэша не подтверждено.'

  return (
    <>
      <PageHeading title="Поиск равновесия">
        Сервер строит трассу односторонних строгих улучшений; воспроизведение ниже не пересчитывает игру и не подменяет данные.
      </PageHeading>

      <Card title="Расчёт">
        <p>Игроков: {draft.players}; узлов: {draft.nodes.length}; политика: {draft.algorithm === 'first_improvement' ? 'первое строгое улучшение' : 'лучший ответ'}.</p>
        {randomSeedInvalid ? <Notice tone="warning">Выбран случайный старт, но фиксированное семя не задано или некорректно. Расчёт заблокирован.</Notice> : null}
        <div className="button-row">
          <button type="button" className="button button--primary" onClick={() => void runSimulation()} disabled={loading || randomSeedInvalid}>{loading ? 'Расчёт…' : 'Рассчитать трассу'}</button>
          <Link className="button button--secondary" to="/model">Изменить модель</Link>
        </div>
        {error ? <Notice tone="error">{error}</Notice> : null}
      </Card>

      {loading ? <Loading label="Сервер рассчитывает последовательность улучшений…" /> : null}
      {result === null && !loading ? <Notice tone="info">Результата пока нет. Настройте модель и запустите серверный расчёт.</Notice> : null}

      {result !== null ? (
        <>
          <Card title="Пошаговая динамика">
            <div className="playback-controls" aria-label="Управление воспроизведением">
              <button type="button" className="button button--secondary" onClick={() => { setPlaying(false); setCursor((value) => Math.min(value + 1, result.trace.length)) }} disabled={isAtEnd}>Шаг вперёд</button>
              <button type="button" className="button button--primary" onClick={() => setPlaying((value) => !value)} disabled={isAtEnd}>{playing ? 'Пауза' : 'Воспроизвести'}</button>
              <button type="button" className="button button--secondary" onClick={() => { setPlaying(false); setCursor(0) }} disabled={cursor === 0}>Сброс</button>
              <output className="step-output" aria-live="polite">Шаг {cursor} из {result.trace.length}</output>
            </div>
            <MoveExplanation step={currentStep} names={nodeNames} />
            <div className="metric-grid">
              <div className="metric"><span>Потенциальная функция F</span><strong>{formatExact(currentSummary?.potential)}</strong></div>
              <div className="metric"><span>Суммарное время S</span><strong>{formatExact(currentSummary?.total_time)}</strong></div>
            </div>
            <CurrentTables summary={currentSummary} names={nodeNames} />
          </Card>

          <Card title="Потенциальная функция">
            <PotentialChart initial={result.initial?.potential} steps={result.trace} visibleCount={cursor} />
          </Card>

          <Card title="Независимая проверка равновесия">
            <Notice tone={result.nash_check.is_equilibrium ? 'success' : 'warning'}>{equilibriumText}</Notice>
            {result.nash_check.checked_moves !== undefined ? <p>Проверено допустимых переходов: {result.nash_check.checked_moves}.</p> : null}
            {result.nash_check.explanation ? <p>{result.nash_check.explanation}</p> : null}
            {result.nash_check.violations && result.nash_check.violations.length > 0 ? (
              <div className="table-scroll">
                <table>
                  <thead><tr><th scope="col">Задание</th><th scope="col">Текущий узел</th><th scope="col">Выгодный узел</th><th scope="col">Текущее время</th><th scope="col">Время после перехода</th></tr></thead>
                  <tbody>{result.nash_check.violations.map((move) => <tr key={`${move.player}-${move.from}-${move.to}`}><td>{move.player}</td><td>{nodeLabel(move.from, nodeNames)}</td><td>{nodeLabel(move.to, nodeNames)}</td><td>{formatExact(move.current)}</td><td>{formatExact(move.candidate)}</td></tr>)}</tbody>
                </table>
              </div>
            ) : null}
            {result.status ? <p className="muted">Статус расчёта: {result.status}{result.message ? ` — ${result.message}` : ''}.</p> : null}
            {isAtEnd ? <Link className="button button--primary" to="/comparison">Сравнить с оптимумом</Link> : null}
          </Card>
        </>
      ) : null}
    </>
  )
}
