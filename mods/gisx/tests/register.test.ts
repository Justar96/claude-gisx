import { expect, test } from 'claude-code/testing'
import type { ProcessRunResult, SessionContextBreakdown, SessionUsage } from 'claude-code'

const breakdown: SessionContextBreakdown = {
  categories: [
    { name: 'System prompt', tokens: 9_000, color: 'promptBorder', isDeferred: false, kind: 'used' },
    { name: 'Messages', tokens: 112_000, color: 'purple_FOR_SUBAGENTS_ONLY', isDeferred: false, kind: 'used' },
    { name: 'Free space', tokens: 253_000, color: 'promptBorder', isDeferred: false, kind: 'free' },
    { name: 'Autocompact buffer', tokens: 13_000, color: 'inactive', isDeferred: false, kind: 'buffer' },
  ],
  totalTokens: 121_000,
  maxTokens: 387_000,
  rawMaxTokens: 387_000,
  autocompactSource: 'settings',
  percentage: 31,
  gridRows: [],
  model: 'claude-opus-5-5',
  memoryFiles: [],
  mcpTools: [],
  agents: [],
  autoCompactThreshold: 374_000,
  isAutoCompactEnabled: true,
  apiUsage: null,
}

const usage: SessionUsage = {
  startedAt: 0,
  context: { tokens: 121_000, window: 1_000_000, percent: 12, breakdown },
  rateLimits: [],
}

const ran: ProcessRunResult = { exitCode: 0, stdout: '', stderr: '', isStdoutTruncated: false, isStderrTruncated: false }

test('hands the breakdown to claude-gisx ingest when the context moved', async ($, on) => {
  const calls: { argv: readonly string[]; stdin?: string }[] = []
  on('session.measure', ($, e) => ({ changed: e.changed }))
  on('session.usage', () => ({ value: usage }))
  on('session.id', () => ({ value: 'b1d2c3e4-0000-4000-8000-000000000001' }))
  on('process.run', ($, e) => {
    calls.push({ argv: e.argv, stdin: e.init?.stdin })
    return { value: ran }
  })

  await $.session.measure({ context: { tokens: 121_000, window: 1_000_000, percent: 12 }, rateLimits: [], changed: ['context'] })

  expect(calls.length).toBe(1)
  const [call] = calls
  expect(call?.argv).toEqual(['claude-gisx', 'ingest'])
  expect(JSON.parse(call?.stdin ?? '')).toEqual({
    v: 1,
    session_id: 'b1d2c3e4-0000-4000-8000-000000000001',
    max_tokens: 387_000,
    total_tokens: 121_000,
    auto_compact_threshold: 374_000,
    categories: [
      { name: 'System prompt', tokens: 9_000, kind: 'used', color: 'promptBorder' },
      { name: 'Messages', tokens: 112_000, kind: 'used', color: 'purple_FOR_SUBAGENTS_ONLY' },
      { name: 'Free space', tokens: 253_000, kind: 'free', color: 'promptBorder' },
      { name: 'Autocompact buffer', tokens: 13_000, kind: 'buffer', color: 'inactive' },
    ],
  })
})

test('stays quiet when only the rate limits moved', async ($, on) => {
  let runs = 0
  on('session.measure', ($, e) => ({ changed: e.changed }))
  on('process.run', () => {
    runs += 1
    return { value: ran }
  })

  await $.session.measure({ context: { window: 1_000_000 }, rateLimits: [], changed: ['rateLimits'] })

  expect(runs).toBe(0)
})

test('skips ingest when the engine gives no breakdown', async ($, on) => {
  let runs = 0
  on('session.measure', ($, e) => ({ changed: e.changed }))
  on('session.usage', () => ({ value: { ...usage, context: { tokens: 121_000, window: 1_000_000, percent: 12 } } }))
  on('process.run', () => {
    runs += 1
    return { value: ran }
  })

  await $.session.measure({ context: { tokens: 121_000, window: 1_000_000, percent: 12 }, rateLimits: [], changed: ['context'] })

  expect(runs).toBe(0)
})

test('a failed ingest leaves the measurement whole', async ($, on) => {
  on('session.measure', ($, e) => ({ changed: e.changed }))
  on('session.usage', () => ({ value: usage }))
  on('session.id', () => ({ value: 'b1d2c3e4-0000-4000-8000-000000000001' }))
  on('process.run', () => ({ value: { ...ran, exitCode: 1, stderr: 'ingest: unknown version 1\n' } }))

  // The engine skips the hook and reports its error; the session goes on.
  const measured = await $.session.measure({ context: { tokens: 121_000, window: 1_000_000, percent: 12 }, rateLimits: [], changed: ['context'] })
  expect(measured).toEqual({ changed: ['context'] })
})
