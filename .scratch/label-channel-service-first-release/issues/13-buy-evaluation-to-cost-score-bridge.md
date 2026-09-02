# 13 `BUY` 评价到成本准则分值之间没有桥，也没有逐候选批量评价口

Category: enhancement
Status: in-progress——MCP-4 认领后崩溃，MCP-6 接手；三道缝均已落地待评审，见文末 Comments
Blocked by: 01（已 resolved：出局判断落**渠道择优比较器**这一层，`parcelpricing` 只答算不算得出价）

## 缺口

两头都齐，中间断开：

- **`BUY` 侧评价齐备**：`internal/parcelpricing/domain/value_objects.go` 的
  `PricingDirectionBuy` 与 `PricingPurposeSupplierCost` 配对，装载按方向隔离
  （`adapters/postgres/price_card_catalog.go` 的 `LoadApplicable`），评价用例在
  `application/evaluate_pricing.go`；计费重与体积系数机制在 `domain/weight_rounding.go`、
  `domain/dimensions.go`，价表族在 `domain/rate_families.go`。
- **比较器齐备**：`SelectRouteCandidate` 按准则序作字典序比较，`CriterionScore` 越小越优。

断开的是：`CriterionScore` 的注释明写「折算发生在事实形成处」，而**「把某候选的
`PricingEvaluation` 总价折成 `COST` 准则分值」这一步今天没有实现方**；`parcelpricing` 侧
也没有「一次输入对多份 `BUY` 价卡逐份评价」的批量口。

## 做什么

两件：

1. **批量评价口**：一次输入（同一批包裹尺寸重量）对多份 `BUY` 价卡逐份评价。
   `PAR-NET-16` 要求「各候选按自己的体积系数与进位算出计费重之后再比总价」——**逐候选各
   算各的计费重**，不是算一次再套不同费率。这是本票最容易做错的一格。
2. **成本分值桥**：把逐候选的 `PricingEvaluation` 折成 `COST` 准则分值。

## 必须守住的一格

`PAR-NET-16` 硬约束：「候选评价为待判断或不可计价时该候选出局，**不得以零金额顶替**」。
`parcelpricing` 有 `EvaluationPending` 这一格（`domain/evaluation_test.go` 的
`DIMENSIONS_REQUIRED`/`RATE_NOT_FOUND`），本票要消费它——**折成零分就是把「算不出」变成
「最便宜」**，那会让不可计价的候选每次都赢。

## 红线

- 不填任何价卡、体积系数、进位分段、金额（`PAR-SET-03`，实例半边）。
- 出局判断放在哪一层已由 `01` 裁定：落**渠道择优比较器**这一层，不放 `parcelpricing`。
  `parcelpricing` 只答「这个候选算不算得出价」（既有 `EvaluationPending` 那一格），
  「算不出就出局、且不得以零金额顶替」是择优的规则，由择优侧守。本票照此落，不自定。

## 完成判据

批量评价口与分值桥各有实现与测试，含「待判断/不可计价候选出局且不以零金额顶替」的用例；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第三段；票 `01`；参数登记册 `PAR-NET-16`。

## Comments

- 2026-09-02 MCP-6：接手 MCP-4 的崩溃现场。它停在 `/tdd` 切片 3 的 red 与 green 之间——测试
  引了尚不存在的 `ErrChannelCandidateCostTied`，`parcelshipment/domain` 整包编译不过。三道缝
  现均已落地：

  **缝一 · 批量评价口**（`parcelpricing/domain/batch_evaluation.go`）。签名收成「一份输入 +
  N 份价卡」，于是「逐候选各算各的计费重」是结构给的：调用方没有地方塞一个自己算好的计费重。
  独立判据取 `PAR-NET-16` 原话「单价更低的候选可以因此总价更高」——两档费率都更低的卡因自己
  的体积系数把计费重推高一档，总价反超；「算一次再套不同费率」那个写法在这一格必选错。
  「一个候选算不出不得中断其余」那一格**没经过红**（缝一的实现干脆不返回 error，把它提前
  满足了），已用一次变异（遇非完成即 return）坐实测试有牙，再还原。

  **缝二 · 成本分值桥**（`parcelshipment/adapters/parcelpricing/cost_bridge.go`）。位置由
  ADR-0025 与 `TestBusinessModulesDoNotReachIntoEachOther` 定死。桥不做出局判断（票 `01` 已把
  它裁给择优那一层），只逐格翻译；另加一格拒绝：`SELL`/客户收费的评价不得读成供应商成本，
  它结构合法、金额齐备，译过去一路绿而下游看不出这个数字答的是另一个问题。

  **缝三 · 择优比较器**（`parcelshipment/domain/channel_candidate_cost.go`）。在 MCP-4 已落的
  两片之上补了三片：并列交冲突（票 `01` 裁决四）、跨币种拒绝比较、金额按十进制数值比而非
  字符串序。

- 2026-09-02 MCP-6：**改了 MCP-4 已写好的成本表示，经 owner 当场裁定**。原设计存
  `minorUnits int64`，但 `minorUnits` 在全仓只出现于该票两个新文件，**没有任何币种小数位表**
  ——USD 要 ×100、JPY 要 ×1，今天没有任何桥能正确填它，编一张表出来就是把未确认参数写死成
  生产默认。改为照 ADR-0048 对客户申报数字的同一立场**原样保全十进制**，PS 侧自带一个十进制
  比较。择优不需要标度：跨币种已拒，同币种内数值序本身就够定谁最便宜。

- 2026-09-02 MCP-6：出局格由一格扩到四格，与 `parcel-pricing` 的四种非完成结果一一对应
  （其 CONTEXT：「四者不得互相替代」）。合并的诱惑在于择优对四格的即时处置都是「出局」，
  但四者续办完全不同（补事实／换渠道／等治理裁决／重试），而票 `14` 的落选留痕要答的正是
  这个。

- 2026-09-02 MCP-6：`gofmt -l` 空、`go build ./...` 与 `go vet ./...` 退 0、`go test ./...`
  除下述一处外全绿，**未接真库**。生产接线棘轮要求为两个新导出工厂各加一行基线并写明为什么
  现在不接（成因同一条：候选装配是票 `12`，未落地），已加；按基线文件自己的规矩重数条目数，
  改前 32、改后**实测 34**。余下那一处红不属本票：`partycommercial` 的
  `RehydrateChannelAccountUseAuthorization` 来自 MCP-5 同时在途的改动，已报回其通道。
  机制清点**未跑**——共享树上同时摆着两个会话的新增 `.go` 与一份迁移，谁先跑都会把对方的
  增量写进生成物；留待提交时在干净树上补。

- 2026-09-02 MCP-6：本票的完成判据已满足（批量口与分值桥各有实现与测试，含「待判断/不可
  计价候选出局且不以零金额顶替」的用例）。**未接生产调用路径**，比较器与批量口都还没有
  应用层调用方，那一步等票 `12`。
