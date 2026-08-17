# ADR-0055: 业务端点 Intake 增设「未配置」格，端点装配属机制半边

Status: Accepted  
Date: 2026-08-17

## Context

进程实际服务的 HTTP 面只有 `/healthz` 与 `/version`。七个接入面处理器早已在五个上下文的 `adapters/http` 成型并有传输层测试，但 `cmd/parcel-api` 的装配点 `assembleBusinessEndpoints` 交回空清单——每个处理器的构造函数都以 Intake 接口为第一参，来源信封只能来自认证结果，而真实接入渠道的认证方式属 `PAR-INT-01` 待提供。[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)第 24 轮据此判「缺口收敛为逐端点等 Intake，不计机制缺口」；[评审 081701](../review/081701.md) 第 2 条则记 P1「业务 API 实际不可用」。两个口径顶在一起，需要裁定。

**空清单把两件事折成了一件。** 从进程外看，「产品没有这个能力」与「租户还没配置接入渠道」同答 404，不可分辨，而两者的恢复动作不同：前者无事可做，后者要去提供接入渠道参数。同款折叠病本仓治过两次——[ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md) 治「空证据或 error 顶网络未登记」，[ADR-0054](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md) 治「零值策略顶策略未登记」——病理一字不差地成立于此：把未配置折进另一格，读的人会把恢复动作指错。

**判据是开发主线自己立的。** 横切缺口节写着「视图的**内容**属实例半边，视图的**实现**不是」。同构套用：接入渠道的行（哪种渠道、什么认证方式、客户业务标识、重试与冲突边界）属实例半边，第 24 轮那句「认证方式属 `PAR-INT-01` 待提供」仍然对，本记录不推翻；但「端点已装配、配置为空时如实答未配置」是实现，属机制半边。

**红线核对。** 装配点与各 Intake 注释禁的是「开发用」的采信头部实现——替租户拟一种认证方式、从请求内容铸造来源信封，那会穿透 [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) 的最高数据隔离边界。未配置即拒恰是其反面：不读业务内容、不铸信封、不出命令。ADR-0052 的分界句同款成立：「读一个空登记册并如实答未配置不是默认实现，恰恰是它想保护的东西」。这里的登记册就是装配点本身——真渠道就位前它没有任何一行，未配置 Intake 是空册的如实答复。

