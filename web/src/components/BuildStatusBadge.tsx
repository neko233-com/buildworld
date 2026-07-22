import { LoaderCircle } from 'lucide-react'
import { buildStatusTone } from '../lib/buildPresentation'

type BuildStatusBadgeProps = {
  status?: string
  label: string
}

export function BuildStatusBadge({ status, label }: BuildStatusBadgeProps) {
  const tone = buildStatusTone(status)
  const live = ['running', 'pending', 'queued', 'pending_approval'].includes((status || '').toLowerCase())

  return <span
    className={`build-status ${tone}${live ? ' build-status-live' : ''}`}
    role={live ? 'status' : undefined}
    aria-live={live ? 'polite' : undefined}
  >
    {live && <LoaderCircle className="timeline-spinner build-status-spinner" size={11} strokeWidth={2.2} aria-hidden="true" />}
    <span>{label}</span>
  </span>
}
