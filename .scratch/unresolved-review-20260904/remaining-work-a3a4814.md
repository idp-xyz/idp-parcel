# `.scratch` 全量复审：截至 `a3a4814` 还剩哪些活（2026-09-04 16:0x）

Category: chore
Status: done——只读复审，一次交全表；不改任何票面 `Status:`、不动代码、不合并任何分支。补上 [report.md](./report.md) 里 C 组未交的那一节，并把 A/B 两组在当日下午的变化一并重核。

## 这份文件回答什么

用户问「全面审查分析 `.scratch`，我们还有哪些工作需要完成」。答法：把 `.scratch` 里每一份未 `resolved` 的票与产物，对照 `a3a4814` 的代码与树况，按**处置类型**分成六类——可立即开工 / 有人在途 / 需人裁 / 实例半边阻断 / 应立而未立 / 纯票面簿记。每条只写事实与可答的选项，不替 owner 裁。

**取证锚**：本地 `main = a3a4814`（2026-09-04 14:58），`origin/main = 21749ef`——**主线比远端多六笔未推**（`0e5a4ea` 面单终局执行器、`754c868` 清点、`19ffad4`、`ade0fa6`、`d59cddd`、`a3a4814`，后四笔纯文档）。工作树 `git diff --ignore-cr-at-eol` 零真改。当日上午的 [report.md](./report.md) 锚 `08e62ec`，本文凡与它结论不同处均因下午的提交而变，逐条注 SHA。

**建议用词封闭集**沿用 report.md：`保持` / `可转 resolved` / `可转 superseded` / `需人裁` / `可立即开工` / `应拆票`。

## 全景（钉 `a3a4814`；数只作此刻取证）

扫全 `.scratch` 283 份 `.md`，逐文件取第一条 `Status:` 行按首词分组：

| 值 | 份数 | 说明 |
|---|---|---|
| `resolved` / `done` / `superseded` / `handed-off` | 207 | 不在范围 |
| `in-progress` | 16 | 含 spec、票、实施材料 |
| `draft` | 12 | |
| `needs-info` | 5 | |
| `ready-for-agent` | 4 | |
| `blocked` | 1 | |
| 无 `Status:` 行 | 25 | report / census / 简报，见末节 |
| 非标准状态词 | 13 | 同上 |

与上午 `08e62ec` 相比当日收口的：`label-channel/11`（`0e5a4ea` + `19ffad4`）、`label-channel/18`（`024cb5f` + `c13cf79`）、`mechanism-executor-triage` 全目录（03/05/06/07/08 + spec，`d59cddd` 收口）、`pricing-reference-series-operations/08`（`0a67406`，`acb4975`）、`first-tenant-runway/06`（`1af59c8`）、`admin-remainder-mechanism-batch` spec + 05。当日新立的：`pricing-reference-series-operations/09`、`pricing-amount-precision/01`（均 draft）。

---

## 一、可立即开工（阻塞已解、形状已裁、无人在途）

