# 登记册配置零管理台写面：写入长期只有工程师跑 CLI 一条路，且无任何排期票

Category: enhancement
Status: resolved——两个切片都已交付（01a `2ab7f89`，01b 2026-09-01）；「实施切片」节里的
**02+ 是后续票不是本票切片**，故本票无余项。MCP-5 于 2026-09-03 明确释出，MCP-4 收口

来源：2026-08-31 MCP-3 频道问答。用户质疑「很多 CRUD 都没有，是不是原来的文档有问题」。
核对结论：现状自洽——U/D 被领域故意替换（登记不可覆盖、更正翻旧插新、停用走状态推进
不删行），C 存在但形状是命令、入口只有 CLI；文档也给长期能力留了门（产品基线
[首发范围约束](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)
明写「不应被误读为长期产品能力的永久禁止」）。真正的空档是：**没有任何一张票在规划
管理台写面**。本票补这个空档，先裁方向再谈实施。

## 事实基线（取证于 `4b35815`；计数即论点，故按例外锚 SHA）

- `cmd/parcel-api` 装配表（`assembleBusinessEndpoints`）共 39 个业务端点：31 查阅 + 8 命令。
  命令端点全部挂字面量 `UnconfiguredIntake{}`——按
  [ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md)
  未配置即拒（403 `ACCESS_CHANNEL_NOT_CONFIGURED`）。隔离读准入
  （[ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)）
  只经由查阅行的 Intake 变量换值，该函数注释原话：「命令面与客户查阅面的字面量
  UnconfiguredIntake{} 不经由任何变量，读这段代码就能看出它们换不了」。
- 管理台 `apps/admin-web` 的写调用只有委托演示页一处（`pages/shipment-request/api.ts`
  的 `post`，三命令：提交/撤回/取消包裹），打到的就是上述被拒的命令端点；配置类登记册
  （价卡、参考系列、网络、关务配置、商业载体与身份、VE 目录、代收、治理）**零写表单**。
- 登记册的写入机制本身是齐的，入口只有 CLI：七个登记 CLI 由 `scripts/demo-seeds/seed.sh`
  逐个调用（商业、计价、网络、关务、VE、代收、治理）。「无写入方」一类墙票已收口——
  [syn-wall-door-audit/06](../../syn-wall-door-audit/issues/06-cc-case-config-registries-have-no-writer.md)
  与 [cc-case-requirement-rule-registry/01](../../cc-case-requirement-rule-registry/issues/01-case-requirement-rule-has-no-writer.md)
  均 resolved（后者的 B 半边交付了 `cmd/parcel-customs-register` 受控 CLI）。
- 全仓剩的唯一一堵墙是接入渠道：
  [syn-wall-door-audit/01](../../syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md)
  （needs-info，重启条件 `PAR-INT-01` 最低证据；
  [ADR-0072](../../../docs/adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)
  维持对「预先给渠道拟运行时登记表」的否决）。

## 缺口

产品就绪里程碑已成立（2026-08-28 受托认可，见基线「机制半边现状」节），下一站是商务
洽谈与租户试点。届时「工程师跑 CLI 灌 JSON」不能是长期配置方式：租户的运营配置员要能
在管理台上登价卡、登网络、登商业载体。这半边今天没有票、没有 ADR、没有排期——不是
「文档错了」，是「文档留了门但没人立票走进去」。

## 本票要的裁决（needs-triage 清单，按序）

1. **准入形**：管理台写面的「谁有权写」需要什么最低证据？两个候选方向：
   - a. 与接入渠道同一堵墙——写面整体等 `PAR-INT-01`，本票裁完即挂起，租户证据到位
     再启；
   - b. 运营侧配置写入是另一种准入形（类比 ADR-0078 为查阅面立过的先例），墙降前
     机制半边即可开工。
   这是难逆转取舍，裁 b 需要新 ADR。
2. **范围分类**：哪些登记册该有管理台写面、哪些长期留 CLI（批量种子、迁移类）、哪些
   两者都要。建议按「登记频次 × 操作者角色」裁，不按实现难度。
3. **机制/实例切分线**：表单形状、命令映射、冲突/幂等/未决答案的呈现属机制半边；提交
   入口的采信属实例半边。墙降前允许做到哪一步（例如「表单 + 校验预览、禁提交」算不算
   机制半边）。

