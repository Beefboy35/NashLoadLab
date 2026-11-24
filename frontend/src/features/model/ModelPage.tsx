import { Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import type { ChangeEvent } from 'react'
import { api } from '../../api/client'
import type { NodeInput, PreviewResponse, SimulationRequest } from '../../api/types'
import { Card, Loading, Notice, PageHeading, RequiredMark } from '../../components/ui'
import { formatExact, latencyFormula } from '../../lib/format'
import { isSafeSeed, SAFE_SEED_MAX, SAFE_SEED_MIN, seededAssignment } from '../../lib/seed'
import { useLab } from '../../app/lab-context'

function numberOrCurrent(event: ChangeEvent<HTMLInputElement>, current: number, minimum: number, maximum: number): number {
  const next = Number(event.target.value)
  return Number.isInteger(next) && next >= minimum && next <= maximum ? next : current
}

/**
 * Табличная функция должна иметь по одному значению для каждой возможной
 * положительной загрузки. При расширении создаём именно пустые поля, а не
 * придумываем значения времени; при уменьшении убираем недостижимые нагрузки.
 */
function tableValuesForPlayers(values: readonly string[] | undefined, players: number): string[] {
  const retained = values?.slice(0, players) ?? []
  return retained.length === players
    ? retained
    : [...retained, ...Array.from({ length: players - retained.length }, () => '')]
}

function withPlayers(draft: SimulationRequest, players: number): SimulationRequest {
  const fallbackNode = draft.nodes[0]?.id ?? ''
  const assignment = Array.from({ length: players }, (_, index) => draft.initial_assignment[index] ?? fallbackNode)
  return {
    ...draft,
    players,
    nodes: draft.nodes.map((node) => node.kind === 'table'
      ? { ...node, table_values: tableValuesForPlayers(node.table_values, players) }
      : node),
    initial_assignment: assignment,
  }
}

function LatencyPreview({ nodes, players }: { nodes: NodeInput[]; players: number }) {
  const [preview, setPreview] = useState<PreviewResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const signature = JSON.stringify({ players, nodes })

  useEffect(() => {
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      setLoading(true)
      setError(null)
      void api.preview({ players, nodes }, controller.signal)
        .then(setPreview)
        .catch((reason: unknown) => {
          if (reason instanceof DOMException && reason.name === 'AbortError') return
          setPreview(null)
          setError(reason instanceof Error ? reason.message : 'Не удалось получить предварительный просмотр.')
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false)
        })
    }, 350)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  // Полная сериализация модели намеренно служит ключом debounce: запрос идёт только после изменения входных данных.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [signature])

  const valueAt = (load: number, nodeId: string): string | undefined =>
    preview?.rows.find((row) => row.load === load)?.values.find((cell) => cell.node_id === nodeId)?.value

  return (
    <div className="latency-preview">
      <p className="muted">Предварительный просмотр дискретной зависимости T<sub>j</sub>(k) от загрузки k рассчитан сервером.</p>
      {loading ? <Loading label="Сервер рассчитывает таблицу задержек…" /> : null}
      {error ? <Notice tone="error">Предварительный просмотр не получен: {error}</Notice> : null}
      {preview === null ? null : (
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th scope="col">k</th>
              {nodes.map((node) => <th scope="col" key={node.id}>{node.name}</th>)}
            </tr>
          </thead>
          <tbody>
            {preview.rows.map((row) => (
              <tr key={row.load}>
                <th scope="row">{row.load}</th>
                {nodes.map((node) => <td key={node.id}>{formatExact(valueAt(row.load, node.id))}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      )}
    </div>
  )
}

export function ModelPage() {
  const { draft, replaceDraft, presets, presetsLoading, presetsError, initialMode, setInitialMode, applyPreset } = useLab()

  const commitDraft = (nextDraft: SimulationRequest) => {
    if (initialMode === 'random' && isSafeSeed(nextDraft.seed)) {
      replaceDraft({ ...nextDraft, initial_assignment: seededAssignment(nextDraft.players, nextDraft.nodes.map((node) => node.id), nextDraft.seed) })
      return
    }
    replaceDraft(nextDraft)
  }

  const commitManualDraft = (nextDraft: SimulationRequest) => {
    setInitialMode('manual')
    replaceDraft(nextDraft)
  }

  const updateNode = (nodeId: string, patch: Partial<NodeInput>) => {
    const nextId = patch.id ?? nodeId
    commitDraft({
      ...draft,
      nodes: draft.nodes.map((node) => node.id === nodeId ? { ...node, ...patch } : node),
      initial_assignment: nextId === nodeId
        ? draft.initial_assignment
        : draft.initial_assignment.map((assignment) => assignment === nodeId ? nextId : assignment),
    })
  }

  const setNodeKind = (nodeId: string, kind: NodeInput['kind']) => {
    const node = draft.nodes.find((candidate) => candidate.id === nodeId)
    updateNode(nodeId, {
      kind,
      ...(kind === 'table' ? { table_values: tableValuesForPlayers(node?.table_values, draft.players) } : {}),
    })
  }

  const setTableValue = (nodeId: string, loadIndex: number, value: string) => {
    const node = draft.nodes.find((candidate) => candidate.id === nodeId)
    const tableValues = tableValuesForPlayers(node?.table_values, draft.players)
    tableValues[loadIndex] = value
    updateNode(nodeId, { table_values: tableValues })
  }

  const addNode = () => {
    if (draft.nodes.length >= 40) return
    let suffix = draft.nodes.length + 1
    let id = `N${suffix}`
    while (draft.nodes.some((node) => node.id === id)) {
      suffix += 1
      id = `N${suffix}`
    }
    commitDraft({
      ...draft,
      nodes: [...draft.nodes, { id, name: `Узел ${suffix}`, a: '0', b: '1', kind: 'linear' }],
    })
  }

  const removeNode = (nodeId: string) => {
    if (draft.nodes.length <= 1) return
    const nodes = draft.nodes.filter((node) => node.id !== nodeId)
    const replacement = nodes[0]?.id ?? ''
    commitDraft({
      ...draft,
      nodes,
      initial_assignment: draft.initial_assignment.map((assignment) => assignment === nodeId ? replacement : assignment),
    })
  }

  const distributeAll = (nodeId: string) => commitManualDraft({
    ...draft,
    initial_assignment: Array.from({ length: draft.players }, () => nodeId),
  })

  const distributeRoundRobin = () => commitManualDraft({
    ...draft,
    initial_assignment: Array.from({ length: draft.players }, (_, index) => draft.nodes[index % draft.nodes.length]?.id ?? ''),
  })

  return (
    <>
      <PageHeading title="Модель">
        Задайте игроков (вычислительные задания), узлы и начальное распределение. Время выполнения — условные единицы модели.
      </PageHeading>

      <Card title="Справка по обозначениям">
        <dl className="symbol-help">
          <div><dt>n, m</dt><dd>Число заданий (игроков) и число вычислительных узлов.</dd></div>
          <div><dt>T<sub>j</sub>(k)</dt><dd>Время выполнения на узле j при целочисленной загрузке k.</dd></div>
          <div><dt>F</dt><dd>Потенциальная функция Розенталя; каждое строгое улучшение уменьшает F.</dd></div>
          <div><dt>S</dt><dd>Суммарное время выполнения всех заданий. F и S — разные величины.</dd></div>
          <div><dt>K<sub>run</sub></dt><dd>Отношение суммарного времени одного найденного равновесия к оптимуму.</dd></div>
          <div><dt>PoA</dt><dd>Отношение худшего равновесия всей игры к оптимуму; показывается только после полного точного перебора.</dd></div>
        </dl>
      </Card>

      <Card title="Эталонные пресеты">
        {presetsLoading ? <Loading label="Загрузка пресетов из сервера…" /> : null}
        {presetsError ? <Notice tone="error">Пресеты не загружены: {presetsError}</Notice> : null}
        {!presetsLoading && !presetsError && presets.length === 0 ? <Notice tone="warning">Сервер не вернул доступных пресетов.</Notice> : null}
        {presets.length > 0 ? (
          <ul className="preset-list" aria-label="Эталонные пресеты">
            {presets.map((preset) => (
              <li key={preset.id} className="preset-list__item">
                <div><strong>{preset.name}</strong><p>{preset.description}</p></div>
                <button type="button" className="button button--secondary" onClick={() => applyPreset(preset)}>Открыть</button>
              </li>
            ))}
          </ul>
        ) : null}
      </Card>

      <form className="model-form" onSubmit={(event) => event.preventDefault()}>
        <Card title="Игроки и алгоритм">
          <div className="field-grid">
            <label className="field">
              <span>Число игроков<RequiredMark /></span>
              <input type="number" min="1" max="500" step="1" value={draft.players} onChange={(event) => commitDraft(withPlayers(draft, numberOrCurrent(event, draft.players, 1, 500)))} required />
            </label>
            <label className="field">
              <span>Политика динамики<RequiredMark /></span>
              <select value={draft.algorithm} onChange={(event) => commitDraft({ ...draft, algorithm: event.target.value as SimulationRequest['algorithm'] })}>
                <option value="first_improvement">Первое строгое улучшение</option>
                <option value="best_response">Лучший ответ</option>
              </select>
            </label>
            <label className="field">
              <span>Лимит шагов<RequiredMark /></span>
              <input type="number" min="1" max="500" step="1" value={draft.max_steps} onChange={(event) => commitDraft({ ...draft, max_steps: numberOrCurrent(event, draft.max_steps, 1, 500) })} required />
            </label>
            <label className="field">
              <span>Фиксированное семя</span>
              <input type="number" min={SAFE_SEED_MIN} max={SAFE_SEED_MAX} step="1" value={draft.seed ?? ''} placeholder="Не задано" aria-invalid={draft.seed !== undefined && !isSafeSeed(draft.seed)} onChange={(event) => {
                const value = event.target.value.trim()
                const seed = value === '' ? undefined : Number(value)
                const nextDraft = { ...draft, seed }
                if (initialMode === 'random' && isSafeSeed(seed)) {
                  replaceDraft({ ...nextDraft, initial_assignment: seededAssignment(nextDraft.players, nextDraft.nodes.map((node) => node.id), seed) })
                } else {
                  replaceDraft(nextDraft)
                }
              }} />
              <small>Необязательное целое seed в безопасном диапазоне JavaScript: от {SAFE_SEED_MIN} до {SAFE_SEED_MAX}.</small>
            </label>
          </div>
        </Card>

        <Card title="Вычислительные узлы">
          <p className="muted">Выберите формулу T<sub>j</sub>(k)=a<sub>j</sub>+b<sub>j</sub>·k, a<sub>j</sub>+b<sub>j</sub>·k² или таблицу T<sub>j</sub>(1), …, T<sub>j</sub>(n). Для формул сервер проверит a≥0 и b&gt;0. Для таблицы нужно заполнить все значения T<sub>j</sub>(1), …, T<sub>j</sub>(n): они должны быть неотрицательными и неубывающими.</p>
          <div className="table-scroll">
            <table className="editor-table">
              <thead><tr><th scope="col">ID</th><th scope="col">Название</th><th scope="col">a (формула)</th><th scope="col">b (формула)</th><th scope="col">Вид T<sub>j</sub>(k)</th><th scope="col"><span className="sr-only">Действие</span></th></tr></thead>
              <tbody>
                {draft.nodes.map((node) => (
                  <tr key={node.id}>
                    <td><input aria-label={`ID ${node.name}`} maxLength={64} value={node.id} onChange={(event) => updateNode(node.id, { id: event.target.value })} /></td>
                    <td><input aria-label={`Название ${node.id}`} maxLength={120} value={node.name} onChange={(event) => updateNode(node.id, { name: event.target.value })} /></td>
                    <td>{node.kind === 'table' ? <span className="editor-table__not-applicable">Не используется</span> : <input aria-label={`Коэффициент a узла ${node.name}`} maxLength={256} inputMode="decimal" value={node.a} onChange={(event) => updateNode(node.id, { a: event.target.value })} />}</td>
                    <td>{node.kind === 'table' ? <span className="editor-table__not-applicable">Не используется</span> : <input aria-label={`Коэффициент b узла ${node.name}`} maxLength={256} inputMode="decimal" value={node.b} onChange={(event) => updateNode(node.id, { b: event.target.value })} />}</td>
                    <td>
                      <select aria-label={`Вид функции узла ${node.name}`} value={node.kind} onChange={(event) => setNodeKind(node.id, event.target.value as NodeInput['kind'])}>
                        <option value="linear">линейная</option><option value="quadratic">квадратичная</option><option value="table">таблица значений</option>
                      </select>
                      <small>{latencyFormula(node.kind, node.a, node.b)}</small>
                    </td>
                    <td><button type="button" className="button button--danger" disabled={draft.nodes.length === 1} onClick={() => removeNode(node.id)}>Удалить<span className="sr-only"> узел {node.name}</span></button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {draft.nodes.filter((node) => node.kind === 'table').length > 0 ? (
            <section className="table-values-section" aria-labelledby="table-values-title">
              <h3 id="table-values-title">Значения табличных функций</h3>
              <p className="muted">Заполните время T<sub>j</sub>(k) для каждой нагрузки k от 1 до {draft.players}. Все значения должны быть неотрицательными и неубывающими. Пустые поля не подменяются значениями: сервер сообщит, какие данные нужно исправить.</p>
              {draft.nodes.filter((node) => node.kind === 'table').map((node) => {
                const values = tableValuesForPlayers(node.table_values, draft.players)
                return (
                  <fieldset className="table-values-fieldset" key={node.id}>
                    <legend>{node.name}: значения T<sub>j</sub>(k)</legend>
                    <div className="table-values-grid">
                      {values.map((value, index) => (
                        <label className="table-value-field" key={index}>
                          <span>k = {index + 1}</span>
                          <input
                            aria-label={`Время T(${index + 1}) для узла ${node.name}`}
                            inputMode="decimal"
                            maxLength={256}
                            required
                            value={value}
                            onChange={(event) => setTableValue(node.id, index, event.target.value)}
                          />
                        </label>
                      ))}
                    </div>
                  </fieldset>
                )
              })}
            </section>
          ) : null}
          <button type="button" className="button button--secondary" onClick={addNode} disabled={draft.nodes.length >= 40}>Добавить узел</button>
        </Card>

        <Card title="Начальное распределение">
          <p className="muted">Каждое задание выбирает ровно один узел. Начальное распределение можно задать вручную, поместить все задания на один узел, раздать по кругу или сформировать псевдослучайно по фиксированному семени.</p>
          <div className="button-row">
            {draft.nodes.map((node) => <button key={node.id} type="button" className="button button--secondary" onClick={() => distributeAll(node.id)}>Все на {node.name}</button>)}
            <button type="button" className="button button--secondary" onClick={distributeRoundRobin}>По кругу</button>
            <button type="button" className="button button--secondary" onClick={() => {
              setInitialMode('random')
              if (isSafeSeed(draft.seed)) replaceDraft({ ...draft, initial_assignment: seededAssignment(draft.players, draft.nodes.map((node) => node.id), draft.seed) })
            }}>Случайно с фиксированным семенем</button>
          </div>
          {draft.seed !== undefined && !isSafeSeed(draft.seed) ? <Notice tone="warning">Seed должен быть безопасным целым числом JavaScript. Расчёт будет недоступен, пока значение не исправлено.</Notice> : null}
          {initialMode === 'random' && !isSafeSeed(draft.seed) ? <Notice tone="warning">Для случайного распределения укажите корректное фиксированное семя.</Notice> : null}
          {initialMode === 'random' && isSafeSeed(draft.seed) ? <Notice tone="info">Распределение детерминированно сформировано по seed={draft.seed} и будет отправлено серверу вместе с ним.</Notice> : null}
          <div className="assignment-grid">
            {draft.initial_assignment.map((assignment, index) => (
              <label className="field" key={index}>
                <span>Задание {index + 1}</span>
                <select value={assignment} onChange={(event) => {
                  const nextAssignment = [...draft.initial_assignment]
                  nextAssignment[index] = event.target.value
                  commitManualDraft({ ...draft, initial_assignment: nextAssignment })
                }}>
                  {draft.nodes.map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}
                </select>
              </label>
            ))}
          </div>
        </Card>

        <Card title="Предварительный просмотр функции задержки">
          <LatencyPreview nodes={draft.nodes} players={draft.players} />
        </Card>

        <div className="form-actions"><Link className="button button--primary" to="/search">Перейти к поиску равновесия</Link></div>
      </form>
    </>
  )
}
