import { useEffect, useState } from 'react'
import { motion } from 'motion/react'
import { Activity, ArrowDown, ArrowUp, ChevronsDown, ChevronsUp, Clock3, FileClock, FolderTree, GitBranch, History, Hourglass, LayoutDashboard, LoaderCircle, RotateCw, ServerCog, XCircle } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { buildStatusLabel, buildStatusTone, buildTriggerLabel } from '../lib/buildPresentation'
import { canEdit } from '../authz'
import { PageState } from '../components/PageState'
import BuildApprovalPanel from '../components/BuildApprovalPanel'
import { canMoveQueueItem, moveQueueItem, queueWaitReasonKey, type QueueMoveOperation } from '../lib/queuePresentation'
import JenkinsPageShell from '../components/JenkinsPageShell'

export default function BuildQueue() {
  const { t } = useI18n()
  const editable = canEdit()
  const { data, loading, error, reload, setData } = useApi(async () => {
    const [queue, approvals] = await Promise.all([api.listBuildQueue(), api.listPendingApprovals()])
    return { queue, approvals }
  }, [])
  const [refreshing, setRefreshing] = useState(false)
  const [moving, setMoving] = useState<number | null>(null)
  const [cancelling, setCancelling] = useState<number | null>(null)
  const [moveMessage, setMoveMessage] = useState('')

  const list = data?.queue || []
  const approvals = data?.approvals || []
  const running = list.filter((item: any) => item.status === 'running')
  const waiting = list.filter((item: any) => item.status !== 'running')
  const dispatchWaiting = list.filter((item: any) => item.status === 'queued').length
  const approvalWaiting = list.filter((item: any) => item.status === 'pending_approval').length
  const hasActiveQueue = list.some((item: any) => ['running', 'pending', 'pending_approval', 'queued'].includes(item.status))
  useEffect(() => {
    if (!hasActiveQueue) return
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') reload()
    }, 2000)
    return () => window.clearInterval(timer)
  }, [hasActiveQueue, reload])

  const move = async (id: number, operation: QueueMoveOperation) => {
    if (!canMoveQueueItem(list, id, operation)) return
    const previous = list
    const optimistic = moveQueueItem(list, id, operation)
    setMoving(id)
    setMoveMessage('')
    setData({ queue: optimistic, approvals })
    try {
      const result = await api.reorderBuildQueue(id, operation)
      setData({ queue: result.items, approvals })
      setMoveMessage(t('buildQueue.moved'))
    } catch (e: any) {
      setData({ queue: previous, approvals })
      dialogs.notify(e.message)
    } finally {
      setMoving(null)
    }
  }

  const cancel = async (id: number) => {
    const item = list.find((queueItem: any) => queueItem.id === id)
    if (!item) return
    const buildLabel = `${item.project_name || `#${item.project_id}`} #${item.build_number}`
    if (!await dialogs.confirm(`${buildLabel}\n\n${t('buildQueue.cancelConfirm')}`, { title: t('buildQueue.cancelTitle'), action: t('buildQueue.cancelBuild') })) return
    setCancelling(id)
    try { await api.stopBuild(item.build_id); reload() }
    catch (e: any) { dialogs.notify(e.message) }
    finally { setCancelling(null) }
  }

  const refresh = async () => {
    if (refreshing) return
    setRefreshing(true)
    try {
      const [queue, nextApprovals] = await Promise.all([api.listBuildQueue(), api.listPendingApprovals()])
      setData({ queue, approvals: nextApprovals })
    } catch (e: any) {
      dialogs.notify(e.message || t('common.error'))
    } finally {
      setRefreshing(false)
    }
  }

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.18 }}>
      <JenkinsPageShell
        className="jenkins-build-queue-page"
        breadcrumbs={[{ label: t('buildQueue.title') }]}
        sidepanelLabel={t('buildQueue.title')}
        sidepanel={<nav className="jenkins-context-task-list">
          <Link to="/"><LayoutDashboard size={20} />{t('nav.dashboard')}</Link>
          <Link to="/projects"><FolderTree size={20} />{t('nav.projects')}</Link>
          <Link to="/builds"><History size={20} />{t('nav.builds')}</Link>
          <Link to="/build-queue" className="active" aria-current="page"><FileClock size={20} />{t('nav.buildQueue')}</Link>
          <Link to="/agents"><ServerCog size={20} />{t('nav.agents')}</Link>
          <button type="button" onClick={refresh} disabled={refreshing} aria-busy={refreshing}><RotateCw className={refreshing ? 'timeline-spinner' : ''} size={20} />{t('buildQueue.refresh')}</button>
        </nav>}
      >
      <div className="operations-page jenkins-queue-content">
      <header className="jenkins-page-heading">
        <div><p className="jenkins-page-heading-count">{waiting.length}</p><h1>{t('buildQueue.title')}</h1></div>
        <button type="button" className="secondary-command" onClick={refresh} disabled={refreshing} aria-busy={refreshing}><RotateCw className={refreshing ? 'timeline-spinner' : ''} size={15} />{t('buildQueue.refresh')}</button>
      </header>
      <div className="queue-summary" aria-label={t('buildQueue.summary')}>
        <span><Activity size={14} />{t('buildQueue.runningSummary').replace('{count}', String(running.length))}</span>
        <span><Hourglass size={14} />{t('buildQueue.dispatchSummary').replace('{count}', String(dispatchWaiting))}</span>
        <span>{t('buildQueue.approvalSummary').replace('{count}', String(approvalWaiting))}</span>
      </div>
      <p className="sr-only" role="status" aria-live="polite">{moveMessage}</p>
      {running.length > 0 && <section className="queue-running-section" aria-label={t('buildQueue.runningNow')}>
        <header><div><Activity size={16} /><strong>{t('buildQueue.runningNow')}</strong></div><span>{running.length}</span></header>
        <div>{running.map((item: any) => <Link key={item.id} to={`/builds/${item.build_id}`}>
          <LoaderCircle className="timeline-spinner" size={18} aria-hidden="true" />
          <span><strong>{item.project_name || `#${item.project_id}`} <b>#{item.build_number}</b></strong><small>{item.branch || '-'} · {buildTriggerLabel(t, item.trigger)}</small></span>
          <em>{t('buildQueue.reasonRunning')}</em>
        </Link>)}</div>
      </section>}
      {approvals.length > 0 && <section className="approval-inbox">
        <header><div><strong>{t('approvals.inbox')}</strong><p>{t('approvals.inboxHelp')}</p></div><span>{approvals.length}</span></header>
        <div>{approvals.map((approval: any) => <article key={approval.id}><div className="approval-build-link"><Link to={`/builds/${approval.build_id}`}>{approval.project_name} <strong>#{approval.build_number}</strong></Link><small>{approval.branch || '-'} · {buildTriggerLabel(t, approval.trigger)}</small></div><BuildApprovalPanel compact buildId={approval.build_id} approval={approval} onResolved={reload} /></article>)}</div>
      </section>}
      <div className="operations-table-wrap build-queue-wrap">
        <table className="operations-table build-queue-table">
          <caption className="sr-only">{t('buildQueue.title')}</caption>
          <thead>
            <tr>
              <th>{t('buildQueue.position')}</th>
              <th>{t('buildQueue.project')}</th>
              <th>{t('buildQueue.status')}</th>
              <th>{t('buildQueue.waitReason')}</th>
              <th>{t('buildQueue.branch')}</th>
              <th>{t('buildQueue.trigger')}</th>
              <th>{t('buildQueue.queuedAt')}</th>
              <th>{t('buildQueue.priority')}</th>
              <th aria-label={t('projects.actions')} />
            </tr>
          </thead>
          <tbody>
            {waiting.length === 0 && (
              <tr><td colSpan={9} className="operations-empty queue-empty"><FileClock size={19} /><strong>{t(running.length ? 'buildQueue.noWaitingTitle' : 'buildQueue.emptyTitle')}</strong><small>{t(running.length ? 'buildQueue.noWaitingDescription' : 'buildQueue.emptyDescription')}</small></td></tr>
            )}
            {waiting.map((q: any) => (
              <tr key={q.id} aria-busy={moving === q.id || cancelling === q.id}>
                <td><Link className="queue-position build-number-link" to={`/builds/${q.build_id}`}>#{q.queue_position}</Link></td>
                <td><Link className="entity-text-link queue-project-link" to={`/builds/${q.build_id}`}><span>{q.project_name || `#${q.project_id}`}</span><small>#{q.build_number}</small></Link></td>
                <td><span className={`jenkins-build-state ${buildStatusTone(q.status === 'queued' ? 'pending' : q.status)}`}><i aria-hidden="true" /><span>{buildStatusLabel(t, q.status === 'queued' ? 'pending' : q.status)}</span></span></td>
                <td><span className={`queue-wait-reason ${q.wait_reason || 'dispatch'}`}><Hourglass size={13} />{t(queueWaitReasonKey(q.wait_reason))}{q.waiting_for_build_id && <Link to={`/builds/${q.waiting_for_build_id}`}>#{q.waiting_for_build_number || q.waiting_for_build_id}</Link>}</span></td>
                <td><span className="branch-cell"><GitBranch size={13} />{q.branch || '-'}</span></td>
                <td className="muted-cell">{buildTriggerLabel(t, q.trigger)}</td>
                <td className="muted-cell"><span className="queue-time"><Clock3 size={13} />{q.queued_at ? new Date(q.queued_at).toLocaleString() : '-'}</span></td>
                <td className="muted-cell">{q.priority ?? 0}</td>
                <td>
                  <div className="row-actions">{editable && <>
                    {moving === q.id && <LoaderCircle className="timeline-spinner jenkins-queue-moving" size={16} aria-hidden="true" />}
                    <button type="button" className="row-icon" onClick={() => move(q.id, 'move_top')} disabled={moving !== null || cancelling !== null || !canMoveQueueItem(list, q.id, 'move_top')} title={t('buildQueue.moveTop')} aria-label={`${t('buildQueue.moveTop')} ${q.project_name} #${q.build_number}`}><ChevronsUp size={16} /></button>
                    <button type="button" className="row-icon" onClick={() => move(q.id, 'move_up')} disabled={moving !== null || cancelling !== null || !canMoveQueueItem(list, q.id, 'move_up')} title={t('buildQueue.moveUp')} aria-label={`${t('buildQueue.moveUp')} ${q.project_name} #${q.build_number}`}><ArrowUp size={16} /></button>
                    <button type="button" className="row-icon" onClick={() => move(q.id, 'move_down')} disabled={moving !== null || cancelling !== null || !canMoveQueueItem(list, q.id, 'move_down')} title={t('buildQueue.moveDown')} aria-label={`${t('buildQueue.moveDown')} ${q.project_name} #${q.build_number}`}><ArrowDown size={16} /></button>
                    <button type="button" className="row-icon" onClick={() => move(q.id, 'move_bottom')} disabled={moving !== null || cancelling !== null || !canMoveQueueItem(list, q.id, 'move_bottom')} title={t('buildQueue.moveBottom')} aria-label={`${t('buildQueue.moveBottom')} ${q.project_name} #${q.build_number}`}><ChevronsDown size={16} /></button>
                    <button type="button" className="row-icon danger" disabled={moving !== null || cancelling !== null} aria-busy={cancelling === q.id} onClick={() => cancel(q.id)} title={t('buildQueue.cancelBuild')} aria-label={`${t('buildQueue.cancelBuild')} ${q.project_name || `#${q.project_id}`} #${q.build_number}`}>{cancelling === q.id ? <LoaderCircle className="timeline-spinner" size={16} /> : <XCircle size={16} />}</button>
                  </>}</div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      </div>
      </JenkinsPageShell>
    </motion.div>
  )
}
