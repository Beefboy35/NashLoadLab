import type { CalculationStatus, ExactValue, LatencyKind } from '../api/types'

export function formatExact(value: ExactValue | number | null | undefined): string {
  if (value === null || value === undefined || value === '') return 'не вычислено'
  const text = String(value).trim()
  const match = /^(-?)(\d+)(?:\.(\d+))?$/.exec(text)
  if (!match) return text

  const [, sign, integer, fraction] = match
  const grouped = integer.replace(/\B(?=(\d{3})+(?!\d))/g, '\u00A0')
  return `${sign}${grouped}${fraction === undefined ? '' : `,${fraction}`}`
}

export function statusText(status: CalculationStatus | undefined): string {
  switch (status) {
    case 'exact':
      return 'точно вычислено'
    case 'not_computed':
      return 'не вычислено'
    case 'cancelled':
      return 'отменено'
    case 'error':
      return 'ошибка вычисления'
    default:
      return 'не вычислено'
  }
}

export function latencyFormula(kind: LatencyKind, a: string, b: string): string {
  if (kind === 'table') return 'T(k) задано таблицей значений'
  const load = kind === 'quadratic' ? 'k²' : 'k'
  return `T(k) = ${formatExact(a)} + ${formatExact(b)} · ${load}`
}

export function nodeLabel(nodeId: string, names: Map<string, string>): string {
  return names.get(nodeId) ?? nodeId
}
