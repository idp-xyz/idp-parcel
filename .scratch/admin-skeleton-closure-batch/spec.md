# 管理台骨架页收口批（墙降之前把机制半边铺完）

Category: feature
Status: resolved——批收口 2026-08-31：02–06 全落主线转 live（12 页），骨架档收至 2
（`label-transactions` 属票 08 draft 待裁；`acceptance-review` 属范围裁定不入批，批级判据
「降至 1」以范围裁定为准）；收口两审与提交态验证见票 07 Comments。票 08 单列不阻批

用户 2026-08-31 指示「全部解决，规划如何做，并行工作 MCP-3/4/5」。本批把 14 张骨架页里
**不等租户就能完成的机制半边**全部立票；完成后管理台的「未接线」一档收敛为零，余下的差距
全部是实例半边，等一份租户渠道证据。

## 事实基线（取证于 `65b6cf2`，演示库 `idp-parcel-postgres-gate`，`seed.sh` 的 `SYN-` 种子）

- 38 页中 23 已接线、14 骨架、1 演示、0 规划占位；**14 张骨架页的主表全部 0 行**。
- 墙票 15 张里 14 张已 resolved，只剩 [票 01](../syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md)
  为 `needs-info`。**「三堵墙」的说法已过时**：今天只剩接入渠道这一堵，它是一条串行链的入口
  （无委托 → 无计价评价 → 无路由 → 无作业 → 无履约 → 无异常案件 → 无费用 → 无结算），
  13 张页全坐在下游。
- 该墙**不可降**：[ADR-0072](../../docs/adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)
  维持 [ADR-0055](../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md) 对
  「预先给渠道拟运行时登记表」的否决，票 01 重启条件写死为 `PAR-INT-01` 最低证据到位。属实例
  半边，现在造表即破红线。
- **`pilot_governance` 是全仓唯一零 `tenant_id` 的上下文**（8 表 0 处；其余十个上下文均有，
  结算 25 表 78 处、履约 16 表 59 处、追踪异常 30 表 109 处）。因此只有治理线含设计裁决，
  其余各线可逐字照抄 [ADR-0077](../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)
  的读面形状。
- `adapters/http` 现状：`pilotgovernance` 与 `settlementaccounting` **整个包不存在**；
  `nodeoperations`、`transportfulfillment` 有包但 `query_*` 为零（只有命令端点）；其余有先例。
- `cmd/` 下九个二进制，`seed.sh` 调六个登记 CLI，**唯独没调 `parcel-governance-register`**——
  治理八表零行因此是种子没灌，不是墙拦。这是 14 张里唯一能真出数据的一格。
- 全仓迁移中**不存在 `label_transaction` 表**：面单交易页缺的是领域建模，不是接线。

## 范围裁定

- **入批六线**：治理读面键形裁决（01）、试点治理读面与 `stage-admission` 接真（02）、计价评价
  与路由计划读面（03）、结算与核算读面（04）、作业与履约查阅读面（05）、追踪异常案件三页
  读面（06）；另设批务线（07：装配点串行落地）。面单交易建模单列（08，`draft`，待裁）。
- **对上一批口径的有意调整，不是遗忘**：[admin-remainder-mechanism-batch](../admin-remainder-mechanism-batch/spec.md)
  的范围裁定写着事务链页「不入批（等租户，不可代劳）……墙降后按既有裁定另立接线新票」。本批
  就是那张新票，但**提前到墙降之前**，只做机制半边。理由是本仓反复警惕的那类失效：
  **「尚未接线」与「已接线但登记册为空」今天在工作台上长着同一张脸**，而这两句话要人做的事
  相反——前者要人去写代码，后者要人去拿租户证据。把机制半边铺完，这两态就分得开了，且租户
  到位当天全部自动点亮。代价是这批买到的是诚实，不是业务能力，各票判据必须照此写。
- **不入批**：接入渠道登记册与真渠道 Intake（票 01 已裁定显式留待，重启条件是 `PAR-INT-01`
  最低证据）；`acceptance-review`（其地盘 `internal/parcelshipment/**` 此刻有他会话在途改动，
  开批时未协调完，见下）。

## 硬约束（每票逐字适用）

**除 `stage-admission` 外，其余各页接完仍是空册。** 完成判据只能写成「空态文案说的是『读取
入口已配置、登记册为空』，而不是『尚未接线』」，**不得写成「页面有数据」**——写成后者会诱导
下一个人去造数据填满它，那就破了「实例值留空拒默认」。隔离合成只记 `S`，`SYN-` 门禁按
[ADR-0078](../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)。
中文注释；跨文件引用用符号名不用行号；改中文源文件不用 `Set-Content`。

