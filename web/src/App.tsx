import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, ReactNode, RefObject } from 'react'
import { MotionConfig } from 'motion/react'
import { BrowserRouter, Navigate, NavLink, Outlet, Routes, Route, useLocation, useNavigate } from 'react-router-dom'
import { localeLabels, type Locale, useI18n } from './i18n'
import { API_STATUS_EVENT, api, clearToken, type SystemUpdateCheck, type SystemUpdateStatus } from './api'
import { currentRole, isAdmin, type UserRole } from './authz'
import { AppDialogs, dialogs } from './components/AppDialogs'
import { BuildWorldMark } from './components/BuildWorldMark'
import { CommandPalette, type CommandPaletteGroup } from './components/CommandPalette'
import { RouteErrorBoundary } from './components/RouteErrorBoundary'
import InAppNotifications from './components/InAppNotifications'
import { PageState } from './components/PageState'
import { DISTRIBUTED_WORKERS_ENABLED } from './featureFlags'
import { Activity, Bell, BookTemplate, Boxes, CircleUserRound, ClipboardList, CloudOff, Cog, Download, FileClock, Gauge, GitBranch, KeyRound, LayoutDashboard, LoaderCircle, LogOut, Network, RefreshCw, Search, Settings2, ShieldCheck, SlidersHorizontal, TerminalSquare, UsersRound, X } from 'lucide-react'
import { buildStatusLabel } from './lib/buildPresentation'
import './jenkins-shell.css'

const Dashboard = lazy(() => import('./pages/Dashboard'))
const Projects = lazy(() => import('./pages/Projects'))
const ProjectDetail = lazy(() => import('./pages/ProjectDetail'))
const ProjectConfigure = lazy(() => import('./pages/ProjectConfigure'))
const ProjectChanges = lazy(() => import('./pages/ProjectChanges'))
const ProjectBuildWithParameters = lazy(() => import('./pages/ProjectBuildWithParameters'))
const CreateProject = lazy(() => import('./pages/CreateProject'))
const Builds = lazy(() => import('./pages/Builds'))
const BuildDetail = lazy(() => import('./pages/BuildDetail'))
const BuildLogViewer = lazy(() => import('./pages/BuildLogViewer'))
const Agents = lazy(() => import('./pages/Agents'))
const Plugins = lazy(() => import('./pages/Plugins'))
const Settings = lazy(() => import('./pages/Settings'))
const Users = lazy(() => import('./pages/Users'))
const Credentials = lazy(() => import('./pages/Credentials'))
const VCSRoots = lazy(() => import('./pages/VCSRoots'))
const Templates = lazy(() => import('./pages/Templates'))
const Notifications = lazy(() => import('./pages/Notifications'))
const Statistics = lazy(() => import('./pages/Statistics'))
const AuditLog = lazy(() => import('./pages/AuditLog'))
const APITokens = lazy(() => import('./pages/APITokens'))
const BuildQueue = lazy(() => import('./pages/BuildQueue'))
const TestReports = lazy(() => import('./pages/TestReports'))
const Login = lazy(() => import('./pages/Login'))
const BigScreen = lazy(() => import('./pages/BigScreen'))
const MyDashboard = lazy(() => import('./pages/MyDashboard'))

