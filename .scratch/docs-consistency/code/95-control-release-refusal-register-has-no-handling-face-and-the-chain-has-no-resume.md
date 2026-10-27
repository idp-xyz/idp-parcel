# 95 释放未受理册没有查阅与处理面，接受链为拒收响亮失败之后没有续办触发

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-code95（见文末「完成记录」）。此前 ready-for-agent——R2（上线后尽快）：今天走得到这两格的只剩施加时没存作用域的存量行与编程错误（提供方答复译不动），首发前只有演示租户、存量随 `--reset` 即消；但真实租户一旦遇到，被拒收的那一笔会一直占着客户的余额或额度、没有人看得见，接受链那一路的新版本也一直判不了。2026-10-26 协调方据 code/71 隔离评审立（用户「你作为业务和系统专家自决」授权）。
Blocked by: 无
出处：code/71 完成记录越权风险点 1、2；[ADR-0282](../../../docs/adr/0282-a-control-release-the-provider-refuses-is-registered-for-handling-instead-of-compensated.md) 越权风险点 1、2、4。

## 现象

- 释放被结算确定不受理时，撤回、主动拒绝、授权处置、自动拒绝与停进`等待受控补充`各路把那一笔记进释放未受理册（ADR-0282 决定二），之后没有人看得见它：册上只有按委托的读口，没有按租户列未处理行的查阅面、没有计数或告警，也没有处理动作。
- 处理一笔要做的事今天都没有入口：核对之后在结算侧释放那一笔；或给施加时没存作用域的存量结果补登作用域、再按原关联重放释放；处理完把那一行标成已处理并留痕。ADR-0282 越权风险点 2 说的「补登作用域的运维路径」就是这一条。
- 接受链判新提交版本之前放被取代版本，遇到拒收响亮失败、落 `publish_failed`（ADR-0282 决定三）。信封耗尽失败预算后死信；人处理完那一笔之后，没有东西会再驱一次接受链，新版本一直停在`已提交`、判不了。

## 证据

- `ports.ControlReleaseRefusalRegister` 只有 `Record` 与 `ListByShipmentRequest`；`pspostgres.ControlReleaseRefusals` 同此。表 `parcel_shipment.control_release_refusal` 没有「已处理」的列。
- `cmd/parcel-api` 的端点登记处没有释放未受理册的查阅口或处理口。
- `application.ErrControlReleaseNotAccepted` 不在 `acceptanceChainUndecidedSentinels` 里（有意如此，见 ADR-0282 决定三），链上没有任何一类信封在拒收解开之后重驱这份委托。

## 三类

机制（查阅面、处理动作与续办触发的形状）。处理动作谁有权做走既有授权规则登记（租户取值，`PAR-COM-14` 一类），本票不定。

## 规模

M–L：一个运营查阅读口（按租户列未处理行、计数）、一到两个处理命令、迁移加处理状态列与处理留痕、续办信封与消费门。

## 最小的诚实修法

1. 查阅：按租户列未处理的行（委托、版本、控制关联、存下的作用域、原因、登记时刻），给计数；告警阈值是产品策略，给参考配置。
2. 处理动作（运营决定族）：
   - 核对后在结算侧释放：经 SA 的释放入口按原关联释放，答已收口才把那一行标成已处理；
   - 补登作用域并重放：给控制结果补存作用域（只对存量无作用域的那一笔开放），按原关联重放释放；
   - 标记已处理：记处理人、处理依据与时刻，只追加，不删行。
3. 续办：处理动作与「那一笔已收口」同笔交一封续办信封（照 ADR-0094 决定四「续办触发与格同笔落地」），接受链消费它、重驱那份委托的当前版本，让人处理完之后新版本能再判。

## 要裁的

- 处理动作的授权：用哪一类授权规则（运营决定族的哪一种），是否与授权处置共用一套。
- 「核对后在结算侧释放」要不要另立 SA 的人工释放入口，还是复用按原关联的释放编排。
- 续办信封是新立一类，还是复用既有的某一类（如「参数已登记」类）。

## 完成记录

分支 `claude-code95`，基于 main `891a6c349`（收尾 rebase 之后以回报里的最终 SHA 为准）。迁移 `parcel_shipment/0031`、`access_identity/0025`；新 ADR-0293；无新票。

### 做了什么

