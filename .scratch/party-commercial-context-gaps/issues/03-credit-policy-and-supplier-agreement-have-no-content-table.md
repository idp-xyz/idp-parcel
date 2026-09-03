# 信用政策与供应商协议：能入册、能被选中，选中之后拿不到正文

Category: chore
Status: resolved——`877444a`（2026-09-03，MCP-2），验证见 Comments 末条

> **裁决**：信用政策与供应商协议**两张正文表都补**。额度取值形态已定：**并存两列 + 恰一非空**，
> 领域侧配两格封闭值对象（形照 `ProductChannelBinding`），不做「`limitMinor` 加一个布尔」
> ——理由见下面「前置岔口已答」一节。
>
> 连带：`limitMinor` 之外的比例形态是领域缺口，本次一并补（票面原写「裁补则连带处理」）。
> 端口按注释里说的**扩方法不开通用口**。
>
> 地盘：`internal/partycommercial/` 与 `migrations/party_commercial/`（两份新迁移）。
Blocked by: 无

## 这两件为什么合成一票

形状完全相同，且注释里自己就写成了同一件事。`internal/partycommercial/ports/ports.go` 的
`SupplierAgreementCatalogueRow` 注释原话：

> **只有壳**。领域的 `SupplierAgreement` 还携供应商、采购定价方案与方向，但那些今天没有正文
> 表——与 `CommercialPolicyCatalogueRead` 注释里信用政策那一格同形：版本壳可入册，正文册未建。
> 如实只列壳，不从别处拼一份看起来完整的行；正文表落库时在本结构上扩字段，那时才谈得上列它们。

`CommercialPolicyCatalogueRead` 注释那一格原话：

> 六种册子……CONTEXT 词条里的**信用政策**没有独立正文表（版本壳可入册，正文册未建），如实
> 不列——预留一个空方法就是替租户拟一种它还没有的册子；正文表落库时按封闭集扩方法，不开通用口。

**这两段注释是本票的主要证据，也是本票不必反复举证的理由**：缺口是已知的、写下来的、且当时
就按「如实不列」处置过。本票只把它排成可裁的形状。

## CONTEXT 要求什么

### 信用政策

Rules 一节：

> 信用政策和人工费用调整授权按责任法人、业务角色、费用类型、金额或比例形成版本。政策只提供
> 业务判断依据，不直接修改结算余额或形成调整金额。

「接受前财务控制策略」词条把它挂进了接单链路：

> 策略可以要求预付冻结、信用校验、明确接受前无财务控制，或者合同明确规定且相互不冲突的控制
> 组合。

### 供应商商业协议

「供应商商业协议版本」词条：

> 运营企业责任法人与明确供应商、代理商或其他服务提供方在一个适用期间内接受的采购服务、价格、
> 结算和责任条件。它不等于一次实际运输委托、订舱、履约事实或供应商账单。

「供应商商业协议版本」生命周期小节：

> - 供应商商业协议在批准生效后，才能用于新的采购决定和供应商预期成本计算。
> - 采购服务、价格、结算或责任条件变化时形成新版本，不覆盖原版本。
> - 协议版本到期、终止或被替代后，不改变已经形成的运输委托、履约事实、供应商账单主张或审核
>   应付依据。

## 代码里实际有什么（取证 `9d6063c`）

两者都**建得住领域对象**：

- `domain.CreditPolicy` — 由 `NewCreditPolicy` 收下责任法人、权限等级、费用类型、额度（`limitMinor`，
  负值拒），且要求所挂版本 `kind == CreditPolicyObject` 且已生效。
- `domain.SupplierAgreement` — 由 `NewSupplierAgreement` 收下供应商参与方、责任法人、适用范围、
  采购定价方案，`SupportsProcurementAt` 守生效区间，`Direction()` 恒为 `BUY`。

两者也都**在九值封闭集内**（`CreditPolicyObject`、`SupplierAgreementObject`），因此版本壳能入
`commercial_version`、能被解析选中。

缺的是正文：

