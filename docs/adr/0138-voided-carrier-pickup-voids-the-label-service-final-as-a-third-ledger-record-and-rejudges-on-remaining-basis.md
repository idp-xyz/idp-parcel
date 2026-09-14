# ADR-0138：实际承运商首次有效收寄的失效版本使已据前版形成的面单渠道服务非取消终局**失效**——终局采用账加第三种记录「终局失效」回指前版、前版只翻当前位不改一字、包裹回到「当前无有效终局」并在同一拍按剩余依据重新判断；失效不是更正，不循 ADR-0117 的「新值替旧值」、只循它的「按来源声明的关系回指、只插不改」；迟到的被失效那一代按账上的失效锚拒绝形成终局

Status: Proposed（2026-09-14，通道 3 按通道 1 派单 task-ea9c8da8 以 PS owner 口径起草；票 [lc/39](../../.scratch/label-channel-service-first-release/issues/39-voided-carrier-pickup-rederivation-needs-an-adr-before-code.md) 的产物，接受与否归 PS owner；被接受后实现另立票，本记录不动 `internal/**`。起草能力边界：读过 PS `CONTEXT.md`「面单渠道服务终局」「受控关闭决定」词条、Rules and invariants 里关于面单渠道服务终局与继续尝试判断的诸句、Lifecycles「包裹身份与服务」与「面单继续尝试」两节；[ADR-0135](./0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 全文、[ADR-0117](./0117-same-source-correction-forms-a-superseding-adoption-version-chained-to-the-current-responsibility-start.md) 全文、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) Decision；`internal/parcelshipment/domain/final_outcome.go` 全文（`ParcelFinalOutcome.Rederive`、`ResponsibilityOutcomeKind`）、`domain/label_service_final.go` 的 `JudgeLabelServiceFinal` 与 `LabelServiceFinalVerdict`、`application/judge_label_service_final.go` 全文、`application/form_parcel_final.go` 的 `Handle` / `rederive` / `refuse` / `commit` / `deriveCompletion` / `handOff`、`ports/ports.go` 的 `FinalOutcomeRecord` / `FinalOutcomeStore` / `FinalOutcomeHandoff`、`adapters/postgres/final_outcome.go` 头注与 `Save` 的翻旧插新、`migrations/parcel_shipment/0004_adoption_cancellation_final.sql` 的 `final_outcome` 表与四条 CHECK、`adapters/transportfulfillment/judge_on_carrier_first_effective_pickup.go` 全文、VE `adapters/parcelshipment/derive_on_final_outcome.go` 的 `kindForFinalRecord`、`cmd/parcel-dispatch/assemble.go` 路由表里 `final-outcome.formed` 那一行；`git grep` 核过 `internal/settlementaccounting` 不消费终局。没读 `adapters/labelfinal/judge_parcel.go` 全文、`operate_label_transaction.go` 的 `parcelsClosedByFinal` 之外部分、`form_continued_attempt_decision.go` 全文、VE 的里程碑目录、inbox 消费门对同分区内未决重投是否阻塞后续信封的实现。）
Date: 2026-09-14

## Context

取证锚 main `f04ad816`，逐符号名。

**问题。** `transport-fulfillment` 对同一条「实际承运商首次有效收寄」事实会发**失效版本**——依据被源更正为不再表达收寄（ADR-0135 决定六：「链尾失效即该对象当前无首次有效收寄，再次形成从失效版本长出新版本，不回退」）。PS 侧 `JudgeOnCarrierFirstEffectivePickupAdapter.HandleRegisteredCarrierFirstEffectivePickup` 收到 `CarrierPickupVoided` 时停在 `ErrVoidedCarrierPickupRederivationUndecided`：不吸收、不重派生、不判、零写入，消费门按未决重投直到失败预算耗尽。这是 AGENTS.md「未确认规则保持显式未决」的正当停格；停格本身不是答案。ADR-0135 越权风险点 4 把「已据收寄形成的非取消终局在依据失效后是否形成新的采用判断版本回指前版（ADR-0117 的形）还是另有格」归 PS owner。

