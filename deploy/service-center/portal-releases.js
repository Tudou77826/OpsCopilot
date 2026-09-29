(async () => {
  const list = document.getElementById('release-list');
  try {
    const response = await fetch('/api/v1/releases');
    if (!response.ok) throw new Error('版本读取失败');
    const releases = await response.json();
    list.replaceChildren();
    if (!releases?.length) { list.textContent = '暂无已同步的安装包，请稍后再试或联系管理员。'; return; }
    for (const release of releases) {
      const section = document.createElement('article'); section.className = 'release';
      const title = document.createElement('h3'); title.textContent = release.name || release.tag_name; section.append(title);
      const links = document.createElement('div'); links.className = 'release-links';
      for (const asset of release.assets || []) {
        const url = new URL(asset.browser_download_url, location.origin);
        if (url.origin !== location.origin || !url.pathname.startsWith('/downloads/')) continue;
        const link = document.createElement('a'); link.className = 'btn btn-green'; link.href = url.href; link.textContent = asset.name; links.append(link);
      }
      section.append(links);
      const notes = document.createElement('pre'); notes.textContent = release.body || '暂无更新说明'; section.append(notes);
      list.append(section);
    }
  } catch { list.textContent = '暂时无法读取版本，请稍后重试。'; }
})();
