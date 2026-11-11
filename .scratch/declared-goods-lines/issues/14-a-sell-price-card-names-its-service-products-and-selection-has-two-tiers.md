# 14 售价卡声明适用服务产品，选卡分两层

Category: enhancement
Status: ready-for-agent
Blocked by: 无

依据：[ADR-0378](../../../docs/adr/0378-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md) 决定一、二、四、五；ADR-0234（收尾）、ADR-0267 与 ADR-0348（加键换版、旧版字节不动）、ADR-0352（较新版本登记要部署开关）。

## 要做的

- **领域**（`internal/parcelpricing/domain`）：
  - 新值类型「服务产品引用」，不透明、非空，写参与方与商业的服务产品对象标识，不带版本。
  - `PricingPlanVersion` 带一组适用服务产品：去重、排序后保存；空即不声明。只有 `SELL` 方向、`CUSTOMER_CHARGE` 目的的方案能声明，别的答构造错误。登记快照、重建与 `valid()` 一并带上。
- **规范化与摘要**：方案的规范化文档加可缺键 `service_products`（排序后的字符串数组），进内容摘要。带它的方案写 `PPC-7`，不带的照旧取能表达内容的最低一版；`PPC-7` 进受支持版本。既有钉住的摘要常量不变（PPC-5、PPC-6 的旧形状测试照过）。
- **载荷**：价卡方案文档的 JSON 加可缺键 `serviceProducts`（字符串数组）。所有登记口都认：受控登记 CLI、操作者登记 HTTP、导入预览、草稿与批准发布。价卡查阅视图交出 `serviceProducts`，不声明即不带这一键。
- **登记册**：迁移给 `parcel_pricing.price_card_version` 加 `service_products text[] NOT NULL DEFAULT '{}'`。登记时写入，读回时与快照核对，与 scope、direction、purpose 几列同法。
- **选卡**：
  - `PriceCardInForceResolver.ResolveInForce` 加最后一个入参：服务产品引用，零值即没申报。
  - 两层：先看时点在用、指名这个产品的卡，恰一张即它，多于一张答适用冲突；一张都没有，再看不声明产品的卡，恰一张即它，多于一张答冲突，没有答未配置。指名了别的产品的卡不适用。零值只看不声明的卡。
  - `LoadApplicable`（试算的卡集）同法，加同一个入参。
  - 改写解析口注释里「不加列（裁决 3）」那一句。
- **调用方**：本票里三个处理器先传零值，行为与今天相同（只有不声明的卡）；接上申报的产品是票 15。
- **收尾**：不改闸门。补测：同一方案后继改了适用产品可以收尾；收尾后被放下的产品答未配置或由别的卡接。

## 完成判据

- 领域：只有 SELL·客户费用能声明；去重排序；带产品写 PPC-7、不带的摘要与改前逐字节相同（钉一份不带产品的方案的摘要，取证于改前提交）。
- 真库：带产品的卡登记、重放答已登记、同标识改了产品答内容冲突；两层选卡的各格（指名一张、指名两张冲突、只有不声明的、不声明的两张冲突、指名别的产品不适用、零值只看不声明的）。
- 登记较新版本照旧要部署开关。
- 全量 `go test`（带 PG）与 `go vet` 通过。

## 不做的

申报的产品接进三条路（票 15）；管理台（票 16）；演示（票 17）；BUY 与 INTERNAL 卡按渠道产品分卡（ADR-0378 决定一，另立）。
