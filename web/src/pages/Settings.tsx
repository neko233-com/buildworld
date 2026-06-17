import { useI18n } from '../i18n'

export default function Settings() {
  const { t } = useI18n();

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">{t('settings.title')}</h1>
      <div className="bg-white shadow rounded-lg">
        <div className="p-6">
          <div className="space-y-6">
            <div>
              <h2 className="text-lg font-semibold mb-2">{t('settings.general')}</h2>
              <div className="ml-4 space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-gray-600">Server Port</span>
                  <span className="font-mono">6050</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-gray-600">Default Language</span>
                  <select className="border rounded px-2 py-1">
                    <option value="en">English</option>
                    <option value="zh-CN">中文</option>
                  </select>
                </div>
              </div>
            </div>
            
            <div>
              <h2 className="text-lg font-semibold mb-2">{t('settings.security')}</h2>
              <div className="ml-4 space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-gray-600">HTTPS</span>
                  <button className="bg-blue-500 text-white px-4 py-1 rounded">
                    {t('common.enable')}
                  </button>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-gray-600">Two-Factor Auth</span>
                  <button className="bg-gray-300 text-gray-700 px-4 py-1 rounded">
                    {t('common.enable')}
                  </button>
                </div>
              </div>
            </div>
            
            <div>
              <h2 className="text-lg font-semibold mb-2">{t('settings.backup')}</h2>
              <div className="ml-4 space-y-2">
                <button className="bg-green-500 text-white px-4 py-2 rounded">
                  {t('settings.backup')} - Export
                </button>
                <button className="bg-yellow-500 text-white px-4 py-2 rounded ml-2">
                  {t('settings.backup')} - Import
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
