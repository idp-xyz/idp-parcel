# 异常披露规则与冲突信号规则两册有写入口无入口：`ExceptionDisclosureRuleRegistry` / `ConflictSignalRuleRegistry` 只有 postgres 写口与读口，`parcel-ve-register` 与 `parcel-api` 都登不进去，两条编排在生产上只能读到空册

Category: enhancement
Status: in-progress——2026-09-11 通道 3 按通道 1 派单 task-a2cb7477 接单，分支 `mcp3-vedisc02` 基 `51ca1270`，步一 + 步二一起做（03 已进 main）。此前 ready-for-agent——2026-09-10 通道 4 按通道 1 派单 task-d6660969 把推送方三条裁决写进票面（见「裁决」）：取 B（CLI + 端点 + 管理台）、读面另立 [03](./03-exception-disclosure-and-conflict-signal-rule-catalogue-read-face.md)、两册并回 `CatalogRegistration`；**步一（CLI）可即开工，步二（端点 + 管理台写签）等 03 进 main**。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 步一无；步二 [03](./03-exception-disclosure-and-conflict-signal-rule-catalogue-read-face.md)（两册读面——写签跟读签走，读面不在就不铺写签；**03 已进 main（2026-09-11 12:0x，重放 tip `53370bb6`，见票 03「进 main 记录」）**，步二不再被它阻；步一命令名对齐 03 落下的 `kind` 原词 `EXCEPTION_DISCLOSURE_RULE` / `CONFLICT_SIGNAL_RULE`（按 `DISCLOSURE_POLICY` ↔ `disclosure-policy` 的既有变形），是 03 评审 Standards ② 的提醒，2026-09-11 通道 1 推送方记）

## 为什么立在这个目录

`admin-write-faces/02`（其余登记册的在线登记面）与 `/05` 都没有列过这两册（`git grep -n -E 'ExceptionDisclosureRule|ConflictSignalRule' -- .scratch/admin-write-faces` 零），它们是 mech/08 在 2026-09-04 新立的登记面，晚于 awf/02 的范围分辨；按通道 1 派单口径「没列过 → 立在 `ve-disclosure-policy-view/issues/` 下一号」。本目录票 01 裁的是披露**策略**视图（`PAR-VIS-09`），本票是异常披露**规则**（0023）与冲突信号规则（0025）两册——名字相邻、不是同一册，票面各处不混用。

## 缺口（取证于 `3f485e97`）

- 端口在：`ports.ExceptionDisclosureRuleRegistry.RegisterExceptionDisclosureRules`（`internal/visibilityexception/ports/ports.go`）、`ports.ConflictSignalRuleRegistry.RegisterConflictSignalRule`（`ports/conflict_signal.go`）；postgres 写入口 `adapters/postgres/exception_disclosure_rule.go` / `conflict_signal_rule.go`（各带真库用例）；迁移 `visibility_exception/0023_exception_disclosure_rule_and_decision.sql` / `0025_conflict_signal_rule.sql`。
- 读侧已接生产：`cmd/parcel-dispatch/assemble.go` 装 `vepostgres.NewConflictSignalRules`（投影派生的冲突裁决）；`decide_disclosure.go` 读 0023（mech/08 VE-a）。
- 写侧零入口：`git grep -n -E 'DisclosureRule|ConflictSignalRule' -- cmd/ internal/visibilityexception/adapters/http` 只命中 dispatch 那一处读侧；`cmd/parcel-ve-register/main.go` 命令族六册 + 材料两命令（milestone-mapping / triage-rules / notification-policy / claim-eligibility / claim-authorization / disclosure-policy / claim-material-receipt / -revocation），无这两册；`cmd/parcel-api/endpoints.go` 的 `/visibility-catalogue-*-registrations` 六行同样无。
- 读面也无：`/visibility-catalogues` 读口（`VisibilityCatalogueReader`）不含这两册（`git grep -n -E 'ExceptionDisclosureRule|ConflictSignalRule' -- internal/visibilityexception/adapters/http` 零）。
- mech/08 原句：「前两个只立了写入口（postgres，真库测试）与读口，**CLI（`parcel-ve-register`）与在线登记口未接**——单立端口是为了不拆 `CatalogRegistry` 的三处替身与受控 CLI 桩。要接线时形状照 `RegisterNotificationPolicy` 那一族。」
- 后果：`DecideDisclosure` 与 `derive_projection.go` 的 `judgeForks / raiseConflict` 在生产上永远读到空册——前者披露决定全落待确认，后者替代链分叉裁不了——而两者都是「如实未配置」，装配点看不出缺的是入口。

