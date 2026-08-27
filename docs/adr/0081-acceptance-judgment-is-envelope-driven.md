# ADR-0081: 接受判断由信封驱动——提交落库即交出「委托已提交」，推进落在派发一拍；提交事务随之改两段边界

Status: Accepted
Date: 2026-08-27

## Context

[commercial-closure-settlement-key/03](../../.scratch/commercial-closure-settlement-key/issues/03-acceptance-chain-has-no-assembly-point.md) 实读取证（锚 `9c95d7c`）：三个接受判断编排（`AdvanceAcceptanceJudgmentHandler`、`AdvanceFinancialControlJudgmentHandler`、`FormAcceptanceDecisionHandler`）在 `cmd/` 下唯一的构造点是一份测试夹具；SA 施加半边与 SA→PC 控制策略适配器零生产调用方；端点面没有接受判断入口。此前两张票写的「装上适配器到接受链」都建立在一条不存在的链上。

要裁的一问：提交落库之后，接受判断链由什么推进——

- **路 A · HTTP 命令端点**：调用方显式请求推进一次，与提交/撤回/取消三条命令面同形；
- **路 B · 信封驱动**：提交落库即交出「委托已提交」意图，`cmd/parcel-dispatch` 消费门推进，与既有十二类消费者同形。

这不只是装配位置：两条路的失败面与重试语义不同（前者调用方重发，后者 inbox/outbox 续办），属难逆转产品行为，按 AGENTS 走 ADR。

证据面记于[前置裁决简报](../../.scratch/commercial-closure-settlement-key/acceptance-drive-decision-brief.md)（`2cc7545`），两条判定性事实：其一，用例词已定性——UC-PS-001 步骤 8「适用硬规则和所需判断全部通过时**自动接受**」、`AT-PS-033`「不等待无依据的人工审批」、`AT-PS-008` 未决「通过独立接受判断任务**安全续办**」；其二，机制半边的半成品全躺在信封侧——出站意图端口与 Outbox 适配器（事件类型 `parcel-shipment.shipment-request.submitted`）已实现、有真库用例、零生产调用方，消费门四条保证与未决哨兵机制在十二类消费者上全是先例。简报呈报后，用户 2026-08-27 队列答复指示按此协同推进：并行会话落入站半边（`e3fdcff` / `5fce8ab`），本会话落出站半边与本记录。

## Decision

**一、接受判断是被触发的，不是被请求的。** 驱动信封即「委托已提交」`parcel-shipment.shipment-request.submitted`（首个消费者切片简报「事件信封基线」既定形状），不新增事件类型，不在端点面开判断入口。路 A 的第一件事就是发明一个 UC 里不存在的请求者——「谁来调它」会成为新的实例半边缺口，而且 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 的端点面语义是「租户接入渠道的客户动作」，接受判断不是客户动作，混进去会让「渠道未配置」这句话对它失真。

本裁决不动人工复核与主动拒绝的命令面：那是「规则显式要求人工」时**由人请求**的另一条入口，与自动链的驱动是两件事，混裁会把人工兜底升格成默认路径。

**二、同步半边与事件半边的分工一并裁死**（相邻勘察 survey 第 6 条欠的那句明文）：提交编排**不做**接受判断——UC-PS-001 的排除项「不执行接受条件判断」维持原样，HTTP 答复停在`已提交 / 尚未决定`；接受链整段（逐成员可达性 → 整份委托财务控制 → 形成决定）挂「委托已提交」的消费者；`reachability-judgment.formed` 等中间信封**不另立第二条推进路径**——同一份接受判断任务只有一处驱动。

**三、一个消费者按编排顺序推进，不拆三类信封接力。** 任一步未决即整笔回滚等重投；中间态本就落在接受判断任务里，拆出中间事件类型等于给同一件事立第二份状态。消费门的失败分格按 [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md) 第三条的既有纪律：链的未决哨兵（`psinbox.ErrAcceptanceChainUndecided`）在路由条目处翻译成 `dispatch.consumer_undecided`；封闭集合外、装配缺件、空成员清单三格保持 `publish_failed` 响亮——它们重投不自愈，折进未决会重投到失败预算耗尽。

**四、出站半边落在生产提交装配点，事务边界随之从整段壳换成两段**（简报「事务边界」；`cmd/parcel-api/assemble_submission.go`）：

- `preservationBoundary` 给来源保全的每笔写入各开一个事务——保全一经提交就不随后续步骤回滚；
- `submissionBoundary` 携 `OutboxShipmentRequestSubmittedHandoff`——建单与「委托已提交」信封在同一个事务里成立或一起消失；已存在时本事务没写下任何东西，不入队第二份意图。

先前的整段 Handle 单事务壳（`transactionalSubmission`）退役：它让编排报错时把已保全的来源一并回滚，而 UC-PS-001 步骤 2 明写「后续解析或依赖失败不能删除该记录」。`tests/bentocontract` 的 PBC-04/05/07 在真库上取证的正是这两条边界——生产装配必须与被证明的形状同形，而不是各拍各的。

**五、编排签名不动，信封交接是装配层的修饰，不是应用层的新依赖。** `SubmitShipmentRequestHandler` 仍是五参：交接住在 `submissionBoundary` 对仓储口的包装里，应用层照旧不知道事件机制（[ADR-0017](./0017-admission-gates-judged-by-blocking-cause.md) 的 Bento 闸门语义——事务与 Outbox 不进应用层——在接线之后依然成立）。