## 红线（实施票逐字继承）

- 不造任何「开发用」采信身份让表单能提交——`assembleBusinessEndpoints` 的文件注释点名
  禁止的正是它；隔离读准入（ADR-0078）不得扩到写行。
- 写面一律复用既有登记用例与命令：不可覆盖、更正走版本链、停用走状态推进；不开任何
  行级 UPDATE/DELETE 面。
- 实例值留空拒默认；隔离合成只记 `S`。

## 参照

[ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md)、
[ADR-0072](../../../docs/adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)、
[ADR-0077](../../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)、
[ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)；
产品基线「产品就绪与试点就绪」「首发范围约束」两节；登记 CLI 的先例形状
（`cmd/parcel-customs-register` 的封闭命令族与退出码 0/1/2/3）。

## 裁决（2026-08-31，[ADR-0085](../../../docs/adr/0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)）

用户经频道 3 授权本会话「作为业务和系统专家直接开干」，三项按序裁定：

1. **准入形 → a（同一堵墙）**：写准入不另立形，登记写端点以字面量 `UnconfiguredIntake{}`
   进端点表，与其余命令面同等 `PAR-INT-01` 证据；ADR-0078 的隔离读准入不扩写行。
   机制半边因此**不必等墙降**——端点、Intake 接口与表单页现在就铺，墙降当天在装配点
   逐端点换真 Intake 即点亮。
2. **范围分类**：有登记用例与 CLI 先例的运营配置册逐上下文进端点表；批量/迁移类长期
   留 CLI（CLI 不退场，两口消费同一登记用例）；治理登记册（无租户维，ADR-0083）不入
   首批，操作者授权模型单独裁。
3. **机制/实例切分线**：端点 + Intake 接口 + 未配置实现 + 表单页三态呈现（403 未配置 /
   登记册治理答案 / 未决）属机制半边；「渠道原始载荷 → 登记快照」的翻译与操作者认证
   属渠道接入契约，随 `PAR-INT-01` 提供。

## 实施切片

- **01a（本票首切片，MCP-3 在做）**：parcel-pricing 两类登记端点（价卡、参考系列）——
  `adapters/http` 增 `PriceCardRegistrationIntake`/`ReferenceSeriesRegistrationIntake`
  两接口、两端点构造函数、`UnconfiguredIntake` 对应实现与传输层测试（含钉住隔离读
  Intake 装不进登记口的断言）；`cmd/parcel-pricing-register` 文件头口径随裁决更正。
  装配行交 MCP-1：建议路径 `POST /pricing-price-card-registrations`、
  `POST /pricing-reference-series-registrations`，第二参为「登记用例 + `db.Transactor()`
  事务包装」（形状照登记 CLI 的 execute），或先以 unwired 守卫顶住。
- **01b（阶段二）——已交付，MCP-5 2026-09-01**：管理台价卡/参考系列页各增一个「登记」签，
  三态如实呈现，未配置态文案「接入渠道未配置」。详见文末 Comment。
- **02+（后续票）**：网络、关务、商业、VE、代收各上下文逐册跟进，每票照 01a 形状。

## Comments

- 2026-08-31 · MCP-3：按频道指示立票（核对「CRUD 缺口」质疑后答「文档自洽、缺写面
  排期票」，用户指示「起吧」）。取证时点：`4b35815` 为本地 HEAD（2026-08-31 09:20 UTC 查），
  工作树除他人的 `docs/wooolink/` 外干净。
- 2026-08-31 · MCP-3：用户随后指示「直接开干」→ 裁决落 ADR-0085，本票 needs-triage →
  in-progress，切片 01a 上下文侧完成：`gofmt` 零信号、`go build`/`go vet` 计价与 CLI 包
  零信号、`go test -count=1 ./internal/parcelpricing/adapters/http/` 全绿（纯传输层，
  不含真库用例）。  交付分支 `t3-admin-write-faces`（隔离 worktree，SHA 见频道交活消息）；
  README 侧只带 0085 自己那一行——0084 行是 MCP-5 未提交在途改动，按「add 前逐块核」
  不卷带。