**账上今天长什么样。** 面单渠道服务的终局不另存一处：`JudgeLabelServiceFinalHandler` 判出格后经 `responsibilityOutcomeOf` 折成一份 `ResponsibilityOutcome`（种类 `LabelServiceOutcome` / `LabelServiceFailure`，执行证据 `事实@版本` 或 `CONTINUED-ATTEMPT-CLOSURE/<决定>`，来源版本取证据的版本）交 `FormParcelFinalHandler.Handle` 走网络服务同一条采用路径。采用账 `final_outcome` 每行是一次采用判断，`FinalOutcomeRecord` **二居其一**：终局（`Finalized`，带 `ParcelFinalOutcome` 全件）或不采用（带 `RefusalBasis`）；每（租户，包裹）至多一行 `is_current`，且 `final_outcome_current_only_finalized` 钉「不采用记录从不是当前终局」。同源新版本走 `rederive`：`ParcelFinalOutcome.Rederive(version, source, reason)` 换版本、**换一份新的责任结果**、`prior_version` 指回、原因 `SOURCE_REVISED/<来源版本>`；库面先翻前版 `is_current` 再插新行（AT-PS-063，ADR-0117 Alternatives 明写「`final_outcome` 那条路不改」）。`FindCurrentFinal` 是重派生的锚，也是委托完成派生（`deriveCompletion`）、面单交易建立门（`operate_label_transaction.go` 的 `parcelsClosedByFinal`）、继续尝试判断（`continuedAttemptJudgments` 只看 `is_current`）、资料修订门（`amend_customer_source_data.go`）四处的读口。终局与不采用都经 `FinalOutcomeHandoff` 发 `parcel-shipment.final-outcome.formed`；今天唯一消费方是 VE 的 `derive_on_final_outcome.go`，它按 `record.Finalized` 二分为 `final-outcome-adopted` / `final-outcome-not-adopted` 两种来源事实；`settlement-accounting` 不消费终局。

**CONTEXT 已经写了「终局失效」，只是没写它在账上的形。** PS CONTEXT Rules：「关闭或重开决定自身的来源事实被更正或撤销有效性，以及**终局有效性变化**时，包裹级继续尝试判断都依据剩余有效决定历史和更新后的终局结果重新派生：当前无有效终局且没有生效关闭时派生为开放，仍有生效关闭时保持受控关闭。这类重新派生不是业务重开，不自动形成重开决定或新的面单交易，也不得静默改写原决定历史」；「重开生效前若已经形成当前有效终局，该决定不得生效，并且**不得因该终局后来失效而追溯生效**」。Lifecycles：「当前有效终局服务结果存在 → 禁止业务重开：**源事实更正或事件有效性变化使终局失效后**，依据剩余有效决定历史重新派生开放或受控关闭；此前因终局被拒绝的重开决定不追溯生效」。也就是说：CONTEXT 把「终局失效」当成一个会发生的状态转换写进了继续尝试判断的派生规则，「终局一旦形成永不动」在 PS 统一语言里本来就不成立——成立的是更窄的一句：「终局形成后不得回退为**取消**」（`ParcelFinalOutcome` 头注：「类型上没有任何转取消的方法」）。

**第一问：能不能直接循 ADR-0117。** ADR-0117 裁的是**同来源更正**——新值替旧值：更正版本被采用时编排拿更正后的收寄重述承诺、形成新的采用判断版本回指前版。它的形有两半：**语义半**「新值替旧值」，**纪律半**「按来源声明的更正关系识别、回指前版成链、只插不改」。失效是旧值作废、没有新值：`Rederive` 需要一份新的 `ResponsibilityOutcome`，失效版本给不出（它不表达收寄，`ResponsibilityOutcome` 构造门要执行证据与业务时间，一格都填不出）。所以语义半不能循——硬要循就得造一版「结论为未终局」的终局当 current，那撞 `final_outcome_current_only_finalized`，也让 `ParcelFinalOutcome` 长出「我不是终局」的一格。纪律半能循且该循：失效版本回指它作废的那一代（ADR-0135 决定六「每版回指前版」），PS 据此识别「作废的是不是我当前终局的依据」；账上追加不改写；前版只翻当前位。