const ACCOUNT_MENU_FOCUSABLE = [
  'select:not(:disabled)',
  'a[href]',
  'button:not(:disabled)',
  'input:not(:disabled):not([type="hidden"])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

type RouteFocusResult = 'heading' | 'fallback' | 'pending' | 'blocked'

export function focusRouteContent(container: HTMLElement, allowFallback = true): RouteFocusResult {
  if (document.querySelector('[role="dialog"][aria-modal="true"]')) return 'blocked'

  const heading = container.querySelector<HTMLElement>('h1')
  if (heading) {
    if (!heading.hasAttribute('tabindex')) heading.setAttribute('tabindex', '-1')
    heading.style.outline = 'none'
    heading.focus()
    return 'heading'
  }

  if (container.querySelector('.route-loading') || !allowFallback) return 'pending'
  container.focus()
  return 'fallback'
}

function useRouteFocus(mainRef: RefObject<HTMLElement | null>, routeKey: string) {
  useEffect(() => {
    const container = mainRef.current
    if (!container) return

    let observer: MutationObserver | null = null
    let fallbackFocused = false
    const focusWhenReady = () => {
      if (fallbackFocused && document.activeElement !== container && document.activeElement !== document.body) {
        observer?.disconnect()
        return
      }
      const result = focusRouteContent(container, !fallbackFocused)
      if (result === 'fallback') fallbackFocused = true
      if (result === 'heading' || result === 'blocked') observer?.disconnect()
    }

    observer = new MutationObserver(focusWhenReady)
    observer.observe(container, { childList: true, subtree: true })
    const frame = window.requestAnimationFrame(focusWhenReady)

    return () => {
      window.cancelAnimationFrame(frame)
      observer?.disconnect()
    }
  }, [mainRef, routeKey])
}

function accountMenuItems(popover: HTMLElement | null): HTMLElement[] {
  return Array.from(popover?.querySelectorAll<HTMLElement>(ACCOUNT_MENU_FOCUSABLE) || [])
    .filter(element => element.getAttribute('aria-hidden') !== 'true')
}

function updateStatusText(status: SystemUpdateStatus | null, t: (key: string) => string): string {
  if (!status) return ''
  if (status.status === 'succeeded') return t('updates.completed')
  if (status.status === 'rolled_back') return t('updates.rolledBack')
  if (status.status === 'failed') return t('updates.failed')
  if (status.status === 'applying') return t('updates.applying')
  return t('updates.queued')
}

function displayVersion(value: string): string {
  const normalized = value.trim()
  if (!normalized) return ''
  return normalized.toLowerCase().startsWith('v') ? normalized : `v${normalized}`
}

function SystemUpdateAction() {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [check, setCheck] = useState<SystemUpdateCheck | null>(null)
  const [operation, setOperation] = useState<SystemUpdateStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!operation?.operation_id || !['accepted', 'applying'].includes(operation.status)) return
    let active = true
    const timer = window.setInterval(() => {
      api.getSystemUpdate()
        .then(response => { if (active) setOperation(response.operation) })
        .catch(() => undefined)
    }, 1500)
    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [operation?.operation_id, operation?.status])

  const operationInProgress = ['accepted', 'applying'].includes(operation?.status || '')

  const checkForUpdate = async () => {
    setBusy(true)
    setError('')
    try {
      setCheck(await api.checkSystemUpdate())
    } catch (reason: any) {
      setError(reason.message || t('updates.checkFailed'))
    } finally {
      setBusy(false)
    }
  }

  const updateNow = async () => {
    if (!check?.update_available) return
    const confirmed = await dialogs.confirm(
      t('updates.confirm').replace('{version}', displayVersion(check.latest_version)),
      { title: t('updates.title'), action: t('updates.updateAction') },
    )
    if (!confirmed) return
    setBusy(true)
    setError('')
    try {
      setOperation(await api.applyLatestSystemUpdate())
    } catch (reason: any) {
      if (reason?.code === 'no_update_available') {
        setCheck(current => current ? { ...current, latest_version: current.current_version, update_available: false } : current)
      } else {
        setError(reason.message || t('updates.updateFailed'))
      }
    } finally {
      setBusy(false)
    }
  }

  const updateLabel = check?.update_available
    ? t('updates.update').replace('{version}', displayVersion(check.latest_version))
    : t('updates.check')
  const handleTriggerClick = () => {
    setOpen(true)
    if (check?.update_available) {
      if (!busy && !operationInProgress) void updateNow()
      return
    }
    if (!check && !busy) void checkForUpdate()
  }

  const operationText = updateStatusText(operation, t)
  return <div className="system-update-action">
    <button
      type="button"
      className={`topbar-icon-button system-update-trigger${check?.update_available ? ' available' : ''}`}
      aria-label={updateLabel}
      title={updateLabel}
      aria-expanded={open}
      onClick={handleTriggerClick}
    >
      {busy ? <LoaderCircle className="system-update-spinner" size={20} /> : <RefreshCw size={20} />}
      {check?.update_available && <span>{updateLabel}</span>}
    </button>
    {open && <div className="system-update-popover" role="dialog" aria-label={t('updates.title')}>
      <header><div><Download size={16} /><span><strong>{t('updates.title')}</strong><small>{t('updates.adminOnly')}</small></span></div><button type="button" aria-label={t('common.close')} onClick={() => setOpen(false)}><X size={15} /></button></header>
      <div className="system-update-popover-body">
        {!check && !error && <p>{busy ? t('updates.checking') : t('updates.checkHint')}</p>}
        {check && <p className={check.update_available ? 'available' : ''}>{check.update_available ? t('updates.available').replace('{version}', displayVersion(check.latest_version)) : t('updates.latest').replace('{version}', displayVersion(check.current_version))}</p>}
        {operationText && <p className="system-update-operation" role="status">{operationText}{operation?.version ? ` · ${displayVersion(operation.version)}` : ''}</p>}
        {error && <p className="system-update-error" role="alert">{error}</p>}
        <div className="system-update-popover-actions">
          <button type="button" className="secondary-command" disabled={busy} onClick={() => void checkForUpdate()}><RefreshCw size={13} />{busy ? t('updates.checking') : t('updates.check')}</button>
          {check?.update_available && <button type="button" className="primary-command" disabled={busy || operationInProgress} onClick={() => void updateNow()}><Download size={13} />{busy || operationInProgress ? t('updates.updating') : t('updates.update').replace('{version}', displayVersion(check.latest_version))}</button>}
        </div>
      </div>
    </div>}
  </div>
}

