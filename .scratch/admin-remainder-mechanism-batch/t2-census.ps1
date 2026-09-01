# T2 棘轮普查取数脚本（口径全取 production-wiring-ratchet-gate/census-d5e5d20.md「取证方法」节）
#
# 为什么把命令固化成脚本而不是贴在报告里：基线自记两次口径错都出在「命令看着一样、
# 实际问的不是同一个量」（PowerShell 大小写默认、命中数<=1 判零）。脚本让重跑者跑的
# 是同一段字节，差异只可能出在 SHA 上。
#
#   pwsh -File .scratch/admin-remainder-mechanism-batch/t2-census.ps1 -Sha 1665fdb
#
# 输出：四族各自的「声明数 / 唯一名字数 / 零非测试调用点名单（附测试调用数）」。
# 四族不加总——它们的「接线」含义不同，见基线「汇总（只是表尾，不是结论）」。

param(
    [Parameter(Mandatory = $true)][string]$Sha,
    [switch]$ListNames
)

$ErrorActionPreference = 'Stop'

# 调用点范围：internal + cmd 的非测试代码。声明扣除范围另含 migrations——
# 基线索引初版漏掉 migrations/migrations.go 的 13 条，更正后才是完整参照系。
$CallPaths = @('internal', 'cmd', ':(exclude)internal/**/*_test.go', ':(exclude)cmd/**/*_test.go')
$TestPaths = @('internal/**/*_test.go', 'cmd/**/*_test.go')
$DeclPaths = @('internal', 'cmd', 'migrations', ':(exclude)internal/**/*_test.go', ':(exclude)cmd/**/*_test.go')

function Invoke-GitGrepMatches {
    param([string]$Sha, [string]$Pattern, [string[]]$Paths)
    # -o 让 git 自己数自己匹配：源文件不经 PowerShell 读取，绕开大小写与编码两类工具默认。
    $out = & git grep -h -o -E $Pattern $Sha -- @Paths 2>$null
    if ($null -eq $out) { return @() }
    return @($out)
}

function Get-DeclaredNames {
    param([string]$Sha, [string]$Pattern, [string[]]$Paths)
    # 返回声明行原文，调用方自行抽名字。
    return Invoke-GitGrepMatches -Sha $Sha -Pattern $Pattern -Paths $Paths
}

function Measure-NameHits {
    param([string]$Sha, [string[]]$Names, [string[]]$Paths, [string]$Kind)
    # $Kind = 'call' 取 \b名字\( ；'decl' 取 ^func 名字( 与泛型格 ^func 名字[ 。
    # 名字集大时切块，避免单条正则过长。
    $tally = @{}
    foreach ($n in $Names) { $tally[$n] = 0 }
    for ($i = 0; $i -lt $Names.Count; $i += 60) {
        $chunk = $Names[$i..([Math]::Min($i + 59, $Names.Count - 1))]
        $alt = ($chunk -join '|')
        if ($Kind -eq 'call') {
            $pat = '\b(' + $alt + ')\('
        }
        else {
            $pat = '^func (' + $alt + ')(\(|\[)'
        }
        foreach ($m in (Invoke-GitGrepMatches -Sha $Sha -Pattern $pat -Paths $Paths)) {
            $name = ($m -replace '^func ', '') -replace '(\(|\[)$', ''
            if ($tally.ContainsKey($name)) { $tally[$name] = $tally[$name] + 1 }
        }
    }
    return $tally
}

$script:ZeroSets = @{}

