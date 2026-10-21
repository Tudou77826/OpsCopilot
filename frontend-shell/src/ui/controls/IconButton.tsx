import { forwardRef, type ButtonHTMLAttributes } from 'react';
import { TbBooks } from 'react-icons/tb';
import './iconButton.css';

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
    label: string;
    tone?: 'default' | 'primary' | 'danger';
};

export const IconButton = forwardRef<HTMLButtonElement, Props>(function IconButton(
    { label, title = label, tone = 'default', className = '', type = 'button', children, ...props }, ref,
) {
    return <button {...props} ref={ref} type={type} aria-label={label} title={title}
        className={`oc-icon-button is-${tone} ${className}`.trim()}>{children}</button>;
});

/** Books on a shelf identify the library without suggesting copy or settings. */
export function ResourceLibraryIcon({ size = 18 }: { size?: number }) {
    return <TbBooks size={size} aria-hidden="true" />;
}
