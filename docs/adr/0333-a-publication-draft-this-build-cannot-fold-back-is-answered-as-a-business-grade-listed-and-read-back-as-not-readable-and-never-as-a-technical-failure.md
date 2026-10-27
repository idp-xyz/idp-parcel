# ADR-0333：本构建折不回正文的待批准发布载体答业务格而不是技术失败——列表带着它并标出读不回，单份读回、批准与发布答 `DRAFT_NOT_READABLE`，已发布的如实答已发布，摘要绑定照旧

Status: Accepted（2026-10-27，Claude Code 会话代 PC owner 裁定：用户原话「你作为业务和系统专家自决」；协调方据票 [docs-consistency/code/25](../../.scratch/docs-consistency/code/25-sell-volumetric-factor-and-amount-rounding-are-read-from-the-price-card.md) 隔离评审 N1 立票 [docs-consistency/code/87](../../.scratch/docs-consistency/code/87-a-publication-draft-this-build-cannot-rehydrate-answers-5xx-instead-of-a-business-grade.md)；批准与发布各答什么、已发布的载体答什么、发布口是否复用`发布未落定`、录入口要不要新格、读不回的行在列表与单份读回里怎么办，由作者代决。裁决能力边界：读过 ADR-0314 全文、`RehydratePublicationDraft` 与 `RehydratePublicationContent`、载体册适配器、批准 / 发布 / 录入三个用例、两个读回端点与管理台待批准页；越权风险点见文末，待 PC owner 复核。部分停用（收窄）[ADR-0314](./0314-pending-publication-drafts-are-readable-back-by-the-operator-and-approval-and-publication-are-bound-to-the-content-digest-the-caller-saw.md) 越权风险点 5 一句，被停原句与适用场景见「对已接受记录的作用」；ADR-0314 正文不改，只在其 Status 行与 Links 加前向指针。）<!-- 索引注记 -->｜**两格**：载体行的重建门分「行本身坏了」（痕迹、壳、册与摘要对不上，仍是技术失败）与「行没坏、本构建把快照折不回正文」（规范化版本不认得，或正文过不了本构建的构造门——如价格规则上的体积系数）。**读不回的行**交回库上剩下的事实（身份、状态与痕迹、列上的规范化版本与摘要、壳摘要）加成因，不交正文。**批准与发布**：摘要绑定先于状态，其后`已发布`答已发布、其余答 `DRAFT_NOT_READABLE`，一字节不写，200。**录入**：已过`待批准`的读不回行答内容已固定，`待批准`的由同版本重录替换。**读面**：列表带着它并标 `readable:false`，单份读回答 `DRAFT_NOT_READABLE` 加事实、不带文档。**管理台**：状态旁标「本构建读不回」，抽屉说随状态的恢复动作，不摆批准与发布
Date: 2026-10-27

## Context

- ADR-0126 决定三的载体表存规范化文档与摘要，读回经重建门（`RehydratePublicationDraft`）把文档折回正文再算一遍摘要。重建门拒绝的理由有两种性质：行本身对不上（痕迹缺、壳立不起来、快照里的册与壳的类别不同、折得回而摘要不一致），这样的行不可能是本上下文录入或推进出来的，处置是查库；另一种是行没有坏，只是本构建把它的快照折不回正文。
- 第二种今天就会出现，有两个来源。体积系数不在价格政策上声明（ADR-0260 决定二），带 `volumetricFactor` 的 `PCC-1` 价格规则快照折回时被正文构造门点名拒（`ErrVolumetricFactorOnPricePolicy`），这样的载体只可能是那条规则落地之前录入的；规范化版本本构建不认得（`ErrCanonicalizationUnsupported`，ADR-0014），滚动发布或回滚时旧构建读新构建写的行正落在这里。
- 重建门把这两种都包成同一个 `ErrInvalidRehydratedPublicationDraft` 并丢了成因链，载体册原样上抛；批准、发布、录入三个用例把读回的 error 原样包成 error 返回，HTTP 译成 5xx `NO_ANSWER_FORMED`；列表端点任何一行重建门不过整页答 5xx（ADR-0314 越权风险点 5）。
- 这些行的恢复动作不是重试：同一个请求重发一万次仍是同一个答案。恢复在操作者手里——`待批准`的行可被同一版本号的重新录入替换（登记册的修订只在待批准期间替换行），`已批准`的内容已固定、只能换版本号。ADR-0022 的 5xx 意为「答案没形成，退避后重发」，ADR-0029 按调用方的恢复动作分格：这里答成 5xx，操作者看到的是技术失败，叫运维重试，而不是该做什么。同文件的受控发布用例对「客户合同只有壳」答`未受理`而不答 error（ADR-0178 决定一），判据相同。
- ADR-0314 把「一份坏行会让整页答不出」当作可接受的代价，理由是「本产品今天没有撤回或作废载体的口，要靠运维修数据」。对行本身坏了，这个理由仍然成立；对本构建折不回正文的行不成立——操作者自己就能把它换掉，却被一行挡住租户全部在途工作的列表，批准者连其余的载体也看不到。
- 一份读不回的载体到了`已发布`，权威记录早已是那一册里的商业版本，载体只是历史痕迹。重放批准或发布时，把它答成「出错」会让管理台把一版已在册的发布显成故障。

