import { useState } from 'react'
import { useI18n } from '../i18n'

export default function ProjectDetail() {
  const { t } = useI18n()
  const [activeTab, setActiveTab] = useState<'config' | 'builds' | 'settings'>('config')

  const project = {
    id: 1,
    name: 'my-app',
    description: 'A modern web application',
    repoUrl: 'https://github.com/user/my-app',
    branch: 'main',
    status: 'active',
    lastBuild: '2 minutes ago',
    lastBuildStatus: 'success',
  }

  const builds = [
    { id: 123, status: 'success', duration: '2m 34s', branch: 'main', startedAt: '2 minutes ago' },
    { id: 122, status: 'failed', duration: '1m 45s', branch: 'develop', startedAt: '1 hour ago' },
    { id: 121, status: 'success', duration: '3m 12s', branch: 'main', startedAt: '3 hours ago' },
  ]

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <div>
          <h1 className="text-2xl font-bold">{project.name}</h1>
          <p className="text-gray-500">{project.description}</p>
        </div>
        <div className="flex gap-2">
          <button className="bg-blue-500 text-white px-4 py-2 rounded">
            {t('projects.build')}
          </button>
          <button className="bg-gray-300 text-gray-700 px-4 py-2 rounded">
            {t('projects.edit')}
          </button>
        </div>
      </div>

      {/* Project Info */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('projects.status')}</p>
          <p className="font-semibold text-green-600">{project.status}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('projects.lastBuild')}</p>
          <p className="font-semibold">{project.lastBuild}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Repository</p>
          <p className="font-semibold text-blue-500 truncate">{project.repoUrl}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.branch')}</p>
          <p className="font-semibold">{project.branch}</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-white shadow rounded-lg">
        <div className="border-b">
          <nav className="flex">
            <button
              className={`px-4 py-2 text-sm font-medium ${
                activeTab === 'config' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-gray-500 hover:text-gray-700'
              }`}
              onClick={() => setActiveTab('config')}
            >
              Configuration
            </button>
            <button
              className={`px-4 py-2 text-sm font-medium ${
                activeTab === 'builds' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-gray-500 hover:text-gray-700'
              }`}
              onClick={() => setActiveTab('builds')}
            >
              {t('builds.title')}
            </button>
            <button
              className={`px-4 py-2 text-sm font-medium ${
                activeTab === 'settings' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-gray-500 hover:text-gray-700'
              }`}
              onClick={() => setActiveTab('settings')}
            >
              {t('projects.settings')}
            </button>
          </nav>
        </div>

        <div className="p-4">
          {activeTab === 'config' && (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Pipeline Configuration</label>
                <pre className="bg-gray-900 text-green-400 p-4 rounded text-sm overflow-auto">
{`pipeline({
  name: "${project.name}",
  stages: [
    {
      name: "Checkout",
      steps: [
        { type: "git", action: "clone" }
      ]
    },
    {
      name: "Build",
      steps: [
        { type: "shell", command: "npm run build" }
      ]
    },
    {
      name: "Test",
      steps: [
        { type: "shell", command: "npm test" }
      ]
    }
  ]
});`}
                </pre>
              </div>
            </div>
          )}

          {activeTab === 'builds' && (
            <table className="min-w-full">
              <thead>
                <tr className="border-b">
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">#</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">{t('builds.status')}</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">{t('builds.duration')}</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">{t('builds.branch')}</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Started</th>
                </tr>
              </thead>
              <tbody>
                {builds.map((build) => (
                  <tr key={build.id} className="border-b hover:bg-gray-50 cursor-pointer">
                    <td className="px-4 py-2 font-medium">#{build.id}</td>
                    <td className="px-4 py-2">
                      <span className={`px-2 py-1 rounded text-sm ${
                        build.status === 'success' ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'
                      }`}>
                        {t(`status.${build.status}`)}
                      </span>
                    </td>
                    <td className="px-4 py-2 text-gray-500">{build.duration}</td>
                    <td className="px-4 py-2 text-gray-500">{build.branch}</td>
                    <td className="px-4 py-2 text-gray-500">{build.startedAt}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {activeTab === 'settings' && (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Project Name</label>
                <input type="text" defaultValue={project.name} className="w-full border rounded px-3 py-2" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
                <textarea defaultValue={project.description} className="w-full border rounded px-3 py-2" rows={3} />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Repository URL</label>
                <input type="text" defaultValue={project.repoUrl} className="w-full border rounded px-3 py-2" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Default Branch</label>
                <input type="text" defaultValue={project.branch} className="w-full border rounded px-3 py-2" />
              </div>
              <button className="bg-blue-500 text-white px-4 py-2 rounded">{t('common.save')}</button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