| | 领域模型 | 版本壳可入册 | 正文表 | 端口 | 读面 |
|---|---|---|---|---|---|
| 信用政策 | 有 | 是 | **无** | **无** | **无** |
| 供应商商业协议 | 有 | 是 | **无** | 仅目录列壳 | 只读壳 |

`migrations/party_commercial/` 下 `0001`–`0017` 十七份里，六份是各类正文册（接单规则包正文
`0014`、接受前财务控制声明 `0007`、商业价格政策 `0010`、结算政策 `0011`、时点锚声明 `0005`、
授权规则与取消授权目录 `0013`）——**信用政策与供应商协议不在其中**。

## 差在哪儿：解析成功之后是一个空手

这两件与票 01 的形状不同，值得分清：票 01 那个类型**连册都进不去**；这两件**进得去、选得中**，
问题出在选中之后。

于是失败代数在这里落不到正确的格。四格里 `无适用依据` / `适用冲突` / `解析未决` 各有明确
含义，而「解析选中了一份信用政策，但取不到它的额度」不属于任何一格——解析本身成功了。
调用方拿到一个 `AdoptedBasis`，顺着它去问额度，那里什么都没有。

两个具体下游因此接不上：

- **接受前财务控制的信用校验分支**：CONTEXT 允许控制组合里含信用校验，而额度与控制条件取不到，
  这一支只能停在未决或从别处取——从别处取正是 CONTEXT 在别处反复禁的那件事。
- **`settlement-accounting` 的供应商预期成本**：CONTEXT 说协议「批准生效后才能用于……供应商
  预期成本计算」，而采购定价方案与方向今天读不回来。

## 补与不补，各自的连带

### 若补

两张表的内容 CONTEXT 已经点名，不需要发明：

- **信用政策正文**：责任法人、业务角色（权限等级）、费用类型、额度或比例。「金额**或**比例」
  是一处待裁——两种取值形态是并存两列（同在或同缺 CHECK）还是判别式一列，须先定。领域今天
  只实现了金额（`limitMinor`），比例那一支**领域也缺**，补表时要一并补，否则表比领域宽。
- **供应商协议正文**：供应商参与方、责任法人、适用范围、采购定价方案。方向恒为 `BUY`，是否
  成列可裁（不成列则由 CHECK 保证语义，成列则与 `commercial_price_policy` 那套方向封闭集
  一致）。
- 两表都按本模块惯例：`object_kind` CHECK 钉死单一类别（先例：`commercial_price_policy_price_rule_only`
  写的是 `object_kind = 6`）、外键指回 `commercial_version`、生效区间有序 CHECK。
- 端口按注释里说的**扩方法不开通用口**：`CommercialPolicyCatalogueRead` 加一格信用政策，
  `SupplierAgreementCatalogueRow` 上扩字段。注释已经把这条路写好了，照做即可。
- 两表 0 行，无清洗。

### 若不补

- 需要有人明说这两类**首发只登记版本身份、不登记正文**，并说清下游怎么办：信用校验分支恒为
  未配置？供应商预期成本恒取不到依据？
- 若如此，两处注释里「正文表落库时……」的措辞该改成明认（「已裁定不建，理由 X，重启条件 Y」）
  ——它现在读起来像是排期问题而不是决定。

## 前置岔口已答：额度取并存两列 + 恰一非空，不取判别式一列（2026-09-02，MCP-5）

上面留了一问——「金额**或**比例」是并存两列还是判别式一列。按本仓反复在防的那一族判，答案是
**并存两列**，而理由不是省事。

判别式一列的形状是「一个数 + 一个说它是什么的标记」。而额度 100 作金额与作比例，**在那一列里
长得一模一样**：标记设错时没有任何东西能分辨，两种读法都产出一个合法的信用额度，只是一个可能
差几个数量级。这正是本仓记了一整节的那件事——两种状态可观察签名相同，而它们要人做的事不同。
并存两列时**「值落在哪一列」本身就是判别式**，它不可能与自己不一致；恰一非空的 CHECK 再把
「都空」与「都有」挡在外面。

