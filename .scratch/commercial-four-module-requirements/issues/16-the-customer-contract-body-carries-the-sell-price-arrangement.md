# 16 客户合同版本的正文带销售价格约定：有序的销售价格来源，或显式不计销售价

Category: enhancement
Status: ready-for-agent
父票：[spec](../spec.md)
Blocked by: 无
地盘：`internal/partycommercial`（领域、规范化、发布与受控批文）、`cmd/parcel-commercial`、`docs/application/party-commercial`

对应要求：Contract「价卡授权」。规则在参与方与商业 `CONTEXT.md`「客户合同版本」词条与 ADR-0380 决定一、七。

## 现象

取证钉 `54b32c1b8`。`CustomerContract` 的正文只有货主客户账户、接单规则包引用与按费用范围的财务控制约定。合同不说价；一位客户用哪套销售价，今天由他的结算账户在两本采用册上登的计价范围定。

## 要做的

1. **正文加一格销售价格约定**，二选一：一份有序的销售价格来源列表（至少一项，每项指名一个计价范围，同一范围不出现两次）；或显式声明不计销售价并留依据。形照同一份正文里的 `FinancialControlBinding`。新形状下不存在「没说」。
2. **换规范化版本号**（[ADR-0267](../../../docs/adr/0267-a-key-added-to-an-already-canonicalized-body-is-a-shape-change-even-when-old-documents-stay-byte-identical.md)）。旧版本的合同照旧读得回、摘要不变，读出来是「没有销售价格约定」，不补默认。
3. **录入、批准与发布面带上这一格**：在线口、受控批文与待批准发布的正文快照。批准与发布绑定的内容摘要随之包含它。
4. **读面带出这一格**，供管理台（票 [20](./20-admin-web-contract-page-declares-sell-price-sources.md)）与闭包固定（票 [17](./17-sell-price-sources-are-fixed-with-the-acceptance-time-closure-and-read-by-settlement.md)）用。
5. **不核范围在计价一侧有没有卡。** 参与方与商业不读计价的册（ADR-0380 决定一「参与方与商业不核列表里的范围在计价一侧有没有卡」）。

## 要裁的

- 新规范化版本要不要照计价那一侧的做法设部署开关（计价是 [ADR-0352](../../../docs/adr/0352-registering-a-price-card-in-a-canonicalization-newer-than-the-base-one-needs-an-explicit-deployment-switch-that-is-off-by-default.md)）。参与方与商业今天有没有同类的门，开工时先核。
- 「显式不计销售价」留不留，见 ADR-0380 越权风险点 9。PC owner 复核之前照 ADR 做。

## 三类

正文的形、规范化与发布面是机制。每份合同写哪几个范围、排什么顺序，是租户取值：产品不预选范围，不给默认的标准价。

## 完成判据

- 领域构造门拒收：列表为空、同一范围出现两次、两种都没声明、两种都声明、不计销售价而没有依据。
- 一份带销售价格约定的合同版本经录入、批准、发布后读回，列表顺序与录入相同；摘要随列表的内容与顺序变。
- 旧规范化版本的合同读回不变，摘要不变。
- `UC-PC-001`「给开发的交接」里讲销售价格约定的那一段随之删去或改成现状。

## Comments

- 2026-11-12：由票 15 拆出。
- 2026-11-12（激活）：用户在问题卡上选定激活票 16 至 21，本票转 `ready-for-agent`；此前 `draft`，等 ADR-0380 进 `main`（已在 `486a4e8fe` 进）。
