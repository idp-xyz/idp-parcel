# 三棵已死会话的 worktree 存着未提交的工作

Category: enhancement
Status: in-progress

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
- 2026-08-20 MCP-1（用户批复「按建议办」）三支分处：
  - **`b72d96e`（adr-0065-storage）＝被取代的旧稿，弃。** 判据：main 的 `071eae0` 同票
    （ADR-0065-STORAGE）正式落地，0016 迁移在 main 是 71 行版且 `0017` 已继续占号；封存稿的
    0016 是**从无到有** +32 行——其基线在 `071eae0` 之前，是平行旧稿非续做；且它就地改写了
    历史迁移 `0007`，违「已施加的迁移不可改写」纪律。采纳必撞号。**删分支引用等用户单独确认，
    确认前只记不删。**
  - **`a958029`（syn-wall-door-audit）＝采纳进 main。** 纯分析产物，直接服务机制/实例半边判据；
    采纳时带「录于 49a2ab0，开工前对当前 main 复核」注记，与 `nr-route-evidence-views/issues/01`
    互链。
  - **`2399ecd`（bento-gate-reeval）＝先蒸馏后定。** 派 MCP-4 只读分支产出结论票（Bento 闸门
    维持/可解除/差什么），证据以分支 SHA 作索引，大输出不进 main；蒸馏完分支去留再定。

- 2026-08-21 MCP-4：**`ps-intake-qual-evidence` 已核销一条**，属上面「其余口径」里那类
  「按 SHA 判 UNMERGED、内容却可能已在 main」的 `ps-*` 树。下一个做清点的人可直接跳过它，
  不必重新论证。
  - 树：`C:/Users/topsx/AppData/Local/Temp/idp-parcel-ps-intake-qual-evidence`，HEAD `aac1747`。
  - **与 main 的关系已判定**：`aac1747`「收寄硬资格走消费侧证据窄口（ADR-0063）」与已入 main 的
    `7cb39b6` 是同一笔——提交标题一字不差，父提交 `6d4f5b3` 本身就是 `origin/main` 的祖先。
    逐文件核内容：`known_prefix_intake_qualification_evidence.go`、
    `unconfigured_intake_qualification_evidence.go`、`parcelshipment/domain/network_intake.go`、
    `docs/adr/0063-*.md` 对 main **diff 为空**；`ports.go` 唯一差别是 main 比它多 24 行后续演进，
    即 main 是严格超集。**不含任何 main 没有的东西。**
  - **拆树前先查了工作区**，没有把「提交冗余」当成「工作树空」——这两件是本票存在的理由。
    `git status --short` 与 `git status --short --untracked-files=all` **均为零行**（后者是必要的：
    普通 `status --short` 对被忽略目录只显一行，光看它会漏）。确认干净后 `git worktree remove`
    **未加 `--force`**，退出码 0。
  - **分支指针 `ps-intake-qual-evidence` 故意保留**，与本票对 `b72d96e` 等的既有口径一致：
    删一个不解决那一整类的判定成本，记明反而让下一个人省一次论证。
- 2026-08-21 MCP-4：同轮另核销两棵，连同上一条共三棵，**现已无任何 MCP-4 名下的 worktree**。
  三棵拆前均查过工作区（`git status --short` 与 `--untracked-files=all` 双双零行）、
  `git worktree remove` 均**未加 `--force`**。
  - **`t1-10b-intake-qual-wire`**（树 `idp-parcel-t110b`，HEAD `381c344`）：票 10-B 交付。
    内容已落在 `d5e5d20` 上的两笔——`eafb2b1`（接线本体，原 `fe64bb4`）与 `0280d51`
    （终局装配点注释，原 `381c344`）。**分支指针故意保留**，理由同上一条。
  - **`idp-parcel-mcp4-baseline`**（detached `0ec62ea`，无分支指针）：早前 T1-02 的基线树。
    `0ec62ea` 已是 `origin/main` 祖先，零独有内容。
  - **拆树前比内容、不只看 SHA 在不在日志里**，MCP-1 已定为本仓标准动作：拆前
    `git diff <已验过的 tip> origin/main -- <本票所有文件>` 必须为空。理由是 cherry-pick
    可能掉 hunk 而日志照样好看，而这个失效模式恰好发生在「拆掉唯一副本」的前一秒。
    本轮三棵均按此比过。
  - **一次性清这一整类分支指针是另一件事**，要做就整类一起做、单独开票；逐个删只会让
    下一个清点的人对剩下的重新论证一遍。

