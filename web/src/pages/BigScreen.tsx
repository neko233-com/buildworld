import { useState, useEffect, useRef } from 'react'
import { api } from '../api'
import { useI18n } from '../i18n'

interface BigScreenData {
  totalBuilds: number
  running: number
  queued: number
  successToday: number
  failedToday: number
  activeAgents: number
  recentBuilds: Array<{
    id: number
    project_name: string
    number: number
    branch: string
    status: string
    duration: number
    started_at: string
  }>
  trend7d: Array<{
    date: string
    success: number
    failed: number
  }>
  agents: Array<{
    id: string
    name: string
    status: string
    active_builds: number
    max_builds: number
  }>
  projectRanking: Array<{
    id: number
    name: string
    total: number
    success_rate: number
  }>
  recentEvents: Array<{
    id: number
    channel: string
    event: string
    status: string
    time: string
  }>
  systemInfo: {
    goroutines: number
    go_version: string
    os: string
    cpus: number
    uptime: string
  }
}

const mockData: BigScreenData = {
  totalBuilds: 12847,
  running: 8,
  queued: 3,
  successToday: 156,
  failedToday: 12,
  activeAgents: 6,
  recentBuilds: [
    { id: 1, project_name: 'web-frontend', number: 234, branch: 'main', status: 'success', duration: 125, started_at: new Date(Date.now() - 60000).toISOString() },
    { id: 2, project_name: 'api-server', number: 567, branch: 'develop', status: 'running', duration: 45, started_at: new Date(Date.now() - 120000).toISOString() },
    { id: 3, project_name: 'mobile-app', number: 89, branch: 'feature/auth', status: 'failed', duration: 234, started_at: new Date(Date.now() - 300000).toISOString() },
    { id: 4, project_name: 'docs-site', number: 123, branch: 'main', status: 'success', duration: 67, started_at: new Date(Date.now() - 600000).toISOString() },
    { id: 5, project_name: 'backend-service', number: 456, branch: 'hotfix/123', status: 'success', duration: 189, started_at: new Date(Date.now() - 900000).toISOString() },
    { id: 6, project_name: 'cli-tool', number: 78, branch: 'main', status: 'running', duration: 23, started_at: new Date(Date.now() - 1200000).toISOString() },
  ],
  trend7d: [
    { date: '07-04', success: 120, failed: 8 },
    { date: '07-05', success: 145, failed: 15 },
    { date: '07-06', success: 98, failed: 5 },
    { date: '07-07', success: 167, failed: 12 },
    { date: '07-08', success: 134, failed: 9 },
    { date: '07-09', success: 189, failed: 18 },
    { date: '07-10', success: 156, failed: 12 },
  ],
  agents: [
    { id: '1', name: 'build-agent-01', status: 'online', active_builds: 2, max_builds: 4 },
    { id: '2', name: 'build-agent-02', status: 'online', active_builds: 3, max_builds: 4 },
    { id: '3', name: 'build-agent-03', status: 'online', active_builds: 1, max_builds: 4 },
    { id: '4', name: 'build-agent-04', status: 'offline', active_builds: 0, max_builds: 4 },
    { id: '5', name: 'test-agent-01', status: 'online', active_builds: 1, max_builds: 2 },
    { id: '6', name: 'deploy-agent-01', status: 'online', active_builds: 1, max_builds: 2 },
  ],
  projectRanking: [
    { id: 1, name: 'web-frontend', total: 2345, success_rate: 94.5 },
    { id: 2, name: 'api-server', total: 1890, success_rate: 91.2 },
    { id: 3, name: 'mobile-app', total: 1234, success_rate: 87.8 },
    { id: 4, name: 'backend-service', total: 987, success_rate: 95.3 },
    { id: 5, name: 'cli-tool', total: 654, success_rate: 98.1 },
    { id: 6, name: 'docs-site', total: 432, success_rate: 99.2 },
  ],
  recentEvents: [
    { id: 1, channel: 'feishu', event: 'Build Success', status: 'success', time: '2 min ago' },
    { id: 2, channel: 'webhook', event: 'Build Failed', status: 'failed', time: '5 min ago' },
    { id: 3, channel: 'email', event: 'Deploy Complete', status: 'success', time: '12 min ago' },
    { id: 4, channel: 'feishu', event: 'Build Started', status: 'success', time: '18 min ago' },
    { id: 5, channel: 'webhook', event: 'Build Success', status: 'success', time: '25 min ago' },
  ],
  systemInfo: {
    goroutines: 156,
    go_version: 'go1.21.5',
    os: 'linux/amd64',
    cpus: 8,
    uptime: '15d 6h 32m',
  },
}

