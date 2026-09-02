# 竞品公开事实台账

状态：本仓自行核对的竞品公开事实登记处。核对日期 **2026-09-02**，取证方式为逐条抓取来源页正文。

本文是**台账**，只回答「某条竞品事实我们何时在哪里核过、核到了什么」。它不定义任何产品规则、不描述本产品能力、也不作验收依据——产品口径以[产品基线](./PRODUCT-BASELINE.md)与[首发开发主线](./PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)为准，对外叙事以[产品故事](./PRODUCT-STORY.md)为准。

来源清单取自[产品战略与产品蓝图 V1.0](../reference/IDP_Parcel_Product_Strategy_Blueprint_V1.0.md)的附录 A（该文自标核对日期 2026-08-27）。**那份蓝图非本仓权威**，其余章节不在本台账范围内；本台账只承接它的来源清单，并把每条重新核过一遍，标上我们自己的结论。蓝图与本台账不一致处，以本台账为准。

竞品定价与功能矩阵随时变更且不另行通知。本台账记的是**核对时点的公开表述**，不是持续有效的事实，也**不得作为本产品定价、报价或成本假设的依据**。要引用某条，连核对日期一起引；超过一个季度未重核的条目视为过期。

## 三态定义

| 状态 | 含义 |
|---|---|
| `已核实` | 抓取到的正文明确支持该事实 |
| `部分未核` | 页面可达但关键数字未出现在正文中（多为前端动态渲染），只核到定性部分 |
| `未取到` | 页面不可达或已失效，本仓无法确认 |

`未取到`不等于该事实不成立，只等于我们没核过。它不得被引用为已核实。

## Shippo

