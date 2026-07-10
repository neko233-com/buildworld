import { useState } from 'react'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

const roleColors: Record<string, string> = {
  admin: 'bg-red-100 text-red-800',
  developer: 'bg-blue-100 text-blue-800',
  viewer: 'bg-gray-100 text-gray-800',
}

const ROLES = ['admin', 'developer', 'viewer']

export default function Users() {
  const { t } = useI18n()
  const { data: users, loading, error, reload } = useApi(() => api.listUsers())
  const { data: me } = useApi(() => api.me())

  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ username: '', email: '', password: '', role: 'viewer' })
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await api.createUser(form)
      setForm({ username: '', email: '', password: '', role: 'viewer' })
      setShowForm(false)
      reload()
    } catch (err: any) {
      setFormError(err.message || 'Failed to create user')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: number) => {
    if (me && me.id === id) {
      alert('Cannot delete yourself')
      return
    }
    if (!confirm('Delete this user?')) return
    try {
      await api.deleteUser(id)
      reload()
    } catch (e: any) {
      alert(e.message || 'Failed to delete user')
    }
  }

  const handleRoleChange = async (id: number, role: string) => {
    try {
      await api.updateUserRole(id, role)
      reload()
    } catch (e: any) {
      alert(e.message || 'Failed to update role')
    }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('users.title')}</h1>
        <button
          onClick={() => setShowForm(!showForm)}
          className="bg-blue-500 text-white px-4 py-2 rounded"
        >
          {t('users.addUser')}
        </button>
      </div>

      {/* Add user form */}
      {showForm && (
        <form onSubmit={handleCreate} className="bg-white shadow rounded-lg p-6 mb-6 space-y-4 max-w-2xl">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">{t('users.username')}</label>
              <input
                type="text"
                required
                value={form.username}
                onChange={e => setForm({ ...form, username: e.target.value })}
                className="w-full border rounded px-3 py-2"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">{t('users.email')}</label>
              <input
                type="email"
                required
                value={form.email}
                onChange={e => setForm({ ...form, email: e.target.value })}
                className="w-full border rounded px-3 py-2"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Password</label>
              <input
                type="password"
                required
                value={form.password}
                onChange={e => setForm({ ...form, password: e.target.value })}
                className="w-full border rounded px-3 py-2"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">{t('users.role')}</label>
              <select
                value={form.role}
                onChange={e => setForm({ ...form, role: e.target.value })}
                className="w-full border rounded px-3 py-2"
              >
                {ROLES.map(r => <option key={r} value={r}>{r}</option>)}
              </select>
            </div>
          </div>
          {formError && <p className="text-red-500 text-sm">{formError}</p>}
          <div className="flex gap-2">
            <button type="submit" disabled={saving} className="bg-blue-500 text-white px-4 py-2 rounded disabled:opacity-50">
              {saving ? t('common.loading') : t('common.save')}
            </button>
            <button type="button" onClick={() => setShowForm(false)} className="bg-gray-300 text-gray-700 px-4 py-2 rounded">
              {t('common.cancel')}
            </button>
          </div>
        </form>
      )}

      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('users.username')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('users.email')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('users.role')}</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Created</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">{t('users.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {(users || []).length === 0 && (
              <tr><td colSpan={5} className="px-6 py-4 text-gray-500">{t('common.noData')}</td></tr>
            )}
            {(users || []).map((user) => (
              <tr key={user.id} className="border-b">
                <td className="px-6 py-4 font-medium">
                  {user.username}
                  {me && me.id === user.id && <span className="text-gray-400 text-xs ml-2">(you)</span>}
                </td>
                <td className="px-6 py-4">{user.email}</td>
                <td className="px-6 py-4">
                  <select
                    value={user.role}
                    onChange={e => handleRoleChange(user.id, e.target.value)}
                    className={`px-2 py-1 rounded text-sm border-0 ${roleColors[user.role] || 'bg-gray-100 text-gray-800'}`}
                  >
                    {ROLES.map(r => <option key={r} value={r}>{r}</option>)}
                  </select>
                </td>
                <td className="px-6 py-4 text-gray-500">
                  {user.created_at ? new Date(user.created_at).toLocaleString() : '-'}
                </td>
                <td className="px-6 py-4">
                  <button
                    onClick={() => handleDelete(user.id)}
                    className="text-red-500 hover:underline text-sm"
                  >
                    {t('users.delete')}
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
