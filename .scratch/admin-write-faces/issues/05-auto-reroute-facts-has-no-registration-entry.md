# 自动改路四条件事实登记有机制无入口：CLI 与在线登记口都没有，且这张票两头都被指过却从未立

Category: enhancement
Status: resolved（CLI 第八族已提交 `6e40312`，`HEAD:cmd/parcel-network-register/main.go` 含 `auto-reroute-facts`；据 report.md A 组，随 MCP-1 2026-09-04 12:20 那次推送已在远端。「未提交」三字由通道 2 于 2026-09-04 代簿记去掉）——owner 2026-09-03 裁 **A（只补 CLI）**、判据以 **ADR-0085 决定四**
为准；CLI 第八格已交付并验绿。种子那一格**撤销**（做不成，理由见「种子这一格做不成」一节），
遗留一张 demo 运行时路径票未立，见文末 Comment
Blocked by: 无

## 为什么现在立

两份票面都指向一张不存在的票。

- [`syn-wall-door-audit/05`](../../syn-wall-door-audit/issues/05-auto-reroute-facts-catalog-unimplemented.md)
  已 resolved，其交付评论末句写着「进程级入口（CLI）票面未列，未做——如需照
  `parcel-network-register` 先例另立票」。
- 本目录的 [`02`](./02-remaining-registries-take-online-registration-faces.md) 在 02a 网络的
  范围分辨里点名了 `RegisterAutoRerouteFacts`，判它落在首批外，并记「另立票由 MCP-3 安排」。

**那张票从来没立。** 本目录此前只有 01–04。两头各自都以为对方会接，于是这个洞在跟踪器上
不存在——它不是「进行中的活」，是一处没有任何票面在盯的空白。本票补这个空白。

## 事实基线（取证于 `0054a13`）

**机制半边齐全**，由 `syn-wall-door-audit/05` 于 2026-08-24 交付：

- 迁移 `migrations/network_routing/0009_auto_reroute_facts.sql`（判断键六维 + version 历史链，
  零行表达「未配置」，无中间态）；
- 装载口 `adapters/postgres` 的 `AutoRerouteFactsCatalog.LoadAutoRerouteFacts`；
- 写入方 `RegisterAutoRerouteFacts` 与 `FindAutoRerouteFacts`（走 `RequireExecutor`，无环境事务即拒）；
- 登记用例 `application/register_auto_reroute_facts.go`（受理门逐格拒、幂等与冲突分界在编排、
  绝不覆盖，答案代数就是登记册那套）。

**两个入口都缺**：

- `cmd/parcel-network-register` 的 `-kind` 只认七族——节点、连接、线路、服务区域、服务日历、
  可用性调整、路由策略。自动改路事实不在其中。
- `parcel-api` 端点表无对应登记行。

**由此得到一个当下就成立的后果，而它在装配点看不出来。**
`cmd/parcel-dispatch/assemble.go` 已把 `ReassessRouteDeps.AutoReroute` 从 `nil` 换成真适配器
（`NewAutoRerouteFactsCatalog`），读那一行会以为这条路是活的；而 `scripts/demo-seeds/seed.sh`
的网络段只调上述七族，**没有任何路径能往那张表里写一行**。于是隔离演示环境里该表恒为空 →
目录如实答未配置 → 改路评估整段不做。`application` 侧 `rerouteAfterLapse` 的三态分派
（自动改路 / 建议 / 禁行）早已实现且有测试，**但在 demo 里一次也走不到**。

这正是本仓反复记的那一族：**已接线看着像可用**。适配器确实接上了，缺的是让它非空的那道门，
而缺门这件事在 `assemble.go` 上读不出来——那里只看得见「已经不是 nil 了」。

## 先答再开工：它该走哪个入口，判据用哪一条

**两处判据写得不一样，而选哪条直接决定本票做什么。**

- [ADR-0085](../../../docs/adr/0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)
  Decision 四：首切片选 parcel-pricing 的理由是「登记用例与 CLI 先例齐、答案代数已封闭，
  是最小可裁样本」，**而其余上下文的取舍写的是「登记频次 × 操作者角色」由实施票逐册裁**。
- 票 [`01`](./01-registry-configuration-has-no-admin-write-face.md) 的「裁决」第二项则写成
  「**有登记用例与 CLI 先例的**运营配置册逐上下文进端点表」，票 `02` 引的是这一句。

按后者，无 CLI 即落首批外，本票只剩补 CLI；按前者，CLI 有无不是判据，要问的是这四条件
是不是租户运营配置员的常规配置动作。**两句不是同一条规则**：前者是选首切片的理由，后者被
当成了准入门。ADR 是权威，但 Decision 四把裁量权明写为「由实施票逐册裁」，所以这一裁
落在本票，需要 owner 给一句。

