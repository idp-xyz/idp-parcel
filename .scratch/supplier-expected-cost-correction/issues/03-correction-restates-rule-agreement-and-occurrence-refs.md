# 纠错版本重述采购规则版本、协议引用与发生项版本

Category: bug
Status: in-progress
Blocked by: 01

按 [ADR-0067](../../../docs/adr/0067-cost-correction-restates-the-whole-evaluation-result.md) 决定二落地
**引用那一组**。金额与币种那组在 `02`。

## 病灶

`AppendCorrection` 换掉评价引用却把三处引用原样留在旧评价上：

- `ruleVersion`（采购价格规则版本）——`AT-SA-054` 的成因就是采购计费规则被更正，新评价必然引新
  规则版本，纠错版本却仍写旧的。
- `agreement`（供应商协议引用）——同属评价的版本清单。
- `occurrence`（运输收费发生项引用，含版本与业务时点）——`OccurrenceVersion` 的类型注释自己写着
  「发生项有效性更正换版本，预期成本据以追加计价纠错」，而 `AppendCorrection` 不换它。`AT-SA-164`
  的「运输履约使原发生项失效 → 保留原预期成本并追加计价纠错」正是这条路径。

与 `02` 是同一个毛病：换了评价只搬来一部分产物，一份版本的内容出自两个评价。

## 先取证再改

**这是本票与 `02` 分开的唯一理由。** `UC-SA-004` 逐行匹配是「按版本引用」读预期成本的，改完之后
它会读到一份采购规则版本与首版不同的纠错版本。要先回答：

1. `UC-SA-004` 的匹配是否依赖「同一成本链上各版本的规则版本一致」？今天读侧是
   `ports.ExpectedCostView` / `SupplierExpectedCosts.LoadExpectedCost`，按（租户+版本）取单行，
   看上去不依赖，但要核到用例文档而不只是当前实现。
2. 发生项引用的 `ID` 必须保持不变（那是身份四件之一，ADR-0067 决定三），只有版本与业务时点随
   评价走。要确认 `TransportChargeOccurrence` 换新引用时有一条挡住 ID 变化的检查。

取证结论写回本票 `## Comments` 再动代码。

## 要改的（取证通过后）

- `AppendCorrection` 入参长出发生项引用、采购规则版本、协议引用；发生项 `ID` 与被纠正版本不一致
  时拒。
- 幂等三维不必改：`supplier_expected_cost_first_version_unique` 的 `WHERE prior_version IS NULL` 已把
  纠错版本放在三维之外，纠错版本携带新规则版本不撞索引。这一条要有测试守住，否则下次有人看到
  「纠错版本的规则版本和首版不同」会以为是 bug 再改回去。

## Comments

- 2026-08-20 · MCP-3：随 `01` 的裁断创建，`Status: draft` 等 MCP-1 派工。取证那一步没做，本票不含结论。
- 2026-08-20 · MCP-3：「先取证再改」两问取证完成（只读，未动代码）。

  **问 1（`UC-SA-004` 是否依赖同链规则版本一致）：不依赖。用例文档与读侧实现都核过。**

  文档面：匹配建立在金额范围上（主流程「按金额范围建立一对多、多对一、部分和未匹配关系」）；
  规则版本在 UC-SA-004 里只出现在审计记录项（「审计至少记录……协议/履约/成本版本」）与权威
  依据的所有权句（`PC-RULE`），是记录项不是匹配判据。一致性节明文「账单匹配与内部成本更正
  并发时分别保留来源，再按有效版本形成新匹配或调整，不按接收顺序覆盖」——文档自身预设纠错
  后按新有效版本重建关系。反证更硬：UC-SA-002 的 `AT-SA-054`（「供应商计费规则更正但客户规则
  未变 → 只追加供应商预期成本计价纠错」）里纠错版本携带新规则版本正是该验收行的成因；若
  UC-SA-004 要求链上规则版本一致，两份用例互斥。

  实现面：`ports.ExpectedCostView` 自注「按版本取回供应商预期成本供逐行匹配引用」，按（租户+
  版本）单行读；唯一消费方 `receive_supplier_bill.go` 按 `directive.ExpectedVersion` 逐行取，
  从不沿 `PriorVersion()` 走链（SA 包内该方法的调用点只有 postgres 适配器的序列化与测试）。
  `MatchBillLine` 从预期成本只读两样：在场性（`expected.Version().valid()`）与
  `SettlementAmount()`（结算币种对账单币种、结算金额对主张金额）；`RuleVersion()`、
  `Agreement()`、`Occurrence()` 都不进匹配。`BillLineMatch` 与 `FormAuditedPayable` 只存版本
  引用。改完后匹配、审核、审计行为不变，审计反而记到该版本真实采用的依据。**无阻塞。**

  **问 2（有没有挡发生项 ID 变化的检查）：没有——今天也没有任何路径能换引用，守卫必须随本票
  新写。**

  今天 `AppendCorrection` 不接发生项入参，`corrected := cost` 整份拷贝，ID 不可能经这扇门变化
  ——现状靠「没有入口」，不是靠检查。领域内无跨版本比对：`NewTransportChargeOccurrence` 只验
  四件非空；`RehydrateSupplierExpectedCost` 是单行门（ADR-0028），只验回指与原因成对且不自指，
  不取前一版，结构上无从比对 ID。库面 0008 全部约束是单行 CHECK，`prior_version` 无自表外键、
  无触发器——occurrence_id 与被纠正行不同的纠错行今天照收；适配器 `Save` 是单行
  `INSERT ... ON CONFLICT DO NOTHING`，不查前一版。因此票面「发生项 `ID` 与被纠正版本不一致
  时拒」必须是 `AppendCorrection` 里的新检查——那是唯一同时握有被纠正版本发生项与新发生项的
  地方，一行比较即可；重建门与库面结构上守不住。若还要持久化面兜底（自表外键+触发器一类），
  属另一个决策，不随本票顺手做。守 ID 即可：原因、版本与业务时点都随新引用走，ADR-0067 决定
  三只钉身份四件。

  **顺带核实**：幂等三维的 `supplier_expected_cost_first_version_unique` 部分唯一索引条件
  `WHERE prior_version IS NULL` 属实，纠错版本带新规则版本不撞索引——票面「要有测试守住」的
  前提成立。

  取证到此；动代码仍停在 MCP-1 与用户点头之前。
- 2026-08-20 · MCP-1：**取证部分派 MCP-3**（只读：两问结论核到用例文档与代码后写回本票
  Comments）。「要改的」代码部分与 `02` 同受一道用户点头约束，取证结论落回前后都不得开工。
