"""Bundle the existing product homepage for offline service-center hosting."""
from pathlib import Path
import re
import shutil

root = Path(__file__).resolve().parents[2]
out = root / 'internal/servicecenter/portal'
out.mkdir(parents=True, exist_ok=True)
for source, name in [('opscopilot-product-homepage.html', 'index'), ('opscopilot-feature-checklist.html', 'features')]:
    html = (root / 'docs' / source).read_text(encoding='utf-8-sig')
    css = '\n'.join(re.findall(r'<style>(.*?)</style>', html, re.S))
    js = '\n'.join(re.findall(r'<script>(.*?)</script>', html, re.S))
    html = re.sub(r'<style>.*?</style>', f'<link rel="stylesheet" href="/portal/{name}.css">', html, flags=re.S)
    html = re.sub(r'<script>.*?</script>', f'<script src="/portal/{name}.js" defer></script>', html, flags=re.S)
    styles = []
    def external_style(match):
        styles.append(match.group(1))
        return f' data-style="{len(styles)-1}"'
    html = re.sub(r' style="([^"]*)"', external_style, html)
    css += '\n' + '\n'.join(f'[data-style="{i}"]{{{v}}}' for i,v in enumerate(styles))
    html = html.replace('../frontend/src/assets/images/logo-universal.png', '/portal/assets/logo-universal.png')
    html = html.replace('opscopilot-feature-checklist.html', '/portal/features.html')
    html = html.replace('assets/latest-', '/portal/assets/latest-')
    if name == 'index':
        html = html.replace('https://github.com/Tudou77826/OpsCopilot/releases/latest', '#downloads')
        html = html.replace('https://github.com/Tudou77826/OpsCopilot/releases', '#downloads')
        html = html.replace('<b>48 RELEASES</b>', '<b>持续更新</b>')
        html = html.replace('<a class="btn btn-green nav-download"', '<a href="/help">使用帮助</a><a href="/feedback">反馈</a><a class="btn btn-green nav-download"')
        html = html.replace('  </main>', '    <section class="downloads wrap" id="downloads"><div class="kicker">内网版本镜像</div><h2>下载与更新日志</h2><p>安装包由内网服务器同步提供，无需访问 GitHub。</p><div id="release-list" aria-live="polite">正在读取版本…</div></section>\n  </main>')
        css += '\n.downloads{padding-top:80px;padding-bottom:80px;scroll-margin-top:100px}.release{padding:24px 0;border-bottom:1px solid var(--line)}.release pre{white-space:pre-wrap;overflow-wrap:anywhere;font:inherit;color:var(--muted)}.release-links{display:flex;gap:12px;flex-wrap:wrap}.navlinks{flex-wrap:wrap} @media(prefers-reduced-motion:reduce){.reveal{opacity:1;transform:none;transition:none}}\n'
        js += '\n' + (root / 'deploy/service-center/portal-releases.js').read_text(encoding='utf-8')
    (out / f'{name}.html').write_text(html, encoding='utf-8')
    (out / f'{name}.css').write_text('\n'.join(line.rstrip() for line in css.splitlines()).rstrip() + '\n', encoding='utf-8')
    (out / f'{name}.js').write_text(js, encoding='utf-8')
assets = out / 'assets'
assets.mkdir(exist_ok=True)
for filename in set(re.findall(r'/portal/assets/([^"\s]+)', (out / 'index.html').read_text(encoding='utf-8'))):
    source = root / ('frontend/src/assets/images/logo-universal.png' if filename == 'logo-universal.png' else f'docs/assets/{filename}')
    shutil.copyfile(source, assets / filename)