**ADR-0029 的尺。** 失效到达后 PS 要做的事按恢复动作分：被失效的那一代恰是当前终局的依据 → 终局失效、包裹回到当前无有效终局、按剩余依据重判——答案形成，无续办动作；被失效的那一代不是任何当前终局的依据（前版从未形成终局：取消在先、规则未配置停在未决、或还没判到；或当前终局已换了别的依据）→ 无事可做，但要留痕，理由见决定二；TF 读口读不回失效版本 → 等可见（既有 `ErrCarrierPickupNotVisible`）；键与本体不符 → 装配缺陷（既有 `ErrCarrierPickupRecordInconsistent`）。前两格恢复动作都是「无」，可区分性只对审计有用——所以两格合成一种记录、以是否翻了前版分两式，不各占一格结果词。

**用三个场景试候选语言**（每个写运营看到的、账上留的、下游收到的）：

- **(i) 失效后再无新收寄。** 包裹 P 据收寄 F@v1 形成终局 T1（`LABEL_SERVICE_OUTCOME`，证据 `F@v1`）。源更正 F 为不再表达收寄，TF 发失效版本 F@v2（回指 v1）。运营看到：P 从「已终局（收寄）」变成「终局失效，当前无有效终局」；继续尝试判断按剩余决定派生——没有生效关闭即开放，可依适用规则再建面单交易（各资格门照旧）；有生效关闭即保持受控关闭。账上：T1 那行 `is_current` 翻假、其余一字不动；新增一行「终局失效」（键 = (P, `LABEL_SERVICE_OUTCOME`, F@v2)，`prior_version` = T1，锚 `F@v1`，原因 `SOURCE_VOIDED/F@v2`）；P 没有 `is_current` 行；委托完成摘要下次派生时从「全部完成」退到部分或无（它是派生摘要「不是可编辑状态」，`deriveCompletion` 每次重算）。下游：一封 `final-outcome.formed`，记录种类为「终局失效」；VE 据此译第三格（里程碑怎么回退归 VE）；SA 无消费。
- **(ii) 失效后同一事实又来一版有效收寄。** 接 (i)，源再更正，TF 从失效版本长出 F@v3（已形成，业务时间 t3）。运营先看到失效、再看到新终局（生效时间 t3）。账上：失效行之后多一行终局 T3（证据 `F@v3`）——若 (i) 之后没有形成过别的终局，T3 是首派生（无 `prior_version`）；若 (i) 的同拍重判已按关闭路径形成了终局 Tc，T3 走既有分派「同种类 → 重派生」（`SOURCE_REVISED/F@v3`，`prior_version` = Tc）。下游：两封 formed。**乱序支**：若 F@v3 先于 F@v2 到（分区按对象保序时不会，但不靠它）——v3 到时当前终局是 T1、同种类 → 重派生成 T3；v2 再到，它作废的 `F@v1` 已不是任何当前终局的依据 → 落一行「终局失效」但 `prior_version` 缺席、不翻任何行；末态与顺序到达一致。
- **(iii) 失效到达时该委托已有受控关闭生效。** `JudgeLabelServiceFinal` 里收寄先于关闭（规则 2 先于规则 3），所以 T1 是收寄终局，关闭只派生「不允许新增尝试」。失效到达：T1 失效，同一拍以「无收寄」为输入重判——受控关闭生效且未被重开、全部相关交易均已定案、无有效或待确认面单结果 → 立即形成关闭路径终局（有过成功且均已作废 / 失效 → `LABEL_SERVICE_OUTCOME`；从未成功 → `LABEL_SERVICE_FAILURE`；生效时间取关闭生效与最后一份定案 / 作废之中较晚者）；任一条件不满足 → `NOT_FINAL` 带原因，包裹停在受控关闭等定案。运营看到：终局失效紧接着要么一份新终局（依据换成关闭），要么「受控关闭、待定案」。账上：失效行 + （可能）Tc 行；此前因 T1 被拒绝的重开决定不追溯生效（CONTEXT 原句）。下游：一或两封。

