# 计价参考序列登记册缺失,燃油与汇率序列无处登记

Category: enhancement
Status: resolved

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W15。

## 墙

`REFERENCE_SERIES_UNRESOLVED`、`EXCHANGE_RATE_UNRESOLVED`(`parcelpricing/domain/evaluation.go`,评价落 `EvaluationPending`)。墙正确:序列解析不到时评价挂起,不编数值。

## 现状:门四件全缺

ADR-0013 已裁「计价拥有计价参考序列的登记、版本化、发布治理与按基准时点的解析」,但机制未建:`reference_series.go` 是纯内存领域对象,无表、无装载口、无写入方、无登记口。序列今天只能作为评价入参由测试构造。

## 缺的最小机制件

1. 序列登记册:表 + 迁移(来源标识、生效区间、逐期取值、取值凭证引用、登记责任方;取值更正形成新序列版本,不追溯改写)。
2. 装载口:按计价基准时点解析取值,供评价消费并写入版本清单。
3. 登记口:登记用例(缺可复核凭证的期次只有断言强度,只准隔离验证——登记结构须能表达这个等级)。

## 红线

- 序列数值由外部产生(燃油=承运商公布,汇率口径=party-commercial 商业价格政策声明);计价只登记不生产,不选定商业口径(ADR-0013)。
- PAR-SET-11 实例(两个序列的真实期次)待提供是常态;隔离取值不得冒充生产序列(PN-07 准入门槛原文)。

## 参照

ADR-0013;PAR-SET-11;`docs/domain/parcel-pricing/CONTEXT.md`。

## Comments

- 2026-08-20 MCP-2：对 `3324ecb` 重核四件，**结论不变：无门，四件全缺**。
  `reference_series.go` 仍是纯域内对象（`parcelpricing/domain/`），序列无表
  （`migrations/parcel_pricing` 仍只有 `0001_evaluation.sql`）、无端口
  （`ports/ports.go` 三口如旧）、无适配器（`adapters/postgres` 仅评价两件）、无进程
  入口（`cmd` 零引用）。基线 `49a2ab0` 以来 parcel-pricing 零提交，票面与代码无矛盾。
  与票 07 同根：两族配置的门都等同一套 PP 持久化骨架，先后与合并由 triage 定。
- 2026-08-21 · MCP-2：三件（+进程口）落地随本提交置 resolved（task-11e8da0c，分支
  `mcp2-pp-catalog`，与票 07 同批）。
  1. **登记册**：迁移 `0003_reference_series_register.sql`——`parcel_pricing.reference_series_version`，
     键（租户+序列+序列版本），行只增不改；CONTEXT 硬句逐条落 CHECK——种类封闭、汇率必带
     商业价格政策口径（不接受未声明口径的裸汇率）、口径两列成对、区间有序、更正两件成对
     且不自指（取值更正=新序列版本，先例：supplier_expected_cost 纠错成对）。逐期取值与
     凭证在领域折装快照内（含摘要自校），`evidence_grade` 列汇总整版等级：任何一期缺可
     复核凭证即 ASSERTED——断言强度只准隔离验证，不得支撑生产金额。
  2. **装载口**：`ports.ReferenceSeriesRegister.ResolveAt`（租户+序列版本引用+计价基准
     时点）——命中期次给出可冻结进评价输入的 `ReferenceSeriesValue`（汇率带口径），断言
     强度随答案带出；版本不在册或时点落在期次缺口都答未解析，评价侧据以挂起不编数值
     （对应票面 REFERENCE_SERIES_UNRESOLVED / EXCHANGE_RATE_UNRESOLVED 的墙）。
  3. **登记口**：应用 `RegisterReferenceSeriesHandler` + 受控 CLI `cmd/parcel-pricing-register`
     （`-kind reference-series`，与票 07 共用一个进程口）。
  红线守住：零期次生产默认，夹具全 SYN-PRC 合成序列（S 级只记 S）；数值不归计价生产、
  口径不归计价选定（ADR-0013），登记结构原样表达这两条所有权。快照形状自立 PRS-1 号，
  与 PPC 族同一条 ADR-0014 纪律、各自演进。验证：worktree 全仓 `go build`/`go vet`/
  `go test -count=1` 绿，真库用例 `-v` 实跑 PASS 非 SKIP。评价用例接解析口（消费面）
  不在本票。
