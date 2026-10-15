import {expect,it,vi} from 'vitest';
import {createAssetCache} from './assetCache';
it('deduplicates concurrent loads and evicts least recently used resolved entries',async()=>{
 const cache=createAssetCache<string>(2),load=vi.fn(async()=> 'asset');
 const first=cache.get('a',load);expect(cache.get('a',load)).toBe(first);await first;expect(load).toHaveBeenCalledTimes(1);
 await cache.get('b',load);cache.get('a',load);await cache.get('c',load);
 expect(cache.size).toBe(2);expect(cache.get('a',load)).toBe(first);
 await cache.get('b',load);expect(load).toHaveBeenCalledTimes(4);expect(cache.size).toBe(2);
});
it('retries failures without letting an evicted failed load erase a newer entry',async()=>{
 const cache=createAssetCache<string>(1);let fail!:(cause:Error)=>void;
 const old=cache.get('a',()=>new Promise<string>((_,reject)=>{fail=reject;}));await Promise.resolve();
 await cache.get('b',async()=> 'b');const replacement=cache.get('a',async()=> 'new');
 fail(new Error('offline'));await expect(old).rejects.toThrow('offline');
 expect(cache.get('a',async()=> 'wrong')).toBe(replacement);expect(await replacement).toBe('new');
 const reject=vi.fn(async()=>{throw new Error('retry');});await expect(cache.get('failed',reject)).rejects.toThrow('retry');
 await expect(cache.get('failed',async()=> 'recovered')).resolves.toBe('recovered');
});
