// 组件层一次性探针的跑法：把一份 .tsx 探针连同本包源码束成 CJS，在 happy-dom 的真 DOM 里跑一遍。
//
//   node scripts/dom-probe.mjs <probe.tsx>
//
// 探针源放仓外（%TEMP% 之类），源与产物都不入库——它证的是这一次的完成判据，不是长期门禁；长期要钉的
// 纯逻辑抬进 .ts 用 node:test（run-tests.mjs）。探针里 import 本包源码写绝对路径即可，react / react-dom
// 从本包 node_modules 解。探针自己决定怎么报：console 打 ok / FAIL，非零退出即失败，本脚本原样透传退出码。
// 探针正文写在一个 async 函数里再调用——束的是 CJS，顶层 await 编不过。
//
// 为什么要这么一层而不是 node:test 或 renderToStaticMarkup：
//   - .test.ts 编成 CJS 后 require 不到 import 了 @idpxyz/* 的模块（那两包 exports 只给 types + import）；
//   - renderToStaticMarkup 只有一帧，证不了「keydown → state → 重渲」这类事件路径，而壳层接线的完成判据
//     恰恰是这类（票 admin-web-workspace-form/03 的 Ctrl+K 就是第一例）。
//   - happy-dom 装在仓外的固定目录，本包 lockfile 零改动；首次跑要出网装一次（约 4 秒），本机 npm 出网
//     要走代理（见 docs/agents/workflow.md 本机环境「GitHub 只能走代理」那条，同一个代理）。
//
// 探针拿到的全局：happy-dom 的 window / document（url http://localhost/#/workbench）、永远挂起的 fetch（页面层
// 的读口不出网、也不让失败态抢渲染）、IS_REACT_ACT_ENVIRONMENT = true（用 react 的 act 包事件不告警）、
// matchMedia 兜底桩。
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { dirname, isAbsolute, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const appRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const probeArg = process.argv[2];
if (!probeArg) {
  console.error('用法：node scripts/dom-probe.mjs <probe.tsx>');
  process.exit(2);
}
const probePath = isAbsolute(probeArg) ? probeArg : resolve(process.cwd(), probeArg);
if (!existsSync(probePath)) {
  console.error(`探针不存在：${probePath}`);
  process.exit(2);
}

// happy-dom 的版本钉死：探针的结论要能跨机器复现，DOM 实现换了版本就不是同一份证据。
const HAPPY_DOM_VERSION = '20.14.5';
const harnessDir = join(tmpdir(), 'idp-admin-web-dom-probe');
mkdirSync(harnessDir, { recursive: true });
if (!existsSync(join(harnessDir, 'package.json'))) {
  writeFileSync(join(harnessDir, 'package.json'), '{"name":"idp-admin-web-dom-probe","private":true}\n');
}
if (!existsSync(join(harnessDir, 'node_modules', '@happy-dom', 'global-registrator'))) {
  console.log(`首次运行：往 ${harnessDir} 装 happy-dom@${HAPPY_DOM_VERSION}…`);
  const npm = spawnSync(
    process.platform === 'win32' ? 'npm.cmd' : 'npm',
    [
      'install',
      `happy-dom@${HAPPY_DOM_VERSION}`,
      `@happy-dom/global-registrator@${HAPPY_DOM_VERSION}`,
      '--no-audit',
      '--no-fund',
      '--loglevel=error',
    ],
    { cwd: harnessDir, stdio: 'inherit', shell: process.platform === 'win32' },
  );
  if (npm.status !== 0) process.exit(npm.status ?? 1);
}

writeFileSync(
  join(harnessDir, 'setup-dom.ts'),
  `import { GlobalRegistrator } from '@happy-dom/global-registrator';
GlobalRegistrator.register({ url: 'http://localhost/#/workbench', width: 1440, height: 900 });
const pending = () => new Promise<Response>(() => {});
(globalThis as any).fetch = pending;
(window as any).fetch = pending;
(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
if (!(window as any).matchMedia) {
  (window as any).matchMedia = () => ({
    matches: false, media: '', onchange: null,
    addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {},
    dispatchEvent: () => false,
  });
}
`,
);
// 入口先注册 DOM 再进探针：ESM 按 import 顺序求值，@idpxyz/* 在模块顶层就摸 window 的那些才不会先炸。
const entryPath = join(harnessDir, 'entry.tsx');
writeFileSync(entryPath, `import './setup-dom';\nimport ${JSON.stringify(probePath.replace(/\\/g, '/'))};\n`);

// esbuild 不是本包的直接依赖，从 vite 自己的解析上下文里拿——vite 装着的那一份就是 vite build 用的，同一版本。
const viteRequire = createRequire(createRequire(join(appRoot, 'package.json')).resolve('vite/package.json'));
const esbuild = viteRequire('esbuild');
const outfile = join(harnessDir, 'probe.cjs');
try {
  await esbuild.build({
    entryPoints: [entryPath],
    outfile,
    bundle: true,
    platform: 'node',
    format: 'cjs',
    jsx: 'automatic',
    loader: { '.css': 'empty' },
    external: ['happy-dom', '@happy-dom/global-registrator'],
    nodePaths: [join(appRoot, 'node_modules')],
    tsconfig: join(appRoot, 'tsconfig.json'),
    logLevel: 'warning',
  });
} catch {
  // esbuild 已按 logLevel 把错误逐条打到 stderr，这里只定退出码，不再叠一遍堆栈。
  process.exit(1);
}

// 在 harness 目录里跑，两个 external 从它的 node_modules 解；NODE_ENV 留给 react 自己读，开发版才有 act。
const run = spawnSync(process.execPath, [outfile], {
  cwd: harnessDir,
  stdio: 'inherit',
  env: { ...process.env, NODE_ENV: process.env.NODE_ENV ?? 'development' },
});
process.exit(run.status ?? 1);
