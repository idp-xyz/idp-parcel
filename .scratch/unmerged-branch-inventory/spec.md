# 未合并分支与 worktree 集成清单

Category: chore
Status: in-progress——盘点本身已做完（第四遍逐笔打开读完，第三类由四笔降至两笔：第 5 要裁语义、第 13 真缺一段操作警告）；这两笔的去留按本清单自身边界归各分支作者与集成方，本清单不判也不合并，故不自行转 resolved

本清单**不合并、不拆树、不删分支**，只把「哪些活真的没进 main」摆成可派的形态。逐笔的去留归各分支作者或集成方；`.scratch/` 之外一字未动。

## 取证基线

- **`main = aeeb70982bebaced7001a5c854d1cacc392e4d99`**，用 `git ls-remote origin main` 实测于 **2026-09-01 22:14 (UTC+8)**，不是读 `origin/main` 缓存（缓存读数在两个方向上都 fail-open，理由见[并行会话](../../docs/agents/parallel-sessions.md)「断言有保质期」）。
- 本文所有「未合并」「dirty」断言以该时刻为准。共享树上这类断言有保质期，引用前重取一次。
- 下文的**数本身就是论点**（「十四笔真欠账」要立得住，就得说清是十四不是十七），因此每个数都锚住上面那个 SHA 与下面那条命令。

## 量法：为什么不是 `git diff`

先用过一版残差量法，**结论错了，记在这里免得下一个人重走**：

```powershell
$files = git diff --name-only "main...$b"
git diff "$b" main -- $files      # 空 = 内容已在 main
```

它在 `aeeb709` 上把**十七个**分支全判成未集成。错在这个差集里混着 main 在分叉之后对同一批文件的改动——它量的是「两棵树今天差多少」，不是「这个分支的活进去了没有」。同一个问题问错了一格，数是真的，结论是假的。

正确的量法是按 patch-id 判等价提交，它认得出 cherry-pick：

```powershell
git cherry main <branch>          # '+' = main 里没有等价 patch，'-' = 有
```

换问法之后是十四笔。**复核本清单时重导这个问题，不要只重跑这条命令。**

**而 patch-id 自己也有盲区，这一条是被人当场拆出来的。** MCP-4 作为 `mcp4-bento-pbc02` 的分支作者复核第 9 笔，发现它的内容早已进 main：`6ea47f6`（08-24）提交信字面写着「整体采回 mcp4-bento-pbc02@1f7462b（分支指针保留供补验）」。patch-id 对不上，是因为采回那一笔同时退役了重叠件、又补了新用例——**采回时做了增删，patch 就不同，`git cherry` 照样报 `+`**。

所以三种量法各有各的盲区，按盲区选而不是按顺手选：

| 量法 | 认得出 | 认不出 |
|---|---|---|
| `git merge-base --is-ancestor` / 日志 | 直接合并 | cherry-pick、采回 |
| `git cherry`（patch-id） | 直接合并、逐字 cherry-pick | **带增删的整体采回** |
| `git log main --diff-filter=A -- <该笔新增的文件>` | 谁把这些文件带进 main 的 | 只改不增的那种笔 |
| `git log main -S"<该笔引入的符号>" -- <文件>`（pickaxe） | **只改不增的笔是谁落的** | 重命名、纯行为改写 |
| 按**能力**找对应件（同包同职责、不同文件名）逐份打开读 | **改名或拆分之后重落的笔** | 什么都认得出，但只有人做得了 |

**第五行是第三次补进来的，成因与前两次同构**：`--diff-filter=A -- <路径>` 问的是「**这个路径**是谁带进 main 的」，改过名就必然报「不在」，而能力可能不但在、还更强。上一行 pickaxe 的「认不出」栏早写着「重命名」，但那句话当时只被读成一句免责声明，没有人据它去补一种量法——**盲区写在表里不等于被覆盖**。第四遍（见下）在第 12 笔上实测到了这一格。

