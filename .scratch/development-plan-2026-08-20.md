# 后续开发计划（基于完成度评估，2026-08-20）

Category: chore
Status: draft——轨道与依赖是本文的主张；**派工顺序与逐票放行仍等用户/协调岗点头**，本文不替裁。

依据：[完成度评估快照](./completion-assessment-2026-08-20.md)（入库于 `24c6b4a`）+ 票面现状（盘于 `b9f6cba`，含墙/门审计十票入主线与四件悬决裁断之后）。计划只覆盖**机制半边**；实例半边按基线定义等租户，不进本计划。

2026-08-20 晚间复核（协调岗，对 `main=8663948`）：轨道、串行约束与里程碑不变；T0 三行、T2 第 1 步、T3 队列已按当日进展就地更新，明细见文末「复核记录」。

2026-08-20 深夜复核（协调岗，对 `origin/main=d6453a7`）：轨道与约束仍不变；T0 行 2/3 收口、T1 票 04 转 needs-triage、十票开工前重核已全部完成、T2 反查尾巴清账；明细见「复核记录」末条。

2026-08-21 晚间复核（MCP-3 代协调岗，对 `main=a1f283c`）：轨道、串行约束与里程碑判据仍不变；T0 再收两行并新增棘轮门禁一行、T1 五票收口（含关键路径票 03）、T2 第 2 条更正（写下当晚即已失真）、T3 队列出一批进一批；明细见「复核记录」末条。

2026-08-21 晚间裁断轮（MCP-3 受用户委托，对 `main=b394adf`）：全部「等人」格出裁——ADR-0072 立（渠道能力所有权）、SA-05 四问裁、Bento 收口开两行动票、票 11/12/13 分诊裁定、清册三弃候选裁弃；needs-triage 与 ready-for-human 双双归零，可派池见「复核记录」末条。

## 一、切分方式评估：能按 BC 切吗

**结论：BC 不宜作计划主轴，宜作所有权与并行边界。** 三个理由，都取自本仓已发生的事实：

1. **排序驱动力在链位与杠杆，不在上下文归属。** [墙/门审计](./syn-wall-door-audit/report.md)按业务链位（W01–W18）走查，发现单一 BC 的一件工作可以解锁横跨多上下文的七堵墙——PC 声明发布写入方（票 03）一件事解 W04–W08、W11、W12，消费方却在 PS/SA/NR。按 BC 泳道排期会把这条关键路径切碎，看不见杠杆。
2. **共享装配点是全局串行资源。** `cmd/parcel-dispatch/assemble.go` 与 `cmd/parcel-api/endpoints.go` 是全体消费/接入接线的汇聚文件，协调协议已有红线「B 类票占用 assemble.go 时不派第二张碰它的票」。十个 BC 十条泳道的并行幻觉在这两个文件上必然撞车。
3. **横切件无 BC 归属。** outbox 分区键裁断、载荷规范化摘要、准入范围装配、达标重盘、工程残局清理——这些都不落在任何单一上下文里。

**但 BC 仍是不可替代的边界**，体现在三处：每票必须标主责上下文（领域语言、`CONTEXT.md` 约束、评审范围跟着它走）；T1 门族**内部**恰好按 BC 分组可安全并行（不同 BC 的写口互不撞文件）；跨上下文工作一律落消费方侧适配器（ADR-0025），BC 决定代码放哪。本仓现行方法（PN 纵向切片为轴 + 票族 + 主责上下文标注）已经是「缝为轴、BC 为界」的混合式——本计划沿用，不另起炉灶。

## 二、轨道

### T0 清欠账（在办票收口）

