/** Whether the caret is on the first or last visual line of a textarea. */
export function atHistoryBoundary(input: HTMLTextAreaElement, direction: 'up' | 'down'): boolean {
    const position = input.selectionStart;
    if (position !== input.selectionEnd) return false;
    const value = input.value;
    if (direction === 'up' && value.slice(0, position).includes('\n')) return false;
    if (direction === 'down' && value.slice(position).includes('\n')) return false;
    if (!value || (direction === 'up' ? position === 0 : position === value.length)) return true;

    // A logical line may wrap. Measure the caret and the edge with the same
    // typography and width so that arrows still navigate wrapped text normally.
    const bounds = input.getBoundingClientRect();
    if (!bounds.width) return true; // Non-layout environments, e.g. jsdom.
    const style = getComputedStyle(input);
    const mirror = document.createElement('div');
    for (const property of ['box-sizing','border-top-width','border-right-width','border-bottom-width','border-left-width',
        'padding-top','padding-right','padding-bottom','padding-left','font-family','font-size','font-weight','font-style',
        'font-stretch','font-variant','line-height','letter-spacing','word-spacing','text-indent','text-transform','tab-size',
        'word-break','overflow-wrap','direction'] as const) mirror.style.setProperty(property, style.getPropertyValue(property));
    Object.assign(mirror.style, {position:'fixed', visibility:'hidden', pointerEvents:'none', top:'0', left:'0',
        width:`${bounds.width}px`, height:'auto', whiteSpace:'pre-wrap', overflow:'hidden', borderStyle:'solid'});
    const first = document.createElement('span'), caret = document.createElement('span'), last = document.createElement('span');
    first.textContent = caret.textContent = last.textContent = '\u200b';
    mirror.append(first, document.createTextNode(value.slice(0, position)), caret, document.createTextNode(value.slice(position)), last);
    document.body.appendChild(mirror);
    try {
        const current = caret.getBoundingClientRect().top;
        const edge = (direction === 'up' ? first : last).getBoundingClientRect().top;
        return Math.abs(current - edge) < 1;
    } finally { mirror.remove(); }
}
