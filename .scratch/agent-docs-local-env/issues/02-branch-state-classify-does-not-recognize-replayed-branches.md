# `branch-state.ps1 -Classify` 认不出被重放过的分支：blob 相等判不了「加行都在 main 里」

Category: bug
Status: in-progress——2026-09-09 15:4x 通道 6 领票，分支 `mcp6-adle02` 基 `8f020eff`，树 `D:/tops/idp-parcel-mcp6-adle02`。立票说明（通道 1 代裁，用户授权自决）：另一会话 09-09 实测 `-Classify` 对 12 / 13 / 14 / 15 的分支误判（报 `ABSORBED`）且误报未推 origin，
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
