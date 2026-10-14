# 26 演示租户采用受理前估价形态：售价卡绑分区目录、演示委托带邮编、结算账户登采用行（种子，证据 `S`）

Category: enhancement
Status: 完工，待评审与重放——2026-10-14 通道 2（派单 `task-2e6a7f57`）。规则包 `SYN-RULEPKG-01` 从适用组拿掉没有生产者的 `REQUIRED_DOCUMENT`，摘要重算为 `PCC-1:266a5536…`。一次性库 `psb26_mcp2b` 上格 5 越过：委托 `SHR-LP2T5FOTHYPN37HRASSBSHVPZ4` 成`已接受`；冻结 `FRZ-0001` 金额 9600，估价留痕合计 9600，信用暴露 `EXP-0001` 金额 9600。下一停点是初始路由停在 `CANDIDATE_COSTS_PENDING`（`acceptance-decision.formed` 信封落 `dispatch.consumer_undecided`）。走完已删库。未推 main。此前 in-progress——2026-10-14 通道 2（派单 `task-0e5c89bc`）。一次性库 `psb26_mcp2` 灌种子退 0。委托 `SHR-HKFJE6I5RMGT423QGPW5235Z2A`（来源键 `SYN-CUSTREF-20261014T103803Z`）建成`已提交`。dispatch 不再停在财务控制：阶段 `ACCEPTANCE_DECISION`，原因 `ACCEPTANCE_JUDGMENT_INCOMPLETE`，没有 `SA-` 串。受理链后一步未决，整笔回滚，所以 `funds_freeze`、估价留痕、信用暴露、处理尝试仍是 0 行，冻结金额没有留下。种子经登记面映射的入账余额是 20000（200.00 CNY），信用状况逾期为假、登记额度 0。判据 1 仍是 ❌：财务控制不再是停点，但冻结没有落库。走完已删库。未推 main。此前 in-progress——2026-10-14 通道 4（派单 `task-018784d1`）。隔离接单已把草案测量编进声明画像（`15a7e77b`）。一次性库 `psb26_mcp4c` 上格 5 仍没越过：进程日志 `FINANCIAL_CONTROL_NOT_FORMED`，生产装配探针（事务回滚）原因 `SA-BALANCE_UNAVAILABLE`。冻结行 0。判据 1 仍是 ❌。未推 main。此前 完工，待评审与重放——2026-10-14 通道 4 再跑判据 1（派单 `task-b02a598a`）。解析键仍是 `21b102fd`。一次性库 `psb26_mcp4b` 上格 5 仍没越过：进程日志停在 `FINANCIAL_CONTROL_NOT_FORMED`，生产装配探针（事务回滚）原因 `SA-ESTIMATE_DECLARATION_INCOMPLETE`。未推 main。此前 完工，待评审与重放——2026-10-14 通道 1 重走。解析键补 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 后（`21b102fd`），一次性库 `psb26_mcp1` 上格 5 仍没越过：停点从 `FINANCIAL_CONTROL_NOT_CONFIGURED` 前移到 `FINANCIAL_CONTROL_NOT_FORMED`。冻结、估价留痕、运营余额仍是 0 行。未推 main。此前 in-progress——2026-10-14 通道 1：判据 1 停在策略未配置，因为两户解析键的 `requiredBases` 没列 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`，闭包不采用 `SYN-FIN-CONTROL-01`。键已补上这一项，实跑待重走。此前：完工，待评审与重放——2026-10-14 通道 4 接续（派单 `task-d69e73d2` ← 通道 1；原 `task-b6c3588e` → 通道 5 作废）。行为代码仍是 `45247ce0`。判据 1 ❌、判据 2 ✅。此前 in-progress——2026-10-13 21:29 通道 5 认领（派单 `task-b6c3588e` ← 通道 1），分支 `mcp5-psb26`，基 `b3c8bd46`（21:26 `ls-remote` 实测即远端 main），隔离 worktree `idp-parcel-mcp5-psb26`。此前 ready-for-agent——2026-10-12 20:4x 通道 1：[ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 已接受（用户授权通道 1 代裁），决定一没改，三格照旧。此前 needs-info——2026-10-12 通道 3 随[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md)第 2 项分诊立（派单 `task-85d7350d` ← 通道 1），等用户决定是否接受 ADR-0179
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

## 完成记录（通道 4 · 2026-10-14 · 派单 `task-d69e73d2` ← 通道 1，接通道 5 作废的 `task-b6c3588e` · 分支 `mcp5-psb26` · 行为代码 `45247ce0`）

做什么第 1–3 条已在 `45247ce0`，本轮没改那三笔，也没改执行器。第 4 条补记写在合成演示动线「墙三」下。证据只记 `S`。不自评。评审与重放归通道 1。

- ❌ 判据 1。55432 上一次性库 `psb26_mcp4` 干净灌种子退 0。API `:19084`（19080 已被遗留探针占用，未碰）、dispatch 七个变量照动线「前置」，读写真开关同取 `SYN-TENANT-01`。提交 `SHR-YUYNANNZLNPGUH76IVC6ZFV3SE`（来源键 `SYN-CUSTREF-20261014T083249Z`）答复已提交。格 5 没越过：可达性判断形成、答`可达`（`SYN-LINE-CN-SG-01@1` 为 `QUALIFIED`）；受理链停在受理前财务控制，处理尝试落 `FINANCIAL_CONTROL_NOT_CONFIGURED` / `等待运营登记`，委托仍是`已提交`，「委托已提交」信封入账。同一笔用生产装配再问一次控制（事务回滚，探针未入库），原因 `SA-CONTROL_POLICY_NOT_CONFIGURED`。闭包采用集里没有 `SYN-FIN-CONTROL-01`；演示解析键 `requiredBases` 不含 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`。`funds_freeze`、估价留痕、`operational_balance` 都是 0 行——没读到运营余额，停点不是「运营余额没有生产写入方」。走完停掉本轮进程，`DROP DATABASE psb26_mcp4`。遗留库 `psb26_walk` 与其上的探针进程未碰。
- ✅ 判据 2。种子摘要自校在 `45247ce0` 的快照上已对得上，本轮没有再改钉。`go build ./...` 退 0；`go vet ./...` 退 0；`go test -count=1 -p 1` 跑 `internal/parcelpricing/adapters/pricecardtemplate`、`cmd/parcel-pricing-register`、`cmd/parcel-commercial`、`internal/architecture`，四包皆 `ok`（DSN=`postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）。`-v` 单跑 `TestDemoSeedClearsTheCitationGateAndItsDeclaredDigests` 与 `TestSeedCardsImportToTheSeedDigests` 均为 `--- PASS`，不是 `SKIP`。没跑全仓。

## 完成记录（通道 4 · 2026-10-14 · 派单 `task-b02a598a` ← 通道 1 · 分支 `mcp5-psb26` · 行为代码 `21b102fd`）

解析键那一项已在 `21b102fd`，本轮没有再改键，也没有加 `CREDIT_POLICY`。没有直写 `operational_balance` 或 `credit_standing`。执行器未改。墙三补记写这次实测停点。证据只记 `S`。不自评。评审与重放归通道 1。

- ❌ 判据 1。55432 上一次性库 `psb26_mcp4b` 干净灌种子退 0。API `:19088`（19080 仍是遗留探针，未碰）、dispatch 七个变量照动线「前置」，读写真开关同取 `SYN-TENANT-01`。提交 `SHR-TWC4EII6DIC3ZMFE37N32LELWA`（来源键 `SYN-CUSTREF-20261014T094857Z`，提交版本 `SUBV-Q2PSBXS33JFF623QE44ZJNJAEU`）答复已提交。格 5 没越过。dispatch 对「委托已提交」信封答 `dispatch.consumer_undecided`：受理链未决，阶段 `FINANCIAL_CONTROL_JUDGMENT`，原因 `FINANCIAL_CONTROL_NOT_FORMED`。处理尝试随未决回滚，表上 0 行。同一笔用生产装配再问一次控制（事务回滚，探针未入库），原因 `SA-ESTIMATE_DECLARATION_INCOMPLETE`。快照上有两端邮编、没有声明画像。提交草案已经带了这一件的 1.50 KG 与 20×15×10 CM；隔离接单入口把它们编进摘要的服务条目，没有放上声明画像。再改种子里的数值推不动这一格。`funds_freeze`、估价留痕、`operational_balance` 都是 0 行。走完停掉本轮进程，`DROP DATABASE psb26_mcp4b`。遗留库 `psb26_walk` 与其上的探针进程未碰。
- ✅ 判据 2。本轮没有改种子快照，摘要自校仍对得上。`go build ./...` 退 0；`go vet ./...` 退 0；`go test -count=1 -p 1` 跑 `internal/parcelpricing/adapters/pricecardtemplate`、`cmd/parcel-pricing-register`、`cmd/parcel-commercial`、`internal/architecture`，四包皆 `ok`（DSN=`postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）。`-v` 单跑 `TestDemoSeedClearsTheCitationGateAndItsDeclaredDigests` 与 `TestSeedCardsImportToTheSeedDigests` 均为 `--- PASS`，不是 `SKIP`。没跑全仓。

