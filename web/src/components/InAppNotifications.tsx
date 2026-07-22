import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Bell, CheckCircle2, CircleDot, LoaderCircle, X, XCircle } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import { useI18n } from '../i18n'
import { buildStatusLabel } from '../lib/buildPresentation'
import { formatDateTime } from '../lib/dateTime'

type NotificationPayload = {
  event?: string
  build_id?: number
  build_number?: number
  project_id?: number
  project?: string
  status?: string
  branch?: string
  message?: string
}

type InAppNotification = {
  id: number
  build_id?: number
  event_type: string
  payload: string
  created_at: string
}

type NotificationFeed = {
  items: InAppNotification[]
  unread_count: number
  last_read_id: number
}

function parsePayload(item: InAppNotification): NotificationPayload {
  try {
    return JSON.parse(item.payload) as NotificationPayload
  } catch {
    return { build_id: item.build_id }
  }
}

function StatusIcon({ status }: { status?: string }) {
  if (status === 'success') return <CheckCircle2 size={15} />
  if (status === 'failed' || status === 'cancelled') return <XCircle size={15} />
  if (status === 'running' || status === 'pending') return <LoaderCircle className="timeline-spinner" size={15} />
  return <CircleDot size={15} />
}

type InAppNotificationsProps = {
  menu?: boolean
  menuOpen?: boolean
}

export default function InAppNotifications({ menu = false, menuOpen = true }: InAppNotificationsProps) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const wrapperRef = useRef<HTMLDivElement>(null)
  const initializedRef = useRef(false)
  const latestEventRef = useRef(0)
  const [feed, setFeed] = useState<NotificationFeed>({ items: [], unread_count: 0, last_read_id: 0 })
  const [open, setOpen] = useState(false)
  const [toasts, setToasts] = useState<InAppNotification[]>([])

  const load = useCallback(async () => {
    try {
      const next = await api.listInAppNotifications(30) as NotificationFeed
      const items = next.items || []
      const latest = items[0]?.id || 0
      if (initializedRef.current && latestEventRef.current > 0) {
        const fresh = items.filter(item => item.id > latestEventRef.current).reverse()
        if (fresh.length) {
          setToasts(current => [...current, ...fresh].slice(-4))
          for (const item of fresh) {
            window.setTimeout(() => setToasts(current => current.filter(value => value.id !== item.id)), 7000)
          }
        }
      }
      initializedRef.current = true
      latestEventRef.current = Math.max(latestEventRef.current, latest)
      setFeed(next)
    } catch {
      // The global API status banner handles availability. Notification polling
      // remains silent so it never interrupts project work.
    }
  }, [])

  useEffect(() => {
    void load()
    const timer = window.setInterval(load, 7000)
    const onFocus = () => { void load() }
    window.addEventListener('focus', onFocus)
    return () => {
      window.clearInterval(timer)
      window.removeEventListener('focus', onFocus)
    }
  }, [load])

  useEffect(() => {
    const closeOutside = (event: MouseEvent) => {
      if (wrapperRef.current && !wrapperRef.current.contains(event.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', closeOutside)
    return () => document.removeEventListener('mousedown', closeOutside)
  }, [])

  useEffect(() => {
    if (!menuOpen) setOpen(false)
  }, [menuOpen])

  const markRead = async () => {
    const latest = feed.items[0]?.id || 0
    if (!latest || feed.unread_count === 0) return
    setFeed(current => ({ ...current, unread_count: 0, last_read_id: latest }))
    try {
      await api.markInAppNotificationsRead(latest)
    } catch {
      void load()
    }
  }

  const togglePanel = () => {
    const next = !open
    setOpen(next)
    if (next) void markRead()
  }

  const openBuild = (item: InAppNotification) => {
    const payload = parsePayload(item)
    const buildID = payload.build_id || item.build_id
    setOpen(false)
    setToasts(current => current.filter(value => value.id !== item.id))
    if (buildID) navigate(`/builds/${buildID}`)
  }

  const notificationCopy = (item: InAppNotification) => {
    const payload = parsePayload(item)
    return {
      payload,
      title: `${payload.project || t('builds.project')} · #${payload.build_number || payload.build_id || '-'}`,
      status: buildStatusLabel(t, payload.status || 'pending'),
      event: item.event_type === 'build.started' ? t('notifications.webStarted') : t('notifications.webCompleted'),
    }
  }

  return <>
    <div className="in-app-notification-center" ref={wrapperRef}>
      <button
        type="button"
        className={`notification-bell ${menu ? 'notification-bell-menu' : ''} ${open ? 'active' : ''}`}
        aria-label={t('notifications.webTitle')}
        aria-expanded={open}
        onClick={togglePanel}
      >
        <Bell size={17} />
        {menu && <span className="notification-menu-label">{t('notifications.webTitle')}</span>}
        {feed.unread_count > 0 && <span className="notification-unread-count">{feed.unread_count > 99 ? '99+' : feed.unread_count}</span>}
      </button>
      {open && <section className="notification-popover" role="dialog" aria-label={t('notifications.webTitle')}>
        <header><div><Bell size={16} /><div><h2>{t('notifications.webTitle')}</h2><p>{feed.unread_count ? t('notifications.webUnread').replace('{count}', String(feed.unread_count)) : t('notifications.webAllRead')}</p></div></div><button type="button" onClick={() => setOpen(false)} aria-label={t('common.close')}><X size={16} /></button></header>
        <div className="notification-popover-list">
          {!feed.items.length && <p className="notification-popover-empty">{t('notifications.webEmpty')}</p>}
          {feed.items.map(item => {
            const copy = notificationCopy(item)
            return <button type="button" key={item.id} className={`web-notification-item ${copy.payload.status || 'pending'}`} onClick={() => openBuild(item)}>
              <span className="web-notification-icon"><StatusIcon status={copy.payload.status} /></span>
              <span><strong>{copy.title}</strong><small>{copy.event} · {copy.status}{copy.payload.branch ? ` · ${copy.payload.branch}` : ''}</small><time>{formatDateTime(item.created_at)}</time></span>
            </button>
          })}
        </div>
      </section>}
    </div>
    {toasts.length > 0 && typeof document !== 'undefined' && createPortal(<aside className="web-notification-toasts" aria-live="polite">
      {toasts.map(item => {
        const copy = notificationCopy(item)
        return <article key={item.id} className={copy.payload.status || 'pending'} onClick={() => openBuild(item)}>
          <span><StatusIcon status={copy.payload.status} /></span>
          <div><strong>{copy.title}</strong><p>{copy.event} · {copy.status}</p></div>
          <button type="button" onClick={event => { event.stopPropagation(); setToasts(current => current.filter(value => value.id !== item.id)) }} aria-label={t('common.dismiss')}><X size={14} /></button>
        </article>
      })}
    </aside>, document.body)}
  </>
}
