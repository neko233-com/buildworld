import { useI18n } from '../i18n'

export default function Dashboard() {
  const { t } = useI18n();

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">{t('dashboard.title')}</h1>
      
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <div className="bg-white p-6 rounded-lg shadow">
          <h2 className="text-lg font-semibold">{t('dashboard.projects')}</h2>
          <p className="text-3xl font-bold text-blue-600">12</p>
        </div>
        <div className="bg-white p-6 rounded-lg shadow">
          <h2 className="text-lg font-semibold">{t('dashboard.activeBuilds')}</h2>
          <p className="text-3xl font-bold text-green-600">3</p>
        </div>
        <div className="bg-white p-6 rounded-lg shadow">
          <h2 className="text-lg font-semibold">{t('dashboard.workers')}</h2>
          <p className="text-3xl font-bold text-purple-600">5</p>
        </div>
      </div>
      
      <div className="bg-white shadow rounded-lg p-6">
        <h2 className="text-lg font-semibold mb-4">{t('dashboard.recentBuilds')}</h2>
        <div className="space-y-3">
          <div className="flex items-center justify-between p-3 bg-gray-50 rounded">
            <div>
              <span className="font-medium">my-app #123</span>
              <span className="text-gray-500 ml-2">2 minutes ago</span>
            </div>
            <span className="px-2 py-1 bg-green-100 text-green-800 rounded text-sm">{t('status.success')}</span>
          </div>
          <div className="flex items-center justify-between p-3 bg-gray-50 rounded">
            <div>
              <span className="font-medium">api-server #45</span>
              <span className="text-gray-500 ml-2">5 minutes ago</span>
            </div>
            <span className="px-2 py-1 bg-red-100 text-red-800 rounded text-sm">{t('status.failed')}</span>
          </div>
          <div className="flex items-center justify-between p-3 bg-gray-50 rounded">
            <div>
              <span className="font-medium">website #67</span>
              <span className="text-gray-500 ml-2">1 hour ago</span>
            </div>
            <span className="px-2 py-1 bg-green-100 text-green-800 rounded text-sm">{t('status.success')}</span>
          </div>
        </div>
      </div>
    </div>
  )
}
