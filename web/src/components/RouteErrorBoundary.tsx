import { Component, type ErrorInfo, type ReactNode } from 'react'
import { AlertTriangle, House, RotateCcw } from 'lucide-react'

type Props = {
  children: ReactNode
  resetKey: string
  reloadPage?: () => void
}

type State = {
  error: Error | null
  chunkLoadError: boolean
}

const CHUNK_RECOVERY_PREFIX = 'buildworld:chunk-recovery:'
const CHUNK_ERROR_PATTERNS = [
  /failed to fetch dynamically imported module/i,
  /importing a module script failed/i,
  /loading (?:css )?chunk .+ failed/i,
  /chunkloaderror/i,
  /dynamically imported module/i,
]

export function isChunkLoadError(error: Error): boolean {
  const message = `${error.name} ${error.message}`
  return CHUNK_ERROR_PATTERNS.some(pattern => pattern.test(message))
}

export function chunkRecoveryKey(error: Error): string {
  const source = `${error.name}:${error.message}`
  let hash = 2166136261
  for (let index = 0; index < source.length; index++) {
    hash ^= source.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return `${CHUNK_RECOVERY_PREFIX}${(hash >>> 0).toString(16)}`
}

export class RouteErrorBoundary extends Component<Props, State> {
  state: State = { error: null, chunkLoadError: false }

  static getDerivedStateFromError(error: Error): State {
    return { error, chunkLoadError: isChunkLoadError(error) }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Route render failed', error, info.componentStack)
    if (!isChunkLoadError(error) || typeof window === 'undefined') return

    const recoveryKey = chunkRecoveryKey(error)
    try {
      if (window.sessionStorage.getItem(recoveryKey)) return
      window.sessionStorage.setItem(recoveryKey, new Date().toISOString())
    } catch {
      // Storage can be unavailable in hardened browser profiles. Reloading is
      // still preferable to leaving a stale deployment on a broken route.
    }
    this.reloadPage()
  }

  componentDidUpdate(previous: Props) {
    if (this.state.error && previous.resetKey !== this.props.resetKey) {
      this.setState({ error: null, chunkLoadError: false })
    }
  }

  private reloadPage = () => {
    if (this.props.reloadPage) this.props.reloadPage()
    else window.location.reload()
  }

  render() {
    if (!this.state.error) return this.props.children

    return <section className="route-error" role="alert">
      <AlertTriangle size={24} />
      <div>
        <h1>{this.state.chunkLoadError ? '应用已更新' : '页面加载失败'}</h1>
        <p>{this.state.chunkLoadError ? '检测到服务器已发布新版本，请加载最新页面资源后继续。' : (this.state.error.message || '页面遇到了意外错误，请重试。')}</p>
        <div>
          <button onClick={this.state.chunkLoadError ? this.reloadPage : () => this.setState({ error: null, chunkLoadError: false })}><RotateCcw size={15} />{this.state.chunkLoadError ? '加载最新版本' : '重试'}</button>
          <a href="/"><House size={15} />返回仪表盘</a>
        </div>
      </div>
    </section>
  }
}