- **迁移与端口**：`control_release_refusal_handling`（只追加，主键与外键都是册行主键，做法封闭两值，处理人与依据必填；册加（租户，登记时刻）索引）；决定种类 `CONTROL_RELEASE_REFUSAL_HANDLING` 与 `access_identity/0025`（CHECK 取此前各份并集再加一词）；端口 `ControlReleaseRefusalHandlings`（追加加读回赢家）、`ControlReleaseRefusalHandlingView`、`ControlReleaseRefusalCatalogue`、`ControlReleaseRefusalHandledHandoff`；`pspostgres` 三只适配器（处理留痕一句 INSERT…SELECT 连册行与租户一并核对，败者读回赢家；查阅 NOT EXISTS 反连接，租户条件进每句）。
- **查阅**：`GET /control-release-refusals`（主数据与运营查阅读面，沿用 PS 操作者查阅读那一只 Intake 加一个方法）：按租户列未处理的行（委托、被拒收的版本、控制关联、客户账户、存下的作用域、原因、登记时刻）加全部未处理计数与 `truncated`；页大小 200 取自渠道产品策略、不采信请求；读不回 500 不伪装成空册。
- **处理动作**：`POST /control-release-refusal-handlings`（「运营决定」面的 `CONTROL_RELEASE_REFUSAL_HANDLING`）。`HandleControlReleaseRefusalHandler`：标记已处理只留痕；核对后在结算侧释放复用 `occupationRelease.releaseFormed`（不另立 SA 入口），先取作用域锁，提供方答已收口才留痕，可重试未形成与再被拒收（含译不动）什么也不写、把确定的格答给处理人；已处理与并发输掉的一方读回先到的那一份、不再放、不交第二封信封；册上没有这一笔答统一不可见（404 `SHIPMENT_REQUEST_NOT_VISIBLE`）。事务壳整笔包留痕、信封与结算侧释放，编排返回错误即回滚。
- **续办**：新立信封类型 `parcel-shipment.shipment-request.control-release-refusal-handled`（载荷是委托此刻的当前版本与成员清单），`OutboxControlReleaseRefusalHandledHandoff` 与处理留痕同事务；接受链又一扇消费门 `ControlReleaseRefusalHandledConsumer`，共用既有译码与折法；`cmd/parcel-dispatch` 的 `assemble.go` 多一只留痕读口与一扇门、`routes_acceptance_chain.go` 多一条路由。接受链开头 `releaseSuperseded` 对提供方又确定不受理的那一笔，按（租户、委托、版本、控制关联）逐段查处理留痕，已处理的放行、接着判新版本。
- **管理台**：导航「释放拒收处理」（挨着授权处置），`ControlReleaseRefusalsPage`（ReviewFlowTemplate：队列、详情、两个动作，依据即必填理由，标记已处理再拦一道确认），纯逻辑 `control-release-refusals.ts` 与 node:test。`api.ts` 只加不重排。
- **文档与种子**：ADR-0293；ADR-0282 的 Status 注记与 Links 加前向指针；UC-PS-005 规则一条、`AT-PS-161` 至 `AT-PS-165` 五行与交接一行；PS CONTEXT 词条「控制项结果」加一句；盘点五行入①与一段追记；演示种子给配置员这一种授予（`steps/480`、`data/access/20`、`21`、`data/governance/58`），README 记一段。

### 代决

1. **立 ADR-0293。** 链开头对「人已处理」的拒收放行，收窄 ADR-0282 决定三「响亮上抛」一句，不能只挂读法注；同一条记录回答 ADR-0282 越权风险点 1、2、4，并记新立的决定种类与续办信封语义。
2. **续办信封新立一类，不复用。** 逐类核过：「委托已提交」的信封 ID 由来源身份认领，一份委托至多一封，发不了第二封；「复核已完成」「新提交版本已形成」的类型名说的是别的事，运维读 outbox 与死信时类型名是唯一线索，借用就是让它说假话；「参数已登记」类的消费门按租户取`等待运营登记`队列重驱，本场景的委托不在那个等待态。门里译码与折法共用，新增的只是一个类型串与一个 inbox 名。
3. **链开头必须认出人已处理的拒收。** 协调方令里没写，但缺了它「标记已处理」之后的续办仍撞同一个拒收、再落一次 `publish_failed`（没存作用域的存量行提供方每次都拒收）。提供方仍先被问，只在提供方又确定不受理而那一笔逐段对得上处理留痕时放行；没处理过的、别的版本或别的控制关联的拒收照旧响亮失败；留痕读不回按「读不动才是未决」停在本步内部重试（`ControlReleasePending`），不当成处理过也不当成没处理过。
4. **授权就是这一种授予加准入范围，不另问 `party-commercial`。** 授予名 `CONTROL_RELEASE_REFUSAL_HANDLING`，与授权处置（`AUTHORIZED_DISPOSITION`）分开；缺席 403 `OPERATOR_NOT_GRANTED`，渠道未配置 403 `ACCESS_CHANNEL_NOT_CONFIGURED`。查阅挂主数据与运营查阅读，不要这一种授予。
5. **依据两种动作都必填。** 核对后释放也要说核对了什么，事后读册的人才分得出例行放掉与出过差错。
6. **回执不回显处理人**，与别的运营决定口一条纪律；谁处理的只在留痕里（`发行方#sub`）。
7. **册上没有这一笔复用 `SHIPMENT_REQUEST_NOT_VISIBLE`。** 统一不可见在本上下文只有一个传输层写法；别的租户的委托标识在寻址那一步就与越权探针同答 403。
8. **告警阈值不做**（`grep -rniE 'alert|threshold' internal cmd --include=*.go -l` 命中的是计价、路由、治理里的业务阈值与事件文案，没有告警引擎或可复用的告警模式）：页头与响应只给计数与截断标志。
9. **补登作用域并重放不做**（协调方裁定）；存量无作用域行的「核对后在结算侧释放」恒再被拒收，页面与文档如实说，只剩先在结算侧手工放掉再标记已处理。
10. **标记已处理在管理台再拦一道确认**（只留痕、不能撤销、会让接受链此后不再为这一笔停下）。
11. **分区主体登记表（`internal/architecture/partition_subject_registry_test.go`）补一行**，同提交信封的（租户/客户账户/委托）；这是新信封口的架构门禁要求，不是行为改动。