三个场景里没有一处需要「回退为取消」、没有一处需要改前版一字、没有一处需要新的结果词之外的东西；场景 (iii) 逼出「同一拍重判」——不重判，受控关闭下已全部定案的包裹在 F@v2 之后永远没有终局，因为再也不会有一封信来触发它。

## 候选与反方

**甲 · 循 ADR-0117：失效版本形成一版新的采用判断回指前版，结论「未终局」或「终局撤回」。** 反方：ADR-0117 的语义半是新值替旧值，`Rederive` 要新的 `ResponsibilityOutcome`，失效给不出；「结论未终局的终局版本」若为 current 撞 `final_outcome_current_only_finalized`，若不为 current 就不是「新的采用判断**版本**」而是另一种记录；`FindCurrentFinal` 的四个读口都得学会「current 但不是终局」这一格。反方能否定甲的**版本**那半，否定不了它的**回指前版、只插不改**那半。**处置：取纪律半、弃语义半**——失效不形成终局的新版本，形成采用账的**第三种记录**，见决定一。甲经此修正即本记录。

**乙 · 失效不是更正：终局不退，账上追加「依据已失效」标记并交人核（对照 ADR-0135 决定四「不构成」不落版本）。** 反方：PS CONTEXT 已写「源事实更正或事件有效性变化**使终局失效后**，依据剩余有效决定历史重新派生开放或受控关闭」与「不得因该终局后来失效而追溯生效」——「终局失效」是 CONTEXT 里已有的状态转换，乙让终局站着就是拒绝执行 CONTEXT；乙依赖的「终局一旦形成不回退」在 CONTEXT 里只有「不得回退为**取消**」这一句，撑不起「不得失效」；ADR-0135 决定四「不构成不落版本」说的是**从未构成**收寄的证据，这里是**曾构成、后被源作废**，TF 自己落了失效版本（决定六），PS 若让据它形成的终局继续有效，就成了 PS 认 TF 已否认的事实；「交人核」把一件机制上可判的事推给运营，且运营核完要做什么（让终局失效）乙没定义——人核就无路可走。反方成立，乙否决。乙有一半是对的且保留：**失效不是取消、不是重开**——不复活旧交易、不自动形成重开决定、不追溯生效被拒的重开（决定五）。

**丙 · 不落账，读时派生：`FindCurrentFinal` 时去 TF 问收寄链尾是否失效。** 反方：把跨上下文读放进每一次终局读取，违 ADR-0005（派生状态从本上下文事实派生）与 ADR-0025（翻译只在消费侧适配器一处）；VE 收不到任何信号；TF 链尾再长新版本时 PS 的「当前终局」会跳来跳去、无一行可审计。否决。

**丁 · 失效时删除或 UPDATE 前版行的 `finalized`。** CONTEXT「不得静默改写原决定历史」、AT-PS-063「历史不删」直接否决，不展开。

## Decision

**一、失效不是更正。「终局失效」是终局采用账（`FinalOutcomeRecord`）的第三种记录，与「终局」「不采用」并列；它不是第三种终局，也不是不采用。** 一行终局失效记录带：采用键（租户，包裹，`LABEL_SERVICE_OUTCOME` / `LABEL_SERVICE_FAILURE`，失效版本号——与其余采用记录同键形，按 TF 版本幂等）；**失效锚**——被作废的那一代收寄引用（`F@v1`，取自失效版本回指的前版）；被失效的终局版本 `prior_version`（可缺席，见决定二）；原因词 `SOURCE_VOIDED/<失效版本>`；采用时间。不带终局全件、不带不采用依据；`is_current` 恒为假。前版终局行只翻 `is_current`（AT-PS-063 既有做法），其余一字不动。失效之后该包裹 `FindCurrentFinal` 答「没有」——CONTEXT「使终局失效后」在账上的形就是「当前无有效终局」。**词：用 CONTEXT 原词「终局失效」，不用「撤回」**——「撤回」在本上下文是委托撤回的词（`Withdrawal`），拿来叫终局会让两件事共用一个名字；代码名照 TF 的 `CarrierPickupVoided` 取 `Voided`，与面单有效期的「不可逆失效」（`Lapsed`）也分开。

