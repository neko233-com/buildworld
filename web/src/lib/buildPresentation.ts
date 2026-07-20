type Translate = (key: string) => string

const triggerKeys: Record<string, string> = {
  manual: 'builds.triggerManual',
  retry: 'builds.triggerRetry',
  webhook: 'builds.triggerWebhook',
  schedule: 'builds.triggerSchedule',
  scheduled: 'builds.triggerSchedule',
  http: 'builds.triggerHttp',
  api: 'builds.triggerApi',
}

function translatedOrFallback(t: Translate, key: string, fallback: string): string {
  const translated = t(key)
  return translated === key ? fallback : translated
}

export function buildStatusLabel(t: Translate, status?: string): string {
  const normalized = (status || 'pending').toLowerCase()
  return translatedOrFallback(t, `builds.${normalized}`, status || '-')
}

export function buildStatusTone(status?: string): 'success' | 'failed' | 'running' | 'pending' | 'cancelled' {
  const normalized = (status || 'pending').toLowerCase()
  if (normalized === 'success' || normalized === 'passed') return 'success'
  if (normalized === 'failed' || normalized === 'rejected' || normalized === 'error') return 'failed'
  if (normalized === 'running') return 'running'
  if (normalized === 'cancelled' || normalized === 'skipped') return 'cancelled'
  return 'pending'
}

export function buildTriggerLabel(t: Translate, trigger?: string): string {
  if (!trigger) return '-'
  const normalized = trigger.toLowerCase()
  const key = triggerKeys[normalized]
  return key ? translatedOrFallback(t, key, trigger) : trigger
}