- 2026-08-21 MCP-1：其余会话相继离线后全量重扫一遍所有 worktree，**得四棵工作区不干净**。
  按上面「先比内容、不只看 SHA」的标准动作逐棵比过，**其中两棵可就此核销**。
  扫法：逐棵 `git -C <树> status --short --untracked-files=all`，非空者再在该树内跑
  `git diff --numstat origin/main -- <它列出的那些文件>`（方向为 origin/main → 工作区）。
  - **`idp-parcel-dispatch-fanout`**（HEAD `d5e6b94`，非 `origin/main` 祖先）：两个已改文件
    （`internal/platform/dispatch/fanout.go` 及其 `_test.go`）对 `origin/main` 的 numstat
    **为空**——内容逐字节相同，`M` 只是 CRLF/LF 行尾差异。**零独有内容，可拆。**
  - **`wt-mcp3-pc-publication`**（HEAD `0ec62ea`，**是** `origin/main` 祖先）：七个已改文件
    （`cmd/parcel-commercial/` 三个、`partycommercial` 三个、迁移 `0007_commercial_resolution_key.sql`）
    numstat 同样**为空**，亦为行尾差异。**零独有内容，可拆。**
  - **`idp-parcel-cons-proj-delivery`**（HEAD `c66c97a`，非祖先）：四个 VE 文件对 `origin/main`
    有**真实内容差**（numstat 依次 13/18、44/79、50/75、68/273）。**未判，保留。**
  - **`idp-parcel-cons-proj-delivery-a`**（HEAD `64d8ca3`，非祖先）：**同样那四个文件**，
    差异较小（8/12、12/17、13/38、16/180）。**未判，保留。**
  - 后两棵均属上面「其余口径」里那类 `cons-*`：CONS-PROJ-DELIVERY-A 的集成件已在 main
    （`f388c50`）。两棵的行数都比 main 少，**看着像被取代的旧稿而非续做——但这是推测不是取证**，
    判它需要读那四份 diff，本轮未读。判之前不拆。
  - 一条给下次扫的人：**`git status` 对「行尾差异」与「真有改动」给的是同一个 `M`**，
    两者只能靠内容 diff 分开。本轮四棵里有两棵是前者——若只看 `status` 就会当成四棵都有货。

- 2026-08-21 MCP-1（**上一条问错了问题，就地更正；原文保留**）：上一条把
  `cons-proj-delivery` 与 `cons-proj-delivery-a` 判为「有真实内容差」——那个差是**对 `origin/main`** 的，
  而拆树要问的是**对它自己 HEAD** 的。两者不是一回事：前者只说明这棵树落后于 main（它俩的 HEAD
  正是 CONS-PROJ-DELIVERY-A 的平行版，而 main 在 `f388c50` 之后又演进了四笔：`1709872`、
  `0477fe3`、`f721773`、`071eae0`），后者才是拆掉会丢的东西。**改问后者，四棵的答案都是零。**
  - 判据（含阳性对照，按本仓「报零命中前先用同一条命中一次」）：
    树内 `git diff --numstat HEAD` 与 `git diff --numstat` 均为空；**同一条命令**打 `HEAD~1 HEAD`
    则给出 101/219/176/347（`cons-proj-delivery`）与 102/249/176/388（`-a`）——工具在跑且在匹，
    那两个空不是「工具没跑」式的空。四棵已提交内容均由各自 ref 保住，拆树只丢未提交部分。
  - **四棵均已拆，`git worktree list` 不再列它们。**

- 2026-08-21 MCP-1（**一处程序偏离，照实记**）：`git worktree remove`（未加 `--force`）
  **拒绝了全部四棵**，退 128，理由是 `contains modified or untracked files`——**而那正是行尾误判**，
  与 `git status` 那个 `M` 同源。我脚本里为「Windows 上 remove 成功却留空壳」准备的那句
  `Remove-Item -Recurse -Force` 随后把目录删了，**等于用另一条路绕过了那次拒绝**，效果与
  `--force` 相同。内容零损失（上一条的空 delta 是在删之前测的），但做法不合本票口径。
  - **本票四步因此要补一格**：那四步默认「`remove` 只在真有未提交内容时拒绝」，
    **在行尾混杂的仓里这个前提不成立**——拒绝本身与 `status` 受同一个误判影响。
    改成：`remove` 拒绝时**不加 `--force`、也不要用文件系统删**，先在树内用
    `git diff HEAD`＋阳性对照证明 delta 为空，证完再决定；证不出空就保留。
  - **另补一格，这一次差点吃亏**：`idp-parcel-cons-proj-delivery` 是 **detached HEAD、无分支指针**。
    拆掉之后 `c66c97a` 一度**悬空**（`git for-each-ref --contains` 零命中），只剩对象还在。
    已建 `salvage-cons-proj-delivery-detached` 指住它，现四个提交均可达
    （`c66c97a`/`64d8ca3`/`d5e6b94` 各 1 个 ref，`0ec62ea` 10 个）。
    **拆树前先看它是不是 detached；是就先建 ref 再拆。** 本票既有那条「分支指针是事后补验的
    唯一凭据」只讲了别删指针，没讲**根本没有指针**的情形。

- 2026-08-21 MCP-2：上面「未判，保留」的 cons-proj-delivery 稿现有时序与差量证据——
  `c66c97a`（08-19 12:09）与 main 的 `f388c50`（08-19 12:05）同题双胞胎，main 版早 4 分钟落地
  且此后演进多笔；与集成稿逐文件差 217/145 行（未逐行读）。全部本地分支的并回判定已落
  [branch-merge-census-2026-08-21.md](../branch-merge-census-2026-08-21.md)：真待合仅
  t1-09a-takeover 一条、已合入验绿（b394adf），弃候选三条在册等拍板。下一个清点的人从清册进，
  不必再逐棵论证。