| 票 | 主责 | 状态 |
|---|---|---|
| [outbox 分区键·第二步范围（八口+接管格）](./outbox-partition-key/issues/03-step-two-scope-eight-ports-and-four-undecided.md) | platform | 全族收口：八口+接管格入 main（`a1463ae`），四处待裁已裁断（`ac04366`：案件链不设跨口保序、关闭信封携周期维、同对象二次揽收成立且分区取到对象），票 01/02/03 全 resolved |
| [供应商成本更正·规则协议与发生项引用](./supplier-expected-cost-correction/issues/03-correction-restates-rule-agreement-and-occurrence-refs.md) | SA | 02/03 resolved（`747ba74`/`3324ecb`，ADR-0067）；同族 [04 客户费用同币种](./supplier-expected-cost-correction/issues/04-customer-charge-single-currency-contradicts-context.md)已实现收口 resolved（`f538fa7`，币种三件组按 CONTEXT 字面执行）；衍生 [05 费用调整币种三件组](./supplier-expected-cost-correction/issues/05-charge-adjustment-currency-triple-unruled.md) ready-for-human，入 T3 |
| [索赔资格查询七维缺四维](./ve-claim-eligibility-dimensions/issues/01-eligibility-query-cannot-carry-four-of-seven-dimensions.md) | VE | 票已关 resolved（切块 (b) `cacf78d`、(c) `0db1acf`） |
| [PAR-NET-14 机制半边切割](./nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md) | NR | in-progress（四件悬决裁断放行） |
| [脏 worktree 残局](./dead-session-salvage/issues/02-dirty-orphan-worktrees-hold-uncommitted-work.md) | 工程 | **resolved**（08-21 用户批复后执行）：[并回清册](./dead-session-salvage/branch-merge-census-2026-08-21.md)判定 61 支、两路同裁弃三支后，33 棵非主树全拆、55 支指针删除（51 已吸收逐支重验 + 4 弃定）、6 支按册保留（`bento-gate-reeval` 证据仍被 bento 票 02 引用，收口时归档再删）；执行记录在[清理票 03](./dead-session-salvage/issues/03-obsolete-branch-pointer-cleanup.md) |
| [生产接线棘轮门禁](./production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md) | 工程 | in-progress：门禁已落地（`82f6b74` 判方向不判状态、基线冻结今日 33 条；`cb570dd`/`129b3e0` 两笔修正），余步以票面为准；普查基线 [census-d5e5d20](./production-wiring-ratchet-gate/census-d5e5d20.md) 顺手给 T2 一把量尺 |

### T1 门族：写入方与登记口（墙/门审计十票，本计划最大块）

审计判定：十八墙中**无门** W01/W02/W03/W09/W10/W14/W15，其余半门。开工次序按杠杆不按 BC，但**组内按 BC 并行安全**（互不撞文件）：

| 组 | 票 | 主责 | 解锁 |
|---|---|---|---|
| 关键路径 | [03 PC 声明无发布写入方](./syn-wall-door-audit/issues/03-pc-declarations-have-no-publication-writer.md) | PC | 一件解七墙（W04–W08/W11/W12）——**已收口 resolved**：死会话现场复活全绿合入（`364606f`/`8821e28`），AT-PC-011 批内逐项独立成败与 runPublish 真库批推进补齐（`119a0b0`/`acd74d5`）；PS/SA 墙后消费自此可排 |
| 可并行 | [04 网络定义登记册无写入方无解析层](./syn-wall-door-audit/issues/04-network-definition-register-no-writer-no-resolver.md)、[05 自动改路事实目录](./syn-wall-door-audit/issues/05-auto-reroute-facts-catalog-unimplemented.md) | NR | W09/W10；04 票面已按 NR-CATALOG-MECH（`3b9f212`，ADR-0068）改写完，复为 ready-for-agent；05 照旧 ready-for-agent，碰 `assemble.go` 装配点，占号 |
| 可并行 | [06 CC 案件配置登记册无写入方](./syn-wall-door-audit/issues/06-cc-case-config-registries-have-no-writer.md) | CC | W13；A 半边已入 main（`171e08b` 五本登记册写口+真库往返，`2cc7146` 五类登记用例、冲突判定落编排），票仍 ready-for-agent，余量以票面为准 |
| 可并行 | [07 价卡无版本仓储](./syn-wall-door-audit/issues/07-pricing-plan-and-rate-table-no-version-repository.md)、[08 参考序列登记册缺失](./syn-wall-door-audit/issues/08-pricing-reference-series-register-missing.md) | PP | W14/W15——**双双 resolved**（`a4070fe`：版本表+按方向范围时点装载/按基准时点解析+结果代数写入+受控 CLI 登记口） |
| 可并行 | [09 VE 规则与政策登记册无写入方](./syn-wall-door-audit/issues/09-ve-rule-and-policy-registries-have-no-writer.md) | VE | W16/W17/W18——**A/B 拆分收口**：A 半边（登记册写口六方法覆盖五类七表、迁移 0019）合入 `b394adf`，票转 resolved；B 半边成[票 15](./syn-wall-door-audit/issues/15-ve-catalog-registration-has-no-process-entry.md)（受控 CLI 登记入口，ready-for-agent，**不占号**——开票时纠正了「B=assemble.go」的误绑） |
| 桥/缝 | [02 生产归属权威无适配器](./syn-wall-door-audit/issues/02-production-ownership-authority-has-no-adapter.md) | PS←PG | W03——**resolved**（`57e0b1f` 治理侧补「尚未恢复暂停」读口并接桥，`c0ea050` 准入暂停查询三态化）；收口后衍生三张 needs-triage 新票，见 T3 |
| 桥/缝 | [10 收寄资格证据源未实现](./syn-wall-door-audit/issues/10-intake-qualification-evidence-source-unimplemented.md) | PS←NO | W11 证据口（ADR-0063）——**resolved**（`5f795bc` 节点执行事实接证据口，`eafb2b1` 接进采用装配，未认领时显式未配置） |
| 平台 | [01 接入渠道登记册与首个真实 Intake](./syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md) | platform/HTTP | W01/W02——**已裁（ADR-0072，08-21 受托）**：所有权归共享接入身份能力（`internal/accessidentity/` 落点），ADR-0055 否决维持，登记册形状等 `PAR-INT-01` 证据；票转 needs-info，W01/W02 记「按票裁定显式留待」；载荷规范化摘要拆出为[票 14](./syn-wall-door-audit/issues/14-ps-payload-canonicalization-digest.md)（ready-for-agent，纯领域件不占号） |

