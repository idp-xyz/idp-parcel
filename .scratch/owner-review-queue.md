# owner 复核队列 · 钉 `ade810bd` · 2026-09-07 23:30

由 `scripts/owner-review-queue.ps1` 生成；只有摘录，判断看原文。复核完一条：认可就在原文旁写一句「owner 复核 YYYY-MM-DD 认可」，不认可走 supersede。

## 一、ADR 里的越权风险点

### 0027-multi-step-cross-context-protocol-state-held-by-the-provider.md

> **提供方后续阶段的入参从闭包本体改为解析标识加调用方身份，按标识取回原闭包。** 用例「应用流程」表的步骤 5 要求 `party-commercial`「固定解析标识……返回不可覆盖解析结果」，`AT-PC-024` 要求相同输入与修订「返回原解析语义」——两处都指向提供方对一次解析负有超出单次调用的责任，本记录把它明确为**按标识取回**。取回后必须重校调用方身份与该解析键的租户、客户账户是否一致，不一致按 `输入未受理` 处理：…

### 0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md

> [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) 把跨上下文多步协议的中间状态判给提供方，消费方只回指解析标识，提供方「按标识取回原闭包」。它同时要求「取回后必须重校调用方身份与该解析键的租户、客户账户是否一致，不一致按`输入未受理`处理：解析标识不得成为一张能力凭证，否则 `AT-PC-028` 的越权探测会从这里进来」…

> **那条防线堵住了读取，没堵住回答。** `party-commercial` 两个取回步骤都照它实现了，也都拒绝了越权调用，但三种取不到各自落在不同取值上：

> **声称与实现在这里是反的。** 本记录落地之前，两处取回的注释都写着本上下文「说不出它是不存在还是不属于你」，而代码说得出。更要紧的是 `TestARevalidationNamedByAnotherCustomerIsNotAccepted` 与第二阶段的同名守卫：两条当时都以 `AT-PC-028`「不泄露候选」为由，要求越权探测返回`输入未受理`——**两条声称在守护该验收项的测试，恰好是保证泄露的那两条。**

> - 本记录不解除任何闸门，也不改变 [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) 关于中间状态归属与端口按阶段分方法的任何一条。它只收窄那条记录里「不一致按`输入未受理`处理」一句所指的取值：越权那一支改落「回第一阶段重解」的取值，而该句要防的「解析标识不得成为一张能力凭证」由本记录加强——原先只挡读取，现在连回答也不…

> - **越权探测一律返回技术错误。** 否决：违反 [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 与本上下文一贯的分界——那是一个业务答案而非故障，译成错误会让消费方的续办路径变成内部重试，对着一个永远不会成立的请求重试到底。
> - **合并成一格，但在结果里附一个只有运维可读的诊断码。** 否决：那个码会随结果穿过端口，消费方读得到它就等于没合并；读不到则它不属于结果代数，应当留在提供方的审计里，而审计本就该记。
> - **连`输入未受理`的短路支也一并合并，让取回失败只有两格。** 否决：短路支在查询发生之前就决定了答案，不依赖任何解析存不存在，因此不构成预言机；合并它只会抹掉一个调用方用得上的区别（改自己的请求 vs 回第一阶段重解），那正是全函数那条要挡的。
> - **不立记录，只修这两个函数。** 否决：违反红线「单一权威」。分格维度是这条边上第一次被明确回答，散在两个函数里，下一个写取回步骤的人只能靠反推，而反推最容易得出的正是被本记录否决的第一条。

> - [ADR-0022：HTTP 状态码只回答「有没有形成答案」，业务判别一律进响应体](./0022-http-status-carries-answer-formed-not-business-verdict.md)：越权探测是业务答案而非故障，本记录沿用同一条分界
> - [ADR-0003：采用集团租户、法人责任与货主客户账户三级边界](./0003-group-tenant-legal-entity-customer-account.md)：被保护的隔离边界是客户账户这一级，跨租户由取回端口按租户取已经挡住
> - [UC-PC-002：按范围与时点解析商业依据](./../application/party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md)：`AT-PC-028`「不泄露候选」与`输入未受理`定义的出处
> - [party-commercial 上下文](./../domain/party-commercial/CONTEXT.md)：结果代数所属的领域语言

### 0055-business-endpoint-intake-has-an-unconfigured-grade.md

> **四、与 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)「越权探针与真不存在同答」的关系写明：该条保护租户域资源的存在性，不保护产品能力目录。** 未配置格只披露「本产品有此端点、渠道未配置」，那是产品表面；租户数为零时也没有任何客户数据可泄。业务资源层面的探针同答约束在真渠道 Intake 就位后照常适用，本记录不触碰…

