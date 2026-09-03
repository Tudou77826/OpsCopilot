import { OpsError } from './errors.js'
export function workspacePath(value: unknown) {
  if (typeof value !== 'string' || value.length > 4096 || /[\\:\x00-\x1f]/.test(value) || value.split('/').includes('..')) throw new OpsError('INVALID_ARGUMENT', '文件区路径无效')
  return value || '/'
}
export function remotePath(value: unknown, destructive = false) {
  if (typeof value !== 'string' || !value || value.length > 4096 || /[\x00-\x1f]/.test(value)) throw new OpsError('INVALID_ARGUMENT', '远端路径无效')
  if (destructive && (value.split('/').some(v => v === '..') || !value.replace(/[/.]/g, ''))) throw new OpsError('FORBIDDEN', '不能修改远端根目录或父目录')
  return value
}
export type Transfer = { taskId: string; sessionId: string; state: 'running' | 'completed' | 'cancelled' | 'failed' | 'interrupted'; bytesDone: number; bytesTotal: number; ok?: boolean; cancelled?: boolean; message?: string }
