import { ShieldCheck } from 'lucide-react'
import { useMemo } from 'react'
import { useI18n } from '../i18n'
import { readApprovalPolicy, writeApprovalPolicy, type ApprovalPolicySettings } from '../lib/approvalPolicy'

type Props = {
  source: string
  disabled?: boolean
  onChange: (source: string) => void
  onError?: (message: string) => void
}

export default function ApprovalSettingsEditor({ source, disabled = false, onChange, onError }: Props) {
  const { t } = useI18n()
  const parsed = useMemo(() => {
    try {
      return { policy: readApprovalPolicy(source), error: '' }
    } catch (error: any) {
      return { policy: null, error: error.message || t('config.invalid') }
    }
  }, [source, t])

  const update = (next: ApprovalPolicySettings) => {
    try {
      onChange(writeApprovalPolicy(source, next))
      onError?.('')
    } catch (error: any) {
      onError?.(error.message || t('config.invalid'))
    }
  }

  if (!parsed.policy) {
    return <section className="approval-settings-card invalid"><div><ShieldCheck size={17} /><div><h3>{t('approvals.buildGate')}</h3><p>{t('approvals.fixConfigFirst')}</p></div></div><code>{parsed.error}</code></section>
  }

  const policy = parsed.policy
  const toggleRole = (role: 'admin' | 'developer') => {
    const selected = policy.requiredRoles.includes(role)
    const roles = selected ? policy.requiredRoles.filter(item => item !== role) : [...policy.requiredRoles, role]
    update({ ...policy, requiredRoles: roles.length ? roles : [role] })
  }

  return <section className={`approval-settings-card ${policy.enabled ? 'enabled' : ''}`}>
    <header>
      <div><ShieldCheck size={17} /><div><h3>{t('approvals.buildGate')}</h3><p>{t('approvals.policyHelp')}</p></div></div>
      <label className="switch-control">
        <input type="checkbox" checked={policy.enabled} disabled={disabled} onChange={event => update({ ...policy, enabled: event.target.checked })} />
        <span aria-hidden="true" />
        <em>{policy.enabled ? t('common.enabled') : t('common.disabled')}</em>
      </label>
    </header>
    {policy.enabled && <div className="approval-settings-body">
      <fieldset disabled={disabled}>
        <legend>{t('approvals.allowedRoles')}</legend>
        <label><input type="checkbox" checked={policy.requiredRoles.includes('admin')} onChange={() => toggleRole('admin')} />{t('users.role_admin')}</label>
        <label><input type="checkbox" checked={policy.requiredRoles.includes('developer')} onChange={() => toggleRole('developer')} />{t('users.role_developer')}</label>
      </fieldset>
      <label className="approval-checkbox"><input type="checkbox" checked={policy.allowRequester} disabled={disabled} onChange={event => update({ ...policy, allowRequester: event.target.checked })} /><span><strong>{t('approvals.allowRequester')}</strong><small>{t('approvals.allowRequesterHelp')}</small></span></label>
      <label><span>{t('approvals.prompt')}</span><input value={policy.prompt} disabled={disabled} onChange={event => update({ ...policy, prompt: event.target.value })} placeholder={t('approvals.promptPlaceholder')} /></label>
    </div>}
  </section>
}
