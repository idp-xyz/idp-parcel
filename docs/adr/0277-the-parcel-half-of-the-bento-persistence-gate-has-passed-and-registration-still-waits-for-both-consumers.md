# ADR-0277：Bento 持久化闸门的 Parcel 半边已过——锁定候选的消费者证明齐备之后，持久化实现进生产装配不再限于取证用途；`B-06` 登记与宣称闸门通过仍等双消费者证明

Status: Accepted（2026-10-21，Claude Code 会话代决，代架构 owner 裁定：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/10（10b 半边），出处审计 [audit/cross.md](../../.scratch/docs-consistency/audit/cross.md) PB·M1（归 B）与待核 Q1，兼答 docs-consistency/01「交后续」交给票 10 的「闸门此后的状态」一条。部分停用 [ADR-0017](./0017-admission-gates-judged-by-blocking-cause.md)「**Bento 持久化技术闸门原样保留。**」一段里「PostgreSQL Repository 实现、迁移、事务与 Outbox 仍然阻断」一句与「**端口接口与端口实现分开裁决。**」一段里「它的 PostgreSQL 适配器属 Bento 技术闸门范围，仍阻断。」一句，部分停用 [ADR-0026](./0026-persistence-written-for-consumer-proof-precedes-gate-passage.md)「**即刻解锁的范围**」里「用途限于产出 `PBC-01` 至 `PBC-09` 的证明」一句，被停原句与适用场景见「对已接受记录的作用」；两份正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待架构 owner 复核，第 2 点同时待 Bento owner 会签。）<!-- 索引注记 -->｜**两半**：Bento 持久化闸门分 Parcel 半边与登记半边。Parcel 半边——锁定候选 `v0.1.0-rc.2` 的空缓存下载与 `PBC-01` 至 `PBC-09` 对同一 checksum 全部通过——自 `ccbf79e` 起成立；登记半边——把 Parcel 提交绑为 `B-06` 候选基线、宣称闸门已通过——等 TMS 与 Parcel 双消费者证明，不是 Parcel 单方动作，至今未发生。**放行**：Parcel 半边成立之后，PostgreSQL Repository、迁移、事务与 Outbox 实现进 `cmd/*` 生产装配并按端口继续铺开，不再限于产出证明；`PBC-08` 常驻门禁、真库测试与「不得建立本地框架替身」照旧。**仍禁**：宣称闸门已通过、登记 `B-06`、改锁别的候选而不复评。**写法**：文档说「Parcel 半边已过、持久化进生产装配获准；闸门本身未通过、`B-06` 未登记」，不说「闸门解除」。租户流量进生产仍只经 PN-08 `Go`，本记录不改。越权风险点含 RC 进生产装配、登记半边可能长期不发生、租户生产与 `B-06` 的关系未裁、08-17 至 08-25 进装配的实现事后追认、越过 08-25 受托裁定保留的结论格、候选变化时已铺开的实现怎么办

Date: 2026-10-21

## Context

三份现行文档对同一道闸门的现状说法相反，谁也没停用谁：

- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「机制半边现状」写「持久化闸门已解除」，横切缺口一条写「Bento 闸门解除（`v0.1.0-rc.2` 经 `PBC-01`/`PBC-06` 锁定）后」「未落库的端口按批次推进，不再有闸门阻断」。
- [首个消费者切片简报](../design/parcel-go-first-consumer-slice-decision-brief.md)闸门表的 Bento 行写「保持阻断」「登记前不得宣称闸门已通过」「闸门本身未通过」，同一行的仍禁止范围含「宣称闸门已通过或据此改写开发主线的横切缺口状态」。
- [AGENTS.md](../../AGENTS.md)「当前默认切片」写「租户取值与 Bento 持久化闸门仍阻断」。

代码站在开发主线一边：`internal/*/adapters/postgres` 进了各 `cmd/*` 的生产装配，CI 的真库门禁缺 DSN 即失败，用途早已不只是产出 `PBC-*` 证明。ADR-0026 把解锁范围限定为「用途限于产出 `PBC-01` 至 `PBC-09` 的证明」，理由写在它的替代方案里：「限定用途是本记录能与「发布基线登记仍在闸门后」并存的前提」。那时证明还一项没过。

事实此后变了，变在简报自己的闸门表里：`PBC-01` 至 `PBC-09` 已对锁定候选的同一 checksum 全部通过，空缓存下载以独立 `GOMODCACHE` 取证，2026-08-25 受托裁定「**Parcel 半边自 `ccbf79e` 起就绪**」；`B-06` 绑定要双消费者证明，「TMS 侧最小切片本仓不可观测、证明校验归 Bento 仓协调作业，故绑定不是 Parcel 单方动作」。

时间线要如实：postgres 适配器早在 `64bae1288`（2026-08-17，派发组合根接通真实连接池、Outbox 与首个消费者）就进了 `cmd/parcel-dispatch` 的生产装配，先于 `ccbf79e`（2026-08-25）Parcel 半边的证明齐备；那一段装配走在 ADR-0026「用途限于产出 `PBC-01` 至 `PBC-09` 的证明」之外。

