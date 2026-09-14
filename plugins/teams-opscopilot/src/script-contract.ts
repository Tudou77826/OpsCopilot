import { OpsError } from './errors.js'

function invalid(): never { throw new OpsError('INVALID_ARGUMENT', '脚本内容或变量无效') }
function record(value: unknown): Record<string, any> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return invalid()
  return value as Record<string, any>
}
function str(value: unknown, max: number, fallback = ''): string {
  const result = value ?? fallback
  if (typeof result !== 'string' || result.length > max || result.includes('\0')) return invalid()
  return result
}
function list(value: unknown, max: number): unknown[] {
  if (value == null) return []
  if (!Array.isArray(value) || value.length > max) return invalid()
  return value
}
export function scriptChanges(value: unknown) {
  const sc = record(value)
  const name = str(sc.name, 128)
  if (!name.trim()) return invalid()
  // Canonical steps prevent deleted steps from resurrecting via legacy commands.
  const steps = list(sc.steps ?? sc.commands, 1000).map(value => {
    const step = record(value), delay = step.delay ?? 0
    if (!Number.isSafeInteger(delay) || delay < 0 || delay > 300000 || typeof step.enabled !== 'boolean') return invalid()
    return { command: str(step.command ?? step.content, 65535), comment: str(step.comment, 4096), delay, enabled: step.enabled }
  })
  const names = new Set<string>()
  const variables = list(sc.variables, 128).map(value => {
    const variable = record(value), name = str(variable.name, 128)
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name) || names.has(name) || typeof variable.required !== 'boolean') return invalid()
    names.add(name)
    return { name, display_name: str(variable.display_name, 128), default_value: str(variable.default_value, 8192), required: variable.required, description: str(variable.description, 4096) }
  })
  return { name, description: str(sc.description, 4096), steps, variables, commands: [] }
}

export function replayValues(value: unknown): Record<string, string> {
  const values = record(value ?? {}), result: Record<string, string> = Object.create(null)
  if (Object.keys(values).length > 128) return invalid()
  for (const [name, value] of Object.entries(values)) {
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) return invalid()
    result[name] = str(value, 8192)
  }
  return result
}

export interface ReplayState { id: string; scriptId: string; terminalId: string; state: 'running' | 'stopping' | 'dispatched' | 'stopped' | 'failed' | 'interrupted' | 'unknown'; sent: number; total: number }
export function activeReplay(state: string) { return state === 'running' || state === 'stopping' || state === 'unknown' }
