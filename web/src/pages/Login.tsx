import { useState } from 'react'
import { Braces, Eye, EyeOff, LoaderCircle, LockKeyhole, Network, PackageCheck, Workflow } from 'lucide-react'
import { Navigate } from 'react-router-dom'
import { api, setToken } from '../api'
import { localeLabels, type Locale, useI18n } from '../i18n'

const capabilityIcons = [Workflow, Network, PackageCheck]

export default function Login() {
  const { t, locale, locales, changeLocale } = useI18n()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  if (localStorage.getItem('token')) return <Navigate to="/" replace />

  const handleLogin = async (event: React.FormEvent) => {
    event.preventDefault()
    setLoading(true)
    setError('')
    try {
      const { token } = await api.login(username.trim(), password)
      setToken(token)
      window.location.assign('/')
    } catch (reason: any) {
      setError(reason.message || t('login.failed'))
    } finally {
      setLoading(false)
    }
  }

  return <main className="login-page">
    <section className="login-context" aria-label={t('login.productOverview')}>
      <div className="login-context-inner">
        <div className="login-brand"><span><Braces size={18} /></span><strong>{t('app.title')}</strong></div>
        <div className="login-intro"><p>{t('login.eyebrow')}</p><h1>{t('login.heroTitle')}</h1><span>{t('login.heroDescription')}</span></div>
        <div className="login-capabilities">
          {[t('login.capabilityProjects'), t('login.capabilityWorkers'), t('login.capabilityPortability')].map((label, index) => {
            const Icon = capabilityIcons[index]
            return <div key={label}><Icon size={16} /><span>{label}</span></div>
          })}
        </div>
      </div>
    </section>

    <section className="login-panel">
      <header className="login-toolbar">
        <label htmlFor="login-language">{t('shell.language')}</label>
        <select id="login-language" value={locale} onChange={event => changeLocale(event.target.value as Locale)}>
          {locales.map(value => <option key={value} value={value}>{localeLabels[value]}</option>)}
        </select>
      </header>

      <div className="login-card">
        <span className="login-card-icon"><LockKeyhole size={21} /></span>
        <p>{t('login.welcome')}</p>
        <h2>{t('login.title')}</h2>
        <span className="login-card-description">{t('login.description')}</span>

        <form onSubmit={handleLogin}>
          <label htmlFor="login-username">{t('login.username')}</label>
          <input
            id="login-username"
            name="username"
            required
            autoFocus
            autoComplete="username"
            value={username}
            onChange={event => setUsername(event.target.value)}
          />

          <label htmlFor="login-password">{t('login.password')}</label>
          <div className="login-password-field">
            <input
              id="login-password"
              name="password"
              required
              type={showPassword ? 'text' : 'password'}
              autoComplete="current-password"
              value={password}
              onChange={event => setPassword(event.target.value)}
            />
            <button type="button" onClick={() => setShowPassword(value => !value)} title={showPassword ? t('login.hidePassword') : t('login.showPassword')} aria-label={showPassword ? t('login.hidePassword') : t('login.showPassword')}>
              {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
            </button>
          </div>

          {error && <p className="login-error" role="alert">{error}</p>}
          <button className="login-submit" type="submit" disabled={loading || !username.trim() || !password}>
            {loading ? <LoaderCircle className="timeline-spinner" size={16} /> : <LockKeyhole size={16} />}
            {loading ? t('login.signingIn') : t('login.signIn')}
          </button>
        </form>
        <small>{t('login.accountHelp')}</small>
      </div>
    </section>
  </main>
}
