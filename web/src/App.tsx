import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { BrowserRouter, Navigate, NavLink, Outlet, Routes, Route, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { localeLabels, type Locale, useI18n } from './i18n'
import { API_STATUS_EVENT, api, clearToken } from './api'
import { currentRole, isAdmin, type UserRole } from './authz'
import { AppDialogs } from './components/AppDialogs'
import { BuildWorldMark } from './components/BuildWorldMark'
import { CommandPalette, type CommandPaletteGroup } from './components/CommandPalette'
import { RouteErrorBoundary } from './components/RouteErrorBoundary'
import InAppNotifications from './components/InAppNotifications'
import { PageState } from './components/PageState'
import { Activity, Bell, BookTemplate, Boxes, CircleUserRound, ClipboardList, CloudOff, FileClock, GitBranch, KeyRound, LayoutDashboard, Network, PackageOpen, Search, Settings2, ShieldCheck, SlidersHorizontal, TerminalSquare, UsersRound } from 'lucide-react'
import { buildStatusLabel } from './lib/buildPresentation'

const Dashboard = lazy(() => import('./pages/Dashboard'))
const Projects = lazy(() => import('./pages/Projects'))
const ProjectDetail = lazy(() => import('./pages/ProjectDetail'))
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
const Deployments = lazy(() => import('./pages/Deployments'))
const BuildQueue = lazy(() => import('./pages/BuildQueue'))
const TestReports = lazy(() => import('./pages/TestReports'))
const Login = lazy(() => import('./pages/Login'))
const BigScreen = lazy(() => import('./pages/BigScreen'))

function NavItem({ href, label, icon: Icon }: { href: string; label: string; icon: typeof LayoutDashboard }) {
  return <NavLink to={href} end={href === '/'} className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`} aria-label={label} title={label}><Icon size={16} /> <span>{label}</span></NavLink>
}

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
  const paletteOriginRef = useRef<HTMLElement | null>(null)

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
    { label: t('nav.builds'), detail: t('shell.navigationDetail'), href: '/builds', icon: Activity },
    { label: t('nav.buildQueue'), detail: t('shell.buildQueueDetail'), href: '/build-queue', icon: FileClock },
    { label: t('dashboard.workers'), detail: t('shell.workersDetail'), href: '/agents', icon: Network },
    { label: t('nav.vcsRoots'), detail: t('shell.navigationDetail'), href: '/vcs-roots', icon: GitBranch },
    { label: t('nav.templates'), detail: t('shell.navigationDetail'), href: '/templates', icon: BookTemplate },
    { label: t('nav.deployments'), detail: t('shell.navigationDetail'), href: '/deployments', icon: PackageOpen },
    { label: t('nav.plugins'), detail: t('shell.pluginsDetail'), href: '/plugins', icon: TerminalSquare },
    { label: t('nav.apiTokens'), detail: t('shell.navigationDetail'), href: '/api-tokens', icon: KeyRound },
    { label: t('nav.statistics'), detail: t('shell.navigationDetail'), href: '/statistics', icon: SlidersHorizontal },
    { label: t('nav.bigScreen'), detail: t('shell.navigationDetail'), href: '/bigscreen', icon: ShieldCheck },
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
      <aside className="app-sidebar">
        <NavLink to="/" end className="app-brand"><span className="brand-mark"><BuildWorldMark size={22} /></span><span>buildworld</span></NavLink>
        <div className="sidebar-section"><p>{t('shell.workspace')}</p><NavItem href="/" label={t('nav.dashboard')} icon={LayoutDashboard} /><NavItem href="/projects" label={t('nav.projects')} icon={Boxes} /><NavItem href="/builds" label={t('nav.builds')} icon={Activity} /><NavItem href="/build-queue" label={t('nav.buildQueue')} icon={FileClock} /></div>
        <div className="sidebar-section"><p>{t('shell.execution')}</p><NavItem href="/agents" label={t('nav.agents')} icon={Network} /><NavItem href="/vcs-roots" label={t('nav.vcsRoots')} icon={GitBranch} /><NavItem href="/templates" label={t('nav.templates')} icon={BookTemplate} /><NavItem href="/deployments" label={t('nav.deployments')} icon={PackageOpen} />{admin && <NavItem href="/notifications" label={t('settings.notifications')} icon={Bell} />}</div>
        <div className="sidebar-section"><p>{t('shell.administration')}</p>{admin && <NavItem href="/credentials" label={t('nav.credentials')} icon={KeyRound} />}<NavItem href="/api-tokens" label={t('nav.apiTokens')} icon={KeyRound} /><NavItem href="/plugins" label={t('nav.plugins')} icon={TerminalSquare} />{admin && <><NavItem href="/users" label={t('nav.users')} icon={UsersRound} /><NavItem href="/audit-log" label={t('nav.auditLog')} icon={ClipboardList} /><NavItem href="/settings" label={t('nav.settings')} icon={Settings2} /></>}</div>
        <div className="sidebar-bottom"><NavLink to="/statistics" className="sidebar-link" aria-label={t('nav.statistics')} title={t('nav.statistics')}><SlidersHorizontal size={16} /><span>{t('nav.statistics')}</span></NavLink><NavLink to="/bigscreen" className="sidebar-link" aria-label={t('nav.bigScreen')} title={t('nav.bigScreen')}><ShieldCheck size={16} /><span>{t('nav.bigScreen')}</span></NavLink></div>
      </aside>
      <section className="app-frame">
        {apiUnavailable && <div className="service-status-banner" role="alert"><CloudOff size={15} /><span><strong>{t('shell.serviceUnavailable')}</strong>{t('shell.serviceUnavailableHint')}</span></div>}
        <header className="app-topbar">
          <button className="command-trigger" onClick={openPalette} aria-label={t('shell.searchPlaceholder')} title={t('shell.searchPlaceholder')}><Search size={15} /><span>{t('shell.searchPlaceholder')}</span><kbd>Ctrl K</kbd></button>
          <div className="topbar-actions">
              <span className={`session-role ${role}`}>{t(`users.role_${role}`)}</span>
              <InAppNotifications />
              <select
                value={locale}
                onChange={(e) => changeLocale(e.target.value as Locale)}
                className="language-select"
                aria-label={t('shell.language')}
              >
                {locales.map((l) => (
                  <option key={l} value={l}>{localeLabels[l]}</option>
                ))}
              </select>
              <button onClick={handleLogout} className="logout-button" aria-label={t('shell.logout')} title={t('shell.logout')}><CircleUserRound size={16} /><span>{t('shell.logout')}</span></button>
          </div>
        </header>
        <main className="app-main"><RouteErrorBoundary resetKey={location.pathname}><Suspense fallback={<RouteLoading />}><Outlet /></Suspense></RouteErrorBoundary></main>
      </section>
      {paletteOpen && <CommandPalette ariaLabel={t('shell.globalSearch')} placeholder={t('shell.searchPlaceholder')} query={query} groups={commandGroups} loading={searchLoading} loadingLabel={t('shell.searchLoading')} emptyLabel={t('shell.noResults')} keyboardHint={t('shell.searchKeyboardHint')} error={searchError} onQueryChange={setQuery} onNavigate={navigateFromPalette} onClose={closePalette} />}
    </div>
  )
}

function App() {
  return (
    <BrowserRouter>
      <AppDialogs />
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/builds/:id/logs" element={<Suspense fallback={<RouteLoading />}><BuildLogViewer /></Suspense>} />
        <Route element={<Layout />}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/projects" element={<Projects />} />
          <Route path="/projects/new" element={<RoleGate roles={['admin', 'developer']}><CreateProject /></RoleGate>} />
          <Route path="/projects/:id" element={<ProjectDetail />} />
          <Route path="/pipeline" element={<LegacyPipelineRedirect />} />
          <Route path="/builds" element={<Builds />} />
          <Route path="/builds/:id" element={<BuildDetail />} />
          <Route path="/builds/:id/tests" element={<TestReports />} />
          <Route path="/build-queue" element={<BuildQueue />} />
          <Route path="/agents" element={<Agents />} />
          <Route path="/vcs-roots" element={<VCSRoots />} />
          <Route path="/templates" element={<Templates />} />
          <Route path="/deployments" element={<Deployments />} />
          <Route path="/plugins" element={<Plugins />} />
          <Route path="/credentials" element={<RoleGate roles={['admin']}><Credentials /></RoleGate>} />
          <Route path="/notifications" element={<RoleGate roles={['admin']}><Notifications /></RoleGate>} />
          <Route path="/statistics" element={<Statistics />} />
          <Route path="/bigscreen" element={<BigScreen />} />
          <Route path="/git-hooks" element={<Navigate to="/projects" replace />} />
          <Route path="/audit-log" element={<RoleGate roles={['admin']}><AuditLog /></RoleGate>} />
          <Route path="/api-tokens" element={<APITokens />} />
          <Route path="/users" element={<RoleGate roles={['admin']}><Users /></RoleGate>} />
          <Route path="/settings" element={<RoleGate roles={['admin']}><Settings /></RoleGate>} />
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}

function RoleGate({ roles, children }: { roles: UserRole[]; children: ReactNode }) {
  return roles.includes(currentRole()) ? children : <Navigate to="/" replace />
}

function LegacyPipelineRedirect() {
  const [searchParams] = useSearchParams()
  const projectID = Number(searchParams.get('project'))
  return <Navigate to={Number.isFinite(projectID) && projectID > 0 ? `/projects/${projectID}?view=settings` : '/projects'} replace />
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
