# owner-review-queue.ps1 —— 把「等 owner 拍板的事」从文档里算成一页：ADR 与票面里的越权风险点、needs-info 与带 Blocked by 的票。
#
# 用法：
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/owner-review-queue.ps1                      # 打到标准输出
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/owner-review-queue.ps1 -Out .scratch/owner-review-queue.md
#
# 它只读文档、不改文档。「越权风险点」是 agent 按 owner 授权代裁时的自报（见各 ADR / 票面），
# owner 不认可走 supersede，不改历史（docs/agents/parallel-sessions.md「决定权」）。
param([string]$Out)
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = 'Stop'
# $repo 保持 git 给的原样（Windows 上 C:/…，WSL 里 /home/…）：换成反斜杠的路径在 WSL 里喂不进
# `git -C` 与路径拼接。两种分隔符的差别只在 Rel 里一处抹平。
$repo = (git rev-parse --show-toplevel 2>$null)
if (-not $repo) { Write-Error '不在 git 仓库里'; exit 2 }
$root = ($repo -replace '\\', '/').TrimEnd('/') + '/'
function Rel([string]$full) {
    $p = $full -replace '\\', '/'
    if ($p.StartsWith($root, [System.StringComparison]::OrdinalIgnoreCase)) { $p.Substring($root.Length) } else { $p }
}
$sha = (git -C $repo rev-parse --short HEAD)
$lines = New-Object System.Collections.Generic.List[string]
function Add([string]$s) { $lines.Add($s) }

Add "# owner 复核队列 · 钉 ``$sha`` · $(Get-Date -Format 'yyyy-MM-dd HH:mm')"
Add ''
Add '由 `scripts/owner-review-queue.ps1` 生成；只有摘录，判断看原文。复核完一条：认可就在原文旁写一句「owner 复核 YYYY-MM-DD 认可」，不认可走 supersede。'
Add ''

# 一、ADR 里的越权风险点：取含「越权风险点」或「越权点」的行，连同其后到空行为止的列表项。只匹配「越权」会把 ADR-0027 / 0029 / 0055 里的领域词「越权探测 / 越权探针」也收进来（2026-09-09 实测三篇误收）。
Add '## 一、ADR 里的越权风险点'
Add ''
$adrs = Get-ChildItem (Join-Path $repo 'docs/adr') -Filter '0*.md' | Sort-Object Name
$n = 0
foreach ($f in $adrs) {
    $text = Get-Content $f.FullName -Encoding UTF8
    $idx = @(); for ($i = 0; $i -lt $text.Count; $i++) { if ($text[$i] -match '越权风险点|越权点') { $idx += $i } }
    if ($idx.Count -eq 0) { continue }
    $n++
    $reviewed = ($text | Where-Object { $_ -match 'owner 复核 \d{4}-\d{2}-\d{2} 认可' }).Count -gt 0
    Add ("### " + $f.Name + $(if ($reviewed) { '（已有 owner 复核记录）' } else { '' }))
    Add ''
    $printed = @{}
    foreach ($i in $idx) {
        for ($j = $i; $j -lt $text.Count -and $j -le $i + 12; $j++) {
            if ($j -gt $i -and $text[$j].Trim() -eq '') { break }
            if ($printed.ContainsKey($j)) { continue }
            $printed[$j] = $true
            $t = $text[$j]; if ($t.Length -gt 220) { $t = $t.Substring(0, 220) + '…' }
            Add ('> ' + $t)
        }
        Add ''
    }
}
Add "（共 $n 篇 ADR 含越权风险点）"
Add ''

# 二、票面里等人或等别处的票，两类各取：
#   needs-info —— Status 以 needs-info 起头（triage-labels 的状态角色）。
#   带 Blocked by —— 未了结的票（Status 属 needs-triage / needs-info / ready-for-agent / in-progress /
#   ready-for-human / draft）里 `Blocked by:` 一行不以「无」起头的。本仓的阻塞写法就是这一行
#   （AGENTS.md「红线」一节），triage-labels 没有 blocked 角色。resolved、wontfix 等已了结的票不入：
#   它们的 Blocked by 是历史，数以百计会淹掉这一节。Blocked by 里写着已 resolved 的票号时不替人判，
#   照列并带出原文，由看的人定。
# 票所在的目录：feature 下的 issues/，以及文档自洽那一组的 code/（头部格式相同）；目录名 / 与 \ 都认。
Add '## 二、needs-info 与带 Blocked by 的票'
Add ''
$issues = Get-ChildItem (Join-Path $repo '.scratch') -Recurse -Filter '*.md' | Where-Object { ($_.DirectoryName -replace '\\', '/') -match '/(issues|code)$' } | Sort-Object FullName
$openStates = 'needs-triage|needs-info|ready-for-agent|in-progress|ready-for-human|draft'
$m = 0
foreach ($f in $issues) {
    $text = @(Get-Content $f.FullName -Encoding UTF8)
    $status = $text | Where-Object { $_ -match '^Status:' } | Select-Object -First 1
    if (-not $status) { continue }
    $blocker = $text | Where-Object { $_ -match '^Blocked by:' } | Select-Object -First 1
    $hasBlocker = $blocker -and ($blocker -notmatch '^Blocked by:\s*无')
    $needsInfo = $status -match '^Status:\s*needs-info'
    $isBlocked = $hasBlocker -and ($status -match "^Status:\s*($openStates)")
    if ($needsInfo -or $isBlocked) {
        $m++
        $s = $status; if ($s.Length -gt 200) { $s = $s.Substring(0, 200) + '…' }
        Add "- ``$(Rel $f.FullName)``"; Add "  $s"
        if ($hasBlocker) { $b = $blocker; if ($b.Length -gt 200) { $b = $b.Substring(0, 200) + '…' }; Add "  $b" }
    }
}
Add ''; Add "（共 $m 张）"; Add ''

# 三、票面里的越权风险点（只列文件，判断看原文）。
Add '## 三、票面里提到越权风险点的票'
Add ''
$k = 0
foreach ($f in $issues) {
    $hits = @(Get-Content $f.FullName -Encoding UTF8 | Where-Object { $_ -match '越权风险点|越权点' })
    if ($hits.Count -eq 0) { continue }
    $k++
    $rel = Rel $f.FullName
    $first = $hits[0]; if ($first.Length -gt 160) { $first = $first.Substring(0, 160) + '…' }
    Add "- ``$rel``（$($hits.Count) 处）— $first"
}
Add ''; Add "（共 $k 张）"

$outText = ($lines -join "`n") + "`n"
if ($Out) {
    $path = if ([System.IO.Path]::IsPathRooted($Out)) { $Out } else { Join-Path (Get-Location) $Out }
    [System.IO.File]::WriteAllText($path, $outText, (New-Object System.Text.UTF8Encoding $false))
    "written: $path"
} else { $outText }
