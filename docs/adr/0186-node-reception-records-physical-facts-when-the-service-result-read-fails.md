# ADR-0186：节点收寄遇跨上下文读不动——服务结果读不动照常形成收寄并记「读不动」，身份核对读不动与本方落册失败不形成答案

Status: Accepted（2026-10-18。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话经隔离子代理研判、三份对抗核验（权威一致性、实现与回归、业务后果）与合成，代 NO owner 裁定，裁定原文落在票 [operator-channel/29](../../.scratch/operator-channel/issues/29-node-reception-records-physical-facts-when-cross-context-reads-fail.md)「裁定」一节，本记录落地后取代那一节。实施与成文由该会话起的实施子代理完成。裁决能力边界：读过 UC-NO-002 的结果语义契约与应用流程表、票 [operator-channel/25](../../.scratch/operator-channel/issues/25-node-reception-carries-parcel-shipment-service-result-markers.md) 的裁定与非作者评审、票 29 全文；代码读过 `ReceiveDeliveredUnitHandler`、NO 的 HTTP 收寄端点、前线导入的收寄子命令、NO 的收寄库 postgres 适配器、两处生产装配与 settlementaccounting 适配器的 SAVEPOINT 先例。**没读** NO `CONTEXT.md` 全文、ADR-0022/0023/0089 全文与 bento `WithinTransaction` 的源码；后者的行为以真库用例实测为准。）部分停用：决定二「身份核对读不动：不形成答案」与决定七「身份读口不隔离」两处由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定五停用，随票 no-reception-source/05 进 main 生效；被停的原句见本记录文末「复核记录（2026-10-19，ADR-0199 决定五部分停用两处）」与 adr/README 该行。

Date: 2026-10-18

## Context

票 operator-channel/25 让收寄在身份唯一成立时向 PS 取取消与终局、落成服务结果标记，并裁定读不动时「答`收寄待确认`、带续办引用、不落册」。非作者评审与票 29 的真库实测量到三处走不通：

- **2xx 让设备出队，而那一次什么都没落。** 读口报 Go 层错误时端点答 200 `RECEPTION_UNDECIDED`、零行、不交意图；ADR-0022 让设备收到 2xx 就出队，这一次接收从此在服务端没有记录。UC-NO-002 结果表里的`收寄判断待确认`以「物理事实已保存」为前提，零行的待确认与它对不上。
- **SQL 级读错误把收寄那笔事务一起作废。** PS 读跟收寄走同一笔事务，一条 SQL 级错误把事务打成中止态，提交时得到 rollback，端点答 500、零行；「读不动答待确认」在真库上只剩「不落册」一半。
- **NO 自身失败也答零行的 200 待确认。** 收寄库读写失败、摘要比对未知、版本签发失败、装配缺读口，都走同一支。输入缺陷两支（缺件、未知接收观察）同样答 200 待确认，而它们重发不会变好。

UC-NO-002 写明「即使包裹已经取消或终局，节点仍可形成真实接收与控制事实」：服务结果只限制后续方向性作业，不是收寄的前提。

## Decision

适用范围只管 UC-NO-002 收寄编排（`ReceiveDeliveredUnitHandler` 与它的两处生产装配），不管 no-directional-basis/01 的方向性读面与方向依据记录。

**一、身份唯一而服务结果读不动：照常`节点收寄已形成`。** 收寄、控制、关联、发布意图都照常，首答 201。收寄记录新增**服务结果核对态**，只有 INTAKE_FORMED 记录带，封闭取值「已查」（`CHECKED`）与「读不动」（`UNREADABLE`），类型化为 ports 的 `ServiceResultCheck`。「读不动」指读口交回错误、没取得 PS 的答复，不是「查过了，没有取消与终局」；读不动时标记必为空。不补查，也不发续办引用；重投按已有结果作答，核对态照读回。

**二、身份核对读不动：不形成答案。** 编排交回 error；HTTP 答 500 `NO_ANSWER_FORMED`、不带 `outcome`，前线导入记未决。不落册，不拿空候选顶替（那会把可能已知的包裹落成待识别）。目标形态「待识别 + 身份未核对」另立票 no-reception-source/01（本记录落地时尚未立）。

**三、NO 自身落不了册：同第二条。** 收寄库读写失败、已存摘要的形状认不出、并发读回失败、版本签发失败，都交回 error。装配缺依赖改在构造期拒。

**四、输入缺陷交回被拒类错误。** 缺租户、来源、节点、实物或发生时间，与接收观察不在三态之内，交回 `ErrReceptionCommandRejected`：HTTP 答 400 `MALFORMED_REQUEST`，前线导入记被拒。

**五、续办引用只给册上的记录。** 编排不再为没落册的尝试签发续办引用；结果与 HTTP 响应上的那一格随之删去，续办引用只留给意图入队未跟上的重发（AT-NO-027）。

**六、其余各支不变。** 拒收、仅扫描、零候选、多候选、身份唯一可读五支的答法不变，`outcome` 词表不增不减；核对态在 HTTP 响应与前线导入明细里是信息格（`serviceResultCheck`），INTAKE_FORMED 的首答与重放都带，不进 `outcome`。

