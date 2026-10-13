import { useEffect, useId, useRef, useState, type KeyboardEvent, type Ref } from 'react'
import { FiCheck, FiChevronDown, FiSearch } from 'react-icons/fi'
import './SearchableSelect.css'

export type SearchableSelectOption = { value: string; label: string; detail?: string }

type SearchableSelectProps = {
  id: string
  value: string
  options: SearchableSelectOption[]
  onChange: (value: string) => void
  placeholder: string
  emptyLabel: string
  keyboardHint: string
  disabled?: boolean
  invalid?: boolean
  describedBy?: string
  inputRef?: Ref<HTMLInputElement>
}

export function SearchableSelect({ id, value, options, onChange, placeholder, emptyLabel, keyboardHint, disabled, invalid, describedBy, inputRef }: SearchableSelectProps) {
  const listID = useId().replace(/:/g, '')
  const rootRef = useRef<HTMLDivElement>(null)
  const controlRef = useRef<HTMLInputElement>(null)
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const filtered = options.filter(option => `${option.label} ${option.value} ${option.detail || ''}`.toLowerCase().includes(query.trim().toLowerCase()))
  const selected = options.find(option => option.value === value)
  const currentIndex = Math.min(activeIndex, Math.max(0, filtered.length - 1))
  const expanded = open && !disabled
  const active = filtered[currentIndex]
  useEffect(() => {
    if (expanded) document.getElementById(`${listID}-${currentIndex}`)?.scrollIntoView?.({ block: 'nearest' })
  }, [expanded, listID, currentIndex, query])
  const close = () => { setOpen(false); setQuery('') }
  const show = () => {
    setQuery('')
    setActiveIndex(Math.max(0, options.findIndex(option => option.value === value)))
    setOpen(true)
  }
  const select = (option: SearchableSelectOption) => {
    onChange(option.value)
    close()
    controlRef.current?.focus()
  }
  const keyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.nativeEvent.isComposing || disabled) return
    if (event.key === 'Escape' && expanded) {
      event.preventDefault()
      event.stopPropagation()
      close()
    } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (!expanded) show()
      else if (filtered.length) setActiveIndex((currentIndex + (event.key === 'ArrowDown' ? 1 : -1) + filtered.length) % filtered.length)
    } else if (expanded && (event.key === 'Home' || event.key === 'End')) {
      event.preventDefault()
      setActiveIndex(event.key === 'Home' ? 0 : Math.max(0, filtered.length - 1))
    } else if (expanded && event.key === 'Enter') {
      event.preventDefault()
      if (active) select(active)
    } else if (event.key === 'Tab') close()
  }

  return <div className={`searchable-select ${expanded ? 'is-open' : ''}`} ref={rootRef} onBlur={event => { if (!rootRef.current?.contains(event.relatedTarget)) close() }}>
    <div className="searchable-select-control">
      <FiSearch aria-hidden="true" className="searchable-select-search" />
      <input ref={element => { controlRef.current = element; if (typeof inputRef === 'function') inputRef(element); else if (inputRef) inputRef.current = element }} id={id} role="combobox" autoComplete="off" aria-autocomplete="list" aria-expanded={expanded} aria-controls={expanded ? listID : undefined} aria-activedescendant={expanded && active ? `${listID}-${currentIndex}` : undefined} aria-invalid={invalid || undefined} aria-describedby={describedBy} disabled={disabled} value={expanded ? query : selected?.label || value} placeholder={placeholder} onFocus={show} onChange={event => { setQuery(event.target.value); setActiveIndex(0); setOpen(true) }} onKeyDown={keyDown} />
      <button type="button" className="searchable-select-toggle" tabIndex={-1} aria-label={placeholder} disabled={disabled} onMouseDown={event => event.preventDefault()} onClick={() => { if (expanded) close(); else { controlRef.current?.focus(); show() } }}><FiChevronDown aria-hidden="true" /></button>
    </div>
    {expanded && <div className="searchable-select-popup">
      <div id={listID} role="listbox" aria-labelledby={id} className="searchable-select-options">
        {filtered.map((option, index) => <button id={`${listID}-${index}`} type="button" role="option" aria-selected={option.value === value} tabIndex={-1} key={option.value} className={currentIndex === index ? 'active' : ''} onMouseDown={event => event.preventDefault()} onMouseEnter={() => setActiveIndex(index)} onClick={() => select(option)}>
          <span><strong>{option.label}</strong>{option.label !== option.value && <code>{option.value}</code>}{option.detail && <small>{option.detail}</small>}</span>
          {option.value === value && <FiCheck aria-hidden="true" />}
        </button>)}
        {!filtered.length && <p role="status">{emptyLabel}</p>}
      </div>
      <small className="searchable-select-hint">{keyboardHint}</small>
    </div>}
  </div>
}
