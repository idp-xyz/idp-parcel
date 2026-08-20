# 后续开发计划（基于完成度评估，2026-08-20）

Category: chore
Status: draft——轨道与依赖是本文的主张；**派工顺序与逐票放行仍等用户/协调岗点头**，本文不替裁。

依据：[完成度评估快照](./completion-assessment-2026-08-20.md)（入库于 `24c6b4a`）+ 票面现状（盘于 `b9f6cba`，含墙/门审计十票入主线与四件悬决裁断之后）。计划只覆盖**机制半边**；实例半边按基线定义等租户，不进本计划。

2026-08-20 晚间复核（协调岗，对 `main=8663948`）：轨道、串行约束与里程碑不变；T0 三行、T2 第 1 步、T3 队列已按当日进展就地更新，明细见文末「复核记录」。

## 一、切分方式评估：能按 BC 切吗

**结论：BC 不宜作计划主轴，宜作所有权与并行边界。** 三个理由，都取自本仓已发生的事实：

1. **排序驱动力在链位与杠杆，不在上下文归属。** [墙/门审计](./syn-wall-door-audit/report.md)按业务链位（W01–W18）走查，发现单一 BC 的一件工作可以解锁横跨多上下文的七堵墙——PC 声明发布写入方（票 03）一件事解 W04–W08、W11、W12，消费方却在 PS/SA/NR。按 BC 泳道排期会把这条关键路径切碎，看不见杠杆。
2. **共享装配点是全局串行资源。** `cmd/parcel-dispatch/assemble.go` 与 `cmd/parcel-api/endpoints.go` 是全体消费/接入接线的汇聚文件，协调协议已有红线「B 类票占用 assemble.go 时不派第二张碰它的票」。十个 BC 十条泳道的并行幻觉在这两个文件上必然撞车。
3. **横切件无 BC 归属。** outbox 分区键裁断、载荷规范化摘要、准入范围装配、达标重盘、工程残局清理——这些都不落在任何单一上下文里。

**但 BC 仍是不可替代的边界**，体现在三处：每票必须标主责上下文（领域语言、`CONTEXT.md` 约束、评审范围跟着它走）；T1 门族**内部**恰好按 BC 分组可安全并行（不同 BC 的写口互不撞文件）；跨上下文工作一律落消费方侧适配器（ADR-0025），BC 决定代码放哪。本仓现行方法（PN 纵向切片为轴 + 票族 + 主责上下文标注）已经是「缝为轴、BC 为界」的混合式——本计划沿用，不另起炉灶。

## 二、轨道

### T0 清欠账（在办 5 票收口）

| 票 | 主责 | 状态 |
|---|---|---|
| [outbox 分区键·第二步范围（八口+接管格）](./outbox-partition-key/issues/03-step-two-scope-eight-ports-and-four-undecided.md) | platform | 八口+接管格已入 main（`a1463ae`）；四处待裁取证已出（[evidence-four-undecided.md](./outbox-partition-key/evidence-four-undecided.md)），拍板另行成轮；票 01 已 resolved（`c3227b9`） |
| [供应商成本更正·规则协议与发生项引用](./supplier-expected-cost-correction/issues/03-correction-restates-rule-agreement-and-occurrence-refs.md) | SA | [02](./supplier-expected-cost-correction/issues/02-correction-restates-amounts-and-currency.md)/03 实现已开工（MCP-3，占 SA 迁移 0012；裁断组入库 `c8f9b99`、03 取证入库 `b9f6cba`）；同族 [04 客户费用同币种](./supplier-expected-cost-correction/issues/04-customer-charge-single-currency-contradicts-context.md)仍 needs-triage 未进本轮，收口时补裁，口径同出 ADR-0067 |
| [索赔资格查询七维缺四维](./ve-claim-eligibility-dimensions/issues/01-eligibility-query-cannot-carry-four-of-seven-dimensions.md) | VE | 切块 (b) `cacf78d`、(c) `0db1acf` 均已入 main；七维查询与通过路接通，票可关 |
| [PAR-NET-14 机制半边切割](./nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md) | NR | in-progress（四件悬决裁断放行） |
| [脏 worktree 残局](./dead-session-salvage/issues/02-dirty-orphan-worktrees-hold-uncommitted-work.md) | 工程 | in-progress，按票内裁定逐棵处置 |

### T1 门族：写入方与登记口（墙/门审计十票，本计划最大块）

审计判定：十八墙中**无门** W01/W02/W03/W09/W10/W14/W15，其余半门。开工次序按杠杆不按 BC，但**组内按 BC 并行安全**（互不撞文件）：

