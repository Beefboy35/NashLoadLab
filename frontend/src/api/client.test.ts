import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'

describe('api.preview', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('отправляет коэффициенты с десятичной точкой, сохраняя локальный ввод с запятой', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ rows: [] }), {
      headers: { 'Content-Type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await api.preview({
      players: 2,
      nodes: [{ id: 'A', name: 'Узел A', a: '0,5', b: '1,25', kind: 'linear' }],
    })

    expect(fetchMock).toHaveBeenCalledOnce()
    const [, options] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(options.body).toBe(JSON.stringify({
      players: 2,
      nodes: [{ id: 'A', name: 'Узел A', a: '0.5', b: '1.25', kind: 'linear' }],
    }))
  })

  it('отправляет табличные значения с десятичной точкой', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ rows: [] }), {
      headers: { 'Content-Type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await api.preview({
      players: 2,
      nodes: [{
        id: 'A',
        name: 'Узел A',
        a: '0',
        b: '1',
        kind: 'table',
        table_values: ['0,5', '1,25'],
      }],
    })

    const [, options] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(options.body).toBe(JSON.stringify({
      players: 2,
      nodes: [{
        id: 'A',
        name: 'Узел A',
        a: '0',
        b: '1',
        kind: 'table',
        table_values: ['0.5', '1.25'],
      }],
    }))
  })

  it('добавляет к ошибке сервера подробную причину из details.reason', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      message: 'Некорректные параметры модели.',
      details: { reason: 'значения T(A) должны быть неубывающими' },
    }), {
      status: 422,
      headers: { 'Content-Type': 'application/json' },
    })))

    await expect(api.preview({
      players: 2,
      nodes: [{ id: 'A', name: 'Узел A', a: '0', b: '1', kind: 'linear' }],
    })).rejects.toThrow('Некорректные параметры модели. Причина: значения T(A) должны быть неубывающими')
  })
})