## 语言从哪里来

- `parcel-ve-register` 文件头：「VE 五类规则与策略目录……的受控登记口：运营方操作员在数据库网络内手跑……配置内容属实例半边（`PAR-VIS-01`/`05`/`07`/`08`/`09` 待提供），留待租户登记。」
- ADR-0085 决定一（CLI 与端点消费同一登记用例）、决定四（「登记频次 × 操作者角色」由实施票逐册裁）；awf/05 owner 裁决记录：判据以决定四为准。
- mech/08 VE-a / VE-d 对两册用途的记载（异常披露规则 → 三态披露决定；冲突信号规则 → 替代链分叉按业务时间裁不了时形成信号）。

## 做法（一张两步，照 awf/05 的形；已裁 B，两步都做——步一可即开工，步二等 03 进 main）

**步一 · CLI**（A、B 共有）：`parcel-ve-register` 加两命令 `exception-disclosure-rules` / `conflict-signal-rule`，走 `application.CatalogRegistration` 新增的 `RegisterExceptionDisclosureRules` / `RegisterConflictSignalRule`（形照 `RegisterNotificationPolicy`：租户绑定、`CatalogRegistrationOutcome` 三格、`channel_execution` 留痕、`approvedBy` 责任由登记方带）；输入 `-input` JSON，未知字段拒，退出码沿既有格。mech/08 当时不拆 `CatalogRegistry` 三处替身——本票要动 `CatalogRegistration` 的依赖，替身跟随（票面记为何现在拆得起：两册写入口与读口都已在，替身只是补两个方法）。

**步二 · 在线登记端点 + 管理台**（已裁 B，做）：`adapters/http/register_catalog.go` 加两个 `*Registrar` 接口与处理器，`cmd/parcel-api/endpoints.go` 加 `/visibility-catalogue-exception-disclosure-rule-registrations` / `/visibility-catalogue-conflict-signal-rule-registrations` 两行（共享接线文件，动前占号），装配以字面量 `UnconfiguredIntake{}` 起步；管理台 `pages/visibility/` 加登记签——**写签跟着读签走**：两册今天无读面（上面「缺口」第 4 条），读面由 [03](./03-exception-disclosure-and-conflict-signal-rule-catalogue-read-face.md) 另立并先行，03 进 main 前本步不开工、不铺写签。

## 红线

- 规则取值全属实例半边（`PAR-VIS-*`）：本票只建入口，不写任何默认披露规则或冲突信号规则；合成值只记 `S`。
- 不新增覆盖语义：登记不可覆盖、`已存在` / `内容冲突` 是答案不是失败，照 postgres 写入口已有代数转写。
- 不动 `decide_disclosure.go` / `derive_projection.go` 的读路径；两册为空时的行为原样（如实未配置）。
- 若裁 B，写准入不另立形（ADR-0085 决定二）；隔离 demo 里如实答未配置。

## 完成判据（非作者评审逐项对）

1. `git grep -E 'RegisterExceptionDisclosureRules|RegisterConflictSignalRule' -- cmd/` 各至少一处非测试命中（步一后 CLI；步二后加端点装配）。
2. CLI 单测：两命令各一条绿路径 + 治理两格 + 受理门拒绝 + 依赖故障退出码；不含真库（写口真库用例已在 `adapters/postgres`，本票零改动那一层）。
3. `application/register_catalog.go` 两个新方法有应用层用例；三处替身编译通过。
4. 若裁 B：http 单测只收 POST、未配置 403、三态响应；`cmd/parcel-api` 装配用例真库一正一反；`internal/architecture` 管理台路径门禁绿；读面先于写签。
5. 基线不加宽；机制清点 tip 重生成（接入面 / 端点数如裁）。

