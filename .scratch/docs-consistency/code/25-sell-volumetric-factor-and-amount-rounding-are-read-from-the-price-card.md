# 25 PP SELL 方向的体积系数与金额取整：文档归商业价格政策，代码与演示种子只读价卡

Category: bug
Status: ready-for-agent——R2（上线后尽快）：SELL 评价已在生产路径上；PC 价格政策口径对 SELL 必填体积系数，PP 却只读卡上那一份，两处各自合法，合同政策上声明的那个静默不起作用。实施时两处登同值即可绕开（可靠，但全靠人），所以列 R2；租户若按合同政策调系数而不改卡，计费重量与金额就与预期不符。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: docs-consistency/07（PC+PP 修订）先裁「SELL 评价的计价条款（金额取整、体积系数、汇率口径）从哪一份工件读」——即 [audit/pc-pp.md](../audit/pc-pp.md) H5 的 B 路与 [audit/cross.md](../audit/cross.md) B2-02、B2-03 同一裁。
出处：[audit/pc-pp.md](../audit/pc-pp.md) H5（审计归 C，并注明 owner 若认定 SELL 卡自带系数与取整就是首发形态则改归 B）；同根的 [audit/cross.md](../audit/cross.md) B2-02（金额取整，B）、B2-03（汇率口径，B）在本票登记、不另立。审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，PC、PP 代码与种子在此期间未改。

## 现象

PP CONTEXT、PC CONTEXT 与 ADR-0107 都说销售方向的体积系数与金额取整属商业价格政策；PP 代码不分方向，一律读价卡上的系数与取整，`internal/parcelpricing` 里没有任何地方读 PC 价格政策口径。PC 价格政策口径只有税务、体积、汇率三格，没有金额取整。

## 证据

- 文档与决定：
  - PP CONTEXT「体积系数」：「采购方向的系数属价卡内容，销售方向的系数属商业价格政策」；「金额取整策略」：「采购方向的策略属价卡内容，销售方向的属商业价格政策」。
  - PC CONTEXT「商业价格政策版本」：「……以及销售方向的体积系数……`parcel-pricing` 按声明的口径解析取值」。
  - ADR-0107 决定一（同上，与体积系数两侧归属同构）。
  - 相反一侧（cross B2-02）：ADR-0190 三类把「价卡的金额取整声明」列为租户取值；ADR-0198 决定二「卡没声明金额取整，请求记`待判断`」；ADR-0179 决定三选 SELL 卡「不走 PC 价格政策的定价方案绑定」。
- 代码与种子：
  - PC `internal/partycommercial/domain/volumetric_caliber.go` 的 `NewVolumetricCaliber`：SELL 必须声明系数引用，BUY / INTERNAL 不得声明。
  - PP `domain.PricingPlanVersion` 不分方向携带卡自己的 `VolumetricFactor` 与金额取整；`domain/input.go` 的 `deriveVolumetricWeight` 只读 `policy.VolumetricFactor()`。
  - `scripts/demo-seeds/data/pricing/price-card-cn-sg.json`（`direction: SELL`）自带 `weightPolicy.volumetricFactor`（除数 5000）与 `amountRounding`（`HALF_UP`）。
  - PC `domain.PricePolicyCaliber` 只有 tax、volumetric、fx 三格。

## 三类

机制：SELL 评价从裁定的那一份工件取计价条款，另一处拒收或不再要求。产品策略：哪一份是权威（待 07 裁）。租户取值：系数、取整、汇率口径的具体值。

## 规模

M（裁为价卡：撤掉 PC 的 SELL 体积口径必填，改 PC 校验与登记；裁为商业价格政策：PP 的 SELL 评价读 PC 口径，SELL 卡拒收这几格，按 ADR-0179 决定三「两处同一笔改」一并改选卡，L）。

## 最小的诚实修法

两种裁向都要做到「同一格只有一处合法声明」：

- 认价卡：PC 的 SELL 体积口径改为不要求（或撤掉），已登的行如实标「不参与计价」；PP CONTEXT 与 ADR-0107 由 07 改。
- 认商业价格政策：PP 的 SELL 评价从 PC 口径取系数、取整与汇率口径；SELL 卡上出现这几格即拒收；PC 口径补金额取整一格。

## 要裁的

全部在 Blocked by 那一裁里。
