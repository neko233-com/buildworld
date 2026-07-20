import { useEffect, useSyncExternalStore } from 'react'
import { AlertTriangle, CheckCircle2, Info, X } from 'lucide-react'
import { API_FEEDBACK_EVENT, type APIFeedbackDetail } from '../api'
import { useI18n } from '../i18n'
import { ModalDialog } from './ModalDialog'

type Notice = {
  id: number
  message: string
  tone: 'error' | 'success' | 'info'
  operationId?: string
  source: 'api' | 'manual'
  createdAt: number
}
type Confirmation = { message: string; title: string; action: string; resolve: (value: boolean) => void }

let nextNoticeID = 1
let notices: Notice[] = []
let confirmation: Confirmation | null = null
let currentState: { notices: Notice[]; confirmation: Confirmation | null } = { notices, confirmation }
const listeners = new Set<() => void>()

function emit() {
  currentState = { notices, confirmation }
  listeners.forEach(listener => listener())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

function snapshot() {
  return currentState
}

export const dialogs = {
  notify(message: string, tone: Notice['tone'] = 'error', options: { operationId?: string; source?: Notice['source'] } = {}) {
    const source = options.source || 'manual'
    if (options.operationId) {
      notices = notices.filter(notice => notice.operationId !== options.operationId)
    } else if (source === 'manual') {
      const apiNotice = [...notices].reverse().find(notice =>
        notice.source === 'api' && notice.tone === tone && Date.now() - notice.createdAt < 1000,
      )
      if (apiNotice) notices = notices.filter(notice => notice.id !== apiNotice.id)
    }
    const id = nextNoticeID++
    notices = [...notices, { id, message, tone, operationId: options.operationId, source, createdAt: Date.now() }]
    emit()
    window.setTimeout(() => {
      notices = notices.filter(notice => notice.id !== id)
      emit()
    }, 6000)
  },
  confirm(message: string, options: { title?: string; action?: string } = {}) {
    return new Promise<boolean>(resolve => {
      confirmation = { message, resolve, title: options.title || 'Confirm action', action: options.action || 'Confirm' }
      emit()
    })
  },
}

export function AppDialogs() {
  const { t } = useI18n()
  const state = useSyncExternalStore(subscribe, snapshot, snapshot)
  const activeConfirmation = state.confirmation
  useEffect(() => {
    const handleAPIFeedback = (event: Event) => {
      const detail = (event as CustomEvent<APIFeedbackDetail>).detail
      if (!detail?.message || !detail.operationId) return
      dialogs.notify(detail.message, detail.tone, { operationId: detail.operationId, source: 'api' })
    }
    window.addEventListener(API_FEEDBACK_EVENT, handleAPIFeedback)
    return () => window.removeEventListener(API_FEEDBACK_EVENT, handleAPIFeedback)
  }, [])
  const dismiss = (id: number) => {
    notices = notices.filter(notice => notice.id !== id)
    emit()
  }
  const resolveConfirmation = (accepted: boolean) => {
    const current = confirmation
    confirmation = null
    emit()
    current?.resolve(accepted)
  }
  return <>
    <aside className="app-notices" aria-live="polite" aria-label={t('common.notifications')}>
      {state.notices.map(notice => <div className={`app-notice ${notice.tone}`} role={notice.tone === 'error' ? 'alert' : 'status'} key={notice.id}><span>{notice.tone === 'error' ? <AlertTriangle size={16} /> : notice.tone === 'success' ? <CheckCircle2 size={16} /> : <Info size={16} />}</span><p>{notice.message}</p><button onClick={() => dismiss(notice.id)} title={t('common.dismiss')} aria-label={t('common.dismiss')}><X size={15} /></button></div>)}
    </aside>
    {activeConfirmation && <ModalDialog className="app-confirm-dialog" ariaLabel={activeConfirmation.title} closeOnBackdrop onClose={() => resolveConfirmation(false)}><header><div><AlertTriangle size={18} /><div><h2>{activeConfirmation.title}</h2><p>{activeConfirmation.message}</p></div></div><button onClick={() => resolveConfirmation(false)} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button></header><footer><button data-dialog-cancel data-dialog-initial-focus onClick={() => resolveConfirmation(false)}>{t('common.cancel')}</button><button className="danger-action" onClick={() => resolveConfirmation(true)}>{activeConfirmation.action}</button></footer></ModalDialog>}
  </>
}