同时 ADR-0085 Decision 一说「CLI 不退场——CLI 与端点消费同一登记用例，是同一能力的受控
批量口与在线口」。它描述的是两口并存的完整形状，**没有要求在没有 CLI 的地方现造一个**。

### 三种落法

- **A · 只补 CLI**：`parcel-network-register` 增第八格 `-kind`，seed 加一份合成条件。
  解开 demo 可达性，在线口留到 `PAR-NET-14` 形态定了再谈。改动最小，且不动共享接线文件。
- **B · CLI 与在线登记端点都补**：照 ADR-0085 Decision 一的完整形状，`adapters/http` 增
  登记命令端点、装配以 `UnconfiguredIntake{}` 起步，管理台网络页加登记签。**要占
  `cmd/parcel-api/endpoints.go` 一行。**
- **C · 都不补**：把这个洞如实记在票面上等 `PAR-NET-14`。代价是 demo 里那条路继续
  死着，而 `assemble.go` 继续看着像活的。

选 A 或 B 都要连带答一句：**四条件（改善阈值、改路条件与权限、冻结边界）改起来是谁的活、
多久一次。** 那正是 Decision 四「登记频次 × 操作者角色」要问的，而本票无权替租户回答——
没有租户，这一问只能按产品设想裁，裁完写进票面当依据。

## 要建什么（裁为 A 或 B 后生效）

1. **CLI 第八格**（A、B 共有）：`-kind auto-reroute-facts`，输入一份登记行 JSON，未知字段
   一律拒绝，退出码沿既有四格。形状照同文件既有七格，不另立。
2. ~~**seed 一份合成条件**：`scripts/demo-seeds/data/network/` 加一份 `SYN-` 风格四条件，
   `seed.sh` 网络段追一行，让 `rerouteAfterLapse` 的三态在 demo 里走得到。~~
   **2026-09-03 撤销，理由见下「种子这一格做不成，而硬做会更糟」。**
3. **在线登记端点**（仅 B）：`internal/networkrouting/adapters/http` 增 Intake 接口 + 处理器
   接口 + 封闭响应形状（ADR-0022 状态码语义），装配以字面量 `UnconfiguredIntake{}` 起步；
   装配行进 `cmd/parcel-api/endpoints.go`（共享接线文件，动前在频道占号）。
4. **管理台**（仅 B）：网络目录页的登记签加这一族。**写签跟着读签走**——若这四条件今天
   没有读面，先不铺登记签，另记一条，别让人登进去却看不到自己登了什么。

## 红线

- 阈值与条件取值全属实例半边（`PAR-NET-14` 待提供）。本票只建入口，一个默认值都不写死；
  合成条件只记 `S`。
- 不新增任何覆盖语义：登记不可覆盖、内容冲突是答案不是失败，逐格照 `RegisterAutoRerouteFacts`
  已有的答案代数转写，不在入口层重新发明。
- 目录未配置时保持现状（整段不做、失效照常落库），不得半配置地只做建议不做说明——
  这条从 `syn-wall-door-audit/05` 原样继承。
- 若裁为 B，写准入不另立形（ADR-0085 Decision 二），隔离 demo 里该端点如实答未配置。

## 种子这一格做不成，而硬做会更糟

**立票时把它当成「顺手加一行」写进了要建什么，实现时发现它建不了。** 记在这里而不是
悄悄不做——立票的人（同一个会话）当时没有核这一格。

事实目录的读口按**判断键**取行：`reassess_route.go` 调的是
`LoadAutoRerouteFacts(ctx, trigger.Key())`，而那个键是 `InitialRouteJudgmentKey` 六维——
租户、客户账户、委托请求、**接受基线版本**、申报包裹、服务目的。后四维是运行时产物：
委托提交、接受决定形成基线、逐包裹派生判断范围，这一串跑完才有键。

而 `seed.sh` 灌的全是**配置类主数据**（价卡、网络七族、关务八册、VE 六类、代收、治理），
全仓种子数据里 `shipment_request` / `declared_parcel` / `acceptance_baseline` 一个都不出现。
代收那份 `parcelRef: SYN-PARCEL-COD-01` 是代收册自己发明的引用，不是路由过的真包裹。

**所以静态种子只能造一个永远命不中的键。** 那一行进表之后，表非空了——目录**看起来**
已配置——而每一次真实复核仍然拿到 `configured=false`。**它比空表更坏**：空表如实答
「这个判断键从未登记过事实」，而一行命不中的种子让「未配置」与「配置了但不是这个键」
在读口答案上变成同一格，恰好就是本票开头写的那个病，只是搬到了下游。

