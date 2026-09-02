# 「机制半边现状」改为生成物，停掉第二十八轮手工重盘

Category: enhancement
Status: resolved

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

## Comments

- 2026-09-02 · MCP-6（交付收口，转 resolved）。

  **落点。** 工具落 `tools/mechanism-inventory/`，**自成一个模块**——票面原写「并入主 `go.mod`」，那条是错的：端口清点要 `golang.org/x/tools`，而主模块的直接依赖只有三个（chi、pgx、idp-bento-go），且「直接依赖三个」这件事本身被开发主线当作「参数显式未配置」的佐证在引用。并进去会让那句话变假。嵌套模块不进父模块的 `./...`，因此 CI 单列两步。

  **搬来的与新写的。** 端口两口径判定逐字搬自 `.scratch/mechanism-reinventory-r26/tool/`（只把「打印」换成「交回结构」——两口径与历轮的可比性靠的正是判定没变）。新写的是文件面清点：逐上下文生产/测试文件、应用编排、postgres/http 适配器、Outbox 投递、跨上下文消费缝、迁移份数。

  **测试。** 文件面清点走 `io/fs`，用 `fstest.MapFS` 钉七条，其中四条钉的是手工重盘反复数错的那几格：Outbox 按类型声明认而不认「提到名字」；`adapters` 与目标名必须紧邻（不在 `adapters/` 下、只是碰巧叫 `postgres` 的目录不算）；缝按「消费方—提供方」配对算而不是按消费方去重；`README.md` 不是迁移。端口清点不另测——它是搬来的、且每次真跑就是它自己的验证。

  **CI 两步**（`.github/workflows/ci.yml`）：工具模块单独 vet + test；以及「重新生成后与提交进来的那份比对，不一致即红」。比对用 `git status --porcelain` 不用 `git diff --exit-code`——后者看不见未跟踪文件，报告一旦被删会静默通过（本笔实测到这一格：报告未入库时 `git diff` 空转返回 0）。

  **文档改动。** 报告落 `docs/product/MECHANISM-INVENTORY.md` 并在 `docs/README.md` 登入口（标明生成物）。开发主线的「机制半边现状」只改了快照来源那两段：改成指向生成物、并声明正文里仍留的数是锚在 `f6f0029` 的历史叙述、已被报告取代。**没有重写那一节下面的密集叙述**——那里编码了大量既有裁定，整段重写风险高于收益，留作后续一遍单独的清理（见下「未做完的一件」）。

  **首跑就抓到两件实事。**

  一、**文档里的数已经全线过期。** 手工快照锚在 `f6f0029`，本笔生成锚在 `f53d789`：生产文件 543→633，测试 552→577，应用编排 71→80，postgres 适配器 165→195，Outbox 投递 47→48，迁移 92→101，端口 240→274。上下文数也不再是「十个」——`accessidentity` 与 `collectionremittance` 已在 `internal/` 下，业务上下文实为 12。

  二、**多出一个没被点名的端口缺口。** 精确口径缺 7 个，其中六个正是正文点名的那六（SA 合同责任/金额规则/审核授权、PS 规则登记与修订授权、VE 通知网关），**第七个 `settlementaccounting.ConfirmedChargeFactsView` 不在名单上**。它是留待还是遗漏本笔不判——那是 SA 地盘的判断，只如实报出来。

  **验证。** 根目录 `gofmt -l .` 空（含 `tools/`）；主模块 `go vet ./...`、`go build ./...` 退 0；工具模块 vet + test 绿；报告路径无关（不含任何绝对路径）且**两次生成 SHA256 相同**。

  **`go test -count=1 ./...` 在本工作树上有 3 个红，不是本笔造成的**：`TestNoServiceProductCanTakeAnIndependentWaybillChannelForm`、`TestServiceProductFormIsAFacetNotASeparateCatalog`、`TestEveryConstructableServiceProductFormHasAnExplicitTranslation`。它们源于另一会话正在写的未提交改动（`internal/partycommercial/**` 与新迁移 `0017_service_product_form_label_channel.sql`，属 `label-channel-service-first-release` 票 05 的服务产品形态第二取值）。在 `f53d789` 的干净 worktree 上这两个包都 `ok`——**本笔一个 `internal/` 文件都没碰**。报告也因此改从干净 HEAD 生成，不含那份在途改动。

  **未做完的一件（建议另票）**：开发主线「机制半边现状」正文里那些历史计数现已声明为过期叙述，但仍占着篇幅。把它们逐段删到只剩定级与理由，是一次独立的清理，应由该节的地盘方或在有评审的情况下做。