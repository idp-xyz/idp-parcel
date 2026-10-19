# ADR-0202：运输委托、订舱与承运答复的入口族与协议核对——委托与订舱各口每口一种运营决定，承运答复走集成客户端，委托在提交时点核供应商协议并从协议快照采购责任法人

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/23](../../.scratch/demo-chain-gaps/issues/23-transport-commission-submission-face.md) 起草并落决定一（委托提交那一种）、三、四、六、七，决定二与决定五由票 24 落，决定一其余各种由票 24、25 落。越权风险点见文末，待 TF、access-identity owner 复核，PC owner 知会。）

Date: 2026-10-19

## Context

- UC-TF-004 要求「固定供应商协议、履约条件和成本来源快照」（步 2），再「建立运输委托并提交订舱申请」（步 3）。TF CONTEXT 生命周期写的是「运输委托草稿 → 已提交：固定请求方、服务提供方、运输范围、条件和供应商协议依据」。
- 委托、订舱与承运应答三本册子与编排早已在（迁移 `0003_commission_booking_answer.sql`，`application.CommissionTransportHandler`），但在 cmd 下零调用：没有命令面，运营决定也没有这几种。委托命令的协议、条件、角色、责任都是自由串，不核商业侧的供应商协议；演示种子里已发布的两份 SYN 供应商协议引不进来、核不了。
- `transport-fulfillment.transport-commission.submitted` 有生产方适配器，派发路由表里没有这个类型。ADR-0066 决定四把它裁为「缓」：「提交≠取得控制」，映成任何里程碑都不是 VE 单方能定的。编排与端口注释却写着意图「交 settlement-accounting 作成本预期源」，与 SA CONTEXT、ADR-0164（BUY 由发生项形成发起）不符。
- 票 22 已经开了 PC 的版本点读口与 TF 的协议核对适配器（接口形状与判法代决见该票完成记录），并把两件事留给本记录：检查次序与服务方比较放在消费侧适配器，与 ADR-0025「适配器只翻译，不判断」有张力；ADR-0038 的区间更正没有折进点读。
- 本批代决的单一权威是 [tf-chain-plan](../../.scratch/demo-chain-gaps/tf-chain-plan.md)「02 委托、订舱、承运答复」D02-1 至 D02-5、D02-9 与「跨票通则」X2、X3、X4；本记录把其中的裁定写成决定，计划此后只作索引。

## Decision

**一、委托与订舱各口每口一种运营决定（产品角色模型）。** 委托与订舱是运营人员在管理台上作出的生产决定，按 ADR-0151 的定族判据归操作者渠道的「运营决定」能力面，授予按决定种类：

- `TRANSPORT_COMMISSION_SUBMISSION`：提交运输委托（本票）。
- `BOOKING_REQUEST_SUBMISSION`：提交订舱（票 24）。
- `TRANSPORT_COMMISSION_CANCELLATION`、`BOOKING_REQUEST_CANCELLATION`：取消委托、取消订舱（票 25）。
- `FULFILLMENT_SEGMENT_COMMISSION_ADOPTION`：履约段采用运输委托（票 25；语义归 ADR-0204）。

授予的粒度等于权力的粒度：能提交委托的不因此能取消它，先例是受控关闭与重开分成两种（ADR-0151）。种类是产品的角色模型；谁被授予哪一种、准入对照与权威区间是租户取值，演示租户以 `SYN-` 登记。

**二、承运答复走集成客户端族（机制，票 24 落）。** 承运答复的提交方是承运系统，按 ADR-0151 的判据「看提交方是谁」归 ADR-0149 决定三的集成客户端族，新增外部事实类型 `CARRIER_BOOKING_ANSWER`。集成客户端绑定的来源身份必须等于该订舱所属委托的服务提供方，不等答不受理：一个承运商的客户端不能替另一个承运商答复。本条补进 ADR-0149 决定三的列举（不停用其中任何一条）。

**三、委托在提交时点核供应商协议（判法是产品策略，快照是机制）。**

