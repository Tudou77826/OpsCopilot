// E2E 拉起器：为一次测试运行组装完整的被测环境。
//
//  fakessh(带 SFTP 沙箱) ─┐
//   fakessh(无 SFTP)     ─┼─ 全部杀进程级隔离，端口固定，状态每次运行重置
//   opscopilot dev exe   ─┘   （exe 所在目录 = 应用配置目录，与开发实例互不相干）
//
// 应用实例不是 `wails dev`，而是直接 `go build -tags dev` 出的 exe：
// wails v2 的 dev 版 exe 读环境变量 devserver/assetdir 自行托管 web 界面
// （internal/app/app_dev.go），因此无需 wails CLI、无需 vite，秒级热缓存启动，
// 且配置天然落在自己的 exe 目录里。前端用构建产物 frontend/dist（assetdir），
// 与生产形态一致。
//
// 退出语义：作为 Playwright webServer 运行，进程存活期间监管子进程；
// 收到终止信号或任一子进程意外退出时，taskkill 整棵进程树。
import { spawn, spawnSync } from 'node:child_process';
import { mkdir, rm, writeFile, readdir } from 'node:fs/promises';
import { existsSync, readFileSync } from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..', '..');
const runtimeDir = path.join(__dirname, '..', '.runtime');
const appDir = path.join(runtimeDir, 'app');
const sftpRoot = path.join(runtimeDir, 'sftp-root');
const localDir = path.join(runtimeDir, 'local');
const downloadDir = path.join(localDir, 'dl');

const APP_PORT = 34315;
const FAKESSH_SFTP_PORT = 34316;
const FAKESSH_NOSFTP_PORT = 34317;
const FAKESSH_BULK_PORT = 34318;
const APP_URL = `http://127.0.0.1:${APP_PORT}/`;

const UPLOAD_PAYLOAD = 'e2e-upload-payload-v1\n';
const DOWNLOAD_PAYLOAD = 'e2e-download-payload-v1\n';

// 刷屏端点的横幅：150 行日志由服务端在 shell 建立时主动下发，
// 模拟 grep 大输出的"服务器→客户端"方向（不是客户端粘贴）。
const BULK_TOKEN_EARLY = 'TOKEN-EARLY-7f3a';
const bulkBanner = (() => {
  const lines = [];
  for (let i = 0; i < 150; i++) {
    lines.push(
      i === 2
        ? `2026-09-27T10:00:02 ERROR [auth] failed login attempt marker=${BULK_TOKEN_EARLY} user=probe\r\n`
        : `2026-09-27T10:00:${String(i).padStart(2, '0')} INFO [worker${i % 7}] heartbeat ok seq=${1000 + i}\r\n`,
    );
  }
  return lines.join('');
})();

function log(msg) {
  process.stdout.write(`[launch] ${msg}\n`);
}

function die(msg) {
  process.stderr.write(`[launch] ${msg}\n`);
  process.exit(1);
}

function checkPortFree(port, what) {
  return new Promise((resolve) => {
    const sock = net.connect({ host: '127.0.0.1', port }, () => {
      sock.destroy();
      die(`${what} 端口 ${port} 已被占用——可能是上一次 E2E 没退干净（taskkill /IM opscopilot-e2e.exe /IM fakessh.exe /F 后重试）。`);
    });
    sock.on('error', () => resolve());
  });
}

function waitForTcp(port, what, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  return new Promise((resolve, reject) => {
    (function tryOnce() {
      const sock = net.connect({ host: '127.0.0.1', port }, () => {
        sock.destroy();
        resolve();
      });
      sock.on('error', () => {
        if (Date.now() > deadline) reject(new Error(`等待 ${what}(${port}) 超时`));
        else setTimeout(tryOnce, 300);
      });
    })();
  });
}

async function waitForHttp(url, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch { /* 尚未监听 */ }
    if (Date.now() > deadline) die(`等待应用 ${url} 超时（看上方 exe 输出）`);
    await new Promise((r) => setTimeout(r, 400));
  }
}

function run(cmd, args, opts = {}) {
  const r = spawnSync(cmd, args, { stdio: 'inherit', shell: false, ...opts });
  if (r.status !== 0) die(`命令失败：${cmd} ${args.join(' ')}`);
}

// ── 构建（Go 构建缓存热时约几秒；每次重建以覆盖最新代码——防护网的意义所在） ──
const skipBuild = ['1', 'true'].includes((process.env.E2E_SKIP_BUILD ?? '').toLowerCase());
if (!skipBuild) {
  log('go build（dev exe + fakessh）…');
  run('go', ['build', '-tags', 'dev', '-o', path.join(appDir, 'opscopilot-e2e.exe'), '.'], { cwd: repoRoot });
  run('go', ['build', '-o', path.join(runtimeDir, 'fakessh.exe'), './cmd/fakessh'], { cwd: repoRoot });
}

