import { useLayoutEffect, useRef, useState, type RefObject } from 'react';
import { TbArrowDown } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';

/** Separate from scroll content; hold the target steady through pointer/focus
 * interaction even if an approval, queued message or new reply resizes it. */
export function LatestMessageButton({ anchor, visible, onClick }: {
    anchor: RefObject<HTMLDivElement>; visible: boolean; onClick(): void;
}) {
    const [position, setPosition] = useState({ left: 0, top: 0 });
    const [held, setHeld] = useState(false);
    const engaged = useRef(false);
    const pressed = useRef(false);
    const button = useRef<HTMLButtonElement>(null);
    const active = visible || held;
    const measure = useRef(() => {});
    useLayoutEffect(() => {
        const element = anchor.current;
        if (!element || !active) return;
        const update = () => {
            if (engaged.current) return;
            const rect = element.getBoundingClientRect();
            setPosition({ left: rect.left + (rect.width - 34) / 2, top: Math.max(rect.top, rect.bottom - 46) });
        };
        measure.current = update;
        update();
        const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(update);
        observer?.observe(element);
        window.addEventListener('resize', update);
        window.addEventListener('scroll', update, true);
        return () => { observer?.disconnect(); window.removeEventListener('resize', update); window.removeEventListener('scroll', update, true); };
    }, [anchor, active]);
    function release() { engaged.current = false; pressed.current = false; setHeld(false); measure.current(); }
    useLayoutEffect(() => {
        if (!held) return;
        const endOutside = (event: PointerEvent) => {
            // An outside release has no click to wait for. Inside releases are
            // handled by onClick so the button survives until it is delivered.
            if (!button.current?.contains(event.target as Node)) release();
        };
        window.addEventListener('pointerup', endOutside);
        window.addEventListener('pointercancel', release);
        window.addEventListener('blur', release);
        return () => {
            window.removeEventListener('pointerup', endOutside);
            window.removeEventListener('pointercancel', release);
            window.removeEventListener('blur', release);
        };
    }, [held]);
    if (!active) return null;
    return <IconButton ref={button} label="回到最新" className="ai-chat-latest" style={position}
        onPointerEnter={() => { engaged.current = true; setHeld(true); }}
        onPointerLeave={() => { if (!pressed.current) release(); }}
        onPointerDown={event => {
            if (event.button !== 0) return;
            // Avoid browser focus scrolling before click is delivered.
            event.preventDefault(); engaged.current = true; pressed.current = true; setHeld(true);
        }}
        onPointerUp={() => { pressed.current = false; }}
        onPointerCancel={release}
        onFocus={() => { engaged.current = true; setHeld(true); }}
        onBlur={release}
        onClick={() => { onClick(); release(); }}><TbArrowDown size={18} /></IconButton>;
}
