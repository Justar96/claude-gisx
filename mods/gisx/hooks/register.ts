import type { Register, SessionContextBreakdown } from 'claude-code'

import { GISX } from './env'

// The status line's payload carries the context window's totals but never
// its make-up; /context's breakdown reaches mods alone. After each turn that
// moved the window, this hands that breakdown to the gisx binary, which keeps
// it beside the session for its next render. A `summary` breakdown is counted
// locally from the last response's usage, so it costs no API call.

export type Ingest = {
  v: 1
  session_id: string
  max_tokens: number
  total_tokens: number
  auto_compact_threshold: number | null
  categories: { name: string; tokens: number; kind: string; color: string }[]
}

export function toIngest(sessionId: string, b: SessionContextBreakdown): Ingest {
  return {
    v: 1,
    session_id: sessionId,
    max_tokens: b.rawMaxTokens,
    total_tokens: b.totalTokens,
    auto_compact_threshold: b.isAutoCompactEnabled ? (b.autoCompactThreshold ?? null) : null,
    categories: b.categories.map(c => ({ name: c.name, tokens: c.tokens, kind: c.kind, color: c.color })),
  }
}

export const register: Register = on => {
  on('session.measure', async ($, e, next) => {
    const result = await next(e)
    if (!e.changed.includes('context')) {
      return result
    }
    const usage = await $.session.usage({ breakdown: 'summary' })
    const b = usage.context.breakdown
    if (b === undefined) {
      return result
    }
    const payload = toIngest(await $.session.id(), b)
    const run = await $.process.run([GISX, 'ingest'], { stdin: JSON.stringify(payload), timeoutMs: 5000 })
    if (run.exitCode !== 0) {
      throw new Error(`${GISX} ingest exited ${run.exitCode}: ${run.stderr.trim()}`)
    }
    return result
  })
}
