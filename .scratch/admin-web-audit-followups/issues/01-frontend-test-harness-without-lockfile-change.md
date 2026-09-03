# 01 前端测试底座——零新依赖，不动 `pnpm-lock.yaml`

Category: enhancement
Status: resolved（2026-09-03，MCP-2，`d722301`）
Blocked by: 无

## 缺什么

`apps/admin-web` 没有一个测试文件（取证 `0d492b8`：全树无 `*.test.*` / `*.spec.*`），也没有
测试运行器。四态判读（`classifyMasterDataResponse` 的五格结果代数）、词表、各页行转写全靠
`tsc --noEmit` 与人工看页面在守。

## 为什么不是「加 vitest」

`docs/agents/workflow.md` 本机环境一节：`pnpm install` 带 `--frozen-lockfile` 被拒
（lockfile 的 `overrides` 指向别的宿主上的 `file:` 路径），不带则**重写 lockfile**，属「扫当前树
再写回去」的全仓派生态命令，共享树上不得单方面跑。所以本票**一个新依赖都不加**：

- 运行器用 Node 22 自带的 `node:test` + `node:assert/strict`（本机 `v22.22.0`）。
- 源码 import 无扩展名（`from './api'`），Node ESM 解析不了，`--experimental-strip-types` 也不
  改这一条（实测 `node --test` 直接跑 `.ts` 在 `./a` 上失败）。因此**先编译再跑**：
  `tsconfig.test.json` 继承主配置，改 `module: CommonJS`、`noEmit: false`、输出到 `.tmp-test/`，
  只 `include` 测试文件，tsc 按依赖拉进被测模块。
- 本机没有 `@types/node`（`node_modules/@types` 下只有 react 两个），`import 'node:test'` 在
  `tsc` 下会报找不到模块。补一份**最小的**环境声明 `src/test/node-builtins.d.ts`，只声明用到的
  几个签名。它是替身不是实现：`@types/node` 进来那天整份删掉。

**边界**：只测纯 `.ts` 模块（`api.ts`、`presentation.ts`、`catalogue-api.ts`、`catalogue-view.ts`
及本批抽出的行转写模块）。不引入 DOM 替身、不渲染组件——页面 `.tsx` 在 Node 里 `require`
`@idpxyz/*` 会撞 `window`，那一层留给 vitest + jsdom 那天。

## 做什么

1. `apps/admin-web/tsconfig.test.json`、`package.json` 加 `test` 脚本、`apps/admin-web/.gitignore`
   忽略 `.tmp-test/`。
2. `src/test/node-builtins.d.ts` 最小声明。
3. 第一片测试落在 `src/pages/catalogue-api.test.ts`：stub `globalThis.fetch`，钉
   `exchangeMasterData` / `postMasterData` 的五格结果代数——`403 + ACCESS_CHANNEL_NOT_CONFIGURED`
   → `unconfigured`；其余 4xx → `callerProblem` 带 code；5xx → `noAnswer`；非 JSON → `transport`；
   `2xx` → `outcome`。期望值取 ADR-0022 / ADR-0055 的原句，不从实现推。

## 完成判据

- `pnpm test` 在 `apps/admin-web` 下跑通，输出 `node:test` 的 TAP 汇总，`fail 0`。
- `tsc --noEmit -p .`（主配置）仍无输出——测试文件被主配置一并检查，没有 `@ts-ignore`。
- 票 02 的红绿循环能在这套底座上跑。

## Comments

- 2026-09-03 · MCP-2：落于 `d722301`（父 `0d492b8`）。`pnpm test` → tsc 发射 + `node --test`
  `.tmp-test/**/*.test.js`，11 例 `fail 0`；主配置 `tsc --noEmit -p .` 无输出（98 + 1 文件）。
  两处实测改了票面原设想：`node --test <目录>` 在 v22.22 不收目录，改传 glob；产物目录要单独
  一份 `{"type":"commonjs"}`，否则 `"type": "module"` 的包会把 tsc 发的 CJS 当 ESM 读。
  **只在本机验过**（Windows，Node v22.22.0）；CI 尚无非 Go job（README 已知跟进），这套底座
  进 CI 那天要一并核 Node ≥ 22 与 glob 传参。
