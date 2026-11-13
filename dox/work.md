# 在做与待做

按什么规矩开工见 [AGENTS.md「交付模式」](../AGENTS.md#交付模式)。做完的条目删掉；这里不记经过。

## 在做

- **客户要求清单**（合同、价卡、计费、报价），用户 2026-11-12 点名全做。票面在这一批做完之前仍留在 [.scratch 的规格](../.scratch/commercial-four-module-requirements/spec.md)下，不搬。
  - 销售价来源这条链：票 16（合同正文带销售价安排，在途，分支 `cursor/cfmr-16-contract-sell-price-arrangement-r2`）→ 17（随受理时点闭包固定并供结算读）；18（计价按有序范围列表选卡）；19（结算带范围列表请求评价，等 17、18）；20（管理台合同页）；21（演示种子与走查）。
  - 其余各票：见规格的子票表。

## 待做

- **等用户裁定：替代链分叉按不按业务时间裁决。** 旧文档说不裁、各后继不参与派生；代码（`judgeForks`）按业务时间裁，同刻才立信号。见 [contexts/visibility-exception.md](./contexts/visibility-exception.md) 末节。
- **给 `accessidentity` 与 `pilotgovernance` 各补一个上下文文件。**
- **把上下文之间的所有权关系收成 `dox/context-map.md`**，替掉对旧 CONTEXT-MAP 的指向。
- **写 `product.md` 与 `tenant.md`**。

## 上线前要核的

- 「受理前估价采用」与「SELL 计费时点采用」两本册登的是同一个范围。
- 首个带 PR 的推送看一次 Actions，确认两种事件各跑各的步骤。
- 要对客展示 ETA 时，先过冻结清单「记录」里它那条清单。

## 搁置

等用户点名：`cursor/wf-in-warehouse`、`cursor/reporting-source-adr`、`shelved/company-subject-profile`（都在 origin）；冻结的票见[冻结清单](../.scratch/launch-line-freeze-2026-10-27.md)。
