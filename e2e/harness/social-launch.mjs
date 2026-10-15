// Real Wails dev application + local social service + isolated SSH transport.
// No application data is reset; every launch owns a new directory under .runtime.
import { spawn, spawnSync } from 'node:child_process';
import { copyFile, mkdir, writeFile } from 'node:fs/promises';
import { randomBytes } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import net from 'node:net';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const log = message => process.stdout.write(`[social-local] ${message}\n`);
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

async function free(port) {
  await new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once('error', () => reject(new Error(`端口 ${port} 已占用，请停止占用该端口的实例后重试`)));
    server.listen(port, '127.0.0.1', () => server.close(resolve));
  });
}
async function ready(url, timeout = 90000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try { if ((await fetch(url)).ok) return; } catch { /* starting */ }
    await delay(300);
  }
  throw new Error(`启动超时：${url}`);
}
function build(command, args, cwd = root) {
  const result = spawnSync(command, args, { cwd, stdio: 'inherit', shell: false, windowsHide: true });
  if (result.error || result.status !== 0) throw new Error(`构建失败：${command}`);
}

export async function startSocialReview({ port = 34115 } = {}) {
  if (process.platform !== 'win32') throw new Error('本地 Wails 实测脚本当前支持 Windows');
  for (const value of [port, port + 1, port + 2]) await free(value);
  const runDir = path.join(root, 'e2e/.runtime', `social-${Date.now()}`);
  const appDir = path.join(runDir, 'app'), serviceDir = path.join(runDir, 'service');
  await mkdir(path.join(appDir, 'docs'), { recursive: true });
  await mkdir(serviceDir, { recursive: true });
  const appURL = `http://127.0.0.1:${port}`, serviceURL = `http://127.0.0.1:${port + 1}`;
  const adminToken = randomBytes(32).toString('hex');
  const appConfig = {
    experimental: { companion_enabled: false },
    docs: { dir: path.join(appDir, 'docs') }, scripts: { dir: path.join(appDir, 'scripts') },
    log: { dir: path.join(appDir, 'logs') }, appearance: { theme: 'light' },
    llm: { BaseURL: 'http://127.0.0.1:1/v1', APIKey: '', Model: 'local-test-no-ai' },
    patch_store: { enabled: false }, session_share: { enabled: false },
  };
  await writeFile(path.join(appDir, 'config.json'), JSON.stringify(appConfig, null, 2));
  await writeFile(path.join(appDir, 'quick_commands.json'), '[]');
  await writeFile(path.join(appDir, 'service-center.json'), JSON.stringify({ baseUrl: '', choice: 'disabled' }));
  for (let i = 1; i <= 5; i++) await writeFile(path.join(appDir, 'docs', `guide-${i}.md`), `# 本地实测知识 ${i}\n\n用于验证查阅不同知识文件的计数。\n`);
  log('构建真实前端、Wails dev 后端、本地排名服务与 SSH 测试端点…');
  build('cmd.exe', ['/d', '/s', '/c', 'npm run build -- --logLevel warn'], path.join(root, 'frontend'));
  build('go', ['build', '-tags', 'dev', '-o', path.join(appDir, 'opscopilot-social.exe'), '.']);
  build('go', ['build', '-o', path.join(runDir, 'service-center.exe'), './cmd/service-center']);
  build('go', ['build', '-o', path.join(runDir, 'fakessh.exe'), './cmd/fakessh']);

  const processes = new Map();
  let stopping = false;
  function launch(name, exe, env) {
    const child = spawn(exe, [], { cwd: runDir, env: { ...process.env, ...env }, stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true });
    processes.set(name, child);
    child.stdout.on('data', data => process.stdout.write(`[${name}] ${data}`));
    child.stderr.on('data', data => process.stderr.write(`[${name}] ${data}`));
    child.on('error', error => { log(`${name} 启动失败：${error.message}`); stop(); process.exitCode = 1; });
    child.on('exit', code => {
      if (!stopping && processes.get(name) === child) { log(`${name} 意外退出 ${code}`); stop(); process.exitCode = 1; }
    });
    return child;
  }
  function kill(name) {
    const child = processes.get(name);
    processes.delete(name);
    if (child?.pid && child.exitCode === null) spawnSync('taskkill', ['/PID', String(child.pid), '/T', '/F'], { stdio: 'ignore', windowsHide: true });
  }
  function stop() { if (stopping) return; stopping = true; for (const name of [...processes.keys()]) kill(name); }
  const serviceEnv = { OPS_SERVICE_DATA: serviceDir, OPS_SERVICE_LISTEN: `127.0.0.1:${port + 1}`, OPS_SERVICE_PUBLIC_URL: serviceURL, OPS_SERVICE_ADMIN_TOKEN: adminToken, OPS_SERVICE_LOCAL_HTTP: 'true' };
  const appEnv = { devserver: `127.0.0.1:${port}`, assetdir: path.join(root, 'frontend/dist'), LLM_API_KEY: '', LLM_BASE_URL: 'http://127.0.0.1:1/v1', LLM_MODEL: 'local-test-no-ai' };
  async function startService() { launch('social-service', path.join(runDir, 'service-center.exe'), serviceEnv); await ready(`${serviceURL}/healthz`); }
  async function startApp() { launch('desktop', path.join(appDir, 'opscopilot-social.exe'), appEnv); await ready(appURL); }
  process.once('exit', stop);
  process.once('SIGINT', () => { stop(); process.exit(130); });
  process.once('SIGTERM', () => { stop(); process.exit(143); });
  try {
    await startService();
    const members = {};
    for (const [key, displayName, organizationId] of [['self', '本地实测成员', 'local-review'], ['peer', '同组织实测成员', 'local-review'], ['outsider', '其他组织实测成员', 'other-review']]) {
      const response = await fetch(`${serviceURL}/api/admin/social/members`, { method: 'POST', headers: { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ displayName, organizationId }) });
      if (!response.ok) throw new Error(`创建本地实测账号失败：${response.status}`);
      members[key] = await response.json();
    }
    launch('ssh-test', path.join(runDir, 'fakessh.exe'), { FAKESSH_ADDR: `127.0.0.1:${port + 2}`, FAKESSH_SFTP_ROOT: path.join(appDir, 'docs'), FAKESSH_BANNER: '== local social SSH transport ==\r\n' });
    await startApp();
    const endpoint = { appURL, serviceURL, runDir, appDir, serviceDir, members, sshPort: port + 2 };
    // Test credentials only; this file is inside ignored isolated runtime state.
    await writeFile(path.join(runDir, 'endpoint.json'), JSON.stringify(endpoint, null, 2));
    log(`已启动 ${appURL}/；数据目录 ${runDir}`);
    async function startPeerApp() {
      await free(port + 3);
      const peerDir = path.join(runDir, 'peer-app'), peerURL = `http://127.0.0.1:${port + 3}`;
      await mkdir(path.join(peerDir, 'docs'), { recursive: true });
      await copyFile(path.join(appDir, 'opscopilot-social.exe'), path.join(peerDir, 'opscopilot-social.exe'));
      await writeFile(path.join(peerDir, 'config.json'), JSON.stringify({ ...appConfig, docs: { dir: path.join(peerDir, 'docs') }, scripts: { dir: path.join(peerDir, 'scripts') }, log: { dir: path.join(peerDir, 'logs') } }, null, 2));
      await writeFile(path.join(peerDir, 'quick_commands.json'), '[]');
      await writeFile(path.join(peerDir, 'service-center.json'), JSON.stringify({ baseUrl: '', choice: 'disabled' }));
      launch('peer-desktop', path.join(peerDir, 'opscopilot-social.exe'), { ...appEnv, devserver: `127.0.0.1:${port + 3}` });
      await ready(peerURL);
      endpoint.peerURL = peerURL;
      await writeFile(path.join(runDir, 'endpoint.json'), JSON.stringify(endpoint, null, 2));
      return peerURL;
    }
    return { endpoint, stop, startPeerApp, stopService: () => kill('social-service'), startService, restartApp: async () => { kill('desktop'); await startApp(); } };
  } catch (error) { stop(); throw error; }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await startSocialReview();
  log('按 Ctrl+C 停止本次实测环境。');
}
