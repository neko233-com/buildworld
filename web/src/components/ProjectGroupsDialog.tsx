import { FolderPlus, FolderTree, Pencil, Plus, Trash2, X } from 'lucide-react'
import { useState } from 'react'
import { api } from '../api'
import { useI18n } from '../i18n'
import { PROJECT_GROUP_COLORS, normalizeProjectGroupColor, sortProjectGroups, type ProjectGroup, type ProjectGroupColor } from '../lib/projectGroups'
import { dialogs } from './AppDialogs'
import { ModalDialog } from './ModalDialog'

type Props = {
  groups: ProjectGroup[]
  projects: any[]
  onReload: () => void
  onClose: () => void
}

type GroupForm = {
  id?: number
  name: string
  description: string
  color: ProjectGroupColor
}

const emptyForm: GroupForm = { name: '', description: '', color: 'neutral' }

export default function ProjectGroupsDialog({ groups, projects, onReload, onClose }: Props) {
  const { t } = useI18n()
  const [form, setForm] = useState<GroupForm>(emptyForm)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const orderedGroups = sortProjectGroups(groups)

  const edit = (group: ProjectGroup) => {
    setForm({ id: group.id, name: group.name, description: group.description || '', color: normalizeProjectGroupColor(group.color) })
    setError('')
  }

  const save = async (event: React.FormEvent) => {
    event.preventDefault()
    const editing = Boolean(form.id)
    setSaving(true)
    setError('')
    try {
      const payload = {
        name: form.name.trim(),
        description: form.description.trim(),
        color: form.color,
      }
      if (form.id) await api.updateProjectGroup(form.id, payload)
      else await api.createProjectGroup(payload)
      setForm(emptyForm)
      onReload()
      dialogs.notify(t(editing ? 'projectGroups.updated' : 'projectGroups.created'), 'success')
    } catch (reason: any) {
      const message = reason.message || t('projectGroups.saveFailed')
      setError(message)
      dialogs.notify(message)
    } finally {
      setSaving(false)
    }
  }

  const remove = async (group: ProjectGroup) => {
    if (!await dialogs.confirm(
      t('projectGroups.deleteConfirm').replace('{name}', group.name),
      { title: t('projectGroups.deleteTitle'), action: t('common.delete') },
    )) return
    try {
      await api.deleteProjectGroup(group.id)
      if (form.id === group.id) setForm(emptyForm)
      onReload()
      dialogs.notify(t('projectGroups.deleted'), 'success')
    } catch (reason: any) {
      dialogs.notify(reason.message || t('projectGroups.deleteFailed'))
    }
  }

  return <ModalDialog className="project-groups-dialog" ariaLabel={t('projectGroups.manage')} busy={saving} onClose={onClose}>
    <header>
      <div><FolderTree size={18} /><div><h2>{t('projectGroups.manage')}</h2><p>{t('projectGroups.manageHelp')}</p></div></div>
      <button type="button" onClick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button>
    </header>
    <div className="project-groups-dialog-body">
      <section className="project-groups-list">
        <header><strong>{t('projectGroups.directoryList')}</strong><span>{groups.length}</span></header>
        {!orderedGroups.length && <div className="project-groups-empty"><FolderPlus size={20} /><strong>{t('projectGroups.empty')}</strong><small>{t('projectGroups.emptyHelp')}</small></div>}
        {orderedGroups.map(group => {
          const count = projects.filter(project => project.group_id === group.id).length
          return <article key={group.id} className={form.id === group.id ? 'selected' : ''} data-group-color={normalizeProjectGroupColor(group.color)}>
            <button type="button" className="project-group-main" onClick={() => edit(group)}>
              <span className="project-group-color-mark" aria-hidden="true" />
              <FolderTree size={16} />
              <span><strong>{group.name}</strong><small>{group.description || t('projectGroups.folderHelp')}</small></span>
              <em>{t('projectGroups.projectCount').replace('{count}', String(count))}</em>
            </button>
            <div>
              <button type="button" onClick={() => edit(group)} title={t('common.save')} aria-label={`${t('common.save')} ${group.name}`}><Pencil size={14} /></button>
              <button type="button" className="danger" onClick={() => remove(group)} title={t('common.delete')} aria-label={`${t('common.delete')} ${group.name}`}><Trash2 size={14} /></button>
            </div>
          </article>
        })}
      </section>
      <form className="project-group-editor" onSubmit={save}>
        <header><div>{form.id ? <Pencil size={16} /> : <Plus size={16} />}<strong>{form.id ? t('projectGroups.edit') : t('projectGroups.create')}</strong></div>{form.id && <button type="button" onClick={() => { setForm(emptyForm); setError('') }}>{t('projectGroups.newGroup')}</button>}</header>
        <label><span>{t('projectGroups.name')}</span><input data-dialog-initial-focus required value={form.name} onChange={event => setForm({ ...form, name: event.target.value })} placeholder={t('projectGroups.namePlaceholder')} /></label>
        <fieldset className="project-group-palette">
          <legend>{t('projectGroups.color')}</legend>
          <div>
            {PROJECT_GROUP_COLORS.map(color => <label key={color} className={form.color === color ? 'selected' : ''} data-group-color={color}>
              <input type="radio" name="project-group-color" value={color} checked={form.color === color} onChange={() => setForm({ ...form, color })} />
              <span aria-hidden="true" />
              <em>{t(`projectGroups.colors.${color}`)}</em>
            </label>)}
          </div>
          <small>{t('projectGroups.colorHelp')}</small>
        </fieldset>
        <label><span>{t('projectGroups.description')}</span><textarea rows={4} value={form.description} onChange={event => setForm({ ...form, description: event.target.value })} placeholder={t('projectGroups.descriptionPlaceholder')} /></label>
        <p className="project-group-delete-help">{t('projectGroups.deleteHelp')}</p>
        {error && <p className="form-error">{error}</p>}
        <footer><button type="button" className="secondary-command" onClick={onClose}>{t('common.cancel')}</button><button type="submit" className="primary-command" disabled={saving}>{saving ? t('common.loading') : form.id ? t('common.save') : t('projectGroups.create')}</button></footer>
      </form>
    </div>
  </ModalDialog>
}
