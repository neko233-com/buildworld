import { BrowserRouter, Routes, Route } from 'react-router-dom'
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
import { useI18n } from './i18n'

function App() {
  const { t, locale, changeLocale, locales } = useI18n();

  return (
    <BrowserRouter>
      <div className="min-h-screen bg-gray-100">
        <nav className="bg-white shadow">
          <div className="max-w-7xl mx-auto px-4">
            <div className="flex justify-between h-16">
              <div className="flex">
                <a href="/" className="flex items-center px-2 py-2 text-gray-900 font-bold">
                  {t('app.title')}
                </a>
                <a href="/projects" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  {t('nav.projects')}
                </a>
                <a href="/pipeline" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  Pipeline
                </a>
                <a href="/builds" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  {t('nav.builds')}
                </a>
                <a href="/agents" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  Agents
                </a>
                <a href="/plugins" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  {t('nav.plugins')}
                </a>
                <a href="/users" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  {t('nav.users')}
                </a>
                <a href="/settings" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  {t('nav.settings')}
                </a>
              </div>
              <div className="flex items-center">
                <select
                  value={locale}
                  onChange={(e) => changeLocale(e.target.value as any)}
                  className="ml-4 border rounded px-2 py-1 text-sm"
                >
                  {locales.map((l) => (
                    <option key={l} value={l}>
                      {l === 'en' ? 'English' : '中文'}
                    </option>
                  ))}
                </select>
              </div>
            </div>
          </div>
        </nav>
        <main className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/projects" element={<Projects />} />
            <Route path="/projects/new" element={<CreateProject />} />
            <Route path="/projects/:id" element={<ProjectDetail />} />
            <Route path="/pipeline" element={<Pipeline />} />
            <Route path="/builds" element={<Builds />} />
            <Route path="/builds/:id" element={<BuildDetail />} />
            <Route path="/agents" element={<Agents />} />
            <Route path="/plugins" element={<Plugins />} />
            <Route path="/users" element={<Users />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}

export default App
