import { useEffect, useState } from 'react'
import { motion } from 'motion/react'
import { Activity, ChevronLeft, ChevronRight, FileClock, FileText, FolderTree, LayoutDashboard, Pin, PinOff, RotateCcw, Search, SlidersHorizontal, X } from 'lucide-react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { buildStatusLabel, buildStatusTone, buildTriggerLabel } from '../lib/buildPresentation'
import { canEdit } from '../authz'
import { PageState } from '../components/PageState'
import { formatDuration } from '../lib/durationPresentation'
import { activeBuildFilterCount, readBuildSearchParams } from '../lib/buildSearch'
import JenkinsPageShell from '../components/JenkinsPageShell'

const buildStatuses = ['', 'running', 'failed', 'success', 'pending', 'pending_approval', 'cancelled', 'rejected']
const buildTriggers = ['', 'manual', 'retry', 'webhook', 'schedule', 'http', 'api']
const ACTIVE_BUILD_STATUSES = new Set(['running', 'pending', 'pending_approval', 'queued'])

export default function Builds() {
  const { t } = useI18n()
  const editable = canEdit()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const serializedParams = searchParams.toString()
  const filters = readBuildSearchParams(searchParams)
  const { data: result, loading, error, reload } = useApi(() => api.searchBuilds(filters), [serializedParams])
  const { data: projects } = useApi(() => api.listProjects())
  const [pinning, setPinning] = useState<number | null>(null)
  const [retrying, setRetrying] = useState<number | null>(null)
  const [queryDraft, setQueryDraft] = useState(filters.q)
  const [branchDraft, setBranchDraft] = useState(filters.branch)
  const projectMap = new Map((projects || []).map(project => [project.id, project.name]))
  const totalPages = Math.max(1, Math.ceil((result?.total || 0) / filters.limit))
  const filterCount = activeBuildFilterCount(filters)
  const hasActiveBuilds = (result?.items || []).some(build => ACTIVE_BUILD_STATUSES.has(build.status))

  useEffect(() => {
    setQueryDraft(filters.q)
    setBranchDraft(filters.branch)
  }, [filters.branch, filters.q])

  useEffect(() => {
    if (!result || filters.page <= totalPages) return
    const next = new URLSearchParams(searchParams)
    if (totalPages > 1) next.set('page', String(totalPages))
    else next.delete('page')
    setSearchParams(next, { replace: true })
  }, [filters.page, result, searchParams, setSearchParams, totalPages])

  useEffect(() => {
    if (!hasActiveBuilds) return
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') reload()
    }, 2000)
    return () => window.clearInterval(timer)
  }, [hasActiveBuilds, reload])

  const updateFilters = (updates: Record<string, string | number | boolean>) => {
    const next = new URLSearchParams(searchParams)
    for (const [key, value] of Object.entries(updates)) {
      if (value === '' || value === 0 || value === false) next.delete(key)
      else next.set(key, value === true ? '1' : String(value))
    }
    next.delete('page')
    setSearchParams(next)
  }

  const applyTextFilters = (event: React.FormEvent) => {
    event.preventDefault()
    updateFilters({ q: queryDraft.trim(), branch: branchDraft.trim() })
  }

  const changePage = (page: number) => {
    const next = new URLSearchParams(searchParams)
    if (page <= 1) next.delete('page')
    else next.set('page', String(page))
    setSearchParams(next)
  }

  const handleRetry = async (buildId: number) => {
    setRetrying(buildId)
    try {
      const retried = await api.retryBuild(buildId)
      navigate(`/builds/${retried.id}`)
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    } finally {
      setRetrying(null)
    }
  }

  const handlePin = async (buildId: number, currentPinned: boolean) => {
    setPinning(buildId)
    try {
      await api.pinBuild(buildId, !currentPinned)
      reload()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('common.error'))
    } finally {
      setPinning(null)
    }
  }

  if (loading && !result) return <PageState />
  if (error && !result) return <PageState error={error} onRetry={reload} />

  const list = result?.items || []
  const firstItem = result?.total ? result.offset + 1 : 0
  const lastItem = result ? Math.min(result.offset + result.items.length, result.total) : 0

  return <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.18 }}>
    <JenkinsPageShell
      className="jenkins-build-history-page"
      breadcrumbs={[{ label: t('builds.title') }]}
      sidepanelLabel={t('builds.title')}
      sidepanel={<nav className="jenkins-context-task-list">
        <Link to="/"><LayoutDashboard size={20} />{t('nav.dashboard')}</Link>
        <Link to="/projects"><FolderTree size={20} />{t('nav.projects')}</Link>
        <Link to="/build-queue"><FileClock size={20} />{t('nav.buildQueue')}</Link>
        <Link to="/builds" className="active" aria-current="page"><Activity size={20} />{t('nav.builds')}</Link>
        {filterCount > 0 && <button type="button" onClick={() => { setQueryDraft(''); setBranchDraft(''); setSearchParams({}) }}><X size={20} />{t('builds.clearFilters')}</button>}
      </nav>}
    >
    <div className="operations-page build-history-page">
    <header className="jenkins-page-heading">
      <div><p>{result?.total || 0}</p><h1>{t('builds.title')}</h1></div>
      <div className="build-history-summary"><SlidersHorizontal size={15} /><span>{filterCount ? t('builds.activeFilters').replace('{count}', String(filterCount)) : t('builds.allBuilds')}</span></div>
    </header>

    <section className="build-history-filters" aria-label={t('builds.filters')}>
      <form onSubmit={applyTextFilters}>
        <label className="build-search-input"><Search size={15} /><input value={queryDraft} onChange={event => setQueryDraft(event.target.value)} placeholder={t('builds.searchPlaceholder')} aria-label={t('builds.searchBuilds')} /></label>
        <label><span>{t('builds.branch')}</span><input value={branchDraft} onChange={event => setBranchDraft(event.target.value)} placeholder={t('builds.anyBranch')} aria-label={t('builds.filterBranch')} /></label>
        <button type="submit" className="primary-command">{t('builds.applyFilters')}</button>
      </form>
      <div className="build-filter-row">
        <label><span>{t('builds.project')}</span><select aria-label={t('builds.filterProject')} value={filters.projectId} onChange={event => updateFilters({ project: Number(event.target.value) })}><option value={0}>{t('builds.allProjects')}</option>{(projects || []).map(project => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label>
        <label><span>{t('builds.status')}</span><select aria-label={t('builds.filterStatus')} value={filters.status} onChange={event => updateFilters({ status: event.target.value })}>{buildStatuses.map(status => <option key={status || 'all'} value={status}>{status ? buildStatusLabel(t, status) : t('builds.allStatuses')}</option>)}</select></label>
        <label><span>{t('builds.trigger')}</span><select aria-label={t('builds.filterTrigger')} value={filters.trigger} onChange={event => updateFilters({ trigger: event.target.value })}>{buildTriggers.map(trigger => <option key={trigger || 'all'} value={trigger}>{trigger ? buildTriggerLabel(t, trigger) : t('builds.allTriggers')}</option>)}</select></label>
        <button type="button" className={`pinned-filter ${filters.pinned ? 'active' : ''}`} aria-pressed={filters.pinned} onClick={() => updateFilters({ pinned: !filters.pinned })}><Pin size={14} />{t('builds.pinnedOnly')}</button>
        {filterCount > 0 && <button type="button" className="clear-build-filters" onClick={() => { setQueryDraft(''); setBranchDraft(''); setSearchParams({}) }}><X size={14} />{t('builds.clearFilters')}</button>}
      </div>
    </section>

    {error && <div className="build-history-inline-error" role="alert"><span>{error}</span><button type="button" onClick={reload}>{t('common.retry')}</button></div>}
    <section className="operations-table-wrap build-history-results" aria-busy={loading}>
      <table className="operations-table builds-table">
        <thead><tr><th>{t('builds.buildNumber')}</th><th>{t('builds.project')}</th><th>{t('builds.status')}</th><th>{t('builds.trigger')}</th><th>{t('builds.branch')}</th><th>{t('builds.commit')}</th><th>{t('builds.duration')}</th><th aria-label={t('projects.actions')} /></tr></thead>
        <tbody>
          {!list.length && <tr><td colSpan={8} className="operations-empty"><FileText size={18} />{filterCount ? t('builds.noMatchingBuilds') : t('builds.noBuilds')}</td></tr>}
          {list.map(build => <tr key={build.id}>
            <td><Link className="build-number build-number-link" to={`/builds/${build.id}`}>{build.pinned && <Pin size={13} />}#{build.number}</Link></td>
            <td><Link className="entity-text-link" to={`/projects/${build.project_id}`}>{projectMap.get(build.project_id) || `#${build.project_id}`}</Link></td>
            <td><span className={`build-status ${buildStatusTone(build.status)}`}>{buildStatusLabel(t, build.status)}</span></td>
            <td className="muted-cell">{buildTriggerLabel(t, build.trigger)}</td>
            <td className="branch-cell">{build.branch || '-'}</td>
            <td><code>{build.commit_sha?.slice(0, 8) || '-'}</code></td>
            <td className="muted-cell">{formatDuration(build.duration_ms)}</td>
            <td><div className="row-actions">{editable && <><button type="button" className="row-icon" disabled={retrying === build.id || ['running', 'pending', 'pending_approval'].includes(build.status)} title={t('builds.retryBuild')} aria-label={`${t('builds.retryBuild')} #${build.number}`} onClick={() => handleRetry(build.id)}><RotateCcw size={15} /></button><button type="button" className={`row-icon ${build.pinned ? 'selected' : ''}`} disabled={pinning === build.id} title={build.pinned ? t('builds.unpin') : t('builds.pin')} aria-label={`${build.pinned ? t('builds.unpin') : t('builds.pin')} #${build.number}`} onClick={() => handlePin(build.id, !!build.pinned)}>{build.pinned ? <PinOff size={15} /> : <Pin size={15} />}</button></>}<Link className="row-icon" title={t('builds.openBuild')} aria-label={`${t('builds.openBuild')} #${build.number}`} to={`/builds/${build.id}`}><FileText size={15} /></Link></div></td>
          </tr>)}
        </tbody>
      </table>
      <footer className="build-history-pagination">
        <span>{t('builds.resultRange').replace('{first}', String(firstItem)).replace('{last}', String(lastItem)).replace('{total}', String(result?.total || 0))}</span>
        <div>
          <button type="button" onClick={() => changePage(filters.page - 1)} disabled={filters.page <= 1} aria-label={t('builds.previousPage')}><ChevronLeft size={15} /></button>
          <strong>{t('builds.pageOf').replace('{page}', String(filters.page)).replace('{total}', String(totalPages))}</strong>
          <button type="button" onClick={() => changePage(filters.page + 1)} disabled={filters.page >= totalPages} aria-label={t('builds.nextPage')}><ChevronRight size={15} /></button>
        </div>
      </footer>
    </section>
    </div>
    </JenkinsPageShell>
  </motion.div>
}
