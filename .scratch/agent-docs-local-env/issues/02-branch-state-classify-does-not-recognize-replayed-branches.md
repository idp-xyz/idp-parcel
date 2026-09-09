# `branch-state.ps1 -Classify` 认不出被重放过的分支：blob 相等判不了「加行都在 main 里」

Category: bug
Status: resolved——2026-09-09 16:2x 通道 6，分支 `mcp6-adle02` 基 `8f020eff`，代码 tip `18489aeb`；完成记录见文末。判据 2 的第三个案例实测与票面预期相反且有证据，记在完成记录里等 owner 定。
立票说明（通道 1 代裁，用户授权自决）：另一会话 09-09 实测 `-Classify` 对 12 / 13 / 14 / 15 的分支误判（报 `ABSORBED`）且误报未推 origin，
「直接比较文件历史和本地 / 远端 SHA 后均不成立，当前分类输出不能作为回放依据」。分类是推送方改名 `merged/` 前的门，门不准就只能靠人肉 range-diff
Blocked by: 无

## 症状与成因

`-Classify` 的判据（脚本注释原文）：「分支改过的每份非簿记文件，其 tip blob 是否在 main 该文件历史里出现过」。这对 **cherry-pick 零冲突**的重放成立（blob 原样进 main），
对**推送方逐 hunk 手工并过的共享文件**不成立——本册的加行夹在邻册的加行之间，main 上那份文件的任何一个历史版本都不等于分支 tip 的 blob，于是判
`NOT-ABSORBED`；而对**还没重放**的分支，若它改的文件恰好都已被别的票以相同内容带进 main（同一现场两条分支各封存一次），又会判 `ABSORBED`。两种错各一边。
「未推 origin」那半：脚本按 `ls-remote --heads` 的分支名比 SHA，远端已删或改名就报未推——09-09 起 `merged/` 指针不推远端且远端旧名分支已清，
这一格对 `merged/` 分支报「未推」是对的、对在途分支才有意义；症状报告里的误报可能是 fetch 过期，本票复现一次再定。

## 完成判据

1. **换判据为「加行多重集包含」**：对分支自 merge-base 起改过的每份非簿记文件，取该文件在分支上的净加行（`git diff <mb> <b> -- f` 的 `+` 行多重集），
   判它是否是 main 上该文件当前内容行多重集的子集；删行同理反向（分支删掉的行不在 main 上）。全部文件成立 → `ABSORBED`；否则列出不成立的文件与差的行数。
   这正是推送方 09-09 手工核每份共享文件用的判据（票 13「进 main 记录」），脚本只是把它算出来。
2. 用三个真实案例回归：`merged/mcp6-awf13`（六份共享文件全手工并）应 `ABSORBED`；`salvage/mcp5-awf13`（封存现场，内容经 `896994ac` 另一条路进了 main）应
   `ABSORBED`（内容层面它确实被吸收了——脚本输出要**同时**注明「tip 不是 main 祖先、经重放进入」，别让人误以为可以 `--is-ancestor`）；`salvage/mcp4-tf03`
   应 `NOT-ABSORBED`。三个结果与解释写进脚本头注当例子，钉 SHA。
3. 「未推 origin」一格：对 `merged/` / `salvage/` 前缀不判（规矩是不推）；对在途分支先 `git fetch --prune` 再比，报文里带远端 SHA。
4. 保持只读、PS 5.1 可跑、`.ps1` 带 BOM（同文件头注的先例）；`-Classify` 在 100 支 ref 上跑完 < 60 秒（写进头注）。

## 边界

只改 `scripts/branch-state.ps1` 与其头注；不改 parallel-sessions 的流程（它已经写「改名前跑 -Classify」）；`-Audit` 不动。

## 完成记录（2026-09-09，通道 6，分支 `mcp6-adle02` 基 `8f020eff`）

