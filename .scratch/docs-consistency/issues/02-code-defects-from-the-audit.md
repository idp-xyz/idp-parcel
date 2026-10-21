# 02 把审计里的代码错（C 类）归拢成代码票

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dc02（见文末「完成记录」）。此前 in-progress——2026-10-21 认领；原 ready-for-agent——2026-10-21 协调方立（文档自洽修订第一波，见 [README](../README.md)「修订执行」）。
Blocked by: 无
出处：八份审计报告（[audit/](../audit/)）里归类为 C 的 37 条，以及 B 类里明说「裁完要改代码」的那些（只登记，不裁）。

## 要什么

1. 逐条读 C 类条目，按 main 现状复核（审计钉 `6b0074d0`，之后进 main 的票 88、145、146+94、150 等可能已修掉一些；已修的写「已由某票消解」并指出 SHA）。
2. 与既有票去重：报告里已点名有票的（例如 VE-08 → demo-chain-gaps/124、VE-09 → /135、VE-10 → /123、M17 → /130、TF H2 相关的 /57 等）不另立，只在那张票追加一节「文档自洽审计复核（2026-10-21）」写明审计条目号；没有票的才立新票。
3. 新票立在 `.scratch/docs-consistency/code/` 下，文件名 `NN-<kebab>.md`，编号从 01 起；每张写：现象（引审计条目原文要点与审计报告链接）、证据（代码符号 + 文档引文）、三类、规模、最小的诚实修法、要裁的（若依赖某个 B 类先裁，写 Blocked by 对应修订票号，例如「docs-consistency/09 裁 CR 入账层」）。同一上下文、同一根因的合成一张。
4. 按上线就绪口径分级（口径见 [triage-2026-10-19](../../demo-chain-gaps/triage-2026-10-19.md)「上线就绪评估」：R1 上线前必修——答错、静默丢失、不留痕、碰钱或数据完整性；R2 上线后尽快；R3 不急），写在每张票的 Status 里；在本票完成记录里给一张总表（票、上下文、分级、Blocked by）。

## 不变式

- 只写 `.scratch/` 下的票面；不改任何代码、不改 docs/。
- 一条审计条目只落一张票。

## 完成记录（2026-10-21，分支 claude-dc02）

八份报告的 C 类 37 条逐条按 main `67438a0f` 读码复核（审计钉 `6b0074d0`；其间进 main 的代码只有票 88、146、94，全在 TF 与 access-identity，其余上下文的代码与文档未变）。37 条现象**全部仍在**，没有一条已被消解。B 类里裁定之后无论裁向都要改代码的 3 条（CR H3、H5，NR H2）照「只登记，不裁」立了代码票，Blocked by 对应修订票；cross B2-02、B2-03 与 pc-pp H5 同一裁，登在 code/25。

分级口径照 [triage-2026-10-19](../../demo-chain-gaps/triage-2026-10-19.md)「上线就绪评估」：R1 上线前必修（答错、静默丢失、不留痕、碰钱或数据完整性、卡住无可靠绕法）、R2 上线后尽快（会撞但有可靠绕法或只是看得见的卡住）、R3 不急（低频、无生产路径或只是不方便）。没有生产路径的一律 R3，票面写明「接线之前必须先修」。既有票的分级只写在追加的那一节里作建议，票面 Status 不改，由协调方定稿。

### 总表

新立票在 [code/](../code/)：

