# branch-state.ps1 —— 把「谁在分支上、什么没进 main」从 git 算出来，不靠会话自报、不靠台账。
#
# 用法（在仓库任一目录，PowerShell 5.1 / 7 均可）：
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1              # 默认：先 fetch --prune，再出状态页
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1 -NoFetch     # 离线：不碰网络，远端一格按本地 origin/* 镜像判并注明
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1 -Path internal/partycommercial   # 派单第 0 步：这块地盘上谁有半成品
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1 -Classify    # 给未加前缀的分支判 ABSORBED / NOT-ABSORBED（改名 merged/ 前跑）
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1 -Branch merged/mcp6-awf13,salvage/mcp5-awf13   # 只判点名的分支（隐含 -Classify），任何前缀都行，逗号分隔
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1 -Audit       # 游离提交审计（慢，几分钟）
#
# 约定（见 docs/agents/parallel-sessions.md「拆工作树」「派发前先点名」两节）：
#   merged/*  —— 内容已全进 main 的指针；salvage/* —— 只防丢、不集成；未加前缀 —— 在途或归用户。
#   本脚本只读，不改任何 ref、不动工作树。输出是 Markdown，可直接贴进完工报或 tasks.md。
#
# -Classify 的判据（2026-09-09 换，票 .scratch/agent-docs-local-env/issues/02-branch-state-classify-does-not-recognize-replayed-branches.md）：
#   对分支自 merge-base 起改过的每份非簿记文件——
#     加行：分支净加行的多重集 ⊆ main 上该文件**当前内容**的行多重集；
#     删行：分支净删行的多重集 ⊆ main 自 merge-base 起对该文件的净删行多重集（`git diff <merge-base> main` 的 - 行）。
#       不用「merge-base 版本减 main 当前版本」：那会被别人在同一文件里新加的同文行（空行、右花括号）抵消，把本分支正当删掉的行算成仍在。
#   全部文件两条都成立 → ABSORBED；否则列出不成立的文件与差的行数。比的是行内容不是位置，行尾 CR 不计，多重集按出现次数比。
#   当前内容不成立时再回看一层：main 自 merge-base 起对该文件每笔提交的加/删行从最早一笔起累加，累加到某一笔把本分支的净加行与
#   净删行都覆盖了，就算在那一笔被吸收、之后又被 main 改过——报文写「吸收于 <SHA>，之后 main 又改过」而不是 NOT-ABSORBED。
#   没有这一层，分支刚并进去、邻票随即重构同一批行，它就翻成 NOT-ABSORBED（09-09 awf/24 对 awf/13 正是这样）。
#   这正是推送方逐份共享文件手工核对用的判据，脚本只是把它算出来。换掉的旧判据是「tip blob 在 main 该文件历史里出现过」，
#   它对 cherry-pick 零冲突的重放成立，对推送方逐 hunk 手工并过的共享文件必然失败——本册加行夹在邻册加行之间，main 任何一个
#   历史版本都不等于分支 tip 的 blob。ABSORBED 的报文同时注明 tip 是否 main 祖先：经重放进入的分支 tip 不是祖先，
#   `git merge-base --is-ancestor` 判不出「内容已吸收」，别拿它当替代。
#   边界：比的是行的多重集，不是语义。只加了空行、右花括号这类满仓都有的行的分支会平凡地 ABSORBED；推送方并入时改写过
#   一行措辞（票面一句、表格一格）就会 NOT-ABSORBED 一行——报文给出行数，一行两行的差由人看一眼 range-diff 定。
#   三个回归案例（取证于 main 56ed4111，2026-09-09，PowerShell 5.1）：
#     merged/mcp6-awf13  @ da2736e5 → ABSORBED，tip 不是 main 祖先、经重放进入；publication_canonicalization_pre_acceptance_financial_control_policy.go
#       吸收于 53480472、之后 main 又改过（awf/24 的 8eefc6ac 把三个反查循环合一成一行委托）。旧判据报 NOT-ABSORBED，点名八份以上。
#     salvage/mcp5-awf13 @ feaf5bbb → ABSORBED，tip 不是 main 祖先、经重放进入。封存现场的内容经 896994ac 由 mcp6-awf13 那条路带进 main；旧判据报 NOT-ABSORBED 两份。
#     salvage/mcp4-tf03  @ 44808f31 → ABSORBED，tip 不是 main 祖先、经重放进入。票面预期 NOT-ABSORBED，实测不成立：同一分钟 MCP-3 以 4cbe5266
#       把同一份改动（五份文件、--stat 逐份一致）提进 main，tip 与该提交只差 56eb4eac 先到的 Judgments 几行——旧判据正因为这几行报两份 blob 不同。
#   耗时：-Branch <全部本地 ref>（138 支，同一 SHA 取证）PowerShell 5.1 实测 27 秒、1057 次 git 进程（每次约 25 毫秒，进程数就是耗时）；默认只判在途分支，秒级。
#
# 「未推 origin」一格只出现在在途分支一节：先 fetch --prune，再拿本地 origin/<分支> 镜像比 SHA，报文带远端 SHA；fetch 失败写「远端未判」而不猜。
#   merged/ 与 salvage/ 前缀不判——规矩是不推，它们在远端没有同名指针是常态。
param(
    [switch]$NoFetch,
    [string]$Path,
    [switch]$Classify,
    [string[]]$Branch,
    [switch]$Audit,
    [int]$SinceDays = 2
)
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = 'Stop'
$repo = (git rev-parse --show-toplevel 2>$null)
if (-not $repo) { Write-Error '不在 git 仓库里'; exit 2 }
$repo = $repo -replace '/', '\'
# 经这个函数传参给 git 有两处暗礁，都是 PowerShell 参数绑定吃掉了东西、git 收到的与写的不一样（09-09 复现于 -Classify）：
#   传数组必须用 @数组 展开（如下方 `'--' @files`）——[string[]] 剩余参数把一个数组实参拼成一个空格连接的串，git 当成
#   一个文件名报 `Filename too long`；路径分隔符 `--` 必须写成带引号的 `'--'`——裸的 `--` 是绑定器的「参数结束」记号，
#   进不了 $a，git 于是把后面的路径当修订解析，文件不在工作树里就报 `no such path in the working tree`。
function G { param([Parameter(ValueFromRemainingArguments = $true)][string[]]$a) & git -C $repo @a 2>$null }