逐笔：`1b39313c` 领票 → `18489aeb` 脚本与头注（唯一一笔代码）→ 本笔票面。取证时 main 在 `8f020eff`（red）与 `56ed4111`（green；通道 1 中途把重放链推进了 main，awf/24 的 `8eefc6ac` 恰好改了 awf/13 刚并进去的文件，于是判据 1 的「当前内容」半句在同一小时内就被现实检验了一次，见下）。

### red（旧判据，main `8f020eff`，PowerShell 5.1）

旧 `-Classify` 只判在途分支，三个案例带前缀进不了它；把它那段判据原样抄到临时脚本对三个 ref 跑：

- `merged/mcp6-awf13@da2736e5` → NOT-ABSORBED，点名八份以上（`CommercialPoliciesPage.tsx`、`publication_draft_payload.go`、`publication_canonicalization.go`…）——票面预期 ABSORBED，误报。
- `salvage/mcp5-awf13@feaf5bbb` → NOT-ABSORBED 两份（`publication_draft_payload.go`、`publication_canonicalization.go`）——票面预期 ABSORBED，误报。
- `salvage/mcp4-tf03@44808f31` → NOT-ABSORBED 两份（`end_fulfillment_participation.go`、`register_transport_handover.go`）。

另外三处不在票面上、复现时撞出来的 PowerShell 5.1 绑定暗礁，每一处都让旧脚本整页中断而不是答错：

1. 旧 `-Classify` 在真实在途分支上跑，第一支改了多于一份文件的分支就 `fatal: … Filename too long` 死掉——`G` 的 `[string[]]` 剩余参数把 `$files` 数组拼成一个空格连接的串交给 git。所以旧 `-Classify` 在 PS 5.1 下从未对多文件分支给过答案；09-09 那份「12/13/14/15 报 ABSORBED」的输出在 PS 5.1 下复现不出来，应当来自别的跑法（如 pwsh，本机没装、未验）。
2. `G` 调用里裸的 `--` 被绑定器当「参数结束」吃掉，git 收不到路径分隔符；文件不在工作树里（分支新增、main 尚无）就 `no such path in the working tree`。`-Path` 一节同病，一并改成 `'--'`。
3. 默认（不带 `-NoFetch`）那行 `G fetch origin --prune`：fetch 有任何更新都往 stderr 写摘要，`2>$null` 在 Stop 下把它变成终止错误，脚本在第一行就死——即默认跑法只在远端毫无变化时才出得来页。实测：删掉一支 `origin/*` 镜像再跑旧行，退 1、一行不印；新行 `-q` + try/catch 同一情形正常出页。

「未推 origin」误报没能复现：`8f020eff` 时六支在途分支 `ls-remote` 与本地 SHA 全部一致。旧代码走的是 `ls-remote` 活查，fetch 过期影响不到它；能让它对在途分支报「未推」的只有 ls-remote 本身失败——而那在 Stop 下是整页死，不是一格误报。判据 3 照做：改比 fetch --prune 后的 `origin/<分支>` 镜像并带远端 SHA，fetch 失败写「远端未判」；`-NoFetch` 时注明按本地镜像。

### green（新判据，main `56ed4111`，PowerShell 5.1）

```
- `merged/mcp6-awf13` @ da2736e5：ABSORBED · tip 不是 main 祖先、经重放进入（--is-ancestor 判不出） · 之后 main 又改过：internal/partycommercial/domain/publication_canonicalization_pre_acceptance_financial_control_policy.go（吸收于 53480472，之后 main 又改过）
- `salvage/mcp5-awf13` @ feaf5bbb：ABSORBED · tip 不是 main 祖先、经重放进入（--is-ancestor 判不出） · 之后 main 又改过：internal/partycommercial/domain/publication_canonicalization_pre_acceptance_financial_control_policy.go（吸收于 53480472，之后 main 又改过）
- `salvage/mcp4-tf03` @ 44808f31：ABSORBED · tip 不是 main 祖先、经重放进入（--is-ancestor 判不出）
```

