import { expect, it } from 'vitest';
import desktopStyles from '../App.css?raw';
import desktopApp from '../App.tsx?raw';
import productChrome from '../../../frontend-shell/src/ui/product/ProductChrome.tsx?raw';

it('desktop delegates theme definitions to the shared Shell stylesheet', () => {
    expect(desktopStyles).toContain("@import '../../frontend-shell/src/ui/styles/shell-theme.css'");
    expect(desktopStyles).not.toMatch(/--[\w-]+\s*:/);
    expect(desktopApp).toContain("from '../../frontend-shell/src/ui/product/ProductChrome'");
    expect(desktopApp).toContain('toolbar={<ProductToolbar');
    expect(productChrome).toContain('className="shell-brand-logo"');
});
