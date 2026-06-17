import { useI18n } from '../i18n'

export default function Users() {
  const { t } = useI18n();

  const users = [
    { id: 1, username: 'root', email: 'admin@buildworld233.local', role: 'admin' },
    { id: 2, username: 'developer', email: 'dev@example.com', role: 'developer' },
    { id: 3, username: 'viewer', email: 'viewer@example.com', role: 'viewer' },
  ];

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('users.title')}</h1>
        <button className="bg-blue-500 text-white px-4 py-2 rounded">
          {t('users.addUser')}
        </button>
      </div>
      
      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('users.username')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('users.email')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('users.role')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('users.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {users.map((user) => (
              <tr key={user.id} className="border-b">
                <td className="px-6 py-4">{user.username}</td>
                <td className="px-6 py-4">{user.email}</td>
                <td className="px-6 py-4">
                  <span className={`px-2 py-1 rounded text-sm ${
                    user.role === 'admin' ? 'bg-red-100 text-red-800' :
                    user.role === 'developer' ? 'bg-blue-100 text-blue-800' :
                    'bg-gray-100 text-gray-800'
                  }`}>
                    {user.role}
                  </span>
                </td>
                <td className="px-6 py-4">
                  <button className="text-blue-500 hover:underline mr-2">{t('users.edit')}</button>
                  <button className="text-red-500 hover:underline">{t('users.delete')}</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
