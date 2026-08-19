# declaration-submission 信封 ID 无版本维的去重核证（DECL-ENVELOPE-VERSION-DEDUP-CHECK）

取证基准：`5073851`（detached worktree，非共享树）。只读核证，未改任何 `.go` / `.sql`，不裁决。

被核证的推断（出自 `.scratch/envelope-n-object-ruling/report.md` 附带观察）：「`declaration-submission.formed` 的信封 ID 不含版本维，重报换版本可能被 `EnqueueOnce` 去重——若属实，重报版本永远发不出第二封。」

## 结论三格

**坐实（机制半边）＋今天无触发路径（现状半边）。** 分开说：

1. **机制半边坐实**：信封 ID 由（租户/单元/程序）三维拼成、不含版本维；`EnqueueOnce` 按（来源＋事件 ID）先查后插、查到即静默成功。只要同一（租户/单元/程序）键下出现第二个提交版本并走出账口，第二版意图**必然**不入队、无错误、无日志分格。每一环都有代码原文，见 (a)(b)。
2. **触发半边今天不存在**：当前编排（`SubmitDeclarationHandler`）同键异内容直接答 `SOURCE_CONFLICT` 拒绝、同键同内容答 `EXISTING_VERSION` 返原版本；当前存储 `ON CONFLICT (tenant_id, unit_id, procedure_ref) DO NOTHING` 一键一行一版本。同键第二版在今天的代码里**造不出来**。见 (c)。
3. **引信在 CONTEXT 里**：CC CONTEXT 明文要求「原案内更正或补充形成新的正式申报资料和**提交版本**」且**保留申报单元身份**——即同（单元＋程序）键下的第二版是领域明文预期的演进方向。一旦按 CONTEXT 落地原案内更正编排（存储必须先改成容纳多版本），出账口若沿用现拼法，第二版即被吞。见 (c)。
4. **订正上票错引**：上票把重报语义挂在 ADR-0045 上——**错引**。ADR-0045（「同一委托的新提交版本」）说的是 parcel-shipment 的 `ShipmentRequest` 委托提交版本，与 CC 申报无关。CC 侧「重报」按 CONTEXT 恰恰**不是**同键新版本（重报必须建立**新的逻辑申报目标**/替代申报单元，即新键新信封，无去重问题）；真正命中同键新版本的是「原案内更正/补充」这条路。上票的坑是真的，但点名的触发场景（重报）点错了，正确触发场景是原案内更正。

## (a) CC 侧出账信封 ID 怎么拼

`internal/customscompliance/adapters/postgres/declaration_submission_handoff.go`：

- 拼法（`declarationSubmissionEventID`）：

> ```go
> func declarationSubmissionEventID(key ports.DeclarationSubmissionKey) string {
> 	return key.TenantID.String() + "/" + key.Unit.String() + "/" + key.Procedure.String()
> }
> ```

