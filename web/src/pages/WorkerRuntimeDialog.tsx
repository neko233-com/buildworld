import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type WorkerRuntimeView } from '../api'
import { useI18n } from '../i18n'
import { ModalDialog } from '../components/ModalDialog'
import { dialogs } from '../components/AppDialogs'
import { formatDateTime } from '../lib/dateTime'
import './WorkerRuntimeDialog.css'

export function WorkerRuntimeDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const { t } = useI18n()
  const [data, setData] = useState<WorkerRuntimeView | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [capacity, setCapacity] = useState(4)
  const [file, setFile] = useState<File | null>(null)
  const [version, setVersion] = useState('')
  const [digest, setDigest] = useState('')
  const uploadIdentity = useRef({ input: '', request: '' })
  const mounted = useRef(true)
  useEffect(() => {
    let active = true
    mounted.current = true
    let timer: number
    const refresh = async () => {
      if (!document.hidden) {
        try { const value = await api.workerRuntime(id); if (active) { setData(value); setError('') } }
        catch (reason) { if (active) setError((reason as Error).message) }
      }
      if (active) timer = window.setTimeout(refresh, 5000)
    }
    void refresh()
    return () => { active = false; mounted.current = false; window.clearTimeout(timer) }
  }, [id])
  const operate = async (kind: 'diagnose' | 'drain' | 'resume' | 'concurrency') => {
    setBusy(true); setError(''); setNotice('')
    try {
      const value = await api.workerOperation(id, { request_id: crypto.randomUUID(), kind, ...(kind === 'concurrency' ? { capacity } : {}) })
      if (mounted.current) setNotice(`${t('workerOps.queued')}: ${value.id}`)
    } catch (reason) { if (mounted.current) setError((reason as Error).message) }
    finally { if (mounted.current) setBusy(false) }
  }
  const upload = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!file || !await dialogs.confirm(t('workerOps.updateConfirm'), { title: t('workerOps.update'), action: t('workerOps.update') })) return
    const identity = `${file.name}/${file.size}/${file.lastModified}/${version}/${digest}`
    if (uploadIdentity.current.input !== identity) uploadIdentity.current = { input: identity, request: crypto.randomUUID() }
    setBusy(true); setError(''); setNotice('')
    try {
      const value = await api.uploadWorkerUpdate(id, file, version, digest, uploadIdentity.current.request)
      if (mounted.current) setNotice(`${t('workerOps.queued')}: ${value.id}`)
    } catch (reason) { if (mounted.current) setError((reason as Error).message) }
    finally { if (mounted.current) setBusy(false) }
  }
  const sample = data?.runtime
  return <ModalDialog ariaLabel={t('workerOps.title')} className="worker-runtime-dialog" busy={busy} onClose={onClose}>
    <header><div><h2>{t('workerOps.title')}</h2><p>{data?.worker.name || id}</p></div><button data-dialog-initial-focus disabled={busy} onClick={onClose}>{t('common.close')}</button></header>
    {error && <p className="form-error" role="alert">{error}</p>}
    {notice && <p className="worker-operation-notice" role="status">{notice}</p>}
    {!data && !error && <p>{t('common.loading')}</p>}
    {data && <>
      <p>{data.worker.status} · {data.draining ? t('workerOps.drained') : t('workerOps.accepting')} · {t('workerOps.observed')}: {formatDateTime(data.observed_at)}</p>
      {!sample?.version ? <p className="form-error">{t('workerOps.noReport')}</p> : <dl className="worker-runtime-facts">
        <div><dt>{t('workerOps.version')}</dt><dd>{sample.version} <code>{sample.source_commit || t('workerOps.noIdentity')}</code></dd></div>
        <div><dt>{t('workerOps.process')}</dt><dd>{sample.os}/{sample.arch} · PID {sample.pid} · Session {sample.session}</dd></div>
        <div><dt>{t('workerOps.capacity')}</dt><dd>{sample.tasks.length} / {sample.capacity} · {sample.goroutines} goroutines · {Math.round(sample.heap_bytes / 1048576)} MiB</dd></div>
      </dl>}
      <div className="worker-runtime-actions">
        <button disabled={busy || !sample?.version} onClick={() => operate('diagnose')}>{t('workerOps.diagnose')}</button>
        <button disabled={busy || !sample?.version} onClick={() => operate(data.draining ? 'resume' : 'drain')}>{data.draining ? t('workerOps.resume') : t('workerOps.drain')}</button>
        <label>{t('workerOps.capacity')} <input type="number" min={1} max={64} value={capacity} onChange={event => setCapacity(Number(event.target.value))} /></label>
        <button disabled={busy || !sample?.version || !Number.isInteger(capacity) || capacity < 1 || capacity > 64} onClick={() => operate('concurrency')}>{t('workerOps.apply')}</button>
      </div>
      <h3>{t('workerOps.tasks')}</h3>
      <div className="worker-runtime-table"><table><thead><tr><th>Build</th><th>{t('workerOps.stage')}</th><th>{t('workerOps.workspace')}</th><th>{t('workerOps.observed')}</th></tr></thead><tbody>
        {(sample?.tasks || []).map(task => <tr key={task.id}><td><Link to={`/builds/${encodeURIComponent(task.id)}`} onClick={onClose}>#{task.id}</Link><small>{task.project}</small></td><td>{task.stage} / {task.step}<small>{task.state}</small></td><td><code>{task.workspace}</code></td><td>{formatDateTime(task.updated_at)}</td></tr>)}
      </tbody></table>{!sample?.tasks?.length && <p>{t('workerOps.noTasks')}</p>}</div>
      <h3>{t('workerOps.operations')}</h3>
      <div className="worker-runtime-operations">{data.operations.map(operation => <article key={operation.id}><strong>{t(`workerOps.${operation.kind}`)} · {t(`workerOps.${operation.status}`)}</strong><code>{operation.id}</code><p>{operation.message}</p><small>{formatDateTime(operation.updated_at)}</small>{!!operation.result && <details><summary>{t('workerOps.evidence')}</summary><pre>{JSON.stringify(operation.result, null, 2)}</pre></details>}</article>)}</div>
      <h3>{t('workerOps.update')}</h3><p>{t('workerOps.updateHelp')}</p>
      <form onSubmit={upload} className="worker-update-form">
        <label>{t('workerOps.bundle')}<input type="file" accept=".zip,.gz,.tgz" required disabled={busy} onChange={event => setFile(event.target.files?.[0] || null)} /></label>
        <label>{t('workerOps.version')}<input value={version} required placeholder="1.17.0" pattern="v?[0-9]+\.[0-9]+\.[0-9]+" disabled={busy} onChange={event => setVersion(event.target.value)} /></label>
        <label>SHA-256<input value={digest} required pattern="[a-f0-9]{64}" disabled={busy} onChange={event => setDigest(event.target.value.trim())} /></label>
        <button type="submit" disabled={busy || !sample?.version}>{busy ? t('common.loading') : t('workerOps.update')}</button>
      </form>
    </>}
  </ModalDialog>
}
