# ADR-0187：受控关闭与重开把请求证据与操作证据落在决定上——请求证据与请求方同在同缺，提交操作者只进操作证据一格；部分停用 ADR-0183 决定五首句与 ADR-0151 决定二受控关闭与重开一项的半句

Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide，簇 C5 终裁），代 PS owner 裁定，裁定原文落在票 [operator-channel/38](../../.scratch/operator-channel/issues/38-controlled-closure-and-reopening-record-request-and-operation-evidence.md)「裁定」一节；实施与成文由该会话起的实施子代理完成。裁决能力边界：读过 ADR-0183 全文（含两份复核记录与 owner 复核行）、ADR-0151 全文、ADR-0028 全文、ADR-0146 的 Status 与决定一至三、adr/README 的部分停用机制；PS CONTEXT「面单继续尝试决定」的定义、Rules 里受控关闭与重开各条、Boundaries 相关句，GLOSSARY 同名词条，SCENARIOS 面单场景；pc-gaps/13 的「裁决」与「要裁的」，label-channel/30 的「裁决」与两次评审；受控关闭与重开的领域、编排、HTTP 适配器与快照适配器全文。没读：UC-PS 各篇正文，ADR-0029、0030、0100、0139 正文，PILOT-PARAMETER-REGISTER 与开发主线。拿不准的列在文末「越权风险点」。）；**部分停用** ADR-0183 决定五首句与 ADR-0151 决定二受控关闭与重开一项「不进命令的任何一格」半句，适用场景见决定七

Date: 2026-10-19

## Context

**请求方在册上无据可查。** 受控关闭与重开的请求方由 ADR-0183 定形：只取包裹所属已接受委托的客户账户，写作 `CUSTOMER-ACCOUNT/<标识>`。可请求方凭什么提出这次关闭或重开，册上没有一格：货主重授权证据过了重开那道门就丢掉（`FormReopeningCommand` 头注原句「证据引用不落册——领域决定上没有它的格，它只在这一步作门」），关闭载荷连证据格都没有。决定方是 party-commercial 交回的运营角色，不是具体的人，查不到是谁代录了货主指令；主动拒绝与授权处置两口却把提交操作者记成了证据（`operatorEvidence`），两边不对称。

**冒用残余属实。** ADR-0183 Context 写过「货主的指令被冒用了两次」。研判在票 28 的代码上实测：货主指令关闭不要任何证据就能形成，重开写任意非空串就能过门。

**权威链指向 PS 保存证据，但必备项里没有它。**

- PS CONTEXT Boundaries 把「包裹级关闭或重开请求证据」「请求方与实际决定方证据」划给本上下文；
- SCENARIOS 面单场景写「`parcel-shipment` 先保存请求方、请求原因和依据」；
- [pc-gaps/13](../../.scratch/party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md) 裁决 ② 写明「具体请求与决定的证据归 PS（CONTEXT 原句）」，它所取候选 (a) 的代价句是「PS 存引用不判真伪」；
- 但 CONTEXT 定义与 Rules、PRODUCT-BASELINE 列举的必备项都没有证据。

所以本记录是对必备项的扩充：只增不减，与基线相容。

**提交操作者进决定，抵触两句已接受原文。** ADR-0183 决定五首句「两条命令没有提交操作者的格；本记录不给它们加格」；ADR-0151 决定二受控关闭与重开一项「提交操作者只作操作证据，不进命令的任何一格」。复核记录只加不停（ADR-0183 两份复核记录都注明「决定原文不动」），停用要由后续记录按 adr/README 的部分停用机制来做，先例是 ADR-0151 停用 ADR-0149 决定一的列举项。ADR-0183 的 owner 复核行已写明决定五首句走 supersede、由本记录部分停用。

**核验探针的结论。** 研判稿的严格重建门有两个后果：回滚再前滚后该包裹读不回册，面单交易建立、终局判断与交易列表一并失败；它配的守卫迁移要求「清空……重新形成」，违背 CONTEXT「不删除原关闭历史」「不得静默改写原决定历史」与禁止回填。研判稿把同在同缺核在签发标识之后，坏输入会消耗标识、还可能被答成`已有结果`。本记录的决定二、四据此改定。

这一格是机制加产品策略（ADR-0146）：证据格的形状、同在同缺的门、两扇门的宽严、快照键与回执格、证据不进重放比对是机制；操作者渠道上货主指令关闭必带证据、另两种来源不得带，是产品策略。证据的具体格式、操作证据的保存期限与查阅权限、谁被授予代录货主指令，是租户取值，本记录不设默认。

## Decision

**一、请求证据落在决定上，不另立请求册。** `面单继续尝试决定`多一格请求证据：请求方所凭依据的引用。货主指令关闭落货主那份指令的引用；重开落货主新的有效授权的引用（载荷 `shipperAuthorizationEvidence`，ADR-0183 决定四「由操作者如实记下」）。去掉两端空白后逐字保存，不解析、不判真伪、不加种类格。它是引用不是内容：工单号、来电记录号、邮件标识一类，不记依据原文，也不记联系方式。

