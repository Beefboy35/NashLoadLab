import { describe, expect, it } from 'vitest'
import { formatExact, statusText } from './format'

describe('formatExact', () => {
  it('сохраняет точную дробную часть и использует русскую десятичную запятую', () => {
    expect(formatExact('12345.50')).toBe('12\u00a0345,50')
    expect(formatExact('-0.5')).toBe('-0,5')
  })

  it('не подменяет отсутствующее значение нулём', () => {
    expect(formatExact(undefined)).toBe('не вычислено')
    expect(statusText('not_computed')).toBe('не вычислено')
  })
})
