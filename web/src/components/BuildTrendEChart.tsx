import { useEffect, useRef } from 'react'
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { formatDate } from '../lib/dateTime'

echarts.use([BarChart, CanvasRenderer, GridComponent, LegendComponent, LineChart, TooltipComponent])

type TrendPoint = { date: string; success: number; failed: number; running: number }

export function BuildTrendEChart({ data, dark = false }: { data: TrendPoint[]; dark?: boolean }) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const element = ref.current
    if (!element || navigator.userAgent.includes('jsdom')) return
    const chart = echarts.init(element, undefined, { renderer: 'canvas' })
    chart.setOption({
      animationDuration: 420,
      backgroundColor: 'transparent',
      grid: { top: 36, right: 18, bottom: 28, left: 34 },
      tooltip: { trigger: 'axis' },
      legend: { top: 4, textStyle: { color: dark ? '#a8bdd2' : '#647482', fontSize: 10 } },
      xAxis: { type: 'category', data: data.map(point => formatDate(point.date)), axisLine: { lineStyle: { color: dark ? '#29465e' : '#d8e1e9' } }, axisLabel: { color: dark ? '#8fa6bb' : '#738291', fontSize: 9 } },
      yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: dark ? '#1f3a50' : '#edf1f5' } }, axisLabel: { color: dark ? '#8fa6bb' : '#738291', fontSize: 9 } },
      series: [
        { name: '成功', type: 'bar', stack: 'builds', data: data.map(point => point.success), itemStyle: { color: '#22a06b', borderRadius: [3, 3, 0, 0] } },
        { name: '失败', type: 'bar', stack: 'builds', data: data.map(point => point.failed), itemStyle: { color: '#db5862', borderRadius: [3, 3, 0, 0] } },
        { name: '运行中', type: 'line', smooth: true, data: data.map(point => point.running), symbolSize: 6, lineStyle: { color: '#3e8ee8', width: 2 }, itemStyle: { color: '#3e8ee8' } },
      ],
    })
    const observer = new ResizeObserver(() => chart.resize())
    observer.observe(element)
    return () => { observer.disconnect(); chart.dispose() }
  }, [dark, data])

  return <div ref={ref} className="build-trend-echart" role="img" aria-label="构建趋势图" />
}
