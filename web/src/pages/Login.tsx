import { useState } from 'react'
import { Eye, EyeOff, LoaderCircle } from 'lucide-react'
import { Navigate } from 'react-router-dom'
import { api, setToken } from '../api'
import { localeLabels, type Locale, useI18n } from '../i18n'
import { BuildWorldMark } from '../components/BuildWorldMark'
import './Login.jenkins.css'

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

  return <main className="jenkins-login-page">
    <label className="jenkins-login-language" htmlFor="login-language">
      <span>{t('shell.language')}</span>
      <select id="login-language" value={locale} disabled={loading} onChange={event => changeLocale(event.target.value as Locale)}>
        {locales.map(value => <option key={value} value={value}>{localeLabels[value]}</option>)}
      </select>
    </label>

    <section className="jenkins-login-content" aria-labelledby="login-title">
      <div className="jenkins-login-brand" aria-label={t('app.title')}>
        <span><BuildWorldMark size={48} /></span>
        <strong>{t('app.title')}</strong>
      </div>

      <header className="jenkins-login-heading">
        <h1 id="login-title">{t('login.title')}</h1>
        <p>{t('login.description')}</p>
      </header>

      <form className="jenkins-login-form" aria-busy={loading} onSubmit={handleLogin}>
        {error && <div id="login-error" className="jenkins-login-error" role="alert">{error}</div>}

        <div className="jenkins-login-field">
          <label htmlFor="login-username">{t('login.username')}</label>
          <input
            id="login-username"
            name="username"
            required
            autoFocus
            disabled={loading}
            autoComplete="username"
            autoCorrect="off"
            autoCapitalize="off"
            spellCheck={false}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? 'login-error' : undefined}
            value={username}
            onChange={event => setUsername(event.target.value)}
          />
        </div>

        <div className="jenkins-login-field">
          <label htmlFor="login-password">{t('login.password')}</label>
          <div className="jenkins-login-password">
            <input
              id="login-password"
              name="password"
              required
              disabled={loading}
              type={showPassword ? 'text' : 'password'}
              autoComplete="current-password"
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? 'login-error' : undefined}
              value={password}
              onChange={event => setPassword(event.target.value)}
            />
            <button type="button" disabled={loading} onClick={() => setShowPassword(value => !value)} title={showPassword ? t('login.hidePassword') : t('login.showPassword')} aria-label={showPassword ? t('login.hidePassword') : t('login.showPassword')}>
              {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
            </button>
          </div>
        </div>

        <button className="jenkins-login-submit" type="submit" disabled={loading || !username.trim() || !password}>
          {loading && <LoaderCircle className="jenkins-login-spinner" size={17} aria-hidden="true" />}
          {loading ? t('login.signingIn') : t('login.signIn')}
        </button>
      </form>

      <p className="jenkins-login-help">{t('login.accountHelp')}</p>
    </section>
  </main>
}
