export function formatDuration(milliseconds?: number | null): string {
  if (milliseconds === undefined || milliseconds === null || !Number.isFinite(milliseconds) || milliseconds < 0) return '-'
  if (milliseconds < 1000) return `${Math.round(milliseconds)} ms`
  if (milliseconds < 60_000) {
    const seconds = milliseconds / 1000
    return `${seconds.toFixed(seconds < 10 ? 2 : 1).replace(/\.?0+$/, '')}s`
  }
  if (milliseconds < 3_600_000) {
    const minutes = Math.floor(milliseconds / 60_000)
    const seconds = Math.floor((milliseconds % 60_000) / 1000)
    return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`
  }
  const hours = Math.floor(milliseconds / 3_600_000)
  const minutes = Math.floor((milliseconds % 3_600_000) / 60_000)
  return minutes ? `${hours}h ${minutes}m` : `${hours}h`
}
