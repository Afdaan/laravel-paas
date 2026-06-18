/* eslint-disable react-refresh/only-export-components */
import React from 'react'
import { siMysql, siPostgresql } from 'simple-icons'
import { cn } from '@/lib/utils'

export function DatabaseEngineIcon({ engine, className }: { engine?: string; className?: string }) {
  const norm = (engine || '').toLowerCase().trim();
  let icon = siMysql;
  if (norm.includes('post') || norm.includes('pg')) {
    icon = siPostgresql;
  }

  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      className={cn('w-5 h-5 shrink-0', className)}
      xmlns="http://www.w3.org/2000/svg"
    >
      <path fill={`#${icon.hex}`} d={icon.path} />
    </svg>
  );
}

export const getEngineDisplayName = (engine?: string) => {
  const norm = (engine || '').toLowerCase().trim();
  if (norm.includes('post') || norm.includes('pg')) {
    return 'PostgreSQL';
  }
  return 'MySQL';
};

export const parseDbType = (dbType: string) => {
  const typeLower = dbType.toLowerCase();
  let length: number | string = 255;
  const match = typeLower.match(/\((\d+)\)/);
  if (match) {
    length = parseInt(match[1], 10);
  }

  if (typeLower.includes('varchar') || typeLower.includes('string') || typeLower.includes('char')) {
    return { type: 'varchar', length };
  }
  if (typeLower.includes('tinyint(1)') || typeLower.includes('bool') || typeLower.includes('boolean')) {
    return { type: 'boolean', length: '' };
  }
  if (typeLower.includes('bigint')) {
    return { type: 'bigint', length: '' };
  }
  if (typeLower.includes('int') || typeLower.includes('integer')) {
    return { type: 'integer', length: '' };
  }
  if (typeLower.includes('text')) {
    return { type: 'text', length: '' };
  }
  if (typeLower.includes('timestamp') || typeLower.includes('datetime') || typeLower.includes('date')) {
    return { type: 'timestamp', length: '' };
  }
  if (typeLower.includes('decimal')) {
    return { type: 'decimal', length: '' };
  }
  return { type: 'varchar', length: 255 };
};

