import { OpsError } from './errors.js'

export interface ConnectionDraft { name: string; host: string; port: number; user: string; protocol: 'ssh'; group: string }
export function connectionDraft(input: unknown): ConnectionDraft {
  if (!input || typeof input !== 'object' || Array.isArray(input)) throw new OpsError('INVALID_ARGUMENT', '连接参数无效')
  const value = input as Record<string, unknown>
  const str = (key: string, fallback = '', max = 255) => {
    const text = value[key] ?? fallback
    if (typeof text !== 'string' || text.length > max || /[\0\r\n]/.test(text)) throw new OpsError('INVALID_ARGUMENT', '连接字段无效')
    return text.trim()
  }
  const host = str('host'), user = str('user'), port = value.port ?? 22
  if (!host || !user || !/^[A-Za-z0-9._:[\]%-]+$/.test(host) || !Number.isInteger(port) || Number(port) < 1 || Number(port) > 65535) throw new OpsError('INVALID_ARGUMENT', '主机、端口或用户名无效')
  if ((value.protocol && value.protocol !== 'ssh') || value.bastion) throw new OpsError('UNSUPPORTED_CAPABILITY', '此连接入口仅支持直连 SSH')
  return { host, user, port: Number(port), protocol: 'ssh', name: str('name', `${user}@${host}`, 128) || `${user}@${host}`, group: str('group', '', 128) }
}

export function sessionPassword(value: unknown): string | undefined {
  if (value === undefined) return undefined
  if (typeof value !== 'string' || value.length > 8192 || value.includes('\0')) throw new OpsError('INVALID_ARGUMENT', '密码参数无效')
  return value
}
