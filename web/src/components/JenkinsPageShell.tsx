import { useEffect, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router-dom'
import { useI18n } from '../i18n'

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

type JenkinsHeaderBreadcrumbProps = {
  breadcrumbs: JenkinsBreadcrumb[]
  ariaLabel?: string
}

export function JenkinsHeaderBreadcrumb({ breadcrumbs, ariaLabel = 'Breadcrumb' }: JenkinsHeaderBreadcrumbProps) {
  const { t } = useI18n()
  const [breadcrumbHost, setBreadcrumbHost] = useState<HTMLElement | null>(null)

  useEffect(() => {
    setBreadcrumbHost(document.getElementById('jenkins-header-breadcrumbs'))
  }, [])

  if (!breadcrumbHost) return null

  return createPortal(<nav className="jenkins-context-breadcrumb" aria-label={ariaLabel}>
    <ol>
      <li>
        <span className="jenkins-breadcrumb-separator" aria-hidden="true">/</span>
        <Link to="/">{t('common.all')}</Link>
      </li>
      {breadcrumbs.map((item, index) => <li key={`${item.label}-${index}`}>
        <span className="jenkins-breadcrumb-separator" aria-hidden="true">/</span>
        {item.to ? <Link to={item.to}>{item.label}</Link> : <strong>{item.label}</strong>}
      </li>)}
    </ol>
  </nav>, breadcrumbHost)
}

export default function JenkinsPageShell({ breadcrumbs, sidepanel, children, className = '', sidepanelLabel }: JenkinsPageShellProps) {
  return <>
    <JenkinsHeaderBreadcrumb breadcrumbs={breadcrumbs} />
    <section className={`jenkins-context-page ${className}`.trim()}>
      <div className="jenkins-context-layout">
        <aside className="jenkins-context-sidepanel" aria-label={sidepanelLabel}>{sidepanel}</aside>
        <div className="jenkins-context-main">{children}</div>
      </div>
    </section>
  </>
}