**每票开工前按采纳注记对当时 main tip 重核四件**（仓储/装载口/写入方/登记口）——十票已全部重核完毕（`ee58c1a` 核 01/02/07/08/10，`d6453a7` 核 03/04/05/06/09，结论录各票 Comments），后续开工只需对新 tip 做连续性确认，不必重做全量。

### T2 消费装配铺满（缝工作，天然跨 BC）

1. 消费方向清点**已完成，不再排期**：[outbox-handoff-consumption-map/report.md](./outbox-handoff-consumption-map/report.md)（46 适配器×4 栏，逐行带 CONTEXT-MAP/UC 判据，取证于 `4d57ecd`）——已有消费者 1 类、应有未开·跨上下文 26、应有未开·同上下文 19、混合 5、本就不应该有 2、说不清 3（pilot-governance 缺领域文档是最大判据缺档）。尾巴剩一条：消费本表前按新 HEAD 重取证（表内自注保质期）；UC 正向反查已完成并入库（[uc-reverse-gaps.md](./outbox-handoff-consumption-map/uc-reverse-gaps.md)，`b3004d0`）。终点仍不是 46 类全接（ADR-0049「接不住的类型登记比不登记更糟」）。
2. ~~首个 A/B 票对候选：`initial-route.formed` 入队即撞 `no_subscriber`~~——**本轮更正：此条写下当晚即已失真**（成因与证据见复核记录末条）。当下事实以 `cmd/parcel-dispatch/assemble.go` 的 `wireDispatcher` 路由表注释为准：十二类事件已登记、`initial-route.formed` 已投 VE 投影；仍准确的半边是其 NO/TF 侧应然消费者不存在、按 ADR-0049 第三条不登记。T2 余量的量尺改用棘轮普查（[census-d5e5d20](./production-wiring-ratchet-gate/census-d5e5d20.md)一族：46 交接口装配 5 口，41 零生产调用点——与本节第 1 步 46 适配器清点表互为对照）。
3. 按缝排 **A/B 票对**（A：消费方 inbox+adapter；B：`assemble.go` 接线）——沿用 CONS-PROJ 票族已验证的模式。B 票全局串行；A 票可与不同 adapter 的 A 票并行。
4. 与 T1 的 05、01 两票共享装配点占号约束。

### T3 裁断队列（needs-triage / draft / needs-info）