## Decision

**一、重建门分两格，读不回的一格交回库上的事实和成因。**（机制）

- 行本身对不上，仍是 `ErrInvalidRehydratedPublicationDraft`：状态不是本上下文的取值、录入者或时刻缺失、壳立不起来、状态与痕迹对不上、快照里的册与壳声明的类别不同、折回的正文规范化不了、折得回而摘要与列上不一致。处置是查库，上层按技术失败。
- 行没坏而快照折不回正文——文档的规范化版本本构建不认得，或文档里的正文过不了本构建的构造门——是 `ErrPublicationDraftNotReadable`，交回的错误是 `UnreadablePublicationDraft`：它同时认这个哨兵和折不回的成因（错误链保留 `ErrCanonicalizationUnsupported`、`ErrVolumetricFactorOnPricePolicy` 等，调用方不解析文字）。痕迹与壳先于正文检查，所以它交出的事实都是校过的：版本壳（含壳摘要）、状态与批准 / 发布痕迹、录入者与时刻、列上的规范化版本与内容摘要原值。它不带正文，交不出发布规格，推进不了状态。
- 载体册点读遇到它，错误链里带这个值；列表把它作为一行留在结果里（见决定四）。

**二、批准与发布遇到读不回的载体：摘要绑定先于状态，其后`已发布`答已发布，其余答 `DRAFT_NOT_READABLE`，一个字节不写，200。**（机制）

- 判序与读得回的载体一致（ADR-0314 决定二）：调用方带的内容摘要与壳摘要，与读不回的行此刻的列上摘要和壳摘要比，任一个不同答 `DRAFT_CHANGED`，先于一切状态答案。
- `已发布`答 `DRAFT_ALREADY_PUBLISHED`。它是终点，不需要正文就能如实答；答`读不回`会叫操作者去重录一个已在册的版本。
- `待批准`与`已批准`答 `DRAFT_NOT_READABLE`：批准口新增这一格，发布口新增同名一格。批准口不去读审批职责规则（读不回先于「未配置」）；发布口对`待批准`的读不回行也答这一格而不是「还没批准」——批准它同样要正文，答「还没批准」会把操作者引向一次必败的批准。`已批准`的读不回行不答`DRAFT_ALREADY_APPROVED`，那一句会引向一次必败的发布。
- 两口的答复带载体状态 `status` 与成因 `cause`，不带内容摘要、壳摘要与规范化版本（读不回的载体没有可批准的内容，摘要不能被原样交回去批）；发布口没有发布用例的答案。两口都是形成了的答案，取 200（ADR-0022）。
- 不复用发布口的`发布未落定`（`PUBLICATION_NOT_LANDED`）：那一格的恢复动作是照发布用例的答案续办，载体留在`已批准`；读不回的载体没有发布用例的答案可带，恢复动作是重录或换版本号（ADR-0029）。

**三、录入遇到读不回的既有行：已过`待批准`的答`内容已固定`，`待批准`的由重录替换。**（机制）

- 登记册的修订只在`待批准`且内容或壳与库上不同时替换行：读不回行的列上摘要不可能与本构建算出的相同，所以`待批准`的读不回行被同版本重录替换（答`已修订`），之后读得回、照常批准。
- 修订落空后读回既有行读不回：已不在`待批准`的，它不可能与这次录入是同一次录入，答`内容已固定`（`CONTENT_FIXED`，恢复动作同为换版本号，不另立一格）；仍在`待批准`的行让修订落空，只可能是摘要相同而文档折不回——行坏了，按错误交回，不装成内容已固定。

**四、读面：列表带着读不回的行，单份读回答 `DRAFT_NOT_READABLE`。**（机制）

