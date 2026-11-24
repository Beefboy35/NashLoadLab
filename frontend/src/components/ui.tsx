import type { ReactNode } from 'react'
import { statusText } from '../lib/format'
import type { CalculationStatus } from '../api/types'

export function PageHeading({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <header className="page-heading">
      <h1>{title}</h1>
      {children ? <div className="page-heading__description">{children}</div> : null}
    </header>
  )
}

export function Card({ title, children, className = '' }: { title?: string; children: ReactNode; className?: string }) {
  return (
    <section className={`card ${className}`.trim()}>
      {title ? <h2 className="card__title">{title}</h2> : null}
      {children}
    </section>
  )
}

export function Notice({ children, tone = 'info' }: { children: ReactNode; tone?: 'info' | 'error' | 'success' | 'warning' }) {
  return <div className={`notice notice--${tone}`} role={tone === 'error' ? 'alert' : 'status'}>{children}</div>
}

export function StatusBadge({ status }: { status: CalculationStatus }) {
  return <span className={`status status--${status}`}>{statusText(status)}</span>
}

export function RequiredMark() {
  return <span className="required" aria-hidden="true"> *</span>
}

export function Loading({ label = 'Загрузка…' }: { label?: string }) {
  return <p className="loading" role="status">{label}</p>
}