**六、入站半边的实例半边一格不填。** 时点取值源、可达性闭包标识、结算账户目录、控制金额源全部留 nil，各按「显式未配置」停下（时点停`未配置`、控制停 `CONTROL_SCOPE_NOT_CONFIGURED` / `CONTROL_AMOUNT_NOT_CONFIGURED`）。解析键登记面反而接真：它是本上下文自己的登记表，空册按「显式未配置」答`解析未决`——接上它与留 nil 的区别不在结果在来源，恢复动作从「写代码」变成「登记参数」（[ADR-0063](./0063-intake-qualification-proof-is-a-consumer-side-evidence-port.md) 的判据）。

## Consequences

- **提交的同步答复从此到`已提交 / 尚未决定`为止，接受在下一拍。** 调用方要接受结果得查询或等通知——这不是缺口，是 UC-PS-001 本来的形状。
- **`wireDispatcher` 的依赖面再宽一格**（ADR-0049 认下的那笔代价），且这是本进程第一条发布侧与消费侧同属 parcel-shipment 的自发自收链——提交事务只把意图落进 outbox，推进判断是下一拍的事。
- **生产提交从此每单在建单事务里落恰好一份信封。** 原子性与信封内容由 PBC-03/04/05 取证；`cmd/parcel-api` 的装配测试另证生产边界壳真的把信封接上了（重放不入队第二份）——夹具副本证不了生产类型这一层。
- **在墙一（接入渠道）与墙二（治理坐标）拆掉之前，生产 HTTP 路径造不出信封**：提交停在 `ACCESS_CHANNEL_NOT_CONFIGURED` 或 `OWNERSHIP_UNRESOLVED`，链因此不动。全链验证走合成路径与种子租户（与票 02/03 的完成标准同路）；这不是本裁决的欠账，是那两堵墙各自票面的事。
- **被拒提交的来源保全从此独立存活。** 整段壳时代「编排报错则保全一并回滚」的行为消失——重放一份曾被业务拒绝的输入答`已有结果`，靠的是首笔保全事务真的提交了。
- **三个编排的第一次生产装配点在 `cmd/parcel-dispatch` 成立**（`e3fdcff` / `5fce8ab`）：接受链依赖图接真库，空库时链停在商业解析`未决`且失败码是 `dispatch.consumer_undecided`——运维照它去补登记，而不是去查一个不存在的传输故障。

## Alternatives considered

- **路 A · HTTP 命令端点。** 否决：与 UC-PS-001 步骤 8 / `AT-PS-033` 的「自动接受」正面冲突——裁 A 等于在裁决之前先改用例；「谁来调它」成为新的实例半边问题；未决的续办责任被推给一个不存在的调用方，而 `AT-PS-008`/`BD-PS-001` 要的系统续办正是 inbox/outbox 的既有语义。
- **三类信封接力（拆出可达性已推进、控制已推进等中间事件）。** 否决：中间态已落在接受判断任务里，第二处状态从出生起就得与第一处对账；每多一类信封多一行路由与一份哨兵名单，买回来的只是把一个消费者的顺序拆进派发器。
- **提交编排内同步判接受。** 否决：UC-PS-001 排除项明写提交不执行接受条件判断；同步判会把接受链的全部未决面（时点、可达性、控制、复核）搬进提交的 HTTP 答复，提交事务也被拉长到跨上下文调用之上。
- **编排签名加 Downstream 参数，由编排自己交信封。** 否决：事件机制不进应用层（ADR-0017）；装配层修饰器达成同一效果，还免去并行会话跟着改签名。
- **HTTP 与信封双轨并存。** 否决：同一份判断任务两处驱动，幂等靠双方约定而不是靠结构；失败面也从两格变四格。

## Links

- [票 03：接受链没有装配点也没有进程入口](../../.scratch/commercial-closure-settlement-key/issues/03-acceptance-chain-has-no-assembly-point.md)：事实链与「要先裁的一件事」
- [接受判断驱动方式前置裁决简报](../../.scratch/commercial-closure-settlement-key/acceptance-drive-decision-brief.md)（`2cc7545`）：两路逐维对比与建议，本记录的 Context 素材
- [首个消费者切片简报](../design/parcel-go-first-consumer-slice-decision-brief.md)：「事务边界」两段拍与「事件信封基线」的出处
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：步骤 2（保全存活）、步骤 8（自动接受）、排除项（提交不判接受）
- [ADR-0049：发布通道在装载证据之前是进程内投递](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：路由登记与未决哨兵纪律，本记录消费门失败分格的依据
- [ADR-0055：业务端点 Intake 增设「未配置」格](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：端点面的语义边界，路 A 否决理由之一
- [ADR-0017：准入闸门按阻塞原因裁判](./0017-admission-gates-judged-by-blocking-cause.md)：事务与 Outbox 不进应用层，Decision 五的依据
- [ADR-0063：收寄硬资格证明由消费侧窄口取证](./0063-intake-qualification-proof-is-a-consumer-side-evidence-port.md)：「显式未配置」与恢复动作分界，Decision 六的判据
