# 58 `owner-review-queue.ps1` 只认没人写的 `Status: blocked`，路径匹配又只认 Windows 分隔符

Category: bug
Status: ready-for-agent——R3（不急）：复核队列是给 owner 看的汇总页，不挡任何业务路径；但在 WSL 里跑它，第二节永远是空的。2026-10-21 文档自洽 10（10b）立（用户「你作为业务和系统专家自决」授权），分支 claude-dc10b。
Blocked by: 无
出处：[audit/cross.md](../audit/cross.md) AG·L6（归 B，10b 代决）。取证于 main `4e7874ba0`。

## 现象

- `scripts/owner-review-queue.ps1` 第二节按 `^Status:\s*(needs-info|blocked)` 取票。[triage-labels](../../../docs/agents/triage-labels.md) 的状态角色里没有 `blocked`；本仓的阻塞写法是票面的 `Blocked by:` 一行（AGENTS.md「红线」一节「立票与分诊时」那句）。票面里从没出现过 `Status: blocked`。
- 同一节用 `$_.DirectoryName -match '\\issues$'` 筛票、`Replace($repo + '\', '')` 算相对路径，只认 Windows 分隔符；照 [workflow「当前宿主」](../../../docs/agents/workflow.md#本机环境)在 WSL 里用 `pwsh` 跑时一张票都筛不进来。
- 10b 裁定：不在 triage-labels 补 `blocked` 角色，脚本与文档都改认 `Blocked by:`（[parallel-sessions](../../../docs/agents/parallel-sessions.md) 那一句已改为「`needs-info` 与带 `Blocked by:` 的票」）。

## 三类

机制（脚本）。无产品策略、无租户取值。

## 规模

S。

## 最小的诚实修法

1. 第二节改为两类各取：`Status:` 以 `needs-info` 起头的票，与 `Blocked by:` 一行不是「无」起头的票；节标题随之改为「needs-info 与带 Blocked by 的票」。
2. 目录筛选与相对路径改用 `[IO.Path]::DirectorySeparatorChar` 或先把路径统一成 `/` 再匹配，WSL 与 Windows 两种跑法都能筛中。
3. 改完在 WSL 里跑一次，第二节列得出今天带 `Blocked by:` 的票。

## 要裁的

`Blocked by:` 写「无」或「无（……）」的票不算阻塞；写已关闭票号的算不算，由修的人照票面原文判，拿不准就列出来。