**二、失效记录以被作废的那一代为锚，先到后到都落一行；只有它恰是当前终局的依据时才翻前版。** 判据两件：当前有效终局存在且 `Finalized`；它的执行证据恰是失效版本所回指的那一代（`F@v1`）。两件齐 → 翻前版 `is_current`、失效记录 `prior_version` 指它。任一不齐（当前无终局：前版收寄判过但取消在先或规则未配置停在未决、或还没判到；当前终局依据别的证据：已被替代版本重派生、或走了关闭路径）→ 失效记录照落、`prior_version` 缺席、不翻任何行。两式恢复动作相同（无），按 ADR-0029 不分结果词，只由 `prior_version` 在不在区分。**锚的用处在反向**：形成或重派生收寄终局之前，采用路径先按（租户，包裹，证据 `F@vN`）查账上有没有以它为锚的失效记录，有 → 不采用，依据 `EVIDENCE_VOIDED/<失效版本>`——这堵住「前版停在未决、失效先到、前版后来重投成功而据一条已作废的收寄形成终局」那一格，不靠分区保序、不问 TF 链尾。

**三、失效使当前终局失效之后，同一拍以「无收寄」为输入重判一次，不等下一封信。** 触发它的事实已在手上、输入全在本上下文（取消视图、该包裹全部交易、登记册、有效期规则），等下一封信等于等一个不一定会来的事件（场景 i）；场景 (iii) 受控关闭下已全部定案的包裹若不重判就永远没有终局。重判走既有 `JudgeLabelServiceFinal` → 形成终局的格照既有采用路径落账：`FindCurrentFinal` 已答「没有」，所以是**首派生**（无 `prior_version`）——它是另一条依据上的新形成，不是从失效的收寄终局重派生出来的新值；挂成重派生会让 `prior_version` 指向一个已失效的版本，读链的人会以为收寄那一版仍在链上有效。不形成 → `NOT_FINAL` 带原因，如今天。失效记录与重判形成的终局**同一笔事务**落（消费门一拍一笔的既有形），后半失败整拍回滚重投，不留「失效已落、重判未做」的中间态（ADR-0134 决定五同一取法）。只有真的翻了前版才重判；决定二的「`prior_version` 缺席」那式不重判——当前终局若在，它不是据被作废的证据形成的，没有东西要重派生。

**四、失效之后同一事实的新有效版本照今天的路走，不新造格。** F@v3（已形成）到达 → 收寄终局 → 采用键版本 F@v3：当前无终局 → 首派生；当前有同种类终局（场景 iii 重判形成的 Tc）→ 既有分派「同种类 → 重派生」，`SOURCE_REVISED/F@v3`，证据从关闭换回收寄、生效时间换成收寄时间——这正是 AT-PS-063 重派生本来的用途。`LabelServiceOutcome` 与 `LabelServiceFailure` 两格是否算「同源」可互相重派生，本记录不裁（越权风险点 2）。

**五、终局失效是「终局有效性变化」，它引起的一切都是派生，不是新决定。** 包裹级继续尝试判断按剩余有效决定历史重新派生（CONTEXT 原句：无生效关闭 → 开放；仍有生效关闭 → 保持受控关闭）；此前因该终局被拒绝的重开决定不追溯生效；委托完成摘要随成员终局回退（派生摘要，每次重算）；面单交易建立门读不到关闭该包裹的终局，继续尝试判断为开放时可再建交易、各资格门照旧。不复活已失败 / 已作废 / 已不可逆失效的旧交易，不自动形成重开决定，不形成新的面单交易，不改任何一条既有决定。

**六、下游契约：失效记录经同一意图口 `FinalOutcomeHandoff` 出去，信封类型不变，记录种类显式三值。** 今天 VE 按 `Finalized` 真假二分，失效记录若不显式会被读成 `final-outcome-not-adopted`（「迟到来源未采用」）——语义相反。记录（连同它跨上下文出去的形）带封闭种类 {终局，不采用，终局失效}；VE 消费侧照 ADR-0025 自己译第三格，里程碑回退还是标注归 VE owner；`settlement-accounting` 今天不消费终局，无事；委托完成派生不对外发信，无事。

