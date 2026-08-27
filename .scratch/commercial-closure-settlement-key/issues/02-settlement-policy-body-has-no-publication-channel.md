# 结算政策正文没有发布通道——库表、端口、适配器、用例都在，就是没人写得进去

Category: enhancement
Status: ready-for-agent

从 [01](./01-resolution-key-registration-cannot-carry-the-settlement-selector.md) 收口时撞出来的阻断一（锚 `9c95d7c`）。那一票把解析键的登记面扩到了结算三维，闭包因此第一次真去问「这个范围里的结算约定是哪一份」——问完发现权威册里一份也没有，而且没有任何进程放得进去。

## 事实链（实读代码取证，锚 `9c95d7c`）

1. **持久化半边齐全**。`internal/partycommercial/adapters/postgres/settlement_policy.go` 的 `SaveSettlementPolicy` 撞键不覆盖、配对用例在 `settlement_policy_test.go`；表 `commercial_settlement_policy`（迁移 0011）在；`CommercialPublications.LoadForScope` 已经把结算政策行连同版本一条语句取回并 `RegisterSettlementPolicy` 进册。
2. **写入侧断在应用层**。`PublishCommercialAuthorityHandler.declarationWrites` 有九路声明通道，没有结算政策这一路；`CommercialDeclarations` 也没有对应字段。全仓对 `SaveSettlementPolicy` 的调用只有测试替身与真库用例。
3. **进程口同缺**。`cmd/parcel-commercial` 的 `declarationsDocument` 没有承载方式与六维适用范围的字段，发布批因此表达不出一份结算政策正文。
4. **后果已经从「页面空一列」升级成「主径断」**。种子 README 的「已知边界」写这条时，症状只是商业策略页的结算政策列为空（ADR-0077 下空册本身就是内容，那时是对的）。票 01 之后同一个缺口挡住的是接受前控制链：解析键要得动结算依据了，闭包却只能答`无适用依据`——而那句话的意思是「权威说这个范围没有结算约定」，实情是没人能让权威说话。

真库取证（演示库灌完种子，票 01 的探针）：

    outcome=NO_APPLICABLE_BASIS unresolved=[SETTLEMENT_POLICY]
    conflicting=[] premiseUnresolved=[]

`premiseUnresolved` 空——合同解出来了，前提立住了，这一项是真的问过。

## 要先定的两件事

- **`declarationWrite` 的形状**。既有九路的 `save` 一律交回 `ports.DeclarationSaveOutcome`；`SaveSettlementPolicy` 交回的是 `SettlementPolicySaveOutcome`（三格同构但类型不同，`SavePricePolicy` 亦然）。要么把两族落点在应用层折成一族，要么给 `declarationWrite` 加一格。折成一族更省，但那是在说「政策正文与声明正文是同一种东西」——两者的键不同（政策按版本四元组一行，声明按拥有版本挂），值得在动手前想清楚。
- **价格政策的同处缺席要不要一并补**。`SavePricePolicy` 与 `SaveServiceProduct` 都没有生产调用方。价格政策还多一道：`RehydrateAdoptedBasisSpec` 的注释记着它的快照重建至今缺席（要方案方向与跨向转换两个入参，而类型本身不留存它们）。补发布通道不等于补重建，两者可以分开，但要显式分开，不能补完发布就以为价格方向那一维也通了。

## 实现范围

- `internal/partycommercial/application`：`CommercialDeclarations` 加结算政策正文一格；`declarationWrites` 加一路，构造走 `domain.NewSettlementPolicy(version, method, applicability)`——构造门要求拥有版本已生效，与其余九路同一条纪律（挂在`已计划生效`版本上的正文整项拒绝）。
- `cmd/parcel-commercial`：发布批文档加对应字段（方式两取值的名称镜像、六维适用范围）；未知字段拒收与集合外取值拒收两条不动。
- `scripts/demo-seeds`：发布批补一份 `SYN-SETTLEMENT-*`，六维要与票 01 那行解析键的三维加解出的合同版本严丝合缝——法人 `SYN-LE-01`、相对方 `SYN-ACCOUNT-01`、费用范围 `SYN-CHARGE-PREPAID`、币种 `CNY`、合同 `SYN-CONTRACT-01/v1`、区间含锚点 `2026-02-01T00:00:00Z`；方式取 `PREPAID`（合同正文对该费用范围绑的正是预付财务控制策略）。
- 迁移不动：0011 已在。

## 完成标准

- 种子灌完后，票 01 的那份解析键解出`唯一解析`，闭包 `AdoptedFor(SETTLEMENT_POLICY)` 带得出方式与六维范围。
- 六维差一维就命不中：至少一例证「币种（或费用范围）换一个字，同一份政策不再被采用」——本上下文禁止借宽泛关系跨维归集，这条得有人在发布通道这一层也守着。
- 结算政策册页面（`/commercial-policies` 结算政策页签）上出现那一行，不再是空册。

## 地盘

`internal/partycommercial/{application,ports}`、`cmd/parcel-commercial`、`scripts/demo-seeds`。不碰 `internal/parcelshipment/**`（票 01 已落），不碰 `cmd/parcel-api`。
