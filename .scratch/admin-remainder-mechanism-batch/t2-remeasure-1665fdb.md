# T2 量尺重核（对 `main=1665fdb`，批务票 05 第 1 项）

上一轮是 [t2-remeasure-7d68475](./t2-remeasure-7d68475.md)，口径源头是
[census-d5e5d20](../production-wiring-ratchet-gate/census-d5e5d20.md)。本轮不另造口径：
git 侧匹配、大小写敏感、调用点扣**全部**声明行（按包+名，不按「命中数 ≤ 1」）、四族不加总。

- 取证点：`1665fdb`（提交态，非工作树）。工作树此刻另有 22 个已修改 + 24 个未跟踪文件，
  全部不在本文取证范围内。
- 取数命令固化在 [`t2-census.ps1`](./t2-census.ps1)，三个锚点的原始输出留在
  `t2-census-out-<sha>.txt`。重跑：`powershell -File .scratch/admin-remainder-mechanism-batch/t2-census.ps1 -Sha <sha> -ListNames`。

## 先说口径本身：这一轮是可比的，除了一个数

基线两次已发生的口径错都不是数错，是「命令看着一样、问的不是同一个量」。所以本轮先把同一段
脚本回跑到两个历史锚点上，再跑新 HEAD。

**在 `d5e5d20` 上，四族全部逐位复现**——46/41、62/53、72/13，四族 97 个断言文件 / 66 个
构造函数 / 46 个零调用点，且那 46 个名字与基线名单逐字一致（含 `NewDispositionRequests`、
`NewSignalEpisodes`、`NewSourceDataVersions` 三个「声明 2 处」的）。

**在 `7d68475` 上，零调用点四个数（33/35/13/20）与构造函数四个数（47/68/72/84）也全部复现。
只有「断言文件 123」这一个复现不出来，我得 118。**

按「复核一个带锚的数要重导它的问题，不是重跑它的命令」查下来：**唯一能得 123 的问法是在
识别断言文件那一步不排除 `_test.go`**（同一模式、同一路径、只去掉排除项，`7d68475` 上正好
123）。但同一个问法在 `d5e5d20` 上给 **100**，而基线白纸黑字是 97——**所以 123 是问法漂了
一格，不是树长了五个文件。** 本文的 118 与基线 97 才是同一个量。

**这一格不外溢。** 两种问法取到的**构造函数集合完全相同**（`7d68475` 上都是 86 条声明行 /
84 个唯一名字），因为那五个 `_test.go` 里没有 `^func New*(`。所以上一轮的 84 与 20 不受影响，
照旧作数；受影响的只有「123」这一个数字本身。

> 顺带一格：上一轮把四族写成「构造函数 66→84」，而 84 是**唯一名字数**，声明行是 86。
> 基线的 66 两者相等（当时四族名字无一重复），所以那一轮不必分。**从 `7d68475` 起两者已经
> 分开，本文两个数都列**——判零走的是名字，说「这一族有多少个构造函数」走的是声明行。

## 四族对照（三个锚点并排）

| 族 | `d5e5d20` 总/零 | `7d68475` 总/零 | `1665fdb` 总/零 | 本轮 delta |
|---|---|---|---|---|
| 一族 outbox 交接口 `NewOutbox*Handoff` | 46 / 41 | 47 / 33 | 47 / 33 | 完全持平 |
| 二族 应用层处理器 `New*Handler` | 62 / 53 | 68 / 35 | 71 / 35 | +3 总；零不动 |
| 三族 领域工厂（十二前缀） | 72 / 13 | 72 / 13 | 76 / 13 | **+4 总**；零不动 |
| 四族 ports 适配器（按接口断言认） | 97 文件 / 66 声明 / 66 名 / 46 零 | 118 / 86 / 84 / 20 | 138 / 107 / 104 / 20 | +20 文件、+21 声明、+20 名；零不动 |

**四族零单在 `7d68475` 与 `1665fdb` 之间逐名对差，四族全部逐字一致**（33、35、13、20 四个
集合的 `Compare-Object` 均为空）。同数可能掩盖换名，这一步是专为它跑的，结论是没有换名。

三个「零不动」不叫还债停滞，也不叫还债进度——基线自注仍然作数：**零调用点绝大多数是十八墙
里还没建门的口，属缺席而非在场且错**。本轮真正的动静全在分母上。

## 分母长在哪：三族里长出来的那四个，是同一个新上下文

### 三族：十二前缀集第一次长了，一格没长的说法到此为止

上一轮写「三族完全持平，名单逐字一致」，并记下「8-21 以来全部新领域函数走 `New*` /
`Rehydrate*` 前缀，十二前缀集一格没长」。**本轮它长了 4 个，且四个全在同一个上下文**：

