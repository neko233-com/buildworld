import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { MotionConfig } from 'motion/react'
import { BrowserRouter, Navigate, NavLink, Outlet, Routes, Route, useLocation, useNavigate } from 'react-router-dom'
import { localeLabels, type Locale, useI18n } from './i18n'
import { API_STATUS_EVENT, api, clearToken } from './api'
import { currentRole, isAdmin, type UserRole } from './authz'
import { AppDialogs } from './components/AppDialogs'
import { BuildWorldMark } from './components/BuildWorldMark'
import { CommandPalette, type CommandPaletteGroup } from './components/CommandPalette'
import { RouteErrorBoundary } from './components/RouteErrorBoundary'
import InAppNotifications from './components/InAppNotifications'
import { PageState } from './components/PageState'
import { Activity, Bell, Boxes, ChevronDown, CircleUserRound, ClipboardList, CloudOff, Cog, FileClock, Gauge, GitBranch, KeyRound, LayoutDashboard, LogOut, Network, Search, Settings2, ShieldCheck, SlidersHorizontal, TerminalSquare, UsersRound } from 'lucide-react'
import { buildStatusLabel } from './lib/buildPresentation'
import './jenkins-shell.css'

const Dashboard = lazy(() => import('./pages/Dashboard'))
const Projects = lazy(() => import('./pages/Projects'))
const ProjectDetail = lazy(() => import('./pages/ProjectDetail'))
const ProjectConfigure = lazy(() => import('./pages/ProjectConfigure'))
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
const Notifications = lazy(() => import('./pages/Notifications'))
const Statistics = lazy(() => import('./pages/Statistics'))
const AuditLog = lazy(() => import('./pages/AuditLog'))
const APITokens = lazy(() => import('./pages/APITokens'))
const BuildQueue = lazy(() => import('./pages/BuildQueue'))
const TestReports = lazy(() => import('./pages/TestReports'))
const Login = lazy(() => import('./pages/Login'))
const BigScreen = lazy(() => import('./pages/BigScreen'))
const MyDashboard = lazy(() => import('./pages/MyDashboard'))

export const WORKSPACE_NAV_ITEMS = [
  { href: '/', labelKey: 'nav.dashboard', icon: LayoutDashboard },
  { href: '/projects', labelKey: 'nav.projects', icon: Boxes },
  { href: '/build-queue', labelKey: 'nav.buildQueue', icon: FileClock },
  { href: '/builds', labelKey: 'nav.builds', icon: Activity },
  { href: '/vcs-roots', labelKey: 'nav.vcsRoots', icon: GitBranch },
] as const

function Layout() {
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

  useEffect(() => {
    if (!accountOpen) return
    const onPointerDown = (event: PointerEvent) => {
      if (!accountRef.current?.contains(event.target as Node)) setAccountOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      setAccountOpen(false)
      accountButtonRef.current?.focus()
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [accountOpen])

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
    setAccountOpen(false)
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
    { label: t('nav.vcsRoots'), detail: t('shell.navigationDetail'), href: '/vcs-roots', icon: GitBranch },
    { label: t('dashboard.workers'), detail: t('shell.workersDetail'), href: '/agents', icon: Network },
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
        <NavLink to="/" end className="app-brand" aria-label="BuildWorld">
          <BuildWorldMark size={28} />
          <span>BuildWorld</span>
        </NavLink>
        <div className="topbar-actions">
          <button className="topbar-icon-button" onClick={openPalette} aria-label={t('shell.searchPlaceholder')} title={t('shell.searchPlaceholder')}><Search size={18} /></button>
          <InAppNotifications />
          <NavLink to="/my-dashboard" className={({ isActive }) => `topbar-dashboard-link ${isActive ? 'active' : ''}`}><Gauge size={17} /><span>{t('nav.myDashboard')}</span></NavLink>
          {admin && <NavLink to="/settings" className={({ isActive }) => `topbar-settings-link ${isActive ? 'active' : ''}`}><Cog size={18} /><span>{t('nav.settings')}</span></NavLink>}
          <div className="account-menu" ref={accountRef}>
            <button
              ref={accountButtonRef}
              type="button"
              className="account-menu-trigger"
              aria-label={t('shell.my')}
              aria-expanded={accountOpen}
              aria-controls="account-menu-popover"
              onClick={() => setAccountOpen(value => !value)}
            >
              <CircleUserRound size={19} />
              <ChevronDown size={13} />
            </button>
            {accountOpen && <div id="account-menu-popover" className="account-menu-popover">
              <div className="account-menu-summary"><CircleUserRound size={20} /><span><strong>{t('shell.my')}</strong><small>{t(`users.role_${role}`)}</small></span></div>
              <label className="account-language"><span>{t('shell.language')}</span><select value={locale} onChange={(event) => changeLocale(event.target.value as Locale)} aria-label={t('shell.language')}>{locales.map((item) => <option key={item} value={item}>{localeLabels[item]}</option>)}</select></label>
              <button type="button" className="account-logout" onClick={handleLogout}><LogOut size={16} /><span>{t('shell.logout')}</span></button>
            </div>}
          </div>
        </div>
      </header>
      <section className="app-frame">
        {apiUnavailable && <div className="service-status-banner" role="alert"><CloudOff size={15} /><span><strong>{t('shell.serviceUnavailable')}</strong>{t('shell.serviceUnavailableHint')}</span></div>}
        <main className="app-main"><RouteErrorBoundary resetKey={location.pathname}><Suspense fallback={<RouteLoading />}><Outlet /></Suspense></RouteErrorBoundary></main>
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
            <Route path="/projects/:id" element={<ProjectDetail />} />
            <Route path="/builds" element={<Builds />} />
            <Route path="/builds/:id" element={<BuildDetail />} />
            <Route path="/builds/:id/tests" element={<TestReports />} />
            <Route path="/build-queue" element={<BuildQueue />} />
            <Route path="/agents" element={<Agents />} />
            <Route path="/vcs-roots" element={<VCSRoots />} />
            <Route path="/templates" element={<Navigate to="/projects" replace />} />
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
