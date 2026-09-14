export type ErrorCode = 'INVALID_ARGUMENT' | 'FORBIDDEN' | 'NOT_FOUND' | 'BUSY' | 'TIMEOUT' | 'PROTOCOL_MISMATCH' | 'RUNTIME_UNAVAILABLE' | 'OPERATION_FAILED' | 'UNSUPPORTED_CAPABILITY'

/** Public errors never include raw RPC payloads, process stderr or SSH credentials. */
export class OpsError extends Error {
  constructor(readonly code: ErrorCode, message: string) { super(message); this.name = 'OpsError' }
}