`AcceptCollectionFact`、`FormRemittanceBatch`、`OpenSubledger`、`RecordSubledgerPosting`
（均在 `internal/collectionremittance/domain/`）。

四个都不在零单上——它们都有非测试调用点。零单仍是原来那 13 个，名字一字未变。

**这一格对下一个拿三族当量尺的人有话说**：前缀集这次跟上了，不是因为有人维护它，是因为新
上下文的命名恰好落在册内。基线「前缀集是判断不是穷举」那条约束没有被本轮证伪，只是这次没
被撞上。三族盲区（前缀集外的 132 个非 `New*` 导出函数、`Rehydrate*` 族）**本轮未重扫**，
基线的「26 才是当时的下界」仍未被任何一轮翻新。

### 二族 +3：全部已接线，但有一个名字的形状要记一笔

新增 `NewHandler`、`NewRegisterPortsPathsHandler`、`NewRegisterProductChannelHandler`，
三个都有非测试调用点，因此零单纹丝不动。

`NewHandler` 值得单记：**它是二族第一个不带任何上下文词根的名字**（在
`internal/collectionremittance/application/register_collection.go`）。实测于 `1665fdb`，
它全仓只有这一处顶层声明、只被 `cmd/parcel-collection-register` 的 `main` 调用一次，
**所以今天这一格数得对**。记它不是因为它现在错，是因为基线那条「一个构造函数要包 + 名两者
才定得住」原先只在四族兑现过；二族现在也有了一个跨包必撞的名字，**门禁按名字建索引会在
第二个 `NewHandler` 出现的那天静默把它读成已接线**，而那时不会有任何东西变红。

### 四族 +20：读面接线批的产物，跨包重名从 3 涨到 6

新增 20 个名字（断言文件同增 20）：`NewCaseReview`、`NewChargeCatalogue`、
`NewCodSubledgerCatalogue`、`NewCollectionPointView`、`NewCollectionRegistrations`、
`NewEvaluationCatalogue`、`NewFundsApplicationCatalogue`、`NewGovernanceRegisters`、
`NewOperatingCatalogue`、`NewPortsPathsCatalogue`、`NewPortsPathsPointView`、
`NewPortsPathsRegistrations`、`NewProductChannelMappings`、`NewRemittanceBatchView`、
`NewRemittanceStore`、`NewReviewCatalogue`、`NewRoutePlanCatalogue`、`NewStatementCatalogue`、
`NewSubledgerBalanceView`、`NewSubledgerStore`。二十个全部有非测试调用点。

**基线那条「不存在第四个待发现」已经过期，而且是按它自己的口径过期的。** 基线实测
`d5e5d20` 上四族恰有三个跨包重名（`NewDispositionRequests`、`NewSignalEpisodes`、
`NewSourceDataVersions`），并据此写下「MCP-1 那 46 是完整的，不存在第四个待发现」。那句话
锚在 `d5e5d20` 上没错；**`1665fdb` 上是六个**，新增 `NewOperationsCatalogue`（三处声明）、
`NewParcelCancellations`（两处）、`NewReviewCatalogue`（两处）。

一至三族在 `1665fdb` 上仍**各自零个跨包重名**（用与基线同一套前缀集机械对查，不引入第二套
口径）。全仓顶层声明索引同步长到 **2415 条 / 2231 个名字 / 79 个名字有多处声明**（基线
1883 / 1804 / 47；范围同为 `internal` + `cmd` + `migrations`，非测试，含泛型声明格）。

## 一族：装配面没动，零 33 口仍是 A/B 票对的候选池

一族两个数与上一轮完全相同。上一轮记的「装配点已从单一 `cmd/parcel-dispatch/assemble.go`
扩为多处」这一条仍然成立且未扩大。零 33 口与消费清单的对映关系见
[消费清单本轮刷新记录](../outbox-handoff-consumption-map/report.md)。

## 本轮没做的事（如实记）

- 三族盲区（前缀集外 132 个非 `New*` 导出函数、`Rehydrate*` 族 43+ 个）未重扫，沿用基线
  「13 是下界、26 才是当时的下界」。
- 四族「无断言适配器」盲区（基线记 110 个含 `New*` 的无断言文件）未重扫。
- 消费清单主表 46 行 × 4 栏的逐行 CONTEXT-MAP/UC 判据未重验——**本轮与上一轮同样只刷装配面**。
- 上一轮点名的 `NewScopeVersionRelations`（治理关系表，非测试 0 / 测试 2）**在 `1665fdb`
  上原样还在四族零单里**，本轮同样只点名不开票。