- 版本只进载荷与 Subject，不进 ID：载荷结构体 `declarationSubmissionPayload` 四字段 tenantId/unitId/procedure/**versionId**（注释「只有下游 FindByKey 所需的幂等键三维，不带组成快照或发送尝试明细」——versionId 是键三维之外多带的一件）；信封 `Subject` 取 `intent.Record.Version.ID().String()`。
- 入队调用点（`HandOffDeclarationSubmission` 尾部）：`outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope)`；注释「信封 ID 取幂等键——意图由幂等键认领（ADR-0043）」。这里「幂等键」指 `DeclarationSubmissionKey` 三维（`ports.go` 定义：TenantID + Unit + Procedure），**store 的键与信封 ID 同构，都无版本维**。

出账时机（`submit_declaration.go`）：首存成功（`DeclarationSubmissionSaved`）与重放命中已有（`existingResult`）都会调 `handOff`——重放重发同一份，靠 EnqueueOnce 去重，这半边是 ADR-0043 的正确用法。

## (b) EnqueueOnce 的去重键与语义

`internal/platform/outboxintent/enqueue_once.go` 全文短小，语义三句：

- 去重键是**（来源＋事件标识）**：`SELECT EXISTS(SELECT 1 FROM bento.outbox WHERE source = $1 AND event_id = $2)`（表名经 `migrate.SchemaBento` 拼出）。
- **查到即成功返回，不入队**：`if exists { return nil }`。包注释自述「重发同一份不出第二份，且不撞出中止态」；函数注释解释先查后插是为避免同事务撞唯一约束（SQLSTATE 23505）把事务打进中止态、连带回滚业务写入。
- 并发首发竞态由唯一约束兜底。

推断确认点：`EnqueueOnce` 分不出「同 ID 的重放」与「同 ID 的新内容」——它不比内容、不比载荷，ID 相同即视为已发。对「重放重发同一份」是正确幂等；对「同 ID 不同版本」就是静默吞。

## (c) CC 领域里会不会产生同引用新版本

### CONTEXT 说会（未来态）

`docs/domain/customs-compliance/CONTEXT.md` 硬句（规则节）：

> 监管规则允许的更正或补充**在原案件内形成新的正式申报资料和提交版本**，原提交及其结果永久保留。监管规则要求撤销重报时，必须建立替代申报单元或替代关务案件并保存替代关系，不能把所有重报都伪装成原提交的新版本。

> 同版本技术再次尝试、原案内补充、原案内更正、撤销动作和重报替代必须分别表达。……**补充或更正只有在监管规则明确保留原案件和申报单元身份时才形成原案内新资料与新提交版本**；……**重报必须形成新的逻辑申报目标**。

生命周期节：

> 提交后允许更正或补充 → **在原案件内形成新的资料快照与提交版本**；原版本不可修改。

四条路对信封 ID 的影响各不相同：
- **同版本技术再次尝试**（`SubmissionAttempt.ControlledResend`，安全再次发送判断）：同版本新尝试，不产新版本、不产新信封——无关。
- **原案内更正/补充**：**保留申报单元身份 → 同（租户/单元/程序）键 → 新提交版本 → 信封 ID 与首版相同 → 被吞**。命中。
- **撤销**：独立监管动作、自身提交——新逻辑目标，键不同，无关。
- **重报替代**：「新的逻辑申报目标」＝替代申报单元（新 `DeclarationUnitID`）→ 新键新信封——**不命中**（上票以「重报换版」为触发的表述在此被推翻）。

### 当前代码不会（现状态）

- 编排（`submit_declaration.go`）：`FindByKey` 命中且指纹不同 → `DeclarationSourceConflict`，注释原句「同一逻辑申报目标携带不同组成或快照：已固定版本不可覆盖，修订走撤销重报，不在这里顶替」；命中且指纹同 → `DeclarationExistingVersion` 返原版本重发同一份意图。**没有任何入口产出同键第二版。**（顺带：这句注释把「修订」全部指向「撤销重报」，与 CONTEXT「原案内更正在原案件内形成新提交版本」的口径有偏差——按 CONTEXT，保留单元身份的更正不该走撤销重报。这不是本票要裁的，但裁定原案内更正编排时这句注释要一并修。）
- 存储（`declaration_submission.go`）：`INSERT ... ON CONFLICT (tenant_id, unit_id, procedure_ref) DO NOTHING`，撞键答 `AlreadyRecorded` 且不落尝试行——**表结构一键一行一版本**，同键第二版今天连库都进不去。`submission_index.go` 注释另证 `version_id` 有唯一约束（`declaration_submission_version_unique`），版本标识全局唯一但每键仅一版。
- 领域类型允许多版本存在（`SubmissionVersionID` 是独立身份、`CustomsSubmissionVersionSnapshot` 无键约束），但没有任何编排造第二版。

### 结论

「同引用新版本」是 CONTEXT 明文预期、当前实现尚未落地的形状。**吞版本的坑真实存在于信封 ID 构造里，引信是「原案内更正/补充」编排的落地**——落地那天存储键必然要动（一键一行装不下第二版），若信封 ID 拼法不随之补版本维（或按版本认领），第二版意图静默丢失。

## (d) 若触发：可观测后果链，与今天的消费者现状

**消费者现状**：`customs-compliance.declaration-submission.formed` 字符串全仓只出现在 CC 自己的 outbox 适配器与其测试（`declaration_submission_handoff.go` / `_test.go`）。本仓消费者纪律是「消费方自己写出事件类型字符串」（veinbox 六个消费者皆如此），因此无消费者写出即无消费者。`cmd/parcel-dispatch/assemble.go` 的 `NewDirectPublisher` 路由表（上票逐行读过，本票 grep 复核未变）也没有此类型。**今天发布即撞 `dispatch.no_subscriber`（ADR-0049 认下的未登记），风险纯潜伏——没有任何下游会察觉丢失。**

**若日后接线（事实源勘察已把此条列为 VE「需裁定」六条之一）且原案内更正已落地**，丢失链逐环：

1. CC 更正编排产出同键 V2，`HandOffDeclarationSubmission` 组装信封——ID 与 V1 相同（租户/单元/程序）。
2. `EnqueueOnce` 查 `bento.outbox`：(source=CC, event_id=同) 已存在 → `return nil`。**V2 意图不入队，调用方拿到成功**。编排的 `SubmissionHandoffReference` 续办机制也不会救——它只在 handoff 返回错误时留续办引用，而这里返回的是 nil。
3. 派发器永远只投过 V1 信封。下游（假设 VE 已接）inbox 键（消费者名＋来源＋事件 ID）也只见 V1 那一封。
4. VE 按载荷键 FindByKey 重读——注意载荷三维键读回的是**当前行**：若 CC 存储改成「当前版单行」（UPDATE/Replace 式），VE 迟到重放 V1 信封时会读到 V2 内容，撞内容指纹进 `FactSourceConflict`；若改成多版本多行、载荷 versionId 参与取数，VE 只能取到 V1。两种实现下 **V2 的成员快照、新资料引用都到不了下游投影**；追踪上该申报永远停在首版组成。
5. 无任何一环报错：CC 侧 nil、outbox 无新行、派发无失败、下游无投递。唯一可能察觉处是人工比对 CC 库的版本行数与 outbox 的信封数。

**最小爆炸半径**（只列改动点，不设计方案；修复属 CC owner）：

- `declarationSubmissionEventID` 的拼法（是否加版本段，或改由版本认领）——单函数；
- 与其配套的 `declaration_submission_handoff_test.go` 断言；
- 载荷形状不必动（versionId 已在）；
- 消费侧（尚不存在）按什么维立 inbox 账随裁定走；
- 「原案内更正」编排落地票本身（存储键、SOURCE_CONFLICT 分支、`submit_declaration.go` 注释口径）是前置依赖，不属本坑的修复面。

## (e) customs-case.established 是否同形命中

**不命中。** 三层证据：

- **键即身份、无版本概念**：`CustomsCaseKey` 五维（租户＋辖区＋方向＋程序＋义务范围），`customsCaseEventID` 五维全拼；案件没有「同键换版」的语义——CC CONTEXT：「原申报单元不能继续使用但案件固定辖区、方向、程序和法定义务范围不变时，建立**替代申报单元**；这些固定身份变化或形成独立监管义务时，建立**替代或后续关务案件**」——案件固定身份变即键变，键变即新信封 ID。
- **编排幂等口径**（`establish_customs_case.go` 的 Handle 注释原句）：「幂等按身份键四维（同一法律行为一案：同键同包裹集重放返原，同键异包裹集是范围冲突不顶替——同袋同总单同班次都不能自动证明同一案件，**扩大范围要走案件自己的变更**）」。同键第二次建案要么重放（重发同一份，EnqueueOnce 去重正确）要么 `SCOPE_CONFLICT`（不产出第二封）。
- **范围变更不是第二封 established**：包裹关联扩大属「案件自己的变更」，今天没有该编排；即便将来有，它是另一个事实（变更），照本仓先例应是新事件类型，不会复用 `customs-case.established` 的信封 ID。若将来有人把范围变更也从 established 口发出去，才会撞同形——那属于未来实现走形，不是现有结构的坑。

`customs-case.established` 的消费者现状与 declaration-submission 相同：字符串只在 CC outbox 侧与测试，无消费者，`no_subscriber` 潜伏（对它而言这只意味着「还没人听」，无被吞风险）。

## 覆盖声明

**逐条读过取证的（打开文件读到函数、SQL 或原句本身）：**

- `outboxintent/enqueue_once.go` 全文（去重查询 SQL、exists 短路、包注释与函数注释）。
- `declaration_submission_handoff.go` 全文（eventID 拼法、载荷结构体、EnqueueOnce 调用点、Subject 取版本）。
- `submit_declaration.go` 全文（键三维、指纹分界、SOURCE_CONFLICT/EXISTING_VERSION 分支、commit/existingResult/handOff、续办引用只在 handoff 报错时留）。
- `declaration_submission.go`（postgres store）全文（FindByKey 单行查询、Save 的 ON CONFLICT 三维键 DO NOTHING、版本行+尝试行同事务、latestAttempt）。
- `submission_index.go` 读到注释与查询（version_id 唯一约束 `declaration_submission_version_unique` 的说法出处）。
- `customs_case_handoff.go` 全文（五维 eventID 拼法）；`establish_customs_case.go` 读到 Handle 注释与分格枚举（SCOPE_CONFLICT/EXISTING 语义）。
- CC CONTEXT 按关键词（撤销重报/重报/提交版本）取到的全部命中段，含规则节两句硬句、生命周期节「申报单元、就绪与提交」「后续申报动作与替代」两节原文。
- ADR-0045 全文（确认其对象是 parcel-shipment 的 ShipmentRequest，订正上票错引）。
- CC 领域 `declaration_submission.go`（domain）的 SubmissionAttempt.ControlledResend 段（上票已全读，本票据其确认「同版本技术再次尝试」不产新版本）。
- 全仓 grep 两个事件类型字符串（`*.go`），确认只在 CC outbox 侧出现。

**按同类归并推断、没有逐条打开的（推断不是取证）：**

- `bento.outbox` 表的唯一约束形状（(source, event_id)）按 `EnqueueOnce` 的查询列与注释「撞唯一约束（SQLSTATE 23505）」推断，未打开 idp-bento-go 框架源码或 bento schema 迁移核对。
- 「派发器只投已入队信封、无消费者时撞 `dispatch.no_subscriber`」沿用上票对 `wireDispatcher`/ADR-0049 的取证与事实源勘察报告，本票只 grep 复核了路由表没有这两个事件类型,未重读派发循环。
- (d) 第 4 环「CC 存储改单行当前版 vs 多行多版本」两种未来实现下的下游读回行为是结构推演，无代码可证（那份实现还不存在）。
- 后果链假设「接线方按 veinbox 现行形状立 inbox 账」——按仓内六个先例归并推断，裁定后的真实接线形状可能不同。
- CC 的更正/补充/撤销/重报四条路除 CONTEXT 与既有编排外未查 UC-CC 用例正文（票面未要求；CONTEXT 硬句已足够钉「原案内更正=同键新版本」这一格）。
