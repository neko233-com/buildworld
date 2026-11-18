import { useState, useEffect, useCallback, useRef, type SetStateAction } from 'react'

// useApi: simple data fetching hook with loading/error state
export function useApi<T>(fetcher: () => Promise<T>, deps: any[] = []) {
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const scope = useCallback(() => fetcher(), deps)
  const [result, setResult] = useState<{ scope: typeof scope; data: T | null; error: string | null; settled: boolean }>()
  const requestID = useRef(0)
  const active = useRef<number | null>(null)

  const run = useCallback(async (background: boolean) => {
    if (background && active.current !== null) return
    const activeRequest = ++requestID.current
    active.current = activeRequest
    if (!background) {
      setResult(current => current?.scope === scope
        ? { ...current, error: null, settled: current.data !== null }
        : current)
    }
    try {
      const data = await scope()
      if (activeRequest === requestID.current) setResult({ scope, data, error: null, settled: true })
    } catch (reason) {
      const error = reason instanceof Error ? reason.message : String(reason)
      if (activeRequest === requestID.current) {
        setResult(current => ({ scope, data: current?.scope === scope ? current.data : null, error, settled: true }))
      }
    } finally {
      if (active.current === activeRequest) active.current = null
    }
  }, [scope])
  const reload = useCallback(() => run(false), [run])
  const poll = useCallback(() => run(true), [run])
  const setData = useCallback((update: SetStateAction<T | null>) => {
    requestID.current++
    active.current = null
    setResult(current => ({
      scope,
      data: typeof update === 'function' ? (update as (data: T | null) => T | null)(current?.scope === scope ? current.data : null) : update,
      error: null,
      settled: true,
    }))
  }, [scope])

  useEffect(() => {
    void reload()
    return () => {
      requestID.current++
      active.current = null
    }
  }, [reload])

  const current = result?.scope === scope ? result : undefined
  return { data: current?.data ?? null, loading: !current?.settled, error: current?.error ?? null, reload, poll, setData }
}

export function useVisiblePolling(refresh: () => void, interval: number | null) {
  useEffect(() => {
    if (interval === null) return
    const tick = () => {
      if (document.visibilityState === 'visible') refresh()
    }
    let timer = window.setInterval(tick, interval)
    const visibilityChanged = () => {
      window.clearInterval(timer)
      if (document.visibilityState !== 'visible') return
      tick()
      timer = window.setInterval(tick, interval)
    }
    document.addEventListener('visibilitychange', visibilityChanged)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', visibilityChanged)
    }
  }, [refresh, interval])
}
