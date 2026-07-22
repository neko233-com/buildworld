import { LoaderCircle } from 'lucide-react'
import { buildStatusTone } from '../lib/buildPresentation'

type BuildStatusBadgeProps = {
  status?: string
  label: string
}

export function BuildStatusBadge({ status, label }: BuildStatusBadgeProps) {
  const tone = buildStatusTone(status)
  const running = tone === 'running'

  return <span
    className={`build-status ${tone}${running ? ' build-status-live' : ''}`}
    role={running ? 'status' : undefined}
    aria-live={running ? 'polite' : undefined}
  >
    {running && <LoaderCircle className="timeline-spinner build-status-spinner" size={11} strokeWidth={2.2} aria-hidden="true" />}
    <span>{label}</span>
  </span>
}