**七、服务结果读口经 savepoint 隔离，身份读口不隔离。** 装饰器 `SavepointedServiceResultView` 在调用方事务里先开 savepoint，内层报错即退回 savepoint、把错误原样交回，成功即释放；ctx 不在事务里时直通。身份读口读不动本就不形成答案，整笔回滚正是要的结果。savepoint 与第一至五条同票落地，不单独先落：单独落下时，读不动会以零行的 200 待确认出现，正是 Context 第一条。

**八、标记只在 ports 一处合成，不立解析。** `ParcelServiceResults.Markers` 是唯一的合成入口，产出的串逐字不变；记录、存储、响应里仍存串。标记串只给人看与审计，程序消费方一律经 `ParcelServiceResultView` 现问，不得解析标记串。

## Consequences

- **用 savepoint 隔离跨上下文读，在本仓是新用法。** settlementaccounting 用 savepoint 吞本上下文自己的撞键，这里是第一次拿它隔开另一上下文的读。往后同事务读别家的编排，要么照这里包一层，要么按下一条把读挪出写路径。
- **结构上的备选是把读挪出写路径**，即 ADR-0081、ADR-0134 那种「写入事务只落本方事实，读与派生后置一拍」。本记录不取它：收寄的原子范围只有 UC-NO-002 步骤 8「原子提交收寄判断、控制、审计和对应发布意图」，PS 读不加锁，在 READ COMMITTED 下同事务读既无一致性收益也无代价，挪出去要多一个两段边界，收益只在隔离一项。
- **意图入队失败另立票**（operator-channel/45）。入队报 SQL 级错误时整笔回滚、答 5xx；「结果已提交而意图没跟上」只在入队以 Go 层错误失败时出现。本记录不改这一格，也不把 savepoint 扩到意图交付口。
- **兄弟编排同形，另立票**（票 29 为它记了 operator-channel/46，本记录落地时尚未立）。承接与集运编排里同样有「依赖读不动答零行待确认」的形状，本记录不改。
- **设备遇 5xx 逐条退避、不得队首阻塞**（依据 ADR-0023：事实身份与发生时间由设备签发，服务端不按接收顺序定序）。身份核对读不动现在答 5xx，设备留队重投；同一队里排在后面的事实不应被这一条卡住。
- 存储：`node_operations.reception` 加核对态一列（迁移 0006），封闭 CHECK、只有 INTAKE_FORMED 带、读不动则标记为空数组；既有 INTAKE_FORMED 行回填「已查」，推定依据写在迁移头注。

## Alternatives

- **原方案（a）：维持票 25 裁定**——读不动答`收寄待确认`、带续办引用、不落册，只把事务修到在真库上成立。不取：2xx 让设备出队，零行的待确认等于这一次接收没有发生，与「物理事实已保存」的结果定义相悖。
- **方案（b）：读不动时落一条`收寄待确认`记录**，保全物理事实但不形成收寄、不建控制，待补查后再形成。不取：服务结果不是收寄的前提（UC-NO-002「即使包裹已经取消或终局，节点仍可形成真实接收与控制事实」），把它当前提会让一次 PS 故障挡住真实的接收与控制；补查还要一条现在没有的续办通道。
- **身份核对与服务结果读不动两支都答 5xx。** 不取：服务结果读不动时身份已经唯一成立，物理事实与关联都已知，答 5xx 让设备反复重投一条本可落册的事实。
- **身份核对读不动即落待识别。** 不取：零候选是「查过了，查无此号」的业务答案，读不动拿它顶替会把可能已知的包裹落成待识别；目标形态见决定二。
- **把读挪出事务**（见 Consequences 第二条）。
- **服务结果读走独立连接。** 不取：要在装配点另拿一条连接、绕开 bento 的事务上下文，读与写各走各的连接池，失败面更大，换来的只是 savepoint 已给的隔离。
- **改 bento，让 `WithinTransaction` 容忍读错误。** 不取：框架的事务合同对所有上下文成立，为一处读口放宽它会让别处的真故障被吞。

## 复核记录（2026-10-19，决定原文不动）

