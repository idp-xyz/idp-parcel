# 接受链没有装配点也没有进程入口——三个编排至今只在一份测试夹具里被构造过

Category: enhancement
Status: resolved
Owner: MCP-1（入站）+ MCP-2（裁决、出站、往返取证）

从 [01](./01-resolution-key-registration-cannot-carry-the-settlement-selector.md) 收口时撞出来的阻断二（锚 `9c95d7c`）。票 01 与 [sa-preacceptance-policy-view/01](../../sa-preacceptance-policy-view/issues/01-sa-preacceptance-control-policy-view-has-no-production-adapter.md) 都写着「装上 SA→PC 控制策略适配器到 `cmd/parcel-api` 的接受链」，两份票面的占号核对也据此判「只加装配行、不增删端点」。**那条接受链不存在**，两份票面的这一条都建立在一个没核过的前提上。

## 事实链（实读代码取证，锚 `9c95d7c`）

1. **三个编排在 `cmd/` 下只有一个调用点，且是测试夹具**。`psapplication.NewFormAcceptanceDecisionHandler`、`NewAdvanceAcceptanceJudgmentHandler`、`NewAdvanceFinancialControlJudgmentHandler` 全仓的非测试调用方为零；`cmd/parcel-dispatch/synthetic_v0_test.go` 是唯一在 `cmd/` 下构造过它们的地方。
2. **SA 施加半边同样零生产调用方**。`saapplication.NewApplyPreAcceptanceControlHandler` 只在测试里被调。`cmd/parcel-api/assemble_withdrawal.go` 装的是 `pssettlement.PreAcceptanceControlAdapter` 的**释放**半边，`Apply` 与 `Scopes`/`Amounts` 三格显式留空，函数注释把理由写在那里。
3. **SA→PC 控制策略适配器零生产调用方**。`internal/settlementaccounting/adapters/partycommercial.NewPreAcceptanceControlPolicy` 只被它自己的真库用例调用——sa-preacceptance-policy-view/01 收口时记的「适配器暂不装进 `cmd/parcel-api`」，今天仍是「没有地方可装」。
4. **端点面没有接受判断入口**。`assembleBusinessEndpoints` 的清单里，parcel-shipment 侧只有提交、撤回、包裹取消与委托查阅四条；`internal/parcelshipment/adapters/http` 下也只有这四个 `New*Endpoint`。

## 要先裁的一件事

**接受判断由什么驱动。** 两条路先例都在库里：

- **HTTP 端点**，与提交/撤回同形——调用方显式推进一次判断。好处是装配点与既有三条命令面同构，`transactionalSubmission` 那套事务边界照抄即可；代价是「谁来调它」成了新的实例半边问题。
- **信封驱动**，与 `cmd/parcel-dispatch` 的十二类消费者同形——提交落库后由派发一拍推进。语义上更贴：接受判断是提交之后的自动推进，不是外部再发一条命令。代价是要新增事件类型与消费门，且 `wireDispatcher` 是全仓依赖面最宽的包（ADR-0049 认下的那笔代价会再宽一格）。

这一裁决够一份 ADR：它决定了接受判断在产品上是「被请求的」还是「被触发的」，两者的失败面与重试语义不同（前者调用方重发，后者 inbox/outbox 续办）。

## 实现范围（定案后）

- 装配点：按裁决落在 `cmd/parcel-api`（新 `assemble_acceptance.go` + 端点行）或 `cmd/parcel-dispatch`（新消费者 + 路由表一行 + 未决哨兵一份）。**两处都属占号**，动手前按 `docs/agents/parallel-sessions.md` 核。
- 接上 `PolicyBackedControlScopeSource`（`SettlementAccountDirectory` 留 nil——账户目录属实例半边，不得为验它而造映射）与 `pssettlement.PreAcceptanceControlAdapter` 的 `Apply` 半边。
- 接上 SA→PC 控制策略适配器 `NewPreAcceptanceControlPolicy`（`CommercialResolutionView` + `PreAcceptanceControlDeclarationView` 两个真库读口）。
- `ControlAmountSource` 留 nil：估价缝属实例半边，停在 `CONTROL_AMOUNT_NOT_CONFIGURED`。