按票分诊，多数是单 BC 领域裁断：[PS 外部标记关系](./ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md)、[PS 地址提供路径边界](./nr-route-evidence-views/issues/02-ps-address-provision-path-is-an-unmade-boundary-decision.md)、[VE 客户视图逐投影版本](./ve-customer-view-per-projection-version/issues/01-does-customer-view-keep-a-generation-per-projection-version.md)、[TF/NR 交付粒度](./route-handoff-delivery-granularity/issues/01-per-parcel-independence-cannot-be-expressed-in-one-delivery-one-transaction.md)、[申报信封版本维](./declaration-envelope-version-dedup/issues/01-envelope-id-lacks-version-dimension.md)（draft）、[r25 端口盘点报告](./port-inventory-r25/report.md)分诊。needs-info 两票（VE-008 [05](./ve-008-late-account-rederive/issues/05-customer-attribution-changes-hands.md)/[06](./ve-008-late-account-rederive/issues/06-customer-attribution-reverts-to-indeterminate.md)）等新证据，重启条件以票面记载的倾向为准。阻于 PAR-INT-01 的 [ops 重放端点](./ve-008-late-account-rederive/issues/04-ops-replay-endpoint-blocked-on-par-int-01.md)在 T1 票 01 落地后重估。

深夜入队的两组均已出队：**四处待裁已裁断**（`ac04366`），裁定随分区键票 03 一并 resolved；**Bento 闸门重估已收口**（[结论票](./bento-gate-reeval/issues/01-reeval-verdict-bento-gate-stays-blocked.md) resolved，08-21 受托裁定：闸门维持阻断不动，行动 1+2 并为[02 PBC-08 行为面收口](./bento-gate-reeval/issues/02-pbc08-behavior-face-closure.md)、行动 3 立[03 六项零证据 PBC 取证推进](./bento-gate-reeval/issues/03-zero-evidence-pbc-evidence-run.md)，双双 ready-for-agent；封存分支指针保留并入整类清理）。

08-21 复核入队的四张已全部裁毕出队（08-21 受托，裁定录各票 Comments）：[11 覆盖关系登记](./syn-wall-door-audit/issues/11-scope-version-coverage-relation-is-unregistrable.md)（关系落治理库面、随 Go/No-Go 决定一并登、两种边都可表达）、[12 治理登记进程入口](./syn-wall-door-audit/issues/12-governance-registration-has-no-process-entry.md)（不属业务端点面走受控 CLI、执行者身份双轨、首批开权威区间+暂停+恢复三类）、[13 生产归属桥接线](./syn-wall-door-audit/issues/13-production-ownership-bridge-has-no-assembly-point.md)（`AnswerValidity` 裁 1 分钟语义「单次提交判断视界、不跨提交复用」；碰 `endpoints.go` 占号）、SA [05 费用调整币种三件组](./supplier-expected-cost-correction/issues/05-charge-adjustment-currency-triple-unruled.md)（四问全裁：三件组覆盖调整、依据按种类分槽、结算币身份上升到类型；权威句已入 SA CONTEXT）——四张全部转 ready-for-agent。另新增[票 14 载荷规范化摘要](./syn-wall-door-audit/issues/14-ps-payload-canonicalization-digest.md)（ready-for-agent，自票 01 拆出）。

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
| M1 | T0 清零 | 表内各票 resolved，在途分支合入或裁弃 |
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
- 2026-08-20 深夜，协调岗对 `origin/main=d6453a7`（较上基准 `8663948` 后 18 笔：分区键八口+接管格九笔与格式收尾 `a1463ae`、NR 目录机制四件 `3b9f212`、索赔资格切块 (c) `0db1acf` 与 0018 授权目录迁移、SA 纠错 02/03 `747ba74`/`3324ecb`、T1 十票重核 `ee58c1a`/`d6453a7`、文档若干）逐轨核对：
  - T0 收口两行：SA 02/03 resolved（同族 04 补裁到期，转入待排）；索赔资格票 resolved。行 1（四处待裁取证已出等拍板）与行 4 照旧；行 5 新增 `mcp1-pc-publication` 死会话现场（票 03 七成实现未提交）。
  - T1 在办与转态：03 派 MCP-3（含既有现场交接待裁）、07/08 派 MCP-2 重启；04 转 needs-triage 先改写票面；09 放行前两处票面事实待更新（MAP-KIND 已完 `1709872`；0018 授权目录两表入登记口范围）。
  - T2：UC 正向反查入库清账，重取证一条尾巴保留。
  - 盘点两笔待裁：未合分支 `nr-applicability-from-resolution`（`ce92bdd`，ADR-0064）无人认领；本地 main 领先两笔经 patch-id 证为上游孪生，可安全对齐 origin——但主树未跟踪的 `.scratch/pg-takeover-replay-handoff/` 是唯一未入库产物，**抢救先于任何清理**。
  - 轨道、装配点串行、里程碑判据：不动。
