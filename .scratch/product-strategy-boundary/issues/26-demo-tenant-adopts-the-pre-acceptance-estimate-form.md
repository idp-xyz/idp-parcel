# 26 演示租户采用受理前估价形态：售价卡绑分区目录、演示委托带邮编、结算账户登采用行（种子，证据 `S`）

Category: enhancement
Status: in-progress——2026-10-13 21:29 通道 5 认领（派单 `task-b6c3588e` ← 通道 1），分支 `mcp5-psb26`，基 `b3c8bd46`（21:26 `ls-remote` 实测即远端 main），隔离 worktree `idp-parcel-mcp5-psb26`。此前 ready-for-agent——2026-10-12 20:4x 通道 1：[ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 已接受（用户授权通道 1 代裁），决定一没改，三格照旧。此前 needs-info——2026-10-12 通道 3 随[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md)第 2 项分诊立（派单 `task-85d7350d` ← 通道 1），等用户决定是否接受 ADR-0179
Blocked by: 无——27 已于 2026-10-13 21:21 进 main（`b3c8bd46`）。此前 [27](./27-pre-acceptance-control-reads-only-the-current-submission-version.md)（2026-10-13 19:3x 通道 1 补：票 25 第二段评审 Standards 1 写明灌采用行之前须守住提交版本换代）。[25](./25-pre-acceptance-estimate-formed-by-settlement.md) 已于 2026-10-13 resolved（main `6e4d9a1d`）
地盘：`scripts/demo-seeds`（`seedgen` 的售价卡结构、注释与重生成的价卡快照、`submit-one-shipment.sh` 的草案、`data/settlement/` 新增一份采用行、`seed.sh` 结算段与参考目录那句注释、README 组成表与数据故事）；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「墙三」下一条带日期的补记。执行器不动。
出处：票 06「第 2 项分诊」；ADR-0179 Consequences 演示租户那一条；AGENTS.md「演示租户就是 `SYN-TENANT-01`，代码按真实租户对待它」；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)。

## 现状（钉 main `ff3b01fa`）

- 售价卡 `SYN-PLAN-CN-SG-01` 只绑燃油序列，不绑分区目录 `SYN-CAT-ZONE-CN-SG`；README 与 `seed.sh` 都写着「价卡不绑这本目录」，评价仍走调用方给的分区。按 ADR-0179 决定四，没绑分区目录的卡在受理前估价里答未形成。
- `submit-one-shipment.sh` 的草案带寄 / 收两端国家码，不带邮编；接入面 `isolated_write_intake.go` 收 `senderPostalCode`、`recipientPostalCode`。
- 结算账户登记册有 `SYN-SETTLEMENT-ACCOUNT-01` 一行，没有估价采用行。

## 做什么（按 ADR-0179 现稿）

1. `seedgen` 让售价卡方案结构绑分区目录 `SYN-CAT-ZONE-CN-SG`，重生成价卡快照；README、`seed.sh` 与 `seedgen` 注释里「卡不绑这本目录」一类说法随改。分区目录的前缀与 Z1 / Z2 映射照现有那一版，不改。
2. `submit-one-shipment.sh` 的草案带两端邮编：目的侧取落在分区目录条目前缀上的合成邮编，始发侧取落在目录始发覆盖里的（开工时照 `seedgen` 的目录条目核）。分区目录按数字前缀匹配，邮编不加 `SYN-` 前缀；它是演示委托的内容，不是任何租户的取值。
3. 登一行采用：`SYN-SETTLEMENT-ACCOUNT-01` → 形态「逐件 SELL 试算合计」、计价范围取售价卡的范围（开工时核，今天是种子的 `scopeID`）；`seed.sh` 结算段调票 25 的登记命令。
4. 演示动线「墙三」下补一条带日期的补记，写越过格 5 之后实测的下一停点。

## 不做

- 不改执行器、不改分区目录内容、不改成本卡。
- 不替任何真实租户登行；新登的对象标识只用 `SYN-` 前缀。

## 完成判据

- 55432 上自己命名的一次性库干净灌种子退 0；照动线「前置」起进程，提交一笔，演示委托越过格 5：受理前控制形成、冻结一笔，金额等于这一件的试算合计。下一停点如实记下。走完删库。证据层级 `S`。
- 读演示种子的用例包全绿（种子摘要自校那条在新快照上重钉）。
