import type { CompanionHost, CompanionSnapshot } from './types';

/** Host transport only; growth and public cards remain backend-owned. */
export function makeCompanionHost(call: (method: string, params?: Record<string, unknown>) => Promise<unknown>): CompanionHost {
  const unwrap = (value: unknown) => {
    const body = value as CompanionSnapshot | { companion: CompanionSnapshot };
    return 'companion' in body ? body.companion : body;
  };
  return {
    snapshot: async () => unwrap(await call('snapshot')),
    hatch: async revision => unwrap(await call('hatch', { revision })),
    rename: async (name, revision) => unwrap(await call('rename', { name, revision })),
  };
}