## 地盘

`cmd/parcel-ve-register/main.go`、`internal/visibilityexception/application/register_catalog.go`（+ 三处替身）——步一；`internal/visibilityexception/adapters/http/register_catalog.go`、`cmd/parcel-api/endpoints.go` 两行、`cmd/parcel-api/assemble_ve_registration.go`、`apps/admin-web/src/pages/visibility/`——步二。不动 `adapters/postgres/**`、`ports/**`。

## 要裁的（已裁，见下节「裁决」）

1. **A 只补 CLI / B CLI + 端点 + 管理台**：按 ADR-0085 决定四「登记频次 × 操作者角色」——两册都是租户上线时登一次、偶尔换版的低频·合规/客服规则；与同族六册（`notification-policy` 等）已进端点表这一事实怎么权衡（同族一致 vs 逐册裁）。归 VE owner。
2. **读面**：两册今天无查阅端点；写签跟读签走，则读面是本票步二的前置还是另立票（形照 `/visibility-catalogues` 的 `VisibilityCatalogueReader`）。归 VE owner。
3. **`CatalogRegistration` 是否收两册**：mech/08 当时单立端口是为了不拆三处替身；本票倾向并回 `CatalogRegistration`（同一受控 CLI 桩、同一留痕），代价是替身加两方法。归 VE owner，一句。

## 裁决（2026-09-10，推送方通道 1 裁、通道 4 按 task-d6660969 写入；用户 17:0x 授权「你自决」，读法见 tasks.md 16:5x–17:0x 节）

1. **取 B（CLI + 端点 + 管理台），口径「同族一致」。** 同族六册（`milestone-mapping` / `triage-rules` / `notification-policy` / `claim-eligibility` / `claim-authorization` / `disclosure-policy`）已全进端点表，awf/05 是同形裁法；ADR-0085 决定四的「登记频次 × 操作者角色」是租户运营事实，今天无租户只能是假设——机制半边把入口做齐，谁用、多久用一次由实例半边定。**越权风险点**：「同族一致」这一口径本身归 VE owner 复核；若 owner 认为这两册该按决定四逐册裁成只 CLI，砍掉步二即可，步一不受影响。
2. **读面另立 [03](./03-exception-disclosure-and-conflict-signal-rule-catalogue-read-face.md)**（形照 `/visibility-catalogues` 的 `VisibilityCatalogueReader`：两册读面 + 管理台读签）。本票步二 Blocked by 03；步一 CLI 不阻——写签跟读签走是伞票纪律，CLI 不是签。
3. **并回 `CatalogRegistration`**（票面倾向）。mech/08 当时单立端口只因替身代价；今天两册写口读口都在，代价只是三处替身加两方法，换来同一受控 CLI 桩、同一留痕。

## 参照

[mech/08](../../mechanism-executor-triage/issues/08-ve-six-executors-behind-existing-uc-steps.md) VE-a / VE-d 与登记面那一段；[awf/05](../../admin-write-faces/issues/05-auto-reroute-facts-has-no-registration-entry.md)（一张两步、A/B 裁法）；ADR-0085；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-9；本目录 [01](01-disclosure-policy-is-a-catalog-not-a-content-factory.md)（相邻不同册）。

## Comments

- 2026-09-10 · 通道 4：立票（task-9880bbc9）。awf/02、/05 未列过这两册，按派单口径立在本目录；未动代码。
- 2026-09-10 · 通道 4（task-d6660969，基 `062f5228`，分支 `mcp4-adr0136`）：推送方三条裁决写入「裁决」节，立读面票 03，Status → ready-for-agent（步一可即开工；步二 Blocked by 03）。**只改 .md，未动代码。**