## 完成标准

- 种子租户下接受前控制走通到`要求-预付`分支，停在 `CONTROL_SCOPE_NOT_CONFIGURED` 且原因是**账户目录未配置**——与「闭包形不成」可分辨（票 01 完成标准的第一条，它要的正是这一格）。
- 上一条曾要等 [02](./02-settlement-policy-body-has-no-publication-channel.md)：闭包里没有已采用结算政策时，作用域源在 `SettlementTerms()` 缺席那一支就早退，走不到账户目录那一步。**02 已收口**（`ae966e6` / `41250b1`），种子租户下闭包解出`唯一解析`且结算依据带得出方式与六维范围，这条前置不再成立。

## 进展 · 裁决已定，入站这半已收口（MCP-1，锚 `5fce8ab`）

**裁决取路 B（信封驱动）**，依据见 [前置裁决简报](../acceptance-drive-decision-brief.md)，用户授权 1-2 号通道协同推进。落档的 ADR 与出站那半归 MCP-2，本节只记入站这半。

入站已落库推送两笔：

- `e3fdcff` — 编排 `AdvanceAcceptanceChainHandler` 与消费门 `ShipmentRequestSubmittedConsumer`。编排把三步串成一件事（逐成员可达性 → 整份委托财务控制 → 形成决定），任一步未决即整条停下，停在哪一步由 `AcceptanceChainStage` 单独交回；装配缺件与空成员清单响亮报错而不压成未决。消费门按简报第二格「一个消费者内按顺序推进，不拆中间事件类型」实现。
- `5fce8ab` — `wireDispatcher` 装上这条链：路由表加一行，未决哨兵一格（简报第三格）。三条判断腿共用同一个商业依据适配器实例（形成决定要按判断当初采用的那份解析重校验）；控制策略视图接 PC 声明册，作用域源接 `PolicyBackedControlScopeSource`。

实现范围里的三件按票面办了：`NewPreAcceptanceControlPolicy` 与 `PreAcceptanceControlAdapter` 的 `Apply` 半边都接真，`SettlementAccountDirectory` 与 `ControlAmountSource` 留 nil。

### 完成标准第一条已取证

种子库（`SYN-TENANT-01`）上按生产同一条路径造键、解析、提问，一次性探针实测：

```
KEY            purpose=ACCEPTANCE_CONTROL bases=[SERVICE_PRODUCT ACCEPTANCE_RULE_PACKAGE CUSTOMER_CONTRACT SETTLEMENT_POLICY]
CLOSURE        outcome="UNIQUELY_RESOLVED" resolutionID="CLO-2e7e0fc5e2b61c04" fixed=SAVED
SETTLEMENT     adopted present=true method=PREPAID version=SYN-SETTLEMENT-PREPAID-01/v1
CONTROL POLICY configured=true required=true method=PREPAID adopted=SYN-SETTLEMENT-PREPAID-01/v1
APPLY          outcome=NOT_FORMED reason=CONTROL_SCOPE_NOT_CONFIGURED
```

两格正是本条要的：控制策略答**要求-预付**（第三、四行），控制本身停在 `CONTROL_SCOPE_NOT_CONFIGURED`（第五行）。**与「闭包形不成」可分辨**靠的就是第三、四行——策略视图要先取到唯一已解析闭包、再从闭包里取出已采用结算政策才答得出方式，它既然答了`预付`，闭包必然形得成，那么第五行的缺口只可能是账户目录（与金额源，它在作用域之后）。

探针用完即删，因为它要的是**已灌种子**的库，而 `pgtest` 给每个用例开一个独立空库——留下来只会在别人的干净库上红。代价如实记：它在演示库里留下了一份固定闭包 `CLO-2e7e0fc5e2b61c04`，那正是一次真解析该留的东西，复灌用 `seed.sh --reset` 清。

