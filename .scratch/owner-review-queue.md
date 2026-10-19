# owner 复核队列 · 钉 `980c16d5` · 2026-10-19 00:55

由 `scripts/owner-review-queue.ps1` 生成；只有摘录，判断看原文。复核完一条：认可就在原文旁写一句「owner 复核 YYYY-MM-DD 认可」，不认可走 supersede。

## 一、ADR 里的越权风险点

### 0113-carrier-master-document-is-an-independent-register-keyed-by-declared-reference-and-version.md（已有 owner 复核记录）

> ## 越权风险点

> - owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，五条逐条）：1. 「关联重述」作第三种版本——关联变化是一次登记事件，不是撤销也不是替代，硬塞进那两格会让替代版本的「内容变了」失真，CONTEXT 两处句子与 CHECK 连同认可；2. 「运输履约范围」落实际履约段——本上下文自己拥有的范围身份，委托 / 订舱另有两列引用…

### 0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md（已有 owner 复核记录）

> ## 越权风险点

### 0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md（已有 owner 复核记录）

> Status: Accepted（2026-09-07，MCP-6 按 MCP-1 派单 task-aafca372「owner 授权自决口径：硬句不改、决定写理由、越权风险点单列供用户复核」裁决。裁决能力边界：读过 `internal/partycommercial/domain/authority_grant.go` 全文、`application/adjudicate_commercial_authorization.go` 全文…
> Date: 2026-09-07

> ### 越权风险点（单列，供 owner 复核）

> - **委派缺席答未配置。** 否决理由在 Decision 三与越权风险点第一条。
> - **顺手加撤回格。** 否决：一票一格；撤回那条线（`PAR-COM-14`「客户及其授权代表」）要不要走委派、走哪种委派方，归它自己的建模。

> - owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，三条逐条）：1. 委派缺席答 `ErrNotAuthorized` 不答未配置——按恢复动作分格，缺的是客户的委派（业务事实）不是租户的规则（配置），把它折成未配置会让 ADR-0055 那一格失去「产品能力表面」的含义；2. 请求方作主体引用进 `Authorization…

### 0119-label-validity-is-a-declaration-slot-on-the-final-rule-content.md（已有 owner 复核记录）

> Status: Accepted（2026-09-07，MCP-6 按 MCP-1 派单 task-390c4f53「owner 授权自决口径：硬句不改、决定写理由、越权风险点单列供用户复核」裁决。裁决能力边界：读过票 [pc-gaps/09](../../.scratch/party-commercial-context-gaps/issues/09-final-rule-content-has-no-validity-declara…
> Date: 2026-09-07

> ### 越权风险点（单列，供 owner 复核）

> - owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，三条逐条）：1. 时长用 `interval` / `time.Duration`——锚是带时刻精度的业务时间，时长同精度；商业上若只以日计，收窄批文子集到 `PnD` + CHECK 整日即可，类型不换；2. 有效期落父行、一版至多一条——PS 端口只问「这一笔成功结果」，…

### 0120-source-data-amendment-allowance-is-a-third-stage-content-family-on-the-rule-package-version.md（已有 owner 复核记录）

> Status: Accepted（2026-09-07，MCP-6 按 MCP-1 派单 task-390c4f53「owner 授权自决口径：硬句不改、决定写理由、越权风险点单列供用户复核」裁决。裁决能力边界：读过票 [pc-gaps/10](../../.scratch/party-commercial-context-gaps/issues/10-source-data-amendment-allowance-is-a-third…
> Date: 2026-09-07

> ### 越权风险点（单列，供 owner 复核）

> - owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，四条逐条）：1. `closed` 是父行一格布尔——PS 端口问的是一格，按资料组封闭没有消费形状；2. `closed = true` 下允许零行——「这一版什么都不许改」是一句合法的显式话，要求至少一行等于逼登记方编一行假的，违「不给默认」；`closed` 本身可缺且…

### 0123-decimal-canonical-spelling-is-a-value-invariant-and-digests-compare-canonical-spellings.md（已有 owner 复核记录）

> - [票 wiring-baseline-remainder/07](../../.scratch/wiring-baseline-remainder/issues/07-pp-decimal-rebuild-boundary-accepts-non-canonical-spellings.md)：四格取证、甲乙丙三条路、裁决与越权风险点
> - [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：换号的判据（「规范化结构的变化」）与「代价在此刻支付最低」那句
> - [ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md)：金额取整策略不受本记录影响；`RoundToIncrement` 经 `decimalFromBig` 的去尾随零是本记录之前唯一顺手规范掉写法的路径
> - [parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)：「版本内容摘要」「规范化版本」词条与重放不变量——本记录让它们在数字写法这一格成立，不改写它们

### 0124-evaluation-replay-is-triggered-through-a-parcel-api-command-endpoint-and-never-handed-to-settlement.md（已有 owner 复核记录）

> Status: Accepted（2026-09-07，用户经 IDP 队列通道 1 派单 `task-269d98b6` 授权 owner「自决口径：硬句一字不改、每个决定写理由、拿不准或跨上下文归属的点单列越权风险点」，据此对 [wiring-baseline-remainder/06](../../.scratch/wiring-baseline-remainder/issues/06-pp-replay-pricing-eval…
> Date: 2026-09-07

> 因此本记录裁的是**触发面走哪条缝、命令三样由谁给、结果去不去结算**。它没有裁 W02 的批量形状、争议复核的业务触发规则、也没有裁管理台表单。越权风险点单列供复核：（1）决定五引 `AT-SA-173` 判「回放交出去会被采用成第二笔费用」是从 SA 验收句推的，没读 SA 消费适配器怎么认 `replayOf`——若 SA 侧已按 `replayOf` 跳过，本决定仍成立（不交更安全），但理由要改口；（2）决定三让触发方声明证据层…

> - owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，两条逐条）：1. 决定五「回放不交结算」——理由不用改口：`internal/settlementaccounting` 全文 `replayOf` 零命中（09-09 grep），SA 今天没有任何按回放引用跳过的处置，交出去就是第二笔费用；即便日后 SA 加了，不交仍更安…

### 0125-parcel-shipment-financial-control-result-carries-per-item-results-and-folds-in-the-domain.md（已有 owner 复核记录）

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

> - owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，票 sa-preacceptance-policy-view/03「越权风险点」五条逐条）：1. ADR-0047 两格降为投影而保留、不 supersede——合成一格是 ADR-0047 自己的改动，今天冗余已无第二来源，不急；2. ADR-0122 决定四「折叠在 PS…
> - 过渡态解除 2026-09-10（票 [sa-preacceptance-policy-view/04](../../.scratch/sa-preacceptance-policy-view/issues/04-authorized-disposition-flow-and-failure-disposition-read.md) 落地，[ADR-0132](./0132-authorized-disposition-decide…


### 0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md（已有 owner 复核记录）

> Status: Accepted（2026-09-08，用户经 IDP 队列通道 1 派单 `task-cc7313e8` 授权 owner「自决口径：硬句一字不改、每个决定写理由、拿不准或跨上下文归属的点单列越权风险点」，据此对票 [admin-write-faces/08](../../.scratch/admin-write-faces/issues/08-publication-form-path-common-half-ser…
> Partially suspended by: [ADR-0178](./0178-customer-contract-versions-carry-their-body-and-contract-shells-bind-service-rule-bodies.md) 决定一（决定二「没接的册与正文缺席的版本没有可比对象，按今天的样子登记声明的串」一句，仅限客户合同一册：该册不再收只有壳的版本；其余各册与其余各条不变，本记录正文不改写）
> Date: 2026-09-08

> 本记录裁的是**摘要由谁算与串的形、批量口的对账门、载体的形与批准门、预览口的路径**。越权风险点单列供 owner 复核：（1）决定一「文档只盖正文不盖壳」是从 `SaveVersion` 逐列比对推的，没逐册核过十类正文里有没有哪一类把范围或区间当正文的一部分——若有，那一册接进来时文档要带它，不换号；（2）决定三把批准引用 = 批准者主体、来源 = 载体引用写进 `ApprovalBasis`，是对既有三格的解释而非改形——若 o…

> - owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，四条逐条）：1. 「文档只盖正文不盖壳」——伞票 07 子票 09–17 逐册接进后核过：把范围 / 区间当正文一部分的册（结算政策六维、价格政策七格）都在**自己的正文**里带着它们，文档盖的仍是正文，没有一册要把壳折进文档；2. `ApprovalBasis` 批准引用 …

### 0128-other-authority-handoff-needs-both-takeover-record-and-handoff-confirmation.md

> ## 越权风险点（单列，供 owner 复核）

### 0129-credit-ratio-base-is-declared-on-the-credit-policy-content.md

> - **越权风险点（供评审与 owner 复核）**：① 「基数属政策正文」是从 CONTEXT「只提供业务判断依据」推出的，CONTEXT 此前没有逐字写基数，本记录补半句；② 成员表按「SA 能自算」选，「合同声明基数」明确不收，将来有租户提出时另立；③ `PRIOR_PERIOD_CONFIRMED_CHARGES` 的定义有三处是本记录的裁定——对账单快照作周期代理、结算账户蕴含责任法人（对账单按账户 + 币种键入而作用域三维）…

### 0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md

> Status: Accepted（2026-09-09，通道 2 按通道 1 派单 task-ddb77473「用户授权代裁，PS owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [tf/12](../../.scratch/tf-segment-lifecycle-closure/issues/12-delivery-place-reference-seam-parcel-shipment.md) 全文…
> Date: 2026-09-09

> ADR-0114 决定三把末端派送任务七件里的**地点**定为「收件地点引用（`parcel-shipment`）」，由 TF 消费侧适配器拉、落进任务的 `Place` 也是引用、地址本体留在 PS；决定四与越权风险点 4 把「引用是什么身份、从哪个读面取、粒度是收件地址还是目的地节点」留给票 tf/12 与 PS owner。TF 那一头已定：`DeliveryPlaceSource.LoadDeliveryPlace(tenant…

> **二、读口按（租户，包裹身份）问，答查询时刻的当前采用判断；委托与接受基线是 PS 内部走到答案的路，不是键。** 这一句供票 14 第 2 问直接引用：**PS 对外一律按（租户，包裹身份）答，不要求消费方持有委托、接受基线或提交版本；PS 内部经接受基线成员关系（含包裹身份谱系回到来源包裹）走到委托，再取该委托收件资料范围上的当前采用判断。** 取「当前采用」而不取「接受基线」也不取「当前提交版本」：对象进派送段时委托必已`已接受…

> ## 越权风险点

> - [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md)：决定三 / 四与越权风险点 4（本记录回答的那一格）
> - [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md)：不拥有地址的上下文不持有明文；以引用与摘要留痕的先例
> - [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：适配器落 TF 侧
> - [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：串带形状版本的先例
> - 票 [tf/12](../../.scratch/tf-segment-lifecycle-closure/issues/12-delivery-place-reference-seam-parcel-shipment.md)（三问出处）、[tf/13](../../.scratch/tf-segment-lifecycle-closure/issues/13-delivery-window-seam-network-routing…

### 0131-planned-leg-is-referenced-by-plan-version-and-ordinal-and-the-delivery-window-seam-answers-content-not-applicability.md

> - **第二分量用「起点节点 → 终点节点」而不是序位。** 未采：节点对可读性好，但一条路径原则上可以回访同一节点，序位在任何段链上都无歧义；见越权风险点。

> ## 越权风险点

### 0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md

> - 越权风险点（供 owner 复核，都不阻断票 04 开工）：
>   1. `ResumePath` 加格与 CONTEXT / UC-PS-001 / ADR-0094 里以数目指称等待态的措辞——按 ADR-0029 判据加格是对的，字面要跟改的归各自所有者（UC 归实施票，ADR-0094 正文不改）。
>   2. 处置`拒绝`由**处置授权**放行、不另问主动拒绝授权——`party-commercial` CONTEXT 把动作定为互不蕴含，「授权处置」要不要成为那边授权动作词汇里的一格、`PAR-COM-14` 实例半边怎么登记，归 PC；本记录只在 PS 开一只独立 `Authorizer` 端口。
>   3. 受限控制项结果加两格采用引用，扩了 ADR-0125 决定一的形——ADR-0125 正文不改，以本记录为准；owner 若要另立一个「采用处置」对象而不扩 `ControlItemResult`，改的是决定三那一句。
>   4. `交客户补充`即释放本版本已成立项的占用——把「接受确定未成立」读到了处置时刻；owner 若认为应保留占用到新版本形成再判，改的是决定一末段。
>   5. `补资金后重判`不入集，按「没有触发不开格」排除；它是真实的运营需要，SA→PS 资金事实缝或「已记录判断失效重判」语义两条路任一落地时回看。
>   6. 多项受限一 `REJECT` 一 `AUTHORIZED_DISPOSITION` 时 `REJECT` 优先——硬句的保守读；今天走不到，依赖 SA 停在首处限制。

### 0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md

> Status: Accepted（2026-09-09，通道 2 按通道 1 派单 task-f5521768「用户授权代裁，PC owner 口径、第 2 问连 PS owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [tf/14](../../.scratch/tf-segment-lifecycle-closure/issues/14-delivery-condition-reference-seam…
> Date: 2026-09-09

> **二、对象走到合同的路归 `parcel-shipment`：按（租户，包裹身份）答商业解析回指；委托、接受基线、接受决定是 PS 内部走到答案的路，不是键。** 这一句与 [ADR-0130](./0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md) 决定二同一句：「PS …

> - **交付条件挂接单规则包版本（ADR-0058 那一族）。** 否决：那一族是接受流程的规则；交付条件是卖出去的服务形态，与保价条件、客户服务规则同族。记为越权风险点 2 供 owner 复核。
> - **建任务时不问 PC，只把回指交给 TF，内容缺席留到有效交付那一步再发现。** 否决：UC-TF-006「派送任务已形成」固定的四件里有「规则」，没有规则的任务是让一线跑一趟注定形不成有效交付的活；ADR-0114 决定三「所有者答没有时任务待形成」在这一拍就该成立。
> - **PS 与 PC 的两个「没有」并成一格。** 否决：恢复动作不同（ADR-0029）。
> - **闭包不在场答「没有」。** 否决：对一份已接受委托的回指答「没登条件」是拿一次数据缺失冒充一句商业责任方说过的话（ADR-0080 决定四同一判据）。

> ## 越权风险点

> 5. **谱系包裹与集运单元同答「没有」。** 与 ADR-0130 越权风险点 2 同形：完整性问题藏进业务答案。

### 0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md

> Status: Accepted（2026-09-10，通道 2 按通道 1 派单 task-138ab1c9「用户授权代裁，PS owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [lc/25](../../.scratch/label-channel-service-first-release/issues/25-external-carrier-first-pickup-triggers-label-…
> Date: 2026-09-10

> - **窗口如实记**：信封到消费之间，`LabelTransactionParcelRow` 那一行答的是旧终局（越权风险点 1）。
> - `production_wiring_baseline.txt` 若因新增生产工厂无调用点而红，按既有纪律加行并写明「等 lc/28 组合根」。

> - **一封一交易，消费者逐覆盖包裹展开。** 否决：一件包裹反查不中，整封信进重投，其余包裹被重复判（幂等所以无害，但那一封永远完成不了）；分区键只能按交易，同一包裹跨交易的两拍落在两条队里，第三个场景的顺序保证没了。记为越权风险点「一封一包裹而不是一封一交易」供 owner 复核。
> - **只在 `RecordChannelResult` 触发。** 否决：第二个场景——作废那一拍改变关闭路径输入却无人去问。
> - **入队失败不翻结果、留续办引用（照 `FormParcelFinalHandler.handOff`）。** 否决：那一形留的正是「结果已落、意图未交」的中间态，是本记录取乙要消掉的东西；同事务里回滚一步的代价只是调用方重放一次结果记录，渠道答案在它手上、不需要重发渠道调用。
> - **消费者按信封所指 revision 读回交易再判。** 否决：判断读全册且读当下（CONTEXT 硬句），交易聚合按 ADR-0084 决定八只有当前快照一行、没有可按版本取回的历史；版本在这里的职责只是让两拍各自入队。
> - **把「这一拍值不值得判」在入队处先筛（比如成功结果且无作废就不入队）。** 否决：筛就是在 06 编排里复述一遍关闭路径的口径，第二套口径；判断本身便宜且无副作用。
> - **不立记录，在 lc/26 票面裁了就做。** 否决：三张票三只编排会各自照抄，触发形状是难逆转的先例（Context 节）；lc/27 的写面票与 lc/28 的组合根都要引同一句。

> ## 越权风险点

### 0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md

> Status: Accepted（2026-09-10，通道 6 按通道 1 派单 task-f05bc5a3-a675-4157-9667-14406307a22c「用户授权代裁，TF owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [lc/25](../../.scratch/label-channel-service-first-release/issues/25-external-carrier-…
> Date: 2026-09-10

> **五、已形成即参与进入：`ParticipationEntryKind` 长第三格。** 段成立句里的「有效收寄」不只是场外揽收，头注「刻意没有第三格」的理由是「扫描、装载、订舱确认都立不起参与」，而已形成的首次有效收寄不是扫描，是本上下文判过「取得运输控制」的控制事实——它正是那句硬话许可的那种进入。进段走 `enterFulfillmentSegment` 同一道门（进段是派生的一侧、失败留续办不回滚来源；段首登后同笔铸实际承运商…

> - 不在本记录内：收寄判读规则目录的形状与登记口（随第一家真源，照 lc/19 有效时间规则目录的形）；规则那一路的节拍（照 lc/20）；PS 收到失效版本后终局怎么重派生（PS owner，见越权风险点 4）；有效交付能不能同样在外部承运轨迹事实之上判出（另一题，同形可循但不在此裁）；段引用由谁铸（越权风险点 1）；UC-PS-004 依据表那行括注「（外部承运轨迹事实）」与本记录不一致，归 PS owner 随 lc/25 改口（…

> - **不加第三格，收寄事实只发给 PS、不进段。** 否决：段成立句「通过有效收寄……进入运输方控制时……才成立」说的就是它；不进段则承运商的段无从成立、后续轨迹无段可归、实际承运商判断无处铸首版。记为越权风险点 2 供 owner 复核第三格的命名与范围。
> - **待确认不落版本，只在触发结果里答。** 否决：CONTEXT「保持待确认」是一个状态不是一次返回值，「为什么取消权还没结束」得有人答得出；ADR-0103「未知也是一个版本」同一条理由。
> - **业务发生时间取源给的发生时间。** 否决：那是源的话不是本上下文判过的话（ADR-0102 决定二 / 三）；取消权从何时起结束按本上下文判过的有效时间答。记为越权风险点 3。
> - **已形成后另一来源的相反证据使收寄回到待确认。** 否决：终局已据它形成、取消权已结束，靠一条新证据（不是更正）收回边界是「按消息最后到达覆盖」（PRODUCT-BASELINE 禁句）；ADR-0103 已把这种冲突安置在段级判断里且明说不阻塞终局。
> - **规则判读塞进认领或有效时间判断的同一事务。** 否决：认领与判读是两次判断（CONTEXT 原句），写在一笔里就分不开「认领过」与「判过收寄」；异步一拍有 ADR-0114 的先例。
> - **规则未配置记为待确认版本。** 否决：待确认是判过了但不够，未配置是没人判——两格恢复动作不同（ADR-0029），前者等身份登记或人裁，后者等实例参数。


> ## 越权风险点

> - [ADR-0117](./0117-same-source-correction-forms-a-superseding-adoption-version-chained-to-the-current-responsibility-start.md)：PS 侧同来源更正的采用版本链（越权风险点 4 所指）
> - [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
> - 票 [lc/25](../../.scratch/label-channel-service-first-release/issues/25-external-carrier-first-pickup-triggers-label-final-judgment.md)（问题出处、PS 半边）、[lc/31](../../.scratch/label-channel-service-first-release/issues/31-ca…

### 0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md

> Status: Accepted（2026-09-10，通道 4 按通道 1 派单 task-d6660969「用户授权代裁，VE owner 口径，键形状连 PC owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [ve-claims/04](../../.scratch/ve-claims-read-seams/issues/04-rule-resolution-key-source-needs-a-r…
> Date: 2026-09-10

> - **目标不是包裹。** VE CONTEXT「客户索赔项」允许目标是「一个包裹或明确服务责任范围」。目标是明确服务范围时，没有任何已接受委托的包裹身份可问 PS，回指取不到。候选乙对这一格答不出规则版本；候选甲能答，但答的是「这个客户账户此刻在某范围下当前有效的规则版本」——那正是第一个场景否掉的读法，换了个目标就成立不了。这一格今天的行为是 `Keys == nil` 那一行：两维如实答未登记、编排停在指名到维的未决、索赔项一字不…
> - **接受时闭包没采用客户服务规则版本。** PS 的解析键登记面（`commercial_resolution_key`）由租户登记必需依据集合；`CustomerServiceRuleObject` 在封闭集内（ADR-0104 决定四），但没有哪一句要求它必在。租户没把它列进去，接受时闭包就没有这一成员，按回指走到闭包再 `AdoptedFor(CUSTOMER_SERVICE_RULE)` 答 `ok=false`。这是「没登…

> - 生产上这条路与 ADR-0079 / ADR-0133 同一停点：PS 的解析键登记面尚未在生产上让闭包形成（ADR-0079 Consequences、ADR-0133 越权风险点 3），机制接上前 PS 那一头对每个对象都答「没有」、两维停在未登记——与今天 `Keys=nil` 的可观察行为一字不变，装配测试经合成解析键钉「已登记」态。
> - 明确服务范围为目标的索赔项两维恒答未登记（越权风险点 1）；`ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED` 那一格对这一状态命名不准，与票 ve-claims/05 点名的两处同因，归那一票。
> - 不在本记录内：起算事实源、业务日历、`Notice` 的来源（票 03「裁决」留格三条，各归其处）；PS 解析键登记面要不要把 `CustomerServiceRuleObject` 与 `CustomerContractObject` 从「可登」改为「必登」（越权风险点 2）；PC 闭包快照是否落已采用的客户服务规则版本并读得回（越权风险点 3）。



> - **闭包在场却未采用客户服务规则版本 → error。** 否决：恢复动作是去 PS 解析键登记面列进必需依据，是登记不是修代码（ADR-0029）；与 ADR-0133 对客户合同版本答 error 不同，因为 PC CONTEXT 说合同恒在、没说客户服务规则恒在。记为越权风险点 2 的反面。
> - **目标是明确服务范围时按客户账户当前有效的规则版本代选。** 否决：正是第一个场景否掉的读法；那种索赔项按什么选规则要先有商业语言（越权风险点 1）。
> - **登记入口放 `parcel-ve-register`（票 04-4，派单方已按 A 类裁定）。** 随决定四消解：VE 没有键要登，`parcel-ve-register` 不加命令。若 owner 复核后回到候选甲，那条裁定照旧适用（ADR-0025 实例半边协作者接口留适配器包内；登键映射是写不是跨上下文读，不碰票 03 那句「受控登记口不接跨上下文读」）。


> ## 越权风险点

> - [ADR-0133](./0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md)：决定一 / 二（本记录同形的两问）、越权风险点 3
> - [ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)：决定二（`scope` 留签名不参与提问）、决定五（同源）、决定八（装配期拒 nil）、决定九（快照读写对称）
> - [ADR-0080](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)：合同维是结论不是输入；两段式串一处拼
> - [ADR-0130](./0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md)：决定二（PS 按（租户，包裹身份）答）
> - [ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md)：决定一 / 四 / 五
> - [ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md)：回指换闭包再取已采用成员的路
> - [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：分格判据
> - [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：接口留适配器包内
> - 票 [ve-claims/04](../../.scratch/ve-claims-read-seams/issues/04-rule-resolution-key-source-needs-a-registration-face.md)（四问出处）、[ve-claims/03](../../.scratch/ve-claims-read-seams/issues/03-claim-deadline-and-materials-re…

### 0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md

> Status: Accepted（2026-09-10，通道 5 按通道 1 派单 task-9a2ff746「用户授权代裁，CC owner 口径，决定四连 SA owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [sa-cc/spec](../../.scratch/sa-cc-funds-and-credential-seams/spec.md) 与 [01](../../.scratch/sa-cc…
> Date: 2026-09-10

> - **折法另立「付款门禁规则」登记册而不进门禁目录。** 否决（落位取舍，记为越权风险点 4）：门禁目录已按（范围 / 动作 / 边界）三维键伺候门禁编排，规则与认定是同一道门的两种登法，再开一册是同一把键的第二张表。
> - **`待确认` / `冲突` 可登记为接受。** 否决：UC-CC-003「未知不能当作……有效」；待确认是「还没答」，不是一个能被接受的答案。
> - **CC 保留 `external-funds-fact` 人工补录口，撞键时答 `已存在` / `内容冲突`。** 否决：两个铸造点的冲突不是幂等问题，是归属问题；SA CONTEXT 与 CC CONTEXT 同句把事实归财务系统、采用归 SA。
> - **人工补录口保留到 02 / 03 落地再去。** 否决：口径现在就定，避免 07 步一先做四个子命令再拆一个；代码上本就不动（07 步一在 02 / 03 之后开工）。

> ## 越权风险点

### 0138-voided-carrier-pickup-voids-the-label-service-final-as-a-third-ledger-record-and-rejudges-on-remaining-basis.md

> **问题。** `transport-fulfillment` 对同一条「实际承运商首次有效收寄」事实会发**失效版本**——依据被源更正为不再表达收寄（ADR-0135 决定六：「链尾失效即该对象当前无首次有效收寄，再次形成从失效版本长出新版本，不回退」）。PS 侧 `JudgeOnCarrierFirstEffectivePickupAdapter.HandleRegisteredCarrierFirstEffectivePicku…

> **四、失效之后同一事实的新有效版本照今天的路走，不新造格。** F@v3（已形成）到达 → 收寄终局 → 采用键版本 F@v3：当前无终局 → 首派生；当前有同种类终局（场景 iii 重判形成的 Tc）→ 既有分派「同种类 → 重派生」，`SOURCE_REVISED/F@v3`，证据从关闭换回收寄、生效时间换成收寄时间——这正是 AT-PS-063 重派生本来的用途。`LabelServiceOutcome` 与 `LabelSer…

> - 不在本记录内：TF 半边（ADR-0135 已裁，失效版本入队、PS 按版本取回）；VE 里程碑对「终局失效」怎么投影（VE owner）；`LabelServiceOutcome` 与 `LabelServiceFailure` 是否同源（越权风险点 2）；网络服务那四格责任结果的来源事实失效（有效交付被更正为无效等）是否同形——本记录只裁面单渠道服务收寄这一路，同形可循但不在此裁（越权风险点 4）。

> ## 越权风险点

> - [ADR-0135](./0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md)：决定六（失效版本）、决定七（按版本取回）、越权风险点 4（本记录所答）
> - [ADR-0117](./0117-same-source-correction-forms-a-superseding-adoption-version-chained-to-the-current-responsibility-start.md)：同来源更正的采用版本链——本记录循其纪律半、不循其语义半
> - [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格（决定二两式合一、越权风险点 3）
> - [ADR-0134](./0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md)：决定三「入队失败即本步 error 上抛、事务回滚、调用方重放」与越权风险点 5（本记录决定三同一取法）
> - [ADR-0005](./0005-source-facts-effective-events-derived-state.md)、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：丙的否决依据
> - 票 [lc/39](../../.scratch/label-channel-service-first-release/issues/39-voided-carrier-pickup-rederivation-needs-an-adr-before-code.md)（本记录的出处）、[lc/25](../../.scratch/label-channel-service-first-release/issues/25-extern…



### 0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md

> ## 越权风险点（归 owner 复核）
>
> 2026-10-19 用户在 Claude Code 会话里答「接受」，修订后接受；风险点按复核建议逐条处置，见 ADR 正文。仍留待 owner：9（首方族是否补静态密钥这一凭据形态，首发不补）。

### 0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md

> ## 越权风险点（归 owner 复核）
>
> 2026-10-19 用户在 Claude Code 会话里答「接受」，修订后接受；风险点按复核建议逐条处置，见 ADR 正文。仍留待 owner：5（SDK）、7（各上下文 owner 逐端点确认描述符）。

### 0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md

> ## 越权风险点（归 owner 复核）
>
> 2026-10-19 用户在 Claude Code 会话里答「接受」，修订后接受；风险点按复核建议逐条处置，见 ADR 正文，无留待项。

### 0142-first-party-webhooks-are-a-product-owned-outbound-channel.md

> ## 越权风险点（归 owner 复核）
>
> 2026-10-19 用户在 Claude Code 会话里答「接受」，修订后接受；风险点按复核建议逐条处置，见 ADR 正文。仍留待 owner：1（v1 对外事件目录由各上下文声明）、5（mTLS 与出向 IP 段）、6（1:N 账户下的订阅）、8（客户确认回写用例）。

### 0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md

> Status: Accepted（2026-09-24，通道 3 经用户在 IDP 队列授权自决——「你能帮我自决吗，你是业务和系统专家」——裁决；票 [admin-web-group-legal-entities/04](../../.scratch/admin-web-group-legal-entities/issues/04-catalogue-read-pagination-sort-filter-contract.md) 的…
> Date: 2026-09-24

> ## 越权风险点

### 0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md

> Status: Accepted（2026-09-24，通道 3 经用户在 IDP 队列授权自决——「你能帮我自决吗，你是业务和系统专家」——裁决；票 [admin-web-group-legal-entities/05](../../.scratch/admin-web-group-legal-entities/issues/05-legal-entity-business-attributes.md) 的产物。票面「裁决」节是同日早…
> Date: 2026-09-24

> ## 越权风险点

### 0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md

> Status: Accepted（2026-09-24，通道 4 经用户在 IDP 队列授权自决——「你是系统和业务专家，请你自决」——裁决；起因是用户同日一句「我们是软件公司，不可能等到租户就绪、业务就绪再去补系统，这是我们的产品，原来的决策有问题」。本记录**部分取代** [ADR-0016](./0016-product-delivery-and-tenant-pilot-as-parallel-tracks.md) 的机制半边定…
> Date: 2026-09-24

> **仓里已有两处按正确方式切开的先例。** ADR-0015：「通用性是文法的属性」——计价文法闭合、归产品，价卡的词汇与数值归租户。登记册 `PAR-NET-16`：比价必须在各候选算出计价重量之后、首发按成本单维择优、并列交人工，这些标「机制已确认」；只有候选集合与各维度取值`待提供`。另有一个现成的待决案例：ADR-0145 越权风险点 2 问 CN / SG 的公开注册号格式要不要作为产品随附的参考数据。

> ## 越权风险点

> - [ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md)：越权风险点 2 由本记录归类为参考配置
> - [开发主线：切片的机制半边与实例半边](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：三分与四条判据的运行定义
> - [参数登记册](../product/PILOT-PARAMETER-REGISTER.md)：`PAR-NET-14` 与 `PAR-NET-16` 两行
> - [合成演示动线](../design/synthetic-demo-journey-script.md)：第四条判据所指的动线
> - [ADR-0162](./0162-cost-allocation-forms-are-weight-piece-and-revenue.md)：成本分摊的按重、按件、按收入
> - [ADR-0164](./0164-buy-evaluation-request-trigger-is-occurrence-formed.md)：BUY 评价请求在发生项形成时发起，原因没登记则不发起
> - [ADR-0168](./0168-charge-attribution-date-is-a-registered-form.md)：费用归属日按已登记形态判定，没登记不形成归属日
> - [ADR-0172](./0172-route-freeze-is-remaining-segment-count.md)：冻结边界首版形态是剩余计划段数，没声明不是未冻结
> - [ADR-0173](./0173-auto-reroute-is-folded-from-the-strategy-version.md)：自动改路由策略版本折出，没声明只形成建议；事实目录不再是判断权威
> - [ADR-0174](./0174-loading-concurrency-is-judged-by-business-time.md)：改路与装载并发按业务时间裁决；同一时刻与因果不明都不并进前两格
> - [ADR-0170](./0170-settlement-orchestrations-enter-through-parcel-api.md)：结算编排从 parcel-api 进入；确认与截单没登记就不做
> - [ADR-0171](./0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md)：没有实测读口时计价输入用申报并标明来源

### 0147-reference-configuration-ships-embedded-and-is-adopted-through-ordinary-registration.md

> Status: Accepted（2026-09-24，通道 4 实施票 `product-strategy-boundary/03` 时立；该票派单写明「第 1 步的形态若属难逆转取舍，按 AGENTS.md 走 ADR」，用户同日在 IDP 队列授权通道 4「作为业务和系统专家自决」。裁决能力边界：读过 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mecha…
> Date: 2026-09-24

> **六、首个样板：注册号类型目录。** CN（统一社会信用代码）与 SG（UEN）各一份，只含公开的格式正则，不含校验位算法；`parcel-commercial register-registration-number-types` 的批文每项可写 `adopt: <标识>@<版本>`，名称、层与格式取自参考配置，依据格由入口写成引用串。首版随附哪些国家与地区、要不要带校验位算法，归 PC owner 与用户（ADR-0146 越权风…

> ## 越权风险点

> - [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：决定三「参考配置显式采用才生效」与越权风险点 4、5
> - [ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md)：越权风险点 2 问的正是本记录的首个样板
> - 票 [`product-strategy-boundary/03`](../../.scratch/product-strategy-boundary/issues/03-reference-configuration-adoption-pattern.md)：实施与各登记册依据格的清单


### 0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md

> Status: Accepted（2026-09-24，通道 5 起草，票 [routing-first-cut/02](../../.scratch/routing-first-cut/issues/02-route-evidence-sourcing-and-candidate-cost-adr.md) 的产物。同日按用户「按你的建议吧」修订决定四与越权风险点一、二：候选成本改按比较币种换算后合成，自营段经内部价格政策计成本。随后用…
> Date: 2026-09-24

> ## 越权风险点

> - [ADR-0189](./0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md)：决定四第 7 条「每段怎样折成价卡的区域词汇」与越权风险点 5 由它接手
> - 票 [routing-first-cut/02](../../.scratch/routing-first-cut/issues/02-route-evidence-sourcing-and-candidate-cost-adr.md)：本记录的派生票；实施票族见 [product-strategy-boundary/04](../../.scratch/product-strategy-boundary/issues/04-rou…

### 0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md

> ## 越权风险点

### 0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md

> Status: Accepted（2026-09-24，通道 4 据用户在 IDP 队列同日两句裁定成文——「只有合成与真实租户数据这道墙在开发的时候也无需考虑，当成真实的就行了，简化开发」，以及对本记录提案的「同意」。本记录**部分停用** [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 决定六的一句与 [ADR-0100…
> Date: 2026-09-24

> ## 越权风险点

### 0151-unassigned-command-faces-get-their-families.md

> Status: Accepted（2026-09-24，用户经 IDP 队列答通道 4 的立票提议「同意你的决定，开干，开干前，全面审查，确保确实如此」；通道 4 起草，2026-09-25 通道 2 按「全面审查」逐条对代码与文档复核后改定（通道 1 独立复核补了 TF 两个判断口），复核记录见 [operator-channel/15](../../.scratch/operator-channel/issues/15-operat…
> Date: 2026-09-24

> **七、不裁的。** 客户侧各口仍归 ADR-0139，接受与否归用户。隔离形态照 ADR-0150：关段、建派送任务、有效时间判断与承运商首次有效收寄判断今天经写开关放行，各口换上运营决定能力面的同一笔撤下；其余各口今天没有隔离放行，本记录不给它们加。授权那一段也不在本记录：委托侧五口越过 Intake 之后，今天在生产装配上各停在自己的授权缺口——复核完成、主动拒绝、受控关闭与重开的授权请求映射是 nil（票 product-str…

> ## 越权风险点

### 0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md

> Status: Accepted（2026-09-25，用户经 IDP 队列答通道 3「按你的理解和建议，你自己独立完成吧」「按你的建议，但是要理解我们的实现方式」授权自决；票 [operator-workspace-gaps/02](../../.scratch/operator-workspace-gaps/issues/02-estimate-evaluation-entry.md) 的产物。越权风险点见「裁决方的能力边界」，同日…
> Date: 2026-09-25

> 因此本记录裁的是**试算归谁、存不存、交不交、取几张卡、入口走哪条缝**，没有裁管理台表单的形状，也没有裁报价。越权风险点单列供复核：

> - 2026-09-25：用户经 IDP 队列答通道 3「你是业务和系统专家，请你自决」，授权通道 3 代为复核四条越权风险点。**这是受托复核，不是 owner 本人认可**；四条逐条维持：
>   1. 范围口径收窄：CONTEXT 已把试算能力放进首发，面向运营的入口是它最直接的消费方；试算不接受、不锁价、不形成承诺，那句要挡的是报价。
>   2. 证据层级 `S`：证据层级描述的是本次执行的数据来源，假设包裹标 `P` 会把假设冒充成生产证据；价卡是生产配置这一面不改变输入的性质。
>   3. 查阅读授予：能力面按语义分，不按 HTTP 方法分；试算不写任何册，用 `POST` 只是因为结构化声明放请求体更合适。另立「试算」能力面会给授予模型添一格没有独立授权理由的面。
>   4. 放弃留痕：要追溯的是报价，将来随报价对象自己留；把试算改成入册只会多出一个被结算误采用的面。

### 0156-reachability-closure-identity-comes-from-the-adopted-resolution.md

> ## 越权风险点

### 0157-built-in-as-of-forms-are-adopted-as-reference-citations.md

> **一、内置时点形态的对外声明以参考配置发布。** 折法仍在认得它的执行器里，参考配置不另写一份折法。首份的键是 `parcel-shipment/as-of-semantics/submission-receipt`，版本 1，作为 ADR-0147 越权风险点 3 的 owner 定键。前缀取 `parcel-shipment`：折法与执行器在那里。

> ## 越权风险点

> 1. 把「采用」扩到形态选择格。归 PC owner 复核，与 ADR-0146 越权风险点 4 同类。
> 2. 标识前缀归 parcel-shipment 而非 party-commercial。归 PS、PC owner 复核。
> 3. 汇率格可以选用，计价侧没有执行器。归 PP owner 复核。

> - [ADR-0147](./0147-reference-configuration-ships-embedded-and-is-adopted-through-ordinary-registration.md)：引用串形状与越权风险点 3；本记录把采用扩到时点语义格
> - 票 [`product-strategy-boundary/06`](../../.scratch/product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md)、[`23`](../../.scratch/product-strategy-boundary/issues/23-pc-as-of-semantics-…

### 0158-settlement-account-register-is-an-immutable-tuple.md

> ## 越权风险点

### 0159-label-channel-resolution-comes-from-the-accepted-decision.md

> ## 越权风险点

### 0160-settlement-read-ports-have-registers.md

> ## 越权风险点

### 0161-claim-and-recovery-amounts-use-one-grammar.md

> ## 越权风险点

### 0162-cost-allocation-forms-are-weight-piece-and-revenue.md

> ## 越权风险点

### 0163-supplier-audit-escalation-compares-the-matched-amount-to-a-ceiling.md

> ## 越权风险点

### 0164-buy-evaluation-request-trigger-is-occurrence-formed.md

> ## 越权风险点

### 0165-periodic-fees-are-minimum-spend-volume-floor-and-tiered-rebate.md

> ## 越权风险点

### 0166-accounting-exchange-uses-a-canonical-document.md

> ## 越权风险点

### 0168-charge-attribution-date-is-a-registered-form.md

> ## 越权风险点

### 0169-sell-evaluation-forms-a-customer-charge.md

> ## 越权风险点

### 0170-settlement-orchestrations-enter-through-parcel-api.md

> ## 越权风险点

### 0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md

> ## 越权风险点

### 0172-route-freeze-is-remaining-segment-count.md

> ## 越权风险点

### 0173-auto-reroute-is-folded-from-the-strategy-version.md

> ## 越权风险点

### 0174-loading-concurrency-is-judged-by-business-time.md

> ## 越权风险点

### 0175-service-calendar-content-folds-absolute-time.md

> ## 越权风险点

### 0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md（已有 owner 复核记录）

> Status: Accepted（2026-10-12 20:4x，用户经 IDP 队列授权通道 1 代裁接受，原话「你是业务和系统专家，你来做这个评估吧」；接受时改了决定二、决定六各一处，补了 ADR-0048、ADR 索引与 PP `CONTEXT` 的指针，接受方的读取边界与逐条理由见文末「接受记录」。起草：2026-10-12，通道 3 随票 `product-strategy-boundary/06` 第 2 项分诊，派单 `…
> Date: 2026-10-12

> - **首版就内置加成、按件定额等形态。** 不取：没有形态证据，加了就是替租户猜（ADR-0146 越权风险点 1）。它们是同一扇门的后继形态，加时不改本记录。
> - **用预估客户费用作估价。** 受理前不存在，见 Context 末条。

> ## 越权风险点

> 越权风险点逐条：

> 4. 认可首版。登记册照 ADR-0158 不设修订、键只到结算账户：同一账户下按产品分卡今天只能分账户或分计价范围，给一户换形态或换计价范围也没有路。首个要按产品分卡或要换范围的租户出现时，另立记录给键加维度、给出停用与接续；首版没有这类形态证据，现在加就是替租户猜（ADR-0146 越权风险点 1）。
> 5. 认可。

### 0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md

> Status: Accepted（2026-10-13 22:5x，用户经 IDP 队列授权通道 3 自决，原话「你是系统和业务专家，你来自决吧」；接受时不改决定原文，接受方的读取边界与逐条理由见文末「接受记录」。此前 Proposed——**草案**，2026-10-13 起草：通道 2 随票 [operator-channel/19](../../.scratch/operator-channel/issues/19-admissi…
> Date: 2026-10-13

> - **按对象分，Intake 把对象交给 `Admits`。** 这要改 14 的接口，票 19「不做」已排除。本记录不否决它，只不在此做，见越权风险点 2。

> **二、键是（租户、能力、事实类型），任一时刻至多对一个对象范围。** 同键的生效区间重叠在登记时拒，与 parcel-shipment 生产归属适配器读治理区间时同一判据（其头注「同维两条区间本身是记录错误」）。对照没登，或登了而那个对象范围上此刻没有本产品的区间，都答不在准入范围（ADR-0149 越权风险点 3）；对照读不动答依赖故障，不折成不在准入范围（14 的分格）。

> ## 越权风险点

> 用户经 IDP 队列授权通道 3 自决，原话「你是系统和业务专家，你来自决吧」。接受方与接手复核引证的是同一通道，读过：本记录全文；票 operator-channel/19 全文；ADR-0100 Context 里信任锚那一句与决定二；ADR-0149 决定四、候选丁与越权风险点 3；ADR-0083 决定一至三；ADR-0150 决定一至三；ADR-0146 决定二；ADR-0151 决定一、三；票 14 判断项 1、2、4，票 …

> 越权风险点逐条：

> 2. 认可首版。粒度约束写进登记指引与 `parcel-access-register` 登记子命令的帮助文本（随票 19 做什么第 1 件）。按对象分要改 14 的接口，等首个按线路或客户分批上线、且一行对照装不下的租户出现时另立；首版没有这类形态证据，现在加就是替租户猜（ADR-0146 越权风险点 1）。
> 3. 认可。错开的那一段朝拦不朝放；演示种子两本册同一笔登（票 19 做什么第 3 件）。
> 4. 照原文保留，不在本记录裁。暂停接不接进准入范围是 PN-08 owner 的事，本记录只照 ADR-0149 决定四第二条读区间；这一格不挡票 19。
> 5. 认可。ADR-0150 决定三列为「一字不变」的是缺省朝拦、`SYN-` 前缀门禁、读写两个开关与放行出声，这一条一样不碰，只拒一种自相矛盾的装配，与读写两开关取值不一即拒同形。
> 6. 认可。
> 7. 认可。能力词一处定义可拆成票 19 里先行的一小笔，派单方定。
> 8. 认可，以实施时核对为准：实施方先读 `access_identity` `0002`、`0004` 与 `parcel-access-register` 的授予、撤销子命令，再定对照册的列与子命令。

> - [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：决定四第二条与越权风险点 3，本记录补出其中「租户」一维的落点
> - [ADR-0083](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)：决定一、三，治理登记册无租户维；本记录不动它
> - [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：决定二的分界检验，决定四据以归类
> - [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：决定一、二、三，演示租户经普通登记越过准入范围、隔离形态过渡期规则不变
> - [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：决定二与 Context 信任锚那一句，部署形态参数与租户参数的分界
> - [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)：决定三，运营决定口要判准入范围
> - 票 [operator-channel/14](../../.scratch/operator-channel/issues/14-admission-scope-read-and-grade.md)：只读口与答复格，判断项 1、2、4
> - 票 [operator-channel/19](../../.scratch/operator-channel/issues/19-admission-scope-bridge-for-operator-and-integration-client-minters.md)：本记录承接的分诊票

### 0181-posted-balance-is-recomputed-in-the-funds-mapping-transaction.md

> ## 越权风险点

> 越权风险点逐条：

### 0182-receivable-account-is-a-funds-mapping-target.md

> ## 越权风险点

### 0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md（已有 owner 复核记录）

> Status: Accepted（2026-10-16，用户原话「你作为业务和系统超级专家，你自己全部独立完成吧」。Claude Code 会话据此代 PS owner 裁定下面六条，落在票 [operator-channel/15](../../.scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md) 做什么第 3 条那一…

> ## 越权风险点

> 用户令「针对上面的问题，你能自决，全部解决吗，符合我们系统业务特点，和实际业务的需求」，就越权风险点逐条复核：

> - [ADR-0151：未归族的命令口各归其族](./0151-unassigned-command-faces-get-their-families.md)：本记录补上它决定二与越权风险点 4 留给 PS owner 的那一格，不改它任何一条
> - [ADR-0146：产品策略是机制与租户取值之间的第三类](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：本记录裁的是产品策略
> - [ADR-0150：合成租户当真实租户对待，隔离形态逐口退场](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：两口今天没有隔离放行，换口不涉及撤放行

### 0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md（已有 owner 复核记录）

> ## 越权风险点

> - owner 复核 2026-10-19 认可（用户 2026-10-16 授权「你作为业务和系统专家你来自决」，会话 idp-parcel-9b 代裁，逐条）：1. 接受时形成——PS CONTEXT「外部标识关联只在被标识对象成为正式对象时形成」，预收寄要先改这句、再补已拒绝与已撤回两格，不取；原句「接受之前送站答待识别」的前提由票 ps-external-mark-relations/04 更正，号只在别处在册时的唯一误关联由票…

> 2. **越权风险点 4 的事实更正**（出处同上，非作者会话对票 02 的评审）：风险点 4 写「同一委托内两件包裹给同一个号不拒收」，那只对领域成立。今天唯一的提交入口 `IsolatedSubmissionIntake` 以客户包裹参考作载荷摘要里的成员键，`CanonicalizeSubmissionPayload` 要求成员不重复，所以同一草案两件同号在规范化那一步就答 4xx，到不了领域。这是 Intake 草案形状的既有纪…

### 0185-external-identifier-relation-identity-includes-its-formation-basis.md（已有 owner 复核记录）

> ## 越权风险点

### 0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md

> Status: Accepted（2026-10-18，Claude Code 会话代决：用户原话「你作为业务和系统专家自决。解决所有的问题」；票 [routing-first-cut/18](../../.scratch/routing-first-cut/issues/18-leg-endpoints-folded-into-price-card-zones.md) 的产物。补 [ADR-0148](./0148-route-evi…

> ## 越权风险点

> - [ADR-0148：路由证据按来源分三路，候选成本由逐段成本依据合成](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md)：本记录补它决定四第 7 条，接手越权风险点 5
> - [ADR-0109：邮编分类事实归 parcel-pricing，作为版本化的计价参考目录登记](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md)：决定四「分区给法逐卡定」
> - [ADR-0152：运营试算是计价自有的应用用例](./0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md)：决定三与决定五，估价与正式评价同一份读数补全
> - [ADR-0179：受理前控制金额由结算按逐件 SELL 试算合计形成](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md)：决定四
> - [ADR-0075：客户地址随判断请求在请求期携带过界](./0075-customer-address-is-carried-with-the-routing-request.md)：邮编路线的来处
> - [ADR-0147：参考配置随产品版本嵌入发布](./0147-reference-configuration-ships-embedded-and-is-adopted-through-ordinary-registration.md)：决定四，采用行与依据格的引用串
> - 票：[routing-first-cut/18](../../.scratch/routing-first-cut/issues/18-leg-endpoints-folded-into-price-card-zones.md)（本记录）、[routing-first-cut/20](../../.scratch/routing-first-cut/issues/20-bound-cost-cards-fold-zones-thro…

### 0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md

> Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，其后又令「继续」，按会话里报告过的次序先做客户费用。取代 [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) Consequences「形成编排不进 `cmd/parcel-dispatch`」与决定四「也不在本记录里把形…

> ## 越权风险点

（共 61 篇 ADR 含越权风险点）

## 二、needs-info / blocked 的票

- `.scratch/no-directional-basis/issues/02-network-routing-has-no-routing-instruction-or-consolidation-compatibility-judgment.md`
  Status: needs-info——2026-10-16 排期已由 Claude Code 会话代决（用户授权，见 01「排期代决」）：随一线作业端，在演示动线走通之后排。此前 needs-info——2026-10-16 随 [01](./01-consolidation-adds-members-without-any-directional-basis.md) 的裁定修订转：本票唯一的消…
- `.scratch/syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md`
  Status: needs-info——2026-09-24 按 [product-strategy-boundary/02](../../product-strategy-boundary/issues/02-split-parameter-register-and-retriage-deferrals.md)「票：未 resolved 的」重新定性：不再等候选租户的渠道证据。操作者一半是机制，…

（共 2 张）

## 三、票面里提到越权风险点的票

- `.scratch/admin-web-group-legal-entities/issues/04-catalogue-read-pagination-sort-filter-contract.md`（1 处）—   越权风险点见 ADR 文末。剩逐册实施票待拆。
- `.scratch/admin-write-faces/issues/08-publication-form-path-common-half-server-side-digest-and-envelope-approval.md`（3 处）— Status: resolved——2026-09-08，MCP-1 接 MCP-5 `mcp5-awf08@c003c845` 两笔续做完（用户 12:4x 指示通道 1 自办、不派工；分支 `mcp1-awf08` 基 main `5aedbb0f`，逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SH…
- `.scratch/catalogue-read-pagination/issues/02-customer-accounts-read-adopts-adr-0144.md`（1 处）— 1. 0035 在本读面上用不上：`customerAccountListed` 的 account 段先按 `DISTINCT ON (account_id) … ORDER BY account_id, revision DESC` 折叠到最新修订，外层 `ORDER BY recorded_at DESC, ac…
- `.scratch/catalogue-read-pagination/issues/03-network-catalog-read-adopts-adr-0144.md`（4 处）— 2. **缺省序要本票裁一次**：网络版本册常按对象看版本，`-registeredAt` 未必顺手（ADR-0144 越权风险点 1）。若某个 `family` 另点缺省序，在读面头注写明
- `.scratch/customer-service-rule-contract-applicability/issues/02-pc-closure-resolves-contract-tier-service-rule-first.md`（3 处）— - **同一合同两版（Q4）**：只在闭包答`适用冲突`，发布面不加门——ADR-0176 没裁发布门，Alternatives 否的是保存面禁止产品版与合同版并存。spec 用户故事 4 的「并拒绝」读作解析时不采纳，不是发布时拒收。**越权风险点 · 待 PC owner 复核。** **owner 复核 2026…
- `.scratch/customer-service-rule-contract-applicability/issues/03-pc-layered-read-port-returns-contract-and-product-base-bodies.md`（3 处）— 2. **读口多带 `declaredProduct`（可缺席），取自冻结闭包键 `ResolutionKey().ServiceProduct`**（02 评审 ← 通道 3 Spec ②）。ADR-0176 决定二、spec 与本票写的读口键是（租户、范围、合同版本、锚点）。不带这一维，同范围两个产品各有一版时底座…
- `.scratch/customer-service-rule-contract-applicability/issues/04-ve-claim-eligibility-checks-contract-and-inherits-rows-from-product-base.md`（2 处）— 3. **点核施于每一份挂合同的正文，含产品层那一版，挂产品的正文不核。** 票面「做什么 3」写「挂产品版时照旧走单版点读，不核」，ADR-0176 决定四写「挂产品的版本不核（查询无产品维）」——两句说的都是正文挂产品；壳空而正文挂合同的那一版正文挂的是合同，02 判断项 1 已写明它回落给别的合同时「由 04 的…
- `.scratch/customer-service-rule-contract-applicability/issues/05-admin-form-contract-tier-empty-means-inherit-and-tighten-only.md`（2 处）— - **只收紧怎么守（Q3）**：本拆法只落 ADR-0176 决定三「不提供删减表达」那一半，不设「合同版行比底座更严」的比对门——底座是读时按锚点选的，起算事件或日历不同的两条期限行也比不出严宽。spec 用户故事 3 据此读作「不能借空行放宽成删减」；收紧是商务纪律，由本票的提示承担。**越权风险点 · 待 PC…
- `.scratch/customer-service-rule-contract-applicability/issues/07-closure-does-not-narrow-customer-contract-by-customer-account.md`（1 处）— 5. **对 ADR-0014 的部分停用补记进 ADR-0177 决定三**：实施时发现钉串用例写着「串变了就是换号」，出自 ADR-0014 Consequences 经 ADR-0126 继承；ADR-0177 起草时只点了 ADR-0176 决定五的 omitempty 先例，漏了这一条。补记照 README「…
- `.scratch/demo-chain-gaps/issues/01-transport-opportunity-has-no-production-entry.md`（1 处）— 运输履约是自动消费初始路由形成机会，还是只由运营准备请求触发（UC-TF-003 两种触发都列了）；路由指令怎样把段引用带到运输履约（ADR-0131 越权点 4）；班次模板与日历归运输履约登记，还是随网络参考配置发布（产品策略还是租户取值）。
- `.scratch/label-channel-service-first-release/issues/24-source-correction-version-refused-as-second-responsibility-start.md`（2 处）— **能力边界**：读了 UC-PS-003 全文、PS CONTEXT 责任起点与更正各句、`adopt_network_intake.go` / `intake_adoption.go` / 迁移 0004 / `network_intake.go` / `pickup_source.go` / `adopt_on_…
- `.scratch/label-channel-service-first-release/issues/25-external-carrier-first-pickup-triggers-label-final-judgment.md`（5 处）— 2. **失效版本到达后，已据前版形成的非取消终局怎么重派生**（ADR-0135 决定六 / 越权风险点 4）。TF 会发失效版本（依据被源更正为不再表达收寄）；`LabelServiceFinalOutcome` 五值之外这是第六格。归 PS owner。**不阻开工**：实施时该格先按 AGENTS.md 红线「…
- `.scratch/label-channel-service-first-release/issues/26-label-transaction-settlement-beat-triggers-label-final-judgment.md`（3 处）— - **谁 / 何时 / 口径**：通道 2，2026-09-10，按通道 1 派单 task-138ab1c9；用户 17:0x 经队列授权「你自决」，B 类按 **PS owner 口径**代裁——硬句不改、拿不准的单列越权风险点供 owner 事后复核。正文与越权风险点在 [ADR-0134](../../../…
- `.scratch/label-channel-service-first-release/issues/28-channel-selection-composition-root-and-call-entry.md`（2 处）— **越权风险点（供 owner 复核）**：是否需要运营人工择优（甲），属产品 owner；本裁决只定「首发不接甲、且形状上不堵甲」。
- `.scratch/label-channel-service-first-release/issues/29-channel-selection-result-to-label-transaction-basis-translation.md`（5 处）— Status: resolved——2026-09-10 19:34 通道 1 推送方重放进 main（非作者评审 ← 通道 6 两轴 0 阻断；main 上 SHA 与分支 SHA 对照见 Comments「进 main 记录」）。此前 resolved——2026-09-10 19:3x 通道 3 接管单（task…
- `.scratch/label-channel-service-first-release/issues/30-controlled-close-reopen-decision-write-face-ps-half.md`（1 处）— - **22:0x 三条当场裁决（2026-09-10 通道 3 报，推送方当场裁，用户授权代裁；原句在 main `e8fe7bf9` `.scratch/tasks.md`「lc/30 要裁的」条，2026-09-11 通道 4 按评审阻断回修时照抄补入——评审 grep 零命中，票面此前漏记）**：① 地盘随做法…
- `.scratch/label-channel-service-first-release/issues/31-carrier-first-effective-pickup-fact-registry-and-handoff.md`（2 处）— 无。ADR-0135 六条越权风险点归 owner 事后复核，不阻塞本票开工；复核若改动决定三（时间取源发生时间）或决定五（段引用由 TF 铸），改的是本票做法 1 / 3 各一格。
- `.scratch/label-channel-service-first-release/issues/35-establish-replay-decision-register-reconciliation-and-select-result-shape.md`（1 处）— 3. **越权风险点**：多一次读（每次建立多一问交易册）——今天没有量化过这条路的读写比，本记录认下这个代价；若将来有证据说它是热点，乙路的引用可以反过来省掉这一问，届时另议。
- `.scratch/label-channel-service-first-release/issues/37-lc25-review-follow-ups-superseding-version-case-header-counts-and-exported-event-type.md`（2 处）— 4. **通道 6 Spec 非阻断 (4) / ADR-0135 越权风险点 5 / 票 25「随本票或另笔」**——`docs/application/parcel-shipment/UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md` 依据表「面单渠道服务非取消终局结果」…
- `.scratch/label-channel-service-first-release/issues/39-voided-carrier-pickup-rederivation-needs-an-adr-before-code.md`（13 处）— Status: 已进 main——2026-09-14 19:3x 通道 1 推送方：`mcp3-lc39@4c6c4e86` 七笔重放到 main `09596d9a` 之上，代码 tip `bb5a6297`（分支→main SHA 对照与验证见 Comments 末「进 main 记录」；纯 .md，`go bu…
- `.scratch/legal-entity-profile/issues/01-registration-number-type-catalogue.md`（1 处）— 3. 所属层钉在类型上：新修订改层不受理，改层即登记新类型代码。ADR-0145 越权风险点 1 说的「在目录上把那类号标为资料层」，本实现
- `.scratch/operator-channel/issues/05-catalogue-read-faces-take-operator-intake.md`（2 处）— ## 越权风险点
- `.scratch/operator-channel/issues/06-commercial-publication-and-replay-take-operator-identity.md`（3 处）— ### 判断项（越权风险点 · 待 owner 复核）
- `.scratch/operator-channel/issues/10-device-register-and-operation-fact-capability-face.md`（14 处）— **越权风险点**
- `.scratch/operator-channel/issues/12-signed-webhook-inbound.md`（3 处）— 4. **签名原文与两形取 Standard Webhooks。** `webhook-id.webhook-timestamp.请求体`；`v1` 为 HMAC-SHA256，`v1a` 为 Ed25519。认不出的签名标识跳过，一项都不剩才答签名不过。见越权风险点 1。
- `.scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md`（5 处）— - 角色模型与授权请求坐标的推导（[psb/07](../../product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)）；party-commercial 授权动作词表里的「授权处置」一格（ADR-0132…
- `.scratch/operator-channel/issues/18-commercial-publications-takes-operator-intake-after-approval-translation-sinks.md`（2 处）— ### 判断项（越权风险点 · 待 owner 复核）
- `.scratch/operator-channel/issues/19-admission-scope-bridge-for-operator-and-integration-client-minters.md`（9 处）— 出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四第二条、越权风险点 3；[ADR-0151](../../..…
- `.scratch/operator-channel/issues/20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md`（2 处）— 出处：[ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 决定三（逐口核载荷规范化，未核完的口不开真渠道）；[ADR-0149](../../../docs/adr/0149-business-command…
- `.scratch/operator-channel/issues/21-work-device-catalogue-page.md`（6 处）— 出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定二与越权风险点 2；[10](./10-device-regis…
- `.scratch/operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md`（1 处）— | 0 | [ps-external-mark-relations/02](../../ps-external-mark-relations/issues/02-register-and-customer-parcel-numbers-feed-node-identity.md) · [ADR-0184](../../…
- `.scratch/operator-channel/issues/32-tf-correction-payloads-pass-absent-instants-to-orchestration.md`（2 处）— ## 裁定（2026-10-16，会话 `idp-parcel-9b` 代决；机制，不是租户取值；代决 · 越权风险点 · 待 TF owner 复核）
- `.scratch/operator-channel/issues/38-controlled-closure-and-reopening-record-request-and-operation-evidence.md`（5 处）— - [label-channel/30](../../label-channel-service-first-release/issues/30-controlled-close-reopen-decision-write-face-ps-half.md)「裁决」22:0x ② 的越权风险点「重开决定应否保存货主新授权…
- `.scratch/operator-channel/issues/51-owner-review-of-adr-0183-0184-0185-overreach-points.md`（5 处）— # 51 ADR-0183、0184、0185 越权风险点的 owner 复核（代决）
- `.scratch/operator-workspace-gaps/issues/02-estimate-evaluation-entry.md`（1 处）— 越权风险点四条列在 ADR「裁决方的能力边界」，待 owner 复核。实现拆票：[05](./05-estimate-endpoint-backend.md)（后端）、[06](./06-estimate-admin-page.md)（管理台页，阻塞于 05）。
- `.scratch/party-commercial-context-gaps/issues/08-authorized-action-lacks-source-data-amendment-and-adjudication-names-no-decider.md`（4 处）— Status: resolved——2026-09-07，MCP-5（task-0d116e60，分支 `mcp5-pcgaps08` 基 `299f2a2e`）：代码半边四笔由 MCP-1 重放进 main（`1e182860`→`0cfe3571` / `6b43484c`→`31903cc5` / `df923d…
- `.scratch/party-commercial-context-gaps/issues/09-final-rule-content-has-no-validity-declaration.md`（3 处）— Status: resolved——2026-09-07，MCP-6（task-390c4f53，分支 `mcp6-pcgaps09` 基 `250e5a43`）：ADR-0119 六问一次答完（越权风险点三条单列在 ADR 里供 owner 复核）、PC CONTEXT「面单服务终局规则」词条补段、迁移 0026、领…
- `.scratch/party-commercial-context-gaps/issues/10-source-data-amendment-allowance-is-a-third-stage-content-family.md`（2 处）— Status: resolved——2026-09-07，MCP-6（task-390c4f53，分支 `mcp6-pcgaps10` 基 `9379c716`）：ADR-0120 七问一次答完（越权风险点四条单列在 ADR 里供 owner 复核）、PC CONTEXT 新词条 + Boundaries 一句、迁移 …
- `.scratch/party-commercial-context-gaps/issues/11-delivery-condition-declaration-family-and-resolution-keyed-read-face.md`（4 处）— Status: resolved——2026-09-10 13:0x 通道 5 收口（单 task-4649070b-bec3-433b-ba5c-58fb8b4ac4b7，改派自通道 4；分支 `mcp5-pcgaps11` 基 `d9a1f571`，每笔已推 origin；完成记录见文末，进 main 记录归推送方…
- `.scratch/party-commercial-context-gaps/issues/12-declared-responsibility-outcome-lacks-label-channel-rows.md`（1 处）— 1. **两行的原词取 `LABEL_SERVICE_COMPLETED`（非取消终局结果）与 `LABEL_SERVICE_FAILED`（终局失败结果）**，Go 常量 `DeclaredLabelServiceCompleted` / `DeclaredLabelServiceFailed`。照既有四值的构词（`…
- `.scratch/party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md`（8 处）— Status: resolved——2026-09-10 20:3x（本机时钟）通道 1 推送方重放进 main（非作者评审：两个隔离子代理鉴权错死掉，改由推送方按 /code-review 两轴串行自评，0 阻断；main 上 SHA 对照见 Comments「进 main 记录」）。此前 resolved——202…
- `.scratch/pp-pricing-input-seams/issues/03-ps-origin-destination-postal-route-read-port.md`（1 处）— 7. **（2026-09-15 10:4x 通道 1 追裁 ← 通道 5 量得条目只进摘要）裁决 1 的前提不成立，取 A：口成形、内容落库归 [05](05-ps-submission-and-source-data-versions-carry-content.md)。** 作者开工对 `3a21dab7` 量到…
- `.scratch/price-card-import/issues/04-approval-and-publication.md`（4 处）— 判断项（标「越权风险点 · 待 PP owner 复核」的是照先例自裁、票面没写定的格）：
- `.scratch/price-card-import/issues/07-parcel-pricing-context-gains-approval-duty-rule-and-operator-subject-terms.md`（1 处）— - 04 判断项里标「越权风险点 · 待 PP owner 复核」的几格（授予集的词汇与来源、状态不对分三格、发布的答案代数）要不要等 owner 复核后再落词条。
- `.scratch/product-strategy-boundary/issues/02-split-parameter-register-and-retriage-deferrals.md`（1 处）— 4. **公开标准只归类、不定发布范围。** 票 10、14 与各连接器的参考配置随附哪些国家、程序与承运商，照 ADR-0146 越权风险点 5 归各 owner 与用户。
- `.scratch/product-strategy-boundary/issues/03-reference-configuration-adoption-pattern.md`（2 处）— 出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定三、越权风险点 4、5；[ADR-0145](../../../docs/adr/0145-l…
- `.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md`（1 处）— 出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定五第四条、越权风险点 2。
- `.scratch/product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md`（5 处）— **等用户**：是否接受 ADR-0179，要紧的是决定一（估价归结算）与决定五（采用登记的键是结算账户）。拿不准的列在那篇「越权风险点」。
- `.scratch/product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md`（1 处）— - 2026-09-25 · 通道 2（随 [ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 复核）：第 2 项的动作清单还缺「授权处置」。party-commercial 的授权动作封闭集（`Author…
- `.scratch/product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md`（1 处）— 首发随附哪些程序与地区的参考配置，归 CC owner 与用户定（与 ADR-0146 越权风险点 5 同一口径）；本票只立形态并带一份样板。
- `.scratch/product-strategy-boundary/issues/12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md`（2 处）— - **非阻断**（随票记）：执行器没有接进截单或确认费用。ADR-0165 越权风险点 3 写明如此。
- `.scratch/product-strategy-boundary/issues/14-pp-postal-prefix-granularity-and-public-unit-reference-configuration.md`（1 处）— 首发随附哪些国家的格式，归 PP owner 与用户定（与 ADR-0146 越权风险点 5 同一口径）。
- `.scratch/product-strategy-boundary/issues/16-mechanism-gaps-without-a-ticket.md`（4 处）— - **非阻断**：ADR-0156 Status 的授权依据查不到出处——通道规则里「继续」是「检查队列」，不是接受 ADR；本项属机制，按 AGENTS 红线开发方本可自决，应改援引这条，并补裁决能力边界与越权风险点一节。解析标识不进 `SameJudgmentScope`，而 UC-NR-002 要「新……商业版…
- `.scratch/product-strategy-boundary/issues/21-sell-evaluation-to-customer-charge.md`（1 处）— 2. SELL 评价形成的是预估客户费用，即三段里的第一段，依据是 ADR-0169 决定二；ADR-0190 沿用它，预估到暂估的转移不在那一份里（越权风险点）。
- `.scratch/product-strategy-boundary/issues/24-plan-given-estimate-evaluations-use-shared-reading-completion.md`（1 处）— ### 判断项（越权风险点 · 待 owner 复核）
- `.scratch/product-strategy-boundary/issues/25-pre-acceptance-estimate-formed-by-settlement.md`（4 处）— ### 判断项与越权风险点
- `.scratch/product-strategy-boundary/issues/27-pre-acceptance-control-reads-only-the-current-submission-version.md`（6 处）— Status: resolved——2026-10-13 21:2x 通道 1：本票七笔纯快进进 main（至 `c5dd5b64`，SHA 与分支 `mcp1-psb27` 相同），进 main 记录与非作者评审见文末；判断项 1、2 两条越权风险点待 PS owner 复核。此前完工，待评审与重放——2026-10…
- `.scratch/product-strategy-boundary/issues/28-psb25-review-tails-estimate-gaps-unit-folding-and-trace-as-of.md`（6 处）— 1. 停格分清：溢出与单份为负各给一格，领域停格、SA 原因、PS 映射三处同笔，PS 侧仍落`未形成`。按决定二，单份为负而合计为正不在停法里；若留现状照停，写判断项并标越权风险点。
- `.scratch/product-strategy-boundary/issues/32-demo-rule-package-declares-no-unprovable-intake-qualification.md`（2 处）— 越权风险点：
- `.scratch/ps-external-mark-relations/issues/02-register-and-customer-parcel-numbers-feed-node-identity.md`（2 处）—     (1) `internal/parcelshipment/adapters/http/isolated_write_intake.go` · `customerParcelNumbers`：号码取客户包裹参考，连带既有纪律——去两端空白后才进册键；同一委托两件同号，在摘要规范化那一步就答 4xx。端到端看，与 …
- `.scratch/ps-external-mark-relations/issues/04-ticket-02-review-follow-ups-records-and-pinned-behavior.md`（11 处）— ADR-0184、ADR-0185 越权风险点的 owner 复核行不在本票（归 operator-channel/51）；本票只写事实更正与补记条目。
- `.scratch/ps-external-mark-relations/issues/05-a-unique-hit-in-someone-elses-number-space-is-not-enough.md`（1 处）— 取 (a) 作现在的产品策略：核对时，号码同时被租户内一份未终局、未接受的委托声明，唯一命中不再答唯一，改答身份冲突、走待识别。声明不进候选，只作歧义信号；这要先改 PS CONTEXT「接受前的声明包裹不是正式包裹，它的号码不进身份核对」那句的读法，并出 ADR（用号 0195，会话已占），写明与 ADR-0184 …
- `.scratch/ps-port-remainder/issues/01-label-validity-rule-is-a-lapse-declaration-on-the-final-rule.md`（1 处）— - 2026-09-10 14:2x · 通道 1（解阻簿记，未动代码；取证于远端 main `5dda0fb2`）：**PC 半边在 main 上了**，PS 半边转 ready-for-agent。对号本票裁决①的 PC 半边三件：有效期声明一格——`final_rule_content` 加 `validity_…
- `.scratch/ps-port-remainder/issues/05-customs-and-node-operations-need-parcel-keyed-stage-fact-read-faces.md`（1 处）—   - **不取 ADR-0124、不改 CC/NO CONTEXT**：两面读法都是既有生命周期句与硬句的直接读法（重开、替代、候选不得当归属），形状可逆、无新词条；跨上下文归属已由 ADR-0118 决定三/四定下。预留号 0124 未消费，请 MCP-1 收回。越权风险点两条，单列：(1) CC「已关闭」把重开读…
- `.scratch/ps-port-remainder/issues/06-delivery-place-reference-read-face.md`（3 处）— 3. **包裹 → 委托的路**：经 `AcceptanceBaseline.covers` 走声明成员；包裹身份谱系（拆分 / 合并后的新包裹）今天领域里没有模型，本票先只按声明成员解析，谱系包裹会落「没有收件地点」——在读口头注写明这一格是「谱系未建模」的今日形状，谱系落地那票要补这一路（同 ps-port-rem…
- `.scratch/ps-port-remainder/issues/07-commercial-resolution-reference-by-parcel-read-face.md`（2 处）— - 2026-09-09 · 通道 2（task-f5521768）：立票（ready-for-agent）。形状在 ADR-0133，本票不再裁；ADR-0133 越权风险点 5（谱系包裹与集运单元同答没有）若 owner 复核后改口径，本票随之改那一格，不回改 ADR。
- `.scratch/ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md`（4 处）— - [ADR-0136](../../../docs/adr/0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md) 决定三：`CustomerServiceRuleObject` 必须在接受时闭…
- `.scratch/routing-first-cut/issues/02-route-evidence-sourcing-and-candidate-cost-adr.md`（3 处）— - [x] ADR Accepted，上列各问各有决定；越权风险点写明碰到的邻接上下文（PS、PP、CC）由谁复核。
- `.scratch/routing-first-cut/issues/09-initial-route-evidence-folded-from-catalog.md`（1 处）— - 三格可空，空不补 0，缺日历不补默认。成本缺席不形成计划。段的责任引用暂时指连接版本，ADR-0175 越权风险点 1，非阻断。
- `.scratch/routing-first-cut/issues/10-candidate-cost-from-leg-buy-evaluations.md`（2 处）— - **比较币种换算随本票落地。** ADR-0148 越权风险点 1 的 PP 侧延伸——换算从结算币种延伸到比较币种、逐段保留原币 / 结算币种 / 所引汇率序列版本与比较币种金额——由本票实现；口径仍由价格政策声明、比较币种仍是路由策略版本上的租户取值。「与 PP 主人约」落地为这次用户裁决。
- `.scratch/routing-first-cut/issues/11-demo-network-adopted-as-reference-configuration.md`（7 处）— - **采用入口照 ADR-0147 先例做。** 先例是 `cmd/parcel-commercial/register_registration_number_types.go` 的 `adopt` 项与 `registrationjson.RegistrationNumberTypeCommand`；`refer…
- `.scratch/routing-first-cut/issues/12-cc-customs-applicability-judgment-for-route-candidates.md`（2 处）— Status: resolved——2026-10-10 16:42 通道 1 重放进 main（`717959d9`…`18be66e8`，清点 `613fdb74`）；评审门当时是推送方自审、不算非作者评审，见文末 Comments 与进 main 记录；19:0x 与 19:2x 先后补得两份非作者评审（通道 3…
- `.scratch/routing-first-cut/issues/17-initial-route-pricing-input-from-customer-declaration.md`（12 处）— 出处：[ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四第 7 条与越权风险点 5；[10](10-candidate-cost-from…
- `.scratch/routing-first-cut/issues/18-leg-endpoints-folded-into-price-card-zones.md`（5 处）— # 18 段起讫折成价卡区域的折法（ADR-0148 越权风险点 5）
- `.scratch/routing-first-cut/issues/19-financial-control-undecided-leaves-no-trace-on-the-task.md`（3 处）— 3. **拆码口径。** 适配器分得开的拆出新码、留原格，只挪触发已到位的那一种（ADR-0094 决定四）。作用域停拆成两码：`CONTROL_SCOPE_NOT_CONFIGURED` 只留给账户没登记（[ADR-0158](../../../docs/adr/0158-settlement-account-reg…
- `.scratch/sa-cc-funds-and-credential-seams/issues/03-cc-inbox-consumer-receives-external-funds-fact.md`（7 处）— Status: resolved——2026-09-10 21:3x 通道 1 推送方重放进 main（非作者评审 ← 通道 6 两轴 0 阻断；main 上 SHA 与分支 SHA 对照见 Comments「进 main 记录」）。此前 resolved——2026-09-10 22:1x（作者自标，git 提交时刻…
- `.scratch/sa-cc-funds-and-credential-seams/issues/04-cc-credential-gate-persists-in-readiness-assessment.md`（4 处）— （用户 17:0x 授权、通道 5 按 CC owner 口径代裁并落 [ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rul…
- `.scratch/sa-cc-funds-and-credential-seams/issues/06-cc-release-gate-reads-duty-payment-verification.md`（1 处）— （1 由用户 17:0x 授权、通道 5 按 CC owner 口径代裁并落 [ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-…
- `.scratch/sa-cc-funds-and-credential-seams/issues/07-cc-credential-and-duty-reconciliation-registration-faces.md`（3 处）— （通道 5 写入，2026-09-10 17:2x；task-b941ce87 分类：1 为 C（拿不准，或 B）、2 为 B、3 为 A；task-9a2ff746 落笔。1 由用户 17:0x 授权按「机制半边现在做」口径定，越权点 CC owner 复核；2 落 [ADR-0137](../../../docs/…
- `.scratch/sa-cc-funds-and-credential-seams/issues/08-sa-evaluation-request-orchestration-records-source-references.md`（2 处）— - **1 → 信封，不同步调用。** 理由：① PP→SA 那条缝（票 01 `parcel-pricing.evaluation.recorded` → SA inbox）已是信封，反向同形——一条缝两个方向一种机制；② 仓内跨上下文写向交接的纪律是「意图与登记同事务入队」（`FinalOutcomeHandoff…
- `.scratch/sa-cc-funds-and-credential-seams/issues/11-pp-inbox-consumer-receives-evaluation-request-envelope.md`（2 处）— 1. **（已裁，见「裁决」）** PP 请求评价的应用入口是什么形：PP 今天形成评价的入口是给谁调的（`git grep -n 'func New.*Evaluat' -- internal/parcelpricing/application` 开工前核）——若已有一个收「计价输入 + 计算目的」的应用入口，消费者…
- `.scratch/sa-cc-funds-and-credential-seams/issues/12-cc-funds-fact-payer-may-be-explicitly-unprovided.md`（6 处）— 3. sa-cc/03 的越权风险点 (b) 由 CC owner 在本票一并复核。
- `.scratch/sa-cc-funds-and-credential-seams/issues/20-sa-external-funds-fact-holds-one-row-per-fact-and-cannot-store-a-correction.md`（2 处）— 5. **能力边界**：裁的是结构（子表 vs 主键、入口归既有用例、链头口径）；`Save` 两步的事务边界、`ListExternalFundsFacts` 的列形、迁移搬列的细节归作者。读过：本票全文、通道 5 取证条、ADR-0137 决定四与越权风险点 6（经取证引文）、CC `0021` 头注（经引文）；*…
- `.scratch/sa-cc-funds-and-credential-seams/issues/27-sa-external-funds-fact-adoption-and-correction-registration-face.md`（4 处）— 8. **能力边界**：裁的是族别、步序、装配纪律与归格原则；具体不变式（`transactionalFundsRegistration` 对两条方法各一个包装还是一个、`registrationjson` 的字段名与 CC 那套是否同名、`FundsHandoffReference` 打印格式）归作者按代码定并写进判…
- `.scratch/sa-preacceptance-policy-view/issues/02-load-control-policy-reads-policy-content-items.md`（1 处）— **越权风险点（供用户复核，不认可走 supersede）**：① 决定二「第一处限制即停后续项」是执行顺序规则，本会话判它不构成「汇总成接受/拒绝
- `.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md`（3 处）— 2026-09-07，通道 3（task-0ec68b23），用户经 IDP 队列授权「owner 自决」口径：硬句一字不改、每个决定写理由、拿不准或跨上下文归属的点单列越权风险点。三问的答落在 [ADR-0125](../../../docs/adr/0125-parcel-shipment-financial-co…
- `.scratch/sa-preacceptance-policy-view/issues/04-authorized-disposition-flow-and-failure-disposition-read.md`（4 处）— **越权风险点**（供 owner 复核，都不阻断开工；全文见 ADR-0132 Consequences）：1. `ResumePath` 加格与 CONTEXT / UC-PS-001 / ADR-0094 里以数目指称等待态的措辞要跟改（UC 归本票实施，ADR-0094 正文不改）；2. 处置`拒绝`由处置授权…
- `.scratch/sell-customer-charge/issues/05-demo-tenant-forms-an-estimated-customer-charge.md`（2 处）— - `SYN-SETTLEMENT-ACCOUNT-01` 采用 `NETWORK_INTAKE_ADOPTED`，计价范围与受理前估价采用册登的那一个相同（ADR-0190 越权风险点 9）。
- `.scratch/tf-carrier-master-document-register/issues/01-carrier-master-document-register-does-not-exist.md`（3 处）— Status: resolved（MCP-5 实施完成，2026-09-07，隔离分支 `mcp5-tf-cmdr`，基线 main `92579b0a`，task-895fbabf；形状取票面第 1 条「独立登记册」，落文 [ADR-0113](../../../docs/adr/0113-carrier-maste…
- `.scratch/tf-face-algebra/issues/02-load-assignment-versions-form-a-single-chain.md`（1 处）— 越权风险点：代 TF owner 定了「本口只管首版」；owner 复核时看今天有没有调用方靠「换版本号」表达变化（演示种子、管理台、探针）。
- `.scratch/tf-face-algebra/issues/08-movement-fact-same-identity-different-content-conflicts.md`（2 处）— 越权风险点：第 2 条与研判推荐相反（研判推荐照同族把业务时间放进摘要），本会话按权威次序取已接受 ADR 的字面；TF owner 复核时看设备侧的发生时间是否真的不会在同一身份下变化。
- `.scratch/tf-face-algebra/issues/09-dispatch-trigger-task-reference-is-a-faithful-identity.md`（1 处）— - 入场依据进键（票 tf-segment-lifecycle-closure/09 越权风险点）：交接更正后会另立任务、前一项不动，按 ADR-0114 字面是新身份。
- `.scratch/tf-segment-lifecycle-closure/issues/09-arrival-triggers-dispatch-task.md`（3 处）— | ①②③ 成文 | ADR-0114（决定一至四、越权风险点四条）+ README 目录行 | `85956673` | `docs/adr/0114-*.md`、`docs/adr/README.md`（只加一行） |
- `.scratch/tf-segment-lifecycle-closure/issues/10-source-correction-rederives-participation.md`（2 处）— **能力边界**：读了 `actual_fulfillment_segment.go`、`segment_rehydration.go`、`fulfillment_segment.go`（端口）、`fulfillment_segment_registry.go`、`enter_fulfillment_segment.g…
- `.scratch/tf-segment-lifecycle-closure/issues/11-control-withdrawing-correction-voids-participation.md`（6 处）— （只确认它经领域 `CloseSegment`）。**越权风险点**：① 失效版本继承离场三件——ADR-0112 决定三写的是替代格，本票把同一条规则
- `.scratch/tf-segment-lifecycle-closure/issues/12-delivery-place-reference-seam-parcel-shipment.md`（5 处）— 1. **「收件地点引用」是什么身份。** 候选：(a) PS 为每个已接受委托版本的收件范围铸一个地点身份，引用带版本；(b) 引用就是（委托、接受基线、收件槽位）的复合指针，不另铸身份；(c) 目的地节点引用（NR 的词）——ADR-0114 越权风险点 4 已把「收件地址引用 vs 目的地节点引用」留给本票，且指…
- `.scratch/tf-segment-lifecycle-closure/issues/13-delivery-window-seam-network-routing.md`（3 处）— 越权风险点五条在 ADR-0131 末节（四态常函数 / 悬空引用同答缺失 / 序位而非节点对 / 引用怎样到登记方手里未裁 / 代裁阅读范围）。
- `.scratch/tf-segment-lifecycle-closure/issues/14-delivery-condition-reference-seam-party-commercial.md`（6 处）— **顺带裁定（决定四，归属；越权风险点 2 供 owner 复核）**：交付条件本体是服务产品版本声明、客户合同版本只能收紧的商业条件（保价条件先例），不归接单规则包版本（ADR-0058 那一族）；PC 今天没有这一族，没登就是没有——所以今天每一份都会答「没有交付条件」，那是真话。
- `.scratch/ve-claims-read-seams/issues/04-rule-resolution-key-source-needs-a-registration-face.md`（10 处）— ## 裁决（2026-09-10，通道 4 按通道 1 派单 task-d6660969；用户授权代裁，VE owner 口径，键形状连 PC owner 口径；落 ADR-0136，越权风险点五条在 ADR 末尾供 owner 复核）
- `.scratch/ve-disclosure-policy-view/issues/02-exception-disclosure-and-conflict-signal-rule-registries-have-no-cli-or-online-entry.md`（1 处）— 1. **取 B（CLI + 端点 + 管理台），口径「同族一致」。** 同族六册（`milestone-mapping` / `triage-rules` / `notification-policy` / `claim-eligibility` / `claim-authorization` / `disclosu…
- `.scratch/ve-disclosure-policy-view/issues/05-ve-review-tails-counts-rename-nil-sentinel-and-adr-0136-addendum.md`（7 处）— # VE 评审尾巴 A 类合票：四处跨文件计数换点名、`application.CatalogRegistry` 跨包同名改名、`assemble_claims.go`「四个协作方」去数、`NewClaimServiceRules` 拒 nil 包哨兵、ADR-0136 越权风险点 2 补一句实测
- `.scratch/wiring-baseline-remainder/issues/01-ps-safe-handoff-is-assessed-nowhere-because-nothing-hands-over.md`（5 处）— ## 裁决（通道 1 代裁，2026-09-09；owner 授权自决口径：硬句不改、每个决定写理由、拿不准的点单列越权风险点）
- `.scratch/wiring-baseline-remainder/issues/03-pc-credit-basis-is-never-asked-for-the-pc-to-sa-seam-does-not-exist.md`（6 处）— **越权风险点**（供评审）：① 解析身份换代（`RES-` / `CLO-` / `CONT-` 指纹含 `CreditSelector`）——无租户故无迁移，但这是键形变化；② expand 段留 nil 沿旧路一格——ADR-0127 决定五写明 contract 何时收；③ `NotFormedReason` …
- `.scratch/wiring-baseline-remainder/issues/06-pp-replay-pricing-evaluation-has-no-executor.md`（2 处）— ### 越权风险点（单列，供 MCP-1 / owner 复核）
- `.scratch/wiring-baseline-remainder/issues/07-pp-decimal-rebuild-boundary-accepts-non-canonical-spellings.md`（4 处）— ### 越权风险点（单列，供 MCP-1 / owner 复核）
- `.scratch/wiring-baseline-remainder/issues/10-credit-ratio-base-is-declared-on-the-credit-policy-content.md`（3 处）—    单列越权风险点，不写进集合。

（共 106 张）