| 票 | 上下文 | 分级 | Blocked by | 审计条目 |
|---|---|---|---|---|
| [code/01](../code/01-fulfillment-attempt-object-outcomes-have-no-pending-confirmation.md) 履约尝试逐对象结果缺「待确认」 | TF | R2 | 无 | tf M16 |
| [code/02](../code/02-tf-comments-cite-stale-decisions-and-context-line-numbers.md) TF 注释引已答的风险点、行号引 CONTEXT | TF | R3 | 无 | tf L9、L10 |
| [code/03](../code/03-customs-case-closure-obligation-items-lack-a-conflict-state.md) 关闭依据项缺「冲突」 | CC | R3 | 无 | cc M8 |
| [code/04](../code/04-customs-external-results-have-no-in-layer-correction-relation.md) 外部结果没有层内更正关系 | CC | R2 | 无 | cc M9 |
| [code/05](../code/05-customer-tracking-view-overwrites-published-versions.md) 客户视图已发布版本原地覆盖、按投影换代 | VE | **R1** | 无 | ve VE-01 |
| [code/06](../code/06-customer-view-shows-dimensions-without-a-visible-judgement.md) 客户视图 ETA 维无判断也答展示 | VE | **R1** | 无 | ve VE-02 |
| [code/07](../code/07-eta-store-overwrites-history-and-mixes-sources.md) ETA 存储整行覆盖、键无来源维 | VE | R3 | 无 | ve VE-11 |
| [code/08](../code/08-disclosure-awaiting-authorization-carries-no-draft.md) 披露待授权装不下草稿 | VE | R3 | 无 | ve VE-12 |
| [code/09](../code/09-demo-conflict-signal-rule-is-registered-as-customs-hold.md) 演示冲突信号规则登成 `CUSTOMS_HOLD` | VE（参考配置） | R2 | 无 | ve VE-16 |
| [code/10](../code/10-recovery-action-kinds-lack-supplementary-assertion.md) 追偿动作缺「补充主张」 | VE | R3 | 无 | ve VE-17 |
| [code/11](../code/11-statement-cut-does-not-check-the-settlement-attribution-date.md) 截单不核结算归属日 | SA | R2 | docs-consistency/09（裁 sa-cr M11 时区与截单时刻的权威） | sa-cr M7 |
| [code/12](../code/12-funds-mapping-has-no-compensation-or-recovery-targets.md) 资金映射缺赔付义务、追偿应收 | SA | R2 | 无 | sa-cr M9 |
| [code/13](../code/13-voided-statement-has-no-replacement-relation.md) 对账单替代关系不存在 | SA | R3 | 无 | sa-cr L7 |
| [code/14](../code/14-collection-principal-posts-more-than-once-and-shortfall-moves-received-funds.md) 同一笔本金多次入账、短款挪走已到账本金 | CR | **R1** | docs-consistency/09（裁 CR 入账层与短款表示法） | sa-cr H3、H5（B） |
| [code/15](../code/15-collection-records-cannot-point-back-to-what-they-revise.md) CR 记录不能回指原记录、无结论格、不核归集截点 | CR | R3 | 无（排在 code/14 之后） | sa-cr L6 |
| [code/16](../code/16-pickup-reassessment-compares-the-pickup-place-with-plan-nodes.md) 揽收复核把揽收地点当计划节点比 | NR | **R1** | docs-consistency/05（裁揽收复核的比法与可控节点） | nr-no H2（B） |
| [code/17](../code/17-still-applicable-plan-never-forms-a-reroute-suggestion.md) 仍适用线从不形成改路建议 | NR | R3 | 无（排在 demo-chain-gaps/115 之后） | nr-no M4 |
| [code/18](../code/18-node-customs-collaboration-acceptance-lacks-pending-supplement.md) 节点监管协作承接缺「待补充」 | NO | R3 | 无 | nr-no M5 |
| [code/19](../code/19-consolidation-containment-is-not-checked-for-cycles.md) 集运包含不查环 | NO | R2 | 无 | nr-no M6 |
| [code/20](../code/20-superseded-submission-version-keeps-its-control-occupation.md) 新提交版本取代旧版本时占用不释放 | PS | **R1** | 无 | ps PS-01（ps-code 1） |
| [code/21](../code/21-label-channel-parcel-cancellation-ignores-carrier-pickup-and-finals.md) 面单渠道包裹取消不核承运商收寄与终局 | PS | **R1** | 无 | ps PS-02（ps-code 2） |
| [code/22](../code/22-customer-face-http-contract-misses-accepted-adrs.md) 客户面 `!found` 答 500、`/v1` 与问题详情未落 | PS | **R1** | 无 | ps PS-07、PS-24（ps-code 3、12） |
| [code/23](../code/23-final-outcome-rederivation-cannot-express-a-voided-delivery.md) POD 失效致有效交付失效表达不了 | PS | R3 | 无 | ps PS-23（ps-code 11） |
| [code/24](../code/24-closed-authorization-rule-and-contract-versions-keep-granting.md) 授权规则、合同收尾后授予与委派照旧生效 | PC | **R1** | docs-consistency/07（裁收尾对授予与委派的作用） | pc-pp H2 |
| [code/25](../code/25-sell-volumetric-factor-and-amount-rounding-are-read-from-the-price-card.md) SELL 体积系数与金额取整只读价卡 | PP（连 PC） | R2 | docs-consistency/07（裁 SELL 计价条款的权威工件） | pc-pp H5；cross B2-02、B2-03（B） |

追加进既有票（「文档自洽审计复核（2026-10-21）」一节）：

| 票 | 上下文 | 分级（建议） | Blocked by | 审计条目 |
|---|---|---|---|---|
| [demo-chain-gaps/130](../../demo-chain-gaps/issues/130-handover-correction-from-pending-to-handed-over-enters-the-segment.md) | TF | R2（与票面既定一致） | 无 | tf M17 |
| [product-strategy-boundary/36](../../product-strategy-boundary/issues/36-submission-authorization-business-role-face-remainder.md) | CC | R3 | 无 | cc L8 |
| [demo-chain-gaps/117](../../demo-chain-gaps/issues/117-projection-entries-are-stored-without-their-supersession.md) | VE | R3 | 无 | ve VE-03 |
| [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md) | VE（连 CC） | R2 | 无 | ve VE-08 |
| [demo-chain-gaps/135](../../demo-chain-gaps/issues/135-departure-gate-signals-carry-causal-condition-and-fact-basis.md) | VE | R2（与票面既定一致） | 无 | ve VE-09 |
| [demo-chain-gaps/123](../../demo-chain-gaps/issues/123-tracking-projects-a-departing-bag-as-one-parcel-and-its-members-get-no-departure.md) | VE | **R1**（静默丢失成员 `DEPARTED`，与票 53 同一后果类） | 无 | ve VE-10 |
| [demo-chain-gaps/15](../../demo-chain-gaps/issues/15-occurrence-revision-does-not-drive-a-cost-correction.md) | SA | **R1**（上线就绪评估已升） | 票面原有 | sa-cr M8 |
| [demo-chain-gaps/155](../../demo-chain-gaps/issues/155-initial-route-unconfigured-undecided-burns-the-acceptance-partition.md) | NR | **R1**（范围扩到并列与成本类各格、兄弟包裹连坐） | 无 | nr-no H1 |
| [demo-chain-gaps/115](../../demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md) | NR | **R1** | 无 | nr-no H3、L15 |

