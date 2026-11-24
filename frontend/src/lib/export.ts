function escapeCsv(value: string | number | null | undefined): string {
  const text = value === null || value === undefined ? '' : String(value)
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text
}

function download(filename: string, type: string, contents: string): void {
  const blob = new Blob([contents], { type: `${type};charset=utf-8` })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.append(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

export function downloadCsv(filename: string, columns: string[], rows: Array<Array<string | number | null | undefined>>): void {
  const contents = [columns, ...rows].map((row) => row.map(escapeCsv).join(';')).join('\r\n')
  download(filename, 'text/csv', `\uFEFF${contents}`)
}

export function downloadJson(filename: string, value: unknown): void {
  download(filename, 'application/json', `${JSON.stringify(value, null, 2)}\n`)
}
