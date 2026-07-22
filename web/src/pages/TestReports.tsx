import { useRef, useState } from 'react'
import { Activity, ArrowLeft, CheckCircle2, CircleSlash2, FileCheck2, FileText, FlaskConical, GitCommitHorizontal, LoaderCircle, Upload, XCircle } from 'lucide-react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api, type BuildTestResultsResponse, type TestCaseResult } from '../api'
import { useApi } from '../hooks'
import { dialogs } from '../components/AppDialogs'
import { BuildStatusBadge } from '../components/BuildStatusBadge'
import { JenkinsHeaderBreadcrumb } from '../components/JenkinsPageShell'
import { PageState } from '../components/PageState'
import { formatDuration } from '../lib/durationPresentation'
import './BuildDetail.jenkins.css'
import './TestReports.jenkins.css'

type TestReportPageData = {
  report: BuildTestResultsResponse
  build: { id: number; number: number; project_id: number; status?: string }
  project: { id: number; name: string }
}

function TestMetric({ icon: Icon, label, value, tone = '' }: { icon: typeof FlaskConical; label: string; value: number; tone?: string }) {
  return <article><span className={tone}><Icon size={17} /></span><div><p>{label}</p><strong>{value}</strong></div></article>
}

function resultPresentation(status: TestCaseResult['status'], t: (key: string) => string) {
  if (status === 'passed') return { tone: 'success', label: t('testReports.passed') }
  if (status === 'failed') return { tone: 'failed', label: t('testReports.failed') }
  return { tone: 'pending', label: t('testReports.skipped') }
}

export default function TestReports() {
  const { t } = useI18n()
  const { id } = useParams<{ id: string }>()
  const buildId = Number(id)
  const validBuildId = Number.isSafeInteger(buildId) && buildId > 0
  const { data, loading, error, reload } = useApi<TestReportPageData | null>(async () => {
    if (!validBuildId) return null
    const [report, build] = await Promise.all([api.getBuildTestResults(buildId), api.getBuild(buildId)])
    const project = await api.getProject(build.project_id)
    return { report, build, project }
  }, [buildId, validBuildId])
  const [uploading, setUploading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)
  const buildNumber = data?.build.number ?? buildId
  const projectName = data?.project.name || t('builds.title')
  const breadcrumb = validBuildId ? <JenkinsHeaderBreadcrumb breadcrumbs={[
    { label: projectName, to: data?.project.id ? `/projects/${data.project.id}` : '/builds' },
    { label: `#${buildNumber}`, to: `/builds/${buildId}` },
    { label: t('testReports.title') },
  ]} /> : null

  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file || uploading) return
    setUploading(true)
    try {
      await api.uploadTestResults(buildId, await file.text())
      await reload()
      dialogs.notify(t('testReports.uploaded'), 'success')
    } catch (reason: unknown) {
      dialogs.notify(reason instanceof Error ? reason.message : t('testReports.uploadFailed'))
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  if (!validBuildId) return <Navigate to="/builds" replace />
  if (loading) return <>{breadcrumb}<section className="jenkins-run-page test-report-page"><PageState /></section></>
  if (error) return <>{breadcrumb}<section className="jenkins-run-page test-report-page"><PageState error={error} onRetry={reload} /></section></>

  const summary = data?.report.summary
  const passed = summary?.passed ?? 0
  const failed = summary?.failed ?? 0
  const skipped = summary?.skipped ?? 0
  const total = summary?.total ?? 0
  const cases = data?.report.cases ?? []

  const buildStatus = data?.build.status || 'pending'

  return <>{breadcrumb}<section className="jenkins-run-page test-report-page">
    <div className="jenkins-run-layout">
      <aside className="jenkins-run-side-panel" aria-label={t('builds.build')}>
        <nav className="jenkins-run-tasks">
          <Link to={`/builds/${buildId}`}><Activity />{t('projectDetail.status')}</Link>
          <Link to={`/projects/${data?.project.id}/changes?build=${buildId}`}><GitCommitHorizontal />{t('projectDetail.changes')}</Link>
          <Link to={`/builds/${buildId}/logs`}><FileText />{t('builds.logs')}</Link>
          <Link className="active" to={`/builds/${buildId}/tests`} aria-current="page"><FlaskConical />{t('builds.testReports')}</Link>
        </nav>
        <section className="jenkins-run-side-summary">
          <h2>{t('builds.build')} #{buildNumber}</h2>
          <dl>
            <div><dt>{t('builds.project')}</dt><dd>{projectName}</dd></div>
            <div><dt>{t('builds.status')}</dt><dd>{t(`builds.${buildStatus}`)}</dd></div>
            <div><dt>{t('testReports.total')}</dt><dd>{total}</dd></div>
            <div><dt>{t('testReports.failed')}</dt><dd>{failed}</dd></div>
          </dl>
        </section>
      </aside>

      <div className="jenkins-run-main">
        <header className="jenkins-run-caption test-report-heading">
          <div className="jenkins-run-caption-identity">
            <BuildStatusBadge status={buildStatus} label={t(`builds.${buildStatus}`)} />
            <h1>{t('testReports.title')}<small>({projectName} #{buildNumber})</small></h1>
          </div>
          <div className="jenkins-run-controls test-report-actions">
            <Link to={`/builds/${buildId}`}><ArrowLeft size={15} />{t('testReports.backToBuild')}</Link>
            <input ref={fileRef} hidden disabled={uploading} type="file" accept=".xml,text/xml,application/xml" onChange={handleUpload} />
            <button type="button" onClick={() => fileRef.current?.click()} disabled={uploading} aria-busy={uploading}>
              {uploading ? <LoaderCircle className="timeline-spinner" size={15} /> : <Upload size={15} />}
              {uploading ? t('common.loading') : t('testReports.upload')}
            </button>
          </div>
        </header>

        <p className="test-report-description">{t('testReports.description')}</p>
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
            <tbody>{cases.map((testCase, index) => {
              const presentation = resultPresentation(testCase.status, t)
              const detail = [testCase.classname, testCase.type, testCase.message].filter(Boolean).join(' · ')
              return <tr key={`${testCase.classname || ''}-${testCase.name}-${index}`}>
                <td><strong>{testCase.name || testCase.classname || '-'}</strong>{detail && <small>{detail}</small>}</td>
                <td><span className={`build-status ${presentation.tone}`}>{presentation.label}</span></td>
                <td className="muted-cell">{formatDuration(testCase.duration_ms)}</td>
              </tr>
            })}</tbody>
          </table></div>}
        </section>
      </div>
    </div>
  </section></>
}
