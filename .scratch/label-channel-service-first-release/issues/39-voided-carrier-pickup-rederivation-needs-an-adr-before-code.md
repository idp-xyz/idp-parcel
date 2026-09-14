# 39 失效版本到达后，已据前版收寄形成的非取消终局怎么重派生——先立 ADR，再谈代码

Category: enhancement
Status: ready-for-agent——2026-09-14 10:2x 通道 1 推送方按用户 10:1x「授权代裁」立票：lc/25「要裁的」2 是设计题不是一句话能裁的（ADR-0135 越权风险点 4 明写「归 PS owner，随 lc/25 实施时判」，lc/25 实施时按红线落了显式未决哨兵），本票的产物是**一份 ADR 草案**（`Proposed`），不是代码；ADR 被接受后实现另立票。取证锚 main `6bbf2bf0`。只写票面，未动代码、未动 `docs/**`。
Blocked by: 无（lc/25 与 lc/31 均已进 main；TF 半边失效版本的入队与 PS 按版本取回都在）
Type: grilling → ADR 草案（走 `/domain-modeling`，PS owner 口径；实现不在本票）

## 缺口（取证于 `6bbf2bf0`，逐符号名）

- TF 会对同一条「实际承运商首次有效收寄」事实发**失效版本**——依据被源更正为不再表达收寄（ADR-0135 决定六）。PS 侧 `internal/parcelshipment/adapters/transportfulfillment/judge_on_carrier_first_effective_pickup.go` 收到失效版本时停在未决哨兵 `ErrVoidedCarrierPickupRederivationUndecided`：不吸收、不重派生、不判、零写入；头注写明「owner 裁定后（ADR-0117 同来源更正的采用版本链是可循的形）在本适配器接上重派生，这一格随之消失」。用例 `judge_on_carrier_first_effective_pickup_test.go` 钉住「失效版本 → 未决且零写入」。
- 也就是说：一笔已据某版收寄判成「面单渠道服务终局已形成」的委托，在那版收寄失效之后，今天**永远停在未决**——消费门会按未决重投，直到失败预算耗尽。这是 AGENTS.md 红线「未确认规则保持显式未决」的正当停格，但停格本身不是答案。
- `LabelServiceFinalOutcome` 的各格（lc/38 条 2 已逐名点过）里没有「依据失效」这一格；`LabelServiceFinalAdopted` 形成后的采用记录（`FinalOutcomeStore` 那一处当前有效终局）今天没有「回退」或「被后版替代」的形。

## 语言从哪里来

- PS `CONTEXT.md`「实际承运商首次有效收寄即形成终局」——终局的**依据**是那条收寄事实；依据失效，终局的依据就没了，CONTEXT 没有说这时终局是什么。
- ADR-0135 决定六（TF 发失效版本）与越权风险点 4（「已据收寄形成的非取消终局在依据失效后是否形成新的采用判断版本回指前版（ADR-0117 的形）还是另有格——归 PS owner」）。
- ADR-0117：同来源更正在采用口形成新的采用判断版本并回指前版，AT-PS-049 不动——它裁的是**更正**（新值替旧值），不是**失效**（旧值作废、没有新值）；能不能直接循它，是本票要回答的第一问。
- ADR-0029：结果按恢复动作分格——「依据失效后重派生」的恢复动作是什么（等新收寄？回到未终局？形成一版「终局撤回」？）决定它该是哪一格。

## 要做的（ADR 草案，不写代码）

