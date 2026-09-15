# owner 复核队列 · 钉 `37f8050a` · 2026-09-15 18:46

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



（共 20 篇 ADR 含越权风险点）

## 二、needs-info / blocked 的票

- `.scratch/auto-reroute-demo-reachability/issues/02-syn-vertical-run-reaches-reroute-after-lapse.md`
  Status: blocked——通道 2 于 2026-09-07 按票面「开工第一步」读完 `PlanReview` / `reviewCandidates` 与生产 `InitialRouteEvidenceView`，结论写在下方「裁决」节：交付第 1 件的头一步（登 SYN 网络定义让初始路由成立）在今天的生产装配里**走不到**——唯一的证据视图实现 `nrpostgres.Netwo…
- `.scratch/first-tenant-runway/issues/03-network-resolution-layer.md`
  Status: blocked（2026-09-02 MCP-5 答完三问；结论是被 `PAR-NET-14` 硬阻断，**本票暂不拆实现票**。阻断解除前唯一可动的是一张前置票，见 `## Answer` 第四节）
- `.scratch/nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md`
  Status: needs-info
- `.scratch/ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md`
  Status: needs-info
- `.scratch/syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md`
  Status: needs-info——ADR-0072 口径的重启条件已满足，票面范围已按其 Consequences 改写（见「重启范围」）；S0 的证据缺口三项未到，未到前不进 S1
- `.scratch/ve-008-late-account-rederive/issues/04-ops-replay-endpoint-blocked-on-par-int-01.md`
  Status: needs-info

（共 6 张）

## 三、票面里提到越权风险点的票

- `.scratch/admin-write-faces/issues/08-publication-form-path-common-half-server-side-digest-and-envelope-approval.md`（3 处）— Status: resolved——2026-09-08，MCP-1 接 MCP-5 `mcp5-awf08@c003c845` 两笔续做完（用户 12:4x 指示通道 1 自办、不派工；分支 `mcp1-awf08` 基 main `5aedbb0f`，逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SH…
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
- `.scratch/party-commercial-context-gaps/issues/08-authorized-action-lacks-source-data-amendment-and-adjudication-names-no-decider.md`（4 处）— Status: resolved——2026-09-07，MCP-5（task-0d116e60，分支 `mcp5-pcgaps08` 基 `299f2a2e`）：代码半边四笔由 MCP-1 重放进 main（`1e182860`→`0cfe3571` / `6b43484c`→`31903cc5` / `df923d…
- `.scratch/party-commercial-context-gaps/issues/09-final-rule-content-has-no-validity-declaration.md`（3 处）— Status: resolved——2026-09-07，MCP-6（task-390c4f53，分支 `mcp6-pcgaps09` 基 `250e5a43`）：ADR-0119 六问一次答完（越权风险点三条单列在 ADR 里供 owner 复核）、PC CONTEXT「面单服务终局规则」词条补段、迁移 0026、领…
- `.scratch/party-commercial-context-gaps/issues/10-source-data-amendment-allowance-is-a-third-stage-content-family.md`（2 处）— Status: resolved——2026-09-07，MCP-6（task-390c4f53，分支 `mcp6-pcgaps10` 基 `9379c716`）：ADR-0120 七问一次答完（越权风险点四条单列在 ADR 里供 owner 复核）、PC CONTEXT 新词条 + Boundaries 一句、迁移 …
- `.scratch/party-commercial-context-gaps/issues/11-delivery-condition-declaration-family-and-resolution-keyed-read-face.md`（4 处）— Status: resolved——2026-09-10 13:0x 通道 5 收口（单 task-4649070b-bec3-433b-ba5c-58fb8b4ac4b7，改派自通道 4；分支 `mcp5-pcgaps11` 基 `d9a1f571`，每笔已推 origin；完成记录见文末，进 main 记录归推送方…
- `.scratch/party-commercial-context-gaps/issues/12-declared-responsibility-outcome-lacks-label-channel-rows.md`（1 处）— 1. **两行的原词取 `LABEL_SERVICE_COMPLETED`（非取消终局结果）与 `LABEL_SERVICE_FAILED`（终局失败结果）**，Go 常量 `DeclaredLabelServiceCompleted` / `DeclaredLabelServiceFailed`。照既有四值的构词（`…
- `.scratch/party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md`（8 处）— Status: resolved——2026-09-10 20:3x（本机时钟）通道 1 推送方重放进 main（非作者评审：两个隔离子代理鉴权错死掉，改由推送方按 /code-review 两轴串行自评，0 阻断；main 上 SHA 对照见 Comments「进 main 记录」）。此前 resolved——202…
- `.scratch/pp-pricing-input-seams/issues/03-ps-origin-destination-postal-route-read-port.md`（1 处）— 7. **（2026-09-15 10:4x 通道 1 追裁 ← 通道 5 量得条目只进摘要）裁决 1 的前提不成立，取 A：口成形、内容落库归 [05](05-ps-submission-and-source-data-versions-carry-content.md)。** 作者开工对 `3a21dab7` 量到…
- `.scratch/ps-port-remainder/issues/01-label-validity-rule-is-a-lapse-declaration-on-the-final-rule.md`（1 处）— - 2026-09-10 14:2x · 通道 1（解阻簿记，未动代码；取证于远端 main `5dda0fb2`）：**PC 半边在 main 上了**，PS 半边转 ready-for-agent。对号本票裁决①的 PC 半边三件：有效期声明一格——`final_rule_content` 加 `validity_…
- `.scratch/ps-port-remainder/issues/05-customs-and-node-operations-need-parcel-keyed-stage-fact-read-faces.md`（1 处）—   - **不取 ADR-0124、不改 CC/NO CONTEXT**：两面读法都是既有生命周期句与硬句的直接读法（重开、替代、候选不得当归属），形状可逆、无新词条；跨上下文归属已由 ADR-0118 决定三/四定下。预留号 0124 未消费，请 MCP-1 收回。越权风险点两条，单列：(1) CC「已关闭」把重开读…
- `.scratch/ps-port-remainder/issues/06-delivery-place-reference-read-face.md`（3 处）— 3. **包裹 → 委托的路**：经 `AcceptanceBaseline.covers` 走声明成员；包裹身份谱系（拆分 / 合并后的新包裹）今天领域里没有模型，本票先只按声明成员解析，谱系包裹会落「没有收件地点」——在读口头注写明这一格是「谱系未建模」的今日形状，谱系落地那票要补这一路（同 ps-port-rem…
- `.scratch/ps-port-remainder/issues/07-commercial-resolution-reference-by-parcel-read-face.md`（2 处）— - 2026-09-09 · 通道 2（task-f5521768）：立票（ready-for-agent）。形状在 ADR-0133，本票不再裁；ADR-0133 越权风险点 5（谱系包裹与集运单元同答没有）若 owner 复核后改口径，本票随之改那一格，不回改 ADR。
- `.scratch/ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md`（4 处）— - [ADR-0136](../../../docs/adr/0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md) 决定三：`CustomerServiceRuleObject` 必须在接受时闭…
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
- `.scratch/tf-carrier-master-document-register/issues/01-carrier-master-document-register-does-not-exist.md`（3 处）— Status: resolved（MCP-5 实施完成，2026-09-07，隔离分支 `mcp5-tf-cmdr`，基线 main `92579b0a`，task-895fbabf；形状取票面第 1 条「独立登记册」，落文 [ADR-0113](../../../docs/adr/0113-carrier-maste…
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

（共 50 张）
