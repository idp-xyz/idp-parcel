# 三棵已死会话的 worktree 存着未提交的工作

Category: enhancement
Status: needs-triage

用户 2026-08-20 裁定拆树口径：**只拆逐棵证过是 `origin/main` 祖先的**。按此执行时，
下列三棵虽然 HEAD 已合入，但工作区**不干净**，`git worktree remove`（未加 `--force`）
如实拒绝，已全部跳过保留。

若当时图省事加 `--force`，这些内容会无声消失。

## 保留的三棵

### `idp-parcel-adr-0065-storage`（HEAD `5073851`，已合入）

已提交部分对应 main 的 `071eae0`（追踪投影版本只增不改写）。工作区另有 **12 个已改文件 + 1 个未跟踪文件**，
含新迁移 `migrations/visibility_exception/0016_tracking_projection_versions.sql`，
以及 `domain/tracking_projection.go`、`ports/ports.go`、`adapters/postgres/projection.go`
与六个适配器测试的改动。

未定：这是 `071eae0` 之后的续做、还是被 `071eae0` 取代的旧稿。**迁移文件尤其要判**——
`0016` 是否与 main 现有迁移号冲突未查。

### `idp-parcel-syn-wall-door-audit`（HEAD `49a2ab0`，已合入）

工作区有 **11 个已暂存（`A`）文件**，是一份完整的墙/门审计：`.scratch/syn-wall-door-audit/report.md`
加 10 张票（准入通道登记册、生产归属权威无适配器、PC 申报无发布写口、网络定义登记册无写口/无解析器、
自动改道事实目录未实装、CC 案件配置登记册无写口、价卡与费率表无版本仓储、计价参考序列登记册缺失、
VE 规则与策略登记册无写口、采认资格证据源未实装）。

这是**分析产物、不是代码**，与实例墙清单直接相关，看起来是有价值的一手清点。从未提交。

### `idp-parcel-bento-gate-reeval`（HEAD `49a2ab0`，已合入）

工作区有未跟踪目录 `.scratch/bento-gate-reeval/`。内容未读。Bento 持久化闸门是
AGENTS.md「当前默认切片」里仍在阻断的一项，这份重估可能与之相关。

## 建议处理顺序（未获点头，勿擅自执行）

先读后判，逐棵定：内容有价值 → 只 `git add` 该票文件提交并走集成口；已被取代 → 记明理由再拆。
`.scratch/` 类（后两棵）比代码类风险低，可先办。

## 其余口径（无需动作，记此备查）

同批 MERGED 且工作区干净的六棵已拆：`b4`、`dispatch-db-ready`、`integrate-delivery-a`、
`integrate-pickup-a`、`integrate-proj-a`、`integrate-proj-b`。

`idp-parcel-inspect-03`（detached `a097d7f`，干净、已合入）**故意保留**：名字指向外部评审 03 票，
而 MCP-4 此刻正在办该票，不排除它要用。归属确认前不动。

大量 `cons-*` / `ps-*` / `syn-*` / `product-version-closure-*` 树按 SHA 判为 UNMERGED——
多因集成时另起 `integrate-*` 分支重做提交，**内容可能已在 main 而 SHA 不是祖先**。
不可按本票口径拆，逐棵判成本高，暂全部保留。

## Comments

- 2026-08-20 MCP-1：MCP-2 / MCP-3 死讯后按用户裁定清点残留树时发现。
- 2026-08-20 MCP-1：用户指示清理。已按「先封存再拆」执行：三棵树的未提交现场各自
  原样提交进其分支（`git add -A` 仅限这三个死会话分支的封存提交，非集成提交），随后
  `git worktree remove`（未加 `--force`）全部干净拆除。**内容零损失**，裁断入口从工作区
  变为分支引用：
  - `adr-0065-storage` → `b72d96e`（13 文件 +361/−47，含迁移 `0016`；续做还是旧稿仍未判）
  - `syn-wall-door-audit` → `a958029`（11 文件 +419，审计报告加 10 张票）
  - `bento-gate-reeval` → `2399ecd`（5 文件 +135，取证脚本与证据输出）
  三棵树已不存在；本票剩余问题只剩**内容采纳与否**。分支在裁断前不删。
  另：`inspect-03` 树已消失（MCP-4 自拆），本票中「故意保留」一条就此了结。
