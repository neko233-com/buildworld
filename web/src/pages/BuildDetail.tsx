import { useState } from 'react'
import { useI18n } from '../i18n'

interface BuildLog {
  timestamp: string
  level: 'info' | 'warn' | 'error'
  stage: string
  message: string
}

export default function BuildDetail() {
  const { t } = useI18n()
  const [activeTab, setActiveTab] = useState<'console' | 'artifacts' | 'parameters'>('console')

  const build = {
    id: 123,
    project: 'my-app',
    status: 'success',
    duration: '2m 34s',
    trigger: 'webhook',
    branch: 'main',
    commit: 'abc123def',
    startedAt: '2026-06-18 10:30:00',
    finishedAt: '2026-06-18 10:32:34',
  }

  const logs: BuildLog[] = [
    { timestamp: '10:30:01', level: 'info', stage: 'Checkout', message: 'Cloning repository...' },
    { timestamp: '10:30:05', level: 'info', stage: 'Checkout', message: 'Repository cloned successfully' },
    { timestamp: '10:30:10', level: 'info', stage: 'Install', message: 'Running npm ci...' },
    { timestamp: '10:30:45', level: 'info', stage: 'Install', message: 'Dependencies installed' },
    { timestamp: '10:30:50', level: 'info', stage: 'Build', message: 'Running npm run build...' },
    { timestamp: '10:31:30', level: 'info', stage: 'Build', message: 'Build completed successfully' },
    { timestamp: '10:31:35', level: 'info', stage: 'Test', message: 'Running npm test...' },
    { timestamp: '10:32:00', level: 'info', stage: 'Test', message: 'All tests passed (42/42)' },
    { timestamp: '10:32:05', level: 'info', stage: 'Deploy', message: 'Deploying to production...' },
    { timestamp: '10:32:30', level: 'info', stage: 'Deploy', message: 'Deployment successful' },
  ]

  const artifacts = [
    { name: 'dist.zip', size: '2.3 MB', type: 'archive' },
    { name: 'build.log', size: '45 KB', type: 'log' },
    { name: 'test-report.html', size: '128 KB', type: 'report' },
  ]

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <div>
          <h1 className="text-2xl font-bold">{build.project} #{build.id}</h1>
          <p className="text-gray-500">Build Details</p>
        </div>
        <div className="flex gap-2">
          <button className="bg-blue-500 text-white px-4 py-2 rounded">
            Rebuild
          </button>
          <button className="bg-gray-300 text-gray-700 px-4 py-2 rounded">
            {t('common.cancel')}
          </button>
        </div>
      </div>

      {/* Build Info */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.status')}</p>
          <p className={`font-semibold ${build.status === 'success' ? 'text-green-600' : 'text-red-600'}`}>
            {t(`status.${build.status}`)}
          </p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.duration')}</p>
          <p className="font-semibold">{build.duration}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.branch')}</p>
          <p className="font-semibold">{build.branch}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">{t('builds.commit')}</p>
          <p className="font-semibold font-mono">{build.commit}</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-white shadow rounded-lg">
        <div className="border-b">
          <nav className="flex">
            <button
              className={`px-4 py-2 text-sm font-medium ${
                activeTab === 'console' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-gray-500 hover:text-gray-700'
              }`}
              onClick={() => setActiveTab('console')}
            >
              {t('builds.logs')}
            </button>
            <button
              className={`px-4 py-2 text-sm font-medium ${
                activeTab === 'artifacts' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-gray-500 hover:text-gray-700'
              }`}
              onClick={() => setActiveTab('artifacts')}
            >
              {t('builds.artifacts')}
            </button>
            <button
              className={`px-4 py-2 text-sm font-medium ${
                activeTab === 'parameters' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-gray-500 hover:text-gray-700'
              }`}
              onClick={() => setActiveTab('parameters')}
            >
              {t('builds.parameters')}
            </button>
          </nav>
        </div>

        <div className="p-4">
          {activeTab === 'console' && (
            <div className="bg-gray-900 text-green-400 p-4 rounded font-mono text-sm overflow-auto max-h-96">
              {logs.map((log, i) => (
                <div key={i} className="flex">
                  <span className="text-gray-500 mr-2">[{log.timestamp}]</span>
                  <span className="text-yellow-400 mr-2">[{log.stage}]</span>
                  <span>{log.message}</span>
                </div>
              ))}
            </div>
          )}

          {activeTab === 'artifacts' && (
            <table className="min-w-full">
              <thead>
                <tr className="border-b">
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Name</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Size</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Type</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Action</th>
                </tr>
              </thead>
              <tbody>
                {artifacts.map((artifact, i) => (
                  <tr key={i} className="border-b">
                    <td className="px-4 py-2 font-medium">{artifact.name}</td>
                    <td className="px-4 py-2 text-gray-500">{artifact.size}</td>
                    <td className="px-4 py-2 text-gray-500">{artifact.type}</td>
                    <td className="px-4 py-2">
                      <button className="text-blue-500 hover:underline">Download</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {activeTab === 'parameters' && (
            <div className="space-y-2">
              <div className="flex justify-between p-2 bg-gray-50 rounded">
                <span className="font-medium">environment</span>
                <span className="text-gray-600">production</span>
              </div>
              <div className="flex justify-between p-2 bg-gray-50 rounded">
                <span className="font-medium">version</span>
                <span className="text-gray-600">v1.2.3</span>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
