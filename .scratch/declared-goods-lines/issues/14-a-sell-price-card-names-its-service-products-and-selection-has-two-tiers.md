# 14 售价卡声明适用服务产品，选卡分两层

Category: enhancement
Status: in-progress
Blocked by: 无（已提交，未进 main；分支 `feat/goods-details-admin`）

依据：[ADR-0382](../../../docs/adr/0382-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md) 决定一、二、四、五；ADR-0234（收尾）、ADR-0267 与 ADR-0348（加键换版、旧版字节不动）、ADR-0352（较新版本登记要部署开关）。

## 要做的

- **领域**（`internal/parcelpricing/domain`）：
  - 新值类型「服务产品引用」，不透明、非空，写参与方与商业的服务产品对象标识，不带版本。
  - `PricingPlanVersion` 带一组适用服务产品：去重、排序后保存；空即不声明。只有 `SELL` 方向、`CUSTOMER_CHARGE` 目的的方案能声明，别的答构造错误。登记快照、重建与 `valid()` 一并带上。
- **规范化与摘要**：方案的规范化文档加可缺键 `service_products`（排序后的字符串数组），进内容摘要。带它的方案写 `PPC-7`，不带的照旧取能表达内容的最低一版；`PPC-7` 进受支持版本。既有钉住的摘要常量不变（PPC-5、PPC-6 的旧形状测试照过）。
- **载荷**：价卡方案文档的 JSON 加可缺键 `serviceProducts`（字符串数组）；带了就至少一个，空数组拒收。所有登记口都认：受控登记 CLI、操作者登记 HTTP、导入预览、草稿与批准发布。价卡查阅视图交出 `serviceProducts`，不声明即不带这一键。
- **导入模板**（`adapters/pricecardtemplate`，ADR-0349）：`card` 表加一行字段 `serviceProducts`，值是分号分隔的产品标识（用现成的分号列表读法），自模板 `PPT-3` 起可用；`TemplateVersion` 升到 `PPT-3`，`PPT-1`、`PPT-2` 照收。只在方向为 `SELL` 时收。规格口与空白模板跟着列出它。
- **登记册**：迁移给 `parcel_pricing.price_card_version` 加 `service_products text[] NOT NULL DEFAULT '{}'`。登记时写入，读回时与快照核对，与 scope、direction、purpose 几列同法。
- **选卡**：
  - `PriceCardInForceResolver.ResolveInForce` 加最后一个入参：服务产品引用，零值即没申报。
  - 两层：先看时点在用、指名这个产品的卡，恰一张即它，多于一张答适用冲突；一张都没有，再看不声明产品的卡，恰一张即它，多于一张答冲突，没有答未配置。指名了别的产品的卡不适用。零值只看不声明的卡。
  - `LoadApplicable`（试算的卡集）加同一个入参，只取第一个非空的层（有指名这个产品的卡就只取它们，没有取不声明的卡；零值只取后者），层内照试算原来的规矩：不同方案并列，同一方案两版才答冲突（UC-PP-001、ADR-0152）。
  - 改写解析口注释里「不加列（裁决 3）」那一句。
- **调用方**：本票里三个处理器先传零值，行为与今天相同（只有不声明的卡）；接上申报的产品是票 15。
- **收尾**：不改闸门。补测：同一方案后继改了适用产品可以收尾；收尾后被放下的产品答未配置或由别的卡接。

## 完成判据

- 领域：只有 SELL·客户费用能声明；去重排序；带产品写 PPC-7、不带的摘要与改前逐字节相同（钉一份不带产品的方案的摘要，取证于改前提交）。
- 真库：带产品的卡登记、重放答已登记、同标识改了产品答内容冲突；两层选卡的各格（指名一张、指名两张冲突、只有不声明的、不声明的两张冲突、指名别的产品不适用、零值只看不声明的）。
- 登记较新版本照旧要部署开关。
- 全量 `go test`（带 PG）与 `go vet` 通过。

## 不做的

