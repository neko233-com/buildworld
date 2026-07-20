import { useState, useEffect, useCallback, useRef } from 'react'
import { api } from './api'

// useApi: simple data fetching hook with loading/error state
export function useApi<T>(fetcher: () => Promise<T>, deps: any[] = []) {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const hasData = useRef(false)
  const requestID = useRef(0)

  const reload = useCallback(() => {
    const activeRequest = ++requestID.current
    if (!hasData.current) setLoading(true)
    setError(null)
    fetcher()
      .then(value => {
        if (activeRequest !== requestID.current) return
        hasData.current = true
        setData(value)
      })
      .catch(e => {
        if (activeRequest === requestID.current) setError(e.message)
      })
      .finally(() => {
        if (activeRequest === requestID.current) setLoading(false)
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  useEffect(() => {
    reload()
    return () => { requestID.current++ }
  }, [reload])

  return { data, loading, error, reload, setData }
}

// useRequireAuth: redirects to /login if no token
export function useRequireAuth() {
  const token = localStorage.getItem('token')
  if (!token) {
    window.location.href = '/login'
    return null
  }
  return token
}

export { api }
