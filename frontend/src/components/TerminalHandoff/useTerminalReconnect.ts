import { useCallback, useEffect, useRef } from 'react';

// A reconnect reply belongs to the tab that requested it. Closing the tab,
// leaving the page or unmounting prevents late replies from installing listeners
// or showing errors; repeated clicks share the already pending request.
export function useTerminalReconnect() {
    const pending = useRef(new Map<string, symbol>());
    const active = useRef(true);
    useEffect(() => {
        active.current = true;
        const leave = () => { active.current = false; pending.current.clear(); };
        const resume = () => { active.current = true; };
        window.addEventListener('pagehide', leave);
        window.addEventListener('pageshow', resume);
        return () => {
            leave();
            window.removeEventListener('pagehide', leave);
            window.removeEventListener('pageshow', resume);
        };
    }, []);
    const cancel = useCallback((id: string) => { pending.current.delete(id); }, []);
    return {
        cancel,
        run: async <T,>(id: string, request: () => Promise<T>, accept: (value: T) => void, reject: (error: unknown) => void) => {
            if (!active.current || pending.current.has(id)) return;
            const operation = Symbol(id);
            pending.current.set(id, operation);
            const current = () => active.current && pending.current.get(id) === operation;
            try {
                const result = await request();
                if (current()) accept(result);
            } catch (error) {
                if (current()) reject(error);
            } finally {
                if (current()) pending.current.delete(id);
            }
        },
    };
}
