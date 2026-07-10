import { BrowserRouter, Routes, Route, useNavigate } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import Projects from './pages/Projects'
import ProjectDetail from './pages/ProjectDetail'
import CreateProject from './pages/CreateProject'
import Builds from './pages/Builds'
import BuildDetail from './pages/BuildDetail'
import Pipeline from './pages/Pipeline'
import Agents from './pages/Agents'
import Plugins from './pages/Plugins'
import Settings from './pages/Settings'
import Users from './pages/Users'
import Credentials from './pages/Credentials'
import VCSRoots from './pages/VCSRoots'
import Templates from './pages/Templates'
import Notifications from './pages/Notifications'
import Statistics from './pages/Statistics'
import AuditLog from './pages/AuditLog'
import APITokens from './pages/APITokens'
import Deployments from './pages/Deployments'
import BuildQueue from './pages/BuildQueue'
import TestReports from './pages/TestReports'
import Login from './pages/Login'
import { useI18n } from './i18n'
import { clearToken } from './api'

function NavItem({ href, label }: { href: string; label: string }) {
  return (
    <a href={href} className="flex items-center px-3 py-2 text-gray-600 hover:text-gray-900 hover:bg-gray-50 rounded transition">
      {label}
    </a>
  )
}

function Layout({ children }: { children: React.ReactNode }) {
  const { t, locale, changeLocale, locales } = useI18n()
  const navigate = useNavigate()

  const handleLogout = () => {
    clearToken()
    navigate('/login')
  }

  const token = localStorage.getItem('token')
  if (!token) {
    window.location.href = '/login'
    return null
  }

  return (
    <div className="min-h-screen bg-gray-100">
      <nav className="bg-white shadow-sm">
        <div className="max-w-7xl mx-auto px-4">
          <div className="flex justify-between h-14">
            <div className="flex items-center gap-1 flex-wrap">
              <a href="/" className="flex items-center px-3 py-2 text-gray-900 font-bold text-lg">
                buildworld233
              </a>
              <NavItem href="/projects" label={t('nav.projects')} />
              <NavItem href="/pipeline" label={t('nav.pipeline')} />
              <NavItem href="/builds" label={t('nav.builds')} />
              <NavItem href="/build-queue" label={t('nav.buildQueue')} />
              <NavItem href="/agents" label={t('nav.agents')} />
              <NavItem href="/vcs-roots" label={t('nav.vcsRoots')} />
              <NavItem href="/templates" label={t('nav.templates')} />
              <NavItem href="/deployments" label={t('nav.deployments')} />
              <NavItem href="/plugins" label={t('nav.plugins')} />
              <NavItem href="/credentials" label={t('nav.credentials')} />
              <NavItem href="/notifications" label={t('settings.notifications')} />
              <NavItem href="/statistics" label={t('nav.statistics')} />
              <NavItem href="/audit-log" label={t('nav.auditLog')} />
              <NavItem href="/api-tokens" label={t('nav.apiTokens')} />
              <NavItem href="/users" label={t('nav.users')} />
              <NavItem href="/settings" label={t('nav.settings')} />
            </div>
            <div className="flex items-center gap-2">
              <select
                value={locale}
                onChange={(e) => changeLocale(e.target.value as any)}
                className="border rounded px-2 py-1 text-sm"
              >
                {locales.map((l) => (
                  <option key={l} value={l}>{l === 'en' ? 'English' : '中文'}</option>
                ))}
              </select>
              <button
                onClick={handleLogout}
                className="text-sm text-gray-500 hover:text-red-600 px-3 py-1 rounded transition"
              >
                Logout
              </button>
            </div>
          </div>
        </div>
      </nav>
      <main className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
        {children}
      </main>
    </div>
  )
}

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/" element={<Layout><Dashboard /></Layout>} />
        <Route path="/projects" element={<Layout><Projects /></Layout>} />
        <Route path="/projects/new" element={<Layout><CreateProject /></Layout>} />
        <Route path="/projects/:id" element={<Layout><ProjectDetail /></Layout>} />
        <Route path="/pipeline" element={<Layout><Pipeline /></Layout>} />
        <Route path="/builds" element={<Layout><Builds /></Layout>} />
        <Route path="/builds/:id" element={<Layout><BuildDetail /></Layout>} />
        <Route path="/builds/:id/tests" element={<Layout><TestReports /></Layout>} />
        <Route path="/build-queue" element={<Layout><BuildQueue /></Layout>} />
        <Route path="/agents" element={<Layout><Agents /></Layout>} />
        <Route path="/vcs-roots" element={<Layout><VCSRoots /></Layout>} />
        <Route path="/templates" element={<Layout><Templates /></Layout>} />
        <Route path="/deployments" element={<Layout><Deployments /></Layout>} />
        <Route path="/plugins" element={<Layout><Plugins /></Layout>} />
        <Route path="/credentials" element={<Layout><Credentials /></Layout>} />
        <Route path="/notifications" element={<Layout><Notifications /></Layout>} />
        <Route path="/statistics" element={<Layout><Statistics /></Layout>} />
        <Route path="/audit-log" element={<Layout><AuditLog /></Layout>} />
        <Route path="/api-tokens" element={<Layout><APITokens /></Layout>} />
        <Route path="/users" element={<Layout><Users /></Layout>} />
        <Route path="/settings" element={<Layout><Settings /></Layout>} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
