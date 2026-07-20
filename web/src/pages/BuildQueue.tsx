import { useEffect, useState } from 'react'
import { motion } from 'motion/react'
import { Activity, ArrowDown, ArrowUp, ChevronsDown, ChevronsUp, Clock3, FileClock, GitBranch, Hourglass, RotateCw, XCircle } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { buildStatusLabel, buildTriggerLabel } from '../lib/buildPresentation'
import { canEdit } from '../authz'
import { PageState } from '../components/PageState'
import BuildApprovalPanel from '../components/BuildApprovalPanel'
import { canMoveQueueItem, moveQueueItem, queueWaitReasonKey, type QueueMoveOperation } from '../lib/queuePresentation'

export default function BuildQueue() {
  const { t } = useI18n()
  const editable = canEdit()
  const { data, loading, error, reload, setData } = useApi(async () => {
    const [queue, approvals] = await Promise.all([api.listBuildQueue(), api.listPendingApprovals()])
    return { queue, approvals }
  }, [])
  const [refreshing, setRefreshing] = useState(false)
  const [moving, setMoving] = useState<number | null>(null)
  const [moveMessage, setMoveMessage] = useState('')

  const list = data?.queue || []
  const approvals = data?.approvals || []
  const running = list.filter((item: any) => item.status === 'running')
  const waiting = list.filter((item: any) => item.status !== 'running')
  const dispatchWaiting = list.filter((item: any) => item.status === 'queued').length
  const approvalWaiting = list.filter((item: any) => item.status === 'pending_approval').length
  const hasActiveQueue = list.some((item: any) => item.status === 'queued' || item.status === 'running' || item.status === 'pending_approval')
  useEffect(() => {
    const poll = () => {
      if (document.visibilityState === 'visible') reload()
    }
    const timer = window.setInterval(poll, hasActiveQueue ? 3000 : 5000)
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
    if (!await dialogs.confirm(t('buildQueue.cancelConfirm'), { title: t('buildQueue.cancelTitle'), action: t('buildQueue.cancelBuild') })) return
    try { await api.stopBuild(item.build_id); reload() }
    catch (e: any) { dialogs.notify(e.message) }
  }

  const refresh = () => {
    setRefreshing(true)
    reload()
    window.setTimeout(() => setRefreshing(false), 450)
  }

  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  return (
    <motion.section className="operations-page" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
      <header className="operations-heading">
        <div><p>{waiting.length}</p><h1>{t('buildQueue.title')}</h1></div>
        <button type="button" className="secondary-command" onClick={refresh} disabled={refreshing}><RotateCw className={refreshing ? 'timeline-spinner' : ''} size={15} />{t('buildQueue.refresh')}</button>
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
          <span className="queue-running-pulse" />
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
              <tr key={q.id}>
                <td><Link className="queue-position build-number-link" to={`/builds/${q.build_id}`}>#{q.queue_position}</Link></td>
                <td><Link className="entity-text-link queue-project-link" to={`/builds/${q.build_id}`}><span>{q.project_name || `#${q.project_id}`}</span><small>#{q.build_number}</small></Link></td>
                <td><span className={`build-status ${q.status === 'running' ? 'running' : 'pending'}`}>{buildStatusLabel(t, q.status === 'queued' ? 'pending' : q.status)}</span></td>
                <td><span className={`queue-wait-reason ${q.wait_reason || 'dispatch'}`}><Hourglass size={13} />{t(queueWaitReasonKey(q.wait_reason))}{q.waiting_for_build_id && <Link to={`/builds/${q.waiting_for_build_id}`}>#{q.waiting_for_build_number || q.waiting_for_build_id}</Link>}</span></td>
                <td><span className="branch-cell"><GitBranch size={13} />{q.branch || '-'}</span></td>
                <td className="muted-cell">{buildTriggerLabel(t, q.trigger)}</td>
                <td className="muted-cell"><span className="queue-time"><Clock3 size={13} />{q.queued_at ? new Date(q.queued_at).toLocaleString() : '-'}</span></td>
                <td className="muted-cell">{q.priority ?? 0}</td>
                <td>
                  <div className="row-actions">{editable && <>
                    <button type="button" className="row-icon" onClick={() => move(q.id, 'move_top')} disabled={moving !== null || !canMoveQueueItem(list, q.id, 'move_top')} title={t('buildQueue.moveTop')} aria-label={`${t('buildQueue.moveTop')} ${q.project_name} #${q.build_number}`}><ChevronsUp size={15} /></button>
                    <button type="button" className="row-icon" onClick={() => move(q.id, 'move_up')} disabled={moving !== null || !canMoveQueueItem(list, q.id, 'move_up')} title={t('buildQueue.moveUp')} aria-label={`${t('buildQueue.moveUp')} ${q.project_name} #${q.build_number}`}><ArrowUp size={15} /></button>
                    <button type="button" className="row-icon" onClick={() => move(q.id, 'move_down')} disabled={moving !== null || !canMoveQueueItem(list, q.id, 'move_down')} title={t('buildQueue.moveDown')} aria-label={`${t('buildQueue.moveDown')} ${q.project_name} #${q.build_number}`}><ArrowDown size={15} /></button>
                    <button type="button" className="row-icon" onClick={() => move(q.id, 'move_bottom')} disabled={moving !== null || !canMoveQueueItem(list, q.id, 'move_bottom')} title={t('buildQueue.moveBottom')} aria-label={`${t('buildQueue.moveBottom')} ${q.project_name} #${q.build_number}`}><ChevronsDown size={15} /></button>
                    <button type="button" className="row-icon danger" onClick={() => cancel(q.id)} title={t('buildQueue.cancelBuild')} aria-label={t('buildQueue.cancelBuild')}><XCircle size={15} /></button>
                  </>}</div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </motion.section>
  )
}