**七、`LabelServiceFinalOutcome` 长第六格 `LABEL_SERVICE_FINAL_VOIDED`，`ErrVoidedCarrierPickupRederivationUndecided` 随实现票消失。** 处理方适配器在 `CarrierPickupVoided` 上走决定二 / 三，结果译成消费结论「入账」（答案形成、不重投）；`carrierFirstEffectivePickupJudgmentUndecidedSentinels` 名单里那一格删。既有的 `ErrCarrierPickupNotVisible`（等可见，重投）与 `ErrCarrierPickupRecordInconsistent`（装配缺陷，不重投）对失效版本同样适用，不另加。

**八、三场景按上述决定各得其所（细节见 Context 的场景段）。** (i) 失效后再无新收寄：决定一 / 二翻前版、落失效行，决定五派生开放或保持受控关闭，决定六一封信；账上无 current 行，不再有别的动作。(ii) 失效后同一事实又来一版有效收寄：决定四——首派生或同种类重派生，不新造格；乱序到达由决定二的锚保证末态一致。(iii) 失效到达时受控关闭已生效：决定三同一拍重判，全部定案且无有效结果即出关闭路径终局（首派生），否则停在受控关闭待定案；被拒过的重开不追溯生效（决定五）。候选甲、乙的反方各自成立的那一半都进了决定：甲的「回指、只插不改」进决定一 / 二，乙的「不是取消、不是重开」进决定五。

## Consequences

- PS CONTEXT Lifecycles「包裹身份与服务」一节加一条转换：「面单渠道服务非取消终局结果 → 终局失效：其依据的实际承运商首次有效收寄事实被源更正为失效版本；包裹回到当前无有效终局，同一拍依剩余有效决定历史与交易定案重新判断；原终局历史不改」；GLOSSARY 是否补「终局失效」归推送方核。**随实现票同笔落，本记录不动 `docs/domain/**`**（票 lc/39 红线）。
- 实现票（PS owner 立，形见票 lc/39 完成记录）：`domain` 的 `FinalOutcomeRecord` 第三种记录与失效锚值对象、`LabelServiceFinalOutcome` 第六格；`application` 的 `form_parcel_final.go` 加「终局失效」命令与「以证据为锚查失效记录」的采用前门、`judge_label_service_final.go` 加「失效 → 重判」入口；`ports.FinalOutcomeStore` 加按（租户，包裹，证据）查失效记录的读口；`adapters/postgres` 的 `final_outcome` 第三种行形（`final_outcome_result_shape` 与 `final_outcome_rederivation_coherent` 两条 CHECK 各加一支，PS 迁移序号实施时重取）与 `Save` 的翻旧插新对失效记录的处置；`adapters/labelfinal` 的处理方核多一个入口与消费翻译表一行；`adapters/transportfulfillment/judge_on_carrier_first_effective_pickup.go` 的 `CarrierPickupVoided` 分支接上、哨兵删；`cmd/parcel-dispatch/assemble.go` 名单一行；VE `derive_on_final_outcome.go` 第三格（VE 地盘，占号或另立票）。
- 代价：`final_outcome` 一张表上第三种行形，读它的四个读口（`FindCurrentFinal` 的四处调用方）语义不变——它们只问 `is_current`，失效记录永不为 current；`FindByKey` 的调用方要认第三种记录。
- 不在本记录内：TF 半边（ADR-0135 已裁，失效版本入队、PS 按版本取回）；VE 里程碑对「终局失效」怎么投影（VE owner）；`LabelServiceOutcome` 与 `LabelServiceFailure` 是否同源（越权风险点 2）；网络服务那四格责任结果的来源事实失效（有效交付被更正为无效等）是否同形——本记录只裁面单渠道服务收寄这一路，同形可循但不在此裁（越权风险点 4）。

## Alternatives considered