| 组 | 票 | 主责 | 解锁 |
|---|---|---|---|
| 关键路径 | [03 PC 声明无发布写入方](./syn-wall-door-audit/issues/03-pc-declarations-have-no-publication-writer.md) | PC | 一件解七墙（W04–W08/W11/W12），PS/SA 消费全排它后面 |
| 可并行 | [04 网络定义登记册无写入方无解析层](./syn-wall-door-audit/issues/04-network-definition-register-no-writer-no-resolver.md)、[05 自动改路事实目录](./syn-wall-door-audit/issues/05-auto-reroute-facts-catalog-unimplemented.md) | NR | W09/W10；05 碰 `assemble.go` 装配点，占号 |
| 可并行 | [06 CC 案件配置登记册无写入方](./syn-wall-door-audit/issues/06-cc-case-config-registries-have-no-writer.md) | CC | W13 |
| 可并行 | [07 价卡无版本仓储](./syn-wall-door-audit/issues/07-pricing-plan-and-rate-table-no-version-repository.md)、[08 参考序列登记册缺失](./syn-wall-door-audit/issues/08-pricing-reference-series-register-missing.md) | PP | W14/W15，PP 是唯一仓储本体都缺的上下文 |
| 可并行 | [09 VE 规则与政策登记册无写入方](./syn-wall-door-audit/issues/09-ve-rule-and-policy-registries-have-no-writer.md) | VE | W16/W17/W18 |
| 桥/缝 | [02 生产归属权威无适配器](./syn-wall-door-audit/issues/02-production-ownership-authority-has-no-adapter.md) | PS←PG | W03（治理侧写入方已有，缺 PS 桥） |
| 桥/缝 | [10 收寄资格证据源未实现](./syn-wall-door-audit/issues/10-intake-qualification-evidence-source-unimplemented.md) | PS←NO | W11 证据口（ADR-0063） |
| 平台 | [01 接入渠道登记册与首个真实 Intake](./syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md) | platform/HTTP | W01/W02；含 ADR-0055 第五条两前置（载荷规范化摘要、准入范围装配），碰 `endpoints.go` 装配点，占号 |

**每票开工前按采纳注记对当时 main tip 重核四件**（仓储/装载口/写入方/登记口）——十票取证于 `49a2ab0`，死会话票面可能冻在过时时刻。

### T2 消费装配铺满（缝工作，天然跨 BC）

1. 消费方向清点**已完成，不再排期**：[outbox-handoff-consumption-map/report.md](./outbox-handoff-consumption-map/report.md)（46 适配器×4 栏，逐行带 CONTEXT-MAP/UC 判据，取证于 `4d57ecd`）——已有消费者 1 类、应有未开·跨上下文 26、应有未开·同上下文 19、混合 5、本就不应该有 2、说不清 3（pilot-governance 缺领域文档是最大判据缺档）。两条尾巴：消费本表前按新 HEAD 重取证（表内自注保质期）；补一步 UC 正向反查「UC 要求但连 handoff 都没有」（报告已点名 collection-remittance 一例）。终点仍不是 46 类全接（ADR-0049「接不住的类型登记比不登记更糟」）。
2. 首个 A/B 票对候选已被表点名：`initial-route.formed`——今天唯一被组合根构造的生产方（CreateInitialRouteHandler）发的就是它，入队即撞 `no_subscriber` 阻塞分区，应然消费者（NO/TF/VE 侧）一个未开。
3. 按缝排 **A/B 票对**（A：消费方 inbox+adapter；B：`assemble.go` 接线）——沿用 CONS-PROJ 票族已验证的模式。B 票全局串行；A 票可与不同 adapter 的 A 票并行。
4. 与 T1 的 05、01 两票共享装配点占号约束。

### T3 裁断队列（needs-triage / draft / needs-info）

按票分诊，多数是单 BC 领域裁断：[PS 外部标记关系](./ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md)、[PS 地址提供路径边界](./nr-route-evidence-views/issues/02-ps-address-provision-path-is-an-unmade-boundary-decision.md)、[VE 客户视图逐投影版本](./ve-customer-view-per-projection-version/issues/01-does-customer-view-keep-a-generation-per-projection-version.md)、[TF/NR 交付粒度](./route-handoff-delivery-granularity/issues/01-per-parcel-independence-cannot-be-expressed-in-one-delivery-one-transaction.md)、[申报信封版本维](./declaration-envelope-version-dedup/issues/01-envelope-id-lacks-version-dimension.md)（draft）、[r25 端口盘点报告](./port-inventory-r25/report.md)分诊。needs-info 两票（VE-008 [05](./ve-008-late-account-rederive/issues/05-customer-attribution-changes-hands.md)/[06](./ve-008-late-account-rederive/issues/06-customer-attribution-reverts-to-indeterminate.md)）等新证据，重启条件以票面记载的倾向为准。阻于 PAR-INT-01 的 [ops 重放端点](./ve-008-late-account-rederive/issues/04-ops-replay-endpoint-blocked-on-par-int-01.md)在 T1 票 01 落地后重估。