# 簿记文件：谁的笔在后谁的数字盖前面，比内容时不算它们（同 parallel-sessions「生成物不占号」）。
$book = @('docs/product/MECHANISM-INVENTORY.md', 'docs/adr/README.md', '.scratch/tasks.md', 'internal/architecture/production_wiring_baseline.txt')
$codeRe = '\.(go|sql|ts|tsx|js|mjs|sh|ps1|yml|yaml|py)$'

# fetch 加 -q：成功时它把 ref 更新摘要写到 stderr，而 2>$null 在 Stop 下会把那段摘要当终止错误，脚本在第一行就死；
# 失败仍会抛，接住记 $fetchOk，远端一格据此写「未判」而不是猜「未推」。
$fetchOk = $false
if (-not $NoFetch) { try { G fetch -q origin --prune | Out-Null; $fetchOk = ($LASTEXITCODE -eq 0) } catch { $fetchOk = $false } }

$mainLocal = G rev-parse main
$mainRemote = G rev-parse origin/main
$now = Get-Date -Format 'yyyy-MM-dd HH:mm'
"# 分支状态页 · $now · main $($mainLocal.Substring(0,8))" 
""
"## main"
""
if ($mainLocal -eq $mainRemote) { "- 本地 main = origin/main = ``$($mainLocal.Substring(0,8))``" }
else {
    $lr = (G rev-list --left-right --count "main...origin/main") -split '\s+'
    "- **本地 main ≠ origin/main**：本地 ``$($mainLocal.Substring(0,8))``，远端 ``$($mainRemote.Substring(0,8))``，领先 $($lr[0]) / 落后 $($lr[1])"
}
if (-not $NoFetch -and -not $fetchOk) { "- **fetch origin --prune 失败**：下面凡是比远端的地方都按本地镜像算，且远端一格写「未判」" }
$dirty = @(G status --porcelain --untracked-files=all)
"- 共享树未提交：$($dirty.Count) 行" + $(if ($dirty.Count -gt 0) { "（先 ``git diff --ignore-cr-at-eol --name-only`` 分清 CRLF 幻影与真改动）" } else { '' })
""