**第三个案例与票面预期相反，且实测站得住**：`salvage/mcp4-tf03` 封存于 09-04 13:11，同一分钟 MCP-3 以 `4cbe5266`（「结束参与依赖缺席改为整笔不落……MCP-3 裁『采』」）把同一份改动提进了 main——五份文件 `--stat` 逐份一致，tip 与该提交只差 `56eb4eac` 先到的 `Judgments` 几行，旧判据正因为这几行报两份 blob 不同。按判据 1 它就是 ABSORBED；「封存现场 = 未集成」这个直觉在这支上不成立。头注照实写，票面预期不改，归 owner 定。

**判据 1 的「当前内容」半句在实测中不够**：`8f020eff` 时 awf/13 的反查循环在 main 当前内容里，`56ed4111`（awf/24 的 `8eefc6ac` 进 main 后）就不在了——同一支 `merged/mcp6-awf13` 严格按「当前内容」会从 ABSORBED 翻成 NOT-ABSORBED，而它什么都没丢。所以加了一层回看：当前不成立时，一次 `log -p -U0` 取 main 自 merge-base 起对该文件每笔提交的加/删行，从最早一笔累加，覆盖到即「吸收于 `<SHA>`，之后 main 又改过」。没这一层，分支刚并进去、邻票随即重构同一批行就误报，而这在 09-09 的节奏下是常态而不是例外。

删行那一半也偏离了票面字面「不在 main 上」：实现比的是 `git diff <merge-base> main` 的净删行多重集，不是「merge-base 版本减 main 当前版本」——后者会被别人在同一文件里新加的同文行（空行、右花括号）抵消，把正当删掉的行算成仍在。

`-Branch a,b,c`（隐含 `-Classify`）是判据 2 要求的回归案例能跑的前提：三支都带前缀，默认名单进不去。经 `powershell -File` 传进来的 `[string[]]` 是一个逗号连接的串，脚本自己拆。

### 判据 4

只读；PS 5.1 `powershell` 跑通（本机无 pwsh，PS 7 未验）；入库 blob 头三字节 `EF BB BF`、无 CR（`git ls-files --eol` 报 `i/lf`）。耗时：`-Branch <138 支本地 ref>` 27 秒、1057 次 git 进程（第一版 84 秒 / 3370 次，砍法：show 前不再 rev-parse 探存在、is-ancestor 由 merge-base 推、每支分支的 diff 与 log -p 各一次批量取、逐行计数内联不抽函数）。不带 `-Classify` 的状态页、`-Path`（含指向工作树里不存在的文件）各跑一遍正常。

### 全部 ref 上的分类结果，供推送方回看

138 支：ABSORBED 116（其中 64 支带「之后 main 又改过」）、NOT-ABSORBED 22（在途 5、salvage/ 8、**merged/ 9**）。九支 `merged/` 报 NOT-ABSORBED 是本票的副产品，不是误报——抽了两支看：`merged/cons-proj-tf-b` 的 `cmd/parcel-dispatch/effective_delivery_projection_test.go` 147 行从未进 main 也没有任何 main 提交加过它们；`merged/mcp4-bento-pbc02` 是决策简报里一行表格被推送方并入时改写了措辞。其余七支：`cr04-backend`（`migrations/line_endings_test.go`）、`mcp3-awf09` 与 `mcp4-awf10`（`publication_canonicalization.go` 一到八行）、`mcp3/admin-skeleton-closure`（几张票面各一行）、`mcp5-frontline-import`、`pc01-identity-face`（多份）、`t14-payload-digest`（一张票面）。跑一遍 `-Branch` 逐支看即可，脚本会列文件与行数；要不要追归推送方。

### 未动与建议

- `-Audit` 未动，但它有同一处裸 `--`（`G log main --format=%H -- $f`）和逐文件 blob 对历史的旧判据；票面边界说不动，另票。
- `docs/agents/parallel-sessions.md` 未动；若要写「`-Branch` 可复核 merged/ 指针」一句，另票。
