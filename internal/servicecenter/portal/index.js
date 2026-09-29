const observer=new IntersectionObserver(es=>es.forEach(e=>{if(e.isIntersecting){e.target.classList.add('visible');observer.unobserve(e.target)}}),{threshold:.08});document.querySelectorAll('.reveal').forEach(el=>observer.observe(el));const viewer=document.getElementById('imageViewer'),viewerImage=document.getElementById('imageViewerImage'),viewerLabel=document.getElementById('imageViewerLabel'),viewerClose=viewer.querySelector('.image-viewer-close');let lastZoomTrigger=null;const openViewer=trigger=>{lastZoomTrigger=trigger;viewerImage.src=trigger.dataset.fullImage;viewerImage.alt=trigger.dataset.imageLabel;viewerLabel.textContent=trigger.dataset.imageLabel;viewer.classList.add('open');viewer.setAttribute('aria-hidden','false');document.body.classList.add('viewer-open');viewer.scrollTop=0;viewerClose.focus()};const closeViewer=()=>{viewer.classList.remove('open');viewer.setAttribute('aria-hidden','true');document.body.classList.remove('viewer-open');lastZoomTrigger?.focus()};document.querySelectorAll('.zoom-trigger').forEach(trigger=>{trigger.setAttribute('aria-label',`查看${trigger.dataset.imageLabel}大图`);trigger.addEventListener('click',()=>openViewer(trigger));trigger.addEventListener('keydown',e=>{if(e.key==='Enter'||e.key===' '){e.preventDefault();openViewer(trigger)}})});viewerClose.addEventListener('click',closeViewer);viewer.addEventListener('click',e=>{if(e.target===viewer)closeViewer()});document.addEventListener('keydown',e=>{if(e.key==='Escape'&&viewer.classList.contains('open'))closeViewer()});

const element = (tag, text, className) => {
  const node = document.createElement(tag);
  if (text != null) node.textContent = text;
  if (className) node.className = className;
  return node;
};
const fileSize = bytes => bytes > 0 ? (bytes / 1048576).toFixed(1) + ' MB' : '';
function releaseNotes(release) {
  const notes = element('div', null, 'release-notes');
  // body_html is produced by the server from Markdown with HTML/images disabled.
  // Limit DOM tags again before adoption; no remote media, forms or executable elements.
  const parsed = new DOMParser().parseFromString(release.body_html || '', 'text/html');
  const allowed = new Set(['P','H1','H2','H3','H4','H5','H6','UL','OL','LI','STRONG','EM','DEL','CODE','PRE','BLOCKQUOTE','A','BR','HR','TABLE','THEAD','TBODY','TR','TH','TD']);
  for (const node of Array.from(parsed.body.querySelectorAll('*')).reverse()) {
    if (!allowed.has(node.tagName)) { node.remove(); continue; }
    const href = node.tagName === 'A' ? node.getAttribute('href') : null;
    for (const attr of Array.from(node.attributes)) node.removeAttribute(attr.name);
    if (href) {
      try {
        const url = new URL(href, location.origin);
        if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) continue;
        node.href = url.href;
        if (url.origin !== location.origin) {
          node.target = '_blank'; node.rel = 'noopener noreferrer'; node.className = 'external-note-link';
          node.title = '外网链接，需要能够访问外网';
          if (node.textContent.trim() === href) {
            const pull = url.pathname.match(/\/pull\/(\d+)$/);
            node.textContent = pull ? '变更记录 #' + pull[1] : url.hostname === 'github.com' && url.pathname.includes('/compare/') ? '完整变更记录' : url.hostname + ' 原文';
          }
        }
      } catch { /* Leave invalid links as plain text. */ }
    }
  }
  const first = parsed.body.firstElementChild;
  if (first && /^H[1-6]$/.test(first.tagName) && [release.tag_name,release.name].includes(first.textContent.trim())) first.remove();
  let technical = null;
  for (const child of Array.from(parsed.body.children)) {
    if (/^H[1-6]$/.test(child.tagName) && /^what[’']?s changed$/i.test(child.textContent.trim())) {
      technical = element('details', null, 'release-technical');
      technical.append(element('summary', '开发者变更记录 · GitHub 链接需外网'));
      notes.append(technical); continue;
    }
    (technical || notes).append(child);
  }
  if (!notes.textContent.trim()) notes.append(element('p', release.body || '暂无更新说明'));
  return notes;
}
(async () => {
  const list = document.getElementById('release-list');
  try {
    const response = await fetch('/api/v1/releases');
    if (!response.ok) throw new Error('版本读取失败');
    const releases = await response.json();
    list.replaceChildren();
    if (!releases?.length) { list.textContent = '暂无已同步的安装包，请稍后再试或联系管理员。'; return; }
    for (const [index, release] of releases.entries()) {
      const section = element('article', null, 'release');
      const header = element('div', null, 'release-header');
      const identity = element('div', null, 'release-identity');
      identity.append(element('h3', release.tag_name || release.name));
      if (index === 0) identity.append(element('span', '最新版本', 'release-badge'));
      header.append(identity);
      const published = new Date(release.published_at);
      if (!Number.isNaN(published.getTime()) && published.getFullYear() > 2000) {
        const time = element('time', published.toLocaleDateString('zh-CN', {year:'numeric',month:'2-digit',day:'2-digit'}), 'release-date');
        time.dateTime = release.published_at; header.append(time);
      }
      section.append(header);
      const grid = element('div', null, 'release-content');
      grid.append(releaseNotes(release));
      const downloads = element('aside', null, 'release-downloads');
      downloads.append(element('h4', '下载安装包'));
      for (const asset of release.assets || []) {
        let url;
        try { url = new URL(asset.browser_download_url, location.origin); } catch { continue; }
        if (url.origin !== location.origin || !url.pathname.startsWith('/downloads/') || url.username || url.password) continue;
        const link = element('a', null, 'release-download'); link.href = url.href;
        const label = /\.zip$/i.test(asset.name) ? '下载 Windows 压缩包' : /\.exe$/i.test(asset.name) ? '下载 Windows 可执行文件' : '下载 ' + asset.name;
        link.append(element('strong', label + ' ↓'), element('span', asset.name + (fileSize(asset.size) ? ' · ' + fileSize(asset.size) : '')));
        downloads.append(link);
      }
      if (downloads.children.length === 1) downloads.append(element('p','该版本暂无可下载的安装包。'));
      downloads.append(element('p', '安装包通过内网下载。', 'release-download-hint'));
      grid.append(downloads); section.append(grid); list.append(section);
    }
  } catch { list.textContent = '暂时无法读取版本，请稍后重试。'; }
})();