1. **`/domain-modeling`**：以 PS owner 口径，把「依据失效后的终局」放进 PS 统一语言里试三个场景——(i) 失效后再无新收寄；(ii) 失效后同一事实又来一版有效收寄（替代版本，ADR-0135 决定六另一支）；(iii) 失效到达时该委托已有受控关闭生效。每个场景写出运营看到的是什么、账上（`FinalOutcomeStore`）留下的是什么、下游（VE 投影 / SA 采用）收到的是什么。
2. **候选至少两条，各写反方**：甲 · 循 ADR-0117——失效版本形成一版新的采用判断（结论「终局依据失效 → 未终局」或「终局撤回」）回指前版，历史不改；乙 · 失效不是更正——终局一旦形成不因依据失效而退，只在账上追加「依据已失效」的标记并交人核（与 ADR-0135 决定四「不构成」不落版本的精神对照）；若还有丙，写。
3. **落 ADR 草案** `docs/adr/0138-*.md`（序号开工时按目录重取；`Status: Proposed`），Context 锚 SHA、Decision 按 ADR 形写、越权风险点单列；`docs/adr/README.md` 加一行；lc/25「要裁的」2 与 ADR-0135 越权风险点 4 各追一句指向它（追加不改写）。
4. **不做**：不改 `judge_on_carrier_first_effective_pickup.go` 与任何 `internal/**`；不改 PS `CONTEXT.md`（ADR 若要求改词条，写进 ADR Consequences，由实现票随 ADR 接受一并落）；不裁 TF 半边（ADR-0135 已裁）。

## 红线

- 未确认规则不写死：ADR 草案是 `Proposed`，哨兵 `ErrVoidedCarrierPickupRederivationUndecided` 在 ADR 接受与实现票落地之前一字不动。
- 不引行号、不数别处的东西；引 CONTEXT / ADR 用引文或决定号。
- 不为任何租户预设「失效后怎么办」的取值——这是机制的形，不是实例参数。

## 完成判据

1. `docs/adr/0138-*.md`（或重取的号）存在、`Status: Proposed`、Decision 至少覆盖「要做的」1 的三个场景与 2 的两条候选各自的反方；README 有行。
2. lc/25「要裁的」2 与 ADR-0135 越权风险点 4 各有一句指向新 ADR（`git grep -n '0138' -- .scratch/label-channel-service-first-release/issues/25-* docs/adr/0135-*` 各 ≥ 1，序号按实取）。
3. `git diff --stat main -- internal/ cmd/ apps/ migrations/` 为空。
4. 完成记录写清：推荐哪条候选、为什么、实现票该长什么样（地盘、判据、要动的哨兵与结果格）——推送方据此立实现票。

## 地盘

`docs/adr/`（新文件 + README 一行 + ADR-0135 追一句）、本票面、lc/25 票面「要裁的」2 一句、lc spec 子票表一行。**不动** `internal/**`、`cmd/**`、`docs/domain/**`。

## 参照

[lc/25](./25-external-carrier-first-pickup-triggers-label-final-judgment.md)「要裁的」2 与「裁决」；[lc/31](./31-carrier-first-effective-pickup-fact-registry-and-handoff.md)；[ADR-0135](../../../docs/adr/0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 决定六 / 越权风险点 4；[ADR-0117](../../../docs/adr/0117-same-source-correction-forms-a-superseding-adoption-version-chained-to-the-current-responsibility-start.md)；ADR-0029；PS `CONTEXT.md`「面单渠道服务终局」一节；`internal/parcelshipment/adapters/transportfulfillment/judge_on_carrier_first_effective_pickup.go` `ErrVoidedCarrierPickupRederivationUndecided` 头注；`internal/parcelshipment/application/judge_label_service_final.go` `LabelServiceFinalOutcome`。

## Comments

- 2026-09-14 10:2x · 通道 1 推送方：立票并直接 ready（用户 10:1x「授权代裁」；这一条我**不一句裁**——它是 ADR-0135 明写留给 PS owner 的设计题，一句话裁掉等于在票面里藏一条难逆转决定）。**只写票面，未动代码与 `docs/**`。** 能力边界：读过哨兵头注与 lc/25「要裁的」2 / ADR-0135 越权风险点 4 / ADR-0117 摘要；没读 `FinalOutcomeStore` 的采用记录形与 VE / SA 对终局的消费路径——「要做的」1 的场景 (ii)(iii) 与下游那半靠作者量。