// ── 状态重置 + 沙箱播种 ──
async function resetState() {
  await Promise.all([rm(sftpRoot, { recursive: true, force: true }), rm(localDir, { recursive: true, force: true })]);
  // 应用配置目录：清掉所有 json/锁/日志，保留 exe —— 每次运行都是"全新装机"状态
  for (const f of await readdir(appDir)) {
    if (f.endsWith('.json') || f.endsWith('.lock') || f.endsWith('.log') || f.startsWith('sessions.json')) {
      await rm(path.join(appDir, f), { force: true });
    }
  }
  await mkdir(sftpRoot, { recursive: true });
  await mkdir(downloadDir, { recursive: true });
  await mkdir(path.join(appDir, 'docs'), { recursive: true }); // 知识库目录缺省时启动会记 ERROR，占位消音
  await writeFile(path.join(sftpRoot, 'download-src.txt'), DOWNLOAD_PAYLOAD);
  await writeFile(path.join(localDir, 'upload-src.txt'), UPLOAD_PAYLOAD);
  log('状态已重置，沙箱已播种');
}

// ── 子进程监管 ──
const children = [];
function supervise(name, exe, args, env) {
  const child = spawn(exe, args, {
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
    windowsHide: true,
  });
  children.push({ name, child });
  child.on('error', (err) => {
    if (shuttingDown) return;
    die(`子进程 ${name} 启动失败: ${err.message}`);
  });
  child.stdout.on('data', (d) => process.stdout.write(`[${name}] ${d}`));
  child.stderr.on('data', (d) => process.stderr.write(`[${name}] ${d}`));
  child.on('exit', (code) => {
    if (shuttingDown) return;
    die(`子进程 ${name} 意外退出（code=${code}），终止整个运行`);
  });
  log(`${name} 已启动 pid=${child.pid}`);
  return child;
}

let shuttingDown = false;
function killAll() {
  if (shuttingDown) return;
  shuttingDown = true;
  for (const { name, child } of children) {
    if (child.exitCode === null && child.pid) {
      spawnSync('taskkill', ['/PID', String(child.pid), '/T', '/F'], { stdio: 'ignore' });
      log(`${name} 已终止`);
    }
  }
}
process.on('SIGINT', () => { killAll(); process.exit(130); });
process.on('SIGTERM', () => { killAll(); process.exit(143); });
process.on('exit', killAll);

// ── 主流程 ──
await checkPortFree(APP_PORT, '应用');
await checkPortFree(FAKESSH_SFTP_PORT, 'fakessh(SFTP)');
await checkPortFree(FAKESSH_NOSFTP_PORT, 'fakessh(无SFTP)');
await checkPortFree(FAKESSH_BULK_PORT, 'fakessh(刷屏)');
await resetState();

supervise('fakessh-sftp', path.join(runtimeDir, 'fakessh.exe'), [], {
  FAKESSH_ADDR: `127.0.0.1:${FAKESSH_SFTP_PORT}`,
  FAKESSH_SFTP_ROOT: sftpRoot,
  FAKESSH_BANNER: '== e2e sftp endpoint ==\r\n',
});
supervise('fakessh-nosftp', path.join(runtimeDir, 'fakessh.exe'), [], {
  FAKESSH_ADDR: `127.0.0.1:${FAKESSH_NOSFTP_PORT}`,
  FAKESSH_BANNER: '== e2e nosftp endpoint ==\r\n',
});
supervise('fakessh-bulk', path.join(runtimeDir, 'fakessh.exe'), [], {
  FAKESSH_ADDR: `127.0.0.1:${FAKESSH_BULK_PORT}`,
  FAKESSH_BANNER: bulkBanner,
});

// devserver: wails dev exe 自托管地址；assetdir: 直接服务 frontend/dist 构建产物
supervise('app', path.join(appDir, 'opscopilot-e2e.exe'), [], {
  devserver: `127.0.0.1:${APP_PORT}`,
  assetdir: path.join(repoRoot, 'frontend', 'dist'),
});

await waitForTcp(FAKESSH_SFTP_PORT, 'fakessh(SFTP)', 10_000).catch(die);
await waitForTcp(FAKESSH_NOSFTP_PORT, 'fakessh(无SFTP)', 10_000).catch(die);
await waitForTcp(FAKESSH_BULK_PORT, 'fakessh(刷屏)', 10_000).catch(die);
log('三个 SSH 端点就绪');
await waitForHttp(APP_URL, 90_000);

await writeFile(
  path.join(runtimeDir, 'endpoint.json'),
  JSON.stringify(
    {
      appUrl: APP_URL,
      appDir,
      sftp: { host: '127.0.0.1', port: FAKESSH_SFTP_PORT, user: 'test', password: 'test', root: sftpRoot },
      nosftp: { host: '127.0.0.1', port: FAKESSH_NOSFTP_PORT, user: 'test', password: 'test' },
      bulk: { host: '127.0.0.1', port: FAKESSH_BULK_PORT, user: 'test', password: 'test', earlyToken: BULK_TOKEN_EARLY, lastMarker: 'seq=1149' },
      fixtures: { localDir, downloadDir, uploadSrc: path.join(localDir, 'upload-src.txt'), uploadPayload: UPLOAD_PAYLOAD, downloadPayload: DOWNLOAD_PAYLOAD },
    },
    null,
    2,
  ),
);
log(`环境就绪：${APP_URL}`);

// 存活直至被终止（Playwright 会在测试结束后杀掉本进程 → killAll 清理子进程）
setInterval(() => {
  if (!existsSync(path.join(appDir, 'opscopilot-e2e.exe'))) die('app exe 消失了？');
}, 5000);
