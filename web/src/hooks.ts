import { useState, useEffect, useCallback } from 'react'
import { api } from './api'

// useApi: simple data fetching hook with loading/error state
export function useApi<T>(fetcher: () => Promise<T>, deps: any[] = []) {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const reload = useCallback(() => {
    setLoading(true)
    setError(null)
    fetcher()
      .then(setData)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  useEffect(() => { reload() }, [reload])

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
