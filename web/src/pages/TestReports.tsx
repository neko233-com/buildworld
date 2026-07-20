import { useRef, useState } from 'react'
import { motion } from 'motion/react'
import { ArrowLeft, CheckCircle2, CircleSlash2, FileCheck2, FlaskConical, LoaderCircle, Upload, XCircle } from 'lucide-react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { PageState } from '../components/PageState'
import { formatDuration } from '../lib/durationPresentation'

function TestMetric({ icon: Icon, label, value, tone = '' }: { icon: typeof FlaskConical; label: string; value: number; tone?: string }) {
  return <article><span className={tone}><Icon size={17} /></span><div><p>{label}</p><strong>{value}</strong></div></article>
}

function resultPresentation(status: string, t: (key: string) => string) {
  if (status === 'passed' || status === 'success') return { tone: 'success', label: t('testReports.passed') }
  if (status === 'failed' || status === 'failure' || status === 'error') return { tone: 'failed', label: t('testReports.failed') }
  return { tone: 'pending', label: t('testReports.skipped') }
}

export default function TestReports() {
  const { t } = useI18n()
  const { id } = useParams<{ id: string }>()
  const buildId = Number(id)
  const validBuildId = Number.isSafeInteger(buildId) && buildId > 0
  const { data, loading, error, reload } = useApi(() => validBuildId ? api.getBuildTestResults(buildId) : Promise.resolve(null), [buildId, validBuildId])
  const [uploading, setUploading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return
    setUploading(true)
    try {
      await api.uploadTestResults(buildId, await file.text())
      await reload()
      dialogs.notify(t('testReports.uploaded'), 'success')
    } catch (reason: any) {
      dialogs.notify(reason.message || t('testReports.uploadFailed'))
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  if (!validBuildId) return <Navigate to="/builds" replace />
  if (loading) return <PageState />
  if (error) return <PageState error={error} onRetry={reload} />

  const summary = data?.summary || data
  const passed = summary?.passed ?? summary?.tests ?? 0
  const failed = summary?.failed ?? summary?.failures ?? 0
  const skipped = summary?.skipped ?? summary?.skips ?? 0
  const total = summary?.total ?? (passed + failed + skipped)
  const cases: any[] = data?.cases || data?.test_cases || data?.testsuites || []

  return <motion.section className="operations-page test-report-page" initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: 'easeOut' }}>
    <header className="operations-heading test-report-heading">
      <div><p>#{buildId}</p><h1>{t('testReports.title')}</h1><small>{t('testReports.description')}</small></div>
      <div className="test-report-actions">
        <Link className="secondary-command" to={`/builds/${buildId}`}><ArrowLeft size={15} />{t('testReports.backToBuild')}</Link>
        <input ref={fileRef} hidden type="file" accept=".xml,text/xml,application/xml" onChange={handleUpload} />
        <button type="button" className="primary-command" onClick={() => fileRef.current?.click()} disabled={uploading} aria-busy={uploading}>
          {uploading ? <LoaderCircle className="timeline-spinner" size={15} /> : <Upload size={15} />}
          {uploading ? t('common.loading') : t('testReports.upload')}
        </button>
      </div>
    </header>

    <section className="test-report-metrics" aria-label={t('testReports.summary')}>
      <TestMetric icon={FlaskConical} label={t('testReports.total')} value={total} />
      <TestMetric icon={CheckCircle2} label={t('testReports.passed')} value={passed} tone="success" />
      <TestMetric icon={XCircle} label={t('testReports.failed')} value={failed} tone="failed" />
      <TestMetric icon={CircleSlash2} label={t('testReports.skipped')} value={skipped} tone="skipped" />
    </section>

    <section className="detail-panel test-results-panel">
      <header><div><FileCheck2 size={17} /><h2>{t('testReports.testCases')}</h2><span>{cases.length}</span></div></header>
      {!cases.length ? <p className="operations-empty test-results-empty"><FlaskConical size={19} /><strong>{t('testReports.noCases')}</strong><small>{t('testReports.noCasesHelp')}</small></p> : <div className="operations-table-wrap"><table className="operations-table test-results-table">
        <thead><tr><th>{t('testReports.name')}</th><th>{t('builds.status')}</th><th>{t('testReports.duration')}</th></tr></thead>
        <tbody>{cases.map((testCase: any, index: number) => {
          const rawStatus = testCase.status || (testCase.failure ? 'failed' : 'passed')
          const presentation = resultPresentation(rawStatus, t)
          return <tr key={`${testCase.classname || ''}-${testCase.name || testCase.testname || index}`}>
            <td><strong>{testCase.name || testCase.testname || testCase.classname || '-'}</strong>{testCase.classname && <small>{testCase.classname}</small>}</td>
            <td><span className={`build-status ${presentation.tone}`}>{presentation.label}</span></td>
            <td className="muted-cell">{formatDuration(testCase.time ? testCase.time * 1000 : testCase.duration_ms)}</td>
          </tr>
        })}</tbody>
      </table></div>}
    </section>
  </motion.section>
}