export const toLocalISOString = (d: Date): string => {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hours = String(d.getHours()).padStart(2, '0')
  const minutes = String(d.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day}T${hours}:${minutes}`
}

export const toLocalDateString = (d: Date): string => {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

export const adjustDatetimeForDatabase = (inputValue: string): string => {
  if (!inputValue) return inputValue

  try {
    // inputValue from datetime-local is like "2026-06-01T15:45" (local time).
    // If it's just a date (length 10), append T00:00 to force parsing as local time,
    // otherwise JS parses "YYYY-MM-DD" as UTC midnight.
    const strToParse = inputValue.length === 10 ? `${inputValue}T00:00` : inputValue
    const d = new Date(strToParse)
    if (isNaN(d.getTime())) return inputValue

    // toISOString() returns UTC like "2026-06-01T08:45:00.000Z"
    return d.toISOString().slice(0, 19).replace('T', ' ')
  } catch (e) {
    return inputValue
  }
}

export const formatDatetimeLocal = (val: unknown) => {
  if (val === null || val === undefined) return ''
  try {
    if (val instanceof Date) {
      if (isNaN(val.getTime())) return ''
      return toLocalISOString(val)
    }

    const strVal = String(val).trim()
    if (!strVal || strVal.startsWith('0000-00-00')) return ''

    // Assume database timestamp is UTC, append Z so Date parses it as UTC
    let parsedStr = strVal.includes(' ') ? strVal.replace(' ', 'T') : strVal
    if (!parsedStr.endsWith('Z') && !/[+-]\d{2}:\d{2}$/.test(parsedStr)) {
      parsedStr += 'Z'
    }

    const d = new Date(parsedStr)
    if (isNaN(d.getTime())) return ''

    // toLocalISOString converts it to the format expected by datetime-local input
    return toLocalISOString(d)
  } catch (e) {
    return ''
  }
}

export const formatDate = (val: unknown) => {
  if (val === null || val === undefined) return ''
  try {
    if (val instanceof Date) {
      if (isNaN(val.getTime())) return ''
      return toLocalDateString(val)
    }

    const strVal = String(val).trim()
    if (!strVal) return ''

    // If it's already in the exact format YYYY-MM-DD, return it
    if (/^\d{4}-\d{2}-\d{2}$/.test(strVal)) {
      return strVal
    }

    // Try parsing Unix timestamp if it's numeric
    if (/^\d+$/.test(strVal)) {
      const num = Number(strVal)
      const date = new Date(strVal.length === 10 ? num * 1000 : num)
      if (!isNaN(date.getTime())) {
        return toLocalDateString(date)
      }
    }

    // Try matching YYYY-MM-DD from the beginning
    const regex = /^(\d{4})-(\d{2})-(\d{2})/i
    const match = strVal.match(regex)
    if (match) {
      const [, year, month, day] = match
      return `${year}-${month}-${day}`
    }

    // Fallback: standard Date parsing
    const parsedStr = strVal.includes(' ') && !strVal.includes('T') ? strVal.replace(' ', 'T') : strVal
    const d = new Date(parsedStr)
    if (isNaN(d.getTime())) return ''
    return toLocalDateString(d)
  } catch (e) {
    return ''
  }
}

export const formatHumanDatetime = (val: unknown) => {
  if (val === null || val === undefined) return ''
  try {
    const strVal = String(val).trim()
    if (!strVal || strVal.startsWith('0000-00-00') || strVal.startsWith('0001-01-01')) return ''

    // Assume database timestamp is UTC, append Z so Date parses it as UTC
    let parsedStr = strVal.includes(' ') ? strVal.replace(' ', 'T') : strVal
    if (!parsedStr.endsWith('Z') && !/[+-]\d{2}:\d{2}$/.test(parsedStr)) {
      parsedStr += 'Z'
    }

    const d = new Date(parsedStr)
    if (isNaN(d.getTime())) return strVal

    const year = d.getFullYear()
    const monthNames = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
    const month = monthNames[d.getMonth()]
    const day = String(d.getDate()).padStart(2, '0')
    const hours = String(d.getHours()).padStart(2, '0')
    const minutes = String(d.getMinutes()).padStart(2, '0')
    const seconds = String(d.getSeconds()).padStart(2, '0')

    return `${day} ${month} ${year}, ${hours}:${minutes}:${seconds}`
  } catch (e) {
    return String(val)
  }
}

export const formatHumanDate = (val: unknown) => {
  if (val === null || val === undefined) return ''
  try {
    const strVal = String(val).trim()
    if (!strVal || strVal.startsWith('0000-00-00')) return ''

    // Extract YYYY-MM-DD directly without parsing as a Date object.
    // Parsing "YYYY-MM-DD" natively in JS converts it to UTC midnight,
    // which causes date-shifting bugs (showing yesterday) for users in timezones behind UTC.
    const regex = /^(\d{4})-(\d{2})-(\d{2})/
    const match = strVal.match(regex)
    if (match) {
      const [, year, month, day] = match
      const monthNames = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
      return `${day} ${monthNames[Number(month) - 1]} ${year}`
    }

    return strVal
  } catch (e) {
    return String(val)
  }
}

export const formatCellValue = (val: unknown, columnType?: string): React.ReactNode => {
  if (val === null || val === undefined) {
    return React.createElement('span', { className: 'text-muted-foreground/30 italic' }, 'NULL')
  }

  const strVal = String(val).trim()
  if (!strVal || strVal.startsWith('0000-00-00') || strVal.startsWith('0001-01-01')) {
    return React.createElement('span', { className: 'text-muted-foreground/30 italic' }, 'NULL')
  }

  // Handle explicit date column type
  const lowerType = columnType?.toLowerCase() || ''
  if (lowerType === 'date') {
    return formatHumanDate(strVal)
  }

  // Check if it matches ISO datetime or DB space-separated datetime
  const datetimeRegex = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|[+-]\d{2}(?::?\d{2})?)?$/i
  if (datetimeRegex.test(strVal)) {
    return formatHumanDatetime(strVal)
  }

  // Check if it matches YYYY-MM-DD date-only
  const dateRegex = /^(\d{4})-(\d{2})-(\d{2})$/
  if (dateRegex.test(strVal)) {
    return formatHumanDate(strVal)
  }

  return strVal
}