| 编号 | 来源 | 状态 | 核到了什么 |
|---|---|---|---|
| `S1` | [Shipping API Pricing](https://goshippo.com/pricing/api) | 部分未核 | 已核实：API Starter 每月前 30 个 label 免费、之后 7¢/label；Rating 1¢/rate generation；delivery date estimate 3¢/estimate；保险按声明价值加运费计费，美国境内 1.25%、国际 1.5%；API Premier 为定制价并含量级折扣。**未核**：蓝图记的 tracking 每次 $0.02 与地址验证美国 $0.02／非美国 $0.08——正文里 tracking 与两档地址验证的 Starter 单价未渲染出来 |
| `S2` | [App Pricing](https://goshippo.com/pricing) | 已核实 | Starter 免费、每月至多 30 个 label，连自有承运商账号时 5¢/label；$17/月一档对应 1–200 label/月，连自有承运商账号免费，年付 $205 标 10% 优惠；Premier 为定制。另一条蓝图未记而有用：与 Shippo label 无关的 API 调用（tracking、rating、地址验证）与 Estimate API 按 API Starter 价另计 |
| `S3` | [Shipping API](https://goshippo.com/products/api) | 已核实 | 单一 API 提供 label、rating、tracking、地址验证、退货、账单与对账；折扣宣称至多 90%，单请求至多 10,000 shipment；rate 覆盖 40+ carrier，tracking 覆盖 1000+ carrier |
| `S4` | [Rates API](https://docs.goshippo.com/api-reference/rates/retrieve-shipment-rates) | 已核实 | Rate 对象带 `servicelevel`、`provider`、`amount_local` 等；文内明确 transaction 即「向服务商购买面单」这一动作 |
| `S5` | ~~`docs.goshippo.com/shippoapi/public-api/transactions/createtransaction`~~ | 未取到 | 蓝图给的地址返回 **404**。现行地址为 [Create a shipping label](https://docs.goshippo.com/api-reference/transactions/create-a-shipping-label)，语义仍成立：既可凭已创建的 rate 购买面单，也可用 shipment 明细加 carrier account 加 service level token 即时购买；返回的 transaction 带 `label_url`，`label_file_type` 可选 PDF/PNG/ZPLII 等 |

## Easyship

| 编号 | 来源 | 状态 | 核到了什么 |
|---|---|---|---|
| `S6` | [Overview Flow Guide](https://developers.easyship.com/docs/overview-flow-guide) | 已核实 | 标准五步：创建 shipment（rate 内联在响应里）→ 取 rate → 更新 shipment → 生成 label → 取轨迹。两处值得记：生成 label 即触发发运并把信息交给承运商；轨迹既可主动查 Trackings API，也可订 `shipment.tracking.checkpointscreated` 等 webhook |
| `S7` | [Plans](https://www.easyship.com/plans) | 部分未核 | **未核**：各档订阅月费——正文里价格位是动态渲染，抓到的全是占位。**已核实**且蓝图未记的更硬：高级 API 端点逐个计量，含额与超额单价按档递减（Rates API 依次为 0 含/$0.005、2,000 含/$0.005、10,000 含/$0.004、50,000 含/$0.003；地址验证、Tax & Duty、HS Code、站外 tracking 各自成行）；Easyship 自家 Tracking API 全档不限量免费；只计成功调用；LYOC 与「自有承运商账号面单费」是套餐矩阵里的两行（金额未渲染） |
| `S8` | [API Usage Billing](https://support.easyship.com/hc/en-us/articles/34766246620562-API-Usage-Billing) | 未取到 | Cloudflare 人机验证挡住，正文取不到。其要点（含额加超额、只计成功调用）已由 `S7` 与 `S10` 独立支持 |
| `S9` | [LYOC Usage Billing](https://support.easyship.com/hc/en-us/articles/34766491011602-Usage-Billing-for-Link-Your-Own-Courier-Account-LYOC-Customers) | 未取到 | 同上。LYOC 这一形态本身由 `S7` 的套餐矩阵行确认存在，但「平台按每张 label 收多少」这个数**本仓未核**，引用时不得当已核实 |
| `S10` | [2026 API Upgrade](https://www.easyship.com/blog/easyships-upgraded-global-shipping-api-for-ecommerce) | 已核实 | 自 2026 年 1 月起高级 API 端点全面开放，无需定制合同；按用量计费，免费档为纯按次付费，付费档为含额加超额；超出含额不中断服务，只按次收超额；覆盖 550+ courier service，宣称折扣至多 91% 且无需自有承运商账号 |
| `S11` | [Rates Request](https://developers.easyship.com/reference/rates_request) | 已核实 | 文档明写可比 cheapest、fastest 与 best value for money，或速度、价格与可靠性的组合；响应带 `value_for_money_rank`、`tracking_rating` 及时效与总价的排名字段。**权重公式未公开**——蓝图称 best value 综合价格、时效、tracking level 与 courier rating，本仓只能核到这些排名字段在场，算法不可核 |
| `S12` | [MCP Server 文档](https://developers.easyship.com/docs/easyship-mcp-server) | 已核实 | 远程端点 `https://mcp.easyship.com/mcp`，Bearer token 鉴权，亦可本地运行；工具逐个列出，每个映射到一个或多个 Easyship API 端点 |
| `S13` | [MCP Server 发布](https://www.easyship.com/blog/easyship-mcp-server) | 已核实 | 2026-04-30 发布，25 个工具，覆盖 550+ courier service 与 200+ 国家和地区，工具分八类（rates、shipments、labels、地址验证、税费、pickup、tracking、账单与分析）；已上 MCP Registry。两条蓝图未记而对商业形态有用：随任何 Easyship 账号免费，且**取价、列单、追踪与分析调用不消耗额度，只有购买 label 或预订 pickup 才消耗** |
| `S14` | [OAuth2 Token](https://developers.easyship.com/reference/oauth2_token) | 已核实 | 访问子公司两条路径：一是 client credentials 授权加 `enterprise.child_company::access` scope，每次请求带 `X_EASYSHIP_COMPANY_ID` 头；二是自定义的委托账户授权，签发绑定某个 `easyship_company_id` 的访问与刷新令牌。scope 按资源与读写分列，新旧两代 API 各一套 |
| `S15` | [International Checkout](https://www.easyship.com/blog/international-checkout-to-show-accurate-rates-taxes-duties-upfront-for-cross-border-shipping-services) | 已核实 | 结账页动态运费属 Plus 档 $29/月；含税费计算、DDU 估算与 DDP 预付的 International Checkout 属 Premier 档，自 $69/月起。DDP 下承运商代垫关税另收手续费与按代垫金额比例计的划付费，这部分并入预付金额 |

## 核完之后

三件事值得记在这里，因为它们改变的是**引用方式**，不是产品规则：

- 蓝图附录 A 的 15 条里，1 条链接已失效（`S5`），2 条被反爬挡住（`S8`、`S9`），2 条的关键价格数字取不到（`S1` 的 tracking 与地址验证、`S7` 的订阅月费）。也就是说照抄那份附录会有五处站不住的引用。要引竞品价格，引本台账的 `已核实` 行。
- 本仓核到而蓝图没记的事实里，有三条与商业形态判断直接相关：Easyship 的取价与追踪调用不消耗额度而只有买面单和订取件才消耗、Shippo 的非面单 API 调用另行计量、Easyship 子公司通过 scope 加公司标识头访问。前两条是「按什么计量才与价值对齐」的现成参照，第三条是多租户委托访问的一种公开做法。它们**只是外部事实**，本仓要不要这么做属产品与技术决定，不在本台账内定。
- LYOC 这一形态（客户连自己的承运商账号、平台仍按生成的面单收平台费）存在已核实，**具体费率未核**。[产品故事](./PRODUCT-STORY.md)给渠道对接写的进入判据是能指出目标客户群里常规形态的真实商业模式；本台账可作那条判据的外部证据之一，但引用时须连「费率未核」一起引。

## 维护规则

- 只登公开可核的事实与核对结果，不登推测、不登结论、不登本产品应当怎么做。
- 每条必须带来源链接与本仓核对日期；改动某条时更新该条日期，不是只改正文。
- 状态只在重新抓取之后才改。把 `未取到` 改成 `已核实` 必须附新的取证。
- 本台账不进入任何验收判据。ADR 或票面若要用某条事实作论据，引用它并连核对日期一起写——数本身是论点，日期就是它的锚。