"## 工作树"
""
$wt = @(); $cur = $null
foreach ($l in (G worktree list --porcelain)) {
    if ($l -like 'worktree *') { if ($cur) { $wt += $cur }; $cur = [ordered]@{ path = $l.Substring(9); head = ''; branch = '(detached)' } }
    elseif ($l -like 'HEAD *') { $cur.head = $l.Substring(5, 8) }
    elseif ($l -like 'branch *') { $cur.branch = $l.Substring(7) -replace '^refs/heads/', '' }
}
if ($cur) { $wt += $cur }
foreach ($t in $wt) {
    $p = $t.path -replace '/', '\'
    $st = @(& git -C $p status --porcelain --untracked-files=all 2>$null)
    $flag = if ($st.Count -gt 0) { "**未提交 $($st.Count) 行**" } else { '干净' }
    "- ``$($t.path)`` @ $($t.head) [$($t.branch)] — $flag"
}
""

"## 在途分支（未加 merged/ 或 salvage/ 前缀，且非 main）"
""
$inflight = @(G for-each-ref --format='%(refname:short)' refs/heads | Where-Object { $_ -ne 'main' -and $_ -notmatch '^(merged|salvage)/' })
if ($inflight.Count -eq 0) { "- （无）" }
foreach ($b in $inflight) {
    $tipFull = G rev-parse $b
    $tip = $tipFull.Substring(0, 8)
    $last = G log -1 --format='%ad' --date=format:'%m-%d %H:%M' $b
    $ahead = G rev-list --count "main..$b"
    $mb = G merge-base main $b
    $onlyHere = @()
    foreach ($f in (G diff --name-only --diff-filter=A $mb $b | Where-Object { $_ -match $codeRe })) {
        # 不用 cat-file -e：文件不在 main 时它往 stderr 写 fatal，在 $ErrorActionPreference = 'Stop' 下
        # 即使 2>$null 也会被 PowerShell 5.1 当成终止错误，整节中断、在途分支一行不印（09-08 MCP-5 实测）。
        # rev-parse -q --verify 同一问、不出声，与下方几处同一写法。
        $null = & git -C $repo rev-parse -q --verify "main:$f" 2>$null
        if ($LASTEXITCODE -ne 0) { $onlyHere += $f }
    }
    # 远端一格不再走 ls-remote 活查：它一失败就是整页死在这里；改比 fetch --prune 后的本地镜像 origin/<分支>，报文带远端 SHA。
    $remote = & git -C $repo rev-parse -q --verify "refs/remotes/origin/$b" 2>$null
    $mirrorNote = if ($NoFetch) { '（-NoFetch，按本地镜像）' } else { '' }
    $pushed = if (-not $NoFetch -and -not $fetchOk) { '远端未判（fetch 失败）' }
    elseif (-not $remote) { "**未推 origin**（fetch --prune 后无 origin/$b）$mirrorNote" }
    elseif ($remote -eq $tipFull) { "已推 origin（同 SHA ``$($remote.Substring(0,8))``）$mirrorNote" }
    else { $lr = (G rev-list --left-right --count "$b...origin/$b") -split '\s+'; "origin/$b @ ``$($remote.Substring(0,8))``，本地领先 $($lr[0]) / 落后 $($lr[1])$mirrorNote" }
    "- ``$b`` @ $tip · 最后提交 $last · 领先 main $ahead 笔 · $pushed · main 上从未有过的代码文件 $($onlyHere.Count) 件"
    if ($onlyHere.Count -gt 0 -and $onlyHere.Count -le 12) { $onlyHere | ForEach-Object { "    - $_" } }
}
""

