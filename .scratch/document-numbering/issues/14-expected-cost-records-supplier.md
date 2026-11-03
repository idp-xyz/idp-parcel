# 14 预期成本记下供应商：登记账单按供应商筛预期成本，匹配门核供应商

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无（从票 [13](./13-supplier-bill-number-and-ranged-audit-outcomes.md) 的分支 tip `4bcdc59f7` 起，已含票 11、票 13 与 main `3b4720283`；票 11、13 进 main 后并一次 main）
出处：票 [11](./11-supplier-bill-and-audited-payable-admin-pages.md) 评审 Spec 非阻断 3——登记表单的预期成本不按供应商筛（只筛当前版与币种，供应商下拉列全部往来单位），匹配门 `MatchBillLine` 也不核供应商（ADR-0340 越权风险点四），误选一次就把账单对到别家的预期成本；票 11 修订查明预期成本的读口、表与领域类型都没有供应商，只有它依的那一版协议（`agreement_ref`），协议属哪家供应商归 party-commercial，于是在 UC-SA-004「给开发的交接」记了一行缺口（去处「待立票」）、登记表单加了一句提示。用户 2026-11-02 在三个选项（预期成本记下供应商 / 匹配时经往来单位与协议方核对 / 暂不补）里选「预期成本记下供应商」：形成预期成本时从协议取出供应商一并记下（之后不变），登记表单按所选供应商筛，匹配时在结算上下文内直接比对、不对就拒；先写 ADR 再做。

## 要做的

1. **ADR**（取下一个空号）：预期成本形成时供应商从哪来（形成载荷带入，还是由 SA 经消费侧适配器问 party-commercial 协议的供应商，择一说理由），记下后不变，协议换供应商时怎么读；匹配门核供应商、答复形状（新拒因码照既有命名）与 ADR-0357 范围审核的关系；旧数据没有供应商一格的读法与能否回填（不得把租户取值写成默认）；对 ADR-0340 越权风险点四的回答；改到的已接受记录照部分停用写法挂指针、引原句逐字；越权风险点与 Alternatives（含「匹配时经协议方核对」「暂不补」）。
2. **CONTEXT / UC / GLOSSARY** 照 AGENTS「改文档」同步；UC-SA-004 立 AT 行，交接表票 11 记的那行缺口改为已补并写落点。
3. **实现**（settlement-accounting，party-commercial 只加读口）：领域、应用、端口、postgres 迁移（只增、在有数据的库上可施加）、形成编排经消费侧适配器取供应商、读口 `SupplierExpectedCostCatalogueRow` 带出供应商、匹配门核供应商；旧行补记口；管理台登记表单按所选供应商筛预期成本，去掉票 11 加的那句「这里不按供应商筛」提示。
4. **测试**：领域单测（供应商不符拒）、应用与 HTTP、真库（旧数据、补记、跨供应商误配被拒、租户隔离）、适配器测试；前端单测。AT 盘点照惯例加行。

## 不做

不改 party-commercial 的领域规则与协议生命周期；不重判已接收的账单；不在接收口临时去问 party-commercial；不改审核、已匹配金额与取号。

## 完成判据

`go build`、`go vet`、`gofmt -l` 无输出，受影响包与 `./internal/architecture/... ./cmd/... ./migrations/...` 真库测试过；前端 `tsc -b`、`pnpm test`、`vite build` 过；种子经 `demo-run/up.sh` 灌一次性库退 0；浏览器在自起后端连一次性库，登记表单选一家供应商只出那家的预期成本，经接口把账单行对到别家预期成本被拒。只做 [workflow「进 main 的步骤」](../../../docs/agents/workflow.md#进-main-的步骤)第 1 步。
