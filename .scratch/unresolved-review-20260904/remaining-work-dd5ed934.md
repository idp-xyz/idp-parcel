# 重核：`remaining-work-a3a4814.md` 各条在 main `dd5ed934` 上还剩哪些（2026-09-10 14:5x）

Category: chore
Status: done——只读重核，一次交全表；不改 [remaining-work-a3a4814.md](./remaining-work-a3a4814.md) 一字（它钉的是 `a3a4814`）、不改任何票面 `Status:`、不动代码、不提方案。

**取证锚**：`dd5ed934`（= 本地 `main` = `origin/main`，2026-09-10 14:2x），在隔离 detached 检出 `D:/tops/idp-parcel-mcp4-rework` 上核；所有 `git` 命令默认在该检出根目录执行。通道 4，2026-09-10 14:3x–14:5x，task `f985c4d0`。

**判定词封闭集**：`已落`（进了 main，给 SHA 与落点）/ `仍余`（此刻仍成立，给可重跑的证据）/ `已被替代`（指向替代它的 ADR / 票 / 提交）/ `未核`（时限内没核到，不猜）。一条里两半判定不同的，两半分写。**只写是什么，不写该怎么做。**

**票面 Status 取法**：逐文件读第一条 `Status:` 行首词（`Get-ChildItem -Recurse -Filter *.md .scratch | Select-String -Pattern '^Status:' -Encoding utf8`）；`.scratch` 共 355 份 `.md`，非 `resolved` 首词的 66 份（含无 `Status:` 行与非标准词）。**本表凡写「票 resolved」都是这么读出来的，不是从 spec 状态行抄的。**

## 表（原条目编号 | 判定 | 证据 / SHA | 机制 / 实例 | 有票否）

编号取原文的节号 + 行序。「机制 / 实例」按 AGENTS.md 那条判据（机制半边现在就做，实例半边留空拒默认值）。「有票否」只对 `仍余` 有意义：给路径或写「无票」。

### 一、原「可立即开工」六条

| 编号 | 判定 | 证据 / SHA | 机制 / 实例 | 有票否 |
|---|---|---|---|---|
| 一-1 `tf-segment-lifecycle-closure/02` 实际承运商判断 | 已落 | 票面 resolved `fe5b3a35`（MCP-2，分支 `mcp2-tf02`）；tf spec 状态行记 main：代码 tip `56d2a437`、`cmd/parcel-api` 装配行 `9e8a9202`；ADR-0103 Consequences 迁移号句改「实际落在 0013」`283fbf4d`。核：`git log --oneline -3 -- .scratch/tf-segment-lifecycle-closure/issues/02-*.md` | 机制 | — |
| 一-2 `party-commercial-context-gaps/05` 客户服务规则正文 | 已落 | 票面 `aa0608f7`（认领）..`469bb9ba`（resolved）；`SaveCustomerServiceRule` / `CustomerServiceRuleContentView` 进 main 在 `05992ce2`（ve-claims-read-seams/03 的 `Blocked by:` 行记）；迁移 `party_commercial/0023`。VE 侧后继见 五-11 | 机制 | — |
| 一-3 `label-channel/19` 有效时间规则目录 | 已落 | 分支 `mcp5-lc19-21` 重放进 main `19cf2ce5`（票面「进 main 记录」：`b3acc4d5→ec1033e4`、`dd96456e→844b52fc`…）；TF 迁移取 **0014**（不是 a3a4814 预估的 0013，0013 被 tf/02 用了）；票面 resolved `fd2d9958`；回填入口拆 `22`（draft，随 `20`） | 机制 | — |
| 一-4 `label-channel/21` 有效时间显式判断在线面 | 已落 | 同批 `19cf2ce5`（`bd44a0d0→afca5344`、`7ba40b04→1c5a7a19`…）；票面 resolved `fab43802` | 机制 | — |
| 一-5 `frontline-transition-import/01` 集运子命令 | 已落（第 2 格）/ 仍余（第 1 格） | 集运三笔 main `351d3029` / `dadcaa21` / `a4740b16`；票面 resolved `2058ebdb`。第 1 格 `RECEIVED` 行身份核对仍等 `ps-external-mark-relations/01`（needs-info，末笔 `fe18c440` 2026-09-01，此后无改动）——见 四-5 | 第 1 格：机制（建模未裁） | 有票（`ps-external-mark-relations/issues/01`，needs-info） |
| 一-6 `auto-reroute-demo-reachability/01` | 已被替代 | 01 resolved `f303d7b0`（推进表逐行重核）；「六维从哪个读面取」那一问拆到 `02`；02 于 2026-09-07 转 **blocked** `b2a78a2e`——生产唯一的 `InitialRouteEvidenceView`（`nrpostgres.NetworkDefinitions`）对已登记范围上抛 `ErrNetworkDefinitionUnresolvable`，等解析层 = ftr/03 = `PAR-NET-14` | 实例（阻断源） | 有票（`auto-reroute-demo-reachability/issues/02`，blocked） |

