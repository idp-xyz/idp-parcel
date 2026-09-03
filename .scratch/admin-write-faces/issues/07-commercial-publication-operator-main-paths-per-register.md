# 07 商业发布九类的运营主路径：按 ADR-0101 决定八逐册裁形

Category: enhancement
Status: draft——伞票；每册拆子票时先写「选哪一形与理由」，不拆完不转 ready-for-agent
Blocked by: 无（票 03 已落 JSON 镜像签）

## 缺什么

票 [03](./03-publication-write-face-blocked-by-two-misaligned-closed-sets.md) 给发布口落了
一签「受控发布（JSON 镜像）」。按 [ADR-0101](../../../docs/adr/0101-operator-facing-registration-payload-shape-is-product-defined.md)
决定一，那是受控批量口的在线镜像，**不是运营配置员的主路径**；决定八要求每册在自己的
实施票里写明选了哪一形（逐字段表单 / 模板导入 → 草稿 → 批准 → 发布）与理由。九类发布对象
今天一册都没裁。

## 九类，各自先答「登记频次 × 操作者角色 × 载荷结构」

| 对象类别 | 显示在 | 初步印象（不是裁决） |
|---|---|---|
| `SERVICE_PRODUCT` | 服务产品页 | 低频、结构简单 → 逐字段表单候选 |
| `CUSTOMER_CONTRACT` | 客户与合同页 | 低频、正文（`0012`）有几组引用 → 逐字段表单候选 |
| `SUPPLIER_AGREEMENT` | 供应商协议页 | 同上（`0021`） |
| `ACCEPTANCE_RULE_PACKAGE` | 政策页·接单规则包册 | 正文是规则集合 + 多种声明通道（时点锚、收寄资格、终局规则）→ 要先答声明随发布怎么在表单里表达 |
| `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` | 今天无册（票 06） | 先解票 06 |
| `PRICE_RULE` | 政策页·商业价格政策册 | 正文是方向 × 方案绑定 + 口径（`0010`/`0022`）→ 逐字段表单候选，方案引用要能从价卡目录选 |
| `SETTLEMENT_POLICY` | 政策页·结算政策册 | 低频 → 逐字段表单候选 |
| `CREDIT_POLICY` | 政策页·信用政策册 | 额度两键恰一（`0020`）→ 表单要把「金额 / 比例」做成二选一 |
| `AUTHORIZATION_RULE` | 政策页·授权规则册 | 取消授权按请求方逐格 → 表单要能加行 |

**印象不是裁决**：拆子票时逐册按决定八写理由；价卡首例走了模板导入（ADR-0101 决定二），
但那是矩阵型，这九类没有一类是矩阵。

## 硬句（从 ADR-0101 与票 pricing/08 带过来，逐册都适用）

- 表单不算摘要、不裁任何门：`contentDigest` 与受理都由服务端答，表单只呈现。
- 若某册要「提交前预览」，预览与发布共用一份规范化路径，同一份载荷过预览与过发布得到逐字节
  相同的摘要（决定四）。
- 操作者身份从 ADR-0100 的 `OperatorEnvelope` 来，表单不收也不送批准人。
- 每册子票开工前占 `cmd/parcel-api/endpoints.go`（若要加预览端点）与该页文件。

## 边界

本伞票不写代码。JSON 镜像签保留为高级口（票 03 已落），不因主路径落地而删。