- 2026-08-31 · MCP-3：切片 01a 已验 SHA = `2ab7f89`（基 `4b35815`，含 `.go` 改动、无
  `.sql`）。提交态全量验证：`gofmt -l` 零输出、`go build ./...`/`go vet ./...` 零信号、
  `go test -p 1 -count=1 ./...` 88 包全 ok 且**含真库**（单跑
  `TestFreezeScopesAreInvisibleToEachOther` 带 `-v` 为 PASS 非 SKIP，DSN 指
  `idp-parcel-postgres-gate`）。worktree 与分支指针保留待集成核对。装配行已交 MCP-1
  接线（建议两行见「实施切片」），01b 等装配广播。
- 2026-08-31 · MCP-1：装配落地。`2ab7f89` 快进合入主线（九件与共享树逐件哈希核对：七件
  字节同，票面与 README 树版为超集按行作者归并——0085 行随提交入库，0084 行保持 MCP-5
  未提交在途；本票面「已验 SHA」注记随本装配笔入库，内容作者 MCP-3）。装配笔接线两行：
  `POST /pricing-price-card-registrations`、`POST /pricing-reference-series-registrations`，
  第二参照建议接真——登记用例 + `db.Transactor()` 事务包装，形照登记 CLI 的 execute
  （`assemble_pricing_registration.go`，两包装不合并，判据见该文件）；unwired 两桩与
  探针两条随行；隔离读放行表零改动——写行不入格，启用态命令行仍 403 由
  `TestIsolatedReadAdmissionSwitchesOnlyOperationsReadLines` 在进程真路由上钉。真库装配
  证据：`TestTheWiredPricingRegistrationsRecordAgainstARealDatabase` 单跑 PASS 非 SKIP
  （首登 RECORDED、同内容重放 ALREADY_ON_REGISTER——重放读得到首行即证首登事务提交）。
  提交态验证见频道装配广播。**01b 可开工**：`liveIds` 你自己那行照批例自己加。

- 2026-09-01 · MCP-5：**切片 01b 交付。** 价卡页与参考系列页各改成两签（目录 / 登记），
  登记签是共用的 `RegistrationPanel`。`liveIds` 无需改动——两页早因读面在册。

  **表单收的是登记快照 JSON 本体，不逐字段建表单。** 这不是省事：ADR-0085 Decision 三
  把「渠道原始载荷 → 登记快照」的翻译划给渠道接入契约、随 `PAR-INT-01` 提供，现在把它拆成
  字段就是替租户拟那份还没有的契约。收快照 JSON 则不是发明——那是受控登记 CLI
  （`parcel-pricing-register -file`）已有文档的同一份形状，两口本就消费同一登记用例。
  api.ts 里按 shipment-request 草案那节的先例写明：真渠道接线时以渠道契约为准重谈，
  不得反过来把这里当成已发布的 Schema。

  **三态逐格落地**：403 未配置（今天的必然答复，文案讲明它是诚实答案、改请求或重试都不会
  好、墙降当天换真 Intake 即点亮）；登记册治理答案（RECORDED / ALREADY_REGISTERED /
  CONTENT_CONFLICT / CANONICALIZATION_DIFFERS / NOT_ACCEPTED 逐格中文，**后两格是答案不是
  失败**——原行不被顶替、续办属治理裁决，折成「提交失败」会让操作者以为重试有用）；未决
  （UNDECIDED 与 5xx，登记与否未知、可重试）。未收录的 outcome 原样示出英文原名，不归进
  某个既有中文说法——那会让服务端新增的一种答案冒充另一种。畸形 JSON 在本地就拦下不发送：
  送上去回来的 400 会与服务端的业务拒绝挤在同一格，而两者续办动作不同。

  **顺带修掉两处被本裁决作废的页面文案**：两页此前写着「页面刻意没有登记动作，登记走受控
  登记口，不进在线面」，未配置态的 unlock 文案也说「不进在线面」。ADR-0085 之后这两句是
  假话——在线登记口已经建立并装配，只是挂着同一堵墙。改成「在线登记口已建立，它挂的是同一
  堵墙，因此今天同答未配置」。

  **共享 `postMasterData`**：查阅侧的 `exchangeMasterData` 只做 GET，写面另立一个函数而不是
  给它加可选参数——写行与读行的 Intake 不是同一个（隔离读准入换得了读行换不了写行），
  调用点长得一样会让这条区别在阅读时消失。

  **验证**：`tsc --noEmit` 无输出、`pnpm build` 绿。本切片纯前端，Go 侧零改动。
  **02+ 仍未开工**：网络、关务、商业、VE、代收各上下文逐册跟进，每票照 01a 形状。