**二、请求方与请求证据同在同缺：形成门两向都严，重建门只拒有证据无请求方。**

- 判据只在领域一处定义（`RequestEvidenceAccompaniesRequester`）。编排在问授权之前用它核命令，与缺项检查同一处；不合格答`输入未受理 · DECISION_INVALID`，不问 party-commercial、不签标识、不认重放。`Append` 再用它兜底。
- 重建门（`RehydrateContinuedAttemptRegister`）只拒「有请求证据、无请求方」——这个形状在哪一版规则下都不成立。「有请求方、无请求证据」照收：规则生效前形成的决定、回滚时旧版写回的决定，都是 ADR-0028 说的已经发生过、当时已经受过约束的事；同一聚合已有「重建门以结果自证一致，不逆推形成过程」的先例。
- 因此不加守卫迁移、不改 DDL。
- 接入层的真实形状不受影响：照 ADR-0183 决定四，重开没带证据时请求方缺席，这种命令照旧走到货主重授权那道门，答更具体的 `SHIPPER_REAUTHORIZATION_MISSING`；责任来源认不出照旧答 `RESPONSIBILITY_SOURCE_UNKNOWN`。

**三、提交操作者进且只进操作证据一格。** 操作者渠道上写作 `OPERATOR/<发行方>#<sub>`，与主动拒绝、授权处置的证据格同一写法（http 的 `operatorEvidence`）。它只进审计：不顶替决定方、授权角色、授权依据快照或请求方，任何门、任何比对都不读它。领域上可以缺（CONTEXT 原话是「可以作为操作证据」），操作者渠道一律填上。

**四、两份证据不进重放比对，回执带回请求证据。** 比对格以 ADR-0183「复核记录二」为准，不变：证据是请求所凭的依据，操作证据是经手人，都不是请求的内容；放进比对，两个人凭两份依据转达同一请求就会追加第二条同内容的关闭，那正是票 28 修掉的缺陷。重投答`已有结果`，交回带原证据的原决定，什么都不写。写面回执随决定带回册上的 `requestEvidence`，只加这一格（决定方、操作证据照旧不回）：`已有结果`什么都没写，调用方比一比这一格才看得出本次证据没落（ADR-0029 按恢复动作分格）。补正证据不是重投，今天没有更正口。

**五、货主指令关闭必带 `shipperInstructionEvidence`。** 关闭载荷加这一格：责任来源为 `SHIPPER-INSTRUCTION` 时必带，全空白与没带同读；`OPERATOR-ACTION`、`EXTERNAL-RESTRICTION` 带了即坏报文。两道都是纯载荷校验，排在按包裹寻址之前，答 `400`。重开载荷不变。

**六、真伪与冒用残余在 PS 内不该堵、也堵不住。** PS 做到的是请求有据可查、经手人可追；今天只能在库里查到，审计读面是后继（label-channel/10 那一族读面）。堵点在别处：货主经客户渠道自己提出（ADR-0139，归用户）；谁能代录货主指令，看运营决定的授予（租户取值）；是否从 `CONTROLLED_CLOSURE` 单分出「代录货主指令的关闭」一种授予，归角色模型 product-strategy-boundary/07（产品策略）；租户内控抽查。关闭不加双人门——CONTEXT 写明「关闭权限可以由授权运营角色单独使用」；重开若要双人控制，是 party-commercial 授权规则里的租户取值。被拒、未决、重放的尝试也要留痕的访问审计，归 ADR-0100 族另裁。

**七、部分停用两句，只限受控关闭口与重开口。**

- ADR-0183 决定五首句「两条命令没有提交操作者的格；本记录不给它们加格」；
- ADR-0151 决定二受控关闭与重开一项「提交操作者只作操作证据，不进命令的任何一格」中「不进命令的任何一格」半句（「只作操作证据」与同项其余各句不变）。

**适用场景**：只限受控关闭口 `/shipment-requests/continued-attempt-closures` 与重开口 `/shipment-requests/continued-attempt-reopenings`——停用理由是 PS CONTEXT 把操作证据写进这两种决定的定义、PS 拥有这两种决定的证据，这个前提不及别的口。两份记录其余各条不变；TF 各口要不要记提交操作者，仍归 TF owner（ADR-0151 越权风险点）。

## Consequences

