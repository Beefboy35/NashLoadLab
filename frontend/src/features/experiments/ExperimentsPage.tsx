import { useEffect, useState } from 'react'
import { api } from '../../api/client'
import type { ExperimentRequest, ExperimentSummary } from '../../api/types'
import { ExperimentChart } from '../../components/charts'
import { Card, Loading, Notice, PageHeading, RequiredMark } from '../../components/ui'

const initialRequest: ExperimentRequest = {
  players: { from: 2, to: 6, step: 1 },
  nodes: { from: 2, to: 3, step: 1 },
  a: { from: '0', to: '1', step: '1' },
  b: { from: '1', to: '2', step: '1' },
  algorithm: 'first_improvement',
  seed: 42,
  max_steps: 100,
}

function experimentStatusText(status: ExperimentSummary['status']): string {
  const labels: Record<ExperimentSummary['status'], string> = {
    queued: 'в очереди',
    running: 'выполняется',
    complete: 'завершён',
    cancelled: 'отменён',
    error: 'ошибка',
  }
  return labels[status]
}

function NumberRangeFields({
  label,
  value,
  onChange,
  maximum,
}: {
  label: string
  value: { from: number; to: number; step: number }
  onChange: (next: { from: number; to: number; step: number }) => void
  maximum: number
}) {
  return (
    <fieldset className="range-fieldset">
      <legend>{label}<RequiredMark /></legend>
      {(['from', 'to', 'step'] as const).map((key) => (
        <label className="field" key={key}>
          <span>{key === 'from' ? 'от' : key === 'to' ? 'до' : 'шаг'}</span>
          <input type="number" min="1" max={maximum} step="1" value={value[key]} onChange={(event) => {
            const parsed = Number(event.target.value)
            onChange({ ...value, [key]: Number.isInteger(parsed) && parsed > 0 && parsed <= maximum ? parsed : value[key] })
          }} required />
        </label>
      ))}
    </fieldset>
  )
}

function DecimalRangeFields({
  label,
  value,
  onChange,
}: {
  label: string
  value: { from: string; to: string; step: string }
  onChange: (next: { from: string; to: string; step: string }) => void
}) {
  return (
    <fieldset className="range-fieldset">
      <legend>{label}<RequiredMark /></legend>
      {(['from', 'to', 'step'] as const).map((key) => (
        <label className="field" key={key}>
          <span>{key === 'from' ? 'от' : key === 'to' ? 'до' : 'шаг'}</span>
          <input maxLength={256} inputMode="decimal" value={value[key]} onChange={(event) => onChange({ ...value, [key]: event.target.value })} required />
        </label>
      ))}
    </fieldset>
  )
}

