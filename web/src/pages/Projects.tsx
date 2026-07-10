import { useNavigate } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

export default function Projects() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { data: projects, loading, error, reload } = useApi(() => api.listProjects())

  const handleBuild = async (id: number) => {
    try {
      await api.triggerBuild(id)
      reload()
    } catch (e: any) {
      alert(e.message || 'Failed to trigger build')
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this project?')) return
    try {
      await api.deleteProject(id)
      reload()
    } catch (e: any) {
      alert(e.message || 'Failed to delete project')
    }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('projects.title')}</h1>
        <button
          onClick={() => navigate('/projects/new')}
          className="bg-blue-500 text-white px-4 py-2 rounded"
        >
          {t('projects.newProject')}
        </button>
      </div>

      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('projects.name')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Repo URL</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Default Branch</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Created</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('projects.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {(projects || []).length === 0 && (
              <tr><td colSpan={5} className="px-6 py-4 text-gray-500">{t('common.noData')}</td></tr>
            )}
            {(projects || []).map((project) => (
              <tr key={project.id} className="border-b">
                <td className="px-6 py-4 font-medium">
                  <button
                    className="text-blue-600 hover:underline"
                    onClick={() => navigate(`/projects/${project.id}`)}
                  >
                    {project.name}
                  </button>
                </td>
                <td className="px-6 py-4 text-gray-500 truncate max-w-xs">{project.repo_url}</td>
                <td className="px-6 py-4 text-gray-500">{project.default_branch}</td>
                <td className="px-6 py-4 text-gray-500">
                  {project.created_at ? new Date(project.created_at).toLocaleString() : '-'}
                </td>
                <td className="px-6 py-4">
                  <button
                    onClick={() => handleBuild(project.id)}
                    className="bg-blue-500 text-white px-3 py-1 rounded text-sm mr-2"
                  >
                    {t('projects.build')}
                  </button>
                  <button
                    onClick={() => handleDelete(project.id)}
                    className="text-red-500 hover:underline text-sm"
                  >
                    {t('projects.delete')}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
