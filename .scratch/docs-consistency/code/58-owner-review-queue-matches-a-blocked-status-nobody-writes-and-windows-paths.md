# 58 `owner-review-queue.ps1` 只认没人写的 `Status: blocked`，路径匹配又只认 Windows 分隔符

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-tool（见文末「完成记录」）。原 ready-for-agent——R3（不急）：复核队列是给 owner 看的汇总页，不挡任何业务路径；但在 WSL 里跑它，第二节永远是空的。2026-10-21 文档自洽 10（10b）立（用户「你作为业务和系统专家自决」授权），分支 claude-dc10b。
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

## 完成记录（2026-10-27，作者会话，分支 claude-tool，基 origin/main `f6a4a5298`）

### 做了什么

只改 `scripts/owner-review-queue.ps1`：

- 第二节标题改为「needs-info 与带 Blocked by 的票」，两类各取：`Status:` 以 `needs-info` 起头的票；未了结的票（`Status:` 属 `needs-triage` / `needs-info` / `ready-for-agent` / `in-progress` / `ready-for-human` / `draft`）里 `Blocked by:` 一行不以「无」起头的票。每条列 `Status:` 行，带 `Blocked by:` 的另列该行原文（各截 200 字）。头注与页头的「needs-info / blocked」同改。
- 路径：目录筛选改成先把 `\` 抹成 `/` 再匹配 `/(issues|code)$`；相对路径统一走一个 `Rel` 函数（两种分隔符都抹成 `/`，再去掉仓库根前缀）。`$repo` 不再被换成反斜杠——那一换在 WSL 里让 `git -C $repo` 直接 fatal（页头钉的 SHA 是空的），也让路径拼接落空。第一节的 `docs\adr` 拼接同改 `docs/adr`。
- 第三节（票面里提到越权风险点的票）与第二节共用同一个票清单，在 WSL 里同样是空的，同一处修好。
- 没有动 `.scratch/owner-review-queue.md`：那是钉着旧 SHA 的生成物快照，由要用它的人重生成。

### 现行 `Status:` / `Blocked by:` 的实际形态（`.scratch` 下 `issues/` 与 `code/` 共 880 张有 `Status:` 的票）

- `Status:` 首词：`resolved` 占绝大多数；其余是 `needs-triage`、`ready-for-agent`、`in-progress`、`draft`、`ready-for-human`、`needs-info`（3 张），以及少数 `done`、`superseded`、`wontfix`、`decided`、`Accepted`、中文起头的收尾写法。`Status: blocked` 一张都没有（审计里提到的 `Status: block` 是 `seed-completeness/issues/03` 的「block 1 done、block 2 已取证」，指的是分块，不是状态）。
- `Blocked by:` 不以「无」起头的有 203 张，其中 156 张的 `Status:` 已了结——那是历史边；其余 47 张是未了结的。

### 代决

1. **「阻塞」= 未了结的票 + `Blocked by:` 不以「无」起头。** 已了结的票不入：203 张里 156 张是历史，全列会把这一节淹成 200 条，owner 看不到真正在等的。票面「要裁的」说「写已关闭票号的算不算，由修的人照票面原文判，拿不准就列出来」——对已了结的票整张判为历史；对未了结的票只按「无」起头与否分，不去解析被挡的票号是否已关闭。
2. **未了结的票里写着「已 resolved」的票号，照列并带出原文。** 今天只有一张（`label-channel-service-first-release/issues/20`，「18（已 resolved）」之外还挡在别的票上），行内字面就写着，读的人一眼能判；解析票号状态要跨目录找文件，代价大于收益。
3. **以起头判，不以包含判。** `needs-info` 与 `Blocked by:` 无的判断都锚在行首，防止 Status 行叙述里提到 needs-info 的旧状态被误收（`.scratch` 下有 16 条 `Status:` 行在行中提到 needs-info，起头的只有 3 张）。
4. **票目录加上 `code/`。** `.scratch/docs-consistency/code/` 有 94 张同格式的票（含在途、带 `Blocked by:` 的 75、78），只认 `issues/` 会漏掉它们；目录名认 `issues` 与 `code` 两个。副作用是第三节也多收这一组里提到越权风险点的票。
5. **不补 `blocked` 角色。** 10b 已裁（见票面现象）；本票只让脚本认 `Blocked by:`。
6. 判据只用 `Status:` 与 `Blocked by:` 各取文件里第一处行首匹配，与原来取第一处 `Status:` 一致；`Blocked by:` 空值（行内没有内容）按「不是无」处理，即列出。

### 越权风险点

- 「未了结」的集合是写死的六个状态词。新增状态角色（`docs/agents/triage-labels.md`）时，这个集合不会自动跟上；漏进来的新状态表现为该状态的票不被第二节收，不会误收。
- 第三节因加了 `code/` 目录，由在 WSL 里的 0 张变为 381 张；其中多少来自 `code/` 没有单独核。
- `.scratch/owner-review-queue.md`（已提交的快照）仍是旧口径的产物，第二节为空；下次谁重生成它时才更新。
- Windows PowerShell 5.1 没有实跑（见验证）。

### 验证

- 本机有 PowerShell 7.6.6（`pwsh`），在 WSL 里对仓库当前 `.scratch` 实跑：改前第二节「共 0 张」、第三节「共 0 张」、页头 SHA 为空（`git -C` fatal）；改后第二节 47 张、第三节 381 张，页头 SHA 为 `f6a4a5298`，路径都是 `.scratch/…` 相对形（正斜杠）。
- 用一段 python 按同一口径复算（`issues|code` 目录，未了结 + `Blocked by:` 非「无」或 `needs-info`；越权风险点同原口径），第二节 47、第三节 381，两节的文件集合与 pwsh 的输出逐一相等。3 张 `needs-info` 的票都在 47 张里（它们也带 `Blocked by:`）。
- 变异（改脚本的副本，在仓库根用 pwsh 跑）：
  - 目录筛选改回只认 `\issues$` → 第二节 0 张，被杀。
  - 去掉「未了结」条件 → 203 张，被杀（淹没）。
  - 去掉「不以无起头」条件 → 140 张，被杀。
  - `Rel` 不再去掉仓库根前缀 → 路径变成 `/home/…` 绝对路径，被杀。
  - 目录筛选只认 `issues`（去掉 `code`）→ 45 张，少 75 与 78，被杀。
- Windows PowerShell 5.1：本机 `powershell.exe` 版本 5.1.26100，但它的 PATH 里没有 `git`，且执行策略拦从 WSL 共享路径跑脚本；绕过执行策略的那条命令被权限检查拒了，没有强行。所以脚本在 5.1 上没实跑。改动只用 5.1 里都有的写法（`-replace`、`StartsWith(…, OrdinalIgnoreCase)`、`Substring`、`Where-Object`、`Select-Object -First`），没用 `[IO.Path]::GetRelativePath`（5.1 没有）；`$repo` 在 Windows 上是 `C:/…`，拼接与匹配都认。
- 不涉及 Go；`go build ./... && go vet ./...` 绿，`gofmt -l .` 无输出。

### 剩余项

无。5.1 的实跑留给在 Windows 宿主上用这个脚本的人。