export function ExperimentsPage() {
  const [request, setRequest] = useState<ExperimentRequest>(initialRequest)
  const [history, setHistory] = useState<ExperimentSummary[]>([])
  const [selected, setSelected] = useState<ExperimentSummary | null>(null)
  const [loadingHistory, setLoadingHistory] = useState(true)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refreshHistory = async () => {
    setLoadingHistory(true)
    try {
      const response = await api.experiments()
      setHistory(response.experiments)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось загрузить историю опытов.')
    } finally {
      setLoadingHistory(false)
    }
  }

  useEffect(() => {
    void refreshHistory()
  }, [])

  const submit = async () => {
    setRunning(true)
    setError(null)
    try {
      const created = await api.createExperiment(request)
      setSelected(created)
      await refreshHistory()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось запустить серию.')
    } finally {
      setRunning(false)
    }
  }

  const openExperiment = async (id: string) => {
    setError(null)
    try {
      setSelected(await api.experiment(id))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось открыть опыт.')
    }
  }

  const removeExperiment = async (id: string) => {
    if (!window.confirm('Удалить этот сохранённый опыт?')) return
    setError(null)
    try {
      await api.deleteExperiment(id)
      if (selected?.id === id) setSelected(null)
      await refreshHistory()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось удалить опыт.')
    }
  }

  return (
    <>
      <PageHeading title="Исследование">
        Серия воспроизводимых серверных экспериментов. Все точки, таблицы и выгрузки содержат фактически рассчитанные данные.
      </PageHeading>

      <Card title="Параметры серии">
        <form onSubmit={(event) => { event.preventDefault(); void submit() }}>
          <div className="experiment-ranges">
            <NumberRangeFields label="Число заданий n" maximum={500} value={request.players} onChange={(players) => setRequest({ ...request, players })} />
            <NumberRangeFields label="Число узлов m" maximum={40} value={request.nodes} onChange={(nodes) => setRequest({ ...request, nodes })} />
            <DecimalRangeFields label="Коэффициент a" value={request.a} onChange={(a) => setRequest({ ...request, a })} />
            <DecimalRangeFields label="Коэффициент b" value={request.b} onChange={(b) => setRequest({ ...request, b })} />
          </div>
          <div className="field-grid">
            <label className="field"><span>Политика</span><select value={request.algorithm} onChange={(event) => setRequest({ ...request, algorithm: event.target.value as ExperimentRequest['algorithm'] })}><option value="first_improvement">Первое строгое улучшение</option><option value="best_response">Лучший ответ</option></select></label>
            <label className="field"><span>Семя<RequiredMark /></span><input type="number" step="1" value={request.seed} onChange={(event) => setRequest({ ...request, seed: Number(event.target.value) })} required /></label>
            <label className="field"><span>Лимит шагов<RequiredMark /></span><input type="number" min="1" max="500" step="1" value={request.max_steps} onChange={(event) => setRequest({ ...request, max_steps: Math.min(500, Math.max(1, Number(event.target.value))) })} required /></label>
          </div>
          <button type="submit" className="button button--primary" disabled={running}>{running ? 'Выполняется…' : 'Запустить серию'}</button>
          <p className="muted">Допускается до 10 000 сочетаний параметров. Дополнительно сервер ограничивает объём сохраняемых трасс и время расчёта.</p>
        </form>
        {error ? <Notice tone="error">{error}</Notice> : null}
      </Card>

      {selected ? (
        <Card title={`Опыт ${selected.id}`}>
          <p>Статус: <strong>{experimentStatusText(selected.status)}</strong>{selected.message ? ` — ${selected.message}` : ''}.</p>
          <div className="button-row">
            <a className="button button--secondary" href={api.exportUrl(selected.id, 'csv')}>Выгрузить CSV</a>
            <a className="button button--secondary" href={api.exportUrl(selected.id, 'json')}>Выгрузить JSON</a>
          </div>
          <ExperimentChart points={selected.points ?? []} />
        </Card>
      ) : null}

      <Card title="История опытов">
        {loadingHistory ? <Loading label="Загрузка истории…" /> : null}
        {!loadingHistory && history.length === 0 ? <p className="muted">Сохранённых опытов пока нет.</p> : null}
        {history.length > 0 ? (
          <div className="table-scroll">
            <table>
              <thead><tr><th scope="col">Дата</th><th scope="col">Статус</th><th scope="col">Точек</th><th scope="col"><span className="sr-only">Действия</span></th></tr></thead>
              <tbody>
                {history.map((item) => (
                  <tr key={item.id}>
                    <td>{item.created_at}</td><td>{experimentStatusText(item.status)}</td><td>{item.points?.length ?? '—'}</td>
                    <td className="table-actions"><button type="button" className="button button--secondary" onClick={() => void openExperiment(item.id)}>Открыть</button><button type="button" className="button button--danger" onClick={() => void removeExperiment(item.id)}>Удалить</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </Card>
      {selected?.request ? <p className="muted">Параметры воспроизведения: n={selected.request.players.from}…{selected.request.players.to}; m={selected.request.nodes.from}…{selected.request.nodes.to}; seed={selected.request.seed}.</p> : null}
    </>
  )
}