### 越权风险点

1. **标记已处理是人的断言，本方不核实。** 标记了而钱实际还占着，接受链放行，新版本的控制看到被旧占用压着的余额——受限即可能按策略拒单，拒绝不可覆盖（ADR-0282 否决「遇拒收也记册、再按未决入账」的同一理由）。缓解只有依据必填、留痕可审、确认弹层与授予分开。
2. **补登作用域并重放不做**：首发后若生产出现无作用域的结果（按 ADR-0282 决定四不应出现），要另立运维路径。
3. **没有告警，只有计数**：积压仍靠人去看页；阈值与告警渠道是产品策略。
4. **链开头的拒收不记册，被取代版本若不是经「停进等待受控补充」换代，册上没有对应的行**：从别的暂停态被取代、从未走过记册的释放那一版，链响亮失败时册上没有这一笔，处理面看不到它，死信本身仍是唯一线索。本票没有为它立登记动作；要立须先裁「链开头那一步该不该记册」（ADR-0282 决定三否决过，理由是整轮回滚会撤掉记下的）。
5. **`cmd/parcel-dispatch` 的 `routes_acceptance_chain.go` 与 `assemble.go` 有改动。** 前者不是被禁的 `routes.go`，是接受链几扇门的路由登记文件，加一条路由；后者按令只动既有消费者构造（一只留痕读口、一扇门）。链装配上的留痕读口没有行为测试（真链往返用例用测试夹具同形装配），漏接时退回响亮失败，响亮但不在装配处响亮（同 ADR-0290 越权风险点 4）。
6. **测试夹具 `customer_supplement_resume_loop_test.go` 为让链看得见留痕读口加了一个依赖**，别的闭环用例不受影响（全仓测试绿）。
7. **并发处理的败者在主键上等先到的事务**；释放跑在提供方账本上持锁久时，后到者跟着等。
8. **标记已处理之后账本被另行占用、或「在结算侧释放」之后又被改动，链不再核对**（处理的认定只看留痕）。
9. **管理台页的 Intake 全在服务端把关**，前端不自判谁有权；演示库里只有配置员一位持这一种授予。

### 测试与变异

- 先红：HTTP 与应用层、消费门用例在实现之前编译不过；授予往返在迁移之前被库内 CHECK 拒（23514）；真链往返的「标记已处理」闭环在过滤去掉时红（见变异）。
- `scripts/test-full.sh ./...` 全仓 158 个包全绿、退 0；`go build ./...`、`go vet ./...`、`gofmt -l` 干净；管理台 `tsc -b --noEmit`、`node scripts/run-tests.mjs`（rebase 之后 786 过）、`vite build` 都过，`./internal/architecture/...`（含 `TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable`）过；`go run ./tools/adr-index` 能生成（README 不提交）、`go run ./tools/at-coverage-counts -check` 退 1（只是总览过期，新号分桶行已补）；演示种子在一次性库灌入零报错、三笔 `RECORDED`。新用例 `-count=3` 稳定，两条并发用例 `-count=5` 稳定。
- 变异（备份、还原后 `cmp` 一致），全部被杀：①处理后不交信封（应用层三例红）；②释放未收口也标处理（应用层不收口三格例红）；③并发输家也交信封（`TestASecondHandlerReadsTheFirstAndTriggersNothing` 红）；④不核册上有无（不可见三例红）；⑤已处理的一笔仍去问提供方（同上红）；⑥释放先于取锁（调用序例红）；⑦两人并发都成立（处理留痕 `ON CONFLICT DO NOTHING` 换成 `DO UPDATE`：适配器层「只一笔成立」与「第二次读回第一次」两例红）；⑧不核租户（查阅去掉租户条件：查阅隔离例红；处理的 INSERT…SELECT 去掉租户条件：他租户处理例红；Intake 采信请求里的 `?tenant=`：Intake 例与生产装配例红）；⑨链开头不滤已处理、把读不动当已处理、键里漏租户或漏版本、读口 nil 时也放行（链开头六例与既有响亮失败例分别红）；⑩链开头过滤整个关掉（真链往返「标记已处理」例红，错误正文就是被已处理的拒收挡着的 `ErrControlReleaseNotAccepted`）。