function Show-Family {
    param([string]$Label, [string[]]$Names, [int]$DeclLines)

    $uniq = @($Names | Sort-Object -Unique)
    # 判零三步：非测试命中 - 全部声明行 = 0 才算零。
    # 不用「命中数 <= 1」——名字跨包可重，第二处声明会被读成一次调用（基线第二次口径错）。
    $calls = Measure-NameHits -Sha $Sha -Names $uniq -Paths $CallPaths -Kind 'call'
    $decls = Measure-NameHits -Sha $Sha -Names $uniq -Paths $DeclPaths -Kind 'decl'
    $tests = Measure-NameHits -Sha $Sha -Names $uniq -Paths $TestPaths -Kind 'call'

    $zero = @()
    foreach ($n in $uniq) {
        $real = $calls[$n] - $decls[$n]
        if ($real -le 0) { $zero += [pscustomobject]@{ Name = $n; Test = $tests[$n]; Decl = $decls[$n] } }
    }

    "{0} : 声明行 {1} / 唯一名字 {2} / 零非测试调用点 {3}" -f $Label, $DeclLines, $uniq.Count, $zero.Count
    # 每行两个计数是有意的：零/零 = 全仓无人调（死代码，该删），零/非零 = 只被测试接线
    # （棘轮要棘的那一种）。两者在只有一个计数时长着同一张脸。
    $dead = @($zero | Where-Object { $_.Test -eq 0 })
    "         其中 测试也为 0（疑死代码）{0} 个：{1}" -f $dead.Count, (($dead.Name | Sort-Object) -join '、')
    if ($ListNames) {
        foreach ($z in ($zero | Sort-Object Name)) { "         - {0}（测试 {1}，声明 {2}）" -f $z.Name, $z.Test, $z.Decl }
    }
    # 结果存进脚本域哈希表而不是 return——函数里每个未捕获的字符串都会并进返回值，
    # 一旦调用方写 `$z = Show-Family ...`，上面那几行报告就再也不出现在标准输出里。
    $script:ZeroSets[$Label] = $zero
}

"=== T2 棘轮普查 @ $Sha ==="

# 一族：outbox 交接口。前缀 NewOutbox + 后缀 Handoff。
$f1Lines = Get-DeclaredNames -Sha $Sha -Pattern '^func NewOutbox[A-Za-z0-9_]*Handoff\(' -Paths @('internal', ':(exclude)internal/**/*_test.go')
$f1 = $f1Lines | ForEach-Object { ($_ -replace '^func ', '') -replace '\($', '' }
Show-Family -Label '一族 outbox 交接口 NewOutbox*Handoff' -Names $f1 -DeclLines $f1Lines.Count

# 二族：应用层命令处理器。
$f2Lines = Get-DeclaredNames -Sha $Sha -Pattern '^func New[A-Za-z0-9_]*Handler\(' -Paths @('internal/*/application/*.go', ':(exclude)internal/**/*_test.go')
$f2 = $f2Lines | ForEach-Object { ($_ -replace '^func ', '') -replace '\($', '' }
Show-Family -Label '二族 应用层处理器 New*Handler' -Names $f2 -DeclLines $f2Lines.Count

# 三族：领域工厂，十二前缀。前缀集是判断不是穷举——基线「⚠ 这个 13 是下界」两节说明盲区。
$p3 = '^func (Form|Establish|Fix|Judge|Grant|Cut|Publish|Accept|Propose|Verify|Open|Record)[A-Za-z0-9_]*\('
$f3Lines = Get-DeclaredNames -Sha $Sha -Pattern $p3 -Paths @('internal/*/domain/*.go', ':(exclude)internal/**/*_test.go')
$f3 = $f3Lines | ForEach-Object { ($_ -replace '^func ', '') -replace '\($', '' }
Show-Family -Label '三族 领域工厂（十二前缀）' -Names $f3 -DeclLines $f3Lines.Count

# 四族：ports 适配器，按编译期接口断言认文件，不按命名认。
# 断言模式要求 `= `：宽模式 ^var _ .*ports\. 会把反方向断言（本地窄接口 = ports 类型）算进来。
$adapterPaths = @('internal/*/adapters/*.go', 'internal/*/adapters/*/*.go', 'internal/*/adapters/*/*/*.go', ':(exclude)internal/**/*_test.go')
$assertFiles = @(& git grep -l -E '^var _ [A-Za-z0-9_]*ports\.[A-Za-z0-9_]+ = ' $Sha -- @adapterPaths) |
    ForEach-Object { $_ -replace ('^' + [regex]::Escape($Sha) + ':'), '' }
$f4Lines = @()
if ($assertFiles.Count -gt 0) {
    $f4Lines = Get-DeclaredNames -Sha $Sha -Pattern '^func New[A-Za-z0-9_]*\(' -Paths $assertFiles
}
$f4 = $f4Lines | ForEach-Object { ($_ -replace '^func ', '') -replace '\($', '' } | Where-Object { $_ -notlike 'NewOutbox*' }
"四族 断言文件（非测试）: {0}" -f $assertFiles.Count
Show-Family -Label '四族 ports 适配器 New*（按断言认）' -Names $f4 -DeclLines @($f4).Count
