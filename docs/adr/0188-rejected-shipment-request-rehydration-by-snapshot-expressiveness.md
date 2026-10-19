# ADR-0188: 已拒绝委托的重建门按快照表达能力开门

Status: Accepted（2026-10-18，票 [operator-channel/33](../../.scratch/operator-channel/issues/33-rejected-shipment-requests-persist-and-read-back.md)。会话 `idp-parcel-9b` 在用户 2026-10-16「你作为业务和系统专家你来自决」的授权下代 PS owner 裁定。）

Date: 2026-10-18

## Context

[ADR-0061](./0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md) 把重建门开到`已接受`，同时写明`已拒绝`仍不开：快照没有主动拒绝的表达（ADR-0061 决定二的表格：「`withdrawal` / 主动拒绝 | **无**」）。

那张表只挡住了读。写侧照常落`已拒绝`行，于是出了两件事：

- **留痕在落库那一刻丢了。** 快照的决定里只有决定标识、`accepted:false`、空依据和决定时刻。主动拒绝与授权处置的`拒绝`去向留下的授权引用、实际决定方、结构化原因、证据都没有字段可写，静默丢失。PS CONTEXT 要求主动拒绝保存这几格，[ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定一要求处置`拒绝`的留痕「与主动拒绝同一份要求」。
- **读回走不到门。** 依据解码要求所有决定都带非空的商业解析标识，而不带依据的拒绝是常态：主动拒绝、处置`拒绝`，以及`没有适用合同`形成的规则拒绝（`ShipmentRequest.Decide`：「`没有适用合同`形成的拒绝恰恰带不出依据」）。这些行读回时在依据解码处报裸错，`ErrRehydrationStateNotSupported` 与 `ErrInvalidRehydratedShipmentRequest` 两个哨兵都不中，绕开了 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 的分格。

凡是按来源身份读委托的编排都受影响：主动拒绝口的重放答未决并每次追加一条处理尝试；处置`拒绝`的重放与复核完成撞上已拒绝委托都答 500；客户原样重放一份已被拒绝的提交答 500 而不是`既有结果`；`AT-PS-036` 以「已拒绝修正」关联原委托时读不回原委托；排队中的续办信封撞上已被主动拒绝的委托，交回`委托不可用`而走不到「既有决定」那一支。替身仓储上同样的重放都答对了：领域代数是全的，断在持久化。

票 33 的第一笔已经补齐表达：快照的决定上有了主动拒绝留痕（`RehydrateActiveRejectionSpec`），依据缺席按拒绝决定放宽。按 [ADR-0030](./0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md) 的两步顺序，先补表达再放出门，本记录回答`已拒绝`这一格现在开不开。

## Decision

**一、重建门开到`已提交`、`已接受`与`已拒绝`。** `已撤回`仍由入口以 `ErrRehydrationStateNotSupported` 拒绝：撤回在快照里还没有表达，撤回口属客户渠道，另立票。

**二、`已拒绝`三条来路的对账表。** 重建只校验、不重算（[ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)）。下表逐格写出每条来路在场什么、缺席什么，`validForRehydration` 照表校验。

| 聚合字段 | 规则拒绝（`Decide`，有失败项） | 主动拒绝（`RejectByAuthority`） | 处置`拒绝`（`DisposeUnderAuthority`） |
|---|---|---|---|
| 决定标识 | 在场 | 在场 | 在场 |
| 接受 | 否 | 否 | 否 |
| 校验明细 | 在场，至少一项`失败` | 缺席 | 缺席 |
| 商业依据 | 可缺席（`没有适用合同`）或在场 | 可缺席或在场 | 缺席 |
| 复核状态 | 等于规则包声明与任务完成事实的合成；依据缺席时为未设 | 未设 | 未设 |
| 决定时刻 | 在场 | 在场 | 在场，等于处置时点 |
| 主动拒绝留痕 | 缺席 | 四格齐全 | 四格齐全，与任务处置记录逐格对得上：授权引用即处置授权引用，决定方即处置方，原因即处置原因，证据即处置证据 |
| `decisionFormed` | 真 | 真 | 真 |
| 任务状态 | `已完成` | `已完成` | `已完成` |
| 等待态 | 缺席 | 缺席 | 缺席 |
| 任务处置记录 | 缺席或`交客户补充` | 缺席或`交客户补充` | `拒绝` |
| 处理记录、复核完成度 | 照收 | 照收 | 照收 |
| 接受基线、预计承诺、客户资料版本、撤回 | 缺席 | 缺席 | 缺席 |
| 历史版本与任务、关联出处、客户包裹件号 | 与`已提交`同样收、同样校验 | 同左 | 同左 |

**三、坏数据判据。** 照 ADR-0061 决定三：门已经开到`已拒绝`，下列形状是这一行立不起来，走 `ErrInvalidRehydratedShipmentRequest`，不折成「本期不支持」。

1. 一个拒绝决定必须**恰好**带「失败校验项」与「主动拒绝留痕」中的一样。两样都没有、两样都有，都是坏数据。
2. 带接受基线、预计承诺或客户资料版本。
3. 没越过决定边界、没有决定、决定是接受、没有决定时刻；任务未`已完成`或仍在等待。
4. 留痕不齐（四格缺任一格）。主动拒绝带校验明细或复核状态。规则拒绝的复核状态与合成结果对不上。
5. 任务处置记录为`拒绝`，而决定不是与之逐格相等的主动拒绝；或者这样的决定带了商业依据、决定时刻不等于处置时点。
6. **补表达之前写下的主动拒绝行与处置`拒绝`行**没有留痕、也没有失败项，落第 1 条，按坏数据拒绝。不从任务处置记录推回留痕：ADR-0028 定的是只校验不重算，而且主动拒绝那条来路根本没有处置记录可推。这类行只可能出自带合成授权的一次性测试库，生产装配上拒绝授权与处置授权从来没接上过（`buildRejectionOrchestration` 的请求映射为 nil，`buildDispositionOrchestration` 装的是 `UnconfiguredAuthorizedDispositionAuthorizer`）。

**四、文档处理照 ADR-0030 → ADR-0061 的先例写成「推进」。** 本记录推进 ADR-0061 决定一里`已拒绝`那一半；ADR-0061 的 Links 加前向指针，Status 与其余各条不变；ADR-0030 的 Links 加一条推进指针。不在 ADR-0061 后面追加复核记录来开门：复核记录只用来记认可，或者指向改动它的新记录（先例是 ADR-0125 的 owner 复核记录「本正文不改，以 ADR-0132 决定三为准」）。

## Consequences

- `ShipmentRequestRepository.FindBySourceIdentity` 对已拒绝行交回带决定与留痕的聚合。各编排本来就有的分支从此走得到：提交口重放答`既有结果`，`AT-PS-036` 的关联成立，处置`拒绝`重放答`任务已完结`并交回那一个决定，续办信封撞上已拒绝委托时交回既有决定。
- 主动拒绝口、处置口撞上已有决定时答哪一格、重放耗不耗标识，不在本记录，归票 operator-channel/36。
- 重建面新增一个受限标识 `RehydrateActiveRejectionSpec`。已接受决定带留痕同样是坏数据。
- PS CONTEXT 主动拒绝那一句里的「适用范围」，领域 `ActiveRejection` 没有这一格（`AT-PS-034` 也写了「记录结构化原因、范围和依据」）。这是已有的领域缺口，交 PS owner。本记录只按领域可达字段对账，不宣称满足了那一整句。
- `已撤回`仍会让运维看到「等这扇门开到那个状态」。

## Alternatives considered

- **依据缺席只对主动拒绝放宽。** 否决：`没有适用合同`形成的规则拒绝同样带不出依据，它会留在裸错上。放宽的分界是拒绝决定，不是主动拒绝。
- **在 ADR-0061 后面追加复核记录来开门。** 否决，理由见决定四。
- **旧形状行从任务处置记录推回留痕。** 否决：违反 ADR-0028 的只校验不重算；主动拒绝来路没有处置记录可推，推得回来的只有一半；这类行在生产上不存在。
- **`已拒绝`与`已撤回`一起开。** 否决：撤回仍无表达。先开门再补字段，正是 ADR-0030 要修的「宣称四个、其实一个」。
- **只修读侧，写侧留痕以后再补。** 否决：生产装配接上拒绝映射与处置授权之后，第一条真实主动拒绝的留痕就会丢，而且补不回来。

## Links

- [ADR-0061：已接受委托的重建门按快照表达能力开门](./0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md)：本记录推进它决定一里`已拒绝`那一半；它的 Status 与其余各条不变
- [ADR-0030：聚合重建按状态逐个开门](./0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md)：判据与两步顺序不变，本记录照它先补表达、再开门
- [ADR-0028：聚合的重建与构造分属两扇门](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：只校验不重算，旧形状行据此不推回
- [ADR-0029：按标识取回原状态失败时按恢复动作分格](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：完好的行不得报成两个哨兵都不中的裸错
- [ADR-0132：授权处置](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md)：处置`拒绝`的留痕要求与主动拒绝相同；撞上已成立的决定答`任务已完结`