### 入站这半的装配取证（真库，随仓测试常驻）

`cmd/parcel-dispatch/assemble_test.go` 三条：毒丸载荷证路由表挂对了人；各维齐全的载荷证整张依赖图在生产装配上跑得动（空库没登记解析键，链停成未决，路由条目翻成 `dispatch.consumer_undecided`）；失败分格用生产的同一份哨兵名单，另三格保持 `dispatch.publish_failed`。全仓真库套件绿（含架构门禁与 `tests/bentocontract`）。

## 收口 · 出站这半与裁决落档（MCP-2，`cc1d646` / `46dbb90` / `9ba78ff`）

裁决落成 [ADR-0081](../../../docs/adr/0081-acceptance-judgment-is-envelope-driven.md)（`cc1d646`）：接受判断是被触发的不是被请求的，驱动信封即「委托已提交」，不新增事件类型也不在端点面开判断入口；同步半边与事件半边的分工、消费门的失败分格、以及实例半边一格不填，都在那份记录里权威，此处只记实现。

**出站装配（`46dbb90`）。** `cmd/parcel-api` 的整段 Handle 单事务壳 `transactionalSubmission` 退役，换成简报「事务边界」的两段——`preservationBoundary` 给来源保全的每笔写入各开一个事务（保全一经提交就不随后续步骤回滚，UC-PS-001 步骤 2 要的正是这条），`submissionBoundary` 携 `OutboxShipmentRequestSubmittedHandoff`（建单与信封同事务原子；`Insert` 答`已存在`时本事务没写下任何东西，不入队第二份意图）。两个壳与 `tests/bentocontract` 里 PBC-04/05/07 取证过的形状同形。编排签名不动，事件机制不进应用层。

**往返取证（`9ba78ff`）。** 接上之后，两侧载荷标签漂开是一条**静默**失效面：译不出即毒丸，消费门显式拒收入账交回 nil，那一封被记成发布成功，而链一次都没跑过——库里的样子与「实例半边还没配置」逐字相同。既有用例守的是各自那一侧对自己抄本的忠诚，不是两侧彼此对得上。实测于 `46dbb90` 的三组探针（改完即还原）：只改发布侧 → PBC-05 红；只改消费侧 → `adapters/inbox` 与 `cmd/parcel-dispatch` 红；**发布侧连同它自己的 PBC-05 镜像一起改（消费侧不动）→ 四个包全绿**。第三组正是最自然的那一步。`TestTheMintedEnvelopeDecodesIntoTheAcceptanceChainCommand` 不手抄载荷：建单落下的那一封按派发一拍同一条认领路径取回来，喂给生产消费门，译不出与译错分两格断言。

**取证强度。** 全仓 `go test -count=1 ./...` 绿（含 PG，`-v` 下 PASS 非 SKIP）；`46dbb90` 另在临时 worktree 上按提交状态复验过一遍。

### 本票收口后仍未合的一格

**生产 HTTP 路径今天造不出信封**：提交先撞墙一（`ACCESS_CHANNEL_NOT_CONFIGURED`）或墙二（`OWNERSHIP_UNRESOLVED`），建单一段走不到，链因此不动。这不是本票的欠账——两堵墙各有自己的票，ADR-0081 的 Consequences 已把这一格记明。上面两条取证走的都是合成路径（归属权威用放行替身，只记 `S`）与种子租户，与票 02/03 的完成标准同路。

## 地盘

`cmd/parcel-api` 或 `cmd/parcel-dispatch` 的装配面（按裁决二选一）、`internal/parcelshipment/adapters/http`（若取 HTTP 路）。占号敏感：这两份接线文件由占号纪律管着。

裁决取路 B 之后本票实际动的是 `cmd/parcel-dispatch` 那一侧（MCP-1）与 `cmd/parcel-api` 的出站装配及其用例（MCP-2）；`internal/parcelshipment/adapters/http` 一行未动，端点面没有新增任何入口。
