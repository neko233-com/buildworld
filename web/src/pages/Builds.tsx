import { useI18n } from '../i18n'

export default function Builds() {
  const { t } = useI18n();

  const builds = [
    { id: 1, project: 'my-app', number: 123, status: 'success', duration: '2m 34s', trigger: 'webhook', branch: 'main' },
    { id: 2, project: 'api-server', number: 45, status: 'failed', duration: '5m 12s', trigger: 'manual', branch: 'develop' },
    { id: 3, project: 'website', number: 67, status: 'success', duration: '1m 45s', trigger: 'schedule', branch: 'main' },
    { id: 4, project: 'mobile-app', number: 89, status: 'running', duration: '3m 20s', trigger: 'webhook', branch: 'feature/auth' },
  ];

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">{t('builds.title')}</h1>
      
      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.buildNumber')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('projects.name')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.status')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.duration')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.trigger')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.branch')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('builds.logs')}</th>
            </tr>
          </thead>
          <tbody>
            {builds.map((build) => (
              <tr key={build.id} className="border-b">
                <td className="px-6 py-4">#{build.number}</td>
                <td className="px-6 py-4 font-medium">{build.project}</td>
                <td className="px-6 py-4">
                  <span className={`px-2 py-1 rounded text-sm ${
                    build.status === 'success' ? 'bg-green-100 text-green-800' :
                    build.status === 'failed' ? 'bg-red-100 text-red-800' :
                    build.status === 'running' ? 'bg-blue-100 text-blue-800' :
                    'bg-gray-100 text-gray-800'
                  }`}>
                    {t(`status.${build.status}`)}
                  </span>
                </td>
                <td className="px-6 py-4 text-gray-500">{build.duration}</td>
                <td className="px-6 py-4 text-gray-500">{build.trigger}</td>
                <td className="px-6 py-4 text-gray-500">{build.branch}</td>
                <td className="px-6 py-4">
                  <button className="text-blue-500 hover:underline">{t('builds.logs')}</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
