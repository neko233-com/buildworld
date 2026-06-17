import { useI18n } from '../i18n'

export default function Projects() {
  const { t } = useI18n();

  const projects = [
    { id: 1, name: 'my-app', status: 'active', lastBuild: '2 minutes ago' },
    { id: 2, name: 'api-server', status: 'active', lastBuild: '5 minutes ago' },
    { id: 3, name: 'website', status: 'active', lastBuild: '1 hour ago' },
    { id: 4, name: 'mobile-app', status: 'inactive', lastBuild: '3 days ago' },
  ];

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('projects.title')}</h1>
        <div className="space-x-2">
          <button className="bg-blue-500 text-white px-4 py-2 rounded">
            {t('projects.newProject')}
          </button>
          <button className="bg-green-500 text-white px-4 py-2 rounded">
            {t('projects.fromTemplate')}
          </button>
        </div>
      </div>
      
      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('projects.name')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('projects.status')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('projects.lastBuild')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('projects.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {projects.map((project) => (
              <tr key={project.id} className="border-b">
                <td className="px-6 py-4 font-medium">{project.name}</td>
                <td className="px-6 py-4">
                  <span className={`px-2 py-1 rounded text-sm ${
                    project.status === 'active' ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'
                  }`}>
                    {project.status}
                  </span>
                </td>
                <td className="px-6 py-4 text-gray-500">{project.lastBuild}</td>
                <td className="px-6 py-4">
                  <button className="bg-blue-500 text-white px-3 py-1 rounded text-sm mr-2">
                    {t('projects.build')}
                  </button>
                  <button className="text-blue-500 hover:underline mr-2">{t('projects.edit')}</button>
                  <button className="text-red-500 hover:underline">{t('projects.delete')}</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
