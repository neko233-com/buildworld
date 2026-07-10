import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

const SAMPLE_JSON = `{
  "stages": [
    {"name": "Build", "steps": [{"name": "compile", "type": "shell", "command": "echo building"}]}
  ]
}`

const SAMPLE_YAML = `name: Build and Deploy
on: [push, manual]
env:
  NODE_ENV: production
jobs:
  build:
    runs-on: local
    steps:
      - uses: actions/checkout@v4
      - name: Install
        run: npm install
        shell: bash
      - name: Build
        run: npm run build
        shell: bash
  test:
    needs: build
    runs-on: local
    steps:
      - name: Test
        run: npm test
        shell: bash
`

export default function CreateProject() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [format, setFormat] = useState<'json' | 'yaml'>('yaml')
  const [form, setForm] = useState({
    name: '',
    description: '',
    repo_url: '',
    repo_type: 'git',
    default_branch: 'main',
    config: SAMPLE_YAML,
    vcs_root_id: '' as number | '',
    template_id: '' as number | '',
  })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const { data: vcsRoots } = useApi(() => api.listVCSRoots())
  const { data: templates } = useApi(() => api.listTemplates())

  useEffect(() => {
    const templateParam = searchParams.get('template')
    if (templateParam) {
      const tid = Number(templateParam)
      setForm(p => ({ ...p, template_id: tid }))
    }
  }, [searchParams])

  useEffect(() => {
    if (form.template_id === '' || form.template_id === undefined) return
    const tmpl = templates?.find((t: any) => t.id === Number(form.template_id))
    if (tmpl && tmpl.config) {
      const cfg = typeof tmpl.config === 'string' ? tmpl.config : JSON.stringify(tmpl.config, null, 2)
      const looksLikeJson = cfg.trim().startsWith('{')
      setFormat(looksLikeJson ? 'json' : 'yaml')
      setForm(p => ({ ...p, config: cfg }))
    }
  }, [form.template_id, templates])

  useEffect(() => {
    if (form.vcs_root_id === '' || form.vcs_root_id === undefined) return
    const root = vcsRoots?.find((r: any) => r.id === Number(form.vcs_root_id))
    if (root) {
      setForm(p => ({
        ...p,
        repo_url: root.url || p.repo_url,
        repo_type: root.type || p.repo_type,
        default_branch: root.branch || p.default_branch,
      }))
    }
  }, [form.vcs_root_id, vcsRoots])

  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
    const value = e.target.value
    if (key === 'vcs_root_id' || key === 'template_id') {
      setForm(p => ({ ...p, [key]: value === '' ? '' : Number(value) }))
    } else {
      setForm(p => ({ ...p, [key]: value }))
    }
  }

  const switchFormat = (f: 'json' | 'yaml') => {
    setFormat(f)
    setForm(p => ({ ...p, config: f === 'json' ? SAMPLE_JSON : SAMPLE_YAML, template_id: '' }))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError('')
    try {
      const data: any = {
        name: form.name,
        description: form.description,
        repo_url: form.repo_url,
        repo_type: form.repo_type,
        default_branch: form.default_branch,
        config: form.config,
      }
      if (form.vcs_root_id !== '') data.vcs_root_id = Number(form.vcs_root_id)
      await api.createProject(data)
      navigate('/projects')
    } catch (err: any) {
      setError(err.message || 'Failed to create project')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">{t('projects.newProject')}</h1>

      <form onSubmit={handleSubmit} className="bg-white shadow rounded-lg p-6 space-y-4 max-w-3xl">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">{t('projects.name')}</label>
          <input type="text" required value={form.name} onChange={set('name')}
            placeholder="my-project" className="w-full border rounded px-3 py-2" />
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
          <textarea value={form.description} onChange={set('description')}
            placeholder="A brief description of your project"
            className="w-full border rounded px-3 py-2" rows={2} />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">VCS Root</label>
            <select value={form.vcs_root_id} onChange={set('vcs_root_id')} className="w-full border rounded px-3 py-2">
              <option value="">Manual (specify URL)</option>
              {vcsRoots?.map((r: any) => (
                <option key={r.id} value={r.id}>{r.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Template</label>
            <select value={form.template_id} onChange={set('template_id')} className="w-full border rounded px-3 py-2">
              <option value="">None</option>
              {templates?.map((t: any) => (
                <option key={t.id} value={t.id}>{t.name}</option>
              ))}
            </select>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Repository URL</label>
            <input type="text" value={form.repo_url} onChange={set('repo_url')}
              placeholder="https://github.com/user/repo" className="w-full border rounded px-3 py-2" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Default Branch</label>
            <input type="text" value={form.default_branch} onChange={set('default_branch')}
              className="w-full border rounded px-3 py-2" />
          </div>
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Repo Type</label>
            <select value={form.repo_type} onChange={set('repo_type')} className="w-full border rounded px-3 py-2">
              <option value="git">git</option>
              <option value="svn">svn</option>
              <option value="hg">hg (Mercurial)</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Config Format</label>
            <div className="flex gap-2">
              <button type="button" onClick={() => switchFormat('yaml')}
                className={`px-4 py-2 rounded ${format === 'yaml' ? 'bg-blue-600 text-white' : 'bg-gray-200'}`}>
                YAML (GitHub Actions)
              </button>
              <button type="button" onClick={() => switchFormat('json')}
                className={`px-4 py-2 rounded ${format === 'json' ? 'bg-blue-600 text-white' : 'bg-gray-200'}`}>
                JSON
              </button>
            </div>
          </div>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            Pipeline Config ({format === 'yaml' ? 'YAML' : 'JSON'})
          </label>
          <textarea value={form.config} onChange={set('config')}
            className="w-full border rounded px-3 py-2 font-mono text-sm" rows={14} />
          <p className="text-xs text-gray-500 mt-1">
            {format === 'yaml'
              ? 'Supports GitHub Actions style: jobs.<id>.steps with run/uses, or native YAML with stages.'
              : 'Native JSON format: stages[].steps[] with type/command/config.'}
          </p>
        </div>

        {error && <p className="text-red-500 text-sm">{error}</p>}

        <div className="flex gap-2 pt-2">
          <button type="submit" disabled={saving}
            className="bg-blue-500 text-white px-6 py-2 rounded disabled:opacity-50">
            {saving ? t('common.loading') : t('common.save')}
          </button>
          <button type="button" onClick={() => navigate('/projects')}
            className="bg-gray-300 text-gray-700 px-6 py-2 rounded">
            {t('common.cancel')}
          </button>
        </div>
      </form>
    </div>
  )
}