**第四行是补进来的**：第 11 笔 `pc01-identity-face` 只改不增，第三种量法对它不适用，本清单第二遍因此只能停在「要人看」。pickaxe 按「某字符串出现次数发生变化」定位，正好补上那个盲区——用它一步就问出引入者是谁（见[第三类的第二遍](#第三类的第二遍溯源量法逐笔跑过之后)第 11 笔那段的补证）。

**因此下面第三类的笔数是上界，不是定数**：未逐笔跑过后两种量法的那些，同样可能已被等价重写。要坐实某一笔，照 MCP-4 的路走一遍——查它新增的文件（或它引入的符号）是谁带进 main 的，再看残差方向是 main 领先还是分支领先。

**另有一个取证陷阱，第 11 笔上踩到过，记在这里**：这类笔上 **blob 相等是错的判据**。跟今天的 main 比会全不同（main 在演进），跟引入提交比也可能全不同（那是独立重写而非重放），两次都看着像「没进去」。能判定的只有两步——先定位引入者，再打开实质执行器看它在做什么。自动化到此为止，剩下的必须有人读。

## 三类，不要混

### 一、已全进 main，可以关掉（2 个分支）

`git cherry` 全部报 `-`，分支上每一笔在 main 里都有等价 patch。

| 分支 | 领先笔数 | 等价 patch 已在 main |
|---|---|---|
| `mcp5/admin-skeleton-0506` | 8 | 8 |
| `mcp5/admin-skeleton-0506-stage2` | 2 | 2 |

关分支前照纪律先比内容再动指针，且**分支指针是事后补验的唯一凭据**——树可以拆，指针留着。

### 二、明示「非集成候选」，未合并是设计如此（2 笔）

两笔都是封存死会话现场，提交信自己写着「仅防拆树抹掉，非集成候选」。**它们不是欠账，不要合并。**

| 分支 | SHA | 内容 |
|---|---|---|
| `bento-gate-reeval` | `2399ecd` | Bento 闸门重估的脚本与证据输出原样入分支（dead-session-salvage 票 02） |
| `syn-wall-door-audit` | `a958029` | 墙/门审计报告与十张票原样入分支（同上） |

### 二之二、内容已整体采回，patch-id 对不上（1 笔）

| 分支 | SHA | 采回它的提交 |
|---|---|---|
| `mcp4-bento-pbc02` | `1f7462b` | `6ea47f6`（08-24，bento 票 03 终收口） |

取证（锚 `main = aeeb709`，MCP-4 首报，MCP-2 独立复验）：`git show --diff-filter=A --name-only 1f7462b` 得十件新增文件，逐件 `git cat-file -e aeeb709:<path>` 全部命中；`git diff --numstat mcp4-bento-pbc02 main -- <这十件>` 只有一行且方向是 **main 领先**（`tests/bentocontract/shipment_request_commit_uncertain_test.go` +95、零删除）。MCP-4 回报记十一件，本轮实测十件，差在哪一件未逐件核对——不影响结论方向。

**分支指针不能删。** 保留它是 `6ea47f6` 自己写下的补验承诺，不是忘了清。

> **同日复发一次，记在这里当量法表第一行的活证据。** 2026-09-02 19:5x，另一个会话用
> `git branch --contains 1f7462b`（只列出它自己）重新判定本笔「真的没合进主线」，并据此建议
> 单独排期。那条命令问的是「这个**提交对象**是不是某分支的祖先」，正是量法表第一行「认不出
> cherry-pick、采回」那一格——而本节上方那段取证问的是「这一笔的**活**进去了没有」，两个问题
> 的答案在采回场景下本来就相反。复核实测（锚 `origin/main = 398a148`）：`1f7462b` 的十件新增
> 文件逐件 `git cat-file -e origin/main:<path>` **十件全部命中**，结论不变。
>
> 值得记的不是谁判错，是**这份文档当天上午刚补完第五行量法、当天傍晚就有人用第一行重走了
> 同一个坑**。量法表拦不住没读过它的人；真正拦得住的只有一条习惯：判「某分支进没进主线」
> 之前先翻表选量法，而不是顺手敲一条自己最熟的命令。

### 三、真欠账：查清后剩四笔（第 4、5、12、13）

按提交时间升序。「关联」一栏只记提交信自己点名的 ADR 或票，不替它推断。

**本表最初列十四笔，如今剩四笔，划掉的各有各的去处，不要看成同一回事**：第 9 笔是带增删的整体采回（「二之二」），第 6、10、14 笔是重新落笔，第 11 笔是隔数日的独立重写，第 1、2、3、7、8 笔是换基座重放（[第三遍](#第三类的第三遍票-0102-交答五笔全部移出)）。**编号一律不重排**——重排会让已经引用过某个号的消息全部指错。

| # | 分支 | SHA | 日期 | 触及路径 | 关联 |
|---|---|---|---|---|---|
| ~~1~~ | ~~`pn07-b6-stage-content`~~ | ~~`22bb69f`~~ | — | **已移出：经 `6e4ccda` 换基座重放进 main（票 01）** | ADR-0058 |
| ~~2~~ | ~~`product-version-closure-b7`~~ | ~~`9d8c09b`~~ | — | **已移出：经 `74a6c35` 换基座重放进 main（票 01）** | ADR-0059 |
| ~~3~~ | ~~`ps-rehydrate-accepted`~~ | ~~`2f7e144`~~ | — | **已移出：经 `6228d8e` 换基座重放进 main（票 02）** | ADR-0061 |
| 4 | `cons-proj-tf-b` | `f3a3c2b` | 08-19 | `cmd/parcel-dispatch` | CONS-PROJ-TF-B |
| 5 | `t12-governance-register` | `4b5432a` | 08-21 | `cmd/parcel-governance-register`、`internal/pilotgovernance`、`migrations/pilot_governance` | 票 12（提交信记 resolved） |
| 6 | `t12-governance-register` | `03a17f6` | 08-21 | `internal/pilotgovernance`、`migrations/pilot_governance` | 票 11（提交信记 resolved） |
| ~~7~~ | ~~`t14-payload-digest`~~ | ~~`321c841`~~ | — | **已移出：经 `b51de75` 换基座重放进 main（票 02）** | ADR-0014 |
| ~~8~~ | ~~`nr04-catalog-registration`~~ | ~~`6cf6c89`~~ | — | **已移出：经 `e2620ab` 换基座重放进 main（票 02）** | ADR-0068、审计票 04（提交信记 resolved） |
| ~~9~~ | ~~`mcp4-bento-pbc02`~~ | ~~`1f7462b`~~ | — | **已移入「二之二」：内容经 `6ea47f6` 整体采回** | — |
| 10 | `cc03-ports-declaration-paths` | `aa2a60e` | 08-28 | `scripts/demo-seeds` | admin-remainder-mechanism-batch/03 |
| 11 | `pc01-identity-face` | `3a6c78c` | 08-28 | `internal/partycommercial`、`cmd/parcel-api`、`apps/admin-web` | 票 01 补格 |
| 12 | `cr04-backend` | `9834388` | 08-28 | `migrations/line_endings_test.go` | — |
| 13 | `cr04-readface` | `2f99bd3` | 08-31 | `scripts/demo-seeds` | admin-remainder-mechanism-batch/04 |
| 14 | `mcp5/admin-skeleton-05-stage2` | `f84c16e` | 08-31 | `apps/admin-web` | admin-skeleton-closure-batch/05 阶段二 |

### 第三类的第二遍：溯源量法逐笔跑过之后

MCP-4 拆出第 9 笔之后，把 `git log main --diff-filter=A -- <该笔新增的文件>` 对**未立票的那八笔**逐笔跑了一遍（锚 `main = aeeb709`）。结果分三格。

**又清掉三笔——内容已在 main，只是 patch-id 对不上：**

| # | 分支 | SHA | 带它进 main 的提交 |
|---|---|---|---|
| 6 | `t12-governance-register` | `03a17f6` | `c26b50f`（标题与原笔基本同文） |
| 10 | `cc03-ports-declaration-paths` | `aa2a60e` | `1773508`（同上） |
| 14 | `mcp5/admin-skeleton-05-stage2` | `f84c16e` | `d76739c`（同上） |

三笔的新增文件在 main 上逐件 `git cat-file -e` 全部命中，且带它们进 main 的提交标题与原笔基本同文——是重新落笔而不是另起炉灶。

**确认真缺，且缺的是哪几件说得出来：**

| # | 分支 | SHA | main 上缺什么 |
|---|---|---|---|
| 5 | `t12-governance-register` | `4b5432a` | 七件新增里缺三件，全是**受控渠道执行留痕**那一半：`internal/pilotgovernance/adapters/postgres/channel_trace{,_test}.go` 与 `migrations/pilot_governance/0004_controlled_channel_execution.sql`。其余四件在位——**这一笔是半进半不进，不要整笔判** |
| 12 | `cr04-backend` | `9834388` | 唯一那件 `migrations/line_endings_test.go` 不在 main。行尾守卫确实没进 |
| 4 | `cons-proj-tf-b` | `f3a3c2b` | 两件新增里缺一件：`cmd/parcel-dispatch/effective_delivery_projection_test.go` |
| 13 | `cr04-readface` | `2f99bd3` | 只改不增，溯源量法不适用；改用逐行比对——该笔在 `scripts/demo-seeds/seed.sh` 加的那一行，main 的同名文件里没有 |

**一笔要人看，机器判不了：**

第 11 笔 `pc01-identity-face` `3a6c78c` 只改不增，它触及的八个文件在 main **全部存在**，但该笔新增的行在 main 里几乎都不在（`internal/partycommercial/adapters/postgres/party_identity_registry_test.go` 与 `ports.go` 是零命中，其余各文件命中数都是个位数而新增行数在数十）。少量命中多半是括号一类的常见行，不作数。

文件同名同位置而内容不同，正是「等价实现」与「真欠账」在机器视角下签名相同的那一格。**打开看之后是等价实现**，逐条取证（锚 `aeeb709`）：

- main 的 `ports.go` 有 `PartyIdentityCatalogueRead`，注释写着它是「`group-legal-entities` 与 `business-parties` 两页的供数面」，正是该笔要立的那个读口；`BusinessPartyRow` 也在 main（main 版 `query_party_identities.go` 的 `businessPartyBodyOf` 形参类型就是 `ports.BusinessPartyRow`）。
- main 版与分支版 `query_party_identities.go` 的顶层声明**集合逐个相同**：`businessPartyListResponse`、`businessPartyBodyOf`、`groupLegalEntityListResponse`、`groupLegalEntityBodyOf`、`partyRelationshipListResponse`、`partyRelationshipBodyOf`。
- 真库侧三件在 main 全在：`party_identity_catalogue.go`、`party_identity_registry.go`、`party_identity_registry_test.go`。

**顶层声明集合相同不等于行为相同**，这条证据链证的是「这个能力在 main 有执行器」，不是「两版逐行等价」。但它足以推翻「这一笔的活没进 main」那一读——第 11 笔移出真欠账。

> **补证（MCP-4，用 pickaxe 定位引入者）**：上面的判断成立，且能指名是谁落的。`git log main -S"ListBusinessParties" -- internal/partycommercial/ports/ports.go internal/partycommercial/adapters/postgres/party_identity_catalogue.go` 与 `git log main -S"commercial-business-parties" -- cmd/parcel-api/endpoints.go` 均只命中 **`726ae08`**（09-01 18:55），其标题点的是同一张票 `admin-remainder-mechanism-batch/01`。
>
> **但它与本清单其余各笔不同类：那是独立重写，不是换基座重放。** 父不同（`3a6c78c^ = e7f3ee7`、`726ae08^ = fe18c44`），且在**引入时**逐件比 blob 八件全不同——隔了五天各写各的。实质执行器却收敛到同一份：`git diff 3a6c78c:<f> 726ae08:<f>` 在 `party_identity_catalogue.go` 上**只差注释措辞**，`ListBusinessParties` 函数体一字不差。
>
> **而且 main 那版更完整**：`3a6c78c` 提交信自己写着「本笔不含装配行」，把 `cmd/parcel-api` 的路由行、路由表与隔离读准入清单留给 MCP-1；`726ae08` 把这三行一并落了。合进 `3a6c78c` 反而得到比 main 现状更少的一份。

### 第三类的第三遍：票 01/02 交答，五笔全部移出

两票（见[子票](#子票)）已交答，锚同为 `main = aeeb709`。**五笔实现已接受 ADR 的提交，其内容全部已在 main，五笔一律是换基座重放**——父都与对应 main 笔不同，patch-id 因上下文行不同而不等价，`git cherry` 的 `+` 由此而来。

| # | 分支 | SHA | ADR | 带它进 main 的提交 | 标题关系 |
|---|---|---|---|---|---|
| 1 | `pn07-b6-stage-content` | `22bb69f` | ADR-0058 | `6e4ccda` | 逐字同 |
| 2 | `product-version-closure-b7` | `9d8c09b` | ADR-0059 | `74a6c35` | 逐字同 |
| 3 | `ps-rehydrate-accepted` | `2f7e144` | ADR-0061 | `6228d8e` | 逐字同 |
| 7 | `t14-payload-digest` | `321c841` | ADR-0014 | `b51de75` | 逐字同，尾部多一句「票 14 转 resolved（票面回写由抢救笔补）」 |
| 8 | `nr04-catalog-registration` | `6cf6c89` | ADR-0068 | `e2620ab` | 逐字同 |

五笔各有新增文件，`--diff-filter=A` 就够用。逐文件比 blob（各自比**引入时**那一版）几乎全同，两处例外都不是缺口：

- 第 1 笔的 `internal/partycommercial/ports/ports.go` 不同——累积型共享文件，两侧基座本就不同；只取该文件的新增行比对则相同。第 2 笔同理。
- 第 3 笔的 `internal/parcelshipment/adapters/postgres/shipment_request.go` 不同——打开看，那段差异整段是 **ADR-0060** 的当前包裹投影（`current_submission_version_id` / `declared_parcel_ids` 两列与 `currentParcelProjection`），与 ADR-0061 无关，且方向是 **main 领先**。

**现在第三类还剩四笔**：第 4、5、12、13（原十三笔减第 6、10、14、11，再减本节五笔）。这四笔已在第二遍里逐笔查过、缺什么都指得出来，因此**这个数不再是上界，而是查清后的定数**——除非有人在 `aeeb709` 之后又往 main 推了东西。其中第 5 笔是半进半不进，不要整笔判。

### 第三类的第四遍：四笔逐笔打开读，剩两笔

MCP-3，锚 **`main = ddba601`**（`git ls-remote origin main` 实测于 2026-09-02 18:5x (UTC+8)，非缓存；
同刻 `HEAD` 与远端同值，主树无未推提交）。第三遍留下的四笔（第 4、5、12、13）逐笔按上表第五行
的量法——**按能力找对应件，不按路径找**——打开读。结果两笔移出、一笔坐实、一笔改判性质。

**第 12 笔移出：主线有一份改了名的等价守卫，而且更严。**

分支那件是 `migrations/line_endings_test.go`，main 上的是 `migrations/eol_guard_test.go`——
**同一个包、同一个走法、同一道空集自检**：都走 `fs.WalkDir(assets, ...)` 遍历嵌入资产而不逐个列
模块加载函数（两份的注释都点明理由是「要靠人记得加名字的清单，漏掉的那次正是它该挡住的那次」），
都在 `checked == 0` 上另设 `t.Fatal`，都同门看 UTF-8 BOM。

差别只有一处，方向是 **main 更严**：分支版数 `bytes.Count(content, []byte("\r\n"))` 只认 CRLF，
main 版取 `bytes.IndexByte(content, '\r')` 连**孤立 CR** 也拦，并报出首见字节偏移。

带它进 main 的是 `9132594`（批务票 admin-remainder-mechanism-batch/05 的「根因跟进」节记着这一笔，
并注明门经证伪：临时 CRLF 化一份 SQL 立即 FAIL、还原后绿）。合进 `9834388` 反而得到较弱的一份。

**第 4 笔移出（信心低于第 12 笔，理由写在下面）：断言被拆成两个用例落进 main。**

分支那件 `cmd/parcel-dispatch/effective_delivery_projection_test.go` 只有一个用例
`TestARegisteredEffectiveDeliveryDerivesAnUnclassifiedProjectionAndStopsFinal`，名字里并列两条断言：
派生出未分类投影、且停在终局。main 上这两条各自成篇——
`effective_delivery_final_test.go` 的 `TestARegisteredEffectiveDeliveryStopsAtUnconfiguredFinalRule`
与 `effective_delivery_kind_mapping_test.go` 的
`TestAMappedEffectiveDeliveryKindClassifiesTheProjectionWhileFinalStaysUnconfigured`。

**这一笔只核到用例名一层，没有逐行比断言体**，因此它的证据强度低于第 12 笔：名字对得上不等于
覆盖对得上（分支版走的是「未分类」那一支，main 的第二个用例走的是「已映射种类」那一支，两者
是不是同一格没读到底）。写成移出而不是待定，是因为该笔的主体（`assemble.go` 的 FanOut 接线）
早已在 main——真要坐实这一件测试的覆盖差，得有人打开两份读断言，那超出只读盘点的范围。
**引用本行时连这句一起引，别转述成「第 4 笔已确认等价」。**

**第 13 笔坐实是真缺，且缺的东西比表里那行字面看着重要。**

`git grep -c "UNDERFUNDED" -- scripts/demo-seeds/seed.sh` 在 main 上零命中。那不是一行代码，是一段
**操作警告**：已灌过的库上重放代收那一节会在记账处以 `UNDERFUNDED`（退出码 4）中止，因为写口的
重放判定排在余额守卫之后——来源位置被原记账清空后守卫先答余额不足，轮不到「已在册」。结论是
非干净库复灌该节一律走 `--reset`，别指望它像前六步那样幂等。

这段话的价值恰恰在于它**记的是一次实测**（原笔注明实测于 `d340014` 对演示库重放）。丢了它，下一个
人重放种子撞上退出码 4 时，读到的是「余额不足」而真正的成因是「这一节不幂等」——又一格
「两种状态可观察签名相同」。**它同时也说明本清单第三节那条判据要读窄**：`.sh` 里的一行注释在
`git cherry` 与溯源量法下与一行代码毫无区别，而它的去留判断完全不同。

**第 5 笔不是改名，是两套模型，判性质而不是判去留。**

main 有 `channel_executions{,_test}.go` 与 `migrations/pilot_governance/0004_channel_execution.sql`，
分支有 `channel_trace{,_test}.go` 与 `0004_controlled_channel_execution.sql`——文件名对得上号，
**但里面不是同一件东西**：

| | 分支 `4b5432a` | main `ddba601` |
|---|---|---|
| 表 | `controlled_channel_execution` | `channel_execution` |
| 主键 | `(registration_kind, registration_ref)` | `(execution_id)` |
| 种类封闭集 | 有，`authority-interval` / `suspension` / `resumption` | 未见 |
| 端口 | `ports.ControlledChannelTraceStore` | 无对应端口断言 |
| 写法 | `Record`（按登记幂等） | `Append`（追加日志） |

按登记键幂等与按执行标识追加是**两种不同的留痕语义**：前者「同一次登记重复执行只留一条」，后者
「每次执行各留一条」。这不是改名，也不是等价重写，**是一个没人做过的取舍**——而它落在
`internal/pilotgovernance` 的地盘上。本清单不判，按其自身边界上报。

**第三类现在剩两笔：第 5（要裁语义，非集成排期）与第 13（真缺一段操作警告）。**
第 4 笔按上面写明的强度移出。**编号仍不重排。**

## 三件要人定，只点名不开票

**一、第 12 笔防的正是本仓当天在犯的病。** `9834388` 是一条迁移资产行尾守卫——嵌入的 SQL 含 CRLF 或 BOM 即失败。同一天在共享树上量到：`git status` 里八份文件（`docs/archive/` 三份、`docs/prd/`、`docs/adr/0067`、三份 `.scratch` 票面）显 ` M` 而 `git diff` 零输出，`git update-index --refresh` 之后仍显 ` M`，八份 mtime 同为 `09-01 17:26:57`；`core.autocrlf=true`。那八份是文档不是迁移，因此这条守卫**拦不住它们**——两件事同源不同域，不要写成「合了这笔就好了」。它只说明这一类在本仓是活的。

**二、~~五笔实现的是已接受 ADR（第 1、2、3、7、8 笔），躺了约两周。~~ 已由票 01/02 答完，此条销案。** 原问是：这与[开发主线](../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「八个切片机制半边全数达标」是否相抵。答案是**不相抵**——五笔的内容全部已在 main（换基座重放，见第三遍），且五条 ADR 的 Decision 在 main 上逐条有执行器（逐条落点见两票的「答案」节）。当初不判是对的：`git cherry` 只答「这一笔的 patch 不在 main」，不答「这件事在 main 里没做」，两个问题的答案在这五笔上正好相反。

**两处结论要读窄一格，不要当成「全绿」转述**（都出自两票的取证，且都与那五笔的 ADR 自身声明一致，不是新缺口）：`CanonicalizeSubmissionPayload`（ADR-0014 委托半边）与 `LoadAcceptanceRulePackage`（ADR-0059）在 main 上**都没有生产调用方**——前者卡在 `PAR-INT-01` 的接入契约，后者是 ADR-0059 自己写明的「显式代价」。「机制有执行器」与「有运行中的消费者」是两件事，就绪度结论引用时要说清引的是哪一件。

**三、第 5、6、8 笔的提交信里写着对应票已转 `resolved`，而提交不在 main。** 票面状态与集成状态是两回事，本清单只记这个并列事实，不改任何票面。

## worktree：主树之外十九棵，分三类

同一时刻用 `git -C <path> status --short --untracked-files=all` 逐棵量（`--untracked-files=all` 不可省，普通 `--short` 对被忽略目录只显一行）。

**有未提交内容，一律不能拆：**

| 路径 | 分支 | 条目数 |
|---|---|---|
| `%TEMP%/idp-parcel-mcp2-cc03` | `cc03-ports-declaration-paths` | 18 |
| `D:/tops/idp-parcel-mcp3` | `mcp3/admin-skeleton-closure` | 3 |
| `%TEMP%/idp-parcel-mcp1-t14` | `t14-payload-digest` | 1 |

`D:/tops/idp-parcel-mcp3` 那棵要多看一眼：它的分支已全进 main，所以那三项是合并之后又写的，不在任何已集成的笔里。判它活还是死要量 mtime，而 mtime 只在很短的窗口内分得开——本清单未量，谁要处置谁量，量完写「截至某时未改动」而不是「已死」。

**七棵 detached 验证树，同刻均为零条目，无分支指针，拆了不丢东西：**
`%TEMP%` 下 `cr04-verify2`、`nr04-verify`、`r27-recheck`、`r27-scan`、`verify-820`、`verify-caselink`，以及 `D:/tops/idp-parcel-verify-02`。

**九棵带分支指针、同刻零条目：**
`%TEMP%` 下 `idp-parcel-mcp2-cr04`（`cr04-backend`）、`idp-parcel-mcp3-cr04`（`cr04-readface`）、`idp-parcel-mcp4-bento02`（`mcp4-bento-pbc02`）、`idp-parcel-mcp6-t12`（`t12-governance-register`）、`idp-parcel-nr04-catreg`（`nr04-catalog-registration`）、`pc01-identity-face`、`t3-admin-write-faces`，以及 `D:/tops/idp-parcel-mcp5`（`mcp5/admin-skeleton-0506-stage2`）、`D:/tops/wt-mcp3-closure`（`mcp3-skeleton-closure`）。

这一类树可拆而**分支指针要留**——树拆了还能补救，分支一删就真的比不成了。

> 本节此前只写「三棵 + 七棵」，把这九棵整类漏了，读的人会以为未列出的不存在。缺口由 MCP-4 复核时指出（它自己那棵既不在三也不在七里）。三 + 七 + 九 = 十九，加主树 `D:/tops/idp-parcel` 与 `git worktree list` 实测的二十棵对得上。MCP-4 回报记「剩下十棵」，实为九棵——它把 `D:/tops/idp-parcel-mcp3` 也算了进来，而那棵已在上面三棵脏树里。

拆时**永不加 `--force`**：本仓三次未提交内容丢失全部发生在孤儿 worktree 被 `--force` 拆掉，而验证树会正当地积下日志与产物，那声拒绝正是该看见的东西。

## 子票

只读取证，两票并行无阻塞边。两票合起来答「三件要人定」第二条——五笔 ADR 实现未合并，究竟是 main 里真的没做，还是 main 已有出自别的分支的等价实现。**两票均已交答**（结论进[第三遍](#第三类的第三遍票-0102-交答五笔全部移出)与「三件要人定」第二条），该条禁令随之解除。

- 01 PC 侧：ADR-0058、ADR-0059 —— MCP-4，`resolved`
- 02 PS/NR 侧：ADR-0014、ADR-0061、ADR-0068 —— 原派 MCP-6，其在派票同一分钟失去响应，`resolved`（由 MCP-4 代做，交接理由记在该票 Comments）

余下八笔（`t12` 两笔、`pc01-identity-face`、`cc03`、`cr04-backend`、`cr04-readface`、`cons-proj-tf-b`、`mcp5/05-stage2`）未立票：它们的提交信没点名 ADR，去留更像集成排期而不是就绪度问题，等前两票的答案再定要不要同法处理。

**这八笔各自的分支作者若在线，最有效的一步是自查**——第 9 笔就是这么被拆出来的，分支作者知道自己那笔被谁采回，而盘点方只看得见 patch-id 对不上。

## 本清单不做

- 不合并、不 cherry-pick、不删分支、不拆 worktree。
- 不判各笔该不该进 main——那归分支作者与集成方，多数不在本会话地盘。
- 不改任何票面 `Status:`，不动 `docs/**`。
- 不判第二节那个「机制半边达标」的问题，理由见「三件要人定」第二条。

## Comments

- 2026-09-02 MCP-3（跨地盘代写：MCP-2 自 2026-09-01 22:21 起未响应，本清单此后无人维护；本轮由用户经频道 3 指示「协同并行工作全部 resolved」而进入）：跑第四遍，锚 `main = ddba601`（`ls-remote` 实测，非缓存）。改动三处：① 量法表补第五行「按能力找对应件」，并记下**盲区写在表里不等于被覆盖**——pickaxe 那行的「认不出」栏本来就写着「重命名」，第 12 笔仍旧被漏判了一整轮；② 新增[第四遍](#第三类的第四遍四笔逐笔打开读剩两笔)，第 12 笔（改名等价且 main 更严，引入者 `9132594`）与第 4 笔（断言拆成两个用例，**证据强度低于第 12 笔，已在正文写明限度**）移出，第 13 笔坐实真缺，第 5 笔改判为「两套留痕语义的未决取舍」而不是集成排期；③ 状态行改写成说明「盘点做完了、去留仍不归本清单」，原来那句「第三类查清后剩四笔」已过期。未合并、未拆树、未删分支、`.scratch/` 之外一字未动。

- 2026-09-01 MCP-4（跨地盘代写，经用户授权）：MCP-2 于 22:21 后失去响应（频道心跳实测），本清单此后无人维护，用户授权由 MCP-4 把两票答案与 pc01 补证并入。改动四处：① 量法表补第四行 pickaxe，并记下「blob 相等在只改不增的笔上是错的判据」这个陷阱；② 新增[第三遍](#第三类的第三遍票-0102-交答五笔全部移出)，第 1、2、3、7、8 笔全部移出真欠账（换基座重放，引入者 `6e4ccda` / `74a6c35` / `6228d8e` / `b51de75` / `e2620ab`），第三类降至四笔且**不再是上界**；③ 第 11 笔补上引入者 `726ae08` 与「独立重写而非重放、且 main 那版多带三行装配」——原判「等价实现」成立，本条是加强不是纠正；④「三件要人定」第二条销案，并标出两处要读窄一格的结论。**MCP-2 第二遍那八笔的判读经复核全部成立，一处未改。** 未合并、未拆树、未删分支、未提交。
- 2026-09-01 MCP-2（第三笔，22:3x）：把 MCP-4 示范的溯源量法对未立票那八笔逐笔跑完，结果见[第三类的第二遍](#第三类的第二遍溯源量法逐笔跑过之后)。又清掉三笔（第 6、10、14），确认真缺四笔且能指名缺哪几件（第 5、12、4、13），留一笔机器判不了要人看（第 11）。第三类降至十笔，仍是上界。**没等分支作者回自查**——他们不一定在线，而这个量法盘点方手上就有；作者侧的信息差（知道自己那笔被谁采回）仍然只有作者补得上，广播不撤。
- 2026-09-01 MCP-2（第二笔，22:2x）：按 MCP-4 复核意见改两处，改前已独立复验。① 第 9 笔 `mcp4-bento-pbc02` 1f7462b 移出第三类——内容经 `6ea47f6` 整体采回，`git cherry` 报 `+` 是因为采回时有增删。由此在量法一节补第三个盲区与三种量法的对照表，并把第三类降为**上界**：其余各笔未逐笔跑过 `--diff-filter=A` 溯源。② worktree 一节补上被整类漏掉的九棵带分支指针的干净树，三 + 七 + 九 = 十九与 `git worktree list` 的二十棵（含主树）对上。MCP-4 的两条意见方向都对，其中「剩下十棵」实为九棵（重复计入已在脏树表里的 `D:/tops/idp-parcel-mcp3`），已在正文注明。
- 2026-09-01 MCP-2：只读盘点落盘。触发来自用户问 MCP-3/5 是否 crash——查证结果是无进程崩溃（七个 `mcpServer.cjs` 进程对应本窗口七个启用服务器），MCP-3 截至 22:09 已约四小时未响应且未消费 22:07 投入的广播，但其三个分支全部已进 main、T2 取证产物已提交在 `f95053a`，零工作风险。顺查工作树时撞见本清单这批陈账。`.scratch/` 之外零改动，未提交。