1. **NO 收寄对缺席的事实身份与发生时刻答 400，与决定四「输入缺陷」是同一格**（票 [operator-channel/42](../../.scratch/operator-channel/issues/42-node-reception-keeps-answering-400-for-absent-identity-and-time.md) 裁定 (b)，会话 `idp-parcel-9b` 代决）：载荷缺 `sourceId` 或 `occurredAt`，接入层构造不出命令，答 400 `MALFORMED_REQUEST`，不交零值给编排。事实身份是判重键，由设备签发、服务端不代铸（ADR-0023），缺了它连同一事实的重放都认不出；发生时刻同由设备签发，服务端不按接收顺序定序，缺了也补不出。重发同样的内容不会变好，设备出队交人，正是决定四的判据。
2. **票 operator-channel/08「缺席交编排」只是 TF 作业事实口的分格，NO 收寄不适用。** 该票评审把「缺席交编排」记在 ADR-0023 名下，那一格在 TF 一侧成立：事实时刻缺席交编排，答 200 `SOURCE_NOT_ACCEPTED`。同一个一线作业端因此会在同一能力面上看到两种答法；按 ADR-0022 的表两者都让离线端出队交人，本记录不拉齐。
3. **不立跨上下文「缺席字段答哪一格」的统一规则。** 缺席答哪一格取决于各口的判重键与定序靠什么，逐口定。ADR-0124 决定三是反向先例：回放命令三样缺任何一样即 400 `MALFORMED_REQUEST`，统一成「缺席交编排」会推翻它。
4. **决定三「本方落册读写失败不形成答案」的适用面扩到 NO 的承接与集运编排**（票 [operator-channel/46](../../.scratch/operator-channel/issues/46-node-collaboration-and-consolidation-answer-no-answer-when-own-store-fails.md)）。两只编排原先在本方册读写失败时答 2xx「未决」、什么也没落，与收寄同形；今后同样交回 error（HTTP 5xx `NO_ANSWER_FORMED`，前线导入记未决、整行回滚），装配缺依赖在构造期拒。跨上下文依据读不动不在此列，仍按 no-directional-basis/01 的依据格记「读不动」。票 46 的真库用例另量到更重的一格：集运移入行在单元写下之后来源事实写失败，旧答法照常提交，单元里留下没有来源事实的成员，重跑会被读成重放而补不回——整行回滚正是为此。

## 复核记录（2026-10-19，ADR-0199 决定五部分停用两处；决定原文不动）

[ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定五（身份核对读不动翻成`待识别实物已建立`加身份核对态「未核对」）由票 [no-reception-source/05](../../.scratch/no-reception-source/issues/05-an-unreadable-identity-check-forms-a-pending-unit-marked-unchecked.md) 落地（会话 `idp-parcel-9b` 代 NO owner 裁定，授权同 Status 行）。翻转的四个前提由票 no-reception-source/02、03、04 与该票自带的「未核对处处可见」补齐。自该票进 main 起停用本记录两处：

1. **决定二「身份核对读不动：不形成答案」。** 被停的原句：「编排交回 error；HTTP 答 500 `NO_ANSWER_FORMED`、不带 `outcome`，前线导入记未决。不落册」。改为：落`待识别实物已建立`，收寄与控制照常，来源文档照存，候选为空，记身份核对态「未核对」（`UNCHECKED`）；不问服务结果、不交意图，同一笔自投一封重核门铃（ADR-0199 决定三的第三个触发）。HTTP 答 200 `PENDING_IDENTIFICATION`，信息格 `identityCheck` 为 `UNCHECKED`；前线导入记落地，明细写「身份未核对：候选缺席不代表查无此号」。同条「不拿空候选顶替」照旧成立，由核对态兑现：未核对与零候选的待识别在设备答复、前线导入明细、复核目录与管理台上分得开，`ParcelContainmentView` 对未核对在袋件答`不可归属`。同条末句「目标形态『待识别 + 身份未核对』另立票」所指的就是这次落地。
2. **决定七「身份读口不隔离」，连同它的理由「身份读口读不动本就不形成答案，整笔回滚正是要的结果」。** 身份读口改经 savepoint 隔离（`SavepointedParcelIdentityView`，与 `SavepointedServiceResultView` 同形），两处生产装配都包：读不动要落册，那条理由的前提不在了。同条服务结果读口那一半不变。

随决定二一并失效的还有 Consequences「设备遇 5xx 逐条退避」一条里「身份核对读不动现在答 5xx，设备留队重投」一句：翻转之后设备收到 2xx 就出队，续上身份靠服务端的重核门铃。同条逐条退避、不得队首阻塞的要求对其余 5xx 不变。

不变的：决定一（服务结果读不动）、决定三（本方落不了册不形成答案；自投门铃入不了队也归这一格）、决定四至六、决定八。Alternatives 里「身份核对读不动即落待识别。不取」一条照留：它否决的是拿零候选顶替，翻转后的形态带核对态，不是那一条。

改钉的用例：编排面 `TestDependencyFailuresFormNoAnswer` 去掉身份核对那一格，由 `TestAnUnreadableIdentityCheckFormsAPendingUnitMarkedUnchecked` 钉新答法；HTTP 面 `TestTransportProblemsCarryOnlyStableCodes` 里身份核对那一格改钉 200 未核对；生产装配面 `TestTheWiredReceptionFormsNoAnswerWhenIdentityIsUnreadable` 由 `TestTheWiredReceptionFormsAnUncheckedPendingUnitWhileIdentityIsUnreadable` 接替；前线导入面 `TestFrontlineImportLeavesTheReceivedRowUndecidedWhenIdentityIsUnreadable` 由 `TestFrontlineImportLandsTheReceivedRowUncheckedWhenIdentityIsUnreadable` 接替。

## Links

- 部分停用于 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定五（决定二、决定七各一处，见上一节）。