### 二、原「有人在途」三条

| 编号 | 判定 | 证据 / SHA | 机制 / 实例 | 有票否 |
|---|---|---|---|---|
| 二-1 `first-tenant-runway/07` D4 | 已落 | PC 半边 `542ebc3c`；PS 半边 `e6920d3b`（inbox「参数已登记」续办门）+ `88694d14`（dispatch 路由第三扇门）；票面 07/08 resolved `53567700`。核：`git grep -n OperatorRegistrationCompleted -- cmd/parcel-dispatch/assemble.go` | 机制 | — |
| 二-2 `pricing-reference-series-operations/05` 第 3 项余格 | 已落 / 已被替代 | 05 resolved `42238d8d`：「挂起评价数」随 05b 四步 `92cc289f`..`867c7cc3` 落；「跳到登记」那半票面 Comments 记「照 MCP-4 理由不做」——不是余格，是裁掉 | 机制 | — |
| 二-3 PP 死码删除（worktree `idp-ppclean` 零提交） | 已落 | main `7cef122f`（2026-09-07，分支 `mcp6-pp-ratchet@1d13d510`）：`MarshalPricingPlanSnapshot` / `RehydratePricingPlanSnapshot` / `ParseCanonical` 删去；`git worktree list` 已无 `idp-ppclean`；票在 `wiring-baseline-remainder/spec.md` PP 段 | 机制 | — |

### 三、原「需人裁」十二条

