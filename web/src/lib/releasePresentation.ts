export function validReleaseEnvironment(value: string): boolean {
  return value.trim() === value && /^[a-z][a-z0-9-]{0,31}$/.test(value)
}

export function releaseActionLabel(t: (key: string) => string, action: string): string {
  if (action === 'restart-version') return t('projectDetail.restartHistoricalVersion')
  if (action === 'publish-latest') return t('projectDetail.publishLatest')
  return action
}
