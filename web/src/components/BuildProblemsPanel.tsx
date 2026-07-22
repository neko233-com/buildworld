import {
  AlertTriangle,
  CheckCircle2,
  FileSearch,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
  Settings2,
  Wrench,
} from 'lucide-react'
import { Link } from 'react-router-dom'
import { useI18n } from '../i18n'
import type { BuildProblem, BuildProblemReport } from '../lib/buildProblems'

export type BuildProblemsPanelProps = {
  buildId: number
  projectId: number
  report?: BuildProblemReport | null
  loading?: boolean
  error?: string | null
  editable?: boolean
  retrying?: boolean
  onRetry?: () => void
  onReload?: () => void
}

function problemTitle(problem: BuildProblem, t: (key: string) => string): string {
  if (problem.step) return problem.step
  if (problem.code === 'build_cancelled') return t('builds.cancelledProblem')
  if (problem.code === 'approval_rejected') return t('builds.rejectedProblem')
  return t('builds.buildFailure')
}

export default function BuildProblemsPanel({
  buildId,
  projectId,
  report,
  loading = false,
  error,
  editable = false,
  retrying = false,
  onRetry,
  onReload,
}: BuildProblemsPanelProps) {
  const { t } = useI18n()
  const problems = report?.problems || []
  const first = problems[0]
  const tone = first?.severity ? ` ${first.severity}` : ''

  return <section className={`build-problems-panel${tone}`} aria-label={t('builds.problems')} aria-busy={loading}>
    {loading && <div className="build-problems-state loading" role="status" aria-live="polite">
      <LoaderCircle className="timeline-spinner" size={20} aria-hidden="true" />
      <span>{t('common.loading')}</span>
    </div>}

    {!loading && error && <div className="build-problems-state error" role="alert">
      <AlertTriangle size={20} aria-hidden="true" />
      <div>
        <strong>{t('builds.problemsLoadFailed')}</strong>
        <p>{error}</p>
      </div>
      {onReload && <button type="button" onClick={onReload}><RefreshCw size={14} aria-hidden="true" />{t('common.retry')}</button>}
    </div>}

    {!loading && !error && !first && <div className="build-problems-state empty" role="status">
      <CheckCircle2 size={20} aria-hidden="true" />
      <span>{t('builds.noProblems')}</span>
    </div>}

    {!loading && !error && first && <>
      <header>
        <div className="build-problems-heading">
          <span><AlertTriangle size={17} aria-hidden="true" /></span>
          <div>
            <h2>{t('builds.problems')}</h2>
            <p>{report?.failed_step_count
              ? t('builds.failedStepCount').replace('{count}', String(report.failed_step_count))
              : report?.summary || t('builds.problemSummary')}</p>
          </div>
        </div>
        <div className="build-problem-actions">
          <Link to={`/builds/${buildId}/logs`} target="_blank" rel="noopener noreferrer"><FileSearch size={14} aria-hidden="true" />{t('builds.inspectFullLog')}</Link>
          {editable && first.suggested_action !== 'inspect_logs' && <Link to={`/projects/${projectId}/configure`}><Settings2 size={14} aria-hidden="true" />{t('builds.fixProjectSettings')}</Link>}
          {editable && onRetry && <button type="button" onClick={onRetry} disabled={retrying}><RotateCcw className={retrying ? 'timeline-spinner' : undefined} size={14} aria-hidden="true" />{retrying ? t('builds.retrying') : t('builds.retryBuild')}</button>}
        </div>
      </header>
      <ol>
        {problems.map((problem, index) => <li key={`${problem.id}-${index}`} className={problem.severity}>
          <span className="problem-index" aria-hidden="true">{index + 1}</span>
          <div>
            <div className="problem-title-row">
              <strong>{problemTitle(problem, t)}</strong>
              {problem.stage && <small>{problem.stage}</small>}
              <code>{problem.code}</code>
            </div>
            <p>{problem.message}</p>
            {problem.excerpt && <pre>{problem.excerpt}</pre>}
          </div>
        </li>)}
      </ol>
      <footer><Wrench size={14} aria-hidden="true" /><span>{t('builds.problemHelp')}</span></footer>
    </>}
  </section>
}
