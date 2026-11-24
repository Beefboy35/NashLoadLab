import type { ChangeEvent } from 'react'
import type { LatencyKind, NodeInput, SimulationRequest } from '../../api/types'
import { latencyFormula } from '../../lib/format'

const MAX_PLAYERS = 500
const MAX_NODES = 40
const MAX_STEPS = 500

interface LiveScenarioEditorProps {
  draft: SimulationRequest
  onChange: (next: SimulationRequest) => void
}

function numberOrCurrent(event: ChangeEvent<HTMLInputElement>, current: number, minimum: number, maximum: number): number {
  const next = Number(event.target.value)
  return Number.isInteger(next) && next >= minimum && next <= maximum ? next : current
}

function assignmentFor(players: number, nodes: readonly NodeInput[], current: readonly string[]): string[] {
  const availableIds = new Set(nodes.map((node) => node.id))
  const fallback = nodes[0]?.id ?? ''

  return Array.from({ length: players }, (_, index) => {
    const assignment = current[index]
    return assignment !== undefined && availableIds.has(assignment) ? assignment : fallback
  })
}

function tableValuesForPlayers(values: readonly string[] | undefined, players: number): string[] {
  const retained = values?.slice(0, players) ?? []
  return retained.length === players
    ? retained
    : [...retained, ...Array.from({ length: players - retained.length }, () => '')]
}

function nodeIdFor(nodes: readonly NodeInput[]): string {
  let suffix = nodes.length + 1
  let id = `N${suffix}`
  const ids = new Set(nodes.map((node) => node.id))

  while (ids.has(id)) {
    suffix += 1
    id = `N${suffix}`
  }

  return id
}

function tableValuesFromText(value: string): string[] {
  if (value === '') return []
  // A semicolon separates values when a Russian decimal comma is used:
  // "1,5; 2,5; 4". A comma-only list remains convenient for whole values.
  return value.split(value.includes(';') ? ';' : ',').map((item) => item.trim())
}

/**
 * Compact, API-free editor used on the live presentation slide.  It intentionally
 * only maintains a valid-shaped request; validation and all calculations stay on
 * the server when the presentation starts a preview or simulation.
 */