function Clock() {
  const [time, setTime] = useState(new Date())

  useEffect(() => {
    const timer = setInterval(() => setTime(new Date()), 1000)
    return () => clearInterval(timer)
  }, [])

  const pad = (n: number) => n.toString().padStart(2, '0')

  return (
    <div className="text-cyan-400 font-mono text-2xl tracking-wider">
      <span className="glow-text">{pad(time.getHours())}:{pad(time.getMinutes())}:{pad(time.getSeconds())}</span>
    </div>
  )
}

function StatCard({ label, value, color, icon }: { label: string; value: number | string; color: string; icon?: string }) {
  return (
    <div className="relative bg-white/5 border border-cyan-500/30 rounded-lg p-4 overflow-hidden group hover:border-cyan-400/60 transition-all duration-300">
      <div className="absolute top-0 left-0 w-full h-1 bg-gradient-to-r from-transparent via-cyan-400 to-transparent opacity-50"></div>
      <div className="absolute inset-0 bg-gradient-to-br from-cyan-500/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity"></div>
      <div className="text-gray-400 text-sm mb-2 flex items-center gap-2">
        {icon && <span>{icon}</span>}
        {label}
      </div>
      <div className={`text-4xl font-bold ${color} glow-text`}>
        {value}
      </div>
    </div>
  )
}

function StatusDot({ status }: { status: string }) {
  const colors: Record<string, string> = {
    success: 'bg-green-400 shadow-green-400/50',
    failed: 'bg-red-400 shadow-red-400/50',
    running: 'bg-blue-400 shadow-blue-400/50 animate-pulse',
    pending: 'bg-yellow-400 shadow-yellow-400/50',
    online: 'bg-green-400 shadow-green-400/50',
    offline: 'bg-gray-500 shadow-gray-500/50',
  }
  return (
    <span className={`inline-block w-2 h-2 rounded-full shadow-lg ${colors[status] || 'bg-gray-400'}`}></span>
  )
}

function Panel({ title, children, className = '' }: { title: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={`relative bg-white/5 border border-cyan-500/20 rounded-lg overflow-hidden ${className}`}>
      <div className="absolute top-0 left-0 w-full h-px bg-gradient-to-r from-transparent via-cyan-400/50 to-transparent"></div>
      <div className="px-4 py-3 border-b border-cyan-500/20">
        <h3 className="text-cyan-300 font-semibold text-sm tracking-wide flex items-center gap-2">
          <span className="w-1 h-4 bg-cyan-400 rounded"></span>
          {title}
        </h3>
      </div>
      <div className="p-4 h-full">
        {children}
      </div>
    </div>
  )
}

