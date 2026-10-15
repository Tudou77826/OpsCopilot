/** A bounded promise cache deduplicates simultaneous loads; failed entries remain retryable. */
export function createAssetCache<T>(limit: number) {
  if (!Number.isInteger(limit) || limit < 1) throw new Error('Asset cache limit must be a positive integer');
  const entries = new Map<string, Promise<T>>();
  return {
    get size() { return entries.size; },
    get(key: string, load: () => Promise<T>): Promise<T> {
      const cached = entries.get(key);
      if (cached) { entries.delete(key); entries.set(key, cached); return cached; }
      const pending = Promise.resolve().then(load);
      entries.set(key, pending);
      while (entries.size > limit) entries.delete(entries.keys().next().value!);
      void pending.catch(() => { if (entries.get(key) === pending) entries.delete(key); });
      return pending;
    },
  };
}
