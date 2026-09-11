import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'

export const LOG_VIRTUAL_OVERSCAN_LINES = 80
export const LOG_VIRTUALIZATION_THRESHOLD = 160

type ViewportMetrics = {
  scrollTop: number
  height: number
}

export function useVirtualLogWindow(
  viewportRef: RefObject<HTMLElement | null>,
  itemCount: number,
  itemHeight: number,
  enabled = true,
) {
  const [metrics, setMetrics] = useState<ViewportMetrics>({ scrollTop: 0, height: 0 })
  const frameRef = useRef<number | null>(null)

  const measure = useCallback(() => {
    if (frameRef.current !== null) return
    frameRef.current = window.requestAnimationFrame(() => {
      frameRef.current = null
      const viewport = viewportRef.current
      if (!viewport) return
      const next = { scrollTop: viewport.scrollTop, height: viewport.clientHeight }
      setMetrics(current => current.scrollTop === next.scrollTop && current.height === next.height ? current : next)
    })
  }, [viewportRef])

  useLayoutEffect(() => {
    measure()
    const viewport = viewportRef.current
    if (!viewport || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(measure)
    observer.observe(viewport)
    return () => observer.disconnect()
  }, [itemCount, measure, viewportRef])

  useLayoutEffect(() => () => {
    if (frameRef.current !== null) window.cancelAnimationFrame(frameRef.current)
  }, [])

  const viewportHeight = metrics.height || 720
  const totalHeight = itemCount * itemHeight
  const scrollTop = Math.min(metrics.scrollTop, Math.max(0, totalHeight - viewportHeight))
  const start = enabled ? Math.max(0, Math.floor(scrollTop / itemHeight) - LOG_VIRTUAL_OVERSCAN_LINES) : 0
  const end = enabled
    ? Math.min(itemCount, Math.ceil((scrollTop + viewportHeight) / itemHeight) + LOG_VIRTUAL_OVERSCAN_LINES)
    : itemCount

  return {
    start,
    end,
    offsetTop: start * itemHeight,
    totalHeight,
    onScroll: measure,
  }
}