申报的产品接进三条路（票 15）；管理台（票 16）；演示（票 17）；BUY 与 INTERNAL 卡按渠道产品分卡（ADR-0382 决定一，另立）。

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/price-card-service-products`（工作树 `idp-parcel-goods-t14`，基线 `37c172945`）上的 `7ab94058d` 与评审修订 `6622695b0`，未推送、未进 main（协调方已把 `7ab94058d` 拣入 `feat/goods-details-admin`）。
- 拣入：在 `feat/goods-details-admin` 上是 `5b239c23f` 与评审修订 `8a675c5e2`（与票 15 的修订同一笔），拣入时无冲突；拣入后带 PG 全量 `go test` 与管理台三道门通过。
- 落点：
  - 领域：新值类型 `ServiceProductReference`（参与方与商业的服务产品对象标识，不带版本，零值即没有产品）。`PricingPlanVersion.WithServiceProducts` 只收 `SELL`·客户费用，别的答 `ErrServiceProductsOnlyOnSellCustomerCharge`（包着 `ErrInvalidPricingPlan`）；至少一个，去重、按标识排序后保存，重算规范化版本与摘要。`NewPricingPlanVersion` 签名不动。
  - 规范化：`canonicalPricingPlan` 加可缺键 `service_products`；带它的卡写 `PPC-7`（`canonicalizationVersionServiceProducts`），是 `PPC-6` 的超集；方案级 `requiredCanonicalization` 取最低一版。`PPC-7` 进 `supportedCanonicalizations`，「不认的版本」用例改为 `PPC-8`。
  - 快照：`pricingPlanSnapshot` 加可缺键 `serviceProducts`，登记、草稿、批准发布都经这一份形状。读回门 `serviceProductsHold`：只在 SELL·客户费用、严格升序；**写了键却是空列表拒**（协调方更正二）。
  - 登记册：迁移 `0016_price_card_version_service_products.sql` 加 `service_products text[] NOT NULL DEFAULT '{}'`，另加检查 `price_card_version_service_products_sell_only`；`Register` 写列，`priceCardRow.plan` 与快照核对（`productsMatch`）。各条读法改用 `priceCardRow.scanTargets` 一处列目标。
  - 选卡：`ResolveInForce`、`LoadApplicable` 加最后一个入参。取层只在 `tierForProduct` 一处，是 ADR-0382 的指名优先：指名这个产品的那一层有行就只看它，没有才看不声明的那一层，零值只看后一层；层在折完截止与退役之后取。`ResolveInForce` 同层多于一张答适用冲突；`LoadApplicable` 层内照旧是试算的卡集，不同方案并列、同一方案两版才是 `ErrAmbiguousPriceCard`（协调方更正一）。解析口注释「不加列（裁决 3）」改写为指名优先的说法（协调方更正五）。三个处理器先传零值。
  - 导入模板（协调方更正三，票面原未列）：`TemplateVersion` 进位 `PPT-3`，`PPT-1`、`PPT-2` 照旧接受、读出逐位相同。`card` 表新增字段 `serviceProducts`（`requiredOptional`，分号列表，逐个交 `NewServiceProductReference`，方案立住后 `WithServiceProducts`）。旧版文件里这一行填了值答 `FIELD_UNKNOWN`「自模板 PPT-3 起才可用」，留空不算；PPT-3 里整行缺席或留空都是不声明；非 SELL 卡填了值答 `CELL_NOT_APPLICABLE`（规范既有「不适用的格填了值」那一码）；重复、空元素照列表读法答 `CELL_INVALID`。规格口逐列加 `since`，`card.serviceProducts` 标 `PPT-3`。`cmd/parcel-pricing-template` 重新生成 `apps/admin-web/public/templates/price-card-import-template-PPT-3.xlsx`（PPT-1、PPT-2 两份照留）。模板规范文档 `docs/design/pp-price-card-import-template-and-validation-spec.md` 与 `docs/README.md` 同步。
  - 查阅：价卡目录行、导入预览与草稿概要（同一份 `priceCardContentBody`）交出 `serviceProducts`，同名同形，不声明即缺键。
- 判据：
  - 领域：只有 SELL·客户费用能声明 ✅；去重排序 ✅；带产品写 PPC-7 ✅；不带产品的摘要钉在取证于 `37c172945` 的 `881e0a73099cabf819a2fd54a3f8e2a2c86b02d6c7560ee7860dbf34271608a5`（`TestAPlanThatNamesNoServiceProductKeepsItsDigestFromBeforeTheKeyExisted`），既有 PPC-5 / PPC-6 钉住的摘要照过 ✅。
  - 真库：带产品的卡登记、重放答已登记、同标识改产品答内容冲突、列与快照分岔读回拒 ✅；两层各格（指名一张、指名两张冲突、只有不声明的、申报了没人指名的产品、不声明的两张冲突、指名别的产品不适用、零值只看不声明的、零值而只有指名卡答未配置）✅；试算卡集同层并列、同方案两版报错 ✅。
  - 收尾：同一方案后继改了适用产品照样收尾；放下的产品自截止起没有不声明的卡答未配置、有就由它接，截止之前仍归旧版 ✅。
  - 登记 PPC-7 照旧要部署开关（`register_price_card_gate_test`）✅。
- 未做：申报的产品接进三条路（票 15）；按票的 SELL 计价（协调方更正四：SELL 评价两条路照旧逐包裹，本票不碰）；管理台（票 16，导入链接须从 `price-card-import-template-PPT-2.xlsx` 换到 `PPT-3`，在 `apps/admin-web/src/pages/pricing/api.ts`）。
- 验证（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432）：`gofmt -l` 无输出、`go vet ./internal/... ./cmd/...`、`go build ./...` 通过；`go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0。
- 评审（非作者，2026-11-11）：无阻断；该改一处、细处若干，已改（`6622695b0`，同分支一笔，自足可拣）：
  - 产品标识不许带分号：`NewServiceProductReference` 与读回门都拒，模板的分号列表永远拆不开一个标识；模板规范「列表」一条改写为按列说明元素是什么，`serviceProducts` 的元素不许含分号。
  - `tierForProduct` 注释不再数口，点名 `ResolveInForce`、`LoadApplicable`。
  - 补测（真库）：同一方案 v1 不声明、v2 指名 X——X 得 v2，别的产品与零值得 v1，试算卡集不报同一方案两版（ADR-0382 决定五）；过期的指名卡落回不声明的卡。
  - 评审后全量门禁：`gofmt -l` 无输出、`go vet ./internal/... ./cmd/...`、`go build ./...` 通过；带 PG 的 `go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0（170 个包 ok）。