只补指针、不承接条目：[ve-customer-view-per-projection-version/01](../../ve-customer-view-per-projection-version/issues/01-does-customer-view-keep-a-generation-per-projection-version.md)（VE-01 指出它的裁定前提在代码里不成立，结论不变，缺陷归 code/05）；[first-party-api/spec.md](../../first-party-api/spec.md)（PS-24 的实施票是 code/22）。

修法只是文档注记、不立代码票的 C 类，交修订票：

| 审计条目 | 交给 | 写什么 |
|---|---|---|
| cc L9（逐包裹、逐申报行范围的 AT 行） | docs-consistency/03 | UC-CC-006 `AT-CC-146`、`AT-CC-147`，UC-CC-009 `AT-CC-279` 一节加首发注：结果范围只到申报单元一级，单元以下等 `PAR-CUS-01/02`（ADR-0226 越权风险点 2）；机制缺口 ADR-0226 已承认 |
| nr-no L3（AT 行以现行口吻写尚无代码的能力） | docs-consistency/05 | 路由指令、路由偏离、集运兼容判断、实测触发、人工改路几块在各 UC「给开发的交接」列为未实现并指票；已知承接票：demo-chain-gaps/34（复评新计划信封）、35（路由偏离）、44（节点实测再准备），其余无票的由 05 判要不要立 |

### 已由某票消解

C 类 37 条：**无**——复核时现象全部仍在（TF 四条落在票 88、146、94 改过的包里，逐条核过，相关符号与注释原样在）。

B 类里审计写了「裁某向则立代码票」的，其中已被进 main 的票裁掉的：

| 审计条目 | 消解它的票 | 依据 |
|---|---|---|
| cross B2-06、pc-pp H1、tf M14（收尾供应商协议版本对在途委托的作用，三处同一件事） | demo-chain-gaps/88（resolved，全量钉 `b20bf209`） | ADR-0231（`d238b6023`）裁为委托不随 PC 收尾改指、由「委托协议换版」运营决定指名后继、收尾后结束的参与进「实际履约未形成」册，并停用 ADR-0233 越权风险点里那条运营约束原句；审计写的「算既有引用 → TF 协议核对改走不折收尾的口」那一支代码票不再成立。修订票 07、08 复核时只补文档 |

其余条件性 B 类（裁某向才要改代码），不在本票立票，由各修订票裁定后按裁向自行立：cc H3（`RegulatoryRestriction` 改内部合规限制模型）→ 03；ps PS-21 / ps-code 6（反向裁则三个编排加 `NOT_FORMED`）、PS-22 / ps-code 10（形成则加 `ExternalIdentifierLapsed`）→ 06；ve VE-04（按需）、VE-05（分开则加一格）→ 04；cross B2-05（双人复核归租户治理则改复核门）→ 07 或 10。

### R1 一句话摘要

- code/05 VE：客户视图替代时原地改写已发布那一行，对外发布历史不留痕，且每个投影版本都换一代。
- code/06 VE：生产上从不形成 ETA，客户视图的 ETA 维却按策略答「展示」。
- code/14 CR：同一笔代收本金能凭三层事实各入一次账，短款又从已到账本金里挪差额，`应付客户`与真实到账对不上（待 09 裁入账层）。
- code/16 NR：每次场外揽收复核都拿客户场所比计划节点码，原计划被判失效（待 05 裁比法）。
- code/20 PS：补件形成新提交版本后，旧版本的预付冻结或额度占用永不释放。
- code/21 PS：面单渠道包裹在承运商收寄、甚至已有终局之后仍能取消，租户一登取消授权即生效。
- code/22 PS：客户面 `!found` 答 500 让离线客户端无限重投，`/v1` 与问题详情未落，上线即固化成兼容性基线。
- code/24 PC：退役或截止授权规则、合同之后授予与委派照旧生效，同起点重发时还命中已退役的那一版（待 07 裁）。
- demo-chain-gaps/123 VE（建议）：整袋出发时袋内每件成员的 `DEPARTED` 永久缺失。
- demo-chain-gaps/15 SA：发生项有效性修订不驱动预期成本纠错。
- demo-chain-gaps/155 NR（建议）：初始路由一件包裹停在并列或未配置，整封回滚，同委托已判成的计划一起撤掉，并列候选不留痕。
- demo-chain-gaps/115 NR（建议）：复核没接候选成本取数，首个计划、受控改路与改路建议在生产上都走不通。
