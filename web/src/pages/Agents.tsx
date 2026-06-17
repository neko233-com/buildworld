import { useState } from 'react'
import { useI18n } from '../i18n'

export default function Agents() {
  const { t } = useI18n()
  const [selectedAgent, setSelectedAgent] = useState<string | null>(null)

  const agents = [
    {
      id: 'agent-01',
      name: 'agent-01',
      address: '192.168.1.100:7050',
      status: 'online',
      labels: ['linux', 'amd64', 'gpu'],
      activeBuilds: 2,
      maxBuilds: 8,
      cpu: 34,
      memory: 45,
      lastHeartbeat: '10 seconds ago',
    },
    {
      id: 'agent-02',
      name: 'agent-02',
      address: '192.168.1.101:7050',
      status: 'online',
      labels: ['macos', 'arm64', 'ios'],
      activeBuilds: 1,
      maxBuilds: 4,
      cpu: 12,
      memory: 38,
      lastHeartbeat: '5 seconds ago',
    },
    {
      id: 'agent-03',
      name: 'agent-03',
      address: '192.168.1.102:7050',
      status: 'offline',
      labels: ['windows', 'amd64', 'dotnet'],
      activeBuilds: 0,
      maxBuilds: 4,
      cpu: 0,
      memory: 0,
      lastHeartbeat: '5 minutes ago',
    },
  ]

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'online': return 'bg-green-100 text-green-800'
      case 'offline': return 'bg-red-100 text-red-800'
      case 'busy': return 'bg-yellow-100 text-yellow-800'
      default: return 'bg-gray-100 text-gray-800'
    }
  }

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('nav.workers')}</h1>
        <button className="bg-blue-500 text-white px-4 py-2 rounded">
          + Add Agent
        </button>
      </div>

      {/* Agent Overview */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Total Agents</p>
          <p className="text-2xl font-bold">{agents.length}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Online</p>
          <p className="text-2xl font-bold text-green-600">{agents.filter(a => a.status === 'online').length}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Offline</p>
          <p className="text-2xl font-bold text-red-600">{agents.filter(a => a.status === 'offline').length}</p>
        </div>
        <div className="bg-white p-4 rounded-lg shadow">
          <p className="text-sm text-gray-500">Active Builds</p>
          <p className="text-2xl font-bold text-blue-600">{agents.reduce((sum, a) => sum + a.activeBuilds, 0)}</p>
        </div>
      </div>

      {/* Agent List */}
      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Name</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Status</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Labels</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Builds</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">CPU</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Memory</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Heartbeat</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Actions</th>
            </tr>
          </thead>
          <tbody>
            {agents.map((agent) => (
              <tr 
                key={agent.id} 
                className={`border-b cursor-pointer ${selectedAgent === agent.id ? 'bg-blue-50' : 'hover:bg-gray-50'}`}
                onClick={() => setSelectedAgent(agent.id)}
              >
                <td className="px-4 py-3 font-medium">{agent.name}</td>
                <td className="px-4 py-3">
                  <span className={`px-2 py-1 rounded text-sm ${getStatusColor(agent.status)}`}>
                    {agent.status}
                  </span>
                </td>
                <td className="px-4 py-3">
                  <div className="flex gap-1">
                    {agent.labels.map((label) => (
                      <span key={label} className="px-2 py-1 bg-gray-100 text-gray-600 rounded text-xs">
                        {label}
                      </span>
                    ))}
                  </div>
                </td>
                <td className="px-4 py-3">{agent.activeBuilds}/{agent.maxBuilds}</td>
                <td className="px-4 py-3">{agent.cpu}%</td>
                <td className="px-4 py-3">{agent.memory}%</td>
                <td className="px-4 py-3 text-gray-500">{agent.lastHeartbeat}</td>
                <td className="px-4 py-3">
                  <button className="text-blue-500 hover:underline mr-2">Enable</button>
                  <button className="text-red-500 hover:underline">Disable</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Agent Details Panel */}
      {selectedAgent && (
        <div className="mt-6 bg-white shadow rounded-lg p-6">
          <h2 className="text-lg font-semibold mb-4">Agent Details</h2>
          {agents.filter(a => a.id === selectedAgent).map((agent) => (
            <div key={agent.id} className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div>
                <p className="text-sm text-gray-500">Address</p>
                <p className="font-mono">{agent.address}</p>
              </div>
              <div>
                <p className="text-sm text-gray-500">Status</p>
                <p className={agent.status === 'online' ? 'text-green-600' : 'text-red-600'}>{agent.status}</p>
              </div>
              <div>
                <p className="text-sm text-gray-500">CPU Usage</p>
                <div className="w-full bg-gray-200 rounded-full h-2">
                  <div className="bg-blue-600 h-2 rounded-full" style={{width: `${agent.cpu}%`}}></div>
                </div>
                <p className="text-xs text-gray-500">{agent.cpu}%</p>
              </div>
              <div>
                <p className="text-sm text-gray-500">Memory Usage</p>
                <div className="w-full bg-gray-200 rounded-full h-2">
                  <div className="bg-green-600 h-2 rounded-full" style={{width: `${agent.memory}%`}}></div>
                </div>
                <p className="text-xs text-gray-500">{agent.memory}%</p>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
