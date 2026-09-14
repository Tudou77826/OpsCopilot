import { randomUUID } from 'node:crypto'
import { OpsError } from './errors.js'

export interface OpsEvent {
  schemaVersion: 1
  instanceId: string
  sequence: number
  generation: number
  type: string
  resourceId?: string
  payload: Record<string, string | number | boolean>
}

/** Only business-authored, allowlisted metadata enters this journal, never raw RPC events. */
export class EventJournal {
  readonly instanceId = randomUUID()
  private sequence = 0
  private entries: OpsEvent[] = []
  constructor(private readonly generation: () => number, private readonly capacity = 256) {}

  cursor() { return { instanceId: this.instanceId, generation: this.generation(), sequence: this.sequence } }
  emit(type: string, resourceId?: string, payload: OpsEvent['payload'] = {}) {
    this.entries.push({ schemaVersion: 1, ...this.cursor(), sequence: ++this.sequence, type, resourceId, payload: { ...payload } })
    if (this.entries.length > this.capacity) this.entries.shift()
  }
  since(input: Record<string, unknown>) {
    if (typeof input.instanceId !== 'string' || !Number.isSafeInteger(input.generation) || !Number.isSafeInteger(input.sequence) || Number(input.sequence) < 0) throw new OpsError('INVALID_ARGUMENT', '事件游标无效')
    const cursor = this.cursor(), sequence = Number(input.sequence)
    const resync = input.instanceId !== cursor.instanceId || input.generation !== cursor.generation || sequence > this.sequence || sequence < (this.entries[0]?.sequence ?? 1) - 1
    return { schemaVersion: 1, ...cursor, resync, events: resync ? [] : this.entries.filter(event => event.sequence > sequence).map(event => ({ ...event, payload: { ...event.payload } })) }
  }
}
