# `PP-S03` `PAR-SET-02/03` 证据与合成契约开发交接

状态：隔离 `S` 契约可验证；真实 SELL/BUY 参数仍为 `待提供`；生产结论保持 `No-Go / 待参数化`

## 目的

本切片把参考设计中可以吸收的小包计价语义收口为一组可交给开发的合成契约。它只验证 `parcel-pricing` 的纯评价边界、版本闭包、方向隔离、费用行语义和回放确定性，并明确评价交给 `settlement-accounting` 后的待判断边界。

本切片不把合成金额变成客户价格、供应商成本、费用、应收、应付、账单、付款或生产证据。所有执行记录的证据等级固定为 `S`。

## 权威边界

| 概念 | 权威上下文 | 本切片的处理 |
|---|---|---|
| 客户合同、供应商协议、价格方向授权、价卡绑定 | `party-commercial` | 只用命名夹具表达引用，不创建或修改商业依据 |
| 可执行价卡、规则版本、SELL/BUY `PricingEvaluation` | `parcel-pricing` | 形成并验证纯评价、解释、版本清单和回放 |
| 原始测量、包裹、履约发生项 | 各来源上下文 | 只以版本化输入引用进入合成快照 |
| 费用项目及计价费用代码唯一映射 | `settlement-accounting` | 只验证交接条件；不在计价上下文创建费用项目或映射 |

## 合成契约

### `SYN-PRC-SELL-01` 客户销售价评价

输入使用固定租户、业务范围、包裹、业务时点和合成测量；价卡方向为 `SELL`，目的为 `CUSTOMER_CHARGE`，价表族只使用当前首期允许的 `WEIGHT_ZONE` 骨架。可包含基础价和有明确顺序的固定附加费/折扣。

必须得到：

- `PricingEvaluation.Status = COMPLETED`，`Evidence = S`；
- 完整版本清单、版本内容摘要、计价重量、命中价表行、组合解释和语义摘要；
- 每条费用行都有稳定 `charge_code`、`scope`、`basis`、`method`、借贷方向、金额和来源引用；
- 评价结果仍只是纯评价。下游没有唯一费用项目映射时，结算结果必须是 `PENDING`，不得创建占位费用项目。

### `SYN-PRC-BUY-01` 供应商采购价评价

输入与 SELL 保持同一事实快照形状，但使用独立的 `BUY` 方案和 `SUPPLIER_COST` 目的。合成履约发生条件可以表达订舱、取消、失败尝试和实际履约，以及原旅程与替代/退运旅程的分离；这些条件是未来结算成本来源语义，不是金额或账单。

必须得到：

- BUY 评价拥有自己的方向、目的、版本清单和语义摘要；
- BUY 与 SELL 不共享可变价卡结果，不因同名服务或渠道自动继承价格；
- 未有真实供应商协议、收费发生条件或价卡时，只能形成隔离 `S` 或 `PENDING`，不能形成生产供应商预期成本。

### `SYN-PRC-GOV-01` 治理与回放

至少覆盖以下结果：

| 条件 | 评价结果 |
|---|---|
| 版本清单缺失、区间空档、币种/单位不一致 | `PENDING` 或 `CONFLICT`，按具体原因记录 |
| 同一范围命中互斥版本或区间 | `CONFLICT` |
| 同一输入、方向、目的、业务时点和版本内容重复评价 | 结果确定且语义摘要一致 |
| 使用原版本清单回放 | 形成新的评价身份，原评价不可变 |
| 版本引用相同但规则内容、金额或费用代码变化 | 回放 `CONFLICT`，不得静默替换 |
| 缺少费用代码到费用项目的唯一映射 | 由 `settlement-accounting` 保持 `PENDING`，不在 `parcel-pricing` 生成占位对象 |

### `SYN-PRC-COR-01` 方向独立更正

只更正 SELL 或只更正 BUY 时，追加对应方向的新评价/计价纠错依据；另一方向和历史评价保持不变。合成执行不能伪装成供应商贷项、客户退款、真实收付或核销。

## 开发验收映射

| 契约 | 现有/新增验证 | 通过标准 |
|---|---|---|
| SELL 费用行语义与组合 | `TestEvaluatePricingCalculatesMaxWeightAndOrderedFixedCharges` | 基础行和固定行的代码、范围、基数、方法、顺序和总额可复算 |
| BUY/SELL 隔离与证据等级 | `TestBuyAndSellPlansRemainIndependentAndReplayUsesFrozenManifest` | 方向、目的、价卡清单和 `S` 不混用；合成证据不能升级 |
| 待判断与冲突 | `TestEvaluatePricingReturnsPendingWithoutTotalForMissingFactsOrRate`、`TestEvaluationRequiresBusinessTimeInsidePlanAndRateTablePeriods` | 没有合格依据时无正式总额，不以零或默认值替代 |
| 版本/费用代码回放 | `TestReplayDetectsChangedPlanContentBehindSameVersionReferences`、`TestReplayDetectsChangedChargeCodeBehindSameVersionReferences` | 内容变化形成冲突，新评价不覆盖旧评价 |
| 输入事实引用保全 | `TestSyntheticContractPreservesFactReferencesAcrossEvaluationAndReplay` | 事实引用随输入快照保存，并在回放中保持一致 |
| 结算交接缺映射 | `AT-SA-174`（结算用例） | 只形成下游待判断；本切片不实现映射或费用对象 |

## 执行记录

每次合成执行至少保存：

- 夹具 ID/版本和执行代码版本；
- 合成合同/价卡/规则引用及内容摘要；
- 输入事实引用、业务时点、方向、目的和版本清单；
- 预期与实际费用行展开、结果状态、问题代码和语义摘要；
- 回放关系、执行/复核方、隔离端点和证据索引。

合成执行统一标记 `S`。即使夹具数值来自参考设计案例或仓库参考价卡，也不能把执行等级改写为 `R` 或 `P`。源文件身份与 Schema 指针已按[源完整性门禁](../domain/parcel-pricing/CONTEXT.md)结案，SHA-256 不可复核且不会再闭合；将来取得真实来源时必须另建真实执行记录，不能把合成执行记录改写成 `R` 或 `P`。

## 明确不做

- 不创建 `settlement-accounting` 代码包、费用项目、映射表、结算账户、费用明细或账期；
- 不猜测客户/合同/方向/适用期等费用映射唯一键；
- 不引入独立计价平台、Quote/锁价、批量导入、周期聚合、复杂多币种或任意脚本；
- 不把「计算目的」扩展为参考设计的完整 `CalculationPurpose`。该语言决策已由 [`REF-OPEN-01`](./pp-reference-design-absorption-coverage.md) 定案：首期取值闭合为三个，与价格方向一一成对，取值集合与配对规则一并重新确认才可拓宽；
- 不用合成 `S` 解除 `PAR-SET-02/03`、`SET-01/10` 或 PN-08 生产门禁。

## 进入真实参数后的下一门

只有取得一条真实 SELL 主链和一条真实 BUY 主链，并补齐责任法人、合同/协议、价卡版本、适用范围、币种/换算依据、费用项目映射、收费发生条件、源文件身份与可复算样例后，才重新评估是否需要实现 `settlement-accounting` 的最小采用器。映射维度必须由真实证据决定，不由本切片预设。