| 编号 | 判定 | 证据 / SHA | 机制 / 实例 | 有票否 |
|---|---|---|---|---|
| 三-1 `tf/08` 揽收登记的更正 | 已落 | 裁 **A 新版本**（票面「裁决」节）；main `91fab19d`..`27116c52`（票面 Comments「进 main 记录」，`7d7b8b5e`）；TF 迁移 0015；段侧重派生拆 tf/10（resolved）；PS 采用口缺口拆 lc/24（resolved，快进 `24d94c94`..`11ee57aa`，ADR-0117） | 机制 | — |
| 三-2 `tf/09` 到达触发派送任务 | 已落 | 四问裁 `21ee4f0`；ADR-0114；分支 `mcp4-tf09` 重放 `081cfc56`..`2346c96e`；三条缝 12 / 13 / 14 → `0c9b846f` / `ffdf1d5c` / `5dda0fb2`；父票 resolved `5dda0fb2`；生产入口端点 `5c3ce7cf` | 机制 | — |
| 三-3 `mcp4-tf03` 封存现场 | 已落 | 裁「采」`3f3a675`，main 重放 `4cbe5266`（tf spec 状态行）；分支改名 `salvage/mcp4-tf03@44808f31`（本地与 origin 同）。核：`git branch -a --format='%(refname:short) %(objectname:short)' \| Select-String tf03` | 机制 | — |
| 三-4 `label-channel/14` 落选留痕 | 已落 | 裁「是」（MCP-3）；八笔原样快进 main `19cf2ce5`，PS 迁移 0015；读面拆 `23`，23 resolved（main `6ca3d6ec`..`a4014fc6`，共享接线 `4c466b0a`） | 机制 | — |
| 三-5 `admin-write-faces/06` 只有壳 | 已落 | 裁 **②** `0cfa12f8` → 立 pc-gaps/07；07 resolved（ADR-0115 `6dc3db12`，四笔 `359b10bd`..`2939d2d9`，票面 `265da8d8`，迁移 PC 0024）；06 读面第九册 `3106cc3c` / `a560d6d8`，票面 `ce83eaf9`，快进后清点 `92579b0a`；SA 读路径后继 `sa-preacceptance-policy-view/02` resolved（ADR-0122） | 机制 | — |
| 三-6 `admin-write-faces/07` 伞票拆票 | 已落 | 拆 08–18 十一张；伞票 resolved `6a387b89`（2026-09-10 11:2x）；最后一张 18 进 main `c2a965c9` | 机制 | — |
| 三-7 `pricing/05` 05b 两件 | 已落 | ADR-0105（票面 `f25d6921` 记 `e53428c`）；四步 `92cc289f`..`867c7cc3`；票面 `42238d8d` | 机制 | — |
| 三-8 `pricing/06` 来源连接器 | 已落（契约 + `FileConnector` + `parcel-pricing-feed`）/ 仍余（CFETS 段） | 裁 `27ced913`；main `22ddf7be`..`ba7cd976`，代码 tip `f6789f0b`，迁移 `parcel_pricing/0005`（簿记 `6f9436f3`）。CFETS 连接器段票面留 draft，开工前置「部署侧登记出网能力」 | CFETS 段：实例 / 部署侧 | 无独立票（留在 06 票内一段） |
| 三-9 `pricing/09` digest 来源 | 已落 | 裁改法 3，ADR-0108 `147c7c06`；实施拆 10，10 resolved（代码 `1b84cd60`、票面 `bb9da13b`） | 机制 | — |
| 三-10 `pricing-amount-precision/01` | 已落 | ADR-0107 `d41f73da`；实施拆 02，02 resolved（换号批十二笔 `67921a58`..`07361d8e`，簿记 `ce09a667`） | 机制 | — |
| 三-11 `first-tenant-runway/09` 自愈否 | 已落 | 裁 **A**，ADR-0106 `d8285d5f`；实施 `facf9720` / `2fd2eb25` / `a1b3f0f4`，票面 resolved `aa78b2e5`，清点 `5032ec95` | 机制 | — |
| 三-12 `unmerged-branch-inventory` 余两笔 | 已落 | 第 5 笔裁「main 追加留痕成立、分支半不合」；第 13 笔操作警告 `cd21e68c`；spec resolved `78edd257` | 机制 | — |

### 四、原「实例半边阻断」七条

| 编号 | 判定 | 证据 / SHA | 机制 / 实例 | 有票否 |
|---|---|---|---|---|
| 四-1 `first-tenant-runway/03` 网络解析层 | 仍余 | 票 blocked；`PAR-NET-14` 登记册仍「待提供」（`git grep -n 'PAR-NET-14 \|' -- docs/product/PILOT-PARAMETER-REGISTER.md`）；`INSERT INTO network_routing.network_definition` 仍只在 `network_definition_test.go`；NR 迁移止于 `0009`；`Blocked by:` 行已补 `926f30d2` | 实例 | 有票（blocked） |
| 四-2 `nr-route-evidence-views/01` | 仍余 | needs-info，末笔 `b44dd518`（2026-08-24），此后无改动。同目录 03 于 2026-09-10 resolved（`85d238ce`）是另一件事 | 实例 | 有票（needs-info） |
| 四-3 `syn-wall-door-audit/01` | 仍余 | needs-info，末笔 `d23293d2`（2026-09-02）；`PAR-INT-01` 登记册仍「待提供」；检查单 C0 / C1 / C3 / C4 / C5 均 `[ ]` | 实例（S0）；S1–S4 机制但票面写「S0 未到前不进 S1」 | 有票（needs-info） |
| 四-4 `ve-008-late-account-rederive/04` | 仍余 | needs-info，末笔 `b44dd518`；等 `PAR-INT-01` | 实例 | 有票（needs-info） |
| 四-5 `ps-external-mark-relations/01` | 仍余 | needs-info，末笔 `fe18c440`（2026-09-01）；同时卡 一-5 第 1 格 | 机制（建模与排期未裁） | 有票（needs-info） |
| 四-6 `label-channel/20` 轨迹拉取节拍 | 仍余 | draft，末笔 `784a3d92`；`PAR-INT-02` 登记册「待提供：…实际渠道产品、服务方、账号持有人…」；`22` 亦 draft 随 20 | 实例（第一家真源） | 有票（draft） |
| 四-7 `tenant-implementation-01` 检查单 | 部分已落 / 仍余 | 已勾：A3、B1、B2（`d7ca59c5`）、C2、E1（`0cb8163`）。仍 `[ ]`：A1 / A2 / A4 / A5、B3、C0、C1、C3、C4、C5、D1–D3、**E2**、F1、F2、G1。核：`git show HEAD:.scratch/tenant-implementation-01/implementation-checklist.md \| Select-String '^\s*- \['` | E2 / C1 / C3 / C5 机制·归工程；其余实例 | E2 **无票**（见名单）；C1/C3/C5 由 `syn-wall-door-audit/01` 覆盖（S1–S4） |