if ($Path) {
    "## 地盘 ``$Path`` 上在途分支里、main 尚无的提交（派单第 0 步；merged/ 与 salvage/ 不列，它们的去向已判）"
    ""
    $any = $false
    foreach ($b in $inflight) {
        $hits = @(G log "main..$b" "--since=$SinceDays days ago" --format='%h %ad %s' --date=format:'%m-%d %H:%M' '--' $Path)
        if ($hits.Count -eq 0) { continue }
        $any = $true
        "- ``$b``："
        $hits | ForEach-Object { "    - " + $_.Substring(0, [Math]::Min(150, $_.Length)) }
    }
    $wtDirty = @()
    foreach ($t in $wt) {
        $p = $t.path -replace '/', '\'
        if ($p -ieq $repo) { continue }
        $d = @(& git -C $p status --porcelain --untracked-files=all -- $Path 2>$null)
        if ($d.Count -gt 0) { $wtDirty += "- ``$($t.path)`` [$($t.branch)] 工作副本里有 $($d.Count) 行未提交落在这块地盘上" }
    }
    if ($wtDirty.Count -gt 0) { $any = $true; $wtDirty }
    if (-not $any) { "- （无：这块地盘上没有 main 之外的提交，也没有未提交现场）" }
    ""
}

# 行多重集用区分大小写的 Hashtable：PowerShell 字面量 @{} 的键不分大小写，会把只差大小写的两行算成同一行。
# 计数循环都内联写，不抽成小函数——PowerShell 每次函数调用都要几十微秒，全部 ref 跑下来要数几十万行，抽出去就慢一倍。
function NewLineSet { New-Object System.Collections.Hashtable }
# main 上某文件的当前内容行多重集；文件不在 main 上算空集。不先 rev-parse 探存在与否，直接 show 并接住它的 fatal
# （2>$null 在 Stop 下把 stderr 变成可 catch 的终止错误），省掉一半 git 进程。
function MainLines {
    param([string]$file, [hashtable]$cache)
    if ($cache.ContainsKey($file)) { return $cache[$file] }
    $h = NewLineSet
    $lines = @(); try { $lines = @(G show "main:$file") } catch { $lines = @() }
    foreach ($l in $lines) { $k = $l.TrimEnd("`r"); if ($h.ContainsKey($k)) { $h[$k]++ } else { $h[$k] = 1 } }
    $cache[$file] = $h
    return $h
}
# 把一段 -U0 的 diff / log -p 输出按 `diff --git` 头切给已知文件，段内 + / - 行计入 $adds[文件] / $dels[文件]（多重集）。
# 头行按已知文件名精确匹配而不用正则截取，路径里有空格也不会切错。$onCommit 在遇到 40 位 SHA 行（log --format=%H）时回调。
function TallyDiff {
    param([string[]]$lines, [hashtable]$head, [hashtable]$adds, [hashtable]$dels, [hashtable]$binary, [scriptblock]$onCommit)
    $cur = $null; $inHunk = $false
    foreach ($l in $lines) {
        if ($head.ContainsKey($l)) { $cur = $head[$l]; $inHunk = $false; continue }
        if ($l.Length -eq 40 -and $onCommit -and $l -match '^[0-9a-f]{40}$') { & $onCommit $l; $cur = $null; continue }
        if (-not $cur) { continue }
        if ($l.StartsWith('@@')) { $inHunk = $true; continue }
        if (-not $inHunk) { if ($l.StartsWith('Binary files')) { $binary[$cur] = $true }; continue }
        if ($l.Length -eq 0) { continue }
        $c = $l[0]
        if ($c -eq '+') { $h = $adds[$cur]; $k = $l.Substring(1).TrimEnd("`r"); if ($h.ContainsKey($k)) { $h[$k]++ } else { $h[$k] = 1 } }
        elseif ($c -eq '-') { $h = $dels[$cur]; $k = $l.Substring(1).TrimEnd("`r"); if ($h.ContainsKey($k)) { $h[$k]++ } else { $h[$k] = 1 } }
    }
}
# $need 里每一行在 $have 里缺几次，加总。
function Shortfall {
    param([hashtable]$need, [hashtable]$have)
    $n = 0; foreach ($k in $need.Keys) { $g = $need[$k] - $(if ($have.ContainsKey($k)) { $have[$k] } else { 0 }); if ($g -gt 0) { $n += $g } }
    $n
}