- 2026-08-21 晚间，MCP-3 代协调岗对 `main=a1f283c`（较上基准 `d6453a7` 后 51 笔：票 03 现场复活合入与测试补齐、票 02 桥接与三态修正、SA 票 04 实现、票 06 A 半边、票 07/08 从零四件、票 10 证据口接装配、四处待裁裁断 `ac04366`、棘轮门禁落地 `82f6b74` 及两笔修正、两处死会话现场封存、其余为 scratch/agents 文档）逐轨核对：
  - T0：行 1（分区键全族）与行 2（SA，票 04 直接做完）收口，衍生 SA 票 05 入 T3；行 4（PAR-NET-14）照旧 in-progress；行 5 两处现场了结、四棵零 delta 树已拆，`mcp1-pc-publication`、六个 `cons-*` 分支、`t1-02-ownership-authority` 等 tip 不在 main 祖先链上的树仍待票内逐棵对账（与 main 已有同方向接线是何关系未 patch-id 对账，属该票范围），`t-ratchet-gate` 树 tip 已合入且实测干净、可拆。**新增棘轮门禁一行**（工程横切，in-progress）；本节标题与 M1 判据同步去计数（「五票」→「表内各票」），按 AGENTS 计数戒条。
  - T1：五票 resolved（02/03/07/08/10），**关键路径票 03 已过，PS/SA 墙后消费自此可排**；04 票面改写完复 ready-for-agent；06 A 半边入 main 票仍开；01 转 ready-for-human；09 两个 `t1-09a-*` worktree 在飞未入 main。票 02 收口衍生 11/12/13 三张 needs-triage（12/13 按定义要落 `cmd`，与装配点占号约束相关）。
  - T2：第 2 条更正——`b4b6a64`（路由表注记「十二类事件」的那笔）经祖先关系核实**先于**晚间基准 `8663948`，即 `initial-route.formed` 在该条写下当晚已登记投 VE 投影，原条目失真于出生；本轮改为引 `wireDispatcher` 路由表注释并以棘轮普查作量尺。同句失真也立在基线文档「机制半边现状」一节（「路由表今天只有一条」「它至今仍是唯一的消费者」），基线更新不属本文范围，**待另轮处理**。重取证尾巴照旧保留。
  - T3：四处待裁出队（随分区键票 03 resolved）；新入队四张（审计 11/12/13、SA 05）；Bento 照旧等用户。
  - 轨道、装配点串行、里程碑判据：不动。M1 余 PAR-NET-14、脏树、棘轮三行；M2 十票余五张开（01 等人裁，04/05/06/09 可派，其中 05 占 `assemble.go` 号、01 占 `endpoints.go` 号）。
  - 增补：本轮复核落笔后 main 旋即由 MCP-2 快进至 `b394adf`（合并基恰为本轮基准 `a1f283c`，merge-tree 零冲突，净增全在 VE 与票面）——09-A 登记册写口落主线，T1 行 09 已随之改写，票 09 仍 ready-for-agent；推送按分工交 MCP-1（已验 SHA `b394adf`）。本条之外各行仍以 `a1f283c` 为取证基准。
