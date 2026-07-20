import { Check, LoaderCircle, ShieldCheck, UserRound, X } from 'lucide-react'
import { useState } from 'react'
import { api } from '../api'
import { canEdit } from '../authz'
import { useI18n } from '../i18n'
import { dialogs } from './AppDialogs'

type Props = {
  buildId: number
  approval: any
  compact?: boolean
  onResolved: () => void
}

export default function BuildApprovalPanel({ buildId, approval, compact = false, onResolved }: Props) {
  const { t } = useI18n()
  const [comment, setComment] = useState('')
  const [resolving, setResolving] = useState<'approve' | 'reject' | null>(null)
  const editable = canEdit()

  const resolve = async (action: 'approve' | 'reject') => {
    if (action === 'reject' && !await dialogs.confirm(t('approvals.rejectConfirm'), { title: t('approvals.rejectTitle'), action: t('approvals.reject') })) return
    setResolving(action)
    try {
      if (action === 'approve') await api.approveBuild(buildId, comment)
      else await api.rejectBuild(buildId, comment)
      dialogs.notify(action === 'approve' ? t('approvals.approved') : t('approvals.rejected'), 'success')
      onResolved()
    } catch (reason: any) {
      dialogs.notify(reason.message || t('approvals.resolveFailed'))
    } finally {
      setResolving(null)
    }
  }

  return <section className={`build-approval-panel ${compact ? 'compact' : ''}`}>
    <header>
      <div className="approval-icon"><ShieldCheck size={18} /></div>
      <div>
        <h2>{t('approvals.waiting')}</h2>
        <p>{approval.prompt || t('approvals.waitingHelp')}</p>
      </div>
      <span>{t('approvals.required')}</span>
    </header>
    <div className="build-approval-context">
      <span><UserRound size={14} />{t('approvals.requestedBy')} <strong>{approval.username || t('approvals.automation')}</strong></span>
      {approval.required_roles?.length > 0 && <span>{t('approvals.allowedRoles')}: <strong>{approval.required_roles.map((role: string) => t(`users.role_${role}`)).join(', ')}</strong></span>}
      {approval.allow_requester === false && <span>{t('approvals.independentRequired')}</span>}
    </div>
    {editable && <div className="build-approval-actions">
      <input value={comment} onChange={event => setComment(event.target.value)} placeholder={t('approvals.commentPlaceholder')} aria-label={t('approvals.comment')} />
      <button type="button" className="approval-reject" onClick={() => resolve('reject')} disabled={resolving !== null}>{resolving === 'reject' ? <LoaderCircle className="timeline-spinner" size={15} /> : <X size={15} />}{t('approvals.reject')}</button>
      <button type="button" className="approval-approve" onClick={() => resolve('approve')} disabled={resolving !== null}>{resolving === 'approve' ? <LoaderCircle className="timeline-spinner" size={15} /> : <Check size={15} />}{t('approvals.approve')}</button>
    </div>}
  </section>
}
