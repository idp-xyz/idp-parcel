# 价卡无版本仓储与装载口,三价表族机制已实现却没有一张真卡能放进系统

Category: enhancement
Status: resolved

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W14。票面点名的三疑似无门之一。

## 墙

评价按无适用价卡落`不可计价`/`待判断`(`parcelpricing/domain/evaluation.go` 的结果代数)。墙本身正确——`不可计价`只表示按该价卡无法形成价格,不推导业务终局。

## 现状:门四件全缺

- `PricingPlanVersion` / `RateTableVersion` 是纯内存领域对象(`plan.go`、`rate_table.go`),parcel_pricing 迁移只有 `0001_evaluation.sql`。
- `parcelpricing/ports` 只有 `EvaluationStore`、`EvaluationHandoff`、`Clock`——没有价卡目录端口,遑论实现。
- `evaluate_pricing` 用例未接任何进程;评价的价卡入参今天只能由测试构造。
- 三个价表族(`WEIGHT_ZONE`、`FIRST_CONTINUE`、`UNIT_PRICE`,规范化版本 PPC-2)机制已全部实现(PAR-SET-03 已更新),但实现支持≠可配置:一份真 BUY/SELL 价卡今天没有任何地方可放。

## 连带

接受链的控制金额缝(`CONTROL_AMOUNT_NOT_CONFIGURED`,清单 W07)等估价,估价等价卡装载口——本票是那条缝的前置。

## 缺的最小机制件

1. 价卡版本仓储:表 + 迁移(方案/价表版本、价格方向隔离、适用期、源文件身份与 SHA-256、规范化版本)。
2. 装载口:按(方向+适用范围+计价基准时点)取版本化价卡,供评价用例消费。
3. 登记口:价卡登记用例(含 BUY/SELL 方向授权引用;发布批准责任)。

## 红线

- 价卡内容全部属实例半边(PAR-SET-02/03 待提供,且价表族激活以取证为准入门槛);本票只建仓储与口,验证用 SYN-PRC-* 隔离夹具,S 级只记 S。
- 内容摘要携带规范化版本、只在同一版本内可比(ADR-0014),仓储形状须承载它。

## 参照

PAR-SET-02、PAR-SET-03;ADR-0014;`docs/domain/parcel-pricing/CONTEXT.md`。

## Comments

- 2026-08-20 MCP-2：对 `3324ecb` 重核四件，**结论不变：无门，四件全缺**。仓储——
  `migrations/parcel_pricing` 仍只有 `0001_evaluation.sql`（评价是运行时事实，不是配置）；
  装载口——`parcelpricing/ports/ports.go` 仍只有 `Clock`/`EvaluationStore`/`EvaluationHandoff`
  三口，无价卡目录端口；写入方——`adapters/postgres` 仍只有 `evaluation.go` 与
  `evaluation_handoff.go`；登记口——`cmd` 全树无 `parcelpricing` 引用，`evaluate_pricing`
  仍未接任何进程。三价表族机制原样在（`domain/rate_families.go`）。基线 `49a2ab0` 以来
  parcel-pricing 零提交。**特核**：PP 仍是全库唯一「配置仓储本体都缺」的上下文——同期
  NR 已长出版本化网络目录七表骨架（`3b9f212`，`0008_network_catalog.sql`，ADR-0068），
  其余上下文配置表俱在（缺的是写入方/登记口层），仅 PP 的价卡与参考序列两族连表都没有。
- 2026-08-21 · MCP-2：四件落地随本提交置 resolved（task-11e8da0c，分支 `mcp2-pp-catalog`，
  与票 08 同批——两族共用同一套 PP 持久化骨架）。
  1. **仓储**：迁移 `0002_price_card_catalog.sql`——`parcel_pricing.price_card_version`，
     键（租户+方案+方案版本），行只增不改；方向/目的封闭、SHA-256 形状、适用期有序等
     CHECK 把领域不变量在库内再守一遍。权威内容在领域折装的登记快照（方案全图 + 源文件
     身份 + 方向授权引用 + 发布批准责任方），列面只做比对与检索。
  2. **装载口**：`ports.PriceCardCatalog.LoadApplicable`（方向 + 适用范围 + 计价基准时点），
     读回经领域整图重验（含按规范化版本重算内容摘要自校）；同一方案两版同时适用交回
     `ErrAmbiguousPriceCard`（先例：NR 目录），多候选全返回不择优。
  3. **写入方**：`adapters/postgres/price_card_catalog.go`，结果代数四分——已登记/幂等
     重放/版本内容冲突/规范化版本不同（摘要只在同一规范化版本内可比，ADR-0014），原行
     永不被顶替。
  4. **登记口**：应用 `RegisterPriceCardHandler` + 受控 CLI `cmd/parcel-pricing-register`
     （`-kind price-card`，输入为领域登记快照 JSON，重建门在入库前拒；独立进程，未碰
     `parcel-api`/`parcel-dispatch`；退出码 0 登记或幂等重放 / 1 输入拒 / 2 治理答案 /
     3 未决）。
  红线守住：零生产默认行，验证夹具全为 SYN-PRC 合成价卡（S 级只记 S）；源文件身份只登
  名称与 SHA-256（真文件外置）。验证：worktree 全仓 `go build`/`go vet`/`go test -count=1`
  绿，真库用例 `-v` 实跑 PASS 非 SKIP。评价用例接装载口（消费面）不在本票，见票面
  「连带」：控制金额缝那条等它。