### 五、原「应立而未立」十三条（含派单点名的三处附加核项）

| 编号 | 判定 | 证据 / SHA | 机制 / 实例 | 有票否 |
|---|---|---|---|---|
| 五-1 面单渠道链组合根与调用入口 | 仍余 | `git grep -n -E 'SelectChannelCandidate\|AssembleChannelCandidates\|ChannelCandidateCostSource' -- cmd/` **零命中**（同 label-channel/12 收口 Comment「生产可达仍差两步」）。lc/23 落的是择优决定**查阅**面（`GET /channel-selection-decisions`），不是择优入口。lc spec 状态行自 `4c7557cd` 起自述「清单尚未闭合」三处后继，至 `dd5ed934` 仍无子票 | 机制 | `dd5ed934` 上无票；通道 1 14:3x 补充：归通道 3 立票 lc/25–28（task-b5dba034） |
| 五-2 `JudgeLabelServiceFinalHandler` 三个调用方 | 仍余 | `git grep -n JudgeLabelServiceFinalHandler -- internal/ cmd/` 非测试命中只有 `internal/parcelshipment/application/judge_label_service_final.go` 自身；PS inbox 无 `external-carrier-tracking` 消费者（`git grep -i external-carrier-tracking -- internal/parcelshipment/adapters/inbox cmd/parcel-dispatch/assemble.go` 零）；`operate_label_transaction.go` 不引用它；票 10 决定口无生产写入方（`git grep ContinuedAttempt -- cmd/` 零）。ps-port-remainder/01 状态行（`dd5ed934`）亦记「三张接线票至今未立」。**订正 a3a4814 一句**：`RehydrateContinuedAttemptRegister` 早在 `0d492b8f`（2026-09-03）出基线（`git log -S RehydrateContinuedAttemptRegister -- internal/architecture/production_wiring_baseline.txt`），a3a4814 写「仍在基线」时已过期；`dd5ed934` 基线 PS 组只剩 `ParseDeliveryPlaceReference` 一条 | 机制 | `dd5ed934` 上无票；通道 1 14:3x 补充：归通道 3 立票 lc/25–28（task-b5dba034） |
| 五-3 PC 半边：`DeclaredResponsibilityOutcome` 加面单渠道两行 + `service_stage_rules.go` 逐格翻译 | 仍余 | `internal/parcelshipment/adapters/partycommercial/service_stage_rules.go` 注释「面单渠道服务的两格（非取消终局结果 / 终局失败结果）在提供方的声明词汇表里今天没有行」仍在（`git grep -n 面单渠道 -- internal/parcelshipment/adapters/partycommercial/service_stage_rules.go`）；`DeclaredResponsibilityOutcome` 四值 `EFFECTIVE_DELIVERY` / `RETURN_COMPLETED` / `SERVICE_TERMINATED` / `REGULATORY_DISPOSITION`，无面单渠道行（`git show HEAD:internal/partycommercial/domain/service_stage_content.go`）。**注意**：那份文件在 PS 消费侧适配器目录，不在 PC | 机制 | 无票 |
| 五-4 有效期规则读口适配器 | 已被替代 | `ps-port-remainder/01`（ready-for-agent，`dd5ed934`）；PC 半边随 pc-gaps/09 进 main：`838b283e` ADR-0119 + `d06192ed` 迁移 PC 0026；14:3x 派通道 6（`git worktree list` 见 `D:/tops/idp-parcel-mcp6-psr01`）。a3a4814 记为实例半边，ADR-0119 裁成终局规则上的声明槽（`PAR-COM-17`），机制半边先做 | 机制（改判） | 有票 |
| 五-5 spec 子票清单 18–21 不承接 1–3 | 已落（簿记）/ 仍余（实质） | spec 状态行与票一览自 `4c7557cd` 起自述「清单尚未闭合」；至 `dd5ed934` 24 张子票里仍无一张承接 五-1 / 五-2 / 五-3 | 机制 | 五-1 / 五-2 归通道 3 立票 lc/25–28；五-3 无票 |
| 五-6 保价三处落点 | 仍余 | `git grep -l 保价 -- internal/ cmd/` **零**；`git grep -l 保价 -- .scratch` 只命中 ftr/06、ftr spec、pc-gaps/11、tf/14、本目录两份——pc-gaps/11 与 tf/14 只把「保价条件」当声明族先例措辞（pc-gaps/11 第 7 条「保价条件日后照抄这一形」），不承接三处落点 | 机制 | 无票（PS 结构化拒绝原因 / PP 服务选项特征 / SA 客户赔付读取规则引用；PC「保价条件」声明族同样无票） |
| 五-7 SA 两件（BUY→SA inbox 消费者；外部资金事实采用发信封 + CC inbox 消费者） | 仍余 | `git ls-files internal/settlementaccounting/adapters` 只有 `http` / `parcelpricing` / `partycommercial` / `postgres`，**无 `inbox`**；`git grep -E 'SupplierExpectedCost\|BuyEvaluationAdoption' -- cmd/` 只命中 `unwired_orchestration.go` 的读面桩；SA `eventing.EventType` 常量表（`git grep -n 'eventing.EventType' -- internal/settlementaccounting/adapters/postgres`）无资金事实采用信封；`git ls-files internal/customscompliance/adapters/inbox` 空 | 机制 | 无票（`.scratch` 无 `sa-*` / `cc-*` 承接目录；`sa-preacceptance-policy-view` 四票均 resolved 且属别事） |
| 五-8 CC 五件 | 仍余（四件机制）/ 仍余（一件实例） | ① 凭证门禁持久化：`judge_credential_applicability.go` 头注仍「本用例只判、不记」；② 放行层代码映射 `PAR-CUS-01/02`：`receive_external_result.go` 注释「属实例半边」，登记册待提供；③ UC-CC-009 步 8 向 SA 交接：`git grep -i handoff -- internal/customscompliance/application` 只有 Case / Declaration / Closure 三种，无付款核对交接；④ 步 10 门禁核对读付款核对：未见执行器；⑤ 在线登记面：`cmd/parcel-api/endpoints.go` 关务只有 `/customs/external-results`、四类目录登记与 `/customs-case-requirement-registrations`，`git grep -E 'RegisterCredential\|DutyPaymentReconciliation\|ReceiveFundsFact' -- cmd/` **零** | ①③④⑤ 机制；② 实例 | 无票（`cc-case-requirement-rule-registry`、`cc-interpretation-rule-version-dimension` 已 resolved 且属别事） |
| 五-9 VE 两登记面的 CLI / 在线登记口 | 仍余 | `ports.ExceptionDisclosureRuleRegistry`（`ports/ports.go`）与 `ports.ConflictSignalRuleRegistry`（`ports/conflict_signal.go`）及 postgres 写入口在；`git grep -E 'DisclosureRule\|ConflictSignalRule' -- cmd/ internal/visibilityexception/adapters/http` 只命中 `cmd/parcel-dispatch/assemble.go` 的读侧 `NewConflictSignalRules`；`cmd/parcel-ve-register/main.go` 命令族无这两册 | 机制 | 无票（`.scratch` 提及只在 mech/08 与 a3a4814） |
| 五-10 PP 死码二选一 | 已落 | `7cef122f`；`ParseCanonical`「自称要守的事」落 wbr/07：ADR-0123 `ef1a8315`、修复 `f1b21c47` | 机制 | — |
| 五-11 VE `ClaimEligibilityRules` 两维改读 PC 点读口 | 已落 / 仍余（一格） | ve-claims-read-seams/03：`555461fc` / `99200610` / `8d9cc4ee`、清点 `91967a6b`、票面 `522ea43d`；`cmd/parcel-api/assemble_claims.go` `buildClaimEligibilityRules` 已叠 PC 规则正文读。余格：解析键来源留 nil → `ve-claims-read-seams/04`（draft，`e44a2e0b`） | 机制 | 有票（04 draft） |
| 五-12 E1 报价表形状核对 + E2 | 已落（E1）/ 仍余（E2） | E1 `0cb8163`（2026-09-04），`price-card-shape-gaps/report.md` done，三处缺口票 01–03 resolved（`67921a58`..`07361d8e`，簿记 `ce09a667`）。E2：检查单 `- [ ] E2`，report.md 写「E2 可开工」。**票面事实一处**：`tf-carrier-master-document-register/01` 已 resolved（`603abcec`，ADR-0113，迁移 TF 0017）而 shape-gaps/03 状态行仍写主单级 blocked 等它——主单级那半是否随之接上**未核** | E2 机制·归工程 | E2 无票 |
| 五-13 `synthetic-vertical-closure/design.md` | 已落 | `512b419f`（2026-09-04）头部加「本表止于 `1fff679`，此后不维护」；此前 `ad20098f` 已标失效行 | — | — |