export function Layout() {
  const { t, locale, changeLocale, locales } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [projects, setProjects] = useState<any[]>([])
  const [builds, setBuilds] = useState<any[]>([])
  const [searchError, setSearchError] = useState('')
  const [searchLoading, setSearchLoading] = useState(false)
  const [apiUnavailable, setApiUnavailable] = useState(false)
  const [accountOpen, setAccountOpen] = useState(false)
  const paletteOriginRef = useRef<HTMLElement | null>(null)
  const accountRef = useRef<HTMLDivElement | null>(null)
  const accountButtonRef = useRef<HTMLButtonElement | null>(null)
  const accountPopoverRef = useRef<HTMLDivElement | null>(null)
  const mainRef = useRef<HTMLElement | null>(null)

  useRouteFocus(mainRef, `${location.pathname}${location.search}`)

  const openPalette = useCallback(() => {
    if (paletteOpen) return
    paletteOriginRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    setPaletteOpen(true)
  }, [paletteOpen])
  const closePalette = useCallback(() => {
    const origin = paletteOriginRef.current
    setPaletteOpen(false)
    setQuery('')
    window.requestAnimationFrame(() => {
      if (origin?.isConnected) origin.focus()
    })
  }, [])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        openPalette()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [openPalette])

  useEffect(() => {
    const onAPIStatus = (event: Event) => {
      setApiUnavailable((event as CustomEvent<{ status: 'available' | 'unavailable' }>).detail.status === 'unavailable')
    }
    window.addEventListener(API_STATUS_EVENT, onAPIStatus)
    return () => window.removeEventListener(API_STATUS_EVENT, onAPIStatus)
  }, [])

  const closeAccount = useCallback((restoreFocus = true) => {
    setAccountOpen(false)
    if (!restoreFocus) return
    window.requestAnimationFrame(() => {
      if (accountButtonRef.current?.isConnected) accountButtonRef.current.focus()
    })
  }, [])

  useEffect(() => {
    if (!accountOpen) return
    const frame = window.requestAnimationFrame(() => accountMenuItems(accountPopoverRef.current)[0]?.focus())
    const onPointerDown = (event: PointerEvent) => {
      if (!accountRef.current?.contains(event.target as Node)) closeAccount()
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      closeAccount()
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      window.cancelAnimationFrame(frame)
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [accountOpen, closeAccount])

  const handleAccountMenuKeyDown = useCallback((event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      closeAccount()
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return

    const items = accountMenuItems(accountPopoverRef.current)
    if (!items.length) return
    event.preventDefault()
    event.stopPropagation()
    const current = items.indexOf(document.activeElement as HTMLElement)
    let next = current
    if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = items.length - 1
    else if (event.key === 'ArrowDown') next = current < 0 ? 0 : (current + 1) % items.length
    else next = current <= 0 ? items.length - 1 : current - 1
    items[next]?.focus()
  }, [closeAccount])

  useEffect(() => {
    if (!paletteOpen) return
    let active = true
    setSearchError('')
    setSearchLoading(true)
    Promise.all([api.listProjects(), api.listBuilds(30)])
      .then(([nextProjects, nextBuilds]) => {
        if (!active) return
        setProjects(nextProjects)
        setBuilds(nextBuilds)
      })
      .catch((error: Error) => { if (active) setSearchError(error.message || 'Unable to load search results') })
      .finally(() => { if (active) setSearchLoading(false) })
    return () => { active = false }
  }, [paletteOpen])
  const handleLogout = () => {
    closeAccount()
    clearToken()
    navigate('/login')
  }

  const token = localStorage.getItem('token')
  if (!token) {
    return <Navigate to="/login" replace />
  }
  const role = currentRole()
  const admin = isAdmin(role)

  const normalizedQuery = query.trim().toLowerCase()
  const matches = (value: string) => !normalizedQuery || value.toLowerCase().includes(normalizedQuery)
  const projectResults = projects.filter((project) => matches(project.name)).slice(0, 6)
  const projectNames = new Map(projects.map(project => [project.id, project.name]))
  const buildResults = builds.filter((build) => matches(`${projectNames.get(build.project_id) || ''} #${build.number} ${build.status} ${build.branch || ''}`)).slice(0, 6)
  const pageCommands = [
    { label: t('nav.dashboard'), detail: t('shell.navigationDetail'), href: '/', icon: LayoutDashboard },
    { label: t('shell.projects'), detail: t('shell.projectsDetail'), href: '/projects', icon: Boxes },
    { label: t('nav.buildQueue'), detail: t('shell.buildQueueDetail'), href: '/build-queue', icon: FileClock },
    { label: t('nav.builds'), detail: t('shell.navigationDetail'), href: '/builds', icon: Activity },
    { label: t('nav.templates'), detail: t('shell.navigationDetail'), href: '/templates', icon: BookTemplate },
    { label: t('nav.vcsRoots'), detail: t('shell.navigationDetail'), href: '/vcs-roots', icon: GitBranch },
    ...(DISTRIBUTED_WORKERS_ENABLED ? [{ label: t('dashboard.workers'), detail: t('shell.workersDetail'), href: '/agents', icon: Network }] : []),
    { label: t('nav.plugins'), detail: t('shell.pluginsDetail'), href: '/plugins', icon: TerminalSquare },
    { label: t('nav.apiTokens'), detail: t('shell.navigationDetail'), href: '/api-tokens', icon: KeyRound },
    { label: t('nav.statistics'), detail: t('shell.navigationDetail'), href: '/statistics', icon: SlidersHorizontal },
    { label: t('nav.bigScreen'), detail: t('shell.navigationDetail'), href: '/bigscreen', icon: ShieldCheck },
    { label: t('nav.myDashboard'), detail: t('shell.myDashboardDetail'), href: '/my-dashboard', icon: Gauge },
    ...(admin ? [
      { label: t('settings.notifications'), detail: t('shell.notificationsDetail'), href: '/notifications', icon: Bell },
      { label: t('nav.credentials'), detail: t('shell.navigationDetail'), href: '/credentials', icon: KeyRound },
      { label: t('nav.users'), detail: t('shell.navigationDetail'), href: '/users', icon: UsersRound },
      { label: t('nav.auditLog'), detail: t('shell.navigationDetail'), href: '/audit-log', icon: ClipboardList },
      { label: t('nav.settings'), detail: t('shell.navigationDetail'), href: '/settings', icon: Settings2 },
    ] : []),
  ].filter((command) => matches(`${command.label} ${command.detail}`))
  const commandGroups: CommandPaletteGroup[] = [
    {
      id: 'commands',
      label: t('shell.commands'),
      items: pageCommands.map(command => ({ ...command, id: `command-${command.href}` })),
    },
    {
      id: 'projects',
      label: t('shell.projects'),
      items: projectResults.map(project => ({
        id: `project-${project.id}`,
        label: project.name,
        detail: project.default_branch || 'main',
        href: `/projects/${project.id}`,
        icon: Boxes,
      })),
    },
    {
      id: 'builds',
      label: t('shell.builds'),
      items: buildResults.map(build => ({
        id: `build-${build.id}`,
        label: `${projectNames.get(build.project_id) || `#${build.project_id}`} · #${build.number}`,
        detail: `${buildStatusLabel(t, build.status)}${build.branch ? ` · ${build.branch}` : ''}`,
        href: `/builds/${build.id}`,
        icon: Activity,
      })),
    },
  ]
  const navigateFromPalette = (href: string) => {
    closePalette()
    navigate(href)
  }

  return (
    <div className="app-shell">
      <header className="app-topbar">
        <div className="app-topbar-main">
          <NavLink to="/" end className="app-brand" aria-label="BuildWorld">
            <BuildWorldMark size={36} />
            <span>BuildWorld</span>
          </NavLink>
          {admin && <SystemUpdateAction />}
          <div className="app-topbar-breadcrumbs" id="jenkins-header-breadcrumbs" />
        </div>
        <div className="topbar-actions">
          <button className="topbar-icon-button" onClick={openPalette} aria-label={t('shell.searchPlaceholder')} title={t('shell.searchPlaceholder')}><Search size={20} /></button>
          {admin && <NavLink to="/settings" aria-label={t('nav.settings')} title={t('nav.settings')} className={({ isActive }) => `topbar-settings-link ${isActive ? 'active' : ''}`}><Cog size={20} /><span className="jenkins-visually-hidden">{t('nav.settings')}</span></NavLink>}
          <div className="account-menu" ref={accountRef}>
            <button
              ref={accountButtonRef}
              type="button"
              className="account-menu-trigger"
              aria-label={t('shell.my')}
              aria-expanded={accountOpen}
              aria-controls="account-menu-popover"
              onClick={() => accountOpen ? closeAccount() : setAccountOpen(true)}
            >
              <CircleUserRound size={20} />
            </button>
            <div ref={accountPopoverRef} id="account-menu-popover" className="account-menu-popover" hidden={!accountOpen} aria-hidden={!accountOpen} onKeyDown={handleAccountMenuKeyDown}>
              <div className="account-menu-summary"><CircleUserRound size={20} /><span><strong>{t('shell.my')}</strong><small>{t(`users.role_${role}`)}</small></span></div>
              <label className="account-language"><span>{t('shell.language')}</span><select value={locale} onChange={(event) => changeLocale(event.target.value as Locale)} aria-label={t('shell.language')}>{locales.map((item) => <option key={item} value={item}>{localeLabels[item]}</option>)}</select></label>
              <NavLink to="/my-dashboard" className="account-dashboard-link" onClick={() => closeAccount()}><Gauge size={16} /><span>{t('nav.myDashboard')}</span></NavLink>
              <NavLink to="/bigscreen" className="account-dashboard-link" onClick={() => closeAccount()}><LayoutDashboard size={16} /><span>{t('bigScreen.openWall')}</span></NavLink>
              <InAppNotifications menu menuOpen={accountOpen} />
              <button type="button" className="account-logout" onClick={handleLogout}><LogOut size={16} /><span>{t('shell.logout')}</span></button>
            </div>
          </div>
        </div>
      </header>
      <section className="app-frame">
        {apiUnavailable && <div className="service-status-banner" role="alert"><CloudOff size={15} /><span><strong>{t('shell.serviceUnavailable')}</strong>{t('shell.serviceUnavailableHint')}</span></div>}
        <main ref={mainRef} className="app-main" tabIndex={-1} style={{ outline: 'none' }}><RouteErrorBoundary resetKey={location.pathname}><Suspense fallback={<RouteLoading />}><Outlet /></Suspense></RouteErrorBoundary></main>
      </section>
      {paletteOpen && <CommandPalette ariaLabel={t('shell.globalSearch')} placeholder={t('shell.searchPlaceholder')} query={query} groups={commandGroups} loading={searchLoading} loadingLabel={t('shell.searchLoading')} emptyLabel={t('shell.noResults')} keyboardHint={t('shell.searchKeyboardHint')} error={searchError} onQueryChange={setQuery} onNavigate={navigateFromPalette} onClose={closePalette} />}
    </div>
  )
}

export function AppMotionBoundary({ children }: { children: ReactNode }) {
  return <MotionConfig reducedMotion="user">{children}</MotionConfig>
}

function App() {
  return (
    <AppMotionBoundary>
      <BrowserRouter>
        <AppDialogs />
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/builds/:id/logs" element={<Suspense fallback={<RouteLoading />}><BuildLogViewer /></Suspense>} />
          <Route element={<Layout />}>
            <Route path="/" element={<Dashboard />} />
            <Route path="/projects" element={<Projects />} />
            <Route path="/projects/new" element={<RoleGate roles={['admin', 'developer']}><CreateProject /></RoleGate>} />
            <Route path="/projects/:id/configure" element={<RoleGate roles={['admin', 'developer']}><ProjectConfigure /></RoleGate>} />
            <Route path="/projects/:id/changes" element={<ProjectChanges />} />
            <Route path="/projects/:id/build" element={<RoleGate roles={['admin', 'developer']}><ProjectBuildWithParameters /></RoleGate>} />
            <Route path="/projects/:id" element={<ProjectDetail />} />
            <Route path="/builds" element={<Builds />} />
            <Route path="/builds/:id" element={<BuildDetail />} />
            <Route path="/builds/:id/tests" element={<TestReports />} />
            <Route path="/build-queue" element={<BuildQueue />} />
            <Route path="/agents" element={<Agents />} />
            <Route path="/vcs-roots" element={<VCSRoots />} />
            <Route path="/templates" element={<Templates />} />
            <Route path="/plugins" element={<Plugins />} />
            <Route path="/credentials" element={<RoleGate roles={['admin']}><Credentials /></RoleGate>} />
            <Route path="/notifications" element={<RoleGate roles={['admin']}><Notifications /></RoleGate>} />
            <Route path="/statistics" element={<Statistics />} />
            <Route path="/bigscreen" element={<BigScreen />} />
            <Route path="/my-dashboard" element={<MyDashboard />} />
            <Route path="/audit-log" element={<RoleGate roles={['admin']}><AuditLog /></RoleGate>} />
            <Route path="/api-tokens" element={<APITokens />} />
            <Route path="/users" element={<RoleGate roles={['admin']}><Users /></RoleGate>} />
            <Route path="/settings" element={<RoleGate roles={['admin']}><Settings /></RoleGate>} />
            <Route path="*" element={<NotFound />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AppMotionBoundary>
  )
}

function RoleGate({ roles, children }: { roles: UserRole[]; children: ReactNode }) {
  return roles.includes(currentRole()) ? children : <Navigate to="/" replace />
}

function NotFound() {
  const { t } = useI18n()
  return <section className="route-error">
    <strong>404</strong>
    <div>
      <h1>{t('common.pageNotFound')}</h1>
      <p>{t('common.pageNotFoundHelp')}</p>
      <NavLink to="/"><LayoutDashboard size={15} />{t('common.backToDashboard')}</NavLink>
    </div>
  </section>
}

function RouteLoading() {
  return <div className="route-loading"><PageState /></div>
}

export default App
