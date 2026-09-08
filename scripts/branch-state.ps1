# branch-state.ps1 —— 把「谁在分支上、什么没进 main」从 git 算出来，不靠会话自报、不靠台账。
#
# 用法（在仓库任一目录，PowerShell 5.1 / 7 均可）：
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1              # 默认：先 fetch，再出状态页
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1 -NoFetch     # 离线
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1 -Path internal/partycommercial   # 派单第 0 步：这块地盘上谁有半成品
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1 -Classify    # 给未加前缀的分支判 ABSORBED / NOT-ABSORBED（改名 merged/ 前跑）
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1 -Audit       # 游离提交审计（慢，几分钟）
#
# 约定（见 docs/agents/parallel-sessions.md「拆工作树」「派发前先点名」两节）：
#   merged/*  —— 内容已全进 main 的指针；salvage/* —— 只防丢、不集成；未加前缀 —— 在途或归用户。
#   本脚本只读，不改任何 ref、不动工作树。输出是 Markdown，可直接贴进完工报或 tasks.md。
param(
    [switch]$NoFetch,
    [string]$Path,
    [switch]$Classify,
    [switch]$Audit,
    [int]$SinceDays = 2
)
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = 'Stop'
$repo = (git rev-parse --show-toplevel 2>$null)
if (-not $repo) { Write-Error '不在 git 仓库里'; exit 2 }
$repo = $repo -replace '/', '\'
function G { param([Parameter(ValueFromRemainingArguments = $true)][string[]]$a) & git -C $repo @a 2>$null }

# 簿记文件：谁的笔在后谁的数字盖前面，比内容时不算它们（同 parallel-sessions「生成物不占号」）。
$book = @('docs/product/MECHANISM-INVENTORY.md', 'docs/adr/README.md', '.scratch/tasks.md', 'internal/architecture/production_wiring_baseline.txt')
$codeRe = '\.(go|sql|ts|tsx|js|mjs|sh|ps1|yml|yaml|py)$'

if (-not $NoFetch) { G fetch origin --prune | Out-Null }

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
$remoteHeads = @{}
foreach ($l in (G ls-remote --heads origin)) { $p = $l -split "`t"; $remoteHeads[($p[1] -replace '^refs/heads/', '')] = $p[0] }
$inflight = @(G for-each-ref --format='%(refname:short)' refs/heads | Where-Object { $_ -ne 'main' -and $_ -notmatch '^(merged|salvage)/' })
if ($inflight.Count -eq 0) { "- （无）" }
foreach ($b in $inflight) {
    $tip = G rev-parse --short $b
    $last = G log -1 --format='%ad' --date=format:'%m-%d %H:%M' $b
    $ahead = G rev-list --count "main..$b"
    $mb = G merge-base main $b
    $onlyHere = @()
    foreach ($f in (G diff --name-only --diff-filter=A $mb $b | Where-Object { $_ -match $codeRe })) {
        # 不用 cat-file -e：文件不在 main 时它往 stderr 写 fatal，在 $ErrorActionPreference = 'Stop' 下
        # 即使 2>$null 也会被 PowerShell 5.1 当成终止错误，整节中断、在途分支一行不印（09-08 MCP-5 实测）。
        # rev-parse -q --verify 同一问、不出声，与下方两处同一写法。
        $null = & git -C $repo rev-parse -q --verify "main:$f" 2>$null
        if ($LASTEXITCODE -ne 0) { $onlyHere += $f }
    }
    $pushed = if ($remoteHeads.ContainsKey($b)) { if ($remoteHeads[$b].StartsWith((G rev-parse $b))) { '已推 origin（同 SHA）' } else { "origin 落后（远端 $($remoteHeads[$b].Substring(0,8))）" } } else { '**未推 origin**' }
    "- ``$b`` @ $tip · 最后提交 $last · 领先 main $ahead 笔 · $pushed · main 上从未有过的代码文件 $($onlyHere.Count) 件"
    if ($onlyHere.Count -gt 0 -and $onlyHere.Count -le 12) { $onlyHere | ForEach-Object { "    - $_" } }
}
""

if ($Path) {
    "## 地盘 ``$Path`` 上在途分支里、main 尚无的提交（派单第 0 步；merged/ 与 salvage/ 不列，它们的去向已判）"
    ""
    $any = $false
    foreach ($b in $inflight) {
        $hits = @(G log "main..$b" "--since=$SinceDays days ago" --format='%h %ad %s' --date=format:'%m-%d %H:%M' -- $Path)
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

if ($Classify) {
    "## 分类（未加前缀的分支：分支改过的每份非簿记文件，其 tip blob 是否在 main 该文件历史里出现过）"
    ""
    foreach ($b in $inflight) {
        $mb = G merge-base main $b
        $files = @(G diff --name-only $mb $b | Where-Object { $book -notcontains $_ })
        $differ = @(); if ($files.Count -gt 0) { $differ = @(G diff --name-only $b main -- $files) }
        $pending = @()
        foreach ($f in $differ) {
            $bBlob = & git -C $repo rev-parse -q --verify "${b}:${f}" 2>$null
            if (-not $bBlob) { $pending += "$f（分支上已删）"; continue }
            $found = $false
            foreach ($c in (G log main --format=%H -- $f)) { if ((& git -C $repo rev-parse -q --verify "${c}:${f}" 2>$null) -eq $bBlob) { $found = $true; break } }
            if (-not $found) { $pending += $f }
        }
        if ($pending.Count -eq 0) { "- ``$b``：ABSORBED（可改名 merged/）" }
        else { "- ``$b``：NOT-ABSORBED，main 上没出现过的版本：" + (($pending | Select-Object -First 8) -join '；') + $(if ($pending.Count -gt 8) { "…（共 $($pending.Count)）" } else { '' }) }
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