- 登记方只指名一版供应商协议：协议对象与版本标签两格。快照引用与采购责任法人不收登记方给的，提交时从核过的那一版取：快照是 party-commercial 一处拼出的「对象/版本」指称（ADR-0080 决定七），法人取自协议正文。委托上新加采购责任法人一格，与协议、条件、角色、责任四格同样只在提交时固定。
- 三项检查按序：这一版在册（壳与正文都登了）；在提交时点支持新的采购决定（问 PC 自己的 `SupportsProcurementAt`）；协议的供应商就是本次的服务提供方。先不过的那一项定结论。
- 不过时分四格答业务结果（HTTP 200，ADR-0022）：`AGREEMENT_NOT_REGISTERED`、`AGREEMENT_NOT_EFFECTIVE`、`AGREEMENT_TERMINATED`、`PROVIDER_MISMATCH`。续办两两不同（去 PC 发布或补正文、换一版此刻生效的、取得新协议、改服务方），不并进`未受理`（ADR-0029）；服务方不符另带协议约定的供应商。四格都不落委托、不交意图。
- PC 读不动答`未决`（`SUPPLIER_AGREEMENT_UNAVAILABLE`），登记方稍后原样重交。PC 侧坏数据与读不动在值上分不开，命令面续办相同，不另分格。
- 核对口交回的 error 一律答`未决`（`SUPPLIER_AGREEMENT_UNAVAILABLE`），包括适配器译不了 PC 答复时包上的不可译哨兵（所答非所问、未知的生命周期位置、正文挂错版本）。那枚哨兵定义在适配器包里，编排认不出；PC 侧坏数据既已落未决，单把适配器侧的确定性违约挑出来，也换不来「未决就是重试会好」（风险点 10）。编排自己看得见的违约（交回封闭集外的格、在册格缺快照或法人）答没形成答案（HTTP 500）。
- 读口在事务里报 SQL 错误时事务已中止，照常提交只换回提交错误。装配在编排答未决时整笔回滚，再交回那一份未决，线上答的仍是本条写的格，不是 500。
- 「提交时点」取登记方给的提交时刻（ADR-0023 时间由提交方给），不取服务端时钟。
- 条件、角色、责任仍是登记方声明的快照引用，本记录不核它们，作为已知缺口记下（风险点 2），不做假核对。判法只用 PC 拥有的事实，先例是 PS 的 `agreementOf`。

**四、PC 版本点读口与 TF 协议核对适配器（机制，票 22 落）。**

- PC 读口 `SupplierAgreementVersionView.LoadSupplierAgreementVersion` 按（租户、协议对象、版本标签）答版本壳加正文：没有这一版两格都缺；壳不在已生效位置或已生效而正文没登时只交壳；已生效带正文时两者都交。读口不判可用与否。管理台的列表口是受限口，不借用。
- TF 端口 `SupplierAgreementSource` 与它的消费侧适配器把答复译成决定三的五格。正文缺席时按壳位置分格：已计划生效、已到期、已被替代答未生效；已退役答已终止；已生效而正文没登答未在册（供应商与法人都在正文里，没有正文就核不了）。服务方按同一串字面比，不比名称、不归一。
- **这一安排不是 ADR-0025「适配器只翻译，不判断」的例外。** 五格是 TF 在自己的 `ports` 里用自己的语言定义的封闭集，检查次序写在那份端口契约上；适配器做的是把 PC 的生命周期位置、`SupportsProcurementAt` 与参与方身份逐项译进这五格，且是全函数。「这一格怎么处置」——答哪种业务结果、写不写、交不交意图——仍在编排里。服务方比较是两个上下文同一参与方身份的相认，与同包其余适配器认承运主体同一种翻译。把比较挪进编排，委托提交与收费发生项登记（票 27）两处编排就要各抄一份次序与比较，次序这件事又得另找一处唯一定义；先例是 PS 的 `channel_selection_basis.go`。owner 若认定次序是判断，去处是 TF 领域里的一处判法、由两处编排各自调用，端口改答事实（风险点 6）。
- ADR-0038 的区间更正没有折进核对：判据只用正文区间与终止时刻，同 PS `agreementOf` 的先例。若 PC owner 裁定要折入，由 PC 在点读口一处交出更正后的区间，TF 不自己读更正（风险点 5）。

**五、订舱与承运答复的词表（票 24 落）。** 订舱必须引用一个在册的具体班次，不在册答 `SCHEDULE_NOT_FOUND`（UC-TF-004 步 1 校验运输机会）。承运答复的封闭集在接受、拒绝、失效、撤回之外加 `WAITLISTED`（候补）。`待确认`、`待补充`是订舱请求还没有答复时的读法，不是答复值。逐对象接受、委托级接受都不做，记为缺口。

