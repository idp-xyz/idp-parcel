// 测试入口（票 admin-web-audit-followups/01）：清空上次产物 → tsc 按 tsconfig.test.json 发射到
// .tmp-test/ → node --test 跑那里的 *.test.js。
//
// 为什么不是一句 `tsc && node --test`：
//   - 本包 package.json 是 "type": "module"，.tmp-test/ 下的 .js 会被 Node 当 ESM 读，而 tsc 发的是
//     CommonJS；这里给产物目录单独写一份 {"type":"commonjs"} 把它扳回来。
//   - 删掉的测试会在 .tmp-test/ 留下过时 .js 继续被跑，先清目录再编。
//   - 本机 node_modules/.bin/tsc 垫片缺失（见 docs/agents/workflow.md 本机环境），走 typescript
//     包的入口文件对装好垫片的机器同样成立。
import { rmSync, mkdirSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const appRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const outDir = join(appRoot, '.tmp-test');

rmSync(outDir, { recursive: true, force: true });
mkdirSync(outDir, { recursive: true });
writeFileSync(join(outDir, 'package.json'), '{"type":"commonjs"}\n');

const tsc = spawnSync(
  process.execPath,
  [join(appRoot, 'node_modules', 'typescript', 'bin', 'tsc'), '-p', join(appRoot, 'tsconfig.test.json')],
  { stdio: 'inherit', cwd: appRoot },
);
if (tsc.status !== 0) process.exit(tsc.status ?? 1);

// `--test` 不收目录只收文件或 glob；glob 由 Node 自己展开，不经 shell。
const tests = spawnSync(process.execPath, ['--test', '.tmp-test/**/*.test.js'], {
  stdio: 'inherit',
  cwd: appRoot,
});
process.exit(tests.status ?? 1);
