import { useState } from 'react'
import { useI18n } from '../i18n'

export default function Plugins() {
  const { t } = useI18n()
  const [filter, setFilter] = useState<'all' | 'enabled' | 'disabled'>('all')

  const plugins = [
    {
      id: 1,
      name: 'github',
      version: '1.2.0',
      description: 'GitHub integration for webhooks and status checks',
      enabled: true,
      author: 'buildworld233',
      category: 'scm',
    },
    {
      id: 2,
      name: 'docker',
      version: '1.1.0',
      description: 'Docker build and push capabilities',
      enabled: true,
      author: 'buildworld233',
      category: 'build',
    },
    {
      id: 3,
      name: 'slack',
      version: '1.0.0',
      description: 'Slack notifications for build status',
      enabled: false,
      author: 'community',
      category: 'notification',
    },
    {
      id: 4,
      name: 'kubernetes',
      version: '1.0.0',
      description: 'Kubernetes deployment support',
      enabled: true,
      author: 'buildworld233',
      category: 'deploy',
    },
  ]

  const filteredPlugins = plugins.filter(p => {
    if (filter === 'enabled') return p.enabled
    if (filter === 'disabled') return !p.enabled
    return true
  })

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('plugins.title')}</h1>
        <button className="bg-blue-500 text-white px-4 py-2 rounded">
          {t('plugins.install')}
        </button>
      </div>

      {/* Filter */}
      <div className="flex gap-2 mb-4">
        <button
          className={`px-4 py-2 rounded ${filter === 'all' ? 'bg-blue-500 text-white' : 'bg-gray-200'}`}
          onClick={() => setFilter('all')}
        >
          All ({plugins.length})
        </button>
        <button
          className={`px-4 py-2 rounded ${filter === 'enabled' ? 'bg-blue-500 text-white' : 'bg-gray-200'}`}
          onClick={() => setFilter('enabled')}
        >
          Enabled ({plugins.filter(p => p.enabled).length})
        </button>
        <button
          className={`px-4 py-2 rounded ${filter === 'disabled' ? 'bg-blue-500 text-white' : 'bg-gray-200'}`}
          onClick={() => setFilter('disabled')}
        >
          Disabled ({plugins.filter(p => !p.enabled).length})
        </button>
      </div>

      {/* Plugin List */}
      <div className="space-y-4">
        {filteredPlugins.map((plugin) => (
          <div key={plugin.id} className="bg-white shadow rounded-lg p-4">
            <div className="flex justify-between items-start">
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="font-semibold text-lg">{plugin.name}</h3>
                  <span className="text-sm text-gray-500">v{plugin.version}</span>
                  <span className={`px-2 py-1 rounded text-xs ${
                    plugin.enabled ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'
                  }`}>
                    {plugin.enabled ? 'Enabled' : 'Disabled'}
                  </span>
                </div>
                <p className="text-gray-600 mt-1">{plugin.description}</p>
                <div className="flex gap-4 mt-2 text-sm text-gray-500">
                  <span>Author: {plugin.author}</span>
                  <span>Category: {plugin.category}</span>
                </div>
              </div>
              <div className="flex gap-2">
                <button className={`px-3 py-1 rounded text-sm ${
                  plugin.enabled 
                    ? 'bg-yellow-100 text-yellow-800 hover:bg-yellow-200' 
                    : 'bg-green-100 text-green-800 hover:bg-green-200'
                }`}>
                  {plugin.enabled ? t('plugins.disable') : t('plugins.enable')}
                </button>
                <button className="px-3 py-1 rounded text-sm bg-blue-100 text-blue-800 hover:bg-blue-200">
                  {t('plugins.settings')}
                </button>
                <button className="px-3 py-1 rounded text-sm bg-red-100 text-red-800 hover:bg-red-200">
                  {t('plugins.uninstall')}
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