**六、委托信封接上 Outbox、不登路由（机制）。** 「委托已提交」意图与委托同一笔入队，载荷只带（租户、委托）引用，信封 ID 照 `outboxintent.FingerprintEventID` 铸（口名前缀 `transport-commission`），分区键保留可读的「租户/委托/口名」（越限的残余见风险点 11）。派发路由表照 ADR-0066 决定四的「缓」不登记这个类型，派发如实答 `dispatch.no_subscriber`，烧完失败预算落 `ABANDONED`（ADR-0049 决定三）。入队不能省：后继消费者要的就是这份按委托认领的意图（ADR-0043），不入队等于把它丢掉。编排与端口注释里「交 SA 作成本预期源」改成现状：供应商成本的来源是运输收费发生项加 BUY 评价（ADR-0164、ADR-0208），不是委托。

**七、提交编排的次序（机制）。** 委托草稿过构造门 → 按（租户、委托）分重放与冲突 → 核协议 → 以核出的快照与法人提交 → 与意图同笔落库。

- 重放判在核协议之前：同一份委托原样重交答原委托，哪怕那一版协议此后已到期或终止；重放也不去问 PC。
- 内容摘要收登记方声明的那一半（协议对象与版本标签，不是快照与法人），所以不问 PC 就分得出重放与冲突。委托表在提交口接线之前没有写入方，摘要形状沿用 TFC-1 前缀；委托口没有无版本那一形可比，迁移 TF 0025 在表里有行时拒绝施加。
- 并发首交的输家照样按摘要分已有与冲突，不把冲突吞成「已有」。
- 声明不合法的在草稿门答 `INPUT_NOT_ACCEPTED`，不花一次跨上下文读。

三类：决定一的种类是产品角色模型（ADR-0146 的产品策略一侧），授予、对照与区间是租户取值；决定三的检查项、次序与分格是产品策略（内置一种判法），法人快照与零写入是机制；决定二、四、五、六、七是机制；各段由谁履约、外包段采用哪一版协议是租户取值（`PAR-NET-08`），演示租户的声明只写在演示文档里，证据只记 `S`。

## 候选与反方

- **委托、订舱、取消共用一种「运输委托」运营决定。** 能提交的就能取消，授予粒度大过权力粒度。否决。
- **承运答复走运营决定，由人转录。** 提交方是承运系统，不是运营人员（ADR-0151 判据）；转录口另记为缺口（风险点 1）。否决作为默认入口。
- **采购责任法人由登记方给，再与协议比对。** 多一处可以填错的格，比对不符也只能拒；协议正文本就写着法人。否决。
- **协议核对放进委托编排，端口只答 PC 的事实。** 见决定四末两段：两处编排各抄一份。否决，代价见风险点 6。
- **核协议在判重放之前。** 协议终止后原样重放会被答成已终止，PC 一停摆所有重放都成未决。否决。
- **委托信封不入队，等有消费者再接。** 丢掉后继消费者要的意图（ADR-0043）。否决，代价见风险点 4。
- **为了「先接上」给委托信封登一个消费者。** ADR-0066 已裁「缓」，接不住语义的消费者比不登记更糟（ADR-0049 决定三）。否决。
- **点读口按商业范围加载整本范围登记册再查。** 把 PC 的商业范围概念带进 TF，且每单加载整册。否决（PC owner 若拒绝开窄口，这是退路）。

## Consequences

- 代码（票 23）：access-identity 决定种类 `DecisionTransportCommissionSubmission`；TF 领域 `TransportCommissionDraft`（`DraftTransportCommission`、`Submit`）成为成立委托的唯一入口，`TransportCommission.LegalEntity`；编排 `CommissionTransportHandler.SubmitCommission` 多四个否定格、未决原因 `SUPPLIER_AGREEMENT_UNAVAILABLE`，`CommissionNotAccepted` 的线上名改为 `INPUT_NOT_ACCEPTED`（与 TF 其余运营决定口同名）；`CanonicalizeCommissionPayload` 改收协议对象与版本标签；委托口退役无版本摘要；委托信封 ID 改指纹，信封 ID 门禁例外清单剪掉这一行。
- 迁移：access-identity `0008`（决定种类约束重建，列各份重建的并集：迁移不按编号先后施加，在已施加 `0011` 的库上本份后施加，只列本份之前的全集会删掉后来的种类）；TF `0025`（委托加采购责任法人列，表里有行即拒绝施加）。
- 接口：`POST /transport-fulfillment-transport-commission-submissions`，挂操作者渠道的运营决定面；装配另立 `cmd/parcel-api/assemble_transport_commission.go`，整次提交一笔事务，编排答未决时整笔回滚。
- 演示种子：配置员授运输委托提交、准入对照与本产品权威区间各一行。
- 文档：UC-TF-004 补 `AT-TF-107` 至 `AT-TF-109`；ADR-0149 Links 加前向指针；演示文档补委托一节。

