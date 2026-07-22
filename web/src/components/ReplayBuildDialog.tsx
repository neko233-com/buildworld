import { useEffect, useRef, useState } from 'react'
import { GitBranch, LoaderCircle, RotateCcw, SlidersHorizontal, X } from 'lucide-react'
import { api } from '../api'
import { useI18n } from '../i18n'
import { dialogs } from './AppDialogs'
import { ModalDialog } from './ModalDialog'
import './ReplayBuildDialog.css'

export type ReplayBuildTarget = {
  id: number
  number: number
  projectId: number
  projectName: string
  branch?: string
  parameters?: unknown
}

type ReplayBuildDialogProps = {
  target: ReplayBuildTarget
  loadDetails?: boolean
  onBusyChange?: (busy: boolean) => void
  onClose: () => void
  onReplayed: (buildId: number) => void
}

function parameterText(name: string, value: unknown): string {
  if (/(password|passwd|secret|token|key)/i.test(name)) return '••••••••'
  if (typeof value === 'string') return value
  if (value === undefined) return ''
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

export function replayParameterEntries(value: unknown): Array<[string, string]> {
  let parsed = value
  if (typeof value === 'string') {
    if (!value.trim()) return []
    try {
      parsed = JSON.parse(value)
    } catch {
      return []
    }
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return []
  return Object.entries(parsed).map(([name, parameter]) => [name, parameterText(name, parameter)])
}

export default function ReplayBuildDialog({
  target,
  loadDetails = false,
  onBusyChange,
  onClose,
  onReplayed,
}: ReplayBuildDialogProps) {
  const { t } = useI18n()
  const [details, setDetails] = useState(target)
  const [detailsRequest, setDetailsRequest] = useState(0)
  const [loadingDetails, setLoadingDetails] = useState(loadDetails)
  const [detailsError, setDetailsError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState('')
  const submittingRef = useRef(false)

  useEffect(() => {
    if (!loadDetails) return
    let cancelled = false
    setLoadingDetails(true)
    setDetailsError('')
    api.getBuild(target.id).then(build => {
      if (cancelled) return
      setDetails({
        ...target,
        branch: build?.branch || target.branch,
        parameters: build?.parameters,
      })
    }).catch((reason: any) => {
      if (!cancelled) setDetailsError(reason?.message || t('builds.replayDetailsFailed'))
    }).finally(() => {
      if (!cancelled) setLoadingDetails(false)
    })
    return () => { cancelled = true }
  }, [detailsRequest, loadDetails, t, target])

  const parameters = replayParameterEntries(details.parameters)
  const entity = `${details.projectName} #${details.number}`

  const handleReplay = async () => {
    if (submittingRef.current || loadingDetails || detailsError) return
    submittingRef.current = true
    setSubmitting(true)
    setSubmitError('')
    onBusyChange?.(true)
    try {
      const replayed = await api.retryBuild(target.id)
      const replayedID = Number(replayed?.id)
      if (!Number.isSafeInteger(replayedID) || replayedID <= 0) throw new Error(t('builds.replayFailed'))
      setSubmitting(false)
      submittingRef.current = false
      onBusyChange?.(false)
      dialogs.notify(t('builds.replayQueued').replace('{build}', entity), 'success')
      onReplayed(replayedID)
    } catch (reason: any) {
      setSubmitError(reason?.message || t('builds.replayFailed'))
      setSubmitting(false)
      submittingRef.current = false
      onBusyChange?.(false)
    }
  }

  return <ModalDialog
    ariaLabel={t('builds.replayTitle').replace('{build}', entity)}
    className="jenkins-replay-dialog"
    busy={submitting}
    closeOnBackdrop
    onClose={onClose}
  >
    <header>
      <div><RotateCcw size={20} /><div><h2>{t('builds.replay')}</h2><p>{t('builds.replayHelp').replace('{build}', entity)}</p></div></div>
      <button type="button" onClick={onClose} disabled={submitting} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button>
    </header>

    <div className="jenkins-replay-body">
      <section className="jenkins-replay-source" aria-label={t('builds.replaySource')}>
        <span>{t('builds.replaySource')}</span>
        <strong>{entity}</strong>
      </section>

      {loadingDetails
        ? <div className="jenkins-replay-state" role="status"><LoaderCircle className="timeline-spinner" size={18} />{t('builds.replayLoadingDetails')}</div>
        : detailsError
          ? <div className="jenkins-replay-state error" role="alert"><span>{detailsError}</span><button type="button" onClick={() => setDetailsRequest(value => value + 1)}>{t('common.retry')}</button></div>
          : <div className="jenkins-replay-summary">
              <section><div><GitBranch size={16} /><span>{t('builds.branch')}</span></div><strong>{details.branch || '-'}</strong></section>
              <section><div><SlidersHorizontal size={16} /><span>{t('builds.parameters')}</span></div>{parameters.length
                ? <dl>{parameters.map(([name, value]) => <div key={name}><dt>{name}</dt><dd><code>{value}</code></dd></div>)}</dl>
                : <p>{t('builds.replayNoParameters')}</p>}</section>
            </div>}

      {submitError && <div className="jenkins-replay-submit-error" role="alert">{submitError}</div>}
    </div>

    <footer>
      <button type="button" data-dialog-initial-focus onClick={onClose} disabled={submitting}>{t('common.cancel')}</button>
      <button type="button" className="jenkins-replay-submit" onClick={handleReplay} disabled={submitting || loadingDetails || !!detailsError} aria-busy={submitting}>
        {submitting ? <LoaderCircle className="timeline-spinner" size={16} /> : <RotateCcw size={16} />}
        {submitting ? t('builds.replaying') : t('builds.replay')}
      </button>
    </footer>
  </ModalDialog>
}