要在 demo 里让那条路活起来，缺的不是一行种子，是一条**跑到复核的运行时路径**——委托
提交 → 接受 → 初始路由 → 触发复核，然后在那一刻按真实键登记事实。那是另一件事，不在
本票范围，也不该塞进 `seed.sh`（它是主数据灌入脚本，不是流程驱动器）。**另立票或并进
现有的 demo 走通类票**，本票只交入口。

## 验证

CLI 单测：绿路径（六维键、三个折算结论、两份清单、折算依据逐件到达写入口，登记时刻取
时钟）、两格治理答案（重放同一份 → `已存在`，换内容或换折算依据 → `内容冲突`，都译成
退出码 2）、译装门与受理门七格拒绝（都不到达写入口）、依赖故障 → 退出码 3。

**不含真库用例**：本票只加进程级入口，写口自身的版本纪律与租户隔离由
`adapters/postgres/auto_reroute_facts_test.go` 的真库用例证，本票零改动那一层。

裁为 B 再加：http 单测只收 POST、未配置 403、三态响应。

## Comments

- 2026-09-03 · MCP-4：立票。起因是本会话读票 `01` 末条「02+ 仍未开工」后打算接 02a 网络，
  MCP-5 拦下并指出 02a 早已完成、且收口核验正是本会话 2026-09-02 自己做的；顺着那条纠正
  才发现真正的空白在这里。取证时 HEAD 为 `0054a13`，工作树里他人在途改动未动。
  **本票只写票面，未动任何代码。** 三种落法的选择与「登记频次 × 操作者角色」那一问都留给
  owner；ADR 与票面两处判据的不一致如实记在上面，不代为并成一条。

- 2026-09-03 · MCP-4：**owner 裁决 + CLI 交付。**

  **裁决两句**（owner 于通道 4）：落法取 **A**——只补 CLI，在线登记端点不做；判据以
  **ADR-0085 决定四**（「登记频次 × 操作者角色」逐册裁）为准，**不以票 01 裁决二那句
  「有 CLI 先例才进端点表」为准**。后者是选首切片的理由，被当成了准入门。下一个撞到
  这一格的人按 ADR 那句办。因为不做在线口，本票全程未碰 `cmd/parcel-api/endpoints.go`。

  **交付**：`parcel-network-register` 增第八族 `-kind auto-reroute-facts`。

  三处结构性改动，都不是「照抄第八格」那么简单，记下理由：

  1. **`execute` 改收一个登记用例集（`registrars`）而不是单个目录用例。** 两族的结果
     类型与答案代数不同，合并成一个接口要先造一个两边都不自然的结果类型。族路由仍只有
     一处——事实族在 `execute` 开头分出去，`commandFor` 的封闭七格原样不动。
  2. **本口从此有退出码 2。** 文件头原写着「没有退出码 2：本口今天没有治理格」，那句
     对 0008 的七族仍成立（重复版本号由主键挡），对 0009 的事实目录不成立——它的登记
     用例自己就答`已存在`与`内容冲突`，把这两格折成退出码 1 会让操作员以为改输入重试
     就成。文件头已按这个分界重写，不是删掉原句了事。
  3. **族名列成 `supportedKinds` 一处。** `-kind` 的用法文本与未知族的错误文本此前各
     抄了一遍族名，新增一族时漏改其中一段不会有任何东西变红。

  **验证**：`gofmt -l` 零输出；`go build ./...` 零信号；`go vet ./cmd/parcel-network-register/`
  与 `./internal/networkrouting/...` 零信号；`go test -count=1 ./cmd/parcel-network-register/`
  ok，新增四个用例（绿路径 / 三格治理答案 / 七格拒绝 / 未决）全 PASS。**不含真库**——
  本片纯进程级入口，写口的版本纪律与租户隔离由 `adapters/postgres` 的真库用例证，那一层
  零改动。`internal/networkrouting` 的领域与应用层同样零改动。

  **未提交。** 提交与推由 owner 定。

  **遗留已另立票**：[`auto-reroute-demo-reachability/01`](../../auto-reroute-demo-reachability/issues/01-no-runtime-path-reaches-the-reassess-auto-reroute-branch.md)。
  demo 里那条路仍然走不到，成因不是缺种子而是缺一条跑到复核的运行时路径，见上面「种子
  这一格做不成」一节。**别再把它写成「加一行种子」**，那正是上一版票面写错的地方。