- 2026-08-21 晚间裁断轮，MCP-3 **受用户委托**（用户指示：待人类裁决的部分由其作为系统与业务专家直接裁决）对 `main=b394adf` 清一遍全部「等人」格，八件全部出裁：
  - **T1-01**：立 [ADR-0072](../docs/adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)——接入渠道登记册、凭据验证、来源信封铸造归共享接入身份技术能力（落点 `internal/accessidentity/`）；ADR-0055 对预拟渠道表的否决**维持**，登记册形状等 `PAR-INT-01` 证据。票转 needs-info（重启条件写明），W01/W02 显式留待；载荷规范化摘要拆出成票 14（ready-for-agent）。
  - **SA-05**：四问全裁——三件组覆盖调整；纠错类依据挂新 SELL 评价、让利类挂有效商业授权（跨币种让利无换算依据即拒）；`AdjustmentAuthorityReference` 按 Kind 拆两槽；结算币身份上升到类型。权威句已入 [SA CONTEXT](../docs/domain/settlement-accounting/CONTEXT.md)（紧随同币种两额相等条），票转 ready-for-agent（只改领域类型与测试，不建编排不建表）。
  - **Bento**：结论票 resolved——闸门**维持阻断**不动；行动开两票（02 PBC-08 行为面收口、03 六项零证据 PBC 取证推进，均 ready-for-agent）；封存分支指针保留并入整类清理。
  - **票 11**：关系表落治理侧库面（结构半边由关系语义定死，可援引 ADR-0068），参数登记册登脱敏索引；关系随范围版本升版那次 Go/No-Go 决定一并登记，不设独立登记路；承继边与显式互不相干都要能表达。转 ready-for-agent。
  - **票 12**：治理登记不属业务端点面，走受控 CLI（新 `cmd`，不占号）；执行者身份双轨（通道技术身份入口自取不可传参 + 决定人标识显式必填作登记内容），自报不作授权依据；首批开权威区间、暂停、恢复三类。转 ready-for-agent。
  - **票 13**：`AnswerValidity` 裁 1 分钟，语义「单次提交判断的处理视界，答复不跨提交复用」；接线放行，同笔改 `unwired_orchestration.go` 注释，开工首步按棘轮普查核端口在位性，缺者按 ADR-0063 显式未配置接。转 ready-for-agent，碰 `endpoints.go` **占号**。
  - **清册三弃候选**：`mcp3-ve008-wire`、`salvage-cons-proj-delivery-detached`、`mcp1-pc-publication` 全部裁弃（判据录清册「裁定」节；main 侧对映一一核实），指针保留；`adr-0065-storage` 与 `bento-gate-reeval` 的「等用户确认删指针」并入指针整类清理，不再单等。脏树票余量收敛为两件机械执行。
  - 裁断轮之后，**全库「等人」格清零**：needs-triage 归零、ready-for-human 归零（票 01 属 needs-info 等实例证据，非等裁决）。可派池：T1 的 04/05/06 余量、票 12/13/14/15、Bento 02/03、票 11、SA-05；占号约束照旧（05 占 `assemble.go`，13 占 `endpoints.go`，互不冲突可并行）。
  - 裁断轮补一件（MCP-2 问询）：票 09 按其票面预记的 A/B 拆分转 resolved **维持不改回**——A 半边已验收合入，余量按「02→13」同款拆票承载，[票 15](./syn-wall-door-audit/issues/15-ve-catalog-registration-has-no-process-entry.md) 已开（VE 目录登记受控 CLI 入口）；开票时纠正 09 拆分评论把 B 半边误绑 `assemble.go` 一事——登记是操作者动作不是信封消费，按票 12 裁定走 CLI，不占号。
  - 裁断轮合流记（main 至 `3756eeb`）：用户同晚**分别**委托 MCP-3 与 MCP-4/MCP-2 裁同一批件，两处撞车均「独立裁断、结论逐条一致」后合流——①清册三弃候选（MCP-4 合稿入 `3756eeb`，两份证据分工在册：场景对映答「丢不丢行为」、main 对应答「有无接替」）；②SA-05 四问（MCP-2 撤重复段留同裁注记，快照扩列时点从本轮裁定「不预造」）。ADR 号协调：0072 归接入身份（本轮），MCP-2 取 **0073**（申报单元持久化聚合持案件维，随之[申报单元案件关联票](./customs-declaration-case-link/issues/01-declaration-unit-has-no-case-association.md)转 ready-for-agent、衍生裸 caseRef 收敛 02 票）与 **0074**（TF 对象链分区键带口名段，随之[分区键碰撞票](./partition-key-space-collision/issues/01-tf-object-partitions-collide-with-ve-parcel-partitions.md)转 resolved，TF 实现随 `ca03b76`/`409bcf4` 入 main）。
  - 执行轮（用户批复「请你直接干吧」，MCP-3 执行）：①裁断轮全部落档成笔 `327a517`（ADR-0072、索引三行、SA CONTEXT、九票、两新票、计划）；②**推送积压清零**——`git push` 后远端实测 = `327a517`（原停 `a1f283c`），含 `b394adf` 及全部裁决落地提交，推送分工就此由授权执行替代等待 MCP-1；③**脏树票（T0 行 5）收口 resolved**——33 棵非主树全拆、55 支指针按[清理票 03](./dead-session-salvage/issues/03-obsolete-branch-pointer-cleanup.md)配方删除（51 支删前逐支重验 cherry 全 `-`，4 支按两路同裁弃定），6 支按册保留，`bento-gate-reeval` 因证据仍被 bento 票 02 引用改为「随其收口归档后删」；执行记录一支一行回写票 03，票 02/03 双双 resolved。M1 余 PAR-NET-14 与棘轮两行。