### 0113-carrier-master-document-is-an-independent-register-keyed-by-declared-reference-and-version.md

> ## 越权风险点

### 0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md

> ## 越权风险点

### 0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md

> Status: Accepted（2026-09-07，MCP-6 按 MCP-1 派单 task-aafca372「owner 授权自决口径：硬句不改、决定写理由、越权风险点单列供用户复核」裁决。裁决能力边界：读过 `internal/partycommercial/domain/authority_grant.go` 全文、`application/adjudicate_commercial_authorization.go` 全文…
> Date: 2026-09-07

> ### 越权风险点（单列，供 owner 复核）

> - **委派缺席答未配置。** 否决理由在 Decision 三与越权风险点第一条。
> - **顺手加撤回格。** 否决：一票一格；撤回那条线（`PAR-COM-14`「客户及其授权代表」）要不要走委派、走哪种委派方，归它自己的建模。

### 0119-label-validity-is-a-declaration-slot-on-the-final-rule-content.md

> Status: Accepted（2026-09-07，MCP-6 按 MCP-1 派单 task-390c4f53「owner 授权自决口径：硬句不改、决定写理由、越权风险点单列供用户复核」裁决。裁决能力边界：读过票 [pc-gaps/09](../../.scratch/party-commercial-context-gaps/issues/09-final-rule-content-has-no-validity-declara…
> Date: 2026-09-07

> ### 越权风险点（单列，供 owner 复核）

### 0120-source-data-amendment-allowance-is-a-third-stage-content-family-on-the-rule-package-version.md

> Status: Accepted（2026-09-07，MCP-6 按 MCP-1 派单 task-390c4f53「owner 授权自决口径：硬句不改、决定写理由、越权风险点单列供用户复核」裁决。裁决能力边界：读过票 [pc-gaps/10](../../.scratch/party-commercial-context-gaps/issues/10-source-data-amendment-allowance-is-a-third…
> Date: 2026-09-07

> ### 越权风险点（单列，供 owner 复核）

### 0123-decimal-canonical-spelling-is-a-value-invariant-and-digests-compare-canonical-spellings.md

> - [票 wiring-baseline-remainder/07](../../.scratch/wiring-baseline-remainder/issues/07-pp-decimal-rebuild-boundary-accepts-non-canonical-spellings.md)：四格取证、甲乙丙三条路、裁决与越权风险点
> - [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：换号的判据（「规范化结构的变化」）与「代价在此刻支付最低」那句
> - [ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md)：金额取整策略不受本记录影响；`RoundToIncrement` 经 `decimalFromBig` 的去尾随零是本记录之前唯一顺手规范掉写法的路径
> - [parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)：「版本内容摘要」「规范化版本」词条与重放不变量——本记录让它们在数字写法这一格成立，不改写它们

### 0124-evaluation-replay-is-triggered-through-a-parcel-api-command-endpoint-and-never-handed-to-settlement.md

> Status: Accepted（2026-09-07，用户经 IDP 队列通道 1 派单 `task-269d98b6` 授权 owner「自决口径：硬句一字不改、每个决定写理由、拿不准或跨上下文归属的点单列越权风险点」，据此对 [wiring-baseline-remainder/06](../../.scratch/wiring-baseline-remainder/issues/06-pp-replay-pricing-eval…
> Date: 2026-09-07

> 因此本记录裁的是**触发面走哪条缝、命令三样由谁给、结果去不去结算**。它没有裁 W02 的批量形状、争议复核的业务触发规则、也没有裁管理台表单。越权风险点单列供复核：（1）决定五引 `AT-SA-173` 判「回放交出去会被采用成第二笔费用」是从 SA 验收句推的，没读 SA 消费适配器怎么认 `replayOf`——若 SA 侧已按 `replayOf` 跳过，本决定仍成立（不交更安全），但理由要改口；（2）决定三让触发方声明证据层…

### 0125-parcel-shipment-financial-control-result-carries-per-item-results-and-folds-in-the-domain.md

> - 越权风险点单列在票 03「裁决」节，供 owner 复核：ADR-0047 两格未 supersede；ADR-0122 决定四折叠位置那句字面过时；「未执行可算」依赖 SA 停在首处限制与闭包可查；释放触发以 SA 账本语义为依据。

> - **结论合成一格「通过」，去掉 `HELD` / `CREDIT_EXPOSED`。** 否决于本票：它改 ADR-0047 决定三的词汇、迁移 `0005` 的 CHECK 与所有读面，而两格降为投影、结论只从逐项推出之后，冗余已经没有第二个来源；合不合是 ADR-0047 的改动，单列为越权风险点。
> - **保留四参构造器作兼容。** 否决：它就是「结论与逐项可以不一致」的入口，且从结论反推逐项（`HELD` → 一项预付冻结）是在发明事实。
> - **释放仍按 `HELD` 发，另给 `CREDIT_EXPOSED` 加一条。** 否决：组合策略下 `RESTRICTED` 前面的成立项仍是孤儿；「有没有占用」是逐项的事实，不是结论的事实。
> - **本票就建 PS→PC 读处置的适配器，`AUTHORIZED_DISPOSITION` 先停未决。** 否决：停在哪一格、谁续办、读面在哪都未裁，一个没有续办方的未决与一个没人读的读口同样是「进了没人用」；且它会让今天所有 `RESTRICTED` 的确定性拒绝（`AT-PS-035`「确定失败时不接受」）在处置读不到时也停下。流程与读口同票（04）。
> - **记「未执行」项。** 否决：没有结论与依据的「项」；要列全项得读正文，为的是「全部通过」之下改不了结论的信息；可由已采用解析算出。

> - 票 [sa-preacceptance-policy-view/03](../../.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md)：三问、取证与越权风险点——本记录的出处；[04](../../.scratch/sa-preacceptance-policy-view/issu…
> - [ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md)：SA 逐项交回、释放两本账各认领、折叠归 PS——本记录承接它的决定四，把折叠位置从适配器搬进领域
> - [ADR-0115](./0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md)：失败处置「只答委托去向」与共同通过条件在父行——决定五、六的依据
> - [ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)：`CREDIT_EXPOSED` 第四格与两本账——决定二保留其词汇、决定四换掉「释放编排按结论找账本」那一半
> - [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：适配器只翻译不判断、封闭集全函数、集外不吸收——决定一、三的依据
> - [ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：重建与构造分属两扇门、重建只校验不重算——决定二「重建门校验所记结论与逐项一致而不覆盖」的依据
> - [parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「只采用预付冻结、信用校验、明确无控制或合同明确组合所形成的权威结果与依据」；[settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)：「由 `parcel-shipment` 按策略的共同通过条件形成接受判断」
> - [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：校验组「接受前财务控制」行与 `AT-PS-035`——本记录改口的两处
> - `internal/parcelshipment/domain/acceptance_basis.go`、`domain/judgment_translation.go`、`application/form_acceptance_decision.go`、`application/reject_shipment_request.go`、`application/withdraw_shipment_request.go`、`adapt…
> - 来源：IDP 队列通道 1 的授权（2026-09-07）

（共 11 篇 ADR 含越权风险点）

## 二、needs-info / blocked 的票

- `.scratch/auto-reroute-demo-reachability/issues/02-syn-vertical-run-reaches-reroute-after-lapse.md`
  Status: blocked——通道 2 于 2026-09-07 按票面「开工第一步」读完 `PlanReview` / `reviewCandidates` 与生产 `InitialRouteEvidenceView`，结论写在下方「裁决」节：交付第 1 件的头一步（登 SYN 网络定义让初始路由成立）在今天的生产装配里**走不到**——唯一的证据视图实现 `nrpostgres.Netwo…
- `.scratch/first-tenant-runway/issues/03-network-resolution-layer.md`
  Status: blocked（2026-09-02 MCP-5 答完三问；结论是被 `PAR-NET-14` 硬阻断，**本票暂不拆实现票**。阻断解除前唯一可动的是一张前置票，见 `## Answer` 第四节）
- `.scratch/nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md`
  Status: needs-info
- `.scratch/ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md`
  Status: needs-info
- `.scratch/ps-port-remainder/issues/01-label-validity-rule-is-a-lapse-declaration-on-the-final-rule.md`
  Status: blocked——三问已由 MCP-1 代裁（owner 授权，2026-09-07，见 Comments），裁决与本票「裁决」节一致；**PC 半边等 pc-gaps 批（MCP-3）**：`FinalRuleContent` 长一格有效期声明 + 读口 + 发布通道；**PS 半边（消费适配器 `label_validity_rule.go`）Blocked by PC 半边*…
- `.scratch/ps-port-remainder/issues/03-source-data-amendment-authorization-needs-a-pc-action-kind-and-a-decider.md`
  Status: blocked——三问已由 MCP-1 代裁（owner 授权，2026-09-07，见 Comments）；**PC 半边（`AuthorizedAction` 加「资料修订」格 + 合同委派执行器 + 裁定结果带实际决定方）等 pc-gaps 批（MCP-3，pc-gaps/08 起）**；**PS 半边（适配器照撤回那只）Blocked by PC 半边**；「顺带量到」的生…
- `.scratch/syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md`
  Status: needs-info——ADR-0072 口径的重启条件已满足，票面范围已按其 Consequences 改写（见「重启范围」）；S0 的证据缺口三项未到，未到前不进 S1
- `.scratch/ve-008-late-account-rederive/issues/04-ops-replay-endpoint-blocked-on-par-int-01.md`
  Status: needs-info

（共 8 张）

## 三、票面里提到越权风险点的票

- `.scratch/label-channel-service-first-release/issues/24-source-correction-version-refused-as-second-responsibility-start.md`（2 处）— **能力边界**：读了 UC-PS-003 全文、PS CONTEXT 责任起点与更正各句、`adopt_network_intake.go` / `intake_adoption.go` / 迁移 0004 / `network_intake.go` / `pickup_source.go` / `adopt_on_…
- `.scratch/party-commercial-context-gaps/issues/08-authorized-action-lacks-source-data-amendment-and-adjudication-names-no-decider.md`（3 处）— Status: resolved——2026-09-07，MCP-5（task-0d116e60，分支 `mcp5-pcgaps08` 基 `299f2a2e`）：代码半边四笔由 MCP-1 重放进 main（`1e182860`→`0cfe3571` / `6b43484c`→`31903cc5` / `df923d…
- `.scratch/party-commercial-context-gaps/issues/09-final-rule-content-has-no-validity-declaration.md`（2 处）— Status: resolved——2026-09-07，MCP-6（task-390c4f53，分支 `mcp6-pcgaps09` 基 `250e5a43`）：ADR-0119 六问一次答完（越权风险点三条单列在 ADR 里供 owner 复核）、PC CONTEXT「面单服务终局规则」词条补段、迁移 0026、领…
- `.scratch/party-commercial-context-gaps/issues/10-source-data-amendment-allowance-is-a-third-stage-content-family.md`（1 处）— Status: resolved——2026-09-07，MCP-6（task-390c4f53，分支 `mcp6-pcgaps10` 基 `9379c716`）：ADR-0120 七问一次答完（越权风险点四条单列在 ADR 里供 owner 复核）、PC CONTEXT 新词条 + Boundaries 一句、迁移 …
- `.scratch/ps-port-remainder/issues/05-customs-and-node-operations-need-parcel-keyed-stage-fact-read-faces.md`（1 处）—   - **不取 ADR-0124、不改 CC/NO CONTEXT**：两面读法都是既有生命周期句与硬句的直接读法（重开、替代、候选不得当归属），形状可逆、无新词条；跨上下文归属已由 ADR-0118 决定三/四定下。预留号 0124 未消费，请 MCP-1 收回。越权风险点两条，单列：(1) CC「已关闭」把重开读…
- `.scratch/sa-preacceptance-policy-view/issues/02-load-control-policy-reads-policy-content-items.md`（2 处）—    委托接受或拒绝决定」——多项控制在 SA 里各自成结果，编排怎么按 `JointPassCondition` 报「共同通过」而不越权成接受决定。
- `.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md`（2 处）— 2026-09-07，通道 3（task-0ec68b23），用户经 IDP 队列授权「owner 自决」口径：硬句一字不改、每个决定写理由、拿不准或跨上下文归属的点单列越权风险点。三问的答落在 [ADR-0125](../../../docs/adr/0125-parcel-shipment-financial-co…
- `.scratch/tf-carrier-master-document-register/issues/01-carrier-master-document-register-does-not-exist.md`（2 处）— Status: resolved（MCP-5 实施完成，2026-09-07，隔离分支 `mcp5-tf-cmdr`，基线 main `92579b0a`，task-895fbabf；形状取票面第 1 条「独立登记册」，落文 [ADR-0113](../../../docs/adr/0113-carrier-maste…
- `.scratch/tf-segment-lifecycle-closure/issues/09-arrival-triggers-dispatch-task.md`（2 处）— | ①②③ 成文 | ADR-0114（决定一至四、越权风险点四条）+ README 目录行 | `85956673` | `docs/adr/0114-*.md`、`docs/adr/README.md`（只加一行） |
- `.scratch/tf-segment-lifecycle-closure/issues/10-source-correction-rederives-participation.md`（2 处）— **能力边界**：读了 `actual_fulfillment_segment.go`、`segment_rehydration.go`、`fulfillment_segment.go`（端口）、`fulfillment_segment_registry.go`、`enter_fulfillment_segment.g…
- `.scratch/tf-segment-lifecycle-closure/issues/12-delivery-place-reference-seam-parcel-shipment.md`（1 处）— 1. **「收件地点引用」是什么身份。** 候选：(a) PS 为每个已接受委托版本的收件范围铸一个地点身份，引用带版本；(b) 引用就是（委托、接受基线、收件槽位）的复合指针，不另铸身份；(c) 目的地节点引用（NR 的词）——ADR-0114 越权风险点 4 已把「收件地址引用 vs 目的地节点引用」留给本票，且指…
- `.scratch/wiring-baseline-remainder/issues/06-pp-replay-pricing-evaluation-has-no-executor.md`（2 处）— **② 编排：`application.ReplayPricingEvaluationHandler`，结果代数八格。** 取原评价（租户不符视同不在册——评价标识全局唯一，读口不按租户过滤，租户在编排核；越权探针与真不存在同答）→ 按 `original.PlanReference()` 经①取原方案 → `doma…
- `.scratch/wiring-baseline-remainder/issues/07-pp-decimal-rebuild-boundary-accepts-non-canonical-spellings.md`（3 处）— ### 越权风险点（单列，供 MCP-1 / owner 复核）

（共 13 张）