if ($Branch) { $Classify = $true }
if ($Classify) {
    # -Branch 经 powershell -File 传进来时是一个逗号连接的串而不是数组（-File 的实参一律按字符串绑定），这里自己拆。
    $targets = if ($Branch) { @($Branch | ForEach-Object { $_ -split ',' } | ForEach-Object { $_.Trim() } | Where-Object { $_ }) } else { $inflight }
    $who = if ($Branch) { '点名的分支' } else { '未加前缀的分支' }
    "## 分类（${who}：自 merge-base 起每份非簿记文件，净加行多重集 ⊆ main 当前内容、净删行多重集 ⊆ main 自 merge-base 起的净删行；当前不成立再回看 main 之后各笔提交）"
    ""
    if ($targets.Count -eq 0) { "- （无）" }
    $tips = @{}; foreach ($l in (G for-each-ref --format='%(refname:short) %(objectname)' refs/heads)) { $p = $l -split ' '; $tips[$p[0]] = $p[1] }
    $mainCache = @{}
    foreach ($b in $targets) {
        $tipFull = if ($tips.ContainsKey($b)) { $tips[$b] } else { & git -C $repo rev-parse -q --verify "${b}^{commit}" 2>$null }
        if (-not $tipFull) { "- ``$b``：不是本地分支或提交，未判"; continue }
        $tip = $tipFull.Substring(0, 8)
        $mb = G merge-base main $b
        if (-not $mb) { "- ``$b`` @ $tip：与 main 无 merge-base，未判"; continue }
        # merge-base 就是 tip ⇔ tip 是 main 祖先，不必再开一个 --is-ancestor 进程。
        $isAncestor = ($mb -eq $tipFull)
        # --no-renames：改名拆成「旧路径全删 + 新路径全加」两份各自核，免得 rename 探测把两份并成一条只报新名。
        $files = @(G diff --name-only --no-renames $mb $b | Where-Object { $book -notcontains $_ })
        # 先筛掉 tip blob 已与 main 当前一致的文件——它们不必逐行比；剩下的才取净加/删行。
        $differ = @(); if ($files.Count -gt 0) { $differ = @(G diff --name-only --no-renames $b main '--' @files) }
        $pending = @(); $superseded = @()
        if ($differ.Count -gt 0) {
            $head = @{}; $adds = @{}; $dels = @{}; $binary = @{}
            foreach ($f in $differ) { $head["diff --git a/$f b/$f"] = $f; $adds[$f] = NewLineSet; $dels[$f] = NewLineSet }
            # 一次 diff 取全部待核文件的净加/删行；-U0 不带上下文行，段内只剩 + / - / \ 三种行。
            TallyDiff @(& git -C $repo -c core.quotepath=off diff --no-renames --no-color -U0 $mb $b '--' @differ 2>$null) $head $adds $dels $binary $null
            # 删行那一半比的是 main 自 merge-base 起的净删行，不是「merge-base 版本减 main 当前版本」：后者会被别人在同一文件里
            # 新加的同文行（空行、右花括号）抵消，把本分支正当删掉的行算成「仍在 main」。
            $withDels = @($differ | Where-Object { $dels[$_].Count -gt 0 })
            $mainDels = @{}; $mainAdds0 = @{}; $bin0 = @{}
            if ($withDels.Count -gt 0) {
                foreach ($f in $withDels) { $mainDels[$f] = NewLineSet; $mainAdds0[$f] = NewLineSet }
                TallyDiff @(& git -C $repo -c core.quotepath=off diff --no-renames --no-color -U0 $mb main '--' @withDels 2>$null) $head $mainAdds0 $mainDels $bin0 $null
            }
            $failing = @{}
            foreach ($f in $differ) {
                if ($binary[$f]) { $pending += "$f（二进制，tip blob ≠ main 当前）"; continue }
                $missing = Shortfall $adds[$f] (MainLines $f $mainCache)
                $stale = if ($dels[$f].Count -gt 0) { Shortfall $dels[$f] $mainDels[$f] } else { 0 }
                if ($missing -gt 0 -or $stale -gt 0) { $failing[$f] = @($missing, $stale) }
            }
            if ($failing.Count -gt 0) {
                # 当前内容不成立，未必没吸收过：main 之后可能又改了这些行（09-09 awf/24 把 awf/13 刚并进去的反查循环合一成一行委托，
                # 就让 merged/mcp6-awf13 在「当前内容」下翻成 NOT-ABSORBED）。一次 log -p 取 main 自 merge-base 起对这些文件每笔提交的
                # 加/删行，从最早一笔起累加，累加到哪一笔把本分支的净加行与净删行都覆盖了，就算在那一笔被吸收、之后又被改过。
                $names = @($differ | Where-Object { $failing.ContainsKey($_) })
                $steps = @{}; foreach ($f in $names) { $steps[$f] = New-Object System.Collections.ArrayList }
                $sa = @{}; $sd = @{}; $sb = @{}
                $commit = { param($sha) foreach ($f in $names) { $script:sa[$f] = NewLineSet; $script:sd[$f] = NewLineSet; [void]$script:steps[$f].Add(@($sha, $script:sa[$f], $script:sd[$f])) } }
                # log 按新到旧吐；TallyDiff 的回调在每个 SHA 行处为每份文件开一格新的加/删行计数，段内的行就落进当时那一格。
                TallyDiff @(& git -C $repo -c core.quotepath=off log -p --no-renames --no-color -U0 --format=%H "$mb..main" '--' @names 2>$null) $head $sa $sd $sb $commit
                foreach ($f in $names) {
                    $cumA = NewLineSet; $cumD = NewLineSet; $absorbedAt = $null
                    for ($i = $steps[$f].Count - 1; $i -ge 0; $i--) {
                        $step = $steps[$f][$i]
                        foreach ($k in $step[1].Keys) { if ($cumA.ContainsKey($k)) { $cumA[$k] += $step[1][$k] } else { $cumA[$k] = $step[1][$k] } }
                        foreach ($k in $step[2].Keys) { if ($cumD.ContainsKey($k)) { $cumD[$k] += $step[2][$k] } else { $cumD[$k] = $step[2][$k] } }
                        if ((Shortfall $adds[$f] $cumA) -eq 0 -and (Shortfall $dels[$f] $cumD) -eq 0) { $absorbedAt = $step[0].Substring(0, 8); break }
                    }
                    if ($absorbedAt) { $superseded += "$f（吸收于 $absorbedAt，之后 main 又改过）"; continue }
                    $why = @(); if ($failing[$f][0] -gt 0) { $why += "+$($failing[$f][0]) 行不在 main" }; if ($failing[$f][1] -gt 0) { $why += "-$($failing[$f][1]) 行仍在 main" }
                    $pending += "$f（" + ($why -join '、') + '）'
                }
            }
        }
        $rename = if ($b -notmatch '^(merged|salvage)/') { '（可改名 merged/）' } else { '' }
        $later = if ($superseded.Count -gt 0) { ' · 之后 main 又改过：' + (($superseded | Select-Object -First 4) -join '；') + $(if ($superseded.Count -gt 4) { "…（共 $($superseded.Count) 份）" } else { '' }) } else { '' }
        if ($pending.Count -eq 0) {
            # 内容全在而 tip 不是祖先，就是经重放（cherry-pick 或手工并）进的：--is-ancestor 对这类分支答 false，别拿它当替代判据。
            $lineage = if ($isAncestor) { 'tip 是 main 祖先' } else { 'tip 不是 main 祖先、经重放进入（--is-ancestor 判不出）' }
            "- ``$b`` @ $tip：ABSORBED · $lineage$rename$later"
        }
        else {
            $lineage = if ($isAncestor) { 'tip 是 main 祖先' } else { 'tip 不是 main 祖先' }
            "- ``$b`` @ $tip：NOT-ABSORBED · $lineage · " + (($pending | Select-Object -First 8) -join '；') + $(if ($pending.Count -gt 8) { "…（共 $($pending.Count) 份）" } else { '' }) + $later
        }
    }
    ""
}