### 六、原「纯票面簿记」十九条

| 编号 | 判定 | 证据 / SHA |
|---|---|---|
| 六-1 lc spec 状态行 / 子票表 | 已落 | `4c7557cd` → `bb321c53` → `ba883006` → `f2c753a3`；状态行末句「此后以各票文件 `Status:` 为准」 |
| 六-2 lc/13 转 resolved | 已落 | `4c7557cd` |
| 六-3 lc/14 `Blocked by` | 已落 | 票面「无（01、12 均已 resolved）」；票 resolved 同批 `f43c99ee`..`710fc6fa` |
| 六-4 两份盘点转 resolved | 已落 | `4c7557cd` |
| 六-5 awf/02 转 resolved + 更正 `registrationjson` 那句 | 已落 | `ae763578` |
| 六-6 awf/05 去「未提交」 | 已落 | `ae763578` |
| 六-7 tf/09 `Blocked by: 05` | 已落 | 票 resolved `5dda0fb2`，`Blocked by:` 行「无（原阻塞边 12 / 13 / 14 均已进 main）」 |
| 六-8 tf/02 过时协调句 + 迁移号 | 已落 | `283fbf4d` |
| 六-9 ftr spec 补 07/08/09 + 03 `Blocked by` 行 | 已落 | `926f30d2` |
| 六-10 pc-gaps spec 状态行 | 已落 | `fd913057`，后续 `9e4e90bb` |
| 六-11 pricing spec / 04 | 已落 | `fd913057`（04 resolved），`ce47b89f`（spec resolved） |
| 六-12 pricing/09 `Blocked by: 08` | 已落 | `147c7c06` |
| 六-13 unmerged-branch-inventory 去留 | 已落 | `78edd257`（转 resolved，非 handed-off） |
| 六-14 检查单 B2 | 已落 | `d7ca59c5` |
| 六-15 tracking-scope-decision-brief | 已落 | `512b419f`（superseded） |
| 六-16 product-version-closure 三份 | 已落 | `512b419f`（resolved） |
| 六-17 `.scratch/tmp-seed-verify/` 空目录 | 已落 | `dd5ed934` 检出与 `D:/tops/idp-parcel` 工作树 `Test-Path` 均 False；空目录从未入 git（`git log --diff-filter=D -- .scratch/tmp-seed-verify` 无记录） |
| 六-18 report.md C 组节指向 | 已落 | a3a4814 自身 |
| 六-19 spec 状态行普遍落后 | 已落 | 上列各条 |