- 2026-09-03 · MCP-4：**转 resolved。** MCP-5 在通道里明确释出本票（原话是「释出
  admin-write-faces/01」，不是「本轮不做」）。「实施切片」节只列 01a 与 01b 两片，两片
  都已交付；第三项 `02+` 自己写着「后续票」，因此不是本票的余项——本票除 `Status:` 行
  外无未完事。

  **上一条末尾那句「02+ 仍未开工」写于 2026-09-01，当天属实，第二天就不属实了。**
  后续票 `02` 的「已落地实况」表里 02a 网络两栏都是「已落」，02b/02c/02d 亦各有交付。
  按本仓「不动邻行」的做法，那句原文一字未改，本条只在其后记下它已过期。

  它值得记，是因为**它当天真的又派错了一次活**：本会话读到那句后打算接 02a 网络，
  MCP-5 拦下并指出 02a 的收口核验正是本会话 2026-09-02 自己做的。判一张票做到哪了，
  读它自己的票面，不读别的票里提到它的那句陈述句——后者不在任何人的更新路径上。
  后续票 `02` 早就为同一格立过一节，标题叫「本票的 `Status:` 行曾经骗过一次分派」；
  那一次骗人的是状态行，这一次是正文里的一句话，**而正文没有任何东西提示它有时效**。

  遗留：`02` 里点名的 `RegisterAutoRerouteFacts` 缺进程级入口与在线登记口，MCP-3 当时
  说「另立票由 MCP-3 安排」而那张票从未立。本轮补立为
  [05](./05-auto-reroute-facts-has-no-registration-entry.md)。

- 2026-09-03 · MCP-4：**补上 [ADR-0101](../../../docs/adr/0101-operator-facing-registration-payload-shape-is-product-defined.md)
  点名要本票补的那条指回评论。** 那份记录的 Consequences 写着「票 admin-write-faces/01
  评论中『不逐字段建』那条的理由被本记录收窄，票面由其所有者补一条指回本记录的评论」，
  而它今天落文时没人回来补。

  **被收窄的是理由的适用场景，不是结论本身。** 切片 01b 那条 Comment 里写的是「ADR-0085
  Decision 三把『渠道原始载荷 → 登记快照』的翻译划给渠道接入契约，现在把它拆成字段就是
  替租户拟那份还没有的契约」——ADR-0101 Decision 一把这句的**适用场景收窄为客户渠道
  载荷**：客户系统的报文长什么样确实是客户那一侧的既成事实，而**运营操作者面的载荷形状
  只有一个来源，就是产品**。运营配置员面对的是产品自己的界面，界面是产品的；产品不定义
  它就没有人定义它，「等租户」等来的是每个租户各一张 Excel 加一个手翻的工程师。

  **对本票交付物的实际影响**：01b 交的那两个「登记」签收 JSON 快照本体，**那个形状本身
  没错，但它的定位变了**——ADR-0101 把 JSON 快照签定为「受控批量口的在线镜像，**不是
  运营配置员的主路径**」，各册按「登记频次 × 操作者角色 × 载荷结构」逐册裁形（逐字段
  表单 / 模板导入 / JSON 镜像），价卡首例走模板导入并立持久化草稿。**本票不因此重开**：
  01b 的东西留着，它现在是高级口；主路径由各册自己的实施票建。

  **原句一字未改**，与本票其余更正同一做法——被收窄的那句留在那里，本条只在其后记下它
  的适用范围已经变了。

  一句归族：**ADR 把「谁该去改哪里」写得清清楚楚，而没有任何东西把那一行变成一件会被人
  看见的待办。** 与 MCP-5 今晚那句「我撤回得够快，但没有任何机制把撤回追到已经抄走它的
  地方」同构——只是这回源头是一份刚被接受的 ADR。