- 快照文档每条决定加 `requestEvidence`、`operationEvidence` 两键（缺席不写）；无 DDL、无守卫迁移。今天生产与演示都是零行：生产的授权映射为 nil，两口停在`未决 · 授权口不可用`。
- 回滚只丢键、不卡包裹：旧版写回会丢这两键，留下审计缺口，重建门照收。授权映射接上之后，避免跨本版回滚。
- 线格式变化：关闭载荷加 `shipperInstructionEvidence`，回执加 `requestEvidence`。前端、种子与文档里今天都没有消费方。
- 编排的判据 `HasShipperAuthorizationEvidence` 改名为 `HasRequestEvidence`，关闭与重开共用。
- 管理台日后接这两口时：责任来源选「货主指令」须把证据格做成必填，提示填引用、不填联系方式；回执为`已有结果`时提示本次证据未写入。
- label-channel/30「裁决」② 的越权风险点「重开决定应否保存货主新授权证据引用（本票只核不存，PS owner）」与同票评审「偏离 ④」随之关闭。

## Alternatives

- **只核不存**（现状）：请求方无据可查，冒用不可追。否决。
- **只落请求证据**：决定方是运营角色不是人，追不到经手人。否决。
- **提交操作者记在接入通道审计**：accessidentity 没有审计册，与决定之间没有关联键，CONTEXT 又把操作证据写在决定的定义里。否决。
- **另立请求册**（候选丁）：未授权的请求、渠道方或代理商的请求要不要先留痕，归 PS owner；本记录不立。
- **证据进重放比对**：两个人凭两份依据转达同一请求会追加第二条同内容的关闭，重开票 28 修掉的缺陷。否决。
- **同内容、异证据答拒**：请求已经生效，拒了会让操作者以为关闭失败。否决。
- **证据带种类格**：格式是租户取值，PS 存引用不判真伪，种类格没有读者。否决。
- **重建门同严，加守卫迁移**：守卫要求「清空……重新形成」，违背 CONTEXT「不删除原关闭历史」「不得静默改写原决定历史」与禁止回填；严格重建门回滚后卡死包裹。否决。
- **重开证据不得等于原关闭请求证据**：同一张工单常常贯穿停与复，会误拒。改列为租户审计抽查项。否决。

## 越权风险点

1. **代 PS owner 改领域语言。** CONTEXT「面单继续尝试决定」的必备项扩到「请求方（如有）及其请求证据」，并定下「形成时同在同缺」。PS owner 复核时看两件事：货主口头指令拿不出单号时怎么如实记（建议记来电或工单引用）；货主指令关闭必带证据是否符合运营实际。
2. **部分停用两份已接受记录。** ADR-0183 决定五首句（另一会话当天代裁），ADR-0151 决定二受控关闭与重开一项半句（当初经用户「同意你的决定，开干」）。复核时看测试是否钉住：操作证据只进审计、没有任何门或比对读它、决定方与授权角色只取授权答复。
3. **重建门放宽一格。** 册上会存在「有请求方无请求证据」的决定，只可能来自规则生效前或回滚写回；日后审计读面要如实显示「未记请求证据」。PS owner 若要求重建门也严，须先给存量处置方案，且不得清空或重新形成。
4. **个人标识入册。** 操作证据指向具体操作者。保存期限与查阅权限属租户合规取值，本记录不设默认；要不要在参数登记册立一行，交登记册 owner。PS 已在册的主动拒绝与授权处置操作证据处境相同，不是本记录新开的一类。
5. **回执多带一格 `requestEvidence`。** 会把册上的证据引用交给第二个提交人，二者同租户、持同一种授予。若某租户把证据引用视为敏感，归租户合规另裁。
6. **第二次转达的依据不留痕。** 证据不进比对的代价；owner 若要求每次转达都留痕，那是另立请求册，不是本记录。
7. **没读的文档。** UC-PS 各篇、ADR-0029/0030/0100/0139 正文、PILOT-PARAMETER-REGISTER 与开发主线若另有关于请求证据或操作者留痕的说法，以原文为准，本记录让位。

## Links

- [ADR-0183：操作者渠道上受控关闭与重开按包裹寻址](./0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md)：部分停用其决定五首句；「复核记录二」的比较格不变，决定四补明证据不在其内
- [ADR-0151：未归族的命令口定族](./0151-unassigned-command-faces-get-their-families.md)：部分停用其决定二受控关闭与重开一项「不进命令的任何一格」半句
- [ADR-0028：聚合重建是另一扇门](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：形成门严、重建门照收旧历史的出处
- [ADR-0029：取回失败代数按恢复动作分格](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：回执带回请求证据的理由
- [ADR-0146：产品策略是机制与租户取值之间的第三类](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：三类判定
- [operator-channel/38](../../.scratch/operator-channel/issues/38-controlled-closure-and-reopening-record-request-and-operation-evidence.md)：裁定原文与实施
- [pc-gaps/13](../../.scratch/party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md)：裁决 ②「具体请求与决定的证据归 PS」
- [label-channel/30](../../.scratch/label-channel-service-first-release/issues/30-controlled-close-reopen-decision-write-face-ps-half.md)：重开那道门的出处，本记录关闭其「只核不存」的越权风险点