### 七、原「树况与集成」四条

| 编号 | 判定 | 证据 / SHA |
|---|---|---|
| 七-1 六笔未推 | 已落 | `git rev-parse main origin/main` 同为 `dd5ed934` |
| 七-2 worktree 四棵 | 已落 | `git worktree list` 此刻只有 `D:/tops/idp-parcel`（main）、`idp-parcel-mcp2-psr03`、`idp-parcel-mcp6-psr01`（本次 detached `idp-parcel-mcp4-rework` 另计）；`idp-ftr07pc` / `idp-ppclean` / `idp-tf03` / `%TEMP%/idp-mcp1-ftr07-d5` 均不在 |
| 七-3 封存分支两条 | 已落 | `mcp4-tf03` → `salvage/mcp4-tf03@44808f31`（本地与 origin）；`mcp5-pr08` → `merged/mcp5-pr08@c4e6586b`；ftr07 三条 → `merged/mcp1-ftr07-d5` / `merged/mcp5-ftr07-d4-pc` / `merged/mcp6-ftr07-d4-ps` |
| 七-4 TF 迁移号从 0013 起 | 已落 | `git ls-files migrations/transport_fulfillment`：0013（tf/02）、0014（lc/19）、0015（tf/08）、0016、0017（承运总单）、0018（段服务动作）、0019（tf/11） |