先例在本模块：`commercial_price_policy_binding` 用的正是一条复合 CHECK 表达合法组合，而不是
一个标记列加一个自由值——它的注释写明理由是「独立枚举 CHECK 仍挡住集外取值，本约束挡住集内
非法组合」，同一条思路。

**领域侧连带因此也定了形**：比例那一支不能做成「`limitMinor` 加一个 `isRatio` 布尔」，那是把
判别式一列搬进内存。要的是一个两格封闭的值对象，形照 `ProductChannelBinding`（两格各自构造、
零值立不住）。这条不改变票面既有的判断——比例形态是领域缺口，裁「补」则连带处理。

**这一裁只答形状，不答补不补**，后者仍待产品侧裁。若裁「不补」，本节随之搁置。

## 动手前定形的几处（2026-09-03，MCP-2）

裁决把「补不补」与「额度形态」都答了，剩下几处是实现层的形状，动手前写在这里，免得写到一半
各走一路。

**一、额度值对象 `CreditLimit`，两格封闭。** `NewCreditAmountLimit(minor)` 与
`NewCreditRatioLimit(basisPoints)`，零值立不住，访问器各带一个布尔分「本格不适用」与「本该有
却缺了」（后者造不出来），形照 `TaxCaliber.Classification`。比例取**万分比整数**：本上下文
没有 Decimal 类型（票 02 已量过，那是立场不是缺漏），而万分比是商业约定里最细的常用整数刻度；
选 `numeric` 列就得另裁一个精度，那不在裁决里。负值拒、零允许——与既有 `limitMinor` 同判据，
零额度与无政策的分辨仍由 `CreditBasis.Applicable` 承担。

**二、正文进不进整册（`LoadForScope` / `ViewRevision`）——不进，走族 B 点读。** 价格与结算
政策进整册是因为解析要在它们之间选；信用政策与供应商协议今天没有任何解析在它们之间选——
`ResolveCreditPolicy` 在棘轮基线上、零生产调用点。让 `CREDIT_POLICY` 成为闭包里的一种必需
依据是解析语义的改动，裁决没有答它，本票不替它答。消费方走既有路：先按版本壳解析选中，再按
（租户 + 版本）点读正文——与 `CustomerContractContentView` 同形。两个读口因此是
`CreditPolicyContentView.LoadCreditPolicy` 与 `SupplierAgreementContentView.LoadSupplierAgreement`，
`found=false` = 正文未登记，读失败与坏数据走 error 不折成未登记。

**三、写口挂在 `PublicationRegistry` 上，具名 `SaveCreditPolicy` / `SaveSupplierAgreement`，
各配自己的落点类型**（形照 `PricePolicySaveOutcome`，判据见 `ChannelAccountUseSaveOutcome`
注释：两册各自演进，共用类型会让一册多一格时另一册被迫认它）。正文随发布同笔登记——
`CommercialDeclarations` 加 `CreditPolicyBody` 与 `SupplierAgreementBody` 两通道，受控 CLI 的
批文翻译跟上；事后补正文等于改一份已固定的正文，那要发新版本（既有声明通道同一条纪律）。

**四、目录读面**：`CommercialPolicyCatalogueRead.ListCreditPolicies`（按端口注释「正文表落库时
按封闭集扩方法」）；`SupplierAgreementCatalogueRow` 上扩正文字段并带 `HasContent`——壳在正文缺
是合法状态，与 `CustomerContractCatalogueRow.HasContent` 同一条理由。

**五、两件明确不做，写清是决定不是遗漏**：

- 供应商协议**不成方向列**。`Direction()` 在领域里恒为 `BUY`，存一列常量等于为同一件事立第二个
  口径，读回来若不是 `BUY` 反倒要人判是坏数据还是新语义；类别 CHECK（`object_kind = 3`）已把
  「这是一份采购协议」钉住。
