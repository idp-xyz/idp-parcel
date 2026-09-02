# 「机制半边现状」改为生成物，停掉第二十八轮手工重盘

Category: enhancement
Status: in-progress

取证基线 `c9835bf`。本票与演示三墙无交集，可随时插入。

## 缺口

[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)的「机制半边现状」一节在手工维护**代码事实**：生产文件数、编排文件数、PostgreSQL 适配器数、Outbox 适配器数、迁移份数、端口两口径缺口数、测试文件数。这些数每次提交都可能变。

节里自己已经把问题写明白了——它「立节当天即被并行落地的提交反复带假」，至今重盘到**第二十七轮**，并且不得不挂一句「读到与代码不符时以代码为准」。

**一份要靠免责声明来维持正确性的文档，待错了介质。** 而且工具已经有了：`.scratch/mechanism-reinventory-r26/tool/`（`main.go` 自带 `go.mod`，r27 沿用它并做过对 r25 发布树的复现校验）。

## 做什么

三件，顺序执行：

1. **把工具提成仓内正式命令。** 从 `.scratch/` 挪进 `cmd/`（名字建议 `cmd/mechanism-inventory`，做票人可另择），并入主 `go.mod`，去掉它自己那份。补测试。
2. **让 CI 跑它。** `.github/workflows/ci.yml` 已在 `main` 的 push 与每个 PR 上跑 `gofmt`/`go vet`/`go test -race`/`go build`，加一步产出盘点报告。**注意 CI 自 2026-08-20 因账户账务停摆未再实跑**（门禁配置在、运行停），所以这一步落地时要在本机验证一次，不能只靠 CI 绿。
3. **砍掉正文里的数。** 「机制半边现状」正文只留：三条判据、八个切片的状态定级、以及一个指向生成报告的指针。所有可由代码算出的计数、文件数、缺口数从正文移除。

## 这一票的边界在哪

**状态定级不是生成物。**「达标 / 部分 / 未开始」是判断，「显式留待已认可」是裁定记录，这些留在正文由人维护——它们不是代码能算出来的东西。可生成的只有**计数与在位性**。

划错这条线的代价是两种相反的错：把定级也自动化，会让一次判断被一个脚本的启发式覆盖；把计数留在正文，就是第二十八轮。

## 完成判据

命令进 `cmd/` 并有测试；本机跑通并产出与 r27 报告可对照的结果（差异逐条说明，不许「大致一致」）；正文的计数全部移除并改为指针；`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿。

改 `docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md` 属产品权威文档改动，动手前先确认没有并行会话正在改同一节。

## 参照

`.scratch/mechanism-reinventory-r26/tool/`（工具源码）、`.scratch/mechanism-reinventory-r27/`（最近一轮的报告与原始取证）、`.github/workflows/ci.yml`、`docs/agents/parallel-sessions.md`。