- `GET /commercial-open-publication-drafts`：读不回的行留在列表里，位置按录入时刻不变；每份多一格 `readable`（读得回为真）；读不回的 `readable` 为假并带 `unreadableCause`，其余字段取自库上的列——`canonicalization` 与 `contentDigest` 是列上原值，可以是本构建不认得的规范化版本——不带正文。结果码仍是 `PUBLICATION_DRAFTS_LISTED`。行本身坏了与存储读失败仍让整次答案未形成、答 5xx，不交缺了行的列表（ADR-0314 决定一）。
- `GET /commercial-publication-draft-readbacks/{kind}/{objectId}/{version}`：载体在册而读不回，答 200 加 `DRAFT_NOT_READABLE`，`draft` 带上述事实与成因、没有 `content`——折不回的文档不能冒充摘要盖住的那一份。行本身坏了与存储读失败仍答 5xx。
- 已发布的读不回行不在列表里（列表只列`待批准`与`已批准`），单份读回仍答这一格。

**五、管理台把读不回做成可见的状态，不摆动作。**（管理台形状，不是产品规则）

- 待批准列表的状态旁标「本构建读不回」，悬停是成因，计数摘要另报读不回的份数，检索认得「读不回」。
- 详情抽屉照常复述事实，不给规范化文档，不摆批准与发布；用一段话说明为什么、重试不会好，并按状态说恢复动作：`待批准`的按同一版本号重新录入，`已批准`的换新的版本号，`已发布`的无需处理。批准与发布的答复栏补`DRAFT_NOT_READABLE`的中文，并显示答复带的状态与成因。

## 候选与反方

- **读不回的行照旧答 5xx，靠运维修数据。** 否决：重发不会好，ADR-0022 的 5xx 是「退避后重发」；恢复动作在操作者手里，却叫运维。更重的是一行读不回的载体挡住租户全部在途工作的列表。
- **列表悄悄跳过读不回的行。** 否决：批准者会以为列表就是全部（ADR-0314 自己的理由）。读不回的行带着事实留在列表里，既不挡页也不藏。
- **发布口复用`发布未落定`。** 否决：见决定二——恢复动作不同，且那一格的语义是「发布用例答了未决 / 冲突 / 未受理之一」。
- **`已发布`的读不回行也答 `DRAFT_NOT_READABLE`。** 否决：权威记录早已在册，答「出错」把一版已在册的发布显成故障；`已发布`不需要正文就能答，让重建门在只看状态的路径上也折正文，反而是多余的约束。
- **`已批准`的读不回行答「已批准」（只看状态）。** 否决：它会引向一次必败的发布，操作者拿到的是一个注定撞墙的下一步。
- **录入口新增一格「既有行折不回来」。** 否决：`内容已固定`的恢复动作（换版本号）与这一格相同，按恢复动作分格（ADR-0029）不另立。
- **读不回的行也交摘要，让调用方照常带回去批。** 否决：批的是调用方没法看到的内容；摘要在读不回的答复里只用作绑定的比较，不作为可被批准的凭据交出去。
- **读不回的行在列表里单列一个数组。** 否决：丢掉按录入时刻的统一顺序；一个数组加一格 `readable` 能让消费方在每一行上显式判，不把读不回的归进默认。
- **放宽重建门，让本构建不认得的版本也折回。** 否决：ADR-0014，不认得的版本既不能重算也不该猜。
- **撤回 / 作废载体的口。** 不在本记录：同一版本号重录已是待批准载体的替换口；已批准载体的内容固定是 ADR-0126 / ADR-0314 的既定取舍。

## 对已接受记录的作用

- **部分停用（收窄）** [ADR-0314](./0314-pending-publication-drafts-are-readable-back-by-the-operator-and-approval-and-publication-are-bound-to-the-content-digest-the-caller-saw.md) 越权风险点 5「列表里任何一行读不回（重建门不过）整次答案未形成、答 5xx，不交缺了行的列表；本产品今天没有撤回或作废载体的口，一份坏行会让整页答不出，要靠运维修数据——撤回口是日后的事，不在本记录。」一句。**适用场景**：仅限本构建折不回正文的行（规范化版本不认得，或正文过不了本构建的构造门）——它们作为读不回的一行留在列表里、单份读回答 `DRAFT_NOT_READABLE`，恢复动作在操作者手里（重录或换版本号），不需要撤回口；行本身坏了（重建门因痕迹、壳、册与摘要而拒）与存储读失败照原句，整次答案未形成、答 5xx。ADR-0314 其余各句不变。
- **不停用** ADR-0314 决定一「读失败答 5xx，不伪装成空列表」：存储读失败、行本身坏了照旧。**不停用** 决定二：摘要绑定按口解——读不回的行用列上的摘要与壳摘要比较，先于状态答案。
- 同步现行规则：`CONTEXT.md`「待批准发布」词条补读不回一格；UC-PC-001 的 `AT-PC-068` 的读失败一句按本记录改口径，新立 `AT-PC-071`（读不回的载体在读面与批准、发布上的答案）、`AT-PC-072`（录入撞上读不回行的落点）。

