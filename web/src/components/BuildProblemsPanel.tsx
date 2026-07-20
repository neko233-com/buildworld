import { AlertTriangle, FileSearch, RotateCcw, Settings2, UsersRound, Wrench } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useI18n } from '../i18n'
import type { BuildProblem, BuildProblemReport } from '../lib/buildProblems'

type BuildProblemsPanelProps = {
  buildId: number
  projectId: number
  report: BuildProblemReport
  editable: boolean
  retrying: boolean
  onRetry: () => void
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
  editable,
  retrying,
  onRetry,
}: BuildProblemsPanelProps) {
  const { t } = useI18n()
  if (!report.problems.length) return null
  const first = report.problems[0]

  return <section className={`build-problems-panel ${first.severity}`} aria-label={t('builds.problems')}>
    <header>
      <div className="build-problems-heading">
        <span><AlertTriangle size={17} /></span>
        <div>
          <h2>{t('builds.problems')}</h2>
          <p>{report.failed_step_count
            ? t('builds.failedStepCount').replace('{count}', String(report.failed_step_count))
            : t('builds.problemSummary')}</p>
        </div>
      </div>
      <div className="build-problem-actions">
        <Link to={`/builds/${buildId}/logs`} target="_blank" rel="noopener noreferrer"><FileSearch size={14} />{t('builds.inspectFullLog')}</Link>
        {editable && first.suggested_action === 'worker' && <Link to="/agents"><UsersRound size={14} />{t('builds.inspectWorkers')}</Link>}
        {editable && first.suggested_action !== 'worker' && <Link to={`/projects/${projectId}?view=settings`}><Settings2 size={14} />{t('builds.fixProjectSettings')}</Link>}
        {editable && <button type="button" onClick={onRetry} disabled={retrying}><RotateCcw size={14} />{retrying ? t('builds.retrying') : t('builds.retryBuild')}</button>}
      </div>
    </header>
    <ol>
      {report.problems.map((problem, index) => <li key={problem.id} className={problem.severity}>
        <span className="problem-index">{index + 1}</span>
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
    <footer><Wrench size={14} /><span>{t('builds.problemHelp')}</span></footer>
  </section>
}
