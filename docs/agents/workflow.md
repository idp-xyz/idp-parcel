# 技能流程在本仓的落法

[idp-skills](https://github.com/idp-xyz/idp-skills) 的通用流程（`/which-skill` 是权威路由）假设一个从零开始的仓库。本仓不是——产品主线、限界上下文、用例和工作包都已经存在。本文只写**这个落差**：哪些流程步骤本仓已经用文档做过了，一个 PN 切片实际怎么走，以及红线在流程的哪一步生效。

技能本身怎么用不在这里，见各 `SKILL.md` 与 `docs/<bucket>/<name>.md`；本仓的技能路由表在 [AGENTS.md](../../AGENTS.md)。

## 记号

本文用到的编号都在别处权威定义，这张表只给一句话和入口，不展开成第二套定义。

| 记号 | 是什么 | 权威定义 |
|---|---|---|
| `PN-01`..`PN-08` | 首发纵向开发切片编号 | [首发开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#首发纵向开发切片) |
| `W01`..`W09` | 一个切片内的工作包编号；全称带切片前缀，如 `PN03-W01`、`CC-S0-W01`、`S02-W01` | 各 `docs/design/*handoff*` 的「取证与开发工作包」 |
| `P` `R` `S` `N/A` | 证据层级：真实生产、历史回放、受控模拟、本期不适用 | [验收矩阵](../product/PILOT-ACCEPTANCE-MATRIX.md#证据层级) |
| `UC-*` | 应用用例，形如 `UC-PS-001` | [应用用例编写约定](../application/README.md#编写约定) |
| `BD-*` | 未确认的业务选择，形如 `BD-PS-001` | 同上 |

## 本仓已经做过的上游步骤

通用流程里有三步在本仓**已经有产物**。对着已有产物再跑一遍，产出的是第二套口径，违反红线「单一权威」。

| 上游步骤 | 本仓的等价产物 | 因此 |
|---|---|---|
| `/wayfinder` 铺决策地图 | [首发开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md) 的 PN-01..08 切片表 | 地图已成型，不重铺。只有出现基线未覆盖的新方向时才考虑 |
| `/to-tickets` 拆带阻塞边的工单 | `docs/design/*handoff*` 的 `W01..W09` 工作包，阻塞关系写在交接文档里 | 直接认领 `W` 包，不重拆 |
| `/setup-idp-skills` 配 tracker 与布局 | [issue-tracker.md](./issue-tracker.md)、[triage-labels.md](./triage-labels.md)、[domain.md](./domain.md) | 前置已满足，不用跑 |

`/grill-with-docs` 与 `/domain-modeling` 仍然常用，但在本仓是**演进**而非创建：九个 `CONTEXT.md` 与 [ADR 索引](../adr/README.md)里已成文的全部记录都已经存在。这里不复述 ADR 份数——每新增一条就会让它过期，此前写死的「十一份」正是这样烂掉的，而现行与已被取代的划分只在那份索引里权威。改动走 [AGENTS.md 的「改文档」](../../AGENTS.md#改文档)——ADR 只新增或 supersede，不改写已接受的历史。

`.scratch/` 留给**交接文档没覆盖**的工作：外来 bug、临时需求、基线之外的探索。已经是 `W` 包的东西不进 tracker，也不要 `/triage`。

## 一个 PN 切片怎么走

```mermaid
flowchart TD
    slice["读 PN 切片表<br/>确认切片编号与主责上下文"] --> pack["读该 PN 的 docs/design/*handoff*<br/>认领一个 W 包"]
    pack --> confirmed{"W 包的参数<br/>在登记册里是<br/>「已确认」吗？"}
    confirmed -- 否 --> skeleton["只做稳定骨架 + 显式未配置分支<br/>或隔离合成 S 验证"]
    confirmed -- 是 --> real["按真实参数实现"]
    skeleton --> implement["/implement<br/>内驱 /tdd 红绿切片"]
    real --> implement
    implement --> review["/code-review 双轴<br/>Standards + Spec"]
    review -- Spec 轴对照 --> usecase["对应的 UC-* 文档"]
    review --> commit["提交；证据层级如实记录"]
```

第一步的判据在 [AGENTS.md 的「开工顺序」](../../AGENTS.md#开工顺序)，这里不复述。

**参数是否已确认**只看[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)。登记册说「待提供」就是待提供——不用技术默认值补齐，也不因为合成数据跑通了就改状态。

**`/code-review` 的 Spec 轴**对照的是 `UC-*` 用例文档，不是工单描述。用例的输入、结果、失败边界就是验收口径。

## 红线在哪一步生效

四条红线不是审查清单，是流程里的具体动作。

**证据层级诚实**——在提交那一步。三件事最容易做错：

- 隔离环境跑出来的一律是 `S`，重放结果与真实完全一致也不能升级为 `R`
- 影子运行不是第五种层级，按其实际数据来源记为 `R` 或 `S`；影子通过本身不构成 `P`
- `/prototype` 的产出**最高只能是 `S`**，而且按技能本身的规矩，原型代码不进生产实现——它只提供证据，赢的设计交给 `/tdd` 重写

**只实现已确认规则**——在设计那一步。未确认参数与 `BD-*` 保持可配置或显式未决分支。

**所有权清晰**——在分层那一步。变更落在正确的 `internal/<context>/` 下；领域包不依赖 HTTP 或 `pgx`。跨上下文只传递自己拥有的事实、判断或授权引用，接收方形成自己的结果。

**单一权威**——在写文档那一步。用例与交接只引用，不复制第二套口径。本文自己也守这条：凡是别处有权威的都用链接。

## 本机环境

这些查不到，踩过才知道。分两节是因为其中一半只在当前宿主成立：**换宿主时整节删掉下面的「当前宿主」，不要逐条判断**；「与宿主无关」那节继续有效。

### 当前宿主：Windows + WSL

- `~/.cursor/skills` 下 37 个条目是**指向 `D:\tops\idp-skills` 克隆的目录联接**（Windows 无管理员权限，用不了符号链接）。改技能要去那个克隆改并推回上游，就地编辑等于改上游工作区。更新用 `git pull`。**Cursor 的 `Glob` 工具不下降进这层目录联接，且报空不报错**——在这个根下探 `implement/SKILL.md` 得到的是「0 files found」而不是「进不去」，极易读成「没装」；父目录本身是普通目录，`Get-ChildItem -Force` 列得出全部条目，同一个 Glob 在真实目录上也是好的，坏的只有「联接那一格」。判断某个技能装没装一律用**绝对路径 Read**（或 `Test-Path`），不靠 Glob。这个假阴性会与「与宿主无关」一节 `disable-model-invocation` 那条叠加——列表里看不见、Glob 又搜不到时，只有绝对路径 Read 能证明技能在。
- **`go test -race` 在 Windows 侧跑不了，走 WSL。** 竞态检测器是 LLVM ThreadSanitizer 这个 C++ 运行时的 Go 封装，链接它必须过 cgo；本机 `CGO_ENABLED=0`、`CC` 默认为 `gcc` 而 PATH 上并没有任何 C 编译器（Windows 上 Go 不附带），于是报 `-race requires cgo`。这不是缺口，`Ubuntu-24.04` 里有 `/usr/bin/gcc`，Go 装在 `/usr/local/go/bin` 且版本与 `go.mod` 一致——注意它不在非登录 shell 的 PATH 上，要写绝对路径：`wsl -d Ubuntu-24.04 -- sh -c "cd /mnt/d/tops/idp-parcel && CGO_ENABLED=1 /usr/local/go/bin/go test -race ./..."`。CI 跑的就是这一步，本地能自证就不要只标注「由 CI 覆盖」。另一条路是装 MinGW-w64 让 `gcc` 上 PATH。宿主换成 Linux 后本条整条作废：那里 `gcc` 通常就在，`go test -race ./...` 直接可用。
- **WSL 侧的模块缓存是空的，而 WSL 出不了网——上一条那行命令照抄会红一整批包，那不是代码坏了。** 凡是导入 `idp-bento-go` 的包在 setup 阶段报 `dial tcp … i/o timeout`，而同一刻 Windows 侧 `go build ./...` 是绿的，这个反差就是识别标志。宿主的代理只绑 `127.0.0.1`，WSL 够不到；但 Windows 那份模块缓存东西是全的，让 WSL 直接把它当文件代理读即可——在 WSL 里跑一次 `go env -w GOPROXY='file:///mnt/c/Users/topsx/go/pkg/mod/cache/download,https://proxy.golang.org,direct'`，写进 `~/.config/go/env` 后长期有效，`go.mod` 与 `go.sum` 不受影响。网络链留在末尾作兜底：Windows 缓存也缺的时候退回原来的行为，不会更差。**配好之后它仍然不跑 PostgreSQL 用例**——门禁容器绑的是 Windows 回环 `127.0.0.1:55432`，WSL 够不到，所以本机的竞态与 PG 是两半分开覆盖，两样同时成立只有 CI 有；报构建状态时别把两次跑说成一次。
- **GitHub 只能走代理。** Clash Verge 在 `127.0.0.1:7897`，但系统代理开关常是关的，导致 git 直连失败——单次连接尝试约 21 秒超时，但 GitHub 有多个解析地址，git 逐个重试，整条命令实测约 5 分钟才报错，看着像卡死。`idp-skills` 与 `idp-parcel` 两个克隆都已设仓库级 `http.proxy`；新克隆需要自己加 `-c http.proxy=http://127.0.0.1:7897`。
- **`idp-parcel` 是私有仓，远程操作必过 Git Credential Manager**（凭据存在 Windows 凭据管理器的 `git:https://github.com`）。GCM 里存了两个 GitHub 账号（`idpxyz`、`idp-repo`），而远端 URL 不带用户名、`credential.*` 本地全局都没配，于是它**每次 fetch/push 都弹「Select an account」等人点**——实测 `git fetch` 因此要 150 到 190 秒，而连接本身只要几秒，极易误判成网络慢或命令卡死。钉住账号即可消除：`git config --local credential.https://github.com.username idpxyz`；**别加 `--global`**，另一个账号在别的仓库还在用，指错会连那些仓库一起坏。等价做法是把用户名写进远端 URL（`https://idpxyz@github.com/...`），代价是它会出现在 `git remote -v` 里。另有一种偶发的不返回，与账号选择无关：GCM 要弹交互提示，而非交互 shell 没有 `/dev/tty`，于是静默三分钟后才报 `could not read Username`。这时凭据好好存着、代理也通，别去动那两样——查进程，有 `git-credential-manager` 挂着就杀掉重跑。加 `GIT_TERMINAL_PROMPT=0` 能让它立刻失败并露出真实原因，不必先干等。推送本身走代理实测 5 秒到 90 秒都有，别按固定时长判断卡没卡——要判断就查 `git-credential-manager` 进程在不在。
- **`gh` 装不了 winget，认证也不能照搬 git 那份凭据。** winget 直连 `github.com` 取 MSI，不吃仓库级 `http.proxy`，实测卡满两分半毫无进展；经代理拉 zip 只要九秒，`Invoke-WebRequest -Uri "https://github.com/cli/cli/releases/download/v<ver>/gh_<ver>_windows_amd64.zip" -OutFile "$env:TEMP\gh.zip" -Proxy "http://127.0.0.1:7897" -UseBasicParsing`，解压到 `%LOCALAPPDATA%\Programs\gh` 再把其中 `bin` 加进用户 PATH，全程不需要管理员。认证走 `gh auth login --hostname github.com --web --scopes "repo,read:org,workflow"`，授权后 token 进系统密钥环，且实测**不动全局 git 凭据配置**——`git config --global --get-regexp '^credential'` 仍为空，别的仓库不受影响。设备码换 token 那一步会偶发 `unexpected EOF`：浏览器那半边其实已经授权成功，是代理掐了 gh 的 POST，重跑一次即可，实测第二次就过，别据此判定这条路不通。**不要拿 GCM 里那份 token 顶替**：它缺 `read:org`，`gh auth login --with-token` 正是以这条理由拒绝；`GH_TOKEN` 不做这道校验因而拿它能读 CI，但那是没登录时的应急，不是常态。`gh` 出网同样要 `HTTPS_PROXY=http://127.0.0.1:7897`，与上一条 git 用的是同一个代理。
- **git 写 stderr，PowerShell 把 stderr 渲染成红色报错。** `To https://github.com/...` 这类进度信息会显示成 `NativeCommandError`，看着像失败。判断成败只看 `$LASTEXITCODE`。
- **不要用 `Set-Content` 改源文件。** 它默认不是 UTF-8，会把中文注释和破折号写成乱码，`go build` 报 `illegal UTF-8 encoding`；`-Encoding utf8` 在 Windows PowerShell 5.1 又会写入 BOM。需要脚本化批量替换时用 `[System.IO.File]::WriteAllText($path, $text, (New-Object System.Text.UTF8Encoding $false))`。
- **写多行提交消息用 `git commit -F` 加**单引号**here-string，双引号那种会吃掉反引号。** `@" … "@` 是可扩展 here-string，反引号在里面是转义符：`` `resolutionOrder `` 里的 `` `r `` 被当成回车写进消息，整行从此断在那里；`` `CommercialBasisQuery `` 之类没撞上转义名的只是丢掉两个反引号。而本仓的提交消息几乎必然引到代码符号，反引号是默认写法。用 `@' … '@`（单引号 here-string，字面量、不做任何替换），再 `[System.IO.File]::WriteAllText($path, $msg, (New-Object System.Text.UTF8Encoding $false))` 写盘、`git commit -F $path`。**别用 `git commit -m` 写多行中文**：换行处会另有一处损坏。识别标志是消息里出现半行截断或凭空多出的换行，而 `git commit` 本身退 0。
- **读中文源文件同样要显式指定 UTF-8，否则按行统计会静默偏小。** `Get-Content -Raw` 和走 PowerShell 管道的 `git show` 都不假定 UTF-8，无 BOM 时退回 ANSI 代码页（本机 GBK）；中文注释的 UTF-8 字节按 GBK 解会剩下一个落单的前导字节，它把紧跟的换行当成自己的后继字节一并吃掉，于是 `// …守的形状。` 与下一行的 `func TestXxx` 并成一行。后果不是乱码报错，而是**行首锚点失配**——`(?m)^func Test` 在导入 `pgtest` 的 141 个用例文件上实测数出 455 条，显式 UTF-8 读同一批是 806 条，少掉 351 条，全程零报错。识别标志是**同一份计数换个读法就变大**，以及子集反超全集（同一次统计里全仓 772 条竟小于该子集的 806 条）。按行首匹配或按行计数读中文源码时用 `[System.IO.File]::ReadAllText($path, (New-Object System.Text.UTF8Encoding $false))`；非要走 `git show` 管道就先设 `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8`。与上一条是同一族问题的读写两半，上一条至少会红，这一条不会。**拿它做「零命中即干净」那类自查时尤其危险**：偏小时人还可能觉得数对不上，偏到零时，**零命中恰恰就是自查想要的结果**。实测有人用中文模式 `Select-String` 自查「我有没有写过计数」，零命中，差一点据此报「我干净」。
- **`git worktree remove` 的退出码两个方向都不可信，收尾动作因此不能按它写。** 本机实测到过它报 `Permission denied` 退 **255**，而它其实**已经把工作树内容删光、登记也从 `.git/worktrees` 摘掉了**，只剩最后一层空目录没删成；照退出码判会读成「拆失败」，于是去重试、或者加 `--force`——而 `--force` 在别的场合会连真正未提交的改动一起丢。反方向同样不成立：退 0 只说明命令自认为成功。它还是**间歇**的而不是 Windows 必发，同一天里有会话连拆五处全部退 0、零残留，所以别写成「Windows 上必然如此」，那会让下一个人看到退 0 反而怀疑自己。可靠的收尾是四步各自查自己那一格：`git worktree remove <path>`（**永不加 `--force`**）→ `git worktree prune` → `Test-Path <path>`，有壳才 `Remove-Item -Recurse -Force` → `git worktree list` 复核登记已摘。
- **数命令输出的行数时让 git 自己数，别用 `@(...).Count`。** 上一条那个吃换行的毛病同样打在**自验时用来数东西的方法**上，而这一层最难察觉：结果不是报错，是一个小一点的数。实测同一段区间——`@(git log --oneline A..HEAD).Count` 给 **1**，先设 `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8` 再跑同一句给 **2**，`git rev-list --count A..HEAD` 给 **2**（中文提交消息末尾那个字吃掉了换行，两行并成一行）；同一份 diff 默认管道数出 22 行、设 UTF-8 后 34 行、`git diff --numstat` 是 18 增 1 删。**用 `git rev-list --count`、`git diff --numstat` 这类由 git 自己出数的写法**，根本不经 PowerShell 解码——比「记得先设 `OutputEncoding`」那条纪律硬，因为它不依赖谁记得。**但换成 git 自己数只挡住了解码那一半，传参这一半仍在人手里**，而且只有一种写法会坏——别因此一律改写。同一时刻同一仓，正确答案 3：

```powershell
git rev-list --count (git rev-parse HEAD~3)..HEAD     # → 0   错
$r = (git rev-parse HEAD~3).Trim()
git rev-list --count "$r..HEAD"                       # → 3   对
git rev-list --count origin/main~3..HEAD              # → 3   对，裸字面量本来就没事
```

坏的那一种是因为 **PowerShell 不把紧贴在括号表达式后面的字面量并成一个参数**：`cmd /c echo (git rev-parse HEAD~3)..HEAD` 打出来的是 `<sha> ..HEAD`——**两个**参数，git 照单收下算出 `0` 且不报错。**裸字面量与「先赋值再拼带引号字符串」都安全**，不必见到括号就先赋值。两半的失效形状一模一样：给一个小一点的数，不报错。
- **但别据此把凡沾 PowerShell 的旧证据一概作废，两类不受影响。** 一是读 `$LASTEXITCODE` 的判定，根本不过文本。二是**不带行首锚的 ASCII 子串存在性判定**：错误解码把多行并成少行，却**不能把一个存在的 ASCII 子串弄成不存在**——所以拿 `--- FAIL`、`--- SKIP` 这类裸子串数出来的**零结论仍然成立**。**这道免疫有两个前提，缺一不可，而两个前提失效时都倒向「零命中」。** 一是**不带锚**：一旦写成 `^\s*--- SKIP`，并行就能把匹得上的变成匹不上。二是**那根针必须是 ASCII**：按 GBK 解 UTF-8 会把每个汉字拆成几个乱码单字节，**中文针于是真的匹不上一个确实存在的串**。实测同一份 `git grep` 输出（109409 字节、114 个真换行、默认读成 93 行、`-Encoding UTF8` 读成 114 行且逐字正确）：ASCII 针 `AT-CC-246` 两种读法都命中，中文针「超过 150 LB 不予承运」**默认 `False`、`-Encoding UTF8` `True`**，而它就在文件里。**而本仓几乎没有不含中文的文件**——实测于 `fecda37`，用 `git grep -P '\p{Han}'`（**不要用 `[一-龥]` 这类字符区间，git 会把它当字节集，实测它与 `[—→─]` 命中同一批文件，问的其实是「有没有非 ASCII」**）：`docs/` 下 201 份 `.md` 中 **198 份含中文**，不含的三份是 `agents/` 下的 `domain.md`、`issue-tracker.md`、`triage-labels.md`（正文英文，非 ASCII 只有破折号、箭头与制表符，字节里一个 `0xE4`–`0xE9` 前导都没有）；`internal/` 下 890 份 `.go` 有 870 份含。**所以这道免疫在这里基本用不上：拿中文子串得出的零命中什么都不证明。而那三份恰好是 agent 最常去 grep 的几份，在它们上面免疫真的成立**——这三份点名而不抹去，就是为了让下一个人知道自己此刻站在哪一侧。 判自己有没有这道免疫，看的是当初那条命令写没写 `^`、那根针是不是 ASCII，不是看它在数什么。受影响的只有**总数**类断言（如 `PASS` 的条数），它们是下界，可能偏小。分清这两者要紧：一次含 PG 的全量验证里，作准的恰好是退出码与两个零，而总数本来就不作信号。
- **工具的默认值不出声，而它替你选的那一下只改变一个数。** 上面两条各是一例，第三例是 `Select-String` 的模式匹配**默认不区分大小写**：一份按未导出函数名做的普查里，同一模式加不加大小写敏感实测差出四个文件，十七处差异全是工具口径而非期间新增代码（MCP-2 于普查更正中实测，普查锚 `d5e5d20`，在其与 `3ec0b1e` 两个提交上各跑一遍才分离出这两个原因）。**三例的方向还各不相同**——吃换行→偏小、传参被拆成两个→变零、大小写→偏大，所以防不住它的不是粗心，是「数会偏小」这类方向直觉本身不成立。共通的只有一件：**不报错，只是数字不同**。总则因此不列工具清单（清单会过期），只留一句：**拿一个带默认行为的工具去量一个要当基线的数时，先问它默认了什么。** 「要当基线」是分量所在：一次性的数错了下次自会发现，而被吸收进基线的数错了，那道门禁永远不会红。
- **验这类工具要靠探针，而探针要变的是被搜的内容，不是搜的模式。** 换个模式再搜一次只能告诉你匹配器活着，不能告诉你你问对了问题——`PASS` 数出四千证明不了 `--- SKIP` 那串字面量写对了、也证明不了解码路径传得过来它。正确形状是同一串模式打在一处**已知含目标**的内容上，一正一反各一次。实测于 `f6ed413`，`internal/pilotgovernance/adapters/postgres`（另有会话在同一 SHA 上独立跑出同一组，除秒数外同数）：

```powershell
# DSN 未设： exit=0   --- SKIP 23   --- PASS  0   ok … 0.012s
# DSN 已设： exit=0   --- SKIP  0   --- PASS 32   ok … 4.385s
```

反向那次真的数出了 23，字面量与解码路径才算都验过。顺带注意两行的**退出码都是 0、包行都是 `ok`**，差别只在秒数与那两个计数——这正是「PostgreSQL 集成用例默认不跑而包照样显示 `ok`」那条的现场，秒表只配起疑，定性仍要看 `-v` 下的字样。
- 开发机 `idp-110-dev`（`/workspace/idp/`）上技能装在 `~/.claude/skills` 与 `~/.agents/skills`，是 `scripts/link-skills.sh` 建的符号链接，与本机布局不同。

**换宿主时要重新确认的，不是照抄新路径就完事：** 竞态检测能否直接跑（有无 C 工具链）、GitHub 出网方式与凭据存放、shell 如何渲染 stderr 与写文件编码、技能装在哪几个根。工具链版本不在此列——`go.mod` 锁的 `go1.26.5` 与 CI 的 `ubuntu-24.04` 不随宿主变化。

### 与宿主无关

- `~/.cursor/skills` 那 37 个里有 22 个在 `SKILL.md` 前置声明了 `disable-model-invocation: true`，**只在你显式打 `/name` 时才跑，agent 不会自主拾取**，因此不出现在 agent 的可用技能列表里。上方路由表提到的 `/which-skill`、`/ubiquitous-language`、`/implement`、`/triage`、`/to-spec`、`/to-tickets`、`/wayfinder`、`/handoff`、`/grill-with-docs`、`/improve-codebase-architecture`、`/wizard` 都属这一类：装了、能用、但看不见。看不见不等于没装，别去重装。
- **Cursor 同时扫三个技能根**：`~/.cursor/skills-cursor`（Cursor 自带）、`~/.cursor/skills`（上面那 37 个）、`~/.codex/skills`（Codex CLI 的目录，跨工具互通一并读取）。同名技能存在于多个根时去重会挑中 `.codex` 那份，而本机 `~/.codex/skills` 磁盘上只剩六个 `.system` 技能，`domain-modeling`、`grilling`、`grill-with-docs` 的 `.codex` 副本早已删除。结果是 **agent 拿到的技能路径指向不存在的文件，读取失败**，容易误判成「这个技能没装」。两份副本内容还可能不同，`.codex` 那份是旧版。读不到时去 `~/.cursor/skills/<name>/SKILL.md` 找同名的；新开会话通常也能让路径指回 `.cursor`。
- 技能变更**要新开会话才加载**。当前会话的技能列表是会话开始时的快照。
- **PostgreSQL 集成用例默认不跑，而那个包照样显示 `ok`。** `internal/platform/migrate` 的用例要真实 PostgreSQL 16，未设 `IDP_PARCEL_POSTGRES_DSN` 时它们 `t.Skip`；而包级默认输出里，全部跳过与全部通过同样只是一行 `ok`，只有 `-v` 才看得见 `SKIP`。所以本机默认状态下的「全仓 go test 绿」对这些证据是**零覆盖**，不是少盖一块；包数同理，只说明没红，说明不了验了多少。**报构建状态时写明含不含 PG**——「绿（含 PG）」与「绿（未设 DSN，PG 用例跳过）」是两个强度不同的断言，别让读的人以为证据已被重现。要真跑：`docker compose up -d`（仓库根 `compose.yaml`，一次性库，镜像钉 `postgres:16.14`），再 `$env:IDP_PARCEL_POSTGRES_DSN = "postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable"`；端口错开 55432 是因为宿主机 5432 已被占。CI 里缺 DSN 是 `t.Fatal` 而不是跳过，所以门禁不会因为某次 workflow 被改坏而静默蒸发。
- **`gofmt -l` 列出文件时退出码仍是 0。** 它把待格式化的文件名打到 stdout，不拿退出码表达「有活要干」——实测在只有一个未格式化文件的目录上跑，列出了该文件而 `$LASTEXITCODE` 为 0。所以自验若写成 `gofmt -l ./...` 再判退出码，**永远抓不到任何东西**；要判就判有没有输出。这条要紧是因为 `gofmt` 是 BOM 的唯一哨兵：带 BOM 的 `.go` 文件实测 `go build` 退 0、`go vet` 也退 0，只有 `gofmt -l` 点名，而它自己又不用退出码说话——两处都按退出码判的话，BOM 可以一路穿过本地自验。
- **量法自己内嵌的假设同样不出声，而它比工具默认值难防一格：换个工具换不掉。** 与「当前宿主」一节「工具的默认值不出声」那条**不同属**——这里没有任何默认值参与，错的是**你交给工具去数的那个问题里预设了一句恒为真的话**。实测于 `305d372`（普查 `.scratch/production-wiring-ratchet-gate/census-d5e5d20.md`，基线 `d5e5d20`）：判「某构造函数有没有非测试调用点」时用的是「全仓命中数 ≤ 1 即零调用点，那一次命中是声明本身」，而 **Go 的构造函数名跨包可重**，另一个包里的 `func` 声明于是被读成一次调用，三个真正的零调用点被判成已接线。改法是按**包 + 名**定位并扣除该名的**全部**声明——**名字不是唯一键**。
  **单列而不并进上一条，差别落在处方上。** 那一属（吃换行、传参被拆、大小写）的处方都是同一帖「让 git 自己匹配、自己数」，**而这个错正是照那帖药吃了之后犯的**——出错那份普查自己写着数字均以 git 侧匹配、大小写敏感得出。**换工具治不了坏的不是量具的东西。** 问法因此也不同：对工具问「它默认了什么」，对量法问「**我这个判法预设了什么恒为真**」。
  分属有实测依据而不只是分类直觉：同基线把非测试 `.go` 的顶层 `func` 声明（**含 `func Name[T any](` 那一格**）建成索引、四族名单逐一对查，**只有第四族存在跨包重名，另三族一个都没有**；取证与索引口径的两次更正都记在那份普查里。它**依赖被量对象的性质**，而工具的默认值不挑对象——所以把它并进上一条，等于把一帖治不了它的处方挂到它名下。
  **建这类「完整参照系」索引时注意两格**，两格都会静默偏小：pathspec 写成 `internal/` 加 `cmd/` 会漏掉仓库根下的 `.go`（本仓是 `migrations/migrations.go`），模式写成 `^func <名>(` 会漏掉泛型声明。**索引窄一格，靠它得出的「一个都没有」就少一格依据**——而这种错不会被重跑撞上，因为重跑的人照抄的是同一条命令。
  **报一个零或一个方向时，别在这里另立一道自问清单**——[parallel-sessions.md](./parallel-sessions.md) 「写证据，不写结论」那一级就是处方：**总量与净方向是结论，具名机制及其自身的方向才是证据**，后者再冒出第三个误差源也不会变假。取证：四个会话各自枚举 `docs/` 下 `43|150|246` 的命中，先后报 4、6、8 个文件，第四份专为纠偏少而重做、文件数改对了而行数仍停在 11；**真值 12 个文件 / 19 行**（实测于 `1c71203`，`git grep -c` 直出、不经 PowerShell 管道）。**四份全偏少，一份不多**，其中两份出自刚读过前一份教训的人——所以处方不能写成「记得枚举完整」，那句今晚写过，然后被读过它的人犯了两次。**要证的是「没有」时，漏数那一侧在帮忙——那四道漏数无一被当场发现；同一晚被当场拿住的两处恰恰是反向的**（一份枚举里列进了两个声明模式根本产不出的编号）。**会不会被发现，取决于错倒向哪一边：多列的那一侧会自己露馅，漏数的那一侧不会。**
