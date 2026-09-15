# admin-web `tsc --noEmit` 在 main 上已红：sa-cc/19 给 `DutyVerificationRecord` 加了必填 `fundsVersion`，`register-rows.test.ts` 的 `verification()` 夹具没跟——而 CI 没有 admin-web 的类型检查步，这一红自 19 进 main 起无人看见

Category: bug
Status: resolved——**已进 main，2026-09-15 16:2x 通道 1 推送方（新会话）**（第八批，重放 `2a1f4138→0943f7da`，批 tip `0943f7da`；评审门推送方自审（只一份测试夹具 +2 行字面、零行为）；`0943f7da` 带 DSN 全仓 115 ok / 0 FAIL、admin-web `tsc --noEmit` 退 0 而基线 `15433da2` 退 2；见 Comments「进 main 记录」）。此前 resolved——**2026-09-15 15:3x 通道 5（改派）**（task-1d4e6bef；分支 `mcp5-sacc35` 基 main `15433da2`，代码 tip 即本笔；原派通道 4 task-b6892242 crash 零开工。夹具补的不止 `fundsVersion`——`procedure` 同样必填且同样缺席，tsc 一次只报一个属性，见完成记录判断项 ①）。此前 ready-for-agent——**2026-09-15 15:4x 通道 1 立票**（sa-cc/31 作者通道 3 完成记录判断项 (7b) 报出、推送方在 `0b3f0831` 复现：`apps/admin-web` 下 `node node_modules/typescript/bin/tsc --noEmit` 退 2，唯一报错 `src/pages/customs/register-rows.test.ts` `verification()` 返回值缺 `fundsVersion`）。只一份测试夹具，零行为
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

## 完成记录（2026-09-15 15:3x 通道 5（改派），task-1d4e6bef，分支 `mcp5-sacc35` 基 main `15433da2`，单笔即 tip）

**逐条对判据**：
- **判据 1 ✓** `apps/admin-web` 下 `node node_modules/typescript/bin/tsc --noEmit` 退 **0**、报错 0 行（隔离树无 `node_modules`，按 workflow.md 本机环境那条用 junction 指向主树 `d:\tops\idp-parcel\apps\admin-web\node_modules`，验完 `rmdir` 收回；`node_modules/` 在根 `.gitignore`，两种 `git status` 都不见它）。
- **判据 2 ✓** `git diff --stat -- ':!apps/admin-web/src/pages/customs/register-rows.test.ts' ':!.scratch'` 空；`api.ts` / `presentation.ts` / 页面组件 / CI 零 diff。改动只有 `verification()` 字面量的两行。
- **判据 3 ✓** 本记录同笔；清点零差（TS 不在清点内）。

**逐条对做法**：
- **做法 1** `verification()` 字面量加 `fundsVersion: 'SYN-FUNDS-01/v1'`（与同夹具 `funds: 'SYN-FUNDS-01'` 同一事实，版本字面照 `duty: 'SYN-DUTY-01/v1'` 同形）；**另加 `procedure: 'SYN-PROC-01'`**（与本文件上方 `collaboration()` 夹具的 `procedure: 'SYN-PROC-01'` 同一个合成程序），理由见判断项 ①。本文件别处对「缺 `fundsVersion`」没有任何断言（`git grep fundsVersion -- register-rows.test.ts` 改前零命中），`verificationRows` 的行值也不显 `procedure` / `fundsVersion`（`register-rows.ts` 里 `procedure: record.procedure` 那一处在协作事项行，不在核对行），所以两格的夹具值不改任何既有断言的结果。
- **做法 2** `api.ts` 的 `DutyVerificationRecord` 一字未动——`fundsVersion` / `procedure` 都是必填，类型对、夹具错。

