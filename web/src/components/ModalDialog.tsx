import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'

type ModalDialogProps = {
  ariaLabel: string
  children: ReactNode
  className?: string
  busy?: boolean
  closeOnBackdrop?: boolean
  onClose: () => void
}

const focusableSelector = [
  '[data-dialog-initial-focus]',
  'button:not(:disabled)',
  'input:not(:disabled):not([type="hidden"])',
  'select:not(:disabled)',
  'textarea:not(:disabled)',
  '[href]',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function getFocusable(dialog: HTMLElement | null): HTMLElement[] {
  return Array.from(dialog?.querySelectorAll<HTMLElement>('*') || [])
    .filter(element => element.matches(focusableSelector) && element.getAttribute('aria-hidden') !== 'true')
}

export function ModalDialog({
  ariaLabel,
  children,
  className = '',
  busy = false,
  closeOnBackdrop = false,
  onClose,
}: ModalDialogProps) {
  const dialogRef = useRef<HTMLElement>(null)
  const originRef = useRef<HTMLElement | null>(
    typeof document !== 'undefined' && document.activeElement instanceof HTMLElement ? document.activeElement : null,
  )
  const originalOverflowRef = useRef(typeof document !== 'undefined' ? document.body.style.overflow : '')
  const restoreTimerRef = useRef<number | undefined>(undefined)
  const busyRef = useRef(busy)
  const onCloseRef = useRef(onClose)
  busyRef.current = busy
  onCloseRef.current = onClose

  useEffect(() => {
    window.clearTimeout(restoreTimerRef.current)
    const appRoot = document.getElementById('root')
    const rootAriaHidden = appRoot?.getAttribute('aria-hidden')
    const rootWasInert = appRoot?.hasAttribute('inert') ?? false
    document.body.style.overflow = 'hidden'
    appRoot?.setAttribute('aria-hidden', 'true')
    appRoot?.setAttribute('inert', '')
    const frame = window.requestAnimationFrame(() => {
      const preferred = dialogRef.current?.querySelector<HTMLElement>('[data-dialog-initial-focus]')
      const first = preferred || getFocusable(dialogRef.current)[0]
      first?.focus()
    })
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        if (!busyRef.current) onCloseRef.current()
        return
      }
      if (event.key !== 'Tab') return
      const focusable = getFocusable(dialogRef.current)
      if (!focusable.length) {
        event.preventDefault()
        dialogRef.current?.focus()
        return
      }
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.cancelAnimationFrame(frame)
      window.removeEventListener('keydown', handleKeyDown)
      if (rootAriaHidden === null) appRoot?.removeAttribute('aria-hidden')
      else if (rootAriaHidden !== undefined) appRoot?.setAttribute('aria-hidden', rootAriaHidden)
      if (!rootWasInert) appRoot?.removeAttribute('inert')
      restoreTimerRef.current = window.setTimeout(() => {
        document.body.style.overflow = originalOverflowRef.current
        if (originRef.current?.isConnected) originRef.current.focus()
      }, 0)
    }
  }, [])

  const requestClose = () => {
    if (!busyRef.current) onCloseRef.current()
  }

  return createPortal(
    <div className="schedule-modal-backdrop" onMouseDown={event => {
      if (closeOnBackdrop && event.target === event.currentTarget) requestClose()
    }}>
      <section
        ref={dialogRef}
        className={`schedule-modal ${className}`.trim()}
        role="dialog"
        aria-modal="true"
        aria-label={ariaLabel}
        aria-busy={busy || undefined}
        tabIndex={-1}
      >
        {children}
      </section>
    </div>,
    document.body,
  )
}
