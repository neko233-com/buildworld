import { useEffect, useId, useMemo, useRef, useState } from 'react'
import type { ComponentType, KeyboardEvent as ReactKeyboardEvent } from 'react'
import { Search } from 'lucide-react'

type CommandIcon = ComponentType<{ size?: number }>

export type CommandPaletteItem = {
  id: string
  label: string
  detail?: string
  href: string
  icon: CommandIcon
}

export type CommandPaletteGroup = {
  id: string
  label: string
  items: CommandPaletteItem[]
}

type CommandPaletteProps = {
  ariaLabel: string
  placeholder: string
  query: string
  groups: CommandPaletteGroup[]
  loading?: boolean
  loadingLabel: string
  emptyLabel: string
  keyboardHint: string
  error?: string
  onQueryChange: (value: string) => void
  onNavigate: (href: string) => void
  onClose: () => void
}

export function CommandPalette({
  ariaLabel,
  placeholder,
  query,
  groups,
  loading = false,
  loadingLabel,
  emptyLabel,
  keyboardHint,
  error,
  onQueryChange,
  onNavigate,
  onClose,
}: CommandPaletteProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const dialogRef = useRef<HTMLElement>(null)
  const listboxId = useId()
  const [activeIndex, setActiveIndex] = useState(0)
  const items = useMemo(() => groups.flatMap(group => group.items), [groups])
  const activeItem = items[activeIndex]

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  useEffect(() => {
    setActiveIndex(current => items.length ? Math.min(current, items.length - 1) : 0)
  }, [items.length])

  useEffect(() => {
    setActiveIndex(0)
  }, [query])

  const optionId = (index: number) => `${listboxId}-option-${index}`
  const moveSelection = (direction: 1 | -1) => {
    if (!items.length) return
    setActiveIndex(current => (current + direction + items.length) % items.length)
    inputRef.current?.focus()
  }

  const handleKeyDown = (event: ReactKeyboardEvent<HTMLElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      moveSelection(1)
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      moveSelection(-1)
      return
    }
    if (event.key === 'Home' && items.length) {
      event.preventDefault()
      setActiveIndex(0)
      inputRef.current?.focus()
      return
    }
    if (event.key === 'End' && items.length) {
      event.preventDefault()
      setActiveIndex(items.length - 1)
      inputRef.current?.focus()
      return
    }
    if (event.key === 'Enter' && activeItem) {
      event.preventDefault()
      onNavigate(activeItem.href)
      return
    }
    if (event.key !== 'Tab') return

    const focusable = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('input, button:not(:disabled)') || [])
    if (!focusable.length) return
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

  let renderedIndex = 0
  return (
    <div className="palette-backdrop" onMouseDown={onClose}>
      <section
        ref={dialogRef}
        className="command-palette global-command-palette"
        role="dialog"
        aria-modal="true"
        aria-label={ariaLabel}
        onKeyDown={handleKeyDown}
        onMouseDown={event => event.stopPropagation()}
      >
        <div className="command-search">
          <Search size={17} />
          <input
            ref={inputRef}
            role="combobox"
            aria-autocomplete="list"
            aria-expanded="true"
            aria-controls={listboxId}
            aria-activedescendant={activeItem ? optionId(activeIndex) : undefined}
            aria-label={placeholder}
            value={query}
            onChange={event => onQueryChange(event.target.value)}
            placeholder={placeholder}
          />
          <kbd>Esc</kbd>
        </div>
        <div id={listboxId} className="palette-results" role="listbox" aria-busy={loading}>
          {loading && <p className="palette-loading" role="status">{loadingLabel}</p>}
          {error && <p className="palette-empty" role="alert">{error}</p>}
          {groups.map(group => {
            if (!group.items.length) return null
            const headingId = `${listboxId}-${group.id}`
            return (
              <section key={group.id} role="group" aria-labelledby={headingId}>
                <p id={headingId}>{group.label}</p>
                {group.items.map(item => {
                  const index = renderedIndex++
                  const Icon = item.icon
                  return (
                    <button
                      key={item.id}
                      id={optionId(index)}
                      type="button"
                      role="option"
                      aria-selected={index === activeIndex}
                      className={index === activeIndex ? 'active' : ''}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={() => onNavigate(item.href)}
                    >
                      <Icon size={16} />
                      <span>{item.label}{item.detail && <small>{item.detail}</small>}</span>
                    </button>
                  )
                })}
              </section>
            )
          })}
          {!loading && !error && items.length === 0 && <p className="palette-empty">{emptyLabel}</p>}
        </div>
        <footer className="command-palette-footer">{keyboardHint}</footer>
      </section>
    </div>
  )
}