### 末节两件（`tasks.md`「排下一波」余两件）

E1 报价表核对 → 已落（五-12）；集运子命令 → 已落（一-5）。

## 仍余且机制半边且无票

按上表原样抄编号，不合并、不排序、不提做法。第 1、2 条在 `dd5ed934` 上无票，但通道 1 14:3x 补充「此刻由通道 3 立票 lc/25–28（task-b5dba034）」——留在名单里只为对号，不要重立：

1. **五-1** 面单渠道链的组合根与调用入口——`SelectChannelCandidate` / `AssembleChannelCandidates` / `ChannelCandidateCostSource` 在 `cmd/` 零命中。**归通道 3 立票 lc/25–28。**
2. **五-2** `JudgeLabelServiceFinalHandler` 的三个调用方（TF 首次有效收寄事实到达的 PS inbox 消费者 / 面单交易定案那一拍 / 受控关闭·重开决定生效）——非测试引用只有声明文件自身；票 10 决定口在 `cmd/` 零写入方。**归通道 3 立票 lc/25–28。**
3. **五-3** PC `DeclaredResponsibilityOutcome` 的面单渠道两行与 `service_stage_rules.go` 逐格翻译——「今天没有行」那句注释仍在。
4. **五-6** 保价三处落点（PS 接受判断结构化拒绝原因 / PP 服务选项特征 / SA 客户赔付读取规则引用）与 PC「保价条件」声明族——`internal/` `cmd/` 搜「保价」零。
5. **五-7** SA：BUY 评价 → SA 的 inbox 消费者；SA 外部资金事实采用发信封 + CC 侧 inbox 消费者——SA / CC 均无 `adapters/inbox`。
6. **五-8** CC 四件机制：凭证门禁持久化；UC-CC-009 步 8 向 SA 的交接 outbox；步 10 门禁核对读付款核对；凭证登记 / 关税协作 / 资金事实 / 付款核对的在线登记面与 admin 写面——`cmd/` 对这些执行器零引用。
7. **五-9** VE `ExceptionDisclosureRuleRegistry` / `ConflictSignalRuleRegistry` 两册的 CLI（`parcel-ve-register`）与在线登记口。
8. **五-12 / 四-7** E2 Excel → 登记批次的转换工具——检查单未勾、无票；`price-card-shape-gaps/report.md` 写「E2 可开工」。

**不入名单但相邻的**：五-11 余格已有票（`ve-claims-read-seams/04` draft）；三-8 CFETS 段属部署侧登记（实例）；四-5 `ps-external-mark-relations/01` 是机制但有票（needs-info）；四-7 C1 / C3 / C5 由 `syn-wall-door-audit/01` 覆盖。

## 本文没核到的与原因

- 五-12 shape-gaps/03 主单级那半在 TF 承运总单册立起（迁移 0017）之后是否已接上：**未核**，只记两处票面的现状。
- 五-8 CC ④「步 10 门禁核对读付款核对」只以「未见执行器」记，没有逐文件读 `reconcile_duty_payment.go` 之外的编排；③ 的判据是应用层无付款核对类 handoff。
- 未跑任何 `go test`、未设 DSN；所有「零命中」「在 / 不在」都是 `git grep` / `git ls-files` / `git log -S` 与逐文件读 `Status:` 行，证据等级 `S`。
- 未核 `docs/wooolink/` 与 `docs/reference/xls/`。
- 一-5 第 1 格、四-2 至 四-6 只核了票面末笔 SHA 与登记册状态行，未查仓外是否另有进展。
