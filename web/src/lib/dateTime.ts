export type DateTimeValue = Date | string | number | null | undefined
export type DateWeekdayLocale = 'en' | 'zh-CN' | 'ja-JP' | 'ko-KR' | 'ru-RU' | 'hi-IN'

const DATE_ONLY_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/
const DATE_TIME_PATTERN = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2}):(\d{2})(?:[.,](\d{1,9}))?(Z|[+-]\d{2}:\d{2})?$/i
const WEEKDAY_LABELS: Record<DateWeekdayLocale, readonly [string, string, string, string, string, string, string]> = {
  en: ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'],
  'zh-CN': ['星期日', '星期一', '星期二', '星期三', '星期四', '星期五', '星期六'],
  'ja-JP': ['日曜日', '月曜日', '火曜日', '水曜日', '木曜日', '金曜日', '土曜日'],
  'ko-KR': ['일요일', '월요일', '화요일', '수요일', '목요일', '금요일', '토요일'],
  'ru-RU': ['воскресенье', 'понедельник', 'вторник', 'среда', 'четверг', 'пятница', 'суббота'],
  'hi-IN': ['रविवार', 'सोमवार', 'मंगलवार', 'बुधवार', 'गुरुवार', 'शुक्रवार', 'शनिवार'],
}

function localDate(year: number, month: number, day: number): Date | null {
  const date = new Date(0)
  date.setHours(0, 0, 0, 0)
  date.setFullYear(year, month - 1, day)

  if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day) {
    return null
  }
  return date
}

function parseDateTime(value: DateTimeValue): Date | null {
  if (value === null || value === undefined || value === '') return null

  if (value instanceof Date) {
    return Number.isNaN(value.getTime()) ? null : value
  }

  if (typeof value === 'string') {
    const normalized = value.trim()
    if (!normalized) return null

    const dateOnly = DATE_ONLY_PATTERN.exec(normalized)
    if (dateOnly) {
      return localDate(Number(dateOnly[1]), Number(dateOnly[2]), Number(dateOnly[3]))
    }

    const dateTime = DATE_TIME_PATTERN.exec(normalized)
    if (!dateTime) return null

    const year = Number(dateTime[1])
    const month = Number(dateTime[2])
    const day = Number(dateTime[3])
    const hour = Number(dateTime[4])
    const minute = Number(dateTime[5])
    const second = Number(dateTime[6])
    const millisecond = Number((dateTime[7] || '').padEnd(3, '0').slice(0, 3) || '0')
    const timezone = dateTime[8]
    if (!localDate(year, month, day) || hour > 23 || minute > 59 || second > 59) return null

    if (!timezone) {
      const date = new Date(0)
      date.setHours(hour, minute, second, millisecond)
      date.setFullYear(year, month - 1, day)
      return date
    }

    if (timezone !== 'Z' && timezone !== 'z') {
      const [offsetHour, offsetMinute] = timezone.slice(1).split(':').map(Number)
      if (offsetHour > 23 || offsetMinute > 59) return null
    }
    const fraction = dateTime[7] ? `.${String(millisecond).padStart(3, '0')}` : ''
    const iso = `${dateTime[1]}-${dateTime[2]}-${dateTime[3]}T${dateTime[4]}:${dateTime[5]}:${dateTime[6]}${fraction}${timezone.toUpperCase()}`
    const date = new Date(iso)
    return Number.isNaN(date.getTime()) ? null : date
  }

  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

function padded(value: number, width = 2): string {
  return String(value).padStart(width, '0')
}

function datePart(date: Date): string {
  return `${padded(date.getFullYear(), 4)}-${padded(date.getMonth() + 1)}-${padded(date.getDate())}`
}

function activeWeekdayLocale(): DateWeekdayLocale {
  if (typeof document !== 'undefined') {
    const locale = document.documentElement.lang as DateWeekdayLocale
    if (locale in WEEKDAY_LABELS) return locale
  }
  return 'zh-CN'
}

function dateWithWeekday(date: Date, locale: DateWeekdayLocale): string {
  const weekday = WEEKDAY_LABELS[locale][date.getDay()]
  const opening = locale === 'zh-CN' ? '（' : ' ('
  const closing = locale === 'zh-CN' ? '）' : ')'
  return `${datePart(date)}${opening}${weekday}${closing}`
}

export function formatDate(value: DateTimeValue, fallback = '-'): string {
  const date = parseDateTime(value)
  return date ? datePart(date) : fallback
}

export function formatDateTime(value: DateTimeValue, fallback = '-'): string {
  const date = parseDateTime(value)
  if (!date) return fallback

  return `${datePart(date)} ${padded(date.getHours())}:${padded(date.getMinutes())}:${padded(date.getSeconds())},${padded(date.getMilliseconds(), 3)}`
}

export function formatDateWithWeekday(value: DateTimeValue, fallback = '-', locale = activeWeekdayLocale()): string {
  const date = parseDateTime(value)
  return date ? dateWithWeekday(date, locale) : fallback
}

export function formatDateTimeWithWeekday(value: DateTimeValue, fallback = '-', locale = activeWeekdayLocale()): string {
  const date = parseDateTime(value)
  if (!date) return fallback

  return `${dateWithWeekday(date, locale)} ${padded(date.getHours())}:${padded(date.getMinutes())}:${padded(date.getSeconds())},${padded(date.getMilliseconds(), 3)}`
}
