import { useState } from 'react'
import {
  Activity,
  Blocks,
  Code2,
  GitCommitHorizontal,
  LoaderCircle,
  Move,
  Pencil,
  Play,
  Power,
  Settings2,
  Trash2,
  X,
} from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../api'
import { dialogs } from '../components/AppDialogs'
import PluginActionLinks from '../components/PluginActionLinks'
import { ModalDialog } from '../components/ModalDialog'
import { useI18n } from '../i18n'
import { sortProjectGroups, type ProjectGroup } from '../lib/projectGroups'
import './ProjectJobActions.css'

type DialogKind = 'rename' | 'move' | 'syntax' | null
type BusyAction = Exclude<DialogKind, 'syntax' | null> | 'toggle' | null

type ProjectJobActionsProps = {
  project: any
  projectGroups: ProjectGroup[]
  projectId: number
  activeView: 'status' | 'changes'
  editable: boolean
  building: boolean
  parameterized: boolean
  deleting: boolean
  onBuildNow: () => void
  onDelete: () => void
  onProjectUpdated: (project: any) => void
}

export function projectUpdatePayload(project: any, patch: Record<string, unknown>) {
  return {
    enabled: project.enabled !== false,
    name: project.name || '',
    description: project.description || '',
    repo_url: project.repo_url || '',
    repo_type: project.repo_type || 'git',
    default_branch: project.default_branch || 'main',
    vcs_root_id: project.vcs_root_id ?? null,
    template_id: project.template_id ?? null,
    group_id: project.group_id ?? null,
    tags: project.tags || [],
    config: project.config || '',
    pipeline_format: project.pipeline_format || 'auto',
    pipeline_source_mode: project.pipeline_source_mode === 'scm' ? 'scm' : 'inline',
    pipeline_scm_repo: project.pipeline_scm_repo || '',
    pipeline_scm_branch: project.pipeline_scm_branch || '',
    pipeline_scm_path: project.pipeline_scm_path || '',
    ...patch,
  }
}