见「候选与反方」：甲取纪律半弃语义半即本记录；乙、丙、丁否决，理由各在其反方。另两条：

- **失效记录不落账、只在结果里答「已失效」。** 否决：CONTEXT「终局有效性变化时……重新派生」要有人答得出「为什么当前无终局」；ADR-0103「未知也是一个版本」、ADR-0135「待确认是带原因的版本」同一条理由——账上少这一行，审计到前版就断了。
- **失效记录以失效版本为锚而不以被作废的那一代为锚。** 否决：迟到的前版查的是「我据以形成的这一代有没有被作废」，锚在被作废的那一代才查得到；锚在失效版本上得先知道失效版本号，前版不知道。

## 越权风险点

1. **同一拍重判还是下一拍。** 我按「输入全在本上下文、触发事实在手上」取同一拍同一笔；若 PS owner 认为终局失效与重判该分两拍（比如为了让失效那封信先发出去），改的是决定三的事务边界，记录形状不变，但要补「失效已落、重判未做」那一格的续办引用。
2. **`LabelServiceOutcome` 与 `LabelServiceFailure` 是否同源。** 今天 `FormParcelFinalHandler` 按 `ResponsibilityOutcomeKind` 相等判同源；两格都是 `JudgeLabelServiceFinal` 一条判断的产物（`IsLabelService` 已把它们归为一族），关闭路径先出「终局失败」、后来收寄到达要换成「非取消终局」时，按今天的判法会落「异源不采用」。我倾向同族视为同源可重派生，但那改的是网络服务也走的同一段分派，超出本记录，未取。
3. **迟到前版的拒绝词。** 我取不采用 `EVIDENCE_VOIDED/<失效版本>`（不是等谁，无续办）；若 owner 认为该格应是「未决」以便重投——重投不会改变结果（作废是源说的），我按 ADR-0029 不取未决。
4. **网络服务四格的来源失效是否同形。** 有效交付被 TF 更正为无效今天走什么路我没量；本记录的形（第三种记录、以证据为锚、同拍重判）对它可能同样成立，但网络服务的重判入口不是 `JudgeLabelServiceFinal`，不在此裁。
5. **委托完成摘要回退的下游后果。** 摘要是派生、不对外发信，回退不改任何账；但若日后有消费方据「全部完成」做了不可逆的事（结算、归档），它们按各自上下文的更正规则处理，PS 不为此多留一格。
6. **失效锚查询的范围。** 我按（租户，包裹，证据）查；若 owner 认为同一收寄事实可能挂到多件包裹（载运对象是集运单元时），锚查询要按（租户，证据）跨包裹，实现票量。

## Links

- [PS CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「面单渠道服务终局」词条；Rules and invariants「终局有效性变化时……重新派生」「不得因该终局后来失效而追溯生效」；Lifecycles「面单继续尝试」一节「源事实更正或事件有效性变化使终局失效后……」；Lifecycles「包裹身份与服务」一节（本记录新转换的落地处）
- [ADR-0135](./0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md)：决定六（失效版本）、决定七（按版本取回）、越权风险点 4（本记录所答）
- [ADR-0117](./0117-same-source-correction-forms-a-superseding-adoption-version-chained-to-the-current-responsibility-start.md)：同来源更正的采用版本链——本记录循其纪律半、不循其语义半
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格（决定二两式合一、越权风险点 3）
- [ADR-0134](./0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md)：决定五「入队失败回滚整步」（决定三同一取法）
- [ADR-0005](./0005-source-facts-effective-events-derived-state.md)、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：丙的否决依据
- 票 [lc/39](../../.scratch/label-channel-service-first-release/issues/39-voided-carrier-pickup-rederivation-needs-an-adr-before-code.md)（本记录的出处）、[lc/25](../../.scratch/label-channel-service-first-release/issues/25-external-carrier-first-pickup-triggers-label-final-judgment.md)「要裁的」2（问题出处、哨兵落地处）、[lc/31](../../.scratch/label-channel-service-first-release/issues/31-carrier-first-effective-pickup-fact-registry-and-handoff.md)（TF 半边实施）
