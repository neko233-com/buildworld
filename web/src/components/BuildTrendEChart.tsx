import { memo, useEffect, useRef } from 'react'
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { formatDate } from '../lib/dateTime'
import { useI18n } from '../i18n'

echarts.use([BarChart, CanvasRenderer, GridComponent, LegendComponent, LineChart, TooltipComponent])

type TrendPoint = { date: string; success: number; failed: number; running: number }

function TrendChart({ data, dark = false }: { data: TrendPoint[]; dark?: boolean }) {
  const { t } = useI18n()
  const ref = useRef<HTMLDivElement>(null)
  const chartRef = useRef<ReturnType<typeof echarts.init> | null>(null)
  const success = t('builds.success')
  const failed = t('builds.failed')
  const running = t('builds.running')
  const accessibleSummary = data.length
    ? data.map(point => `${formatDate(point.date)} · ${success} ${point.success} · ${failed} ${point.failed} · ${running} ${point.running}`).join('; ')
    : t('common.noData')

  useEffect(() => {
    const element = ref.current
    if (!element || navigator.userAgent.includes('jsdom')) return
    const chart = echarts.init(element, undefined, { renderer: 'canvas' })
    chartRef.current = chart
    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => chart.resize()) : null
    observer?.observe(element)
    return () => {
      observer?.disconnect()
      chart.dispose()
      chartRef.current = null
    }
  }, [])

  useEffect(() => {
    chartRef.current?.setOption({
      animation: !window.matchMedia?.('(prefers-reduced-motion: reduce)').matches,
      animationDuration: 420,
      backgroundColor: 'transparent',
      grid: { top: 36, right: 18, bottom: 28, left: 34 },
      tooltip: { trigger: 'axis' },
      legend: { top: 4, textStyle: { color: dark ? '#a8bdd2' : '#647482', fontSize: 10 } },
      xAxis: { type: 'category', data: data.map(point => formatDate(point.date)), axisLine: { lineStyle: { color: dark ? '#29465e' : '#d8e1e9' } }, axisLabel: { color: dark ? '#8fa6bb' : '#738291', fontSize: 9 } },
      yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: dark ? '#1f3a50' : '#edf1f5' } }, axisLabel: { color: dark ? '#8fa6bb' : '#738291', fontSize: 9 } },
      series: [
        { id: 'success', name: success, type: 'bar', stack: 'builds', data: data.map(point => point.success), itemStyle: { color: '#22a06b', borderRadius: [3, 3, 0, 0] } },
        { id: 'failed', name: failed, type: 'bar', stack: 'builds', data: data.map(point => point.failed), itemStyle: { color: '#db5862', borderRadius: [3, 3, 0, 0] } },
        { id: 'running', name: running, type: 'line', smooth: true, data: data.map(point => point.running), symbolSize: 6, lineStyle: { color: '#3e8ee8', width: 2 }, itemStyle: { color: '#3e8ee8' } },
      ],
    })
  }, [dark, data, failed, running, success])

  return <div ref={ref} className="build-trend-echart" role="img" aria-label={`${t('bigScreen.trend7d')} · ${accessibleSummary}`} />
}

export const BuildTrendEChart = memo(TrendChart, (previous, next) =>
  previous.dark === next.dark && previous.data.length === next.data.length && previous.data.every((point, index) => {
    const other = next.data[index]
    return point.date === other.date && point.success === other.success && point.failed === other.failed && point.running === other.running
  }))
