# 接受链没有装配点也没有进程入口——三个编排至今只在一份测试夹具里被构造过

Category: enhancement
Status: ready-for-agent

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

## 地盘

`cmd/parcel-api` 或 `cmd/parcel-dispatch` 的装配面（按裁决二选一）、`internal/parcelshipment/adapters/http`（若取 HTTP 路）。占号敏感：这两份接线文件由占号纪律管着。