- 供应商协议的**终止（`Terminate`）不入本表**。终止是生效后的一次事件，不是发布时的正文，
  形状与有效性更正（`0009`）同族——按版本追加、不改写原行。它要不要建册、建在哪，是另一裁，
  本票只登正文。今天 `SupportsProcurementAt` 对读回的协议因此永远看不见终止；这与今天完全
  没有正文可读相比是进步不是退步，但要写在这里免得被读成「已支持终止」。

## 边界

本票原写「不建表、不写迁移、不动领域模型、不扩端口」——那是裁决前的措辞，裁决落面后四件都在
本票范围内，上一节是它们的形状。领域对象已建好这一点对两条路都是资产。
上一节答的是**取值形态**这个前置岔口，不是本票动手建表。

有一件明确**不属本票**：`limitMinor` 之外的比例形态是领域缺口，若裁「补」则连带处理，若裁
「不补」则它随之搁置——本票不单独为它开票。

## Comments

- 2026-09-03 · MCP-2：**落地于 `877444a`，转 resolved。** 上一节「动手前定形的几处」五条全部照做，
  无偏离。落点逐层：领域 `CreditLimit` + `CreditPolicy` 改携它 + `SupplierAgreement` 补两个访问器；
  库 `0020_credit_policy.sql` / `0021_supplier_agreement.sql`；端口两具名 Save + 两点读口 +
  `ListCreditPolicies` + `SupplierAgreementCatalogueRow` 扩字段；应用两声明通道；受控 CLI 批文翻译；
  HTTP `?kind=CREDIT_POLICY` 与供应商协议行体正文键。

  **越出票面地盘的两处，写清**：`cmd/parcel-commercial/translate.go`（本上下文的受控 CLI，不翻它
  租户就登不了正文，等于链没通）；`cmd/parcel-api/unwired_orchestration.go` 的
  `unwiredCommercialCatalogue` 加一个方法——它实现 `CommercialPolicyCatalogueRead`，接口扩方法
  必然拆到它，是唯一一处别人地盘上的改动，已在频道报过。

  **「生产可达」差什么**：写侧经受控 CLI 可达；HTTP 发布口的 Intake 仍是未配置（PAR-INT-01 待提供，
  与本票无关）；两个点读口（`CreditPolicyContentView` / `SupplierAgreementContentView`）今天**零
  生产调用方**——它们的消费方是接受前财务控制的信用校验分支与 settlement-accounting 的供应商预期
  成本，两者都还没接。这与基线里「出名单不等于生产可达」那条注记的是同一件事。棘轮基线上
  `ResolveCreditPolicy` 那一行**未剪**：它仍然零生产调用点，本票没有为它造调用方，那属解析语义。

  **验证**：先在共享树跑，再按 parallel-sessions 处方在 detached 临时 worktree 检出 `877444a`
  重跑，两处都是——`gofmt -l .` 为空、`go build ./...` 与 `go vet ./...` 退 0、
  `go test -count=1 ./...` 零 FAIL、架构门禁含棘轮全绿。**这是「绿（含 PG）」**：DSN 指向门禁容器
  `idp-parcel-postgres-gate`，`internal/partycommercial/adapters/postgres` 一包约 59 秒，本票新增
  的真库用例在 `-v` 下逐条 `PASS`（不是 `SKIP`）——额度两格往返、缺正文 found=false、重放与冲突、
  租户绑定、CHECK 拒两空/两满/负值、目录上列。`-race` 本笔未跑，与本批其余票同归收尾那一批。

  **本笔的两轴评审**：独立评审子代理两次都因鉴权错误未能启动，退为串行自评。Standards 轴拿住三处
  ——两处注释写成了变更叙述（「曾缺席、0020 落库后补进来」）、一处「七种册子」计数，已就地改成只
  写现行理由；Duplicated Code 一处记为判断题不改：`creditLimitFrom` 在 CLI 翻译与 postgres 适配器
  各一份，形状同、译的表示不同（JSON 文档 vs SQL 行），抽到领域会把「两个可空指针」这种传输形状
  塞进领域 API。Spec 轴无发现。