**判断项**：
- **① 缺席的必填字段是两个，不是一个；红的起点是 22 不是 19。** 基线 `15433da2` 上 tsc 退 2、唯一报错 `register-rows.test.ts` `verification()` 返回值 TS2322「`fundsVersion` 不兼容」（**red**，与票面复现一致）；只补 `fundsVersion` 后再跑，tsc 仍退 2、唯一报错换成同一处 TS2322「`procedure` 不兼容」——tsc 对一次赋值只报第一个不兼容的属性，`fundsVersion` 在接口里排在 `procedure` 之前，所以票面与推送方复现都只看到它。`DutyVerificationRecord.procedure: string` 是 sa-cc/22 `2e411f63`（10:37）加的，`fundsVersion: string` 是 sa-cc/19 `e0201ca6`（12:00）加的：main 上这一红自 22 进 main 起就在，比票面写的「自 19 进 main 起」早约一个半小时；两笔都没跟夹具，CI 无 admin-web 类型检查步所以两次都没人看见。补两格后 tsc 退 0（**green**）。两格都落在票面地盘那一份文件、同一个字面量里，零行为红线不碰，所以同笔补齐而不另立票。
- **② 夹具值的选法**：`fundsVersion` 取 `'SYN-FUNDS-01/v1'`（票面给定）；`procedure` 取 `'SYN-PROC-01'`——不自造第二个程序名，用同文件 `collaboration()` 已有的那个，让两张夹具说的是同一个合成程序。全 `SYN-`，无真实程序 / 资金引用。
- **③ 两次 tsc 的退出码与报错数**（本树实测）：基线 `15433da2` → 退 2 / 1 处（`fundsVersion`）；只补 `fundsVersion` → 退 2 / 1 处（`procedure`）；补两格 → 退 0 / 0 处。

**验证**（隔离 worktree `%TEMP%\idp-parcel-mcp5-sacc35` 基 `15433da2`，15:2x–15:3x）：见判据 1 与判断项 ③；未占 55432、未跑 go test（票面判据不要求，改动不含 Go）；`git diff --numstat` 2 增 0 删，文件索引 `i/lf`、工作副本 `w/crlf` 是 `text=auto` 检出常态，暂存后仍是两行差。

**能力边界**：读过票面全文、`register-rows.test.ts` 两张夹具与全部用例、`api.ts` `DutyVerificationRecord` 及头注、`register-rows.ts` 两处字段引用、两笔提交（`2e411f63` / `e0201ca6`）的 `-S` 落点；**没读** 19 / 22 票面正文、`presentation.ts` 词表正文、CC 后端。「procedure 也缺」是 tsc 报出来的，不是读票面推的。

## 另记（不成票，归用户）

- CI 没有 admin-web 的类型检查步——`tsc --noEmit` 红了三个多小时无人看见。加一步的代价是 CI 时长与计费；判据 5 一类「本机 `tsc` 0」今天只靠作者自跑。

## 参照

[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md)；[31](31-sa-external-funds-fact-registration-endpoints-and-admin-write-face.md) 完成记录判断项 (7b)；`docs/agents/workflow.md`「本机环境」（`pnpm build` 坏在环境、用 `node node_modules/typescript/bin/tsc`）。

## Comments

- 2026-09-15 15:3x · 通道 5（改派，task-1d4e6bef）：完成记录见上，单笔。与票面不符一处：缺席的必填字段是 `fundsVersion` **和** `procedure` 两个（tsc 一次只报第一个），红自 sa-cc/22 `2e411f63` 起而非 19；两格同笔补齐，`api.ts` 未动。「另记」那条（CI 无 admin-web 类型检查步）本票照旧不碰，归用户。
- 2026-09-15 15:4x · 通道 1：立票（31 作者报出，推送方复现）。只写票面，未动代码。
- **2026-09-15 16:2x · 进 main 记录 · 通道 1 推送方（新会话）**：**自审**（零行为票按 09-08 规矩不点评审人）：`git show --stat 2a1f4138` 两件——本票 .md 与 `apps/admin-web/src/pages/customs/register-rows.test.ts` +2 −0（`fundsVersion: 'SYN-FUNDS-01/v1'` / `procedure: 'SYN-PROC-01'` 两格字面）；`api.ts` / `presentation.ts` / 页面 / CI 零 diff。完成记录判断项 ① 采纳（缺席两个、红自 22 `2e411f63` 起）——标题与「缺口」里的「自 19 起」是立票时的取证，按判断项读，不改历史。**重放** `2a1f4138→0943f7da`（接手推送方 15:4x 叠在 31 六笔 `e9c2f7f8` 之上，`git patch-id` 同；测试文件对作者 tip 零 diff）。**验证钉 `0943f7da`**：本会话带 DSN 全仓 115 ok / 0 FAIL / 16 无测试 / 0 cached（16:19:34→16:21:28）；admin-web `tsc --noEmit`（`node_modules` 以 junction 指主树）在 tip 退 0、阳性对照基线 `15433da2` 退 2 且唯一报错正是本票那一处 TS2322。推送、SHA 对照与 ff 见票 31 进 main 记录同批。分支 `mcp5-sacc35` → `merged/`、远端删；作者树 `%TEMP%\idp-parcel-mcp5-sacc35`（作者已 rmdir junction、`status` 空）归通道 5 拆。「CI 加不加 admin-web `tsc` 步」仍归用户。