## 越权风险点

1. **没有人工转录口**（TF / access-identity owner）。靠邮件或电话答复的承运人，答复进不来。
2. **条件、角色、责任不核**（TF owner）。三格仍是登记方声明的引用，填错不会被挡。
3. **不核协议是否覆盖这段运输范围**（TF owner；知会 PC owner）。干线协议指给末端段，只要供应商对得上就提交得成。
4. **委托信封的红日志**（TF owner，作为 ADR-0066 的后继问题）。每封都会撞 `dispatch.no_subscriber` 直到 `ABANDONED`，演示日志里有红行。
5. **区间更正不折进核对**（PC owner）。PC 更正了壳的区间之后，TF 仍按正文区间判。
6. **检查次序与服务方比较放在适配器**（TF owner）。本记录裁定它不是 ADR-0025 的例外；owner 若认定是判断，按决定四末段的去处改。另：区间外而供应商也不对时只报未生效，登记方改完时点后会再撞一次服务方不符。
7. **「终止」对到版本退役**（PC owner）。PC 没有明文把供应商协议的终止对到版本生命周期的退役；「从未发布」与「发布时漏登正文」落同一格未在册。
8. **种类粒度**（psb/07）。五种决定是本记录按「授予粒度等于权力粒度」定的，种类数属于产品角色模型。
9. **提交时刻由登记方给**（TF owner）。核协议用的是登记方给的提交时刻，倒填到协议有效期内就能用一版已终止的协议提交。与 ADR-0023 同一取舍：服务端不代铸业务时间。
10. **核对口的确定性违约有一部分答未决**（TF owner；知会 PC owner）。适配器侧的不可译答复与 PC 侧坏数据都落 `SUPPLIER_AGREEMENT_UNAVAILABLE`，登记方照「稍后原样重交」只会原样再撞。要分开，先把适配器的不可译哨兵挪进 TF `ports`、由 PC 给坏数据一个哨兵，编排再按 `errors.Is` 改答没形成答案。
11. **分区键越限时同一笔不成立**（TF owner）。分区键与 Subject 随租户、委托标识变长；越过 `eventing.MaxPartitionKeyLength` 时信封在入队前被确定性拒收，事务不中止，委托落册而意图不在 Outbox，重放每次原样再撞。去处是票 [demo-chain-gaps/40](../../.scratch/demo-chain-gaps/issues/40-transport-commission-handoff-does-not-separate-deterministic-rejection.md)；先例是 SELL 评价请求交接口的确定性拒收哨兵。

## Links

- [ADR-0149：主链业务命令面按提交者分两族](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：决定二把承运答复补进其决定三的集成客户端列举
- [ADR-0151：未归族的命令口定族](./0151-unassigned-command-faces-get-their-families.md)：决定一的定族判据与「每口一种决定」的先例
- [ADR-0066：多载运对象信封在消费侧按成员循环拆分](./0066-multi-object-envelope-unrolls-per-member-on-the-consumer-side.md)：决定四把委托信封裁为「缓」，决定六照办
- [ADR-0025：跨上下文调用的适配器落在消费侧](./0025-cross-context-adapters-live-on-the-consumer-side.md)：决定四说明协议核对适配器为何不是它的例外
- [ADR-0038：有效性更正保留原版本与更正关系](./0038-validity-correction-keeps-original-and-relationship.md)：区间更正，决定四记为已知缺口
- [ADR-0043](./0043-publish-intent-claimed-by-result-identity.md)：发布意图由结果标识认领
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：四个否定格答 200 且逐格分开
- [ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md)：业务时间由提交方给，决定三的提交时点
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：没有订阅者显式失败并入账
- [ADR-0080](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)：决定七，「对象/版本」指称只由 PC 一处拼
- [ADR-0164](./0164-buy-evaluation-request-trigger-is-occurrence-formed.md)、[ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md)：供应商成本的来源是发生项，不是委托
- ADR-0204（票 25 起草，号已占）：履约段采用运输委托，决定一最后一种决定的语义
- [UC-TF-004](../application/transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md)：步 1 至 3，`AT-TF-107` 至 `AT-TF-109`
- 计划 `.scratch/demo-chain-gaps/tf-chain-plan.md`「02 委托、订舱、承运答复」；票 `.scratch/demo-chain-gaps/issues/22-…`、`23-…`、`24-…`、`25-…`