export function LiveScenarioEditor({ draft, onChange }: LiveScenarioEditorProps) {
  const emit = (next: SimulationRequest) => onChange({
    ...next,
    initial_assignment: assignmentFor(next.players, next.nodes, next.initial_assignment),
  })

  const changePlayers = (players: number) => {
    const nodes = draft.nodes.map((node) => node.kind === 'table'
      ? { ...node, table_values: tableValuesForPlayers(node.table_values, players) }
      : node)
    emit({
      ...draft,
      players,
      nodes,
      initial_assignment: assignmentFor(players, nodes, draft.initial_assignment),
    })
  }

  const updateNodeAt = (index: number, patch: Partial<NodeInput>) => {
    const previous = draft.nodes[index]
    if (previous === undefined) return

    const nextId = patch.id ?? previous.id
    const nodes = draft.nodes.map((node, nodeIndex) => nodeIndex === index ? { ...node, ...patch } : node)
    const initialAssignment = nextId === previous.id
      ? draft.initial_assignment
      : draft.initial_assignment.map((assignment) => assignment === previous.id ? nextId : assignment)

    emit({ ...draft, nodes, initial_assignment: initialAssignment })
  }

  const changeKind = (index: number, kind: LatencyKind) => {
    const node = draft.nodes[index]
    if (node === undefined) return

    if (kind === 'table') {
      updateNodeAt(index, { kind, table_values: tableValuesForPlayers(node.table_values, draft.players) })
      return
    }

    const { table_values: _tableValues, ...withoutTableValues } = node
    const nodes = draft.nodes.map((candidate, candidateIndex) => candidateIndex === index
      ? { ...withoutTableValues, kind }
      : candidate)
    emit({ ...draft, nodes })
  }

  const addNode = () => {
    if (draft.nodes.length >= MAX_NODES) return

    const id = nodeIdFor(draft.nodes)
    emit({
      ...draft,
      nodes: [...draft.nodes, { id, name: `Узел ${draft.nodes.length + 1}`, a: '0', b: '1', kind: 'linear' }],
    })
  }

  const removeNodeAt = (index: number) => {
    if (draft.nodes.length <= 1) return

    const nodes = draft.nodes.filter((_, nodeIndex) => nodeIndex !== index)
    emit({ ...draft, nodes, initial_assignment: assignmentFor(draft.players, nodes, draft.initial_assignment) })
  }

  const distributeAllToFirst = () => {
    const firstNode = draft.nodes[0]?.id ?? ''
    emit({ ...draft, initial_assignment: Array.from({ length: draft.players }, () => firstNode) })
  }

  const distributeRoundRobin = () => emit({
    ...draft,
    initial_assignment: Array.from(
      { length: draft.players },
      (_, index) => draft.nodes[index % draft.nodes.length]?.id ?? '',
    ),
  })

  const setAssignment = (playerIndex: number, nodeId: string) => {
    const initialAssignment = assignmentFor(draft.players, draft.nodes, draft.initial_assignment)
    initialAssignment[playerIndex] = nodeId
    emit({ ...draft, initial_assignment: initialAssignment })
  }

  return (
    <section className="live-scenario-editor" aria-label="Параметры живого опыта">
      <div className="field-grid">
        <label className="field">
          <span>Число заданий (игроков)</span>
          <input
            type="number"
            min="1"
            max={MAX_PLAYERS}
            step="1"
            value={draft.players}
            onChange={(event) => changePlayers(numberOrCurrent(event, draft.players, 1, MAX_PLAYERS))}
            required
          />
          <small>От 1 до {MAX_PLAYERS}; при изменении сохраняются доступные назначения.</small>
        </label>
        <label className="field">
          <span>Политика переходов</span>
          <select
            value={draft.algorithm}
            onChange={(event) => emit({ ...draft, algorithm: event.target.value as SimulationRequest['algorithm'] })}
          >
            <option value="first_improvement">Первое строгое улучшение</option>
            <option value="best_response">Лучший ответ</option>
          </select>
        </label>
        <label className="field">
          <span>Лимит шагов</span>
          <input
            type="number"
            min="1"
            max={MAX_STEPS}
            step="1"
            value={draft.max_steps}
            onChange={(event) => emit({
              ...draft,
              max_steps: numberOrCurrent(event, draft.max_steps, 1, MAX_STEPS),
            })}
            required
          />
          <small>Достижение лимита не означает равновесие.</small>
        </label>
      </div>

      <div className="table-values-section">
        <h3>Вычислительные узлы</h3>
        <p className="muted">Для каждого узла укажите свою функцию времени. Таблица задаётся значениями T(1), T(2), …, T(n): список разделяйте запятыми, а для десятичной запятой — точками с запятой.</p>
        <div className="table-scroll">
          <table className="editor-table">
            <thead>
              <tr>
                <th scope="col">ID</th>
                <th scope="col">Название</th>
                <th scope="col">a</th>
                <th scope="col">b</th>
                <th scope="col">Вид T<sub>j</sub>(k)</th>
                <th scope="col">Значения таблицы</th>
                <th scope="col"><span className="sr-only">Действие</span></th>
              </tr>
            </thead>
            <tbody>
              {draft.nodes.map((node, index) => (
                <tr key={index}>
                  <td>
                    <input
                      aria-label={`ID узла ${index + 1}`}
                      maxLength={64}
                      value={node.id}
                      onChange={(event) => updateNodeAt(index, { id: event.target.value })}
                    />
                  </td>
                  <td>
                    <input
                      aria-label={`Название узла ${node.id || index + 1}`}
                      maxLength={120}
                      value={node.name}
                      onChange={(event) => updateNodeAt(index, { name: event.target.value })}
                    />
                  </td>
                  <td>
                    {node.kind === 'table' ? <span className="editor-table__not-applicable">Не используется</span> : (
                      <input
                        aria-label={`Коэффициент a узла ${node.name || node.id || index + 1}`}
                        maxLength={256}
                        inputMode="decimal"
                        value={node.a}
                        onChange={(event) => updateNodeAt(index, { a: event.target.value })}
                      />
                    )}
                  </td>
                  <td>
                    {node.kind === 'table' ? <span className="editor-table__not-applicable">Не используется</span> : (
                      <input
                        aria-label={`Коэффициент b узла ${node.name || node.id || index + 1}`}
                        maxLength={256}
                        inputMode="decimal"
                        value={node.b}
                        onChange={(event) => updateNodeAt(index, { b: event.target.value })}
                      />
                    )}
                  </td>
                  <td>
                    <select
                      aria-label={`Вид функции узла ${node.name || node.id || index + 1}`}
                      value={node.kind}
                      onChange={(event) => changeKind(index, event.target.value as LatencyKind)}
                    >
                      <option value="linear">линейная</option>
                      <option value="quadratic">квадратичная</option>
                      <option value="table">таблица значений</option>
                    </select>
                    <small>{latencyFormula(node.kind, node.a, node.b)}</small>
                  </td>
                  <td>
                    {node.kind === 'table' ? (
                      <input
                        aria-label={`Таблица значений узла ${node.name || node.id || index + 1}`}
                        value={node.table_values?.join(', ') ?? ''}
                        onChange={(event) => updateNodeAt(index, { table_values: tableValuesFromText(event.target.value) })}
                        placeholder="Например: 1; 1,5; 2,5"
                      />
                    ) : <span className="editor-table__not-applicable">Не требуется</span>}
                  </td>
                  <td>
                    <button
                      type="button"
                      className="button button--danger"
                      disabled={draft.nodes.length <= 1}
                      onClick={() => removeNodeAt(index)}
                    >
                      Удалить<span className="sr-only"> узел {node.name || node.id || index + 1}</span>
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="button-row">
          <button type="button" className="button button--secondary" onClick={addNode} disabled={draft.nodes.length >= MAX_NODES}>
            Добавить узел
          </button>
          <span className="muted">{draft.nodes.length} из {MAX_NODES} узлов</span>
        </div>
      </div>

      <div className="table-values-section">
        <h3>Начальное распределение</h3>
        <p className="muted">У каждого задания выбран ровно один вычислительный узел. Это стартовая точка опыта, а не результат расчёта.</p>
        <div className="button-row">
          <button type="button" className="button button--secondary" onClick={distributeAllToFirst}>
            Все на {draft.nodes[0]?.name || 'первый узел'}
          </button>
          <button type="button" className="button button--secondary" onClick={distributeRoundRobin}>
            Распределить по кругу
          </button>
        </div>
        <div className="assignment-grid">
          {assignmentFor(draft.players, draft.nodes, draft.initial_assignment).map((assignment, playerIndex) => (
            <label className="field" key={playerIndex}>
              <span>Задание {playerIndex + 1}</span>
              <select value={assignment} onChange={(event) => setAssignment(playerIndex, event.target.value)}>
                {draft.nodes.map((node, nodeIndex) => (
                  <option key={`${node.id}-${nodeIndex}`} value={node.id}>{node.name || node.id || `Узел ${nodeIndex + 1}`}</option>
                ))}
              </select>
            </label>
          ))}
        </div>
      </div>
    </section>
  )
}