## 地盘与共享点

| 票 | 会话 | 地盘 | 共享点约束 |
|---|---|---|---|
| 01→02（串行） | MCP-3 | `internal/pilotgovernance/**`、`docs/adr/`（新 ADR 一份）、`scripts/demo-seeds/`（治理种子）、`apps/admin-web/src/pages/governance/StageAdmissionPage.tsx` | 新 ADR 要在 `docs/adr/README.md` 补目录项——**只改自己那一行，不动邻行**；`seed.sh` 加步前在频道说一声 |
| 03 | MCP-3 | `internal/parcelpricing/**` 与 `internal/networkrouting/**` 的读面部分、`apps/admin-web/src/pages/pricing/PricingEvaluationsPage.tsx`、`.../network/RoutePlansPage.tsx` | 两上下文已有 `adapters/http` 包，只加 `query_*`，不动既有处理器 |
| 04 | MCP-4 | `internal/settlementaccounting/**`，**除 `adapters/partycommercial/`**（该目录此刻有他会话在途改动）、`apps/admin-web/src/pages/settlement/**`、`.../governance/{ReconciliationPage,SettlementApplicationPage}.tsx` | `adapters/http` 从零建包，形状照抄 `internal/collectionremittance`（最新一份从零到接线的完整样板） |
| 05 | MCP-5 | `internal/nodeoperations/**`、`internal/transportfulfillment/**`、`apps/admin-web/src/pages/operations/**` | 两上下文 `adapters/http` 已有包但 `query_*` 为零，只加读面，不动命令端点 |
| 06 | MCP-5 | `internal/visibilityexception/**` 的案件读面部分、`apps/admin-web/src/pages/visibility/{ExceptionCasesPage,ClaimsRecoveryPage}.tsx`、`.../governance/ExceptionTriagePage.tsx` | 该上下文六类目录读面已接线，**不得改动**目录侧任何符号；只加案件侧 |
| 07 | MCP-1 | `cmd/parcel-api/**` 装配四件、批面收口 | **装配点占号在 07** |

### 装配点归 MCP-1 串行落地

三条线都要往 `cmd/parcel-api` 的装配四件（`endpoints.go`、`assemble_isolated_read.go`、
`main.go`、`unwired_orchestration.go`）加行。本仓 2026-08-13 在这类共享接线文件上连断远端构建
两次，一次 `add` 卷带、一次提交竞态，而**竞态只有占号能防**（见
[parallel-sessions](../../docs/agents/parallel-sessions.md) 的「共享接线文件」节）。

因此：**各线不自改装配四件。** 上下文侧（读端口、真库读适配器、`adapters/http` 处理器、隔离读
准入）做完并自验绿后，向频道交出一个**已验 SHA** 与要装配的端点行；MCP-1 按到达顺序逐笔接进
装配表并推送，落地后在频道广播。这等价于永久占号，也用上了「验证可以外包——验的是不动的
对象，不必由推的人亲自验」。

### `liveIds` 与页接真的次序

`apps/admin-web/src/page-registry.tsx` 的 `liveIds` 各线自落、**只加自己那行不动邻行**，但
**必须在 MCP-1 广播该端点已装配之后**。次序反了会得到一张登了 live 却 404 的页——正是本仓
反复点名的「接错看着像接对」，而且没有任何测试会红。各票因此分两阶段：阶段一上下文侧交活，
阶段二收到广播后页接真 + `liveIds`。

## 交活口径

worktree、逐文件 `git add`、不自行推送，照 [parallel-sessions](../../docs/agents/parallel-sessions.md)。
交活时附**已验 SHA**，并报自上一个已验 SHA 以来动没动过 `.go` 或 `.sql`。测试一律 `-count=1`；
报绿要说清是哪一种绿——含真库那一档要用单跑门禁用例看 `PASS` 还是 `SKIP` 证，**不看秒表**
（耗时什么也守不住，理由见 parallel-sessions「不同的『绿』」节）。

## 完成判据（批级）

各票判据之外：全仓 `go build ./...` 与含真库 `go test -count=1 ./...` 绿（注明含不含真库）；
`apps/admin-web` `pnpm build` 提交态绿；工作台「页面骨架」一档降至 1（仅 `label-transactions`，
其建模属票 08）；每张转 live 的页空态文案说的是「登记册为空」而不是「尚未接线」。

## 子票

01、02、03、04、05、06、07、08——见 `issues/`。