| 票 | 票面 Status | 核实（`a3a4814`） | 建议 |
|---|---|---|---|
| `tf-segment-lifecycle-closure/02` 实际承运商判断 | ready-for-agent（MCP-2 11:55 占号） | ADR-0103 与 TF CONTEXT 三处已落；`internal/` 搜 `ActualCarrierJudgment`／「实际承运商判断」**只命中 `effective_delivery.go` 的注释**，domain / ports / adapters / migration 一层都没有。MCP-2 占号后至今**零代码**；TF 迁移 `0012` 已被票 18 用掉，本票要从 `0013` 起按开工那刻重取（ADR-0103 Consequences 写的「`0012` 起」已过时） | `可立即开工`；先在频道确认 MCP-2 是否仍持有，否则释号 |
| `party-commercial-context-gaps/05` 客户服务规则正文 | ready-for-agent（MCP-2 占） | ADR-0104 已落，票面「裁决」节形状完整（父子两表照 0014、两项正文、具名 Save、点读口 `CustomerServiceRuleContentView.LoadCustomerServiceRule`、目录读面一格、CLI 批文一节）；`internal/` 搜 `CustomerServiceRuleContentView` **零命中**。同样占号后零代码 | `可立即开工`；同上确认持有人。做完要另立 VE 侧票（`ClaimEligibilityRules` 两维改读 PC 点读口，见第五节） |
| `label-channel/19` 轨迹源有效时间规则目录 | ready-for-agent | `ports.EffectiveTimeRules` 仍只有接口（`external_tracking_fact.go`）与 `adopt_tracking_material.go` 的消费，无生产实现；MCP-2 12:5x 已释号，无人占 | `可立即开工`；若落表也从 `0013` 起重取——与 tf/02 同目录，两票开工前在频道对一下号 |
| `label-channel/21` 有效时间显式判断的在线面 | ready-for-agent | `application/judge_external_tracking_effective_time.go` 在，无端点无页；要加 `cmd/parcel-api/endpoints.go` 一行（共享接线文件） | `可立即开工`；占号时点名 `endpoints.go` |
| `frontline-transition-import/01` 集运子命令 | in-progress | 票面「阻断在别处的两格」第 2 格已于 `98e1752` 解阻（集运六口收 `domain.WorkFactSource`），备料在映射表「集运：输入逐格」节；`cmd/parcel-frontline-import` 子命令今天仍只有 `intake`。这是 MCP-1 `tasks.md`「排下一波」里点过两次、至今未做的一件 | `可立即开工`（加 `consolidate` 子命令 + 模板列 + 真库往返）；第 1 格（`RECEIVED` 行身份核对）仍等 `ps-external-mark-relations/01`，不动 |
| `auto-reroute-demo-reachability/01` | draft | 票面第一步「复核依赖链」report.md A 组已代做——`cmd/parcel-dispatch/assemble.go` 已接 `NewIntakeAdoptions` / `NewReassessOnNetworkIntakeAdapter` / `NewNetworkIntakeConsumer` / 终局链，`synthetic-vertical-closure/design.md` 的 `CONS-*` 三行依赖今天都在。**剩一个具体问题**：真实 `InitialRouteJudgmentKey` 六维从哪个读面取出来喂给 `parcel-network-register -kind auto-reroute-facts` | `需人裁`一问后即 `可立即开工`；亦可 `应拆票`（demo 走通一条委托→接受→初始路由→复核 ／ 按真实键登记事实 分开） |

## 二、有人在途（不要撞）

| 票 / 现场 | 谁 | 到哪一步 | 还差什么 |
|---|---|---|---|
| `first-tenant-runway/07` D4（ADR-0094 决定四） | MCP-5，分支 `mcp5-ftr07-d4-pc`（worktree `D:/tops/idp-ftr07pc`，干净） | **PC 半边已在分支上**：`3b94bab`「时点策略声明落库同事务经 Outbox 发『参数已登记』信封」+ `01f56f4` 清点，**未合入 main**。D5 两片（`a9e3440` + `a3adb75`）已在 main | PS 半边：`psinbox.NewOperatorRegistrationCompletedConsumer` + `cmd/parcel-dispatch/assemble.go` 路由行 + `undecidedDisposition` 翻转（`TestOperatorRegistrationRollsBackUntilItsResumeTriggerLands` 反过来）+ 三条命令口的 `*RulesNotConfigured` 是否也写第四格；`first-tenant-runway/08` 第一层随此笔收口 |
| `pricing-reference-series-operations/05` 第 3 项余格 | 无人占 | 05a 两刀（`1cacc72` / `aa54a4b`）、摘要条（`f4c2996`）、只看收窄（`b98368d`）已落 | 「跳到登记」——等参考序列册的逐字段登记表单（票 08 已落 `0a67406`，**这一格今天可能已可做**，接的人先核 `ReferenceSeriesPage.tsx` 的表单能否按序列预填）；「挂起评价数」等 05b（见第三节） |
| PP 死码删除 | MCP-5，worktree `D:/tops/idp-ppclean`（分支 `mcp5-pp-unwired-cleanup` = main，**零提交**） | `mechanism-executor-triage/spec` 收口时记为「应立而未立，归 MCP-5 立票」 | 票本身还没立（见第五节）；worktree 已建但没动 |

## 三、需人裁（裁完才能动手；每条给可答选项）

