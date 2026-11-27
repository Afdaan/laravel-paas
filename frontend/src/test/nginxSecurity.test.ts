import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const nginxConfig = readFileSync(resolve(process.cwd(), 'nginx.conf'), 'utf8')
const cspHeader = nginxConfig
  .split('\n')
  .find((line) => line.includes('add_header Content-Security-Policy-Report-Only'))

const directive = (name: string) => {
  const match = cspHeader?.match(new RegExp(`${name} ([^;\"]+)`))
  return match?.[1]?.trim() ?? ''
}

describe('frontend Nginx security headers', () => {
  it('sets CSP report-only on SPA document responses', () => {
    expect(cspHeader).toBeDefined()
    expect(cspHeader).toContain(' always;')
  })

  it('keeps JavaScript restricted to the Runara origin', () => {
    expect(directive('script-src')).toBe("'self'")
  })

  it('allows only the external image host used by GitHub avatars', () => {
    expect(directive('img-src')).toBe("'self' data: blob: https://avatars.githubusercontent.com")
  })

  it('blocks unsafe document capabilities', () => {
    expect(directive('object-src')).toBe("'none'")
    expect(directive('frame-ancestors')).toBe("'none'")
    expect(directive('base-uri')).toBe("'self'")
    expect(directive('form-action')).toBe("'self'")
  })
})