于是「闸门」一词在三份文档里指的不是同一件事。开发主线说的「解除」是 Parcel 半边的证明齐了；简报说的「未通过」是登记半边没发生；AGENTS.md 复述的是后者，却被读成连持久化实现都不许进装配。ADR-0026 的用途限定是为「证明未过」那段时期设的，证明过了以后它挡的只剩一件事：已经证明过的实现进生产装配。这件事不再需要挡，因为 ADR-0026 要防的「未经证明的东西取得基线地位」此时已不成立——实现所锁的候选、checksum 与消费者合同都已证明，没有证明的只是对方那一个消费者。

## Decision

**一、Bento 持久化闸门分两半判，各半各有通过条件。**

- **Parcel 半边**：ADR-0009 的正式 module path、不可变候选（由 module checksum 认定，ADR-0026）、空缓存下载、`PBC-01` 至 `PBC-09` 对同一版本与 checksum 全部通过。这一半自 `ccbf79e` 起成立，以简报闸门表的取证记录为准，本记录不复制证据。
- **登记半边**：把 Parcel 提交绑为 `B-06` 候选基线，执行双消费者证明，此后才可宣称闸门已通过。它要 TMS 一侧的最小切片与 Bento 仓的协调作业，不是 Parcel 单方动作。

**二、Parcel 半边成立之后，持久化实现进生产装配不再限于取证用途。** 针对锁定候选编写的 PostgreSQL Repository、迁移、事务与 Outbox 实现，进 `cmd/*` 的生产装配，按端口继续铺开。约束照旧，一项不松：

- 只用 `go.mod` 锁定的那一个精确候选；不得建立本地框架替身，不用 `replace`、`go.work`、浮动分支、框架源码副本与伪事务（ADR-0026「仍然禁止的范围」）。
- 每个新持久化写口过 `PBC-08` 的常驻门禁（`internal/architecture`）与真库测试；真库适配器不实跑不推。
- 锁定的候选、checksum、消费者合同或迁移形状一变，按简报闸门表「失效或复评触发」一格复评；复评的是 Parcel 半边，复评未过之前新铺开的实现回到 ADR-0026 的取证用途。

**三、登记半边照旧在闸门后。** 登记 `B-06`、宣称闸门已通过，仍等双消费者证明；在那之前任何文档不写「闸门已通过」或「闸门解除」。现状的写法统一为：**Parcel 半边已过、持久化进生产装配获准；闸门本身未通过、`B-06` 未登记。**

**四、本记录不碰业务语义闸门与租户生产。** 业务语义闸门的实例半边照 ADR-0017；某租户的流量进入生产只经参数登记册的实例证据与 PN-08 `Go`。持久化实现进生产装配，不等于任何租户可以上生产。

## 候选与反方

- **维持 ADR-0026 的用途限定，把开发主线改回「取证放行、闸门未过」。** 否决：那要把已经进生产装配的全部持久化适配器判为越闸，而它们锁的候选与 checksum 都已证明；用途限定要防的「未经证明的东西取得基线地位」已经不成立，再守就是守一个空前提。
- **宣布闸门整体通过。** 否决：登记半边的条件一项都不在 Parcel 手里，宣布通过正是简报与 ADR-0026 都明禁的那句话，也会让 Bento 稳定版前的「双消费者证明」读成已有。
- **只改措辞，不立记录。** 否决：开发主线、简报与 AGENTS.md 三处说法相反，改哪一处都要一个外部权威说哪一边算现行；ADR-0017 的 Status 行已把「闸门此后的状态」交给开发主线与简报，而这两份今天说法不一。
- **把租户生产也挂到 `B-06` 上。** 不取：那是新设一道租户上线前置，本轮没有证据说它必要，也不在审计所报的矛盾里；列为越权风险点，由 owner 定。

## 对已接受记录的作用

