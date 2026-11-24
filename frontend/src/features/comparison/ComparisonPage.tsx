import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api/client'
import type { AllocationSummary, NodeLoad, OptimizationRequest } from '../../api/types'
import { LoadBars } from '../../components/charts'
import { Card, Loading, Notice, PageHeading, StatusBadge } from '../../components/ui'
import { useLab } from '../../app/lab-context'
import { formatExact } from '../../lib/format'

function loadsOf(summary: AllocationSummary | undefined): NodeLoad[] {
  return summary?.loads ?? []
}

function maximumLoad(...sets: NodeLoad[][]): number {
  return Math.max(0, ...sets.flatMap((set) => set.map((entry) => entry.load)))
}

export function ComparisonPage() {
  const { draft, simulation, optimization, setOptimization } = useLab()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const runOptimization = async () => {
    if (simulation === null) return
    setLoading(true)
    setError(null)
    const payload: OptimizationRequest = {
      players: draft.players,
      nodes: draft.nodes,
      initial_assignment: simulation.final.assignment ?? simulation.request.initial_assignment,
      algorithm: draft.algorithm,
      max_steps: draft.max_steps,
      max_states: 200_000,
    }
    try {
      setOptimization(await api.optimize(payload))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось рассчитать оптимум.')
    } finally {
      setLoading(false)
    }
  }

  const simulationReachedEquilibrium = simulation?.status === 'equilibrium' && simulation.nash_check.is_equilibrium === true
  // Never infer equilibrium from the final trace state: a step limit or a
  // failed independent check is not a proof. The optimization endpoint only
  // returns `equilibrium` after its own confirmed server-side run.
  const equilibrium = optimization?.equilibrium ?? (simulationReachedEquilibrium ? simulation?.final : undefined)
  const optimum = optimization?.optimum
  const kRunAvailable = equilibrium !== undefined && optimum?.status === 'exact' && optimization?.k_run !== undefined
  const barMaximum = useMemo(
    () => maximumLoad(loadsOf(equilibrium), loadsOf(optimum), loadsOf(optimization?.poa.worst_equilibrium)),
    [equilibrium, optimization?.poa.worst_equilibrium, optimum],
  )

  return (
    <>
      <PageHeading title="Сравнение">
        Сопоставьте найденное равновесие с точным оптимумом. K<sub>run</sub> относится к этому равновесию, а PoA — к худшему равновесию всей игры.
      </PageHeading>

      {simulation === null ? (
        <Card title="Сначала нужна серверная трасса">
          <Notice tone="info">Чтобы сравнить найденное равновесие с оптимумом, сначала выполните поиск равновесия.</Notice>
          <Link className="button button--primary" to="/search">Перейти к поиску</Link>
        </Card>
      ) : (
        <Card title="Точный расчёт">
          <p>Точный перебор ограничен серверным лимитом в 200&nbsp;000 распределений нагрузок. При достижении лимита показатели остаются «не вычислено».</p>
          {!simulationReachedEquilibrium ? (
            <Notice tone="warning">Текущая трасса не подтвердила равновесие Нэша. Её финальное состояние не считается найденным равновесием, а K<sub>run</sub> до подтверждения не вычисляется.</Notice>
          ) : null}
          <button type="button" className="button button--primary" onClick={() => void runOptimization()} disabled={loading}>{loading ? 'Выполняется…' : 'Вычислить оптимум и PoA'}</button>
          {error ? <Notice tone="error">{error}</Notice> : null}
        </Card>
      )}

      {loading ? <Loading label="Сервер перебирает допустимые распределения…" /> : null}
      {optimization !== null && optimum !== undefined ? (
        <>
          <Card title="Статусы точности">
            <dl className="status-list">
              <div><dt>Оптимум S<sub>opt</sub></dt><dd><StatusBadge status={optimum.status} /></dd></div>
              <div><dt>Цена анархии PoA</dt><dd><StatusBadge status={optimization.poa.status} /></dd></div>
            </dl>
            {optimum.message ? <p className="muted">Оптимум: {optimum.message}</p> : null}
            {optimization.poa.message ? <p className="muted">PoA: {optimization.poa.message}</p> : null}
          </Card>

          <Card title="Загрузка узлов">
            <p className="muted">Обе диаграммы начинаются от нуля и используют общий максимум шкалы: {barMaximum}.</p>
            <div className="comparison-grid">
              {equilibrium ? (
                <LoadBars title="Найденное равновесие" loads={loadsOf(equilibrium)} sharedMaximum={barMaximum} />
              ) : (
                <section className="load-bars" aria-label="Равновесие не подтверждено">
                  <h3>Равновесие для запуска</h3>
                  <p className="muted">Не вычислено: финальная точка трассы не была подтверждена как равновесие.</p>
                </section>
              )}
              <LoadBars title="Оптимальное распределение" loads={loadsOf(optimum)} sharedMaximum={barMaximum} />
            </div>
          </Card>

          <Card title="Показатели">
            <div className="metric-grid metric-grid--three">
              <div className="metric"><span>S найденного равновесия</span><strong>{equilibrium ? formatExact(equilibrium.total_time) : 'не вычислено'}</strong></div>
              <div className="metric"><span>S<sub>opt</sub></span><strong>{optimum.status === 'exact' ? formatExact(optimum.total_time) : 'не вычислено'}</strong></div>
              <div className="metric"><span>K<sub>run</sub></span><strong>{kRunAvailable ? formatExact(optimization.k_run) : 'не вычислено'}</strong></div>
            </div>
            {!equilibrium ? <Notice tone="warning">Равновесие для этого запуска не подтверждено; S равновесия и K<sub>run</sub> не вычислены.</Notice> : null}
            <div className="poa-result">
              <h3>PoA</h3>
              {optimization.poa.status === 'exact' && optimization.poa.value !== undefined ? (
                <p><strong>{formatExact(optimization.poa.value)}</strong> — точное отношение худшего равновесия к оптимуму.</p>
              ) : <p>PoA: не вычислено. K<sub>run</sub> не является заменой PoA.</p>}
            </div>
          </Card>

          {optimization.poa.status === 'exact' && optimization.poa.worst_equilibrium ? (
            <Card title="Худшее равновесие для PoA">
              <p>Суммарное время: {formatExact(optimization.poa.worst_equilibrium.total_time)}.</p>
              <LoadBars title="Нагрузка худшего равновесия" loads={loadsOf(optimization.poa.worst_equilibrium)} sharedMaximum={barMaximum} />
            </Card>
          ) : null}
        </>
      ) : null}
    </>
  )
}
