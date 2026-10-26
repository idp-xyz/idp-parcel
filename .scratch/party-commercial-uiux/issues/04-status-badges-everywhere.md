# 04 — 状态一律用徽章

**Parent:** [spec](../spec.md)

**What to build:** 参与方与商业区里同是「状态」，法人、参与方、关系三处是彩色徽章（`statusBadge`），合同、协议、产品、授权规则、策略册与客户账户是纯文字（`labelOf` 或直出），全景里供应商协议的状态也是纯文字。这些词（已发布、已生效、已退役，以及身份那套）在 `src/domain/status.tsx` 的 `domainStatusTones` 里都有色调。列头也有「状态」「身份状态」「生命周期状态」三种叫法；「生命周期状态」在 1440 宽下逐字竖排。

改法：本区各表的状态格一律走 `statusBadge`，列头统一叫「状态」。词表里没有的状态词照现有做法退成中性徽章，不自造色调。

**Blocked by:** None — can start immediately

**Category:** enhancement

**Status:** resolved

- [x] 合同、协议、产品、渠道映射、各政策册、客户账户、参与方全景里的状态格都是徽章。
- [x] 本区列头只用「状态」一种叫法。
- [x] 不新增 `domainStatusTones` 之外的色调。
- [x] 三道门退 0。

## 完成记录（2026-10-26，tops-54，前端切片，worktree `idp-parcel-pcux`）

- **落点：** 本笔提交。
- **改了什么：**
  - `detail-primitives` 加 `statusWordBadge`（按已译成的词出徽章，`statusBadge` 改为经它走）与 `withStatusBadge`（把行模型列表里 `status` 那一列换成徽章）。行模型 `policy-rows`、`supplier-agreement-rows` 仍只交字符串：它们要在 node:test 里跑，引组件库会断在 CJS 链上（先例见 `section-error.test.ts` 文件头），所以徽章在页面渲染那一层套。
  - 客户合同、客户账户、服务产品三表的状态格直接用 `statusBadge`；政策册与供应商协议两表经 `withStatusBadge`；参与方全景里供应商协议那一行的状态也换成徽章。
  - 列头「生命周期状态」「身份状态」一律改「状态」（两份行模型、合同页两表、产品页）。
  - 徽章不折行：窄列里原来挤成「已生 / 效」两行，`statusWordBadge` 外包一层 `whitespace-nowrap`；法人页与参与方页同一个组件一并受益。
- **判据：**
  - ✅ 状态格都是徽章：演示环境里客户合同、供应商协议（已生效绿、已退役灰）、服务产品三页目测；政策册里带状态列的三本（授权规则、接受前财务控制策略、客户服务规则）走同一个 `withStatusBadge`，演示种子里这几本的行未逐本点开看。
  - ✅ 列头只用「状态」：本区源码里「生命周期状态」「身份状态」作列头的写法已无。
  - ✅ 没有新增色调：徽章色只来自 `domainStatusTones`，词表外的词照旧原样显字。
  - ✅ 三道门：`tsc -b --noEmit` 退 0，`run-tests` 615/615，`vite build` 退 0。
- **未改：** 六本政策册（接单规则包、接受前财务控制、商业价格政策、结算政策、接受判断时点策略、信用政策）的读面本来没有状态列，加列要读口带出状态，不在本票。
- **评审：** 改动全在 `pages/**`，自查判据。