| 票 | 卡在哪一问 | 可答选项 | 归谁 |
|---|---|---|---|
| `tf-segment-lifecycle-closure/08` 揽收登记的更正 | 更正模型 | A 新版本（照交接/交付/移动事实/装载分配/费发生项五处既有 `Corrects` 形状）；B 失效+替代（要给 `OffsitePickupRegistry` 开改写口，打破本上下文「只插不改」）。附问：更正改了控制证据/发生时刻时参与起点是同段新起点还是新段 | MCP-3；建议票面 draft → ready-for-human |
| `tf-segment-lifecycle-closure/09` 到达触发派送任务 | 四问 | ①「到达」=对象计划段终点还是班次终点；②「派送范围」是 TF 自有词还是 NR `ServiceArea`（后者连上 `PAR-NET-14` 阻断）；③同事务调 `OpenDispatchTask` 还是异步执行器；④七件里时间窗从哪来。**票面 `Blocked by: 05` 已失效**（05 resolved `4c48bac`） | MCP-3 |
| `mcp4-tf03` 封存现场（`44808f3`） | 是否作为 `tf/06` 「补刀二」 | `ParticipationEnds` 缺席从「结果里具名格 `PARTICIPATION_END_NOT_WIRED`」（`95ea314`，已在 main）改成 `ErrParticipationEndsNotWired` 整笔不落（交付/`已交接`不形成答案）。封存笔自述「由 MCP-3 裁，本笔不裁」；分支 tip 在 main 之外一笔，worktree 干净 | MCP-3 |
| `label-channel/14` 落选留痕对象 | 留痕是不是事实 | 是 → 择优决定作为只追加决定记录落 PS 侧（引用候选与 `ChannelCostUnavailability` 四格因由，不拷内容），票转 ready-for-agent；否 → 认为 PP 评价不删已够，转 wontfix 并在 spec 写明。**票面 `Blocked by: 01, 12` 已失效** | label-channel owner |
| `admin-write-faces/06` 接受前财务控制策略版本读面 | 只有壳怎么办 | report.md 已代做取证：`migrations/party_commercial/` 无策略正文表，SA 读路径只读闭合+声明。① 按票面判据第二条记「只有壳」收口（纯提示句改动）；② 升级为 PC 正文表票，形同 `party-commercial-context-gaps/03`——选 ② 先回 PC CONTEXT 看词条要求带什么 | admin-write-faces owner |
| `admin-write-faces/07` 商业发布九类主路径（伞票） | 拆票 | 建议先拆四本「低频·逐字段表单候选」（`SERVICE_PRODUCT` / `CUSTOMER_CONTRACT` / `SUPPLIER_AGREEMENT` / `SETTLEMENT_POLICY`）；`PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 那册等上一行裁决 | 同上；`应拆票` |
| `pricing-reference-series-operations/05` 05b（第 2 项） | 两件 | ①`domain.EvaluationIssue` 要不要带结构化的序列种类（领域改动，今天种类只混在 message 文本里）；②issue 落库用子表还是 `text[]`（一次评价一组 issue，「加一列 reason_code」不对形） | pricing owner；①按 AGENTS 走 ADR 或 owner 裁 |
| `pricing-reference-series-operations/06` 来源连接器 | 两处待定 | ①出网：parcel-api / worker 有没有出网能力与代理策略——无则本票只做契约 + `FileConnector`，CFETS 留 draft；②工件存放：ADR-0008 MinIO 还是照 ADR-0092「本体存放是未配置出向缝」 | pricing owner |
| `pricing-reference-series-operations/09` 引用 digest 来源 | 三条互斥改法，需 ADR | 1 PRS-2 排除引用槽（换规范化号）；2 PC 读口透出 `content_digest`（只解口径一处）；3 digest 退出引用身份（波及价卡、评价清单）。票面倾向 3。**Blocked by 08 已解**（08 resolved） | pricing owner → ADR |
| `pricing-amount-precision/01` 金额精度槽 | 两件 | ①槽落价卡内容 / 评价请求 / 归 SA 财务采用（票面倾向 1，3 要改 CONTEXT 两句）；②是否进 ADR（倾向要）。红线：不编币种小数位表、裁前消费方不得在评价外取整 | pricing owner；SA 是消费方要知会 |
| `first-tenant-runway/09` 「等待受控补充」自愈吗 | 三选项 | report.md B 组已从代码答了两问（受控补充编排不铸信封且无生产调用方；`waitingOn=CUSTOMER_SUPPLEMENT` 结构上永不落库）。A 新 ADR 停用 ADR-0086 Context 那一句、并入 ADR-0094 提交侧；B 维持 ADR-0086、先做 ADR-0045 划出的「重触发判断」切片；C 维持现状只补取证锚 | owner；建议 needs-info → ready-for-human 并把两问的代码事实记进票 |
| `unmerged-branch-inventory` 余两笔 | 去留 | 第 5 笔 `t12-governance-register@4b5432a`：`controlled_channel_execution`（按登记幂等 `Record`）vs main `channel_execution`（追加 `Append`）**两套留痕语义**，要裁不是要合；第 13 笔 `cr04-readface@2f99bd3`：`seed.sh` 缺一段 `UNDERFUNDED` 操作警告（实测记录），真缺 | 第 5 归 `pilotgovernance` 地盘方裁语义；第 13 可直接补一段注释 |

## 四、实例半边阻断（工程不可自解；只等登记册或用户→客户）

| 票 | 阻断项 | 今天能动的 |
|---|---|---|
| `first-tenant-runway/03` 网络解析层 | `PAR-NET-14`（登记册仍「待提供」）；`network_definition` 零生产写入方；ADR-0068 决定六在力 | 无。票面 Status 写 blocked 却**没有 `Blocked by:` 行**，建议补 `Blocked by: PAR-NET-14（实例半边）` |
| `nr-route-evidence-views/01` | 同上一行（同一件事的另一张票；机制四件已随 ADR-0068 落，余两口只剩 `PAR-NET-14` 折叠规则） | 无；`保持` needs-info |
| `syn-wall-door-audit/01` accessidentity 接入 | S0 三项（`PAR-INT-01` 缺口：重试与冲突边界、资料修订入口、凭据轮换与回调）归用户→客户；S4 另卡 `PAR-GOV-03..07` | S2 已落 `cf34943`；S1/S3 逐项复核过「无一可动」（票面 MCP-4/MCP-5 两条）。**这张票的排期就是整条真实业务链的排期**，与 `tenant-implementation-01` 检查单 C0 是同一件事 |
| `ve-008-late-account-rederive/04` 运维重放口 | `PAR-INT-01` 授权依据 | 无；`保持` |
| `ps-external-mark-relations/01` 外部标识关系 | 排期决定（面单交易与外部标识子域排进主线）；面单交易聚合已在（`e35d898`），但交易级外部标识落脚点与受控关闭/重开一族仍未建模 | 无；它同时卡着 `frontline-transition-import/01` 的 `RECEIVED` 行与 NO 的 `ParcelIdentityView` |
| `label-channel/20` 轨迹拉取节拍 | 随第一家真源立（`PAR-INT-02`） | 无；`保持` draft |
| `tenant-implementation-01` 检查单 | A1/A2/A4/A5 证据处置（归用户，**窗口仍开**：证据五件仍在工作区、未进 git 历史）；B3 双币与 COD 客户书面确认；C0；D1–D3 建册与核验；F1 账号；G1 业务量基线 | 归用户/客户的动作。工程侧唯一可做而**至今未做**的是 E1/E2（见第五节） |

## 五、应立而未立（resolved 票的 Answer 里埋着的后继，今天没有票面在盯）

这一类最容易丢：它们不在任何 `Status:` 里，只活在已收口票的末尾。按上下文归组，每条注出处。

**面单渠道链（PS/PC/TF）**
1. **整条链的组合根与调用入口**——`label-channel/12` 收口 Comment 明写「生产可达仍差两步」；`cmd/` 下 `SelectChannelCandidate` / `AssembleChannelCandidates` 零命中（report.md A 组核于 `08e62ec`，下午无相关提交）。择优是运营端点还是面单交易编排的前置步，是产品流程决定。
2. **`JudgeLabelServiceFinalHandler` 的三个调用方**——`label-channel/11`（`19ffad4`）「不在本票」节：TF 首次有效收寄事实到达（`external-carrier-tracking` 信封的 PS 侧 inbox 消费者）、面单交易定案那一拍（票 06 的 `operate_label_transaction.go`）、受控关闭/重开决定生效（票 10 的决定口，**那口自身还没有生产写入方**——`RehydrateContinuedAttemptRegister` 仍在 `production_wiring_baseline.txt`）。三处各一张接线票。
3. **PC 半边**：`DeclaredResponsibilityOutcome` 加面单渠道两行并在 `service_stage_rules.go` 补逐格翻译；落地前面单渠道终局一律停在 `FINAL_RULE_UNCONFIGURED`。
4. 有效期规则读口的适配器——随首个面单渠道产品的实例登记一起立（实例半边，列出只为不漏）。
5. spec 子票清单 `18`–`21` 全是轨迹侧，**没有一张承接上面 1–3**；spec 「清单已完整」那句因此不成立。

**保价（`first-tenant-runway/06`，`1af59c8`）**
6. 三处落点「另立实现票，不在本票」：PS 接受判断结构化拒绝原因；服务选项进 `parcel-pricing` 特征；`settlement-accounting` 客户赔付读取规则引用。`.scratch` 搜「保价」只命中 06、spec 与 report.md——**后继票尚未立**。

**mechanism-executor-triage 三张实现票留下的**
7. SA（票 06）：BUY 评价→SA 的 inbox 消费者（提供方发触发那一半）；SA 外部资金事实采用**发信封** + CC inbox 消费者（票 07 第 4 条同指，归 SA 立）。
8. CC（票 07）「留给后继票的五件」：UC-CC-003 步 7 记录凭证门禁持久化；放行层代码映射 `PAR-CUS-01/02`（实例半边）；UC-CC-009 步 8 向 SA 的交接 outbox 与步 10 门禁核对读付款核对（各一张）；SA 发信封那一条；三组的在线登记面/端点与 admin 写面。
9. VE（票 08）：`ExceptionDisclosureRuleRegistry` 与 `ConflictSignalRuleRegistry` 两个新登记面只有 postgres 写入口与读口，**CLI（`parcel-ve-register`）与在线登记口未接**；形状照 `RegisterNotificationPolicy` 一族。
10. PP 死码：`MarshalPricingPlanSnapshot` / `RehydratePricingPlanSnapshot` 删除与 `ParseCanonical` 二选一（spec 收口原话「归 MCP-5 立票，不因本目录 resolved 而消失」）；worktree `idp-ppclean` 已建、零提交、无票。

**跨上下文消费缝**
11. `party-commercial-context-gaps/05` 裁决末条：VE 侧 `ClaimEligibilityRules` 两维（索赔期限、最低材料）改经消费侧适配器读 PC 点读口——VE 地盘，另立票；落地前 VE 行为一字不变。

**首个租户实施（工程侧唯一未启动的机制活）**
12. **E1 客户报价表→既有价卡/参考序列形状核对报告 + `price-card-shape-gaps` 立票**——MCP-1 `tasks.md` 在 09-02 两波派工里都排了（MCP-9 那张投给了不存在的通道），至今 `.scratch` 无该目录、无报告；检查单 E1/E2 未勾。它是纯取证（只出结构不出数），不等任何客户答复，**B1 裁决已放行**。同时 D-02（证据目录移出工作区）票面写「排在 E1 取证结束后同日执行」——E1 不做，A2 也一直等着。

**推进表**
13. `synthetic-vertical-closure/design.md` 的 `CONS-*` 三行依赖今天都已落（report.md A 组），该表若继续被当排期用会误导；持有者要么补一行「本表止于某 SHA，此后不维护」，要么 `可转 superseded`。

## 六、纯票面簿记（不改代码，各归 owner，一次可清）

| 文件 | 该改什么 |
|---|---|
| `label-channel-service-first-release/spec.md` | 状态行未列 `11`、`18` 已 resolved；子票表 `13` 行仍写 ready-for-agent（实为已落地）、`15` 行仍写 ready-for-agent（resolved `7904003`）、`14` 行阻塞栏仍写 `12`；「清单已完整」与第五节 1–3 冲突 |
| `label-channel/13` | 三道缝落 `54ae107` + 评审补刀 `5e9d688`，棘轮基线两条已剪，四条完成判据票面自证——`可转 resolved` |
| `label-channel/14` | `Blocked by: 01, 12` 两者均 resolved，改「无」 |
| `label-channel/capability-shape-inventory.md`、`tracking-source-seam-inventory.md` | 已被消费为子票 `02`..`17`——`可转 resolved`（或按只读产物写「已消费」） |
| `admin-write-faces/02` | 四片全落、两缺口由票 03/04 收走，只剩 `pnpm build` 一格按 MCP-3 定的「以 `tsc --noEmit` 代替」记法写明——`可转 resolved`。票面一处事实错顺带改：`networkrouting/adapters/registrationjson` 不存在（核于 `1af59c8`） |
| `admin-write-faces/05` | Status 仍写「resolved（未提交）」，CLI 第八族已提交 `6e40312`——去掉「未提交」 |
| `tf-segment-lifecycle-closure/09` | `Blocked by: 05` → `无（05 已 resolved 4c48bac）` |
| `tf-segment-lifecycle-closure/02` | 「票 03 拆出的 06 号票在动那一段，先在频道对一下顺序」——06 已 resolved `8165c84`，协调句过时；迁移号按开工那刻重取 |
| `first-tenant-runway/spec.md` | 子票表只列 01–06，**07/08/09 不在表内**；状态句仍是立批时文字；`03` 票面补 `Blocked by:` 行 |
| `party-commercial-context-gaps/spec.md` | 状态行写「05 draft → 待 MCP-3 裁」，05 实为 ready-for-agent 且 ADR-0104 已落 |
| `pricing-reference-series-operations/spec.md` | 状态行写 03 in-progress（票文件已 resolved）、04 in-progress；`04` 的 04b 三件已并入 08 且 08 resolved（`0a67406`）——`04` `可转 resolved`，spec 同笔对齐 |
| `pricing-reference-series-operations/09` | `Blocked by: 08` 已解 |
| `unmerged-branch-inventory/spec.md` | 盘点已完，余两笔已交各归属方（第三节末行）；按其自述「不自行转 resolved」可 `保持`，或转 `handed-off` 让清点不再把它数进 in-progress |
| `tenant-implementation-01/implementation-checklist.md` | **B2 可勾**：决策简报已记「落文已办（ADR-0089，ADR-0021 前向指针，`32b78d7`）」，检查单 B2 仍 `[ ]`；`spec.md` 状态行「两项的落文分别归…」那半句随之过期 |
| `admin-web-uiux-20260824/tracking-scope-decision-brief.md` | 「取证完成,待裁决」——`ve-operations-tracking-read` 全批已 resolved，`可转 superseded` |
| `product-version-closure/{design,design-input,open-decisions}.md` | 三份自述「已落地」，状态词统一为 resolved |
| `.scratch/tmp-seed-verify/` | 空目录（零 `.md`），可删 |
| `unresolved-review-20260904/report.md` C 组节 | 指向本文 |

## 七、树况与集成（不是票，但会挡人）

- **六笔未推**：`origin/main = 21749ef`，本地 `main = a3a4814`。`754c868` 已在 `0e5a4ea` 干净检出上重生成清点，其后四笔纯 `.md`，清点应仍新鲜——推之前照规矩再核一次「Mechanism inventory is current」。
- **worktree 四棵**：`D:/tops/idp-ftr07pc`（`mcp5-ftr07-d4-pc`，两笔在途，干净）；`D:/tops/idp-ppclean`（零提交，干净）；`D:/tops/idp-tf03`（`mcp4-tf03` 封存笔，非集成候选，等 MCP-3 裁）；`%TEMP%/idp-mcp1-ftr07-d5`（detached 于 `d59cddd`，干净，D5 已合入——**可拆，不加 `--force`**）。
- **封存分支两条**待处置决定：`mcp4-tf03@44808f3`（第三节）与 `mcp5-pr08@de28399`（票 08 已 resolved，封存内容已被 `0a67406`/`51e3c84` 吸收；指针照惯例保留）。
- **TF 迁移号**：`0012` 已用；tf/02、label-channel/19 若落表都从 `0013` 起按开工那刻重取。

## 末节：无 `Status:` 行与非标准状态词的文件

与 report.md 末节同一批（25 + 13 份），下午无增减；它们是 report / census / 简报 / 交接档，按其自述性质不需要 `Status:`，不建议逐份补状态行。其中两份要单独说：`synthetic-vertical-closure/design.md`（第五节第 13 条）与 `tasks.md`（MCP-1 派工日志，末段「排下一波」里五件今天只剩两件未办：E1 报价表核对、集运子命令——其余已随后续提交落地）。

## 本文没核到的与原因

- 未跑全仓 `go test ./...`，未设 DSN：所有「零命中」「在/不在」都是静态读符号与 `git` 取证，证据等级 `S`。
- 未打开 `docs/wooolink/` 与 `docs/reference/xls/` 客户文件。
- 第五节第 8 条 CC「五件」照票面转录，未逐件核代码。
- `pricing/05` 第 3 项「跳到登记」是否已因票 08 的表单而可做，只提了问，未核 `ReferenceSeriesPage.tsx`。
- MCP-2 对 tf/02 与 pc-gaps/05 的占号是否仍有效，频道未问，只据「占号后至今零代码」如实记。