## 再跑记录（通道 4 · 2026-10-14 · 派单 `task-018784d1` ← 通道 1 · 分支 `mcp5-psb26` · 行为代码 `15a7e77b`）

隔离接单把草案上已声明的重量（有尺寸则一并）编进声明画像，成员是同一下标现签的内部包裹标识。没报重量不立画像。服务条目仍进摘要。没有写 `operational_balance` 或 `credit_standing`，没有给解析键加 `CREDIT_POLICY`。冻结行没有出现，票面不写完工。证据只记 `S`。

- ❌ 判据 1。55432 上一次性库 `psb26_mcp4c` 干净灌种子退 0。API `:19088`（19080 仍是遗留探针，未碰）、dispatch 七个变量照动线「前置」，读写真开关同取 `SYN-TENANT-01`。提交 `SHR-6FJEOCCZ3AHYS2NYYNJXTPAA5A`（来源键 `SYN-CUSTREF-20261014T100701Z`，提交版本 `SUBV-UFZP627PMHBGYINODZ3ZXLQFHQ`）答复已提交。快照上这一件有声明画像：1.50 KG，20×15×10 CM。格 5 没越过。dispatch 答 `dispatch.consumer_undecided`：阶段 `FINANCIAL_CONTROL_JUDGMENT`，原因 `FINANCIAL_CONTROL_NOT_FORMED`。处理尝试随未决回滚，表上 0 行。同一笔用生产装配再问一次控制（事务回滚，探针未入库），原因 `SA-BALANCE_UNAVAILABLE`。`funds_freeze`、估价留痕、`operational_balance`、`credit_standing` 都是 0 行。走完停掉本轮进程，`DROP DATABASE psb26_mcp4c`。遗留库 `psb26_walk` 与其上的探针进程未碰。
- ✅ 判据 2。种子快照未改。`go build ./...` 退 0；`go vet ./...` 退 0；`go test -count=1 -p 1` 跑改过的 `internal/parcelshipment/adapters/http` 与 `internal/parcelpricing/adapters/pricecardtemplate`、`cmd/parcel-pricing-register`、`cmd/parcel-commercial`、`internal/architecture`，皆 `ok`（DSN=`postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）。`-v` 单跑 `TestDemoSeedClearsTheCitationGateAndItsDeclaredDigests` 与 `TestSeedCardsImportToTheSeedDigests` 均为 `--- PASS`，不是 `SKIP`。没跑全仓。

## 完成记录（通道 2 · 2026-10-14 · 派单 `task-2e6a7f57` ← 通道 1 · 分支 `mcp2-psb29t`）

从 `SYN-RULEPKG-01` 的 `applicableGroups` 拿掉 `REQUIRED_DOCUMENT`（仓里没有 `RequiredDocumentCheckFor`），按 PCC-1 重算 `contentDigest`。不造恒通过的单证校验，不改其余适用组，不改消费门回滚规则。证据只记 `S`。不自评。评审与重放归通道 1。

- ✅ 判据 1。55432 上一次性库 `psb26_mcp2b` 干净灌种子退 0。API `:19093`、dispatch 七个变量照动线「前置」，读写真开关同取 `SYN-TENANT-01`。提交 `SHR-LP2T5FOTHYPN37HRASSBSHVPZ4`（来源键 `SYN-CUSTREF-20261014T104918Z`，提交版本 `SUBV-6DSHGAWF54KLTO5INHRQMMJZSM`）越过格 5：委托成`已接受`；`funds_freeze` 一行 `FRZ-0001` 金额 9600；估价留痕合计 9600；信用暴露 `EXP-0001` 金额 9600。受理前控制与可达性判断各 1 行。「委托已提交」信封 `PUBLISHED`。下一停点：`acceptance-decision.formed` 消费侧初始路由未决，包裹停在 `CANDIDATE_COSTS_PENDING`，信封烧完预算落 `ABANDONED` / `dispatch.consumer_undecided`。走完停掉本轮进程，`DROP DATABASE psb26_mcp2b`。遗留库 `psb26_walk` 与 `psb26_mcp4d` 未碰。