## Consequences

- `domain`：新增 `ErrPublicationDraftNotReadable` 与 `UnreadablePublicationDraft`；`RehydratePublicationDraft` 先校痕迹与壳、再折正文，折不回正文交回后者；壳摘要的算法落在 `CommercialVersion`，读得回的载体与读不回的行共用一份。
- `ports`：`PublicationDraftRegistry.LoadDraft` 合同补读不回的答法；`PublicationDraftRead.ListOpenDrafts` 改交 `[]OpenPublicationDraft`（每行是整份载体或读不回的事实，二者互斥）。
- `application`：批准结果多 `ApprovalDraftNotReadable`，发布结果多 `PublicationDraftNotReadable`，结果各带读不回的事实；摘要比较改为比两串，读得回与读不回的各自交出。
- `adapters/postgres`：列表遇读不回的行不再整次失败；录入撞键后的读回按状态答。
- `adapters/http`：载体响应多 `readable` 与 `unreadableCause`；批准与发布答复在读不回一格带 `status` 与 `cause`；读回端点多 `DRAFT_NOT_READABLE`。
- `cmd/parcel-api`：读口占位的签名随端口；端点表、路由与装配不动。
- 管理台：`pages/party` 的类型、答复栏中文、待批准页与抽屉；不动共享件。
- 无迁移：读不回的行就在 ADR-0126 的表里。

## 越权风险点

1. **对 ADR-0314 越权风险点 5 的收窄**（PC owner）。读不回的行不再让整页答 5xx，运维若靠「待批准列表 5xx」发现坏数据，读不回的这一类不再触发它；改看列表里 `readable:false` 的行与成因。行本身坏了与存储读失败仍答 5xx。
2. **分格边界按「折正文这一步」划**（PC owner）。凡 `RehydratePublicationContent` 拒绝（规范化版本不认得、文档形状不符、正文过不了构造门）都算读不回，其中真正的文档损坏但痕迹与摘要列都正常的行也被收进这一格而不再是 5xx；它们的恢复动作与本构建不认得的版本相同（重录或换版本号），所以接受，代价是 5xx 报警不再覆盖这一子集。折得回而摘要对不上、痕迹对不上、类别对不上仍是 5xx。
3. **`已批准`的读不回载体只给「换版本号」**（PC owner、运维）。滚动发布窗口里，旧构建实例读不回新构建写的`已批准`行，新构建铺满后它自己就读得回了；答复按状态给的动作是换版本号，在这个窗口里是过度建议。分辨「滚动窗口」与「永久读不回」要知道各构建实例认得的规范化版本集合，不在本记录。
4. **`已发布`的读不回行答已发布时不核对内容**（PC owner）。调用方带的两个摘要与列上的比，通过即答已发布；读不回的行里正文对不对没人看，但权威记录是那一册里的商业版本，载体只是痕迹。
5. **答复里的 `cause` 是领域错误原文**（PC owner）。含规范化版本串或体积系数的原值，是租户自己录入的内容，与`未受理`的 `cause` 同一可见范围；不是稳定的机器码，管理台只原样显示。若日后要按成因分流，另给封闭词表。

## Links

- [ADR-0314](./0314-pending-publication-drafts-are-readable-back-by-the-operator-and-approval-and-publication-are-bound-to-the-content-digest-the-caller-saw.md)：读口与摘要绑定；本记录收窄其越权风险点 5 一句，按口解其决定二
- [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)：待批准载体；读不回的行就在它的表里，不改它的正文
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：不认得的规范化版本既不能重算也不该猜
- [ADR-0260](./0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md)：体积系数是价卡内容，不在价格政策上声明
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：5xx 意为退避重发，读不回的载体重发不会好，答 200 加业务格
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- [ADR-0178](./0178-customer-contract-versions-carry-their-body-and-contract-shells-bind-service-rule-bodies.md)：受控发布对「客户合同只有壳」答`未受理`而不答 error，同一判据
