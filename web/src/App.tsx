import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import Projects from './pages/Projects'
import Builds from './pages/Builds'

function App() {
  return (
    <BrowserRouter>
      <div className="min-h-screen bg-gray-100">
        <nav className="bg-white shadow">
          <div className="max-w-7xl mx-auto px-4">
            <div className="flex justify-between h-16">
              <div className="flex">
                <a href="/" className="flex items-center px-2 py-2 text-gray-900 font-bold">
                  buildworld233
                </a>
                <a href="/projects" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  Projects
                </a>
                <a href="/builds" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  Builds
                </a>
              </div>
            </div>
          </div>
        </nav>
        <main className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/projects" element={<Projects />} />
            <Route path="/builds" element={<Builds />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}

export default App
