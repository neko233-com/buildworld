import { CloudOff, LoaderCircle, RotateCw } from 'lucide-react'
import { useI18n } from '../i18n'

type PageStateProps = {
  error?: string | null
  onRetry?: () => void
}

export function PageState({ error, onRetry }: PageStateProps) {
  const { t } = useI18n()
  const failed = Boolean(error)

  return <section className={`page-state ${failed ? 'error' : 'loading'}`} role={failed ? 'alert' : 'status'} aria-live={failed ? 'assertive' : 'polite'}>
    <div className="page-state-card">
      <span className="page-state-icon" aria-hidden="true">
        {failed ? <CloudOff size={22} /> : <LoaderCircle className="timeline-spinner" size={22} />}
      </span>
      <div>
        <strong>{failed ? t('common.loadFailed') : t('common.loading')}</strong>
        {failed && <p>{error}</p>}
        {failed && onRetry && <button className="secondary-command" type="button" onClick={onRetry}><RotateCw size={14} />{t('common.retry')}</button>}
      </div>
    </div>
  </section>
}
