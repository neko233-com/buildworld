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

const focusableSelector = 'button, input, select, textarea, a[href], area[href], summary, iframe, [tabindex]'

function getFocusable(dialog: HTMLElement | null): HTMLElement[] {
  return Array.from(dialog?.querySelectorAll<HTMLElement>(focusableSelector) || [])
    .filter(element => element.tabIndex >= 0 && !element.matches(':disabled, input[type="hidden"]') && !element.closest('[hidden], [inert], [aria-hidden="true"]'))
}

const modalStack: HTMLElement[] = []
let unlockPage: (() => void) | undefined

function lockPage() {
  const appRoot = document.getElementById('root')
  const previousAriaHidden = appRoot?.getAttribute('aria-hidden')
  const previousInert = appRoot?.hasAttribute('inert')
  const previousOverflow = document.body.style.overflow
  document.body.style.overflow = 'hidden'
  appRoot?.setAttribute('aria-hidden', 'true')
  appRoot?.setAttribute('inert', '')
  return () => {
    document.body.style.overflow = previousOverflow
    if (previousAriaHidden == null) appRoot?.removeAttribute('aria-hidden')
    else appRoot?.setAttribute('aria-hidden', previousAriaHidden)
    if (!previousInert) appRoot?.removeAttribute('inert')
  }
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
  const restoreTimerRef = useRef<number | undefined>(undefined)
  const busyRef = useRef(busy)
  const onCloseRef = useRef(onClose)
  busyRef.current = busy
  onCloseRef.current = onClose

  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return
    window.clearTimeout(restoreTimerRef.current)
    const parent = modalStack.at(-1)
    modalStack.push(dialog)
    const focusable = getFocusable(dialog)
    const target = focusable.find(element => element.hasAttribute('data-dialog-initial-focus')) || focusable[0] || dialog
    target.focus()
    if (parent) parent.setAttribute('inert', '')
    else unlockPage = lockPage()
    const handleKeyDown = (event: KeyboardEvent) => {
      if (modalStack.at(-1) !== dialog || event.defaultPrevented || event.isComposing || event.keyCode === 229) return
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
      if (!dialog.contains(document.activeElement) || document.activeElement === dialog) {
        event.preventDefault()
        const target = event.shiftKey ? last : first
        target.focus()
      } else if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
      const wasTop = modalStack.at(-1) === dialog
      modalStack.splice(modalStack.indexOf(dialog), 1)
      if (!wasTop) return
      const next = modalStack.at(-1)
      next?.removeAttribute('inert')
      if (!next) {
        unlockPage?.()
        unlockPage = undefined
      }
      restoreTimerRef.current = window.setTimeout(() => {
        if (modalStack.at(-1) !== next) return
        const origin = originRef.current
        if (origin?.isConnected && (!next || next.contains(origin))) origin.focus()
        else if (next) (getFocusable(next)[0] || next).focus()
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