function ScrollList({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [isPaused, setIsPaused] = useState(false)

  useEffect(() => {
    const el = scrollRef.current
    if (!el) return

    let animationId: number
    let scrollTop = 0

    const scroll = () => {
      if (!isPaused && el) {
        scrollTop += 0.5
        if (scrollTop >= el.scrollHeight / 2) {
          scrollTop = 0
        }
        el.scrollTop = scrollTop
      }
      animationId = requestAnimationFrame(scroll)
    }

    animationId = requestAnimationFrame(scroll)
    return () => cancelAnimationFrame(animationId)
  }, [isPaused])

  return (
    <div
      ref={scrollRef}
      className={`overflow-hidden ${className}`}
      onMouseEnter={() => setIsPaused(true)}
      onMouseLeave={() => setIsPaused(false)}
    >
      {children}
      {children}
    </div>
  )
}

function TrendChart({ data }: { data: BigScreenData['trend7d'] }) {
  const max = Math.max(...data.map(d => d.success + d.failed), 1)

  return (
    <div className="h-48 flex items-end justify-around gap-2">
      {data.map((d, i) => {
        const successHeight = (d.success / max) * 100
        const failedHeight = (d.failed / max) * 100
        return (
          <div key={i} className="flex flex-col items-center flex-1 h-full justify-end">
            <div className="w-full flex flex-col-reverse items-center" style={{ height: '85%' }}>
              <div
                className="w-4/5 bg-gradient-to-t from-green-600 to-green-400 rounded-t-sm transition-all duration-500 relative group"
                style={{ height: `${successHeight}%` }}
              >
                <div className="absolute -top-6 left-1/2 -translate-x-1/2 text-xs text-green-400 opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap">
                  {d.success}
                </div>
              </div>
              <div
                className="w-4/5 bg-gradient-to-t from-red-600 to-red-400 rounded-t-sm transition-all duration-500 relative group"
                style={{ height: `${failedHeight}%` }}
              >
                <div className="absolute -top-6 left-1/2 -translate-x-1/2 text-xs text-red-400 opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap">
                  {d.failed}
                </div>
              </div>
            </div>
            <div className="text-xs text-gray-500 mt-2">{d.date}</div>
          </div>
        )
      })}
    </div>
  )
}

function HorizontalBar({ name, total, rate, maxTotal }: { name: string; total: number; rate: number; maxTotal: number }) {
  const width = (total / maxTotal) * 100
  const rateColor = rate >= 95 ? 'bg-green-500' : rate >= 85 ? 'bg-yellow-500' : 'bg-red-500'

  return (
    <div className="mb-3">
      <div className="flex justify-between text-sm mb-1">
        <span className="text-gray-300 truncate mr-2">{name}</span>
        <span className="text-gray-400 flex-shrink-0">
          {total} <span className={`ml-1 ${rate >= 95 ? 'text-green-400' : rate >= 85 ? 'text-yellow-400' : 'text-red-400'}`}>({rate}%)</span>
        </span>
      </div>
      <div className="h-4 bg-white/5 rounded overflow-hidden">
        <div
          className={`h-full ${rateColor} transition-all duration-1000 relative`}
          style={{ width: `${width}%` }}
        >
          <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent"></div>
        </div>
      </div>
    </div>
  )
}

function AgentProgressBar({ active, max }: { active: number; max: number }) {
  const percent = max > 0 ? (active / max) * 100 : 0
  const color = percent >= 80 ? 'bg-red-500' : percent >= 50 ? 'bg-yellow-500' : 'bg-green-500'

  return (
    <div className="h-2 bg-white/10 rounded overflow-hidden flex-1 ml-3">
      <div
        className={`h-full ${color} transition-all duration-500`}
        style={{ width: `${percent}%` }}
      ></div>
    </div>
  )
}

export default function BigScreen() {
  const { t, locale } = useI18n()
  const [data, setData] = useState<BigScreenData>(mockData)
  const [lastUpdate, setLastUpdate] = useState<Date>(new Date())

  const fetchData = async () => {
    try {
      const result = await api.getBigScreenData()
      if (result) {
        setData({ ...mockData, ...result })
      }
    } catch (e) {
      // 使用 mock 数据
    } finally {
      setLastUpdate(new Date())
    }
  }

  useEffect(() => {
    fetchData()
    const timer = setInterval(fetchData, 5000)
    return () => clearInterval(timer)
  }, [])

  const formatDuration = (seconds: number) => {
    const m = Math.floor(seconds / 60)
    const s = seconds % 60
    return `${m}m ${s}s`
  }

  const formatTime = (dateStr: string) => {
    const date = new Date(dateStr)
    return date.toLocaleTimeString(locale === 'zh-CN' ? 'zh-CN' : 'en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  const maxProjectTotal = Math.max(...data.projectRanking.map(p => p.total), 1)

  return (
    <div className="min-h-screen w-screen overflow-hidden bg-[#0a1628] text-white p-4">
      <style>{`
        .glow-text {
          text-shadow: 0 0 10px currentColor, 0 0 20px currentColor, 0 0 40px currentColor;
        }
        @keyframes blink {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.5; }
        }
        .blink-new {
          animation: blink 0.5s ease-in-out 3;
        }
        @keyframes scanline {
          0% { transform: translateY(-100%); }
          100% { transform: translateY(100vh); }
        }
        .scanline {
          position: fixed;
          top: 0;
          left: 0;
          right: 0;
          height: 4px;
          background: linear-gradient(to bottom, transparent, rgba(0, 200, 255, 0.1), transparent);
          pointer-events: none;
          animation: scanline 8s linear infinite;
          z-index: 100;
        }
      `}</style>
      <div className="scanline"></div>

      {/* Header */}
      <header className="flex justify-between items-center mb-4 px-2">
        <div className="text-gray-400 text-sm">
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 bg-green-400 rounded-full animate-pulse shadow-lg shadow-green-400/50"></span>
            <span>SYSTEM ONLINE</span>
            <span className="mx-2">|</span>
            <span>{t('bigScreen.lastUpdate')}: {lastUpdate.toLocaleTimeString()}</span>
          </div>
        </div>
        <h1 className="text-2xl md:text-3xl font-bold text-center flex-1">
          <span className="bg-gradient-to-r from-cyan-400 via-blue-400 to-cyan-400 bg-clip-text text-transparent glow-text">
            {t('bigScreen.title')}
          </span>
        </h1>
        <Clock />
      </header>

      {/* Stats Row */}
      <div className="grid grid-cols-2 md:grid-cols-5 gap-4 mb-4">
        <StatCard label={t('bigScreen.totalBuilds')} value={data.totalBuilds.toLocaleString()} color="text-cyan-400" icon="📊" />
        <StatCard label={t('bigScreen.running')} value={data.running} color="text-blue-400" icon="🔄" />
        <StatCard label={t('bigScreen.successToday')} value={data.successToday} color="text-green-400" icon="✅" />
        <StatCard label={t('bigScreen.failedToday')} value={data.failedToday} color="text-red-400" icon="❌" />
        <StatCard label={t('bigScreen.activeAgents')} value={data.activeAgents} color="text-purple-400" icon="🤖" />
      </div>

      {/* Main Grid */}
      <div className="grid grid-cols-12 gap-4" style={{ height: 'calc(100vh - 200px)' }}>
        {/* Recent Builds */}
        <Panel title={t('bigScreen.recentBuilds')} className="col-span-12 md:col-span-5">
          <ScrollList className="h-full">
            {data.recentBuilds.map((build) => (
              <div key={build.id} className="flex items-center justify-between py-2 px-2 hover:bg-white/5 rounded border-b border-white/5">
                <div className="flex items-center gap-3 min-w-0">
                  <StatusDot status={build.status} />
                  <div className="min-w-0">
                    <div className="text-sm font-medium text-gray-200 truncate">
                      {build.project_name} <span className="text-cyan-400">#{build.number}</span>
                    </div>
                    <div className="text-xs text-gray-500">
                      {build.branch} · {formatTime(build.started_at)}
                    </div>
                  </div>
                </div>
                <div className="text-sm text-gray-400 ml-2">
                  {formatDuration(build.duration)}
                </div>
              </div>
            ))}
          </ScrollList>
        </Panel>

        {/* Trend Chart */}
        <Panel title={t('bigScreen.trend7d')} className="col-span-12 md:col-span-4">
          <TrendChart data={data.trend7d} />
          <div className="flex justify-center gap-6 mt-4 text-xs">
            <div className="flex items-center gap-2">
              <span className="w-3 h-3 bg-green-500 rounded"></span>
              <span className="text-gray-400">{t('status.success')}</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="w-3 h-3 bg-red-500 rounded"></span>
              <span className="text-gray-400">{t('status.failed')}</span>
            </div>
          </div>
        </Panel>

        {/* Agent Status */}
        <Panel title={t('bigScreen.agentStatus')} className="col-span-12 md:col-span-3">
          <div className="space-y-3">
            {data.agents.map((agent) => (
              <div key={agent.id} className="flex items-center">
                <StatusDot status={agent.status} />
                <div className="ml-2 flex-1 min-w-0">
                  <div className="text-sm text-gray-300 truncate">{agent.name}</div>
                  <div className="text-xs text-gray-500">
                    {agent.active_builds}/{agent.max_builds}
                  </div>
                </div>
                <AgentProgressBar active={agent.active_builds} max={agent.max_builds} />
              </div>
            ))}
          </div>
        </Panel>

        {/* Project Ranking */}
        <Panel title={t('bigScreen.projectRanking')} className="col-span-12 md:col-span-4">
          <div className="space-y-1">
            {data.projectRanking.map((project) => (
              <HorizontalBar
                key={project.id}
                name={project.name}
                total={project.total}
                rate={project.success_rate}
                maxTotal={maxProjectTotal}
              />
            ))}
          </div>
        </Panel>

        {/* Recent Events */}
        <Panel title={t('bigScreen.recentEvents')} className="col-span-12 md:col-span-5">
          <ScrollList className="h-full">
            {data.recentEvents.map((event) => (
              <div key={event.id} className="flex items-center gap-3 py-2 px-2 hover:bg-white/5 rounded border-b border-white/5">
                <StatusDot status={event.status} />
                <div className="flex-1 min-w-0">
                  <div className="text-sm">
                    <span className="text-purple-400">[{event.channel}]</span>
                    <span className="text-gray-300 ml-2">{event.event}</span>
                  </div>
                </div>
                <div className="text-xs text-gray-500 ml-2">
                  {event.time}
                </div>
              </div>
            ))}
          </ScrollList>
        </Panel>

        {/* System Info */}
        <Panel title={t('bigScreen.systemInfo')} className="col-span-12 md:col-span-3">
          <div className="space-y-4">
            <div className="flex justify-between items-center">
              <span className="text-gray-400 text-sm">Goroutines</span>
              <span className="text-cyan-400 font-mono">{data.systemInfo.goroutines}</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-gray-400 text-sm">Go Version</span>
              <span className="text-green-400 font-mono text-sm">{data.systemInfo.go_version}</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-gray-400 text-sm">OS</span>
              <span className="text-gray-300 font-mono text-sm">{data.systemInfo.os}</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-gray-400 text-sm">CPUs</span>
              <span className="text-yellow-400 font-mono">{data.systemInfo.cpus}</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-gray-400 text-sm">Uptime</span>
              <span className="text-blue-400 font-mono text-sm">{data.systemInfo.uptime}</span>
            </div>
            <div className="pt-4 border-t border-white/10">
              <div className="text-xs text-gray-500 mb-2">{t('bigScreen.queued')}: {data.queued}</div>
            </div>
          </div>
        </Panel>
      </div>
    </div>
  )
}
