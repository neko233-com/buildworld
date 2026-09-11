import { describe, expect, it } from 'vitest'
import { formatDate, formatDateTime, formatDateTimeWithWeekday, formatDateWithWeekday } from './dateTime'

describe('dateTime presentation', () => {
  it('formats local calendar dates with fixed-width fields', () => {
    expect(formatDate(new Date(2026, 0, 2, 3, 4, 5, 6))).toBe('2026-01-02')
  })

  it('formats local timestamps with seconds and milliseconds', () => {
    expect(formatDateTime(new Date(2026, 0, 2, 3, 4, 5, 6))).toBe('2026-01-02 03:04:05,006')
  })

  it('adds a localized weekday and omits milliseconds', () => {
    const date = new Date(2026, 6, 22, 3, 4, 5, 6)
    expect(formatDateWithWeekday(date, '-', 'zh-CN')).toBe('2026-07-22 星期三')
    expect(formatDateTimeWithWeekday(date, '-', 'zh-CN')).toBe('2026-07-22 03:04:05 星期三')
    expect(formatDateWithWeekday(date, '-', 'en')).toBe('2026-07-22 Wednesday')
  })

  it('keeps the weekday helpers aligned with timezone-aware parsing', () => {
    const value = '2026-07-22T10:20:30Z'
    expect(formatDateTimeWithWeekday(value, '-', 'zh-CN')).toBe(formatDateTimeWithWeekday(new Date(value), '-', 'zh-CN'))
  })

  it.each([
    ['2026-07-22 10:20:30', '2026-07-22 10:20:30,000'],
    ['2026-07-22 10:20:30.7', '2026-07-22 10:20:30,700'],
    ['2026-07-22 10:20:30.07', '2026-07-22 10:20:30,070'],
    ['2026-07-22 10:20:30.007', '2026-07-22 10:20:30,007'],
  ])('accepts SQL-style local timestamps and normalizes milliseconds: %s', (value, expected) => {
    expect(formatDateTime(value)).toBe(expected)
  })

  it('parses date-only values as local calendar dates without timezone drift', () => {
    expect(formatDate('2026-07-22')).toBe('2026-07-22')
    expect(formatDateTime('2026-07-22')).toBe('2026-07-22 00:00:00,000')
  })

  it.each([
    '2026-07-22T10:20:30Z',
    '2026-07-22T10:20:30.123+08:00',
    '2026-07-22T10:20:30.456-05:30',
  ])('renders RFC 3339 values in the browser local timezone: %s', value => {
    expect(formatDateTime(value)).toBe(formatDateTime(new Date(value)))
  })

  it.each([
    null,
    undefined,
    '',
    'not-a-date',
    '2026-02-29',
    '2026-02-29 10:00:00',
    '2026-01-01 24:00:00',
    '2026-01-01T10:00:00+24:00',
  ])('uses the fallback for missing or invalid values: %s', value => {
    expect(formatDate(value)).toBe('-')
    expect(formatDateTime(value, '未知')).toBe('未知')
    expect(formatDateWithWeekday(value, '未知', 'zh-CN')).toBe('未知')
    expect(formatDateTimeWithWeekday(value, '未知', 'zh-CN')).toBe('未知')
  })
})
