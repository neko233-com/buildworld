import type { ReactNode } from 'react'
import { ChevronRight } from 'lucide-react'
import { Link } from 'react-router-dom'

export type JenkinsBreadcrumb = {
  label: string
  to?: string
}

type JenkinsPageShellProps = {
  breadcrumbs: JenkinsBreadcrumb[]
  sidepanel: ReactNode
  children: ReactNode
  className?: string
  sidepanelLabel: string
}

export default function JenkinsPageShell({ breadcrumbs, sidepanel, children, className = '', sidepanelLabel }: JenkinsPageShellProps) {
  return <section className={`jenkins-context-page ${className}`.trim()}>
    <nav className="jenkins-context-breadcrumb" aria-label="Breadcrumb">
      <Link to="/">BuildWorld</Link>
      {breadcrumbs.map((item, index) => <span key={`${item.label}-${index}`}>
        <ChevronRight size={13} aria-hidden="true" />
        {item.to ? <Link to={item.to}>{item.label}</Link> : <strong>{item.label}</strong>}
      </span>)}
    </nav>
    <div className="jenkins-context-layout">
      <aside className="jenkins-context-sidepanel" aria-label={sidepanelLabel}>{sidepanel}</aside>
      <div className="jenkins-context-main">{children}</div>
    </div>
  </section>
}