本轮复核新入队两组：①**四处待裁**——CC 案件链保序三口（含重开再关撞 ID）与 TF `offsite_pickup` 二次登记，[分区键票 03](./outbox-partition-key/issues/03-step-two-scope-eight-ports-and-four-undecided.md) triage 钉「先取证再裁断，PG/CC 无主；裁前四口例外清单行不删」；②Bento 闸门重估结论已出（[维持阻断](./bento-gate-reeval/issues/01-reeval-verdict-bento-gate-stays-blocked.md)），其行动 1-3 是否开票与封存分支去留等用户。

### T4 重盘与达标（终点闸门）

1. 按 r25 口径程序**两口径**重算端口（松口径判「名字出现」，方法集口径判真实缺口）；
2. 第二十六轮机制半边重盘，八切片逐三判据核；
3. 全绿（CI 含 `-race` 含真库）后向用户提交「产品就绪候选」宣布——宣布本身是用户的决定。

## 三、依赖与串行约束

- **关键路径**：T1 票 03（PC 写入方）→ 依赖它的墙后消费（PS 接受链、SA 控制、NR 复核）才能在 S 级走通验证。
- **装配点串行**：`assemble.go`（T1-05、T2 全部 B 票）与 `endpoints.go`（T1-01）同一时刻各只允许一张票占号。复核时点两文件均无人占号。
- **T2 清点先于 T2 接线**；T4 依赖 T0–T3 收口。
- **验测口径**：工人按包路径全量跑（不 `-run` 关键字子集）；集成口 `go test -p 1 -count=1 ./...` 设 DSN 含 PG；报状态必须写明含不含 PG。
- **红线不变**：实例值一律留空拒默认；隔离合成只记 `S`；worktree 纪律与「只 add 本票文件、推已验证 SHA」照旧。

## 四、里程碑（顺序主张，非承诺日期）

| 里程碑 | 内容 | 完成判据 |
|---|---|---|
| M1 | T0 清零 | 五票 resolved，在途分支合入或裁弃 |
| M2 | T1 门族收口 | 十八墙全部「有门」或按票裁定显式留待（裁定记录在票） |
| M3 | T2 按 UC 闭合 + T3 裁断清零 | 消费方向清单上每条要么接通要么裁不接；needs-triage 归零 |
| M4 | T4 重盘通过 | 第二十六轮盘点记录八切片达标，交用户宣布产品就绪候选 |

## 本计划不包含

- 实例半边的一切（真实价卡、合同、渠道参数、阶段决定）——等租户，不等排期。
- 租户接入工程（接真渠道 Intake、陪跑 PN-08 回放/影子/限量）——以租户出现为前提的新工作流。
- 逐票的技术方案——以各票面与开工时对 main 的重核为准，本文只排轨道与依赖。

## 复核记录

- 2026-08-20 晚间，协调岗对 `main=8663948`（较原盘点 `b9f6cba` 后九笔：五文档+三集成+一 triage）逐轨核对：
  - T0 行 1/2/3 就地更新：分区键票 01 关票（`c3227b9`）由范围票 03 延续并已派工 MCP-2；SA 02/03 从「等放行」进入实现（MCP-3 占 SA 迁移 0012，`AppendCorrection` 签名将改 spec 结构体入参，爆炸半径限 SA 包）；索赔资格切块 (b) 合入 `cacf78d`、切块 (c) 已派 MCP-3。行 4/5 复核无失真。
  - T2 第 1 步由「未来工作」改记「已完成」，指向消费方向清点报告，补两条尾巴（按新 HEAD 重取证、UC 正向反查）；新增首个 A/B 候选 `initial-route.formed`。
  - T3 入队两组：四处待裁（CC 案件链保序 + TF offsite_pickup 二次登记）、Bento 蒸馏后续（行动 1-3 开票与封存分支去留，等用户）。
  - M1 进度：五票之一 resolved，两条在途分支（`outbox-pk-annotate`、`claim-elig-b`）均已了结。
  - 轨道划分、装配点串行约束、T1/T4 前提、里程碑判据：核对无变化，不动。