**[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 的一句要随之停用。** 它写着「两项未决期间，业务端点可以存在并由测试替身驱动，但不进 `cmd/parcel-api` 的装配」。那句成立于当时：未配置格尚未发明，进装配的唯一路径是某种默认实现，禁装配是禁默认实现的手段。ADR-0052 之后手段与目的可以分开——目的（不落默认实现、不铸信封）由未配置格保留并加强，手段（禁装配）产生的折叠代价已被评审记成 P1。

## Decision

**一、业务端点不再等接入渠道参数才进装配：七个端点各以「未配置即拒」的 Intake 起步。** 该实现对每个请求不读业务内容、不采信任何自报身份、不构造命令，一律如实答「接入渠道未配置」。这一格住在 Intake 缝里（各处理器的第一参），不在路由层另设闸——路由层不知道渠道这个概念，另设闸就是同一问题的第二处权威。真渠道 Intake 就位时在装配点逐端点替换，路由层与处理器不动，第 24 轮「Intake 就位时在装配点追加一行」的缝形原样保留。[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) Decision 中「但不进 `cmd/parcel-api` 的装配」一句据此停用；同段「禁止在两项未决前落地任何默认实现」一句原文有效，本记录靠它划界而不是绕开它。

**二、未配置自成一格，不并入既有两格。** 传输层新增稳定错误码 `ACCESS_CHANNEL_NOT_CONFIGURED`，各 `adapters/http` 包新增对应哨兵错误（形如 `ErrAccessChannelNotConfigured`），与 `MALFORMED_REQUEST`（调用方改请求）、`INTAKE_FAILED`（运维救依赖）并列。分格判据同 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：恢复动作不同——这一格要的是接入方去提供并配置渠道参数（客户委托面即 `PAR-INT-01`，外部结果面即 `PAR-INT-03`，各端点所等参数以[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)为准；错误码命名状态，不命名参数）。

**三、状态码取 `403`，不取 404、401 或任何 5xx。** [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 的三类表以离线客户端动作为契约：4xx 出队交人、5xx 留队重发。渠道未配置时重发一件人不来配就永远不会好的事，必须出队交人，故 4xx；「重发不会改变结果」在此严格成立，出队后交给的那个人要做的正是配置。404 是本记录要治的折叠；401 邀请调用方换凭证重试，而此刻不存在任何凭证方案；503 会被通用重试库读成「过会儿就好」。403 的一般语义「已理解、拒绝处理」最贴近「当前没有任何已启用的准入路径」。答复对一切请求内容与自报身份一致，不因内容不同而变；按 ADR-0022，4xx 不携带 `outcome`。

**四、与 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)「越权探针与真不存在同答」的关系写明：该条保护租户域资源的存在性，不保护产品能力目录。** 未配置格只披露「本产品有此端点、渠道未配置」，那是产品表面；租户数为零时也没有任何客户数据可泄。业务资源层面的探针同答约束在真渠道 Intake 就位后照常适用，本记录不触碰。

**五、本记录不解锁 Intake 的另两项未决。** 载荷规范化摘要与准入范围装配（`PAR-GOV-03..07`）仍然拦着真渠道 Intake；未配置即拒绕开它们只因它走不到那一步。真渠道落地时它们必须先行或同批。

## Consequences

- 进程从「看起来没有 API」变为可运行、可观察：七端点上线，全部如实答未配置。评审第 2 条由「基线不计缺口 vs 评审记 P1」收敛为「机制半边已认领」；首发路径稳定停在**未配置**，与 ADR-0052 的结果同款。
- 每个 `adapters/http` 包各加一个哨兵错误、一条 403 映射与一个未配置 Intake 实现，传输层测试各补「未配置即拒、不读内容、答复一致」；装配点从空清单改为七行。
- 实现顺序不因本记录提前：按评审 081701 的排序，先修数据约束与并发缺陷、再完成最小产品版本正文及持久化，业务端点装配随「接通 Dispatcher、核心消费者和最小业务 API」那一步落地。本记录定的是范围归属与形状，不是插队令。
- ADR-0022 的 Status 行与 Links 节按 [adr/README](./README.md) 的部分停用机制加前向指针；被停的只有「但不进 `cmd/parcel-api` 的装配」一句，其余各条（三类表、`outcome` 词汇表、不新造幂等键、`!found` 映射、禁默认实现）不变。
- `assembleBusinessEndpoints` 的注释同步补分界句，空清单降级为该批装配落地前的过渡态；完整改写随实现落地。
- 开发主线第 24 轮「不计机制缺口」的表述在下轮盘点时引本记录更新，不在本记录里代改。

## Alternatives considered

- **维持空清单，等 `PAR-INT-01` 一起做。** 否决：租户数为零时那是一件不会到来的事（ADR-0052 同款理由）；等待期间「没有能力」与「未配置」不可分辨，评审已把这个折叠的代价记成 P1。
- **未配置时交回 error，复用 `INTAKE_FAILED`（500）。** 否决：把已知的、正常的实例半边状态伪装成依赖故障；离线客户端会按 ADR-0022 留队重发，运维会去救一个没坏的依赖（ADR-0054 同款否决）。
- **先写「开发用」采信头部 Intake 跑通链路。** 否决：红线明禁；它从请求内容铸造来源信封，穿透 ADR-0003，且事后没有任何东西能把这些信封与真实认证结果区分开。
- **为接入渠道建运行时登记表，读表判配置。** 否决（现在）：渠道配置的形状取决于真实渠道是 API、标准文件还是门户，替它拟表就是替租户拟 `PAR-INT-01` 的样子。装配点足以表达「空册/有行」两态；真渠道就位时若需要表，由那笔工作按实际渠道形状立。
- **归入 ADR-0052 的适用范围。** 否决：0052 的正文与标题限定在网络证据端口；拉伸已接受记录的范围会让回溯的人分不清被接受的是哪一条（ADR-0054 已立过此先例）。

## Links

- [ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)：同病首例，「读空册如实作答不是默认实现」分界句的出处
- [ADR-0054](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md)：同病第二例，「不拉伸已接受记录范围」的先例
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：三类表与 4xx 语义的出处；本记录停用其「但不进 `cmd/parcel-api` 的装配」一句
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格的判据与探针同答约束的原始范围
- [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md)：离线客户端出队/留队分流的出处
- [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)：未配置即拒不得触碰的隔离边界
- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：机制/实例两半划分的唯一权威，「视图的内容属实例半边，实现不是」判别句出处
- [参数登记册](../product/PILOT-PARAMETER-REGISTER.md)：`PAR-INT-01`/`PAR-INT-03` 等各接入面所等参数的登记处
- [评审 081701](../review/081701.md)：触发本记录的 P1 记录与实现排序的出处