if ($Audit) {
    "## 游离提交审计（不在任何 ref 上的提交，两级核：patch-id 对全部 ref → 逐文件 blob 对 main 历史）"
    ""
    $ids = @{}
    foreach ($l in (G log -p --no-merges --format='commit %H' --all --since=2026-08-01 | & git -C $repo patch-id --stable)) { $p = $l -split ' '; if ($p.Count -ge 2 -and -not $ids.ContainsKey($p[0])) { $ids[$p[0]] = $p[1] } }
    $un = @(& git -C $repo fsck --unreachable --no-reflogs --no-progress 2>$null | Where-Object { $_ -match 'unreachable commit (\w+)' } | ForEach-Object { $Matches[1] })
    $equiv = 0; $mdOnly = 0; $rest = @()
    foreach ($c in $un) {
        $pid = (& git -C $repo diff-tree -p --root $c 2>$null | & git -C $repo patch-id --stable)
        if (-not $pid) { $equiv++; continue }
        if ($ids.ContainsKey((($pid -split ' ')[0]))) { $equiv++; continue }
        $code = @(G diff-tree --no-commit-id --name-only -r $c | Where-Object { ($_ -notmatch '\.md$') -and ($book -notcontains $_) })
        if ($code.Count -eq 0) { $mdOnly++; continue }
        $nf = @()
        foreach ($f in $code) {
            $blob = & git -C $repo rev-parse -q --verify "${c}:${f}" 2>$null
            if (-not $blob) { continue }
            $found = $false
            foreach ($m in (G log main --format=%H -- $f)) { if ((& git -C $repo rev-parse -q --verify "${m}:${f}" 2>$null) -eq $blob) { $found = $true; break } }
            if (-not $found) { $nf += $f }
        }
        if ($nf.Count -eq 0) { $equiv++; continue }
        $rest += [pscustomobject]@{ sha = $c.Substring(0, 8); subj = (G log -1 --format='%ad %s' --date=format:'%m-%d' $c); files = ($nf -join '；') }
    }
    "- 游离提交 $($un.Count) 笔：等价/同 blob $equiv · 只动 .md/簿记 $mdOnly · **未对上 $($rest.Count)**"
    foreach ($x in ($rest | Sort-Object subj)) {
        $s = $x.subj; if ($s.Length -gt 100) { $s = $s.Substring(0, 100) + '…' }
        $f = $x.files; if ($f.Length -gt 160) { $f = $f.Substring(0, 160) + '…' }
        "  - $($x.sha) $s"; "      $f"
    }
    ""
}