export default function ProjectJobActions({
  project,
  projectGroups,
  projectId,
  activeView,
  editable,
  building,
  parameterized,
  deleting,
  onBuildNow,
  onDelete,
  onProjectUpdated,
}: ProjectJobActionsProps) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [dialog, setDialog] = useState<DialogKind>(null)
  const [busyAction, setBusyAction] = useState<BusyAction>(null)
  const [renameValue, setRenameValue] = useState(project.name || '')
  const [moveValue, setMoveValue] = useState<string>(project.group_id == null ? '' : String(project.group_id))
  const groups = sortProjectGroups(projectGroups || [])
  const projectEnabled = project.enabled !== false

  const openRename = () => {
    setRenameValue(project.name || '')
    setDialog('rename')
  }

  const openMove = () => {
    setMoveValue(project.group_id == null ? '' : String(project.group_id))
    setDialog('move')
  }

  const persist = async (kind: 'rename' | 'move', patch: Record<string, unknown>, successMessage: string) => {
    setBusyAction(kind)
    try {
      const updated = await api.updateProject(projectId, projectUpdatePayload(project, patch))
      onProjectUpdated(updated || { ...project, ...patch })
      setDialog(null)
      dialogs.notify(successMessage, 'success')
    } catch (reason: any) {
      dialogs.notify(reason.message || t('projectDetail.updateFailed'))
    } finally {
      setBusyAction(null)
    }
  }

  const renameProject = async (event: React.FormEvent) => {
    event.preventDefault()
    const name = renameValue.trim()
    if (!name || name === project.name) {
      if (name === project.name) setDialog(null)
      return
    }
    await persist('rename', { name }, t('projectDetail.renameSaved').replace('{name}', name))
  }

  const moveProject = async (event: React.FormEvent) => {
    event.preventDefault()
    const groupID = moveValue ? Number(moveValue) : null
    const currentGroupID = project.group_id ?? null
    if (groupID === currentGroupID) {
      setDialog(null)
      return
    }
    const destination = groups.find(group => group.id === groupID)?.name || t('projectGroups.ungrouped')
    await persist('move', { group_id: groupID }, t('projectDetail.moveSaved').replace('{name}', project.name).replace('{group}', destination))
  }

  const toggleProject = async () => {
    if (projectEnabled) {
      const confirmed = await dialogs.confirm(
        t('projectDetail.disableConfirm').replace('{name}', project.name),
        { title: t('projectDetail.disableTitle').replace('{name}', project.name), action: t('projectDetail.disableProject') },
      )
      if (!confirmed) return
    }
    setBusyAction('toggle')
    try {
      const enabled = !projectEnabled
      const updated = await api.updateProject(projectId, projectUpdatePayload(project, { enabled }))
      onProjectUpdated(updated || { ...project, enabled })
      dialogs.notify(
        t(enabled ? 'projectDetail.enableSaved' : 'projectDetail.disableSaved').replace('{name}', project.name),
        'success',
      )
    } catch (reason: any) {
      dialogs.notify(reason.message || t('projectDetail.updateFailed'))
    } finally {
      setBusyAction(null)
    }
  }

  const closeDialog = () => setDialog(null)
  const pipelineSourceMode = project.pipeline_source_mode === 'scm' ? 'scm' : 'inline'
  const pipelineSource = pipelineSourceMode === 'scm'
    ? [project.pipeline_scm_repo || project.repo_url, project.pipeline_scm_branch || project.default_branch, project.pipeline_scm_path].filter(Boolean).join('\n')
    : project.config || t('projectDetail.noPipelineSource')

  return <>
    <nav className="jenkins-job-actions">
      <Link className={activeView === 'status' ? 'active' : undefined} to={`/projects/${projectId}`}><Activity size={16} />{t('projectDetail.status')}</Link>
      <Link className={activeView === 'changes' ? 'active' : undefined} to={`/projects/${projectId}/changes`}><GitCommitHorizontal size={16} />{t('projectDetail.changes')}</Link>
      {editable && <button type="button" onClick={onBuildNow} disabled={building || !projectEnabled || busyAction !== null} aria-busy={building || undefined} title={!projectEnabled ? t('common.disabled') : undefined}>{building ? <LoaderCircle className="timeline-spinner" size={16} /> : <Play size={16} />}{t(parameterized ? 'builds.buildWithParameters' : 'projectDetail.buildNow')}</button>}
      {editable && <button type="button" className="danger" onClick={onDelete} disabled={deleting || busyAction !== null} aria-busy={deleting || undefined}>{deleting ? <LoaderCircle className="timeline-spinner" size={16} /> : <Trash2 size={16} />}{t('projectDetail.deletePipeline')}</button>}
      {editable && <Link to={`/projects/${projectId}/configure`}><Settings2 size={16} />{t('projectDetail.configure')}</Link>}
      {editable && <button type="button" onClick={openRename} disabled={busyAction !== null}><Pencil size={16} />{t('projectDetail.rename')}</button>}
      {editable && <button type="button" onClick={openMove} disabled={busyAction !== null}><Move size={16} />{t('projectDetail.move')}</button>}
      <Link to={`/projects/${projectId}/configure#jenkins-configure-pipeline`}><Blocks size={16} />{t('projectDetail.stages')}</Link>
      <button type="button" onClick={() => setDialog('syntax')}><Code2 size={16} />{t('projectDetail.pipelineSyntax')}</button>
      {editable && <button type="button" onClick={toggleProject} disabled={busyAction !== null || deleting} aria-busy={busyAction === 'toggle' || undefined}><Power size={16} />{busyAction === 'toggle' ? t('common.loading') : t(projectEnabled ? 'projectDetail.disableProject' : 'projectDetail.enableProject')}</button>}
      <PluginActionLinks location="project.action" projectId={projectId} />
    </nav>

    {dialog === 'rename' && <ModalDialog className="jenkins-project-action-dialog" ariaLabel={t('projectDetail.renameTitle')} busy={busyAction === 'rename'} closeOnBackdrop onClose={closeDialog}>
      <header><div><Pencil size={18} /><div><h2>{t('projectDetail.renameTitle')}</h2><p>{t('projectDetail.renameDescription').replace('{name}', project.name)}</p></div></div><button type="button" onClick={closeDialog} disabled={busyAction === 'rename'} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button></header>
      <form onSubmit={renameProject}>
        <div className="jenkins-project-action-body"><label>{t('projectDetail.projectName')}<input data-dialog-initial-focus required maxLength={160} value={renameValue} onChange={event => setRenameValue(event.target.value)} autoComplete="off" /></label></div>
        <footer><button type="button" disabled={busyAction === 'rename'} onClick={closeDialog}>{t('common.cancel')}</button><button type="submit" disabled={busyAction === 'rename' || !renameValue.trim()}>{busyAction === 'rename' ? <LoaderCircle className="timeline-spinner" size={14} /> : null}{busyAction === 'rename' ? t('common.loading') : t('projectDetail.rename')}</button></footer>
      </form>
    </ModalDialog>}

    {dialog === 'move' && <ModalDialog className="jenkins-project-action-dialog" ariaLabel={t('projectDetail.moveTitle')} busy={busyAction === 'move'} closeOnBackdrop onClose={closeDialog}>
      <header><div><Move size={18} /><div><h2>{t('projectDetail.moveTitle')}</h2><p>{t('projectDetail.moveDescription').replace('{name}', project.name)}</p></div></div><button type="button" onClick={closeDialog} disabled={busyAction === 'move'} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button></header>
      <form onSubmit={moveProject}>
        <div className="jenkins-project-action-body"><label>{t('projectDetail.destinationGroup')}<select data-dialog-initial-focus value={moveValue} onChange={event => setMoveValue(event.target.value)}><option value="">{t('projectGroups.ungrouped')}</option>{groups.map(group => <option key={group.id} value={group.id}>{group.name}</option>)}</select></label></div>
        <footer><button type="button" disabled={busyAction === 'move'} onClick={closeDialog}>{t('common.cancel')}</button><button type="submit" disabled={busyAction === 'move'}>{busyAction === 'move' ? <LoaderCircle className="timeline-spinner" size={14} /> : null}{busyAction === 'move' ? t('common.loading') : t('projectDetail.move')}</button></footer>
      </form>
    </ModalDialog>}

    {dialog === 'syntax' && <ModalDialog className="jenkins-project-action-dialog jenkins-pipeline-syntax-dialog" ariaLabel={t('projectDetail.pipelineSyntax')} closeOnBackdrop onClose={closeDialog}>
      <header><div><Code2 size={18} /><div><h2>{t('projectDetail.pipelineSyntax')}</h2><p>{t('projectDetail.pipelineSyntaxDescription').replace('{name}', project.name)}</p></div></div><button type="button" onClick={closeDialog} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button></header>
      <div className="jenkins-project-action-body">
        <dl><div><dt>{t('projectDetail.pipelineFormat')}</dt><dd>{project.pipeline_format || 'auto'}</dd></div><div><dt>{t('projectDetail.pipelineSource')}</dt><dd>{t(pipelineSourceMode === 'scm' ? 'projectDetail.pipelineScriptFromSCM' : 'projectDetail.pipelineScript')}</dd></div></dl>
        <pre tabIndex={0} aria-label={t('projectDetail.pipelineSource')}><code>{pipelineSource}</code></pre>
      </div>
      <footer><button type="button" onClick={closeDialog}>{t('common.close')}</button><button type="button" onClick={() => { closeDialog(); navigate(`/projects/${projectId}/configure#jenkins-configure-pipeline`) }}>{t('projectDetail.openPipelineEditor')}</button></footer>
    </ModalDialog>}
  </>
}
