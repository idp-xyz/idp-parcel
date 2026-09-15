# admin-web `tsc --noEmit` 在 main 上已红：sa-cc/19 给 `DutyVerificationRecord` 加了必填 `fundsVersion`，`register-rows.test.ts` 的 `verification()` 夹具没跟——而 CI 没有 admin-web 的类型检查步，这一红自 19 进 main 起无人看见

Category: bug
Status: ready-for-agent——**2026-09-15 15:4x 通道 1 立票**（sa-cc/31 作者通道 3 完成记录判断项 (7b) 报出、推送方在 `0b3f0831` 复现：`apps/admin-web` 下 `node node_modules/typescript/bin/tsc --noEmit` 退 2，唯一报错 `src/pages/customs/register-rows.test.ts` `verification()` 返回值缺 `fundsVersion`）。只一份测试夹具，零行为
Blocked by: 无（[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md) 已进 main `49ffc96c`——`fundsVersion` 进 `api.ts` `DutyVerificationRecord` 的那一笔在 main 是 `e0201ca6`）

## 缺口（取证于 `0b3f0831`）

- `apps/admin-web/src/pages/customs/api.ts` `DutyVerificationRecord.fundsVersion: string`（19 加的：核对记录带它核对比的那版资金事实，头注写「fundsVersion 必填」）。
- `apps/admin-web/src/pages/customs/register-rows.test.ts` `verification(over: Partial<DutyVerificationRecord>): DutyVerificationRecord` 的字面量没有 `fundsVersion`，`...over` 是 `Partial`，所以整体类型是 `fundsVersion?: string | undefined` → TS2322。`tsc --noEmit` 退 2。
- 为什么没人看见：`.github/workflows/ci.yml` 没有 admin-web 的 `tsc` / `pnpm` 步；`internal/architecture/admin_web_endpoint_consumers_gate_test.go` 数的是端点表与消费方对应，不编译 TS。19 进 main（12:22）之后每一批的「全仓绿」都不含它。31 的作者是在跑自己票面判据 5 时撞到的。

## 做法

1. `verification()` 字面量加 `fundsVersion: 'SYN-FUNDS-01/v1'`（与同夹具 `funds: 'SYN-FUNDS-01'` 同一事实、版本字面照 CC 夹具惯例）。若该文件别处的用例对「缺 `fundsVersion`」有断言，以代码为准写判断项。
2. **不改 `api.ts` 的类型**：19 的头注把它定为必填，类型是对的，错的是夹具。

## 红线

- 零行为：只动 `register-rows.test.ts`；`api.ts` / `presentation.ts` / 页面组件零 diff。
- 不加 CI 步——要不要给 CI 加 admin-web 类型检查是另一张票（CI 计费归用户）。

## 完成判据

1. `apps/admin-web` 下 `node node_modules/typescript/bin/tsc --noEmit` 退 0。
2. `git diff --stat -- ':!apps/admin-web/src/pages/customs/register-rows.test.ts' ':!.scratch'` 空。
3. 完成记录同笔；清点零差（TS 不在清点内）。

## 地盘

`apps/admin-web/src/pages/customs/register-rows.test.ts` + 票 .md。撞点：31（`apps/admin-web/src/pages/settlement/`）不同目录；34（CC Go）零重叠。

## 另记（不成票，归用户）

- CI 没有 admin-web 的类型检查步——`tsc --noEmit` 红了三个多小时无人看见。加一步的代价是 CI 时长与计费；判据 5 一类「本机 `tsc` 0」今天只靠作者自跑。

## 参照

[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md)；[31](31-sa-external-funds-fact-registration-endpoints-and-admin-write-face.md) 完成记录判断项 (7b)；`docs/agents/workflow.md`「本机环境」（`pnpm build` 坏在环境、用 `node node_modules/typescript/bin/tsc`）。

## Comments

- 2026-09-15 15:4x · 通道 1：立票（31 作者报出，推送方复现）。只写票面，未动代码。