- **部分停用** [ADR-0017](./0017-admission-gates-judged-by-blocking-cause.md)「**Bento 持久化技术闸门原样保留。**」一段里「PostgreSQL Repository 实现、迁移、事务与 Outbox 仍然阻断」一句（该句已由 ADR-0026 收窄过一次），与「**端口接口与端口实现分开裁决。**」一段里「它的 PostgreSQL 适配器属 Bento 技术闸门范围，仍阻断。」一句：Parcel 半边成立之后，这些实现进生产装配获准（本记录决定二）。**适用场景**：只限锁定候选的 Parcel 半边成立期间；候选一变、复评未过，回到 ADR-0026 的取证用途。同段「仍不得建立本地框架替身」、「端口接口与端口实现分开裁决」这一方法、业务语义闸门各句与 ADR-0017 其余各条不变。
- **部分停用** [ADR-0026](./0026-persistence-written-for-consumer-proof-precedes-gate-passage.md)「**即刻解锁的范围**」里「用途限于产出 `PBC-01` 至 `PBC-09` 的证明」一句：Parcel 半边成立之后不再限于产出证明（本记录决定二）。**适用场景**：同上。「**仍然禁止的范围不变**」一段、「把实现登记为发布基线仍在闸门后」、「闸门的通过条件一项不改」、checksum 认定不可变性与 ADR-0026 其余各条不变。
- [ADR-0043](./0043-publish-intent-claimed-by-result-identity.md) 决定四「实际投递、outbox 与事务发布仍归闸门」、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md) Consequences 里说两边的持久化与事件发布仍阻断于 Bento 闸门那一条、[ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md) Consequences 里说真适配器仍阻断在 Bento 持久化闸门后那一条、[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 与 [ADR-0090](./0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md) Consequences「本记录不解除任何闸门」：不停用条款。这些句子要么是写下时的现状，要么是把持久化交给闸门去判，按本记录决定三读——闸门的 Parcel 半边已过、登记半边未过；ADR-0043、0025、0031 在各自索引注记里挂读法注，ADR-0022、0090 那句说的是它们自己的范围，照旧成立，不挂。
- [ADR-0009](./0009-go-modular-monolith-and-versioned-bento-contracts.md)：不停用。Consequences「在真实不可变 Bento 候选、读取身份和消费者证明就绪前，可以建立本地合同脚手架，但不能声明形成稳定发布基线」照旧成立：本记录不声明稳定发布基线。

## Consequences

- 简报闸门表的 Bento 行、[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)写闸门现状的各句与 AGENTS.md「当前默认切片」改为引本记录、照决定三的写法说，不在三处另立口径。
- ADR-0026 Consequences 说开发主线的「无持久化」「无事件机制」两项「在证明通过后才改状态」，所等的证明已过，开发主线这两项的改状态不越闸。
- 新增一项判断成本：每次援引这道闸门都要说清指的是哪一半。写「闸门」而不分半，按登记半边读，即未通过。

## 越权风险点

1. **RC 进生产装配**（架构 owner）。锁定的是候选 `v0.1.0-rc.2`，不是稳定版。它的比特由公共透明日志钉死、不会漂移，但 Bento 侧对 rc 的支持期没有承诺；owner 若要求稳定版之前生产装配只留取证用途，改的是决定二。
2. **登记半边可能长期不发生**（架构、Bento owner）。TMS 侧最小切片不在本仓可见，`B-06` 何时绑定本记录无从裁；在那之前闸门一直「未通过」，读者要靠决定三的写法分清两半。
3. **租户生产与 `B-06` 的关系未裁**（架构、产品 owner）。某租户进入限量生产时，其候选版本组里的 Parcel 提交要不要已经绑为 `B-06`，本记录不决定；按今天的规则只看 PN-08 `Go`。
4. **08-17 至 08-25 之间进装配的实现是事后追认**（架构 owner）。`64bae1288` 起进生产装配的那批持久化实现，在 Parcel 半边的证明齐备之前就已越出 ADR-0026 的取证用途；本记录不把它们判为越闸，等于追认。owner 若不认，要回头核那一段的提交。
5. **本记录越过了 2026-08-25 受托裁定刻意保留的结论格**（架构 owner）。简报闸门表那条受托裁定（MCP-1）写明「本裁定不改结论格」，把「保持阻断」留给了后续；本记录把结论格改成两半分写，是代决，不是那次裁定的延伸。
6. **候选变化时已铺开的实现**（架构 owner）。决定二说复评未过之前新铺开的实现回到取证用途，但已进装配的那一批不回退；若 owner 要求整批回退，复评就成了一次全仓回滚，代价要另算。

## Links

- [ADR-0017：实现准入闸门按阻断理由分别裁决](./0017-admission-gates-judged-by-blocking-cause.md)：部分停用「仍然阻断」两句，按阻断理由分别裁决的方法沿用
- [ADR-0026：为产出消费者证明而写的持久化实现先于闸门通过，发布基线登记仍在闸门后](./0026-persistence-written-for-consumer-proof-precedes-gate-passage.md)：部分停用「用途限于产出 `PBC-01` 至 `PBC-09` 的证明」一句，发布基线登记仍在闸门后不变
- [ADR-0009：采用 Go 模块化单体并复用版本化 Bento 技术合同](./0009-go-modular-monolith-and-versioned-bento-contracts.md)：不停用
- [首个消费者切片简报](../design/parcel-go-first-consumer-slice-decision-brief.md)：闸门表 Bento 行与证据所在
- [开发主线：机制半边现状](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#机制半边现状)
- 票 `.scratch/docs-consistency/issues/10-cross-cutting-docs.md`（10b 完成记录）
- [ADR-0043](./0043-publish-intent-claimed-by-result-identity.md)、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)、[ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：「闸门挡持久化」的句子按本记录决定三读，各在其注记挂读法注，无条款停用；ADR-0022、ADR-0090 的「本记录不解除任何闸门」照旧成立，不挂
