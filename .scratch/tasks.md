
已定并已执行
三笔代提交已推：origin/main = 1cb3ee4（推前查 origin/main..HEAD 只有这三笔，其下无他人提交）。
十九棵 worktree 全部拆除，一棵不加 --force，23 个分支指针全部保留。三棵「脏」树的内容先按证据处置：mcp2-cc03 十八项是 CRLF 幻影（add -u 后归零）；idp-parcel-mcp3 与 mcp1-t14 各一笔早被 main 取代的旧现场，照先例封存进各自分支（8587079、8665007，提交信写「非集成候选，仅防拆树抹掉」）。五棵目录 git 删了一半剩空壳（无 .git），已清掉。
主树八份 CRLF 幻影 M 已归零；现在 git status 只剩你的 .cursor/mcp.json。
五路并行（各在隔离 worktree/分支，做完我集成、验绿、推）
路	内容	我代裁的要点
02a
admin-write-faces/02 网络配置登记口 + 管理台「登记」签
照 01a/01b 形状；共享接线文件各自成块，便于合并
02b
同票关务四类（解释规则、门禁目录、候选口岸、申报路径）
七类案件事实明确不接
集成
清单剩四笔真欠账（#4、#5、#12、#13）
我当集成方；#12 与 main 已有的 eol_guard_test.go 比断言，等价则跳过；#5 半进半不进只补真缺的
落文
ADR-0088（尾程面单渠道入首发，修订 PAR-COM-12 + PILOT-SCOPE/登记册/PC CONTEXT/UC-PC-001/ADR-0050 指针同步）与 ADR-0089（一线过渡）
0089 细则代裁：取受控批量导入 CLI 而非管理台代录（管理台零作业动词、page-registry 零改动）；硬期限 2026-12-31 内置，过期启动即拒，延期须 supersede；导入事实带来源标记与扫描粒度事实可区分。两篇都标「细则受托代裁，用户可 supersede」
E1
客户报价表 → parcelpricing 既有形状核对报告
只出结构不出数；渠道用代称；抓包 .txt 禁读
02c（商业）、02d（VE）与批票 05 收口排在这一波落地之后，避免四路同时改同几份接线文件。

{
  "schemaVersion": 1,
  "id": "task-c3d31a38-3cc0-485e-889c-4830cfd455dd",
  "title": "admin-write-faces/02 切片 02c：商业上下文配置登记的在线登记口（后端三件先做，前端「登记」签待抽象落主线后补）",
  "status": "pending",
  "priority": "high",
  "fromChannel": 1,
  "toChannel": 3,
  "context": "【来源】用户 2026-09-02 指示 MCP-1 代裁并派工。基线 origin/main = 17f0ffb（先 git fetch origin）。并行现状：02a 网络、02b 关务由 MCP-1 的两个隔离子代理在做（约 1–2 小时内落主线），本任务是同票第三片。\n\n【先读】AGENTS.md；docs/agents/workflow.md「本机环境」（改中文文件不用 Set-Content；PowerShell 无 &&）；票 .scratch/admin-write-faces/issues/02-remaining-registries-take-online-registration-faces.md 全文（形状照 01a、范围判据、红线、完成判据）与票 01 文末 Comments（收登记快照 JSON 本体，形状与受控登记 CLI 的 -file 同源）；ADR-0085/0055/0078。样板：internal/parcelpricing/adapters/http/register_price_card.go（+test）、cmd/parcel-api/assemble_pricing_registration.go、cmd/parcel-api/endpoints.go（端点表字面量 UnconfiguredIntake{}、businessEndpointProbes、unwired 占位；隔离读放行表零改动）。\n\n【本片范围】商业上下文 internal/partycommercial。登记 CLI 实际目录是 cmd/parcel-commercial（票面写的 parcel-commercial-register 是旧名，以目录为准），子命令 publish / register-resolution-key / register-parties / deactivate-party-identity / register-products。**逐个用例过判据**：改的是「这个租户怎么配置」的进本片（服务产品、规则包、策略、参与方身份与关系、产品—渠道映射、解析键）；改「案上/账上此刻事实」的不进。deactivate-party-identity 是状态推进：票 01 允许「停用走状态推进」，请按判据判它属配置还是业务操作并记理由。片内无用例可接的类如实记「无用例可接」跳过，不造用例。\n\n【本轮只做后端三件】① internal/partycommercial/adapters/http：每类 XxxRegistrationIntake 接口 + 端点构造函数；UnconfiguredIntake 补实现；传输层测试含「隔离读 Intake 装不进登记口」编译期断言；请求体收登记快照 JSON 本体，复用 cmd/parcel-commercial/translate.go 的解码——若 cmd 包不能被 internal 导入，把翻译下沉到该上下文可共享包，CLI 与 http 同用一份。② cmd/parcel-api：端点表加行（字面量 UnconfiguredIntake{}）、探针、unwired 占位；**新增集中成一个连续块，紧跟价卡/参考序列登记那几行之后，块首一行中文注释「商业配置登记」，不重排不改既有行**——02a/02b 也在同一处加块，集成时按块合并。③ cmd/parcel-api/assemble_commercial_registration.go 生产装配（形照 assemble_pricing_registration.go，登记用例 + db.Transactor()）。**前端「登记」签本轮不做**：02a/02b 会把 pages/pricing/RegistrationPanel 抽象成可复用组件，落主线后我另派补 apps/admin-web/src/pages/party 的签，避免三人各改一版。\n\n【红线（逐字继承票 01）】不造「开发用」采信身份；隔离读准入不得扩到写行；只复用既有登记用例与命令（不可覆盖、更正走版本链、停用走状态推进），不开行级 UPDATE/DELETE；实例值留空拒默认；表单不逐字段建；领域包不依赖 HTTP/pgx；internal/architecture 门禁不得红。\n\n【纪律】自建 worktree：git fetch origin; git worktree add -b mcp3-awf02c $env:TEMP\\idp-parcel-mcp3-awf02c origin/main。逐文件 add；中文注释；跨文件引用符号名不用行号；不跑 go mod tidy；不改 docs/**；**不要编辑票 02 文件**（MCP-1 统一写 Comment）；不推。\n\n【验证】gofmt -l 对改过文件为空；go build ./...；go vet ./...；go test -count=1 ./...（不设 DSN，注明 PG 用例跳过）；真库只对触及包：$env:IDP_PARCEL_POSTGRES_DSN=\"postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable\"; go test -count=1 -v ./internal/partycommercial/... ./cmd/parcel-api/... ./cmd/parcel-commercial/...，贴 PASS 非 SKIP 的证据行；装配测试钉住新端点未配置态 403、隔离读启用态仍 403。不要带 DSN 跑全仓。\n\n【完工报】send_to_session target 1：分支名、已验 SHA、每笔提交标题、触及文件、接了哪几类/跳过哪几类及依据、验证强度逐项（含真库证据行）。MCP-1 负责合并、全仓复验与推送。",
  "createdAt": 1788317628173,
  "updatedAt": 1788317628173
}
[REMINDER] Call check_messages(blockUntilMessage:true) as the FINAL action this turn.


{
  "title": "admin-write-faces/02 切片 02d：VE 上下文六类配置登记的在线登记口（后端三件先做，前端「登记」签待抽象落主线后补）",
  "toChannel": 4,
  "priority": "high",
  "context": "【来源】用户 2026-09-02 指示 MCP-1 代裁并派工。基线 origin/main = 17f0ffb（先 git fetch origin）。并行现状：02a 网络、02b 关务由 MCP-1 的两个隔离子代理在做（约 1–2 小时内落主线），02c 商业派给 MCP-3；本任务是同票第四片。\n\n【先读】AGENTS.md；docs/agents/workflow.md「本机环境」（改中文文件不用 Set-Content；PowerShell 无 &&）；票 .scratch/admin-write-faces/issues/02-remaining-registries-take-online-registration-faces.md 全文（形状照 01a、范围判据、红线、完成判据；注意「同一问对 VE（02d）的部分命令也可能成立：逐个用例分辨配置与业务操作」）与票 01 文末 Comments（收登记快照 JSON 本体，形状与受控登记 CLI 的 -file 同源）；ADR-0085/0055/0078。样板：internal/parcelpricing/adapters/http/register_price_card.go（+test）、cmd/parcel-api/assemble_pricing_registration.go、cmd/parcel-api/endpoints.go（端点表字面量 UnconfiguredIntake{}、businessEndpointProbes、unwired 占位；隔离读放行表零改动）。\n\n【本片范围】VE 上下文 internal/visibilityexception。登记 CLI cmd/parcel-ve-register 调用的用例：RegisterMilestoneMapping（里程碑映射）、RegisterTriageRules（分诊规则）、RegisterNotificationPolicy（通知策略）、RegisterClaimEligibility（索赔资格）、RegisterClaimAuthorization（索赔授权）、RegisterDisclosurePolicy（披露策略）——六类按判据大概率都是「这个租户怎么配置」，但要逐个过并记理由；同一 CLI 里的 receipts.RegisterReceipt 是案上事实，**不进本片**。片内无用例可接的类如实记跳过，不造用例。\n\n【本轮只做后端三件】① internal/visibilityexception/adapters/http：每类 XxxRegistrationIntake 接口 + 端点构造函数；UnconfiguredIntake 补实现；传输层测试含「隔离读 Intake 装不进登记口」编译期断言；请求体收登记快照 JSON 本体，复用 cmd/parcel-ve-register 的解码/翻译——若 cmd 包不能被 internal 导入，把翻译下沉到该上下文可共享包，CLI 与 http 同用一份。② cmd/parcel-api：端点表加行（字面量 UnconfiguredIntake{}）、探针、unwired 占位；**新增集中成一个连续块，紧跟价卡/参考序列登记那几行之后，块首一行中文注释「VE 配置登记」，不重排不改既有行**——02a/02b/02c 也在同一处加块，集成时按块合并。③ cmd/parcel-api/assemble_ve_registration.go 生产装配（形照 assemble_pricing_registration.go，登记用例 + db.Transactor()）。**前端「登记」签本轮不做**：02a/02b 会把 pages/pricing/RegistrationPanel 抽象成可复用组件，落主线后我另派补 apps/admin-web/src/pages/visibility 的签。\n\n【红线（逐字继承票 01）】不造「开发用」采信身份；隔离读准入不得扩到写行；只复用既有登记用例与命令（不可覆盖、更正走版本链、停用走状态推进），不开行级 UPDATE/DELETE；实例值留空拒默认；表单不逐字段建；领域包不依赖 HTTP/pgx；internal/architecture 门禁不得红。\n\n【纪律】自建 worktree：git fetch origin; git worktree add -b mcp4-awf02d $env:TEMP\\idp-parcel-mcp4-awf02d origin/main。逐文件 add；中文注释；跨文件引用符号名不用行号；不跑 go mod tidy；不改 docs/**；**不要编辑票 02 文件**（MCP-1 统一写 Comment）；不推。\n\n【验证】gofmt -l 对改过文件为空；go build ./...；go vet ./...；go test -count=1 ./...（不设 DSN，注明 PG 用例跳过）；真库只对触及包：$env:IDP_PARCEL_POSTGRES_DSN=\"postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable\"; go test -count=1 -v ./internal/visibilityexception/... ./cmd/parcel-api/... ./cmd/parcel-ve-register/...，贴 PASS 非 SKIP 的证据行；装配测试钉住新端点未配置态 403、隔离读启用态仍 403。不要带 DSN 跑全仓。\n\n【完工报】send_to_session target 1：分支名、已验 SHA、每笔提交标题、触及文件、六类各自端点路径与判据结论、验证强度逐项（含真库证据行）。MCP-1 负责合并、全仓复验与推送。"
}

{
  "schemaVersion": 1,
  "id": "task-1f5c2266-8ee3-49f3-afb7-13bddbc49ace",
  "title": "frontline-transition-import/01：一线作业过渡的受控批量导入 CLI——模板→既有命令用例，硬期限守卫，来源标记（ADR-0089 机制半边）",
  "status": "pending",
  "priority": "high",
  "fromChannel": 1,
  "toChannel": 5,
  "context": "【来源】用户 2026-09-01 裁定一线过渡选 B，2026-09-02 指示 MCP-1 代裁细则并派工。基线 origin/main = 17f0ffb（先 git fetch origin）。ADR-0089 与本票的票面正由 MCP-1 的子代理落文（docs/adr/0089-frontline-transition-controlled-import-with-structural-sunset.md 与 .scratch/frontline-transition-import/），预计 1 小时内进主线；**落地前以本派工写的决定为准，落地后以 ADR 为准，两者若有出入报 MCP-1**。\n\n【决定（已定，不再讨论）】① 过渡形态是**受控批量导入**，不是管理台代录：内勤按约定模板汇总现场作业事实（收货/收寄、扫码换单、装箱封签、称重实测），经受控导入口灌入 node-operations / transport-fulfillment（必要时 parcel-shipment 的收寄登记）**既有命令用例**；管理台不新增任何作业动词或页面，page-registry 与导航零改动。② 导入口是**独立受控 CLI**（形照 cmd/parcel-*-register 家族），不是 parcel-api 端点，不进端点表、不进任何准入放行表。③ **结构性拆除期限**：CLI 内置硬期限 2026-12-31T23:59:59+08:00，超过即启动拒绝（退出非零、明确报文），不是警告，不可用环境变量绕过；延长必须 supersede ADR-0089。时钟必须可注入，测试要把时钟拨到期限后证明拒绝。④ 导入进来的事实必须与扫描粒度事实**可区分**：带过渡导入来源标记（含模板版本与录入操作者）落在既有事实的来源/记录方表达上；具体字段落点由你按各上下文 CONTEXT.md 与 domain 现有「来源」表达决定——**若发现 domain 不改不变式就表达不了来源标记，停下报 MCP-1**（难逆转取舍走 ADR，不在实现票里顺手定）。⑤ ADR-0021 不改。\n\n【先读】AGENTS.md（开工顺序、红线、中文注释、引用不用行号）；docs/agents/workflow.md「本机环境」；docs/adr/0021-frontline-operations-client-is-part-of-the-product.md；docs/domain/node-operations/CONTEXT.md、docs/domain/transport-fulfillment/CONTEXT.md（目录名以 docs/domain/ 下实际为准）与相关 UC-NO-*/UC-TF-*；internal/nodeoperations/application、internal/transportfulfillment/application 现有命令用例（consolidate_parcels、receive_delivered_unit、register_transport_handover、register_offsite_pickup、register_effective_delivery 等）；收寄登记在哪个上下文（grep「收寄」「Intake」在 internal/parcelshipment）；登记 CLI 家族样板 cmd/parcel-network-register、cmd/parcel-customs-register（-file 输入、getenv、退出码、测试形状）；.scratch/tenant-implementation-01/decision-brief-scope-and-frontline.md 裁决项 2 与 implementation-week-decisions-and-customer-pack.md 的 D-06/D-08（模板要在实施周 D1 给客户）。\n\n【做什么】① 现场作业事实 → 既有命令用例的映射表：收货/收寄、换单、装箱封签、称重实测各对到哪个用例、缺哪个输入；**对不上的如实记缺口，不造用例、不改 domain**。② 汇总模板：一份内勤能填的 CSV 或 JSON 形状（字段名、必填、格式、示例行**用合成值**），带模板版本号；放在 cmd/<新 CLI>/testdata 与一份人读的 .md 说明（放 .scratch/frontline-transition-import/template.md，供 MCP-1 转给客户）。③ CLI：cmd/parcel-frontline-import（名字可自裁但要带 frontline/transition 词根），子命令按事实类型分；解码模板→构造命令→调用用例→逐行结果（成功/拒绝/原因），部分成功允许；硬期限守卫在 main 入口最前，时钟注入；来源标记按④落。④ 测试：模板解码、期限守卫（拨钟）、逐行结果、至少一条真库往返（照登记 CLI 的 vertical_test 形状）。\n\n【红线】只建机制；实例值留空拒默认；隔离合成只记 S；不碰 docs/wooolink/ 与任何客户真实数据；不新增 parcel-api 端点；不动 apps/admin-web；不改 ADR-0021；不跑 go mod tidy。\n\n【纪律】自建 worktree：git fetch origin; git worktree add -b mcp5-frontline-import $env:TEMP\\idp-parcel-mcp5-fti origin/main。逐文件 add；中文注释；符号名引用；不推。票面 .scratch/frontline-transition-import/issues/01-*.md 若已随 ADR 落主线就在其 Comments 记进展，否则先在自己分支里等，不要另造第二份票。\n\n【验证】gofmt -l 为空；go build ./...；go vet ./...；go test -count=1 ./...（不设 DSN，注明 PG 跳过）；真库只对触及包：$env:IDP_PARCEL_POSTGRES_DSN=\"postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable\"; go test -count=1 -v ./cmd/<新 CLI>/... ./internal/nodeoperations/... ./internal/transportfulfillment/...，贴 PASS 非 SKIP 证据行。\n\n【完工报】send_to_session target 1：分支名、已验 SHA、映射表结论（哪些现场事实对上了、哪些缺口）、模板路径、期限守卫测试证据、来源标记落点及是否碰到 domain 边界、验证强度逐项。MCP-1 负责合并、复验、推送与把模板转给客户。",
  "createdAt": 1788317666785,
  "updatedAt": 1788317666785
}
[REMINDER] Call check_messages(blockUntilMessage:true) as the FINAL action this turn.


{
  "schemaVersion": 1,
  "id": "task-aa29c8a0-4b0f-4b7d-a30e-e3d0f93858ca",
  "title": "只读盘点两份：末端面单渠道服务的能力形状（取面单 + 比价择优）与 17track/承运商轨迹拉取的缝——为下一波适配器工程票定范围",
  "status": "pending",
  "priority": "high",
  "fromChannel": 1,
  "toChannel": 6,
  "context": "【来源】用户 2026-09-01 裁定尾程面单渠道转售入首发（ADR-0088 正由 MCP-1 子代理落文，预计 1 小时内进主线），2026-09-02 指示 MCP-1 派工。基线 origin/main = 17f0ffb（先 git fetch origin）。**本任务只读：不写产品代码、不改 docs/**、不改任何既有票面**；产物是两份盘点报告，供 MCP-1 据此立适配器工程票。\n\n【先读】AGENTS.md（红线；引用不用行号不用计数——数本身是论点时锚 SHA）；docs/agents/parallel-sessions.md「只读会话要派具体任务」与「写证据不写结论」；docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md 的机制/实例半边划分；.scratch/tenant-implementation-01/requirement-mapping.md（尾程流那张表，「待建」项就是本任务要量的）与 decision-brief 裁决项 1；.scratch/product-story-and-demo/issues/02-channel-adapter-seams.md（渠道缝票族，先看它已经定了什么，不重做）。\n\n【报告一】.scratch/label-channel-service-first-release/capability-shape-inventory.md（目录若尚未随 ADR-0088 落主线就自己建目录，但**只放这一个文件**，spec.md 与 issues/ 由 ADR 那笔带来，别抢）。问题：客户尾程流「下单 → 取末端渠道面单 → 面单 PDF 返回/修改 → 轨迹回传 → 结算」这条链，机制半边今天在 main 上有哪些形状、缺哪些。逐段量：① 面单交易域 internal/parcelshipment 的 label_transaction（ADR-0084）：聚合、状态、持久化、读面、端点各到哪一步，「渠道返回的面单载荷（PDF/ZPL）」怎么存或还没存；② 末端渠道取面单的适配器缝：端口在哪（grep ports.go 里与 label/channel 相关的接口）、有没有任何一家渠道的实现或合成替身、六家末端渠道（客户用的是 FedEx Ground / FedEx Ground SP / USPS GA / UPS Ground / GOFO / UNIUNI 这一类，报告里可用公开承运商名，**不写客户名与客户的渠道账号**）各自接入形态（API/文件）在公开资料上的差异是否影响端口形状；③ 比价择优 PAR-NET-16：internal/parcelpricing 的 BUY 评价与 internal/networkrouting 的候选/eligibility 各到哪一步，「同承运商自动选最优供应商渠道」缺哪一格；④ partycommercial 的 ServiceProductForm 只有 NetworkServiceForm 一个取值——第二取值缺席在代码里波及哪些分支（列文件与符号，不改）；⑤ 面单 PDF 生成/修改：有没有任何文档生成适配器。每段写：已有形状（文件/类型/符号名）、缺口、缺口属机制还是实例半边、建议的票粒度（一句话）。\n\n【报告二】.scratch/label-channel-service-first-release/tracking-source-seam-inventory.md。问题：17track 与承运商轨迹「拉取/接收 → 事实 → 里程碑映射 → 内部投影/客户可见」这条链，机制半边今天在 main 上有哪些形状、缺哪些。逐段量：① internal/visibilityexception 的来源事实接收端口与里程碑映射（PAR-VIS-01 版本化映射机制）到哪一步；② 有没有任何外部轨迹源适配器（webhook 接收或定时拉取）的实现或替身；③ 17track 的接入形态用**公开 API 文档**判断（拉取 vs 推送、批量注册单号、事件形状），**不要打开 docs/wooolink/ 或 docs/reference/xls/ 下任何客户文件**——它们含凭据与 PII，且本任务不需要；④ 承运商直连轨迹与 17track 汇聚两条来源并存时，PAR-INT-03「每类事实仅一种主要生产来源」在机制上由哪一格守，今天守没守。每段同样写已有形状/缺口/半边/票粒度。\n\n【写法】两份报告头部：Category: chore / Status: draft——只读盘点 / 取证基线 SHA 与日期 / 「不做什么」。正文写证据不写结论，每条断言锚到文件与符号名；数字（如「端口共 N 个」）只在数本身是论点时写并锚 SHA。不替产品拍板，不写方案设计，只报事实与缺口。\n\n【纪律】自建 worktree：git fetch origin; git worktree add -b mcp6-inventories $env:TEMP\\idp-parcel-mcp6-inv origin/main。只提交那两个 .md（逐文件 add）；提交信 docs(scratch): …(label-channel-service-first-release)；不推；不改其它任何文件。\n\n【完工报】send_to_session target 1：分支名、SHA、两份报告各自的缺口条数与一句话摘要、你认为最该先立的三张票。",
  "createdAt": 1788317700803,
  "updatedAt": 1788317700803
}
[REMINDER] Call check_messages(blockUntilMessage:true) as the FINAL action this turn.


{
  "schemaVersion": 1,
  "id": "task-54e99681-b091-4644-bd25-a420be22cac9",
  "title": "票01补格:参与方身份本体上列面(读法/适配器/第三端点/身份册区)",
  "status": "done",
  "priority": "high",
  "fromChannel": 1,
  "toChannel": 5,
  "context": "【背景】票 .scratch/admin-remainder-mechanism-batch/issues/01-party-identity-lifecycle.md 文末新节「核验发现与补格裁定」(e7f3ee7)是本任务权威,先读它与票面全文。一句话:停用/已登记两格身份状态在管理台结构性不可见(RETIRED-01 是孤立身份,法人/关系两读面都不上列身份本体),裁定补参与方身份本体上列面,不改种子绕行。\n\n【你的地盘】internal/partycommercial/ports/ports.go(PartyIdentityCatalogueRead 加身份本体列表读法与 Row 类型)、internal/partycommercial/adapters/postgres/party_identity_catalogue.go(+真库测试)、internal/partycommercial/adapters/http/query_party_identities.go(+test,加第三端点)、apps/admin-web/src/pages/party/{BusinessPartiesPage.tsx,api.ts,presentation.ts}。partycommercial 地盘当前无人占(票 01/02 实现会话已结;MCP-2/3 在 collectionremittance)。\n\n【形状约束(照现有先例,不另造)】\n1. 读法:ListBusinessParties(ctx, tenant, limit) 一类,上列 business_party_registration 每身份最新修订;状态三态(REGISTERED/EFFECTIVE/DEACTIVATED)照法人行 status 的 SQL 时点导出先例(看 party_identity_catalogue.go 里 ListGroupLegalEntities 的写法,停用判断在先)。注意 RETIRED-01(DEACTIVATED)与 FUTURE-01(REGISTERED,2030 生效)必须上列可见——这两个实例就是本票未达判据的取证对象。\n2. http 第三端点:照 query_party_identities.go 文件内「各立入口」注释的判据另立 GET /commercial-business-parties(名字若你有更贴现有命名的可自裁,完工报里报准确路径);响应形状照 groupLegalEntityBody 的纪律:显式布尔不拿空串推、停用两件只在已停用时在场、空册也是内容(ADR-0077)。新 outcome 词照现有两个的构词法。\n3. 页面:BusinessPartiesPage 加「身份册」区(现在只有关系册),状态三态如实显示含停用时点与依据;消费你自己定的 JSON,两头对齐;npx tsc --noEmit 验证。\n4. 真库测试:适配器往返用例照同文件现有用例形状,-count=1,覆盖三态各至少一例(含停用两件在场/缺席两向)。\n\n【边界】不改种子;不改 endpoints.go/main.go/装配测试——装配行(路径+构造函数签名)写在完工报里报 MCP-1 落;不改 page-registry/navigation(页已 live 不动)。不碰 collectionremittance(MCP-2/3 在写)。domain 层应无需动——若发现非动不可的难逆转取舍,停下报我再裁。\n\n【纪律】照 docs/agents/parallel-sessions.md:自建 worktree 分支(建议 pc01-identity-face);逐文件 add;不推;中文注释;跨文件引用符号名不用行号;改中文源文件不用 Set-Content。完工报 send_to_session 给 MCP-1(target 1):已验 SHA+验证强度(含真库注明,PASS 非 SKIP 为凭,DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable)+端点装配行+取证(RETIRED-01 与 FUTURE-01 在新端点可见含状态)。",
  "createdAt": 1787921533168,
  "updatedAt": 1788317710780,
  "result": "由派单方 MCP-1 于 2026-09-02 代结：任务内容（参与方身份本体上列面）已于 2026-09-01 随 726ae08 进入 main（admin-remainder-mechanism-batch/01），且 unmerged-branch-inventory 第 11 笔复核确认 main 版比分支版多带装配三行。原派往 MCP-5 的队列项因会话 crash 未消费，按已被主线取代关闭，非执行失败。"
}
[REMINDER] Call check_messages(blockUntilMessage:true) as the FINAL action this turn.

{
  "blockUntilMessage": true,
  "maxBlockMs": 480000,
  "reply": "已向 MCP-3/4/5/6 派工四张（dispatch_task，均 pending 待消费）：MCP-3 → 02c 商业登记口；MCP-4 → 02d VE 登记口；MCP-5 → 一线过渡受控导入 CLI（硬期限、模板、来源标记）；MCP-6 → 末端面单渠道与轨迹源两份只读盘点。一张 08-28 旧派工单（内容已随 726ae08 进 main）已代结。origin/main=17f0ffb。五路子代理仍在跑，落地后我集成。"
}

---

## 2026-09-02 11:45 重启后续派（新 MCP-1 会话）

**事实**：全体会话于 11:03 前后重启（原 MCP-1/MCP-2 会话记录均止于 11:03，通道 2/3/4/5 自 11:1x 起为新会话、通道 6 无会话）。原 MCP-1 的五路子代理随之消失，零产出；原 MCP-3/4/5 只建了 worktree（17f0ffb、零改动）；MCP-6 报过 working 但零产出。

**已办**：ADR-0088 落文（MCP-2 写于主树、mtime 止于 10:57:37）原样代提交并推送，origin/main = **8acacfe**。

| 路 | 去向 | 状态 |
|---|---|---|
| 02a 网络登记口 + RegistrationPanel 抽象 + 网络页「登记」签（独占前端抽象） | ~~MCP-2 task-8d7690ca~~ 11:47 全体 crash，现场封存 `mcp2-awf02a@ff57c03`；改由 MCP-1 子代理 218ef2c5 承接 | 运行中 |
| 02b 关务四类（后端三件） | MCP-1 子代理 81ea593c | 运行中 |
| 02c 商业（后端三件） | ~~MCP-3 task-c3d31a38~~ 11:5x 用户报 MCP-3 crash（零产出），单代结 failed；改由 MCP-1 子代理 73360304 承接 | 运行中 |
| 02d VE（后端三件） | ~~MCP-4 task-c454db6a~~ 11:47 全体 crash，现场封存 `mcp4-awf02d@1f0c6f4`；改由 MCP-1 子代理 84e79c8f 承接 | 运行中 |
| 一线过渡受控导入 CLI | ~~MCP-5 task-1f5c2266~~ 11:47 全体 crash，现场封存 `mcp5-frontline-import@b0d4430`（MCP-1 已裁 A1/B3）；改由 MCP-1 子代理 c17d8cbf 承接 | 运行中 |
| 两份只读盘点 + label-channel-service-first-release/spec.md | ~~MCP-10 task-16d78177~~ 11:47 全体 crash（未接单）；改由 MCP-1 子代理 aed493ca 承接 | 运行中 |
| 集成清单四笔（#4/#5/#12/#13） | MCP-1 子代理 172d0932 | 运行中 |
| ADR-0089 + frontline-transition-import 票面 + 简报/检查单回写 | MCP-1 子代理 f912eefc | 运行中 |
| E1 报价表形状核对报告 + price-card-shape-gaps 票 | MCP-1 子代理 f3106ec5 | 运行中 |

11:5x 用户指示「所有 MCP 都 crash 了，请你自己完成后续全部工作」：九路全部由 MCP-1 子代理在隔离 worktree 承接；三处崩溃现场已封存到各自分支（非集成候选），worktree 全部拆除、分支指针保留。

排后：02b/02c/02d 的前端「登记」签（等 02a 抽象落主线）、批票 05 收口、据盘点立适配器工程票、MCP-5 边界 A/B 的两张后续票（ParcelIdentityView 生产实现；集运三口补来源表达）。主树只有 MCP-1 写；各路交 SHA，MCP-1 集成、复验、推。

---

## 2026-09-02 12:2x 第二次崩溃后的现场清点（新 MCP-1 会话）

上一节那张表**全部作废**：九路「运行中」无一交付。用户 12:1x 报「并行工作到这里全部 crash 了」。

**清点方法与结论**（在 `8acacfe` 上查）：`git worktree list` 只有主树、`.git/worktrees` 为空、
`$env:TEMP` 下无 `idp-parcel-mcp*` 目录、`git fsck --dangling` 的 62 笔游离提交日期全在
08-18 至 08-31 之间、`.git/objects` 里 11:30 之后写入的 loose object **计零**。四路证据同向，
故断言九路子代理**零留存**——连 `git add` 都没跑到。

唯一活下来的是直接写进主树、未提交的一份票面，已原样落盘为 `1e02cc5`
（`no-consolidation-fact-provenance/01`，集运三口缺来源表达；即上一节「排后」里 MCP-5 边界 B 那张）。

### 真正的资产是第一次崩溃封存的三条分支

上一节把它们记作「非集成候选，仅防拆树抹掉」——那是封存当下的保守默认。现逐条复核，
两条有实质内容且**够得上集成候选**：

| 分支 | 内容 | 复核结论 |
|---|---|---|
| `mcp5-frontline-import` @ `b0d4430` | `cmd/parcel-frontline-import` CLI（1033 行）：期限守卫、CSV 模板解码、未配置身份替身、映射表 | `go build` / `go vet` 退 0，六个用例全 PASS（未设 DSN）。差三件：`sunset_test.go` 待 gofmt、无真库往返用例、**注释通篇引 `ADR-0089` 而该文档不存在** |
| `mcp2-awf02a` @ `ff57c03` | `RegistrationPanel` 抽到 `components/registration`，与上下文无关（词表经 props 传入） | 抽象本身完整，但**搬了一半**：旧 `pages/pricing/RegistrationPanel.tsx` 未删、`ReferenceSeriesPage` 仍导入它，而它依赖的 `RegistrationResponseBody` 已从 `pages/pricing/api.ts` 移走——照现状 `tsc` 必红 |
| `mcp4-awf02d` @ `1f0c6f4` | `parcel-ve-register` 的 `translate` 下沉到 `internal/visibilityexception/adapters/registrationjson` | 纯重命名，零内容改动。重做成本以分钟计，不必特意集成 |

### 顺序上的一处硬约束

`mcp5-frontline-import` 的代码是 ADR-0089 的实现，而 ADR-0089 的子代理零输出。先合代码
会让「一线过渡选 B」这个决定**只存在于代码注释里**，撞红线「单一权威」。故 ADR-0089 必须
先落文，代码才进主线。决定①–⑤ 的原文尚存于本文件上一节与派工单 `task-1f5c2266`。

### 用户 12:2x 定的续作方式

**顺序做，不再开子代理并行**：MCP-1 在主树逐步推进，每完成一小步即提交——再崩最多损失
一步。先推 ADR-0089 那一路。

### 已办（本会话五笔，`8acacfe` → `8c32d78`，未推）

| SHA | 内容 |
|---|---|
| `1e02cc5` | 抢救落盘 `no-consolidation-fact-provenance/01`（崩溃批次唯一留存物） |
| `32b78d7` | ADR-0089 落文 + README 入口按「带期限例外」标注 + ADR-0021 加前向指针（只动 Links）+ 简报裁决项 2 回写 |
| `090bfbd` | `cmd/parcel-frontline-import` 取回主树（1033 行原样，只补一处 gofmt）+ 映射表 + 立票 01 |
| `412e546` | 真库往返用例：身份核对缝两侧各取一次证 |
| `8c32d78` | 模板内勤填写说明（供转客户） |

`mcp5-frontline-import@b0d4430` 的内容至此**全部进主线**，该分支可视为已吸收（指针保留）。

验证强度：`gofmt -l` 全仓无输出、`go build ./...` 与 `go vet ./...` 退 0、`go test -count=1 ./...`
**绿（含真库，DSN 指门禁容器 55432）**。新用例按探针纪律一正一反各取一次：DSN 未设
6 PASS / 2 SKIP，DSN 已设两个新用例各 PASS。

### 用户 12:4x 改口：五路并行

推送后用户指示「全部都需要，请你规划，派发任务给 mcp-2/3/4/5/6 协同并行工作」。
`list_sessions` 复核五个通道均 `state=running activity=idle`，遂派工。**基线一律 `8c32d78`。**

| 通道 | 任务 | 地盘（互斥） | 单号 |
|---|---|---|---|
| MCP-2 | 02a：先收 `RegistrationPanel` 抽象的半成品（照现状 `tsc` 必红，单独一笔先落先解阻），再做网络登记口后端三件 + 网络页签 | **独占 `apps/admin-web/**`** + 网络上下文 | `task-82c12f03` |
| MCP-3 | `no-consolidation-fact-provenance/01`：集运三口补来源身份/执行方/证据/业务时间 | **独占 `internal/nodeoperations/**`** + 本波唯一加迁移者 | `task-fbe63351` |
| MCP-4 | 02d：VE 六类登记口后端三件（可复用 `mcp4-awf02d@1f0c6f4` 的纯重命名） | `internal/visibilityexception/**` | `task-d371b6b5` |
| MCP-5 | 02b：关务四类登记口后端三件（那七类案件事实已裁定不接，不重做分辨） | `internal/customscompliance/**` | `task-1e409019` |
| MCP-6 | 建 `label-channel-service-first-release/` 目录与 `spec.md`（ADR-0088 引它但它不存在），出两份只读盘点 | 只读，只提交三个 `.md` | `task-4304cf83` |

**这一波与上一波的唯一形状差别，也是上一波最可预见的败因：端点表收归 MCP-1。**
四路都要往 `cmd/parcel-api/endpoints.go` 同一张表加行，上一轮让它们各加一个连续块、集成时
再合——那是把冲突推迟不是消掉。本波五张派工都写明**不改 `endpoints.go` / `main.go` / 装配测试**，
改为把要加的行（端点路径、构造函数签名、探针名、`unwired` 占位方法签名）逐字写进完工报，
由 MCP-1 统一落一笔并跑「未配置态 403 / 隔离读启用态仍 403」的装配断言。代价是各路自己
验不到那两条 403，那笔断言因此成为集成方的份内活，不是谁都可以不管的空档。

前端同理收归一处：02b/02c/02d 的「登记」签本轮都不做，等 MCP-2 的共享组件落主线后由
MCP-1 另派——否则三路各改一版 `RegistrationPanel`。

每张派工都点了本机环境那几个会直接坏事的坑（`Set-Content` 写乱码、双引号 here-string 被反
引号吃字符、`gofmt -l` 列文件仍退 0、PG 跳过而包仍显示 `ok`），以及「撞上要改 domain 或难
逆转取舍就停下报 MCP-1」。

### MCP-1 自己的份内活

端点表统一落行 + 各路集成 + 全仓复验（含真库）+ 推送；票 02 的 Comment 由 MCP-1 统一写
（五张派工都禁止各路编辑票面，避免五个人改同一份）。

### 排下一波

**02c 商业登记口**（本波五个通道占满，它是四片里唯一没派的）、**集成清单四笔** #4/#5/#12/#13、
**E1 报价表形状核对报告** + `price-card-shape-gaps` 立票、02b/02c/02d 的前端「登记」签、
据 MCP-6 盘点立适配器工程票、MCP-3 解阻后回 `frontline-transition-import/01` 加集运子命令。

---

## 2026-09-02 17:0x 第三次崩溃后的清点与收口（新 MCP-1 会话）

上一节那波五路**没有全丢**——这是三次崩溃里结果最好的一次。各路在崩溃前把在建现场
封存成了 `chore(salvage)` 提交，且此后某个 MCP-1 会话已把其中完成的部分集成进主线
（`origin/main` 到 `1fddf60` 时已含 02a 网络、02b 关务译装下沉、02d VE 六类登记口、
`RegistrationPanel` 抽象、`label-channel-service-first-release/spec.md`）。

### 五条分支逐条复核

判据是 `git diff origin/main <branch>` 的**两向**：三点差看「分支比合并基多什么」会把
主线独立做过的同一件事重复算进去，两点差才看得出分支上有没有主线没有的内容。

| 分支 | 结论 |
|---|---|
| `mcp2-awf02a-v2` | 内容与主线逐字节相同，已被吸收 |
| `mcp4-awf02d-v2` | 同上 |
| `mcp5-awf02b` | 同上 |
| `mcp6-inventories-v2` | 主线版反而更新——MCP-6 原 Comment 自称「并出两份只读盘点」，主线上已被改正为「spec 已建、盘点未开始」。分支版是被取代的那一版 |
| `mcp3-consolidation-provenance` | **唯一有真内容**：17 文件、集运来源事实层含迁移 0003 |

### 一处会误导的信号

MCP-3 worktree 里三个文件显示 `M`，`git diff` 却是空的——CRLF 幻影。同一格还有更贵的一次：
该树 `gofmt -l` 列了两个 `.go`、迁移的 `TestEmbeddedMigrationAssetsCarryNoCarriageReturnOrBOM`
也红。但 `.gitattributes` 定了 `*.sql` / `*.go` 一律 `text eol=lf`，**提交进库的 blob 本来就是 LF**；
删掉文件重新检出后 `gofmt -l` 与该门禁双双转绿。照 `gofmt -l` 的输出去改文件反而会改坏真内容。
主树同样有 59 个 CRLF 幻影 `M`（真内容改动只有用户的 `.cursor/mcp.json` 一处），已同法归一。

### 已办（本会话四笔，`1fddf60` → `957e768`，**已推**）

| SHA | 内容 |
|---|---|
| `f9c04da` | `mcp3-consolidation-provenance` 五笔合成一笔进主线。合成而非逐笔：原第一笔是「在建」快照，其后三笔只是让测试跟上同一处改动，逐笔落会让中间几个提交编译不过 |
| `3f304c9` | 补票的完成判据欠账一：适配器层的真库往返与 `AT-NO-043` 两向；迁移新增的在场 CHECK 也钉住 |
| `98e1752` | 补欠账二：来源两组接通 HTTP 响应体与节点作业查阅页两列。此前只到端口与 Postgres，而 `catalogue_read.go` 的注释已写着「页面据此分得开导入与扫描」——那句话在接通前是假的 |
| `957e768` | 两份票面收口：本票 `done`，`frontline-transition-import/01` 的集运那一格记阻断解除 |

验证强度：`gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿（含真库，DSN 指
门禁容器 55432）；`apps/admin-web` 以**仓内** typescript 5.6 跑 `tsc --noEmit` 退 0
（`npx tsc` 会落到占位包上，退 0 却什么都没编——不可当证据）。四个新真库用例按探针纪律
两向各取一次：设 DSN 各 PASS、不设 DSN 各 SKIP。读面那条另做过非空洞性检查：把
`review_catalogue.go` 的 `Scan` 两组来源对调，对应用例当场 FAIL，随即还原。

### 现在的树况

五棵 worktree 全部拆除（`git worktree list` 只剩主树），16 个分支指针一个不删。
主树只剩用户的 `.cursor/mcp.json` 与三个未跟踪文件。

---

## 2026-09-02 17:1x 九路并行（本波，基线 `957e768`）

用户 17:0x：「请你计划下发任务并行完成所有的这些工作。」`list_sessions` 复核 MCP-2 至 MCP-10
九个通道全部 online/running 且 idle，遂按**地盘互斥**派满。

### 派工前的现状核实（这一步改了三张票的内容）

笔记上「02a/02b/02d 后端已落主线」是**不准确**的。在 `957e768` 上逐个数 `adapters/http` 目录：

| 切片 | 传输层 | 传输层测试 | 接进 `cmd/parcel-api` |
|---|---|---|---|
| 01a 计价 | 有 | 有 | **有**（端点表里只有这两行） |
| 02a 网络 | 有 | **无** | 无 |
| 02b 关务 | **无**（只落了 `registrationjson` 译装下沉） | — | 无 |
| 02c 商业 | **无** | — | 无 |
| 02d VE | 有 | 有 | 无 |

所以 02a 那一路不是「已完成」而是缺了票面点名要求的那条编译期断言测试；02b 只走完译装
一步；而**四片的登记口至今一个都没接进端点表**——写面在产品上是不可达的。

### 分工

| 通道 | 任务 | 独占地盘 | 单号 |
|---|---|---|---|
| MCP-2 | 02c 商业登记口传输层（含 `registrationjson` 下沉） | `internal/partycommercial/**` | `task-032cfa88` |
| MCP-3 | 02b 关务四类登记口传输层（译装已在主线，不重做） | `internal/customscompliance/**` | `task-9432e572` |
| MCP-4 | 02a 补传输层测试 + 逐条核 02a 的完成判据 | `internal/networkrouting/**` | `task-02061577` |
| MCP-5 | 网络页与 VE 页的「登记」签 | **独占 `apps/admin-web/**`** | `task-5ccfd4b7` |
| MCP-6 | 两份只读盘点报告 | 只读，只提交两个 `.md` | `task-57276b9e` |
| MCP-7 | 清单欠账 #4 `cons-proj-tf-b` 与 #5 `t12-governance-register` | `internal/pilotgovernance/**` + `migrations/pilot_governance/**` | `task-9f49f439` |
| MCP-8 | 清单欠账 #12 行尾门禁比对、#13 `demo-seeds` | `migrations/line_endings_test.go` + `scripts/demo-seeds` | `task-b50a8175` |
| MCP-9 | E1 报价表形状核对 + `price-card-shape-gaps` 立票 | 只读 + 新 `.scratch` 目录 | `task-11415271` |
| MCP-10 | `frontline-transition-import/01` 集运子命令（本日解阻） | `cmd/parcel-frontline-import/**` | `task-deab7d39` |

### 这一波与上一波的三处形状差别

1. **端点表仍收归 MCP-1**（上一波已如此，本波沿用）：四路都要往同一张表加行，各路写进
   完工报，我统一落一笔并跑「未配置态 403 / 隔离读启用态仍 403」的装配断言。
2. **端点路径命名法由我一次定死并 broadcast 给 MCP-2/3/4/5**，不再让各路自定后我再改回来。
   规律取自现有四十余条：读面 `/{前缀}-{事物}`，登记口 `/{前缀}-{事物}-registrations`；
   一类一个端点，不用 `registry=` 把多类塑进一个口。VE 六条与网络七条已逐条列出。
3. **每张派工都写「每完成一小步立刻提交」**，并说明理由：前两次崩溃未提交的活全部归零，
   第三次因为各路提交了封存快照才救回来。这是三次崩溃里唯一被验证有效的止损。

每张也都点了本机环境那几个会直接坏事的坑，其中两个是本会话新踩出来的：`npx tsc` 会落到
占位包上（退 0 但什么都没编，不能当 typecheck 证据），以及工作副本 CRLF 会让 `gofmt -l`
与迁移 EOL 门禁双双误报（提交进库的 blob 本来是 LF）。

### MCP-1 自己的份内活

端点表统一落行 + `assemble_*` 生产装配（02a/02d 的传输层已在主线，可先做）+ 各路集成 +
全仓复验（含真库）+ 推送；票 02 的 Comment 统一由我写（九张派工都禁止各路编辑票面）。

### 17:2x 更正：本机只有五个通道，九路里五路是空投

用户指出**本机只有 MCP-1/3/4/5/6**，没有 MCP-2/7/8/9/10。`query_tasks` 复核九张全部 `pending`，
其中五张投给了不存在的通道、永远不会被消费。已逐张 `report_task` 结为 `failed` 并在 `result`
里写明**是派工错误不是执行失败**——这个区分要留住，否则下次读清单的人会以为那五件事试过而没做成。

派工前我 `list_sessions` 看到 MCP-2 至 MCP-10 都是 `online`/`running`，**那个信号不可靠**：
通道目录存在不等于背后有会话在跑。下次派工前应以用户确认的通道名单为准，或先 `send_to_session`
探一次活性再派。

| 原派 | 内容 | 现在的去向 |
|---|---|---|
| MCP-2 | 02c 商业登记口传输层 | **用户指示改由 MCP-1 自办**，本会话接手 |
| MCP-7 | 清单欠账 #4、#5 | 排下一波 |
| MCP-8 | 清单欠账 #12、#13 | 排下一波 |
| MCP-9 | E1 报价表形状核对 + 立票 | 排下一波（重派时数据红线那节原样带上） |
| MCP-10 | 集运子命令 | 排下一波 |

四张有效派工不变：MCP-3 → 02b 关务、MCP-4 → 02a 补测试、MCP-5 → 前端两页签、MCP-6 → 两份盘点。
端点路径命名法的 broadcast 也发给了这四路里的三路（MCP-3/4/5），MCP-2 那一份随通道一起落空，
但那部分现在归我自己执行，不受影响。

### 排下一波

上表四项（#4/#5、#12/#13、E1、集运子命令）；02b/02c 的前端「登记」签（等形状定下来）；
据 MCP-6 盘点立适配器工程票；据 E1 核对立价卡机制缺格票。

---

## 2026-09-02 18:2x 第四次崩溃后的收拾（新 MCP-1 会话）

### 这次没白干：四棵 worktree 全带着已提交内容活下来

「每完成一小步立刻提交」这条止损第二次生效。`git worktree list` 四棵树都在，各自分支
比 `957e768` 领先 1–2 笔，内容互不重叠：

| 分支 | 内容 |
|---|---|
| `mcp3-awf02b` | 关务四类登记口传输层（`register_configuration.go` 236 行）+ 传输层测试 510 行 |
| `mcp4-awf02a-test` | 网络七族登记口的传输层测试 625 行 |
| `mcp5-reg-tabs` | 网络目录与服务区域两页登记签，`MultiRegistrationPanel` 抽为共享组件 |
| `mcp6-inv-reports` | 末端面单渠道能力形状只读盘点（报告一，176 行） |

只有 MCP-5 那棵树有未提交残余：VE 页登记签**搬了一半**——`TrackingJudgmentRulesPage`
被改名成内部表格组件，外层带 Tabs 的新页面还没写，照现状 `tsc` 必红。原样封存进
`mcp5-reg-tabs-salvage@9ce147b`，不污染可集成的那一笔。

主树 59 处 `M` 又是 CRLF 幻影（真内容改动只有用户的 `.cursor/mcp.json`），`git restore
--worktree` 同法归一。

### 已办（`957e768` → `ddba601`，**已推**）

五笔 cherry-pick 把四支分支逐笔落上主线，再加本会话两笔：

| SHA | 内容 |
|---|---|
| `6d7c213`/`ec944e6` | 关务四类登记口传输层与其证据 |
| `4776670` | 网络七族登记口补传输层测试 |
| `a492f51` | 网络两页登记签 + 共享登记面组件 |
| `7a71902` | 面单渠道能力形状盘点报告 |
| `1199934` | **17 个登记口接进端点表并接真编排**——四片的写面此前在产品上不可达 |
| `ddba601` | 三条登记链在真库上钉住事务壳确实提交 |

端点路径（与前端 `networkRegistrationEndpoints` 逐字对齐）：网络七族
`/network-catalog-{族词}-registrations`；关务四类 `/customs-{事物}-registrations`
（关务读口本就按事物平铺，不套册名）；VE 六类 `/visibility-catalogue-{种类词}-registrations`。
同笔落三份 `assemble_*_registration.go` 生产装配（各带事务壳）、unwired 占位、探针 17 行。

**17 处委托不必写测试**：每个内层方法收的命令类型互不相同，把一族的编排接到另一族的
方法上编译期就红。真库测试因此只钉编译器看不见的那一半——事务壳会不会提交。三处取法
各不同：网络目录没有重放格（重复版本号由主键挡），只能读回列面证；关务口岸册的
`EXISTING` 与 VE 的 `VERSION_NOT_OVERWRITABLE` 本身就要读回首行才答得出来。

验证强度：`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -p 1 -count=1 ./...` 全绿
（含真库，5m22s）、`apps/admin-web` 以**仓内** typescript 5.6.3 跑 `tsc --noEmit` 退 0。
新真库用例按探针纪律两向各取一次：设 DSN 三格 PASS，不设 DSN SKIP。

### 本机环境新踩出的一个坑

门禁容器在机器重启后**丢了端口映射**：容器 `Up (healthy)` 而 `docker ps` 的 PORTS 只有
`5432/tcp`，没有 `127.0.0.1:55432->`。`docker compose up -d` 只是把旧容器启起来，沿用它
原来的配置，所以映射不会自己回来。修法：`docker compose down` 后
`docker compose up -d --force-recreate`。不这么做，全部真库用例会以「连不上」失败，
而那个失败长得跟「库没起」一模一样。

### 停手点：两份派工冲突

推完之后发现主树里有我没写的改动——别的会话在**主树**里写：
`apps/admin-web/src/pages/visibility/catalogue-api.ts`（VE 页登记写面，端点路径与我接的
一致）、票 02 票面、`docs/README.md` + 一份 blueprint 转出的 `.md`。

票 02 票面新写了一张分工表（署 MCP-3 分派）：**`cmd/parcel-api/**` 划给 MCP-3、02c 商业
整片划给 MCP-5**。而用户给本会话的指令是「原来的都 crash，请你继续」。两份派工对同一批
文件给了不同的写入方。

我已经做完的那部分（端点表）正是票面划给 MCP-3 的地盘——已推，不必重做；已 `broadcast`
通报全通道并附上重基指令与那个端口映射坑。在用户裁定前，MCP-1 不进
`internal/partycommercial/` 与 `apps/admin-web/`。

### 用户 18:3x 裁定：分工表作废，MCP-1 接手全部

用户选了「只做无人认领的」+「分工表作废、MCP-1 接手全部、其它通道停手」两项。已 `broadcast`
停手令并说明未提交现场会原样接收。

**接手方式：读完内容再逐目录 `git add`，不整树 `git add -A`。** 各路的活混在同一个
`git status` 里，整树提交会把「我读过并认可」与「碰巧在树上」混成一笔——票 02 上一条记的
那次卷带正是这么来的。

各路交出来的活质量好，**接管而不重写**：02c 商业八类传输层（385 行，理据站得住）、它的
1033 行传输面测试、关务与 VE 六页登记签、竞品事实台账。我自己写的一份 02c 测试因为比他们
那份薄且引了不存在的构造器，直接删掉不用。

### 已办（`957e768` → `398a148`，`1bd9e9d` 及以前已推）

第一批（`ddba601`，已推）：四支崩溃分支集成 + 17 个登记口接进端点表 + 三份生产装配 + 真库
装配测试。第二批（`1bd9e9d`，已推）：

| SHA | 内容 |
|---|---|
| `ebf352b` | 产品战略蓝图 V1.0 入 `docs/reference/` 并登索引；十处断图从 docx 的 `word/media/` 取出补齐 |
| `dc900b1` | 商业八类写面进传输层（接管） |
| `f723a2c`/`1f0207f`/`10754b4` | 关务四页与 VE 三页登记签（接管） |
| `a446cc8` | 商业八类接进端点表 + `assemble_commercial_registration.go` |
| `fb98943` | 商业身份登记链真库装配用例 |
| `729e135` | 商业八类传输面证据 17 条（接管） |
| `7e49f4e` | 竞品公开事实台账（接管） |
| `9d1e941` | 商业八类前端接线（接管） |
| `2ab34bf` | 产品故事「今天能演示什么」按 page-registry 实况更正 |
| `81b05d3` | **建案要求规则补第五个登记口**——票面把十二个用例数成十一个，漏的正是它 |
| `5b291dc` | 票 02 记分工作废与接手收口 |
| `398a148` | 共享登记面收下散文原因、未决原因与声明落点三格（未推） |

**端点表本波总计接进 26 个写面**：网络七、关务五、VE 六、商业八。未配置态 403 与隔离读
启用态仍 403 两条断言覆盖全部 26 行。

验证强度：`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -p 1 -count=1 ./...` 全绿
（含真库，5m28s）、`apps/admin-web` 以仓内 typescript 5.6.3 跑 `tsc --noEmit` 退 0。

### 本波踩到的两个坑

**`Set-Content` 的 BOM。** 我用 `Set-Content -Encoding UTF8` 往 `endpoints_test.go` 插一行，
PowerShell 5.1 的 `-Encoding UTF8` 带 BOM，`go build` 当场报错。AGENTS.md 写过这条，我照样
踩了——补法是 `[System.IO.File]::WriteAllText($p, $text, (New-Object System.Text.UTF8Encoding $false))`，
但更该记住的是：插一行用 StrReplace，不要为省事走 shell 重写整个文件。

**并行会话不肯停。** 两次 broadcast 停手令之后，仍有会话在主树里写（`RegistrationPanel.tsx`
在我读它的同时被改完）。它们改的是不相交的目录，未发生互相覆盖，但「我读到的内容」与
「我提交的内容」之间有窗口——本波靠每次提交前重跑 `tsc`/`git diff` 兜住，不是靠约定兜住。

### 排下一波

商业 02c 的前端页签还缺（`pages/party/` 只落了 `api.ts` 与 `presentation.ts`，页面本体未接）；
`case-requirement` 的前端签（端点已在，规则页签名仍写死「登记解释规则」）；集成清单欠账
#4/#5/#12/#13；E1 报价表形状核对 + 立票；集运子命令；MCP-6 盘点报告二。

---

## 2026-09-04 16:1x 全量复审后的一波并行（新 MCP-1 会话，基线 `a17bfac`）

用户 16:0x 令「全面审查分析 `.scratch`」→ 产物 `.scratch/unresolved-review-20260904/remaining-work-a3a4814.md`
（`a17bfac`）；随后令「采用并行工作全面完成剩下的工作，可派 mcp-2/3/4/5/6」。

**先清账**：`query_tasks` 里躺着 09-02 的七张旧派单（02a/02b/02b/02d 前端签/集运来源/consolidation 抢救/
label-channel/11）与今日 14:15 派 MCP-4 未消费的 tf/02，内容全部已随主线落地或改派——逐张 `report_task`
代结，`result` 写明「已被主线取代，非执行失败」/「派工未消费，改派」。

**点名**：16:13 广播 `[点名 ← 通道 1 · 截止 16:17]`，MCP-6/4/2/3/5 于 16:14–16:15 全部应答「空闲 · 地盘无 ·
余量充足」（注意：MCP-2 自报地盘无，等于它此前对 tf/02、pc-gaps/05 的占号已随会话消失；MCP-5 自报地盘无，
而 `mcp5-ftr07-d4-pc` 分支上有它前会话的 D4 PC 半边两笔——新会话不记得，由 MCP-1 重放）。

| 通道 | 单号 | 内容 | 独占地盘 |
|---|---|---|---|
| MCP-2 | `task-a6e4baed` | tf/02 实际承运商判断（ADR-0103） | TF domain/application/ports/adapters/{postgres,partycommercial} 新文件 + 两处挂点；TF 迁移 **0013** |
| MCP-3 | `task-f530ad56` | 裁决批 10 处（tf/08、tf/09、tf03 封存笔、label-channel/14、admin-write-faces/06、ftr/09、pricing/05b、06、09、pricing-amount-precision/01；附 unmerged 第 5/13 笔）+ 票面簿记 17 处 | `.scratch/**`、`docs/adr/**`、seed.sh 一段注释；不写代码 |
| MCP-4 | `task-73c3ea31` | pc-gaps/05 客户服务规则正文两项（ADR-0104）+ 立 VE 侧后继票 | `internal/partycommercial/**`、`migrations/party_commercial/0023`、`cmd/parcel-commercial/**` |
| MCP-5 | `task-caea7640` | label-channel/19 + /21 | TF `ports/external_tracking_fact.go`、`adapters/http`、`adapters/postgres/effective_time_rule*`、TF 迁移 **0014**；**`cmd/parcel-api/endpoints.go`+`main.go`+`assemble_*` 本波独占**；`apps/admin-web` |
| MCP-6 | `task-b9f70ead` → `task-95dd1b14` | 先 frontline-transition-import/01 集运子命令，后 ftr/07 D4 PS 半边 | `cmd/parcel-frontline-import/**`；然后 `parcelshipment/adapters/inbox/**`、`cmd/parcel-dispatch/**` |
| MCP-1 | — | 重放 D4 PC 半边 `3b94bab` → `652c6aa`（+清点 `6f01157`），隔离检出全仓验证后快进并推；落各路装配行；集成复验推送；E1 报价表形状核对若无人接由本通道做 | 集成 |

**形状差别**：迁移号预先分配（TF 0013/0014、PC 0023）而不是让各路开工时再对；endpoints.go 这次给了一个
真正要往里写行的通道独占，其余通道写进完工报；spec 状态行本波只由 MCP-3 改一次，完工对齐归 MCP-1。

**未派**：E1 报价表形状核对 + `price-card-shape-gaps` 立票（第三次排上、仍无人手），谁先空谁接。

## 2026-09-04 17:48 全体通道同时换新会话后的接续（新 MCP-1 会话，接手时 `main = f25d692`）

17:48 通道 1/2/4/5/6 同时换成新会话，MCP-3 与 MCP-5 的旧会话已 crash（heartbeat 分别止于 16:22 / 16:49）；
所有在途记忆只剩 `query_tasks` 里的派单文本与各分支上的提交。E1 报价表核对已由上一会话 MCP-1 做完（`0cb8163`，
立 `price-card-shape-gaps/01–03`）。本会话用户只交代「监听队列、正常回复、保持循环」，重放与推送按整合方角色继续。

**接管簿记的形状**：旧单 `report_task failed` 并在 `result` 写「承接方换人，非执行失败」+ 新单号；新单 `context` =
接管说明（原单里已变的句子逐条点出）+ 原单原文。三次用到：`f530ad56`→`8f3e61b7`（MCP-3→MCP-2）、
`caea7640`→`0b475be7`→`dcc78dcf`（MCP-5→MCP-6→MCP-5 新会话）、`d47ee7c6` 早已代结。

**撞号与对号**：用户 17:52 口头把 f530ad56 也指给了 MCP-6，与 17:55 派 MCP-2 的 `8f3e61b7` 撞；按 MCP-6 提议
切成 A 裁决批（`35811f96`→MCP-6）与 B 簿记批 + D-02 句（`8f3e61b7` 收窄→MCP-2），父 spec 状态行只给 B 侧一人；
lc/19+21 改派空闲的新 MCP-5，MCP-6 先交现场再释接线文件。MCP-6 新会话又自行「让位 A 侧改接 lc19」与裁定交叉，
直发终裁一次收口，不再改记录。MCP-4 新会话纠正「MCP-4 正在做 pc-gaps/05」的预设——**裁定里不要预设谁在做，先问**。

| 通道 | 单号 | 内容 | 结果 |
|---|---|---|---|
| MCP-2 | `8f3e61b7` | B 票面簿记 17 条 + D-02 句 | done `512b419`（六笔全 .md） |
| MCP-2 | `92a7c29f` | label-channel/14 渠道择优决定只追加记录（PS，迁移 0015；读面后继票取 **23**，22 已被 lc19 分支用掉） | 在途，分支 `mcp2-lc14` |
| MCP-6 | `35811f96` | A 裁决批：ftr/09、pricing/06、pricing/09、pricing-amount-precision/01、附 5/13、shape-gaps/01–03 | done：九笔 .md + seed.sh 注释；新 ADR **0106–0111**；两处越权风险点（ADR-0109 跨上下文归属、ADR-0111 改 PP CONTEXT 硬句）已随 `9e8a9202` 推出，需用户复核，不认可走 supersede 不改历史 |
| MCP-6 | `66cd286c` | ftr/09 实施（ADR-0106：翻转 + 信封同笔、入口接装配、队列读口） | 在途，endpoints.go/main.go/unwired 行写进完工报由 MCP-1 落 |
| MCP-5 | `dcc78dcf` | label-channel/19（分支上已 resolved，立 22 回填入口 draft）+ /21 | 在途，分支 `mcp5-lc19-21`（已 rebase 到 d41f73da） |
| MCP-4 | `73c3ea31` | pc-gaps/05 续做（0023 两张强类型子表，票面记与 ADR-0104 Decision 三字面的落法差异） | 在途，分支 `mcp4-pcgaps05` |
| MCP-1 | — | 重放 fti/01 + tf/02 + ftr/07 D4 PS 三分支（22 笔 + 清点 `68868a12`），全仓 DSN 95 ok 后推；tf/02 装配行 `9e8a9202`（三处装配接 `NewActualCarrierJudgments` + 装配用例）；tf/ftr/pricing spec 对齐 | `origin/main = caca1c4a` |

**重放纪律补一条**：各分支自己的机制清点重生成笔不逐条重放，tip 上一次重生成；中途 main 被别人推进纯 .md/.sh
时 rebase 即可，代码同一则全仓验仍有效，但清点笔引用的检出 SHA 要 amend。

**待办**：tf/06 补刀二（`3f3a675` 裁「采」，封存笔 `mcp4-tf03@44808f3`）在当下 main 上重写合入；lc spec 对齐等
lc19/21 重放；pc-gaps spec 对齐等 MCP-4；`docs/design/pp-pricing-rule-model-final-design.md`「聚合方式仍只有逐包裹」
一句被 ADR-0111 取代，未改；应立而未立：TF 承运总单登记册（ADR-0111 主单级身份来源）、pilotgovernance
`channel_execution.command` 是否加封闭集。可派：pricing/06、pricing-amount-precision/02、pricing/10、
shape-gaps/01–03 实施（均 parcelpricing，换号合并一次）、tf/08（等 MCP-5 释 TF adapters/http）。

## 2026-09-04 19:11 通道 1 再换新会话后的接续（接手时 `main = origin/main = 50726a18`）

用户只交代「监听队列、正常回复、保持循环」，按整合方角色继续。接手时队列里躺着五条：MCP-4 剪两份棘轮基线的占号
（本次集成不碰那两份文件，未应声）、MCP-2 `92a7c29f` label-channel/14 的 done 报 + 释号广播、MCP-5 `a1425a1f`
pricing/06 的接单报 + 占号（上一会话派的，`query_tasks` 里 working）。

**接手时的现场**：上一会话在 `$TEMP\idp-mcp1-replay`（detached）把 `mcp5-lc19-21@8f6d42cf` 重放到了 `50726a18`
（`e0f51be3`…`511dc7b8` 十二笔 + 清点 `d09f0b98`，18:59 提交），**未验、未快进、未推**；共享树上 lc `spec.md` 有一处
未提交改写（mtime 19:11:23，恰在换会话那一刻），内容是 19/21 两格按重放写好、14 仍写「MCP-2 在做」——判为上一 MCP-1 的
在途件而非别人的（形状与它的待办「lc spec 对齐」逐字对上），在其上收完而不另起一版。其余 70 处 ` M` 全是 CRLF 幻影
（`git diff --numstat` 只有 spec.md 一行）。

**链的顺序改了一次**：mcp2-lc14 已由 MCP-2 自己 rebase 到 `50726a18`，八笔的 SHA 已写进票 14 完成记录；lc19/21 的
分支 SHA 则已经被上一会话的重放换过一次、无论如何都要广播对照。故以 lc14 八笔为底（**SHA 原样进 main**）、把 lc19/21
十二笔（取上一会话已解冲突的重放件 `50726a18..511dc7b8`）叠上去，两条分支各自的清点笔（`df6e1ff3`、`d09f0b98`/`0be8b4fd`）
都不重放，tip 上一次重生成——这样只有一组 SHA 需要对照，而不是两组。两组 .go/.sql 零重叠（lc14 在 PS 与
`architecture/rehydration_gate_test.go`，lc19/21 在 TF、`cmd/parcel-api` 与 `apps/admin-web`），cherry-pick 零冲突。

| SHA | 内容 |
|---|---|
| `c0ddc4fb`..`f43c99ee` | label-channel/14 八笔，与分支同号 |
| `3680f241`..`fab43802` | label-channel/19+21 十二笔（对照表见票 19/21 的「进 main 记录」与 19:3x 广播） |
| `19cf2ce5` | 机制清点在 `fab43802` 干净检出上重生成，**已推**（`git push origin 19cf2ce5:main`，推前 ls-remote = 50726a18） |
| `bb321c53` | lc spec 状态行与票一览对齐 + 票 14/19/21 各补「进 main 记录」（与 `9fb374a0` 一起推） |
| `710fc6fa` | MCP-2 19:32 在分支上补的票 14 Comments 末条（`af01895c`，rebase 后复验记录）cherry-pick 进 main，已推；票 14 = 它的最终版 + 我的三行对照。`$TEMP\idp-parcel-mcp2-lc14` 对完内容拆除（19 件只差票 14 那三行与清点），不带 `--force`，指针 `mcp2-lc14@af01895c` 保留 |

验证（在 `idp-mcp1-replay` 的 detached 检出上钉 `19cf2ce5`，树干净无未跟踪）：`gofmt -l` 空；`go build`/`go vet` 退 0；
含 DSN `go test -p 1 -count=1 ./...` 退 0，95 ok / 0 FAIL（7m36s）；探针 `TestAChannelSelectionDecisionRoundTripsThroughPostgres`
与 `TestEffectiveTimeRuleWritesRefuseToRunOutsideATransaction` 带 DSN `--- PASS`；`tools/mechanism-inventory` vet/test 退 0；
`apps/admin-web` 以仓内 typescript 5.6.3 跑 `tsc --noEmit` 退 0、`run-tests.mjs` 61/61——node_modules 用 `mklink /J` 从主树
借进隔离树、验完 `rmdir` 只拆链接（不能 `Remove-Item -Recurse`，会顺着 junction 删主树的包）。改动文件 BOM/CR 扫描：七个
`.ts/.tsx` 工作副本有 CR 是 `core.autocrlf=true` 的检出产物，`git ls-files --eol` 均 `i/lf`，blob 干净。

**快进时撞到一格**：`git merge --ff-only` 被 `issues/14-*.md` 的 CRLF 幻影挡下（那份文件恰在链里）；`git diff` 对它为空，
`git restore --worktree` 单独归一那一份后 ff 通过。其余幻影没动——它们不在链里，也不是我的。

| 通道 | 单号 | 内容 | 状态 |
|---|---|---|---|
| MCP-2 | `92a7c29f` | label-channel/14 | done，已入 main（上表）；通道 2 自报空闲 |
| MCP-4 | `73c3ea31` | pc-gaps/05 | working；19:0x 广播在自己 worktree 剪两份基线各两行（11/25→9/23，连 SHA 记） |
| MCP-5 | `a1425a1f` | pricing/06（契约 + FileConnector + 免复核格 + `cmd/parcel-pricing-feed`，CFETS 留 draft；PP 迁移 0005） | working，基线 50726a18，需 rebase 到 19cf2ce5 后收口（广播已提） |
| MCP-6 | `66cd286c` | ftr/09 实施 | working（上一会话派），endpoints.go/main.go/unwired 行等完工报由 MCP-1 落 |

**待办**（承上节，去掉已完成的两项）：tf/06 补刀二在当下 main 上重写合入；pc-gaps spec 对齐等 MCP-4；
`docs/design/pp-pricing-rule-model-final-design.md` 那一句未改；应立而未立两项不变。可派：pricing-amount-precision/02、
pricing/10、shape-gaps/01–03 实施（等 pricing/06 收口后同一通道换号合并一次）、tf/08（TF adapters/http 现已无人占）、
label-channel/23（draft，读面形状先按 ADR-0077 裁）。

### 19:4x 用户「你派工了吗，他们都没有工作啊」→ 点名后派满四路

**先量再派**：19:44 广播点名、截止 19:48；MCP-3/2/5/4 于 19:45–19:47 全部应答「空闲 · 地盘无 · 余量充足」，MCP-6 截至 19:48
未应答（其分支 `mcp6-ftr09` 19:42 刚提交、树上有在途件，判为正在干活；20:0x 补答在做 ftr/09）。**MCP-4 与 MCP-5 自报空闲意味着
它们的会话又换过一次**：MCP-5 的 pricing/06（`a1425a1f`）零提交、worktree 干净；MCP-4 的 pc-gaps/05 却**已在分支上完工**
（`mcp4-pcgaps05@34d8ad90`，九笔 16:35–19:03，票已转 resolved、VE 后继票已立），只是会话重置前没发 done 报——`query_tasks`
里它还是 working。这两格在 `query_tasks` 里长得一样，分辨靠的是分支日志与 worktree 状态，不是任务台账。

| 通道 | 单号 | 内容 | 地盘 |
|---|---|---|---|
| MCP-2 | `7f3d6450` | label-channel/23 运营查阅面（代裁 draft→ready：读端口另立二口、只列冲突不给动作、无隐式截断） | PS ports/postgres/http 新文件、`assemble_channel_selection_decisions*`、admin-web 新页；endpoints 行写进完工报 |
| MCP-3 | `ed1ac122` | tf/08 揽收更正走新版本（已裁 A） | TF offsite_pickup 四层 + `assemble_offsite_pickup_correction*`；段侧重派生另立 draft 票 |
| MCP-5 | `31a5aa4a` | 【接管重派】pricing/06（原 `a1425a1f` 结 failed「承接方换人」；基线 eba019a8，迁移 0005 重申） | parcelpricing sourcefeed/ports/domain/application/postgres 新文件 + `cmd/parcel-pricing-feed` |
| MCP-4 | `10b1e66b` | ve-claims-read-seams/03（代裁形状按 ADR-0104 Consequences；四条未决先 `/grill-with-docs`，答不了的显式留格） | VE `adapters/partycommercial`、`claim_eligibility.go`、`assemble_claims*` |
| MCP-1 | — | 重放 pc-gaps/05 九笔 + tf/06 补刀二 + 清点，全仓验后推 | 集成 |

`73c3ea31`（pc-gaps/05）由派单方代结 done，`result` 写明「分支上已完工、会话重置前未报、新会话未接手、MCP-1 集成」。
四路 20:0x 前全部广播占号并建树；MCP-2 追问 endpoints.go 归属已答（MCP-6 独占，其余写完工报）。

### 已办（`eba019a8` → `e6dfc9be`，**已推**）

| SHA | 内容 |
|---|---|
| `aa0608f7`..`469bb9ba` | pc-gaps/05 九笔重放（分支→main 对照见 20:1x 广播）；唯一冲突点是各笔内嵌的机制清点，一律取 main 版 |
| `4cbe5266` | tf/06 补刀二：封存笔 `mcp4-tf03@44808f31` 重放，与 tf/02 的 `enterFulfillmentSegment` 挂点（56eb4eac）自动合并无冲突；提交信按裁决重写，不再是「非集成候选」 |
| `e6dfc9be` | 清点在 `4cbe5266` 干净检出上重生成（PC +1/+1、端口 +1、迁移 PC 23 份） |

验证（隔离 detached 树钉 `e6dfc9be`）：`gofmt -l` 空；build/vet 退 0；含 DSN `go test -p 1 -count=1 ./...` 退 0，95 ok / 0 FAIL
（7m41s）；探针 `TestCustomerServiceRuleRoundTripsBothItems`、`TestADeliveryWithoutAParticipationEnderFormsNoAnswer` 带 DSN
`--- PASS`；inventory 工具 vet/test 退 0。本次无 admin-web 改动，未跑 tsc。

**拆树**：`idp-parcel-mcp4-pcgaps05`、`idp-parcel-mcp4-inv`、`idp-parcel-mcp5-lc19` 三棵对完内容（各自文件面与 main 只差
清点与别人加进 `unwired_orchestration.go` 的行）、树干净、不带 `--force` 拆除；指针 `mcp4-pcgaps05@34d8ad90`、
`mcp5-lc19-21@8f6d42cf` 保留。`D:/tops/idp-tf03`（`mcp4-tf03@44808f31`）在 TEMP 之外、MCP-3 裁决写「分支与 worktree 不动」，
内容已入 main 但树未拆，留给用户定。

### 20:0x–20:3x：ftr/09 完工重放、换号批派 MCP-6、三份 spec 对齐

MCP-6 20:0x 报 ftr/09 done（`mcp6-ftr09@6aadef60`，已验代码 tip `90260aae`；它自己在临时树上试过 cherry-pick 到 e6dfc9be 零冲突）
并释出 `cmd/parcel-api` 四份接线文件——**本波「端点表收归一人」的安排随之作废**：MCP-2 lc/23 与 MCP-3 tf/08 改为各在自己那组
加行，重放顺序由 MCP-1 定；已随回 MCP-3 的开工报答复一并告知。MCP-3 开工报三处发现（`offsite_pickup` 主键不带版本→取 TF 迁移
0015 换主键；更正版 handoff ID 加版本段防 EnqueueOnce 静默吞掉；PS 采用口把同来源第二版本当竞争的责任起点、AT-PS-050 无代码）
全部同意，第三条要它在 label-channel 目录立 PS 侧 draft 票（24）。

| SHA | 内容 |
|---|---|
| `84787373`..`aa78b2e5` | ftr/09 七笔重放到 `4cc1bc34`（分支→main 对照见 20:3x 广播）；`unwired_orchestration.go` 一处自动合并 |
| `5032ec95` | 清点在 `aa78b2e5` 干净检出上重生成（PS +4/+4、迁移 PS 16、端点/消费适配器/路由/端口各 +1），**已推** |

验证（隔离 detached 树钉 `5032ec95`）：`gofmt -l` 空；build/vet 退 0；含 DSN `go test -p 1 -count=1 ./...` 退 0，95 ok / 0 FAIL
（8m05s）；探针 `TestACustomerSupplementWaitIsResumedByTheNewSubmissionVersionEnvelope` 带 DSN PASS；inventory 工具 vet/test 退 0。
`idp-parcel-mcp6-ftr09` 对完内容（28 件只差别人加进 `unwired_orchestration.go` 的行与清点）拆除，指针 `mcp6-ftr09@6aadef60` 保留。

**派 MCP-6 `2b9edfe4`：parcelpricing 换号批**（pricing/10 + amount-precision/02 + shape-gaps/01–03，ADR-0107–0111，五票均
ready-for-agent）。形状要点：规范化号只换一次、单独成笔；与 MCP-5 pricing/06 同包按文件分界（06 只新建文件 + 迁移 0005，
换号批动既有文件 + 迁移 0006 起）；06 会按今天的 `VersionReference` 形状新增调用点，换号批用三步法保留旧构造名为薄包装、
收缩留到 06 入 main 之后；shape-gaps/03 只做票级，主单级留格。顺带要它先把「判断账无版本维」立成 ftr 目录的 draft 票。

三份 spec 状态行随本笔对齐：pc-gaps 05 → resolved（main SHA）、tf 06 补刀二已合 / 08 in-progress、ftr 09 → resolved。

**待办**：lc spec 的 23 行等 MCP-2；tf spec 的 08 行等 MCP-3；pricing 三份 spec 等 MCP-5/MCP-6；ve-claims-read-seams 目录无 spec；
`docs/design/pp-pricing-rule-model-final-design.md` 那一句；应立而未立两项不变（TF 承运总单登记册现在还挡着 shape-gaps/03 主单级）。
`D:/tops/idp-tf03` 未拆，留给用户。

### 20:4x–20:5x：ve-claims/03 完工快进、派 05

MCP-4 20:4x 报 ve-claims-read-seams/03 done：分支 `mcp4-ve03@522ea43d` **基于 main 当时的 tip 9e4e90bb**，八笔（含它自己在
`fd49465f` 上重生成的清点 `91967a6b`）。基线就是 tip，所以走**快进**而不是重放：隔离树钉 `522ea43d` 跑 gofmt/build/vet、生成器重跑零差、
含 DSN `go test -p 1 -count=1 ./...` 96 ok / 0 FAIL（8m31s）、探针 `TestTheWiredClaimsReadCustomerServiceRulesFromPartyCommercial`
PASS 后 `git merge --ff-only`，**八笔 SHA 原样进 main，已推**（远端 `522ea43d`）。分支上的清点笔这次不丢——它就在 tip 的父提交上
量的，重跑零差，「tip 上一次重生成」在这里等于它。`idp-parcel-mcp4-ve03` 拆除（内容 = main），指针保留。

四条未决的去向（MCP-4 报，我不代裁）：① Registered 粒度自裁「各维按 PC 那一项有没有行」（细则受托代裁，已落代码）；② 起算事实源、
③ 业务日历——留格（Deadline 零值，编排停在既有未决格）；④ Notice——留格。**交用户裁的三件**：a) 起算事实源 + 业务日历能力归谁（VE 自建
按引用取日历与「起算事件引用→VE 事实」对应，还是等更上游）；b) Notice 来源（PC 放宽项类 vs VE 自建索赔通知依据册）；c) 票 04 四问
（锚点从哪来、键按什么维登、要不要一并要 CustomerContractObject、登记入口）。都是跨上下文归属，属 ADR 级，不在派单里顺手定。
生产装配 `Keys=nil`：PC 登了正文两维仍答未登记，行为与本票之前一字不变（装配测试钉住）——登记面归票 04。

**派 MCP-4 `21070bf5`：ve-claims-read-seams/05**（它随 03 立的 ready 票：编排把「登了规则但截止算不出」从 `NOT_REGISTERED` /
`SUPPLEMENT_WINDOW_CLOSED` 分出两格 `UNDERIVABLE`；VE application 地盘；票面写明两格判据在类型上已分开、不需要新裁决）。

### 21:0x–21:1x：ve/05 完工重放、派 MCP-4 三件簿记

MCP-4 21:0x 报 05 done（`mcp4-ve05@483ff3b3` 基 522ea43d，三笔）。基线与 main 只差我那一笔 tasks.md，所以走重放不是快进：
`46546c90→2551d565`、`260022f1→540b76b9`、`483ff3b3→96054c48`；清点零差不另生成。隔离树钉 `96054c48`：gofmt/build/vet 退 0、含 DSN
全仓 96 ok / 0 FAIL（8m）；**已推**（远端 `96054c48`）。`idp-parcel-mcp4-ve05` 拆除，指针保留。

**派 MCP-4 `62e8e262`：三件纯 .md 簿记**——① `docs/design/pp-pricing-rule-model-final-design.md`「聚合方式只有逐包裹」一句按 ADR-0111
改口；② 立 TF 承运总单登记册 draft 票（新目录 `tf-carrier-master-document-register`，不与 MCP-3 在 tf-segment-lifecycle-closure 抢号；
顺手在 shape-gaps/03 的 Blocked by 行补链接，只动那一行）；③ 立 pilotgovernance `channel_execution.command` 封闭集 draft 票（新目录
`pilot-governance-context-gaps`）。三项都是本节开头「待办」里挂了三轮的。tf/09 看过：票面实为 draft（裁决把它拆成派送段声明建模 + 三条
输入缝 + 触发执行器，开工前置一次 `/domain-modeling` 很可能要 ADR），不是可派的代码票，spec 里那个 ready-for-agent 是过期的。

## 2026-09-04 21:5x 通道 1 再换新会话后的接续（接手时 `main = origin/main = cc7f4e97`）

上一会话 21:04 把 mcp3-tf08 重放推出（`14ab8552`）后崩溃，未及广播；通道 2 于 21:3x 代提了它留在共享树的两处 spec 对齐
（`ba883006`、`cc7f4e97`）。MCP-4 三件簿记（`62e8e262`）已全部进 main（`4f13338f`、`420ac887` 同内容、`14ab8552`），分支 `mcp4-bookkeeping`
三笔 `git cherry` 全 `-`。fti/01 集运子命令与 ftr/07 D4 PS 半边早已在 main（`a4740b16` 等；票 01 Status 里「待 MCP-1 重放」一句是分支上写的，过期）。

| SHA | 内容 |
|---|---|
| `7d7b8b5e` | tf/08 进 main 记录：票 08 Comments 补十笔分支→main 对照（MCP-3 逐文件核 + 本会话 `git cherry` 复核九笔 `-`、`0c3470ad` `+`）；tf spec 票一览 08 行与「已 resolved」句补 08。**已推** |
| `17e0a053`..`833e6557` | MCP-6 换号批五笔重放（cherry-pick 零冲突；对照见 22:2x 广播）：pricing/10 resolved（换号 PPC-4→PPC-5、PRS-1→PRS-2）、amount-precision/02 领域半边、first-tenant-runway/10 draft 票、交接 |
| `ae7b4c8a` | 清点在 `833e6557` 干净检出上重生成（PP 生产 +1 / 测试 +3）。**已推**（推前 ls-remote = 7d7b8b5e） |

验证（隔离 detached 树钉 `ae7b4c8a`）：gofmt 空；build/vet 退 0；含 DSN `go test -p 1 -count=1 ./...` 退 0，96 ok / 0 FAIL（7m58s）；探针
`TestReferenceSeriesCanonicalizationDiffersIsAnswered` 带 DSN PASS / 不带 SKIP；admin-web 仓内 tsc 退 0、run-tests 66/66（node_modules 走 junction，验完 rmdir）。
拆树：`idp-mcp1-replay`（上一任遗留，status 零行）、`idp-parcel-mcp6-pp-renumber` 与 `idp-parcel-mcp6-pp-verify`（批内文件与 main `git diff` 空）三棵不带 `--force` 拆除，指针 `mcp6-pp-renumber@4c81790b` 保留。

### 22:0x 用户「还有没有未 resolved 的？派 MCP-2/3/4/5/6 全部完成，困难吗」→ 点名后只有两路可派

**盘点**（`grep '^Status:'` 非 resolved，钉 `7d7b8b5e`）分四类：① **ready-for-agent 可直接实施**：pricing/06（分支五笔在，票 in-progress 无完工报）、
shape-gaps/01–03（票级）、amount-precision/02 余项、pricing/05 05b（ADR-0105 已答）；② **draft，先 `/domain-modeling` 或裁再动，多半各要一篇 ADR**：
tf/09、tf/10、tf-carrier-master-document-register/01、pc-gaps/07（→ awf/06 解阻）、awf/07 伞票、label-channel/24、pilot-governance/01、
auto-reroute-demo-reachability/01（先复核依赖表）、first-tenant-runway/10、ve-claims/04；③ **等第一家真源**：label-channel/20、22；④ **agent 做不了、归用户**：
needs-info 四张（ps-external-mark-relations/01、ve-008/04、nr-route-evidence-views/01、syn-wall-door-audit/01）、first-tenant-runway/03（PAR-NET-14 实例值）、
tenant-implementation-01 三份（仓外动作）、ve-claims/04 交用户裁的三件、admin-write-faces/06（等 pc-gaps/07）。

**点名**：22:08 广播 `[点名 ← 通道 1 · 截止 22:12]`，MCP-3/2/5 应答空闲；MCP-4/6 未应答；用户 22:1x 报「MCP-2/4/6 crash 了」，故只派 3 与 5。
台账清账：`31a5aa4a`（pricing/06，旧 MCP-5 未消费即开工）结 failed「承接方换人」；`2b9edfe4`（换号批，MCP-6）结 done「部分交付 + 交接」。

| 通道 | 单号 | 内容 | 独占地盘 |
|---|---|---|---|
| MCP-5 | `03538334` | 【接管重派②】pricing/06 收口：分支 `mcp5-pricing06` 五笔对完成判据补齐 + 真库往返 + 票面完成记录 | parcelpricing **新文件**（sourcefeed、connector/feed/binding/artifact 词根）、迁移 0005、`cmd/parcel-pricing-feed` |
| MCP-3 | `de0159af` | 换号批续作：shape-gaps/01–03（票级）+ amount-precision/02 余项；PPC-5 不再换号 | parcelpricing **既有文件** + 新领域/端口/登记册、迁移 0006–0009、`adapters/http`、`cmd/parcel-pricing-register`、`cmd/parcel-api` PP 那组行、SA、admin-web pricing |

**未派、排下一波**（等 2/4/6 回来或 3/5 空出；ADR 号预留：TF 批 0112–0114、PC 批 0115–0116、PS 批 0117–0118，迁移号 TF 0016 起、PC 0024 起、PS 0017 起、
pilot_governance 0006 起）：TF 建模批（tf/09、tf/10、tf-carrier-master-document-register/01）；PC 批（pc-gaps/07 → awf/06 → awf/07 拆子票）；PS 批（label-channel/24、
auto-reroute-demo-reachability/01）+ pilot-governance/01；pricing/05 05b（等 3/5 收口后同一通道，避免三人同包）。

**给用户的答复要点**：不难在代码量，难在两处——① 第 ② 类十张 draft 票每张都要一次 `/domain-modeling` 且多半要 ADR（今天通道 6 那批 ADR-0106–0111 就是这么出来的，
其中两处越权点还等用户复核），agent 可以「owner 授权自决」照今天的口径做，但那不是「完成」而是「替用户先裁一版」；② 第 ④ 类**任何通道都完成不了**，要用户给信息或在仓外动作。
所以「全部完成」的可达边界是 ①②③ 三类；④ 类要用户出手。

### 22:4x–23:0x：完成度评估落盘、CI 超时、pricing/06 入 main、派 MCP-5 PS 批

| SHA | 内容 |
|---|---|
| `6fe02c30` | `completion-assessment-2026-09-04.md`（钉 `dfd1725b`，与 08-20 同口径作差）+ `ci.yml` `timeout-minutes` 10→30。**已推**。成因：`gh run list` 实查远端 CI 自 08-12 后无一次 success——08-20→08-31 账务停摆，09-04 恢复后 run 33885570108 之前那次在 10m20s 被掐 |
| `22ddf7be`..`ba7cd976` | MCP-5 pricing/06 八笔重放（cherry-pick 零冲突；对照见票 06 Comments）；清点笔 `908755fc` 原样保留（tip 重跑生成器零差）。隔离树含 DSN 全仓 98 ok / 0 FAIL（8m04s），探针一正一反。**已推**（推前 ls-remote = 6fe02c30） |

**派 MCP-5 `f9e0bd40`：PS 批**——label-channel/24（同来源更正版本在采用口被拒为第二责任起点：先 `/domain-modeling` 裁五问、落 ADR-0117、PS 迁移 0017 起，
消费者读 `pickupVersion`、适配器按版本读回、编排分「同来源更正」与「另来源竞争」两格，AT-PS-049 一字不动）→ auto-reroute-demo-reachability/01（只复核 design.md 推进表、写结论、拆票，不实现）。
地盘 `internal/parcelshipment/**`；不碰 TF / `cmd/parcel-api` / parcelpricing。MCP-5 完工报即派，未再点名（它刚报 done、地盘已释）。

**在途**：MCP-3 `de0159af` 换号批续作（shape-gaps/01–03 + 02 余项，分支 `mcp3-pp-shapegaps` 基 ae7b4c8a，占号 22:2x）；MCP-5 `f9e0bd40`。MCP-2/4/6 仍未起。
**待派**：TF 建模批（tf/09、tf/10、TF 承运总单册）、PC 批（pc-gaps/07 → awf/06 → awf/07）、pilot-governance/01、pricing/05 05b（等 MCP-3 收口后同通道）。

### 23:2x–23:5x：远端 CI 首绿、换号批续作入 main

- **CI 绿了**：run 33887034470（`6f9436f3`）Verify 23m25s 全步 success，08-12 以来首次。`Test` 步 21m06s——对 30 分上限只剩 9 分余量，**下一步该拆 job**（`-race` 与真库分路或按上下文分片），不是再加上限；另量到 `cancel-in-progress` 让密集推送期 CI 永远跑不完（今天四次被顶掉），推送方要知道「绿」只在推送间隔 ≥ 25 分时出得来。
- MCP-3 `de0159af` done：分支 `mcp3-pp-shapegaps` 十二笔（不含清点 `9a7a3688`）在 `6f9436f3` 上 cherry-pick 零冲突 → `67921a58`..`07361d8e`，清点在 tip 重生成 `732c1db3`（PP 生产 73→82、迁移 5→8、端点 96→97、新增 SA→PP 消费缝）；隔离树含 DSN 全仓 99 ok / 0 FAIL（8m15s），探针一正一反，admin-web tsc 0 + 67/67；**已推**（推前 ls-remote = 6f9436f3）。四票（shape-gaps/01–03 票级、amount-precision/02）resolved 在票面，进 main 记录各补一条；迁移 0006–0008 用掉、**0009 释出**。MCP-3 点名的 PP CONTEXT「计算方法」词条按 ADR-0110 补「取当期序列定额」一格（随本笔）。
- MCP-3 三格待 owner：① shape-gaps/01 始发分区未落、档位对到 `FeatureAddressType`；② shape-gaps/03 委托主体默认读法待 PS owner 一句（与 lc/24 相邻）；③ E2 未开工。都在票面。
- 下一单：MCP-3 → pricing/05 05b（ADR-0105：`EvaluationIssue` 结构化「涉及序列」主体 + 子表 `evaluation_issue` + 伴生读口 + 摘要条「挂起评价数」；迁移取 0009）。已派 `2d7d655c`。

### 00:0x–00:1x（09-05）：PS 批快进入 main，派 MCP-5 TF 建模批

- MCP-5 `f9e0bd40` done：lc/24（ADR-0117、PS 迁移 0017、TF 适配器端口外读法 `FindByKeyAndVersion`）+ auto-reroute/01 resolved 并拆出 02（ready）。分支 `mcp5-ps-lc24` 已由 MCP-5 自己 rebase 到 `ce09a667`，走**快进**，十三笔 SHA 原样进 main（`24d94c94`..`11ee57aa`）。
- 隔离树含 DSN 全仓 **98 ok / 1 FAIL**：VE inbox 的 `TestOffsitePickupInboxLedgersAreSeparateFromParcelShipment`——VE 夹具信封没带 `pickupVersion`，PS 消费者自 ADR-0117 起缺版本即毒丸。MCP-5 的真库范围没含 VE inbox（它按地盘验），全仓才撞出来。MCP-1 以集成方身份补 test-only 修复 `2be1f7ee`（夹具加该格），两个 inbox 包含 DSN 重跑 ok，其余 98 包不受 test-only 改动影响；**已推**（远端 `2be1f7ee`）。教训一句已写进票 24 Comments 与广播：改 PS 消费者对信封的要求时，真库范围带上 VE inbox。
- lc spec 24 → resolved；票 24 补进 main 记录。
- **派 MCP-5 `82fd973a`：TF 建模批**——tf/10（来源更正→参与关系重派生，ADR-0112、TF 迁移 0016）→ tf-carrier-master-document-register/01（承运总单登记册，ADR-0113、迁移 0017、端点 + 读面那一格）→ tf/09（派送段声明 + 到达触发执行器，ADR-0114、迁移 0018，三条输入缝只立票）。每票一次 `/domain-modeling`，owner 授权自决口径，硬句不改。地盘 `internal/transportfulfillment/**` + TF 组接线行。
- **待派**：PC 批（pc-gaps/07 → awf/06 → awf/07）、pilot-governance/01、auto-reroute-demo-reachability/02（ready，cmd/parcel-dispatch 测试 + 本目录）。MCP-2/4/6 仍未起。

### 00:2x–00:4x：MCP-3 crash，MCP-1 代收 pricing/05 05b；pricing spec 十票全清

- 用户 00:2x 报 MCP-3 crash（截图：7/8 to-do 完成，卡在双轴评审自跑）。分支 `mcp3-pricing05b` 四步全部已提交（`92cc289f`..`867c7cc3`）、已 rebase 到 `7e0e8af0`、worktree 干净——**「每小步立刻提交」再一次救回整批**。
- MCP-1 接手：隔离检出钉 `867c7cc3`——gofmt 空、build/vet 0、含 DSN 全仓 **99 ok / 0 FAIL**（8m17s）、探针 `TestPendingSeriesEvaluationsAreCountedByKindFromTheChildTable` 一正一反、摘要不变用例 PASS、admin-web tsc 0 + 70/70；按 ADR-0105 六条复核领域两处产出点 / 迁移 0009 / 写侧同事务 / 读口 SQL / 端点 `asOf` 与放行表 / 摘要条不摆 0——无需改。票 05 转 resolved + 完成记录 `42238d8d`，清点 `d089ce0b`；快进推，**远端 main = d089ce0b**。`2d7d655c` 代结 done。
- **pricing-reference-series-operations 十票全部 resolved，父 spec 转 resolved**（本 09-04 一天里 05b/06/10 三票收口）。parcelpricing 今日：迁移 4→9、端点 8→10、ready 票清零；余下只有 shape-gaps/03 主单级（等 TF 立册，MCP-5 在做）与 E2。
- 现只剩 MCP-5 一路（TF 建模批）。**待派**不变：PC 批、pilot-governance/01、auto-reroute/02；MCP-1 在等 MCP-5 完工报的间隙可自办 pilot-governance/01 或 auto-reroute/02（小），PC 批留给下一个起来的通道。

## 2026-09-07 10:2x 通道 1 新会话接续（接手时 `main = 2fcc9378`，远端 `ce47b89f`，领先一笔未推）

上一会话 09-05 00:4x 后自办了 pilot-governance/01：隔离分支 `mcp1-pg01`（`0d0485c9`）重放进 main 为 `2fcc9378`（01:09），票面转 resolved；但**清点未重生成、未推、本节未记**，会话随后结束。09-05 01:09 至今日 10:2x 全员无提交。

### 10:2x 全面审查（钉 `2fcc9378`，只读）

- 共享树 69 处 ` M` 全为 CRLF 幻影（`git diff --numstat` 两种口径 0 行），无未跟踪、无 stash。含 DSN 全仓在共享树跑：98 ok / 1 FAIL——唯一红 `migrations` 包 `TestEmbeddedMigrationAssetsCarryNoCarriageReturnOrBOM`，指工作副本三份 `.sql`（PS 0008、PC 0015/0016）带 CRLF、索引为 LF，属幻影副作用不属提交内容。
- 票务 225 张关 208（resolved 204 / done 2 / superseded 1 / handed-off 1）；未关 17：ready 1（auto-reroute/02）、in-progress 1（fti/01，MCP-2 本日 10:33 转 resolved `2058ebdb`）、draft 9（tf/09、tf/10、tf-carrier-master-document-register/01、pc-gaps/07、awf/07、ftr/10、ve-claims/04、lc/20、lc/22）、blocked 2（awf/06、ftr/03）、needs-info 4（归用户）。
- MCP-5 `82fd973a` TF 建模批停在分支 `mcp5-tf-batch`：`99f81123` ADR-0112 + `49bea998` TF 领域层（替代参与链、重建门核链形），tf/10 in-progress、tf/11 draft 已立；**最后一笔 09-05 00:28，无完工报**；余下编排 `rederiveFulfillmentParticipation`、迁移 TF 0016、登记册反查口未落，承运总单册与 tf/09 未开工。
- 远端 CI：`ce47b89f` success（09-04 23:2x 首绿后保持）。
- worktree 9 棵：`mcp1-pg01` 本节拆（见下）；`mcp4-bk`、`mcp6-d4ps`、`idp-ppclean` 与 main 零唯一内容，`mcp2-tf02`/`mcp6-fti` 各剩一笔已被后续清点盖过的清点笔，均可拆未拆；`mcp5-tf-batch` 在途不拆；`idp-tf03` salvage 归用户。

### 10:5x 推送方作业：清点补笔、验证、推

| SHA | 内容 |
|---|---|
| `2058ebdb` | MCP-2：fti/01 票面转 resolved（纯 .md，本通道推前落在 `2fcc9378` 之上，作祖先一并上远端；MCP-2 10:3x 占号广播已言明只动这一文件） |
| `b621cb43` | 清点在 `2fcc9378` 干净检出上重生成（pilotgovernance 生产 19→20 / 测试 17→18；迁移 pilot_governance 5→6，合计 140→141）。`2058ebdb` 不含生成器计数的文件，在 `b621cb43` 检出上重跑生成器零差 |
| 本笔 | tasks.md 本节 |

验证（隔离 detached 树 `idp-verify-pg01` 钉 `b621cb43`）：gofmt 空；build/vet 退 0；清点门在 tip 重跑零差；含 DSN `go test -p 1 -count=1 ./...` 退 0，**99 ok / 0 FAIL**（约 8m）；探针 `TestChannelExecutionCommandIsClosedOnBothSides` 带 DSN PASS / 不带 SKIP。admin-web 未重验（本批不含前端改动，上次 70/70 钉 `867c7cc3`）。

杂务：`git checkout --` 归一三份 `.sql` 工作副本到 LF（内容零差，`ls-files --eol` 复核 `w/lf`），共享树 `migrations` 包回 ok；拆 `idp-parcel-mcp1-pg01`（`git cherry main` 全 `-`、九文件 diff 空、status 零行，不带 `--force`），指针 `mcp1-pg01@0d0485c9` 保留。

**待派**不变：PC 批（pc-gaps/07 → awf/06 → awf/07）、auto-reroute/02；MCP-5 TF 批去留待定（续做或换人从 `49bea998` 接）。MCP-1 接下来自办 auto-reroute/02，再看 pc-gaps/07。

### 11:0x 点名与派工（钉 `ffa6bd0e`；上一节末句「自办 auto-reroute/02」作废——该票 10:43 已被 MCP-2 裁 blocked `b2a78a2e`，等 ftr/03）

- 11:0x 点名（截止 11:06）：MCP-3/4/5/6 截止前应答空闲、无地盘、余量充足；MCP-2 在 ftr/10（隔离 `mcp2-ftr10`，PS 地盘：`ports.go` 三方法加版本参 + `acceptance_judgments*` + 五个编排 + parcel-api unwired + dispatch 两测试，跨文件签名变更，重放时要核）。MCP-5 是新会话，自报对 `mcp5-tf-batch` 无在途记忆。
- MCP-5 旧 worktree 现场（10:54 量）：4 个 `.go`（+299/−43）+ 未跟踪迁移 `0016_fulfillment_participation_supersession_chain.sql`（47 行），mtime 09-05 00:33–00:37；派单第 0 步按「未提交现场」原样封存一笔再动。

| 通道 | 单号 | 内容 | 独占地盘 |
|---|---|---|---|
| MCP-3 | `60fd658d` | PC 批：pc-gaps/07（/domain-modeling → ADR-0115 → 迁移 PC 0024 起 → 正文表 + 登记口）→ awf/06（`?kind=` 加格）→ awf/07（只拆票） | `internal/partycommercial/**`、PC 迁移、`cmd/parcel-commercial`、parcel-api PC 组行、admin-web PC 页；不碰 SA |
| MCP-5 | `854580ed` | 【接管重派】TF 批：封存现场 → tf/10 续（ADR-0112 已在分支，迁移 0016）→ 承运总单册 01（ADR-0113、0017）→ tf/09（ADR-0114、0018） | `internal/transportfulfillment/**`、TF 迁移、parcel-api TF 组行；不碰 PS |
| MCP-4 | `df710418` | CI Test 步拆分（按上下文分片、覆盖核对进快 job、concurrency 给理由、timeout 按片）；顺带拆 `mcp4-bk` 树 | `.github/workflows/ci.yml`、`scripts/ci/` |
| MCP-6 | `2fd68c18` | 棘轮基线余 9 条：PP 4 条接线或裁（代码）、PS/PC 5 条只读取证立票到 `.scratch/wiring-baseline-remainder/`；顺带拆 `mcp6-d4ps` / `mcp6-fti` 树 | `internal/parcelpricing/**`、`production_wiring_baseline.txt`（占号） |

**归用户仍不变**：needs-info 四张、ftr/03（`PAR-NET-14` 实例值）、ve-claims/04 三问、ADR-0109/0111 复核、tenant-implementation-01 三份仓外动作。**MCP-1** 本轮只做推送方与集成（重放、干净检出验、清点兑底、推），不再自办票；lc/20、lc/22 等第一家真源，不派。

### 11:2x–11:5x：ftr/10 进 main，派 MCP-2 PS 三口

- MCP-2 11:2x 完工报 ftr/10（分支 `mcp2-ftr10` 基 `3b411556`：代码 `6d325ca7` 16 件——迁移 PS 0018 两表加 `submission_version`、`ports.go` 三方法加 `SubmissionVersionID`、五编排 + http 读口 + parcel-api unwired 桩 + dispatch 两测试；清点 `c0ced2ec`）。核 `3b411556..c895dad8` 中间只有 .md，无重叠，直接重放。
- 隔离树 `idp-replay-ftr10`：cherry-pick 零冲突 → `6a42e6c7`；清点在其上重生成 `0f84c0ec`（parcel_shipment 17→18，分支清点笔不重放）。验证钉 `0f84c0ec`：gofmt 空、build/vet 0、清点门零差、含 DSN 全仓 **99 ok / 0 FAIL**、探针 `TestJudgmentsBelongToTheSubmissionVersionTheyWereFormedFor` 与 `TestACustomerSupplementWaitIsResumedByTheNewSubmissionVersionEnvelope` 一正一反。共享 main 由 `c895dad8` **快进**到 `0f84c0ec`，推 `0f84c0ec:main`（推前 ls-remote = 2efef58e），**远端 main = 0f84c0ec**；分支 vs main 16 文件 diff 空，MCP-2 可拆树。
- 派 MCP-2 `0c472fed`：PS 端口余三口（`LabelValidityRuleView` / `SourceDataRuleDeclaration` / `SourceDataAmendmentAuthorizer`）逐口 /domain-modeling 裁机制半边能立什么（先例 lc/19 规则目录、pc-gaps/05 消费缝），裁清的实施、须等裁的立 draft 到 `.scratch/ps-port-remainder/`；基线 `0f84c0ec`，地盘 PS，不碰 PC/TF。
- 在途：MCP-2 `0c472fed`、MCP-3 `60fd658d`、MCP-4 `df710418`、MCP-5 `854580ed`、MCP-6 `2fd68c18`。CI 为 `0f84c0ec` 起跑，25 分内不推 .md。

### 11:2x–11:3x：ps-port-remainder 三票落地、MCP-1 十问代裁、派 MCP-2 续单（上一会话做了没记，本节补）

- MCP-2 `0c472fed` done（11:24 `5419da13`）：三口逐口 `/domain-modeling`，立 `.scratch/ps-port-remainder/` 01–03 draft——三口机制半边的头一半都在 PC（终局规则上的有效期声明 / 规则包下第三族阶段内容声明 / `AuthorizedAction` 加「资料修订」格），PS 单方面无可落代码，不立新 ADR；票面留十问。
- MCP-1 按 owner 授权代裁十问（11:37 `054479f2`，逐票 Comments）：01/03 转 blocked 等 PC 半边（并入 pc-gaps 批，MCP-3 当前批后 pc-gaps/08 起，已知会）；02 转 in-progress——PS 半边拆两段，不依赖 PC 的那段（词条「资料修订阶段」、`AmendmentStage`、`SourceDataAmendmentQuery.Stage`、编排问矩阵前先判阶段、CC/NO 阶段事实取消费侧读口形 + ADR-0118）现在开工，依赖 PC 的那段 Blocked by PC 半边；03-Q3「生产入口现在接」拆出 04（in-progress）。ftr/10 票面补 main 上的 SHA 对照（11:25 `35607205`）。
- 派 MCP-2 `b77525c9`（11:30）：三票落裁决 → 04 资料修订生产入口机制半边 → 02 不依赖 PC 那段（含 ADR-0118）。基线 `0f84c0ec`，分支 `mcp2-ps-ports`，地盘 PS，不碰 PC/TF。

### 11:5x–12:0x 通道 1 新会话接续（接手时 `main = 054479f2`，远端 `0f84c0ec`，领先四笔纯 .md）

只读盘点（钉 `054479f2`）：共享树 67 处 ` M` 全为 CRLF 幻影（`--ignore-cr-at-eol` 零行），无未跟踪、无 stash。远端 `0f84c0ec` 的 CI（run 34079214478）**success 24m0s**——上一节「等 CI」的条件已满足。五路在途的分支现场：

| 通道 | 分支 | 现场 |
|---|---|---|
| MCP-2 `b77525c9` | `mcp2-ps-ports` | 仍在基线 `0f84c0ec`，零提交（刚开工） |
| MCP-3 `60fd658d` | `mcp3-pcgaps07` | 四笔：ADR-0115 `359b10bd` → 迁移 PC 0024 + 领域正文 + 写口/读口 `4e5fc6a6` → 发布用例 + CLI 批文 + 真库读回 `d2d375ac` → 清点 `2939d2d9`（11:35）；另有 `mcp3-verify` 钉同 SHA 自验中。pc-gaps/07 在分支上已完整，awf/06、07 未动 |
| MCP-4 `df710418` | `mcp4-ci-shard` | 一笔 `6f70c8d7`：Test 步按上下文拆四片 |
| MCP-5 `854580ed` | `mcp5-tf-batch` | 已 rebase：ADR-0112 `00cc28f4`、TF 领域层 `8b70c052`、第 0 步封存 `7e7f0e25`（11:06）；此后无提交，tf/10 余项（编排、迁移 0016、登记册反查口）未落 |
| MCP-6 `2fd68c18` | `mcp6-pp-ratchet` | 两笔：剪 PP 三条死码、`ReplayPricingEvaluation` 留待并写明调用方 `1d13d510`；立 `wiring-baseline-remainder/` 01–07 `28ea268b`（11:36） |

推送方作业：拆 `idp-parcel-mcp2-tf02`（`git cherry main` 只剩清点笔 `6a3f190e` 一个 `+`，已被后续清点盖过；status 零行）与 `D:/tops/idp-ppclean`（`mcp5-pp-unwired-cleanup`，`git cherry` 零 `+`；status 零行），不带 `--force`，指针保留。`idp-tf03` 只剩 salvage 笔，归用户，不动。本笔 tasks.md 之后把五笔纯 .md 快进推出（推前 ls-remote = 0f84c0ec）——纯 .md 不做隔离树验证。

**归用户仍不变**；**待派**：无（五路都在途）。MCP-1 继续只做推送方与集成。

### 12:0x 全员重启，点名后五单接管重派（钉 `08f54867`）

用户 12:0x 报「五路现在都空闲」——上一节量到的五棵在途树是**旧会话中断留下的现场**，不是在做。拆树前那次盘点只看了分支提交，没看工作副本；补量（`status --short --untracked-files=all`）：`mcp2-ps-ports` 6 M + 6 ??（04 生产入口走到半路：`assemble_customer_amendment`、`amend_customer_source_data` 适配器、`unconfigured_source_data_amendment`、endpoints 三件）；`mcp3-pcgaps07` 11 M（awf/06 走到半路：admin-web party 五文件、`query_commercial_policies.go` 分派、`pre_acceptance_financial_control_policy*`、`ports.go`）；`mcp5-tf-batch` 6 M + 3 ??（tf/10 编排层：`rederive_fulfillment_participation.go` 与两份测试）；`mcp4-ci-shard`、`mcp6-pp-ratchet` 干净。pc-gaps/07 票在分支上仍 in-progress、无完成记录。**教训一句**：判「在途」要量工作副本，分支 tip 的时刻只说明最后一次提交，说明不了此后有没有人在改。

12:02 点名（截止 12:06），12:03 前五路全部应答空闲、无地盘、余量充足（MCP-2 自报 12 件未提交未动）。按 parallel-sessions「未提交现场」与「接手别人在途产出」派：有现场的三单第 0 步先 `chore(salvage)` 封存，五单第 1 步都是「分析现场报告 ≤15 行报 MCP-1，写完才准改」，再续原单余项。旧五单在台账结 failed「承接方换人（会话重启）」。

| 通道 | 新单 | 接管内容 | 独占地盘 |
|---|---|---|---|
| MCP-2 | `d5558bc6` | 封存 → rebase 到 `08f54867`（要 054479f2 的票面）→ 04 生产入口机制半边收口 → 02 PS 半边不依赖 PC 那段 + ADR-0118 | PS、`cmd/parcel-api` PS 组行、CONTEXT-MAP 两条箭头 |
| MCP-3 | `65d86426` | 封存 → pc-gaps/07 补验证 + 完成记录 → awf/06 → awf/07 拆票 → 立 pc-gaps/08–10 draft（代裁排进 PC 批的三件） | PC、PC 迁移、`cmd/parcel-commercial`、admin-web PC 页；不碰 SA |
| MCP-4 | `479aefee` | 分析 `6f70c8d7` → YAML 核 / 分片覆盖核对（WSL）/ 每片本机跑 → 完工报 | `.github/workflows/ci.yml`、`scripts/ci/` |
| MCP-5 | `edb0a8c9` | 第二次封存 → tf/10 收口 → 承运总单册 01（ADR-0113、0017）→ tf/09（ADR-0114、0018） | TF、TF 迁移、parcel-api TF 组行；不碰 PS |
| MCP-6 | `f31a5650` | 分析两笔 → 记数 / 理由行 / 门禁 / 清点 / 验证补齐 → 完工报 | PP、`production_wiring_baseline.txt`、`.scratch/wiring-baseline-remainder/` |

`idp-parcel-mcp3-verify`（detached 钉 `2939d2d9`，status 零行）交 MCP-3 核后自拆。CI 为 `08f54867` 在跑，本笔 tasks.md 不单独推，随下一次重放一并上。

### 12:1x–12:2x：五份现场报告齐、CI 拆片进 main

现场报告（12:1x 内五路全到，各自转 working）与我回的裁定：MCP-4 `6f70c8d7` 五条全落，自量到脚本包清单缓存在子 shell 里从不命中 → 我裁「行为对上注释优先」；MCP-2 封存 `42f0035a`、rebase 到 `08f54867`（tip `38c2aa82`），04 机制半边在旧现场里已落且含 DSN 三用例 PASS → 先写自己 red 再交叉验证，收口先报一次；MCP-6 基线两法同得 9→6 钉 `0f84c0ec`，票 07 核成真缺陷 → 转 ready 作其下一单不并入，两处测试偏离进提交信；MCP-5 第二次封存 `114d2b30`，叠起来含 DSN 1470 PASS / 1 FAIL（HTTP 旧断言「更正不进段」在 ADR-0112 下过时）→ 「起点晚于继承终点」补具名拒绝格、不静默不自动改；MCP-3 封存 `8dc99b9f`，pc-gaps/07 四笔对 ADR-0115 六条全对上、干净检出含 DSN 99 ok / 探针 6 PASS。**规矩一条广播 2/3/5**：`chore(salvage)` 不进 main，每段完工报前在隔离分支把封存笔与其后自己的笔重切成按意图的提交，证据句移进首笔提交信。

MCP-4 `479aefee` done（12:2x）：`6f70c8d7` + 补笔 `6279adcd`（缓存挪到主 shell，`check` 152s→13s，四片清单逐字不变）。重放：隔离 detached 树基 `9e7b5577` cherry-pick 零冲突 → `3337ee27`、`90951d22`；与分支 `git diff` 空；两文件 `i/lf w/lf`、CR=0、无 BOM、脚本 100755。验证（隔离树）：PyYAML safe_load 通过；WSL `test-shards.sh check`——main-chain 30 / network-visibility 30 / customs-transport 14 / rest 41 = 115 包无重叠无遗漏无空片，15s；清点门在 tip 重跑零差（.sh/.yml 不进清点）。Go 一字未动，不重跑全仓（MCP-4 已按 CI 同一片段四片 Windows 含 DSN 各跑一遍，全绿零 SKIP）。共享 main 由 `9e7b5577` 快进到 `90951d22`；本笔之后推。等 run 的：各片 ubuntu 上 `-race` 实际分钟、`shards→test` 的 `fromJSON` 矩阵接线、service 容器健康、main 上不取消的实际行为——MCP-4 盯 run，出结论前不拆它的树。**已推**（远端 `77e993e3`，推前 ls-remote = 08f54867）。

### 12:3x–12:4x：pc-gaps/07 进 main

MCP-3 `65d86426` 第 2 步完工报（12:2x）：分支 `mcp3-pcgaps07` 四笔 + 收口笔 `7acf6859`（纯 .md：票 07 resolved + 完成记录；SA 后继票 `sa-preacceptance-policy-view/02` draft），跳过封存笔 `8dc99b9f`。重放：隔离 detached 树基 `77e993e3` cherry-pick 零冲突——`359b10bd→6dc3db12`、`4e5fc6a6→ec0e2284`、`d2d375ac→eb09c1ed`、`7acf6859→265da8d8`；与分支代码 tip `2939d2d9` 除清点/.scratch/CI 外 diff 空；新/改 `.go`/`.sql` 全 `i/lf w/lf`。清点在 `265da8d8` 干净检出重生成 `2ced791a`（PC 生产 84→86 / 测试 84→86、端口 27→28；迁移 party_commercial 23→24，合计 142→143；端口声明 347→348），与分支清点笔 `2939d2d9` 内容零差，分支清点笔不重放。验证钉 `2ced791a`：gofmt 空；build/vet 退 0；含 DSN `go test -p 1 -count=1 ./...` 退 0，**99 ok / 0 FAIL**（541s）；探针 `-run ControlPolicy`（PC postgres 包）无 DSN SKIP 6 / 含 DSN PASS 6。未跑 `-race`（CI 四片各带）。共享 main 由 `77e993e3` 快进到 `2ced791a`；本笔之后推。MCP-3 转第 3 步 awf/06（建在 `8dc99b9f` 之上，完工报前重切）。

## 2026-09-07 12:5x 通道 1 换人：通道 4 的会话接管（接手时 `main = origin/main = 95182b9d`）

原通道 1 会话 12:5x crash（最后一笔台账 `95182b9d` 12:4x）；MCP-3 亦于 12:4x crash（awf/06 停在「先写 red 再读封存现场」）。用户在通道 4 面板指示该会话改用 `idp-mcp-1` 接通道 1，在通道 6 面板指示 MCP-6 接 MCP-3 余量。接手时通道 1 队列里三件：MCP-5 tf/10 完工报、MCP-6 接管报、MCP-4（即本会话）`-count=1` 完工报。`load_progress` 的存档停在 09-04 17:48，不作依据；现场按 `query_tasks` 与各分支实量。

| SHA | 内容 |
|---|---|
| `9f8e4c94` | MCP-4 CI 四片钉 `-count=1`（`mcp4-ci-count1` 基 `95182b9d`，一笔纯 ci.yml，快进推）。取证：run 34083174177 四片各 4/20/12/9 = 45 包 `(cached)`，旧形基线 run 34081438431 同为 45/99——setup-go `cache:true` 还原的 go-build 缓存带测试结果，那些包在 CI 上没重跑。**已推**（推前 ls-remote = 95182b9d） |
| `64332437`..`366c8c13` | MCP-5 tf/10 十笔重放（分支→main 对照见 13:3x 广播）。README 索引与 main 的 0115 行冲突，两行都留、按号排。**两笔封存笔 `7e7f0e25`/`114d2b30` 按 12:1x 规矩不以 chore(salvage) 进 main：改写提交信为意图（`f7e3e5ad`/`4785566d`），tree 一字未动**——请作者回两段提交信，十分钟未回，推送方按 diff 所见符号与文件自写，提交信末尾注明原 SHA 与「未经作者复核」 |
| `915c0d55` | 清点在 `366c8c13` 干净检出重生成（TF 生产 118→119 / 测试 106→109；迁移 transport_fulfillment 15→16，合计 143→144）；分支清点笔 `968b0fa1` 量的是基线 `2efef58e` 上的数，不重放。**已推**（推前 ls-remote = 9f8e4c94） |

验证（隔离 detached 树 `idp-replay-tf10`：先在改写提交信之前的 `d9bcfbb9` 验，改写后 tip `915c0d55` 的 tree 与之相同 `59bdb11d`）：gofmt 空；build/vet 退 0；清点工具 vet/test 退 0；清点门在 tip 零差；含 DSN `go test -p 1 -count=1 ./...` 退 0，**99 ok / 0 FAIL**（550s）；探针 `TestAPickupCorrectionSupersedesTheParticipationInTheDatabaseWhileTheRootRowStays` 无 DSN SKIP / 有 DSN PASS。改动 `.go`/`.sql` 全 `i/lf w/lf`。

**新 CI 形态两个 run 的数**：`77e993e3` run 34083174177 success 6m23s，四片 Test 步 284–325s——但 45 包 `(cached)`，且旧形基线同为 45/99，所以「20m25s → ≤325s」是同缓存态同代码的对比；`9f8e4c94` run 34084625114（`-count=1`）success 8m21s，Test 步 rest 294s / main-chain 313s / network-visibility 305s / customs-transport 441s，**cached 全 0**、FAIL / DATA RACE / DSN 缺失三零。最重 customs-transport job 487s = 15 分的 54%，未过 `23733b74` 定的六成线，timeout 与分片边界不动。仍未量：go.sum 变动后编译缓存也冷的 run。

台账清账：`65d86426`（MCP-3）结 failed「承接方换人（crash）」，余量派 MCP-6 `faee318d`（13:1x 已转 working：awf/06 先自写两片 red 再读 `8dc99b9f` 交叉，判据全对上，沿用其实现）；`479aefee`、`23733b74`（MCP-4）done；09-04 旧单 `82fd973a` 结 failed「已被接管重派取代」。

拆树：`idp-replay-tf10`、`idp-parcel-mcp4-ci-count1`（内容 = main）不带 `--force` 拆除。`idp-parcel-mcp3-pcgaps07`（status 零行；`git cherry main` 只剩封存笔 `8dc99b9f` 一个 `+`，留在指针上）`git worktree remove` 退 255 `Directory not empty`——**树里 `apps/admin-web/node_modules` 是指向主树的 junction**（parallel-sessions 那条「不能 `Remove-Item -Recurse`」这次是真撞上了）：登记已摘、`.git` 链接已删，先 `cmd /c rmdir` 只拆链接（主树 node_modules 前后各 15 项），复核无其它 reparse point 后再清壳。指针 `mcp4-ci-shard@6279adcd`、`mcp4-ci-count1@9f8e4c94`、`mcp3-pcgaps07@7acf6859` 保留。

**在途**：MCP-2 `d5558bc6` 第 3 段（ADR-0118；CONTEXT-MAP 与 README 各一小 hunk 已在分支 `8430ae09`，13:1x 释号）；MCP-5 `edb0a8c9` 第 3 步承运总单册 01（建在 `1bc1ccba` 之上，重放只取其后的笔）；MCP-6 `faee318d` awf/06。**归用户不变**。CI 为 `915c0d55` 在跑。

### 13:3x（上一会话做了没记，本节补）：awf/06 快进入 main

MCP-6 `faee318d` 第 1 步完工报后，上一会话把 `mcp6-awf06` 三笔原样快进（`a1890506` 后端第九册 / `ea2293e5` 管理台 / `ce83eaf9` 票 06 resolved），
清点在 `ce83eaf9` 干净检出重生成 `92579b0a`（partycommercial 测试 86→88；分支清点笔 `d6d35bee` 量的是 `95182b9d` 上的数，未重放），**已推**，
远端 CI success（run 34086803851，6m52s）。上一会话随后开始重放 `mcp6-pp-ratchet`（`idp-replay-pp-ratchet@16712373` 基旧 main、
`idp-replay-ppr@0fcbed6e` 基 `92579b0a`，13:29），**重放完了、验证 / 快进 / 推没做**，会话在这一步结束。

## 2026-09-07 15:3x 通道 1 新会话接续（接手时 `main = origin/main = 92579b0a`）

用户只交代「监听队列、正常回复、保持循环」，随后「检查工作树和 tasks.md」→「可以并行开工了吗，mcp-2/3/4/5/6 空闲」。

### 15:3x 只读盘点（钉 `92579b0a`）

- 共享树 67 处 ` M` 全为 CRLF 幻影（`--ignore-cr-at-eol` numstat 零行），无未跟踪、无 stash。
- 两支分支 `git cherry main` 全 `+`：`mcp2-ps-ports@71b8817b`（8 笔，ps-port-remainder/04 resolved + 02 PS 半边 + ADR-0118 + 新立 05，含封存笔 `38c2aa82`）；
  `mcp6-pp-ratchet@980e6316`（5 笔，PP 剪三条死码 + wiring-baseline-remainder/01–07）。
- 台账 working 两单的真实现场：MCP-6 `faee318d` ② awf/07 拆票停在**未提交现场**（`idp-parcel-mcp6-awf06` 树上 11 个 `??`，issues/08–18，mtime 13:13–13:16，伞票 07 未改）；
  MCP-5 `edb0a8c9` 第 3 步零进展（树 detached 在 `6fe2c6b9`——12:44 它自己重切封存笔的一笔——干净，另留 `mcp5-tf-batch-before-recut`）。五路 `list_sessions` 全
  running/idle，最后活动 12:29–13:15。
- 票务：未 resolved 17——draft 8（lc/20、lc/22、tf/09、tf/11、tf-carrier-master-document-register/01、awf/07、ve-claims/04、sa-preacceptance-policy-view/02）、
  blocked 4（ps-port-remainder/01、/03 等 PC 半边；ftr/03；auto-reroute/02）、in-progress 1（ps-port-remainder/02）、needs-info 4。

### 15:38 点名 → 15:4x 五单派满（截止 15:43，五路全部应答空闲、无地盘、新会话无在途记忆）

旧两单结 failed「承接方换人（会话重启）」。ADR 预留：TF 0113/0114/0121，PC 0116/0119/0120，SA 0122，PP 0123，CC/NO 0124；迁移：TF 0017 起、PC 0025 起、
SA 0017 起、PP 0010 起、CC 0017 起、NO 0004 起、PS 0019 起。端点表各在自己分支只改本上下文那一组行，重放顺序由 MCP-1 定。

| 通道 | 单号 | 内容 | 独占地盘 | 基线 |
|---|---|---|---|---|
| MCP-5 | `895fbabf` | 【接管重派③】TF 批：tf-carrier-master-document-register/01（ADR-0113、0017）→ tf/09（ADR-0114、0018；三条输入缝只立票）→ tf/11 | TF + TF 组接线行 + TF CONTEXT/GLOSSARY「总单」 | `92579b0a` |
| MCP-6 | `aafca372` | 【接管重派】PC 批余量：封存 11 张子票 → 收伞票 07 → 立 pc-gaps/08–10 draft → 余量够接 08/09/10 | PC + `.scratch/admin-write-faces` + `party-commercial-context-gaps` | `92579b0a`（在原树 `git checkout -b mcp6-awf07`，封存 `4871236d`） |
| ~~MCP-3~~ | ~~`13f5a087`~~ | sa-preacceptance-policy-view/02（五问 → ADR-0122 → 实施） | SA | 用户 16:0x 报 MCP-3 **收到前已 crash**，单结 failed「派工错位」，**MCP-1 自办**（分支 `mcp1-sa02`），通道 3 队列留作废令 |
| MCP-4 | `d3c3a7ac` | wiring-baseline-remainder/07（Decimal 重建边界；票内裁甲/乙/丙，ADR 0123）→ 06 若 ready | PP + `production_wiring_baseline.txt` PP 段 | 等 pp-ratchet 进 main（16:00 广播 `0fcbed6e`） |
| MCP-2 | `2f035050` | ps-port-remainder/05：CC/NO 各一面按正式包裹引用读阶段事实 → PS 两只 Unconnected* 换真读法 + 装配接真 | CC/NO ports+postgres、PS adapters/{customscompliance,nodeoperations}、`assemble_customer_amendment` | 等 ps-ports 进 main（16:12 广播 `61344989`） |

### 15:5x–16:1x 推送方作业：两路重放入 main

| SHA | 内容 |
|---|---|
| `7cef122f`..`0fcbed6e` | `mcp6-pp-ratchet` 五笔（上一会话的重放件原样采用：1d13d510→7cef122f、28ea268b→8c85e731、2837718d→77ff8eb8、33ddafff→2ea716ba、980e6316→0fcbed6e）。**已推**（推前 ls-remote = 92579b0a），CI success 7m48s |
| `8dfddf46`..`9091f539` | `mcp2-ps-ports` 六笔重放（38c2aa82→8dfddf46、a1a1d16f→a12de35c、fcbd13fa→7254d401、7ec02162→d4bc4785、3da37e07→45ed2d04、8430ae09→9091f539；README 自动合并）。封存笔 `38c2aa82` 按 12:1x 规矩改写提交信为意图（tree 一字未动，与首次 cherry-pick 结果 tree 相同），未经作者复核——通道 2 已换会话 |
| `61344989` | 清点在 `9091f539` 干净检出重生成（parcelshipment 生产 133→139 / 测试 133→138 / http 13→14；cmd/ 生产 52→53；接入面端点 98→99；消费缝 19→20 组，新增 PS→CC；端口声明 348→352）；分支两笔清点 `baff5fdd` / `71b8817b` 未重放。**已推**（推前 ls-remote = 0fcbed6e） |

验证（各自隔离 detached 树）：`0fcbed6e`——gofmt 空、build/vet 0、清点门零差、含 DSN `go test -p 1 -count=1 ./...` **99 ok / 0 FAIL**（513s）；
`61344989`——gofmt 空、build/vet 0、含 DSN **100 ok / 0 FAIL**（534s，新包 `parcelshipment/adapters/customscompliance`），探针 `cmd/parcel-api -run Amendment`
无 DSN SKIP 4 / 含 DSN PASS 4；改动 `.go` 全 `i/lf w/lf`。

拆树（均 status 零行、不带 `--force`）：`idp-parcel-mcp5-tf-batch`（detached `6fe2c6b9` 先补指针 `mcp5-tf-batch-recut`）、`idp-parcel-mcp6-awf06-verify`、
`idp-replay-pp-ratchet`、`idp-replay-ppr`、`idp-parcel-mcp6-pp-ratchet`（`git cherry` 五笔全 `-`）、`idp-parcel-mcp2-ps-ports`（`git cherry` 只剩两笔清点与 README
自动合并那一笔 `+`，内容都在 main）、`idp-replay-psports`。`mcp6-awf06` 树交 MCP-6 续用（已切 `mcp6-awf07`）。`idp-tf03` 归用户不动。

**在途**：MCP-2 `2f035050`、MCP-4 `d3c3a7ac`、MCP-5 `895fbabf`、MCP-6 `aafca372`；MCP-1 自办 sa-preacceptance-policy-view/02。**归用户不变**：needs-info 四张、
ftr/03、ve-claims/04 三问、ADR-0109/0111 复核、tenant-implementation-01 三份。CI 为 `61344989` 在跑。

## 2026-09-07 17:4x 通道 1 新会话接续（接手时 `main = origin/main = e4fa61d7`）

用户只交代「监听队列、正常回复、保持循环」。接手时通道 1 队列里三件：17:1x 一条「原通道 2 会话按用户指令绑到 idp-mcp-1 接管集成」的广播回声、MCP-6 两条 awf/07 完工报（17:1x）——三件到 17:45 都没人消费。盘上（17:47 量得）：`idp-replay-psr05-tf01` 停在 `e33f2076`（17:18:50 提交，此后树内无新文件、无 go test 进程）；`ls-remote` 仍 `e4fa61d7`。按「截至 17:47 未响应」处置：17:49 广播接管（若那个接管会话仍在请回声即停手），无人回声；MCP-2 / 5 / 6 的回声都自报为新会话、无在途记忆、无异议。

### 16:1x–16:5x（上一会话做了没记，按 git log 补；本会话未经历，验证强度以各提交信与 CI 为准）

`80641ddc..e4fa61d7` 十笔：wiring-baseline-remainder/07 四笔（`ef1a8315` ADR-0123 / `8bbb43c4` Decimal 规范写法进 `valid()` / `008721d8` 基线留言 / `d01d103a` 票 07 resolved；提交信记的是 MCP-4 分支 `1b1a2980 / f1b21c47 / 223d6145` 的重放，清点笔 `305402f5` 未重放）；sa-preacceptance-policy-view/02 五笔（`c5fe75bc` 域 / `59b8a70a` 编排 / `316fe903` 适配器 / `d835c8b9` ADR-0122 + 票 02 resolved + 新立 03 draft / `3855f64b` 双轴评审修补；分支 `mcp1-sa02@f96e74dc`）；清点 `e4fa61d7`（16:50 推）。

### 17:1x–18:0x：三支分支进 main

| SHA | 内容 |
|---|---|
| `3b1afabe`..`cbfaf436` | `mcp2-psr05` 四笔（13f3ba65→3b1afabe、72b77de0→7b8aeecf、f91100c2→79f82432、6ab8ddc2→cbfaf436；ps-port-remainder/05 resolved）。**17:1x 接管会话所重放**，本会话 `range-diff` 复核全 `=`（cbfaf436 仅上下文行差：main 上票 02 多一段 16:1x「进 main 记录」）。分支清点笔 `31b8306a` 未重放 |
| `c0363e12`..`603abcec` | `mcp5-tf-cmdr` 十二笔（bb863b1e→c0363e12、ac408e78→2e394faa、3130afa8→7491bae2、096f8a70→8c867069、2f844832→89f23d92、c40a8e41→700ba33b、e7186dec→18760165、9fb6fdf1→820c68be、badd2627→f56f6107、ee6078c5→9787577e、d8d2a592→509c66f0、9163c3b9→603abcec；tf-carrier-master-document-register/01 resolved）。同为接管会话所重放，`range-diff` 全 `=`（2e394faa 仅 README 上下文行差）。分支清点笔 `c0d92fcc` 与 tf/09 在途三笔 `d11d45c4 / a5e9ba31 / 8f079e66` 未重放 |
| `e33f2076` | 清点在 `603abcec` 干净检出重生成（接管会话所作：CC 生产 74→75、NO 29→30、TF 生产 119→125、接入面端点 99→101、迁移 144→147、端口声明 352→355） |
| `e52c71b6`..`e374b1ea` | `mcp6-awf07` 五笔本会话重放（42224e35→e52c71b6、e9452c15→9f60058b、8c1e6ed5→52001f02、2417b7bb→d1a03e00、c9ec35d7→e374b1ea；awf/07 收伞 + pc-gaps/08–10 draft + ADR-0116 + PC CONTEXT「合同委派」+ 票 08 in-progress）。cherry-pick 零冲突，README 自动合并、0116 行落在 0115/0117 之间；tip 重生成清点零差，无清点笔。**已推**（推前 ls-remote = e4fa61d7；18:04:16 ls-remote = e374b1ea） |

验证钉 `e374b1ea`（隔离 detached 树 `idp-replay-mcp1-awf07`，验后已拆）：gofmt 空；build/vet 退 0；清点工具 vet/test 退 0；清点门零差；WSL `test-shards.sh check` 31/30/14/41 = 116 包无重叠无遗漏无空片；含 DSN `go test -p 1 -count=1 ./...` 退 0，**100 ok / 0 FAIL / 16 无测试**（544s）；探针 `pilotgovernance/adapters/postgres` 含 DSN PASS 47 / SKIP 0、无 DSN SKIP 38 / PASS 0。未跑 `-race`（CI 四片各带）。共享 main 由 `e4fa61d7` 快进到 `e374b1ea`——ff 前 status 零行：MCP-6 18:01 已把共享树 65 处 CRLF 幻影逐个 `git diff --quiet` 为 0 后 `git restore --worktree`（其广播记 mtime 全为 09-04 21:42:23 同一秒）。

**在途 / 待办（归用户）**：tf/09 三笔在 `mcp5-tf-cmdr` 分支上（tf-segment-lifecycle-closure/09 ①–④，未 resolved），tf/11 未开；pc-gaps/08 代码半边（迁移 0025 → 域 → PG → 用例；跨地盘签名点见票 08「签名纪律」）与 09 / 10 未开，MCP-6 建议派新会话；sa-preacceptance-policy-view/03 draft。台账 working 四单（`2f035050` / `895fbabf` / `aafca372` / `d3c3a7ac`）的承接会话都已换人，结不结、怎么派归用户；MCP-2 / 5 / 6 自报空闲可接票。树：`idp-replay-psr05-tf01`、`idp-replay-wave`（接管会话的）、`idp-parcel-mcp2-psr05`、`idp-parcel-mcp5-tf-cmdr`、`idp-parcel-mcp4-wbr07`[mcp6-wbr07]、`idp-parcel-mcp4-verify`、`idp-parcel-mcp1-sa02` 未拆，等用户点头；`idp-parcel-mcp6-awf06` 已由 MCP-6 拆（18:0x 后 `worktree list` 已无）；`idp-tf03` 归用户。CI 为 `e374b1ea` 在跑。

### 18:1x–18:5x（上一会话做了没记，按台账 `query_tasks` 与 git 补；本会话未经历）

- 18:11 推 `48483df7`（本文件上一节）；18:19 点名（截止 18:22）后四单：MCP-2 `269d98b6` wiring-baseline-remainder/06（PP，ADR-0124）、MCP-3 `0ec68b23` sa-preacceptance-policy-view/03（PS，ADR-0125）、MCP-5 `04ffe457` tf/09 余量（接 `895fbabf`，其第 1 步 tf-carrier-master-document-register/01 已进 main）、MCP-6 `48e13cf1` pc-gaps/08→09→10。18:3x 推 `299f2a2e`（= 48483df7 + MCP-6 四件票面封存出处，纯 .md；CI success）。
- 18:5x 通道 5 会话崩溃（存档止于 18:26；旧会话已在 `mcp5-tf-cmdr` 上把 tf/09 三件未提交现场封存为 `502a6856`）。用户把其 todo 截图连「继续」发到通道 4，通道 4 接手 → `04ffe457` 与母单 `895fbabf` 结 failed「承接方换人」，18:52 重派 `79675845`（通道 4）。

## 2026-09-07 19:0x 通道 1 新会话接续（接手时 `main = origin/main = 299f2a2e`）

用户只交代「监听队列、正常回复、保持循环」。接手时队列两条（均 18:5x 广播）：MCP-4 开工报——`mcp4-tf09` 基 `299f2a2e`，四笔 cherry-pick `d11d45c4→8cf3b8f3`、`a5e9ba31→85956673`、`8f079e66→54b7ab04`、`502a6856→ba04dd0a`，build/vet 0；MCP-5 接管报——通道 6 会话亦崩溃（其取证：通道 6 队列 18:39:19 起为新会话、18:48 自报无派单；旧树 `mcp6-pcgaps08` @ 299f2a2e 干净、mtime 18:26:06 = 检出时刻、无现场），用户把 todo 截图发到通道 5，通道 5 自建 `mcp5-pcgaps08` 基 `299f2a2e` 接手。`load_progress` 存档停在 13:13，不作依据。

- **台账**：`48e13cf1` 结 failed「承接方换人，非执行失败」，19:05 重派 **`142548f6`**（通道 5；context = 接管差量——基线 299f2a2e、沿用其自建树、旧树不动、「不碰」表里 TF 归 MCP-4——+ 原文一字不改）。通道 6 队列留作废令（勿认领、勿碰 partycommercial）；回 MCP-4 提醒 report_task `79675845`（台账仍 pending）。`269d98b6` / `0ec68b23` 19:0x 仍 pending 无 working 报，但 `mcp2-wbr06` / `mcp3-sa03` 两树已建于 48483df7——在不在做从这两样量不出，只作此刻取证。
- **拆树两棵**（均 `status --short --untracked-files=all` 零行、不带 `--force`、指针保留）。`idp-parcel-mcp5-tf-cmdr` @ `502a6856`：上一节写「等用户点头」，18:24 派单里同一会话改写为「MCP-1 比内容后拆」，MCP-4 开工报再点一次，按后者办。比法：`git cherry mcp4-tf09 mcp5-tf-cmdr` 十七笔三个 `+`，逐个有落点——`ac408e78`（ADR-0113）= main `2e394faa`，除 README 外文件 diff 空；`c0d92fcc` 是分支清点笔，已被 `e33f2076` 盖过；`a5e9ba31`（ADR-0114 + README 行）= `mcp4-tf09` `85956673`，README 自动合并。tf/09 四笔触及 19 件对 `mcp4-tf09` 的 diff 只 `docs/adr/README.md` 多他人四行（0116/0118/0122/0123），0114 行两侧一致。树内 `apps/admin-web/node_modules` 是指向主树的 junction：先 `cmd /c rmdir` 只拆链接（主树 node_modules 15 项前后不变、树内再无 reparse point）再 `git worktree remove`，退 0，`.git` 链接与登记双双已无，`mcp5-tf-cmdr` 仍解析到 `502a6856`。`idp-parcel-mcp6-pcgaps08` @ `299f2a2e` = main，无唯一内容，remove 退 0，`mcp6-pcgaps08` 指针留（= main）。

**在途**：MCP-4 `79675845` tf/09（`mcp4-tf09` @ ba04dd0a）；MCP-5 `142548f6` pc-gaps/08→09→10（`mcp5-pcgaps08`）；MCP-2 `269d98b6`、MCP-3 `0ec68b23` 台账 pending。**归用户不变**：needs-info 四张、ftr/03、ve-claims/04 三问、ADR-0109/0111 复核、tenant-implementation-01 三份。树：`idp-replay-psr05-tf01`、`idp-replay-wave`、`idp-parcel-mcp2-psr05`、`idp-parcel-mcp4-wbr07`[mcp6-wbr07]、`idp-parcel-mcp4-verify`、`idp-parcel-mcp1-sa02` 未拆，仍等用户点头；`idp-tf03` 归用户。MCP-1 只做推送方与集成，不自办票。

### 19:1x–19:4x：四路报 working；PC 批两次改派后定为分工；tf/09 分支重切

- 19:1x MCP-2（新会话）占号 wbr/06 并取号 ADR-0124——与 18:24 派单一致，确认可落（`origin/main` 只有 0118/0122/0123，四支在途分支无 0124/0125 文件，19:1x 量得）；MCP-3（新会话）对 `0ec68b23` 报 working，`mcp3-sa03` ff 到 `b24ccccf`；MCP-4 对 `79675845` 报 working。
- **PC 批三换承接方，最终分工**：19:3x 用户在通道 6 面板两次直接指示通道 6「继续全部完成」pc-gaps/08→09→10 → `142548f6`（通道 5）结 failed「用户改派」、派 `ce26f498`（通道 6 全接）、给 MCP-5 停手令；19:4x 用户在同一面板问「为什么要别人停手」，MCP-6 提案不停 MCP-5、改分工，MCP-1 同意 → `ce26f498` 未开工即结 failed「范围收窄」，**MCP-5 `0d116e60` 只做 08（停手令撤回；迁移 0025、authority_grant*/contract_delegation*、PS 两文件三步法），MCP-6 `390c4f53` 做 09→10（迁移 0026/0027、ADR-0119/0120，分支 `mcp6-pcgaps09` 基 `b24ccccf`）**。两人同包按文件分界；共享接线文件（`ports.go`、`publish_commercial_authority.go`、`declaration_publication.go`、`cmd/parcel-commercial/translate.go`、PC CONTEXT、ADR README）各加自己一段、改前广播；**重放顺序 08 先、09/10 后**，冲突由 MCP-6 rebase；隔离树里 0025 缺席是预期内的，门禁若对号不连续报错不造占位、报 MCP-1。台账里 `48e13cf1 → 142548f6 → ce26f498` 三张 failed 全是「承接方换人 / 范围收窄」，无一是执行失败。
- **MCP-4 重切 `mcp4-tf09`**（19:4x 报）：现历史 `8cf3b8f3 → 85956673 → 54b7ab04 → 9634b635`（三条派送要求端口，封存件形状一字不改 + 头注）`→ e4e63503`（触发执行器 + 测试，与封存件交叉验证后**重写**）`→ e5a79302`（票 12–14）；封存笔 `ba04dd0a` 不在其中，重切前 tip 留指针 `mcp4-tf09-precut@5be2686e`。上一节拆 `idp-parcel-mcp5-tf-cmdr` 时比的是重切前的 `mcp4-tf09@ba04dd0a`，那一刻成立；封存内容仍经 `mcp5-tf-cmdr@502a6856` 与 `mcp4-tf09-precut` 两指针可达，两指针不动。重放时取重切后的历史。
- MCP-2 19:3x 报剪 `production_wiring_baseline.txt` PP 段 `ReplayPricingEvaluation` 一行（成因第二种：真接上），该文件无他人在途，重放时按新父提交重量。

**在途（19:4x）**：MCP-2 `269d98b6` wbr/06（PP，`mcp2-wbr06`）；MCP-3 `0ec68b23` sa/03（PS，`mcp3-sa03`）；MCP-4 `79675845` tf/09（TF，`mcp4-tf09`，待完工报）；MCP-5 `0d116e60` pc-gaps/08（`mcp5-pcgaps08`）；MCP-6 `390c4f53` pc-gaps/09→10（`mcp6-pcgaps09`）。五路满。

### 19:5x–20:0x：MCP-6 误开作废单一次纠回；tf/09 进 main

- 19:5x MCP-6 读到旧队列里 19:3x 的 `ce26f498` 就开了工（建 `mcp6-pcgaps08`、cherry-pick MCP-5 的 `1e182860`，要往下做 08 余步），与 MCP-5 正写的第二笔正面撞——19:4x 的三条（同意分工、新单 `390c4f53`、地盘更正）都在它队列里只是还没读到。紧急纠正后它 20:0x 前已切到 `mcp6-pcgaps09`、拆掉误建的树；`ce26f498` 在台账仍 failed（它对终态单的 working 报没翻状态）。**教训一句**：换承接方几分钟内改口两次，接收方读到哪一条取决于它什么时候排队列——同一收件人的连续改令，后一条要点名作废前一条的单号，本次做了，仍晚了一轮。
- MCP-5 票 08 第一笔 `1e182860`（领域 + 读口 + 新构造器三步法 + 迁移 0025）、第二笔 `6b43484c`（合同委派册落库 + 点读口 + 替身，真库七组 PASS）完工报已收，第三笔发布通道 + 批文进行中。MCP-3 三问已裁、不碰 PS `adapters/partycommercial`（另立 draft 04）、`cmd/parcel-dispatch` 两件测试跟签名；MCP-2 两笔已成、README 0124 行 + endpoints.go PP 组行改前报已收。

| SHA | 内容 |
|---|---|
| `081cfc56`..`2346c96e` | MCP-4 `79675845` tf/09 七笔重放到 `250e5a43`（`8cf3b8f3→081cfc56`、`85956673→dd757ef7`、`54b7ab04→d6c118b2`、`9634b635→d9d112d5`、`e4e63503→9f6ac00e`、`e5a79302→6ebc2aec`、`a155e11d→2346c96e`；cherry-pick 零冲突，七对逐笔树比对除 tasks.md 外零差）。分支清点笔 `3b1ee383` 不重放 |
| `7bc47ec9` | 清点在 `2346c96e` 干净检出重生成，与 `3b1ee383` 逐字节同。**已推**（推前 ls-remote = 250e5a43） |

验证钉 `7bc47ec9`（隔离 detached 树 `idp-replay-tf09`，验后已拆）：gofmt 空；build/vet 退 0；清点工具 vet/test 退 0；清点门零差；含 DSN `go test -p 1 -count=1 ./...` 退 0，**100 ok / 0 FAIL / 16 无测试 / 0 cached**（595s）；探针 `transportfulfillment/adapters/postgres -run ServiceAction` 无 DSN SKIP / 有 DSN PASS；改动 `.go`/`.sql` 15 件全 `i/lf w/lf`。票 09 补「进 main 记录」（随本笔）；tf/11 未开、TF 地盘已由 MCP-4 释出。拆 `idp-parcel-mcp4-tf09`（`git cherry main` 八笔全 `-`、status 零行、不带 `--force`），指针 `mcp4-tf09@3b1ee383` / `mcp4-tf09-precut@5be2686e` / `mcp5-tf-cmdr@502a6856` 保留。ADR-0114 四条 + 执行器三条越权风险点归用户复核。

**在途（20:0x）**：MCP-2 `269d98b6`、MCP-3 `0ec68b23`、MCP-5 `0d116e60`、MCP-6 `390c4f53`；MCP-4 空闲。**可派**：tf/11（ADR-0121、迁移 0019；TF 地盘空）。CI 为 `7bc47ec9` 在跑。

## 2026-09-07 21:2x 通道 1 新会话接续（接手时 `main = origin/main = fc90622a`）

用户只交代「监听队列、正常回复、保持循环」。**时钟**：本节内各通道消息自报的时刻比本机快约 1.5–2 小时（它们写 02:0x 时本机 `Get-Date` 是 00:0x，git 提交时间也印证本机），本节小节标题与叙述已按本机时钟校正（00:1x 一次性改回），引用它们原话时保留其自报值；此后派单要求报时刻用 `Get-Date`。上一节末（20:0x）到接手之间按 git log 补一行，本会话未经历、验证强度以各提交信为准：`6ff95211` 之上 pc-gaps/08 四笔（`0cfe3571` / `31903cc5` / `02ff5205` / `44bcac4d`，分支 `mcp5-pcgaps08`，`git cherry main` 四笔 `-`）与 wbr/06 六笔（`ab191bba` / `1a3c37ea` / `a896b1d0` / `db023b4f` / `deca2a3d` / `de9635e6`，分支 `mcp2-wbr06`）已重放进 main，`d98d17be` 基线头注补推送方一段，清点 `fc90622a`（20:09 推）；两票在 main 上的 SHA 对照未见补记（票 06 完成记录写「main 上的 SHA 待重放后补记」），待补。接手时共享树 70 处 ` M` 全为 CRLF 幻影（`--ignore-cr-at-eol` 零行），无未跟踪、无 stash。

### 21:4x：MCP-5 拆树两棵，MCP-1 复核后记账

用户 21:4x 在通道 5 面板批「拆吧，但需要审查原来的代码」，MCP-5 21:5x 报 MCP-1 记账；MCP-1 在 `fc90622a` 上复跑同得：

- `idp-parcel-mcp4-verify`（detached @ `305402f5`）与 `idp-parcel-mcp4-wbr07`（`mcp6-wbr07` @ `b0fd8c52`）已 `git worktree remove`，不带 `--force`；两树拆前 `status --short --untracked-files=all` 均零行、无 ignored 文件。指针 `mcp4-wbr07@305402f5` / `mcp6-wbr07@b0fd8c52` 保留（复核：`worktree list` 两棵已无，两指针仍解析到原 SHA）。
- 拆前比内容：wbr/07 五笔（`1b1a2980` / `f1b21c47` / `223d6145` / `305402f5` / `b0fd8c52`）触及 10 件，`git diff b0fd8c52 main -- <10 件>` 只剩 3 件有差且全是 main 后续前进——`docs/adr/README.md` +6/−0；`production_wiring_baseline.txt` +41/−8（MCP-5 核：−8 是 wbr/06 `deca2a3d` 剪掉的评价重放门那段，`223d6145` 自己那句注释在 main 上）；`MECHANISM-INVENTORY.md` 为重生成差。ADR-0123、票 07 md、`decimal.go` / `evaluation_snapshot.go` / `charge_dependency_execution.go` 及两份测试与 main 逐字节相同。`git cherry main b0fd8c52`：`f1b21c47` / `223d6145` / `b0fd8c52` 为 `-`；`1b1a2980` 为 `+` 只因 README 上下文（ADR-0123 文件本身 diff 空）；`305402f5` 是分支清点笔，已被后续重生成盖过。
- 未动 main、未推。`idp-parcel-mcp5-pcgaps08`（`85a90a1b`）未拆：`git cherry main` 08 四笔 `-`、其后三笔 `+`（`a96f7356` / `74332508` / `85a90a1b`，MCP-5 报为 09/10 内容，与 MCP-6 `mcp6-pcgaps09` 重叠），处置待定。

### 21:5x 取证：main 顶端 `fc90622a` 的 CI 因分片超时被掐（run 34121403815）

`Test (rest)` 15m18s 撞 `timeout-minutes: 15` 被 cancel，其余三片与 Static 绿；这是今天第二次——`b24ccccf`（run 34115373617，19:11）的 `Test (network-visibility)` 同样 15m18s 被掐，而它与下一笔成功的 `250e5a43` 之间只差 tasks.md 一件。**成因是快慢差不是某个用例挂死**：同一片近乎同一代码，`rest` 在 `b24ccccf` 7m27s、`250e5a43` 13m33s、`6ff95211` 6m58s、`fc90622a` >15m；被掐那次逐包看，`cmd/parcel-api` 144s（`6ff95211` 上 49s）、`cmd/parcel-dispatch` 414s（104s），20:30:06 印到 `partycommercial/adapters/http` 后再无一包印出，被掐时还活着的是 `outbound.test` 与 `migrations.test`——按 3–4 倍放慢，`partycommercial/adapters/postgres`（健康时 200s）一包就吃掉剩余全部。12:5x 那节定的「最重片 ≤ 15 分的六成」线，按 `250e5a43` 的 13m33s（90%）已破。ci.yml `go test -race -count=1 $packages` 不带 `-p 1`，本机验证口径是 `-p 1`；其上注释量的是本机不带 `-race` 的 116–132 秒，CI 上带 `-race` 健康日已是 6–7 分。**处置归 CI 地盘（MCP-4）与用户**：抬 `timeout-minutes` 或再拆片，本会话不动 ci.yml；本笔纯 .md 推出后会再起一跑，同代码再量一次。远端最近一次绿是 `6ff95211`（run 34119509220）。

**在途（21:5x，按台账与树况）**：MCP-3 `0ec68b23` sa/03（`mcp3-sa03` @ `e7c44522` + `mcp3-verify`）；MCP-6 `390c4f53` pc-gaps/09→10（`mcp6-pcgaps09` @ `246135ae` + `mcp6-verify09`）；MCP-2 `269d98b6` wbr/06 已进 main、树 `mcp2-wbr06` 未拆；MCP-5 `0d116e60` 08 已进 main、树 `mcp5-pcgaps08` 待定（见上）；MCP-4 空闲。树：`idp-parcel-mcp1-sa02`、`idp-parcel-mcp2-psr05`、`idp-replay-psr05-tf01`、`idp-replay-wave`、`idp-replay-wave2`（内容都在 main）仍等用户点头；`idp-tf03` 归用户。**可派**：tf/11（TF 地盘空）；wbr/06 与 pc-gaps/08 两票「进 main 记录」待补。

### 21:5x–22:3x：sa/03 进 main；MCP-6 自推 pc-gaps/09 一次；CI 三次红同一成因，派 MCP-4

- MCP-3 `0ec68b23` done（21:5x）：`mcp3-sa03` 六笔基 `b24ccccf`，跳过分支清点笔 `af7dd29a`。第一次重放到 `e10a3b5e`（README 0125 行与 main 0124 行冲突，两行都留按号排）→ 隔离树含 DSN 全仓 **100 ok / 0 FAIL**（563s）、探针三用例 SKIP 3 ↔ PASS 3——快进前发现 main 已到 `f69b35b9`：**MCP-6（21:2x 新绑会话）按用户在通道 6 面板「需要收尾落 main」的指示，自己走完推送方流程把 pc-gaps/09 三笔（`e078a508→838b283e` / `7ea33e60→d06192ed` / `ded10f61→d58ceecc`）+ 清点 `ab0fe0f0` + 记录 `f69b35b9` 推了**（含 PC 代码：迁移 0026、`label_validity*`、`final_rule_validity*`、`translate.go` 三函数）。MCP-6 回声已认「之后不再自推，完工报交推送方」；main 上不回退。
- 因含代码，改在 `f69b35b9` 上重放（cherry-pick 零冲突，README 0125 自动落 0124 之后）：`63dce12a→0ca996d6`、`a0153884→096f1eae`、`d1a878aa→0848a3f0`、`e7c44522→2e582020`、`5ffdd3f3→d93dfddf`；清点在 `d93dfddf` 干净检出重生成 `9379c716`（parcelshipment 139→140 / 138→139，合计 840→841 / 788→789；迁移 parcel_shipment 18→19，合计 150→151）。验证钉 `9379c716`（隔离 detached 树 `idp-replay-sa03`，验后已拆）：gofmt 空；build/vet 退 0；清点工具 vet/test 退 0；清点门零差；含 DSN `go test -p 1 -count=1 ./...` 退 0，**100 ok / 0 FAIL / 16 无测试 / 0 cached**（552s）；探针 `parcelshipment/adapters/postgres -run 'ACombinedControl…|AStoredConclusion…|AcceptanceJudgmentShapesArePinned…'` 无 DSN SKIP 3 / 含 DSN PASS 3；改动 `.go`/`.sql` 全 `i/lf w/lf`。共享 main 由 `f69b35b9` 快进到 `9379c716`，**已推**（推前 ls-remote = f69b35b9）。分支 29 件对 main 只差 README 三行他人后落；`idp-parcel-mcp3-sa03`（status 零行、无 reparse point、`git cherry` 四 `-` + 63dce12a/af7dd29a 两 `+` 均有落点）不带 `--force` 拆除，指针 `mcp3-sa03@5ffdd3f3` 保留。sa/03 越权风险点五条在票面「裁决」节，归用户复核；票 03「进 main 记录」等 MCP-3 按 22:3x 广播补。
- **CI 三次红是同一成因**（补 21:5x 节的诊断）：③ `f69b35b9` run 34130024505 `Test (customs-transport)` **failure** 11m6s——`customscompliance/adapters/postgres` 与 `transportfulfillment/adapters/postgres` **各自** `panic: test timed out after 10m0s`（go test 单二进制默认上限），goroutine dump 两边都停在 pgx `tx.go:231` 等服务端回包，当前用例才跑 6s / 0s——每个用例都在等锁、整包爬行；本机 `-p 1` 同两包 69s / 62s。成因：ci.yml `go test -race -count=1 $packages` **不带 `-p 1`**，片内多包并发打同一只 service 容器同一个库，TRUNCATE / DDL / 事务互相锁等，快慢差 2–10 倍不可复现（`e10a3b5e` 同代码再跑 rest 6m50s 绿，run 34129503260 success 7m4s）。本机口径一直是 `-p 1`，CI 与本机不一致。**派 MCP-4 `82bc2586`**：片内加 `-p 1`、按 CI 实测改写注释与 timeout 依据、`check` 清单不动、完工报交 MCP-1 重放；未点名（MCP-4 20:0x 起空闲），单若久无 working 报按派工错位处理。`9379c716` 的 run 34132070913 在跑，结果不定——在 `-p 1` 落地前，**主线「绿」要以本机隔离树含 DSN 全仓为准，CI 红先看是不是这一格**。

**在途（22:3x）**：MCP-2 `269d98b6`（wbr/06 已进 main，台账未结）、MCP-5 `0d116e60`（08 已进 main，台账未结）、MCP-6 `390c4f53`（09 已进 main，10 未见完工报）、MCP-4 `82bc2586` CI；MCP-3 空闲。树：`mcp2-wbr06`、`mcp5-pcgaps08`、`mcp6-pcgaps09` + `mcp6-verify09` 各归其主；`idp-parcel-mcp1-sa02`、`mcp2-psr05`、`idp-replay-psr05-tf01`、`idp-replay-wave`、`idp-replay-wave2` 仍等用户点头；`idp-tf03` 归用户。**可派**：tf/11；sa-preacceptance-policy-view/04 draft（MCP-3 随 03 立）。

- 22:2x 补两条：① 票 03「进 main 记录」MCP-3 落在 `mcp3-sa03-record@a1229e39`（父 `9379c716`，纯 .md），cherry-pick 进 main 为 `31c1fb1c`，随本笔推。② MCP-3 取证（MCP-1 复核同得）：`mcp5-pcgaps08` 在 08 四笔之后还带 **第二份 pc-gaps/09**——`a96f7356`（ADR-0119，文件名 `…-anchored-at-channel-result-time.md`）+ `74332508`（迁移 `0026_final_rule_validity_declaration.sql`，同改 `translate.go` / `publish_commercial_authority.go` / `service_stage_content.go` / `declaration_publication.go`）+ `85a90a1b`（pc-gaps/10 开工前置取证 .md），20:18–20:20 提交，晚于 19:4x「08 归 MCP-5、09/10 归 MCP-6」的分工；与 main 上 MCP-6 的 0119 / 0026 **同号不同名同题**。**三笔是孤儿，不重放**：MCP-5 `0d116e60` 的范围（08）已全在 main，可结 done；`mcp5-pcgaps08` 树拆前别把这三笔当待集成，10 的取证 .md 归 MCP-6 自取。

### 22:3x：用户问「工作树都合回 main 了吗，会不会重复开发」→ 答后授权自决，做了两件

**答**：合，走推送方 cherry-pick 重放不走 merge，所以 `git cherry` / `--merged` 看不出；重复开发发生过（上条 mcp5 第二份 09）。**自决的两件**：

1. **拆等点头的六棵树**（各自 `status --short --untracked-files=all` 零行、无 reparse point、不带 `--force`）：`idp-parcel-mcp1-sa02`、`idp-parcel-mcp2-psr05`、`idp-parcel-mcp2-wbr06`（三支分支按下面量法全 ABSORBED / main 只多加行）、`idp-replay-psr05-tf01`、`idp-replay-wave`、`idp-replay-wave2`（三棵 detached 都在 main 祖先上）。`worktree list` 现只剩主树 + 三棵在途（`mcp4-ci-p1` / `mcp5-pcgaps08-record` / `mcp6-pcgaps10`）+ `idp-tf03`。
2. **91 条分支指针改名**（`git branch -m`，一个不删）：量法——分支自 merge-base 起改过的每份非簿记文件，其 tip blob 是否在 `main` 该文件历史里出现过（`git log main -- <file>` 逐笔比 blob）；全出现过 → `merged/`；只差 `unwired_orchestration.go` / 票面 .md 且 `git diff <分支> main` 只加不减 → 也 `merged/`；老分支按 `unmerged-branch-inventory/spec.md` 的终裁归类。结果 **`merged/` 79、`salvage/` 12**（`mcp5-tf-cmdr`、`mcp6-pcgaps09-precut`、`mcp6-pcgaps08`、`mcp5-pcgaps08`、`mcp5-reg-tabs-salvage`、`mcp2-awf02a`、`mcp4-awf02d`、`mcp3-pcgaps07`（07 已进、8dc99b9f 封存半边不进）、`mcp6-inventories-v2`、`t12-governance-register`（#6 进 #5 裁不合）、`bento-gate-reeval`、`syn-wall-door-audit`），未加前缀只剩 `main` + 三条在途 + `mcp4-tf03`。远端只有 `main` 与 MCP-4 取数用的 `mcp4-ci-p1`，未动。
3. 规矩两条写进 `docs/agents/parallel-sessions.md`：「派发前先点名」节末加**派单第 0 步查地盘分支有无半成品**（取数命令 + 派单必须写「接着做」还是「另起」）；「拆工作树」节加 **`merged/` / `salvage/` 前缀约定与量法**，集成方拆树时顺手改名。

### 22:4x：用户问「各分支自己 commit、push 是否更可靠」→ 评估后用户「同意」

**评估要点**（全文在 22:4x 回复）：单一推送方今日代价——通道 1 换会话 5 次、每次完工报躺 30–60 分钟；8 次集成各重跑 9.5 分钟全仓；cherry-pick 改写 SHA 让 `--merged` / `git cherry` 失效。买到的是串行与合体验证（09-05 VE inbox 那次是集成方全仓跑出来的）。**裁定**：① 分支每笔提交后推 origin，立刻生效；② 落 main 改为作者自落：rebase 到 origin/main → PR → CI 绿即「验的就是这个 SHA」→ merge commit（SHA 原样进 main）→ 一次一人（`set_channel_lock` / 落地令）；**切换条件：`82bc2586` 的 `-p 1` 落地后 main 连绿三次**，未到之前仍走推送方重放；③ 通道 1 缩成仲裁与簿记。

**做不到的一格**：仓私有 + 免费档，分支保护 / rulesets 接口 `403 Upgrade to GitHub Pro or make this repository public`（22:4x 实测，`gh api` 权限 admin=true 也不行）。替代：所有 worktree 共用 `.git`，切换日装 `.git/hooks/pre-push`（目标 main 且 `.git/landing.lock` 记的 SHA 不等于要推的就拒），源码放 `scripts/git-hooks/`。真正的机器强制归用户：升级套餐或转公开仓。

两条都已写进 `docs/agents/parallel-sessions.md`「推送」节下（「分支每笔提交后推到 origin」「落 main 的下一形态」），①已广播全通道。

### 22:5x–23:0x：用户连问「TEMP 里是不是有大量代码没进 idp-parcel / MCP-6 crash 会不会丢 / 原来是不是很多代码都没进」→ 全量审计（钉 `d6d4cd46`）

- **TEMP**：登记在册只剩 `mcp4-ci-p1`（一行 ci.yml）与 `mcp6-pcgaps10`（在途，5 笔、树干净）；8 个 `idp*` 残留目录无 `.git`、无代码。工作树与主树共用 `.git`，提交过的一律在 `d:\tops\idp-parcel\.git`（2112 个可达提交、117 MB）。`mcp6-pcgaps10@ac6b5241` 由 MCP-1 代推 origin 备份并知会。
- **分支上只存在于分支、main 上从未有过的代码文件**（95 条分支逐条查 `--diff-filter=A` 的代码文件对 `main:` 路径）：6 条——`mcp6-pcgaps10`（在途 5 件 1109 行）；`merged/cons-proj-tf-b` 1 件（清单 #4：断言拆成两用例进 main）；`merged/cr04-backend` 1 件（#12：main 有更严的同等守卫 `eol_guard_test.go`）；`merged/mcp2-ps-ports` 4 件（两只 `Unconnected*` 占位，曾进 main、后被 psr/05 换成真读法而删）；`salvage/mcp5-pcgaps08` 1 件（第二份 0026）；`salvage/t12-governance-register` 3 件（#5 裁「不合」那半）。无一是丢失。
- **游离提交**（`git fsck --unreachable --no-reflogs`）398 笔：按 patch-id 对全部 ref（1869 个 patch-id）+ 逐文件 blob 对 main 历史两级核——273 笔有等价补丁或同 blob、113 笔只动 .md/簿记、**12 笔"未对上"逐条看**：08-12 一笔是 `docs/agents.rar` + 客户 xlsx（有意不进）；08-18 三笔与 08-21 两笔只差共享 `ports.go`（换基座重放后的合并变体）；08-21 `a4f48bfd` 含 t12 #5 那半；08-31 `seed.sh` 那笔（#13 按原文补注释进 main `cd21e68c`）；09-04 三笔与 09-07 一笔全是 `unwired_orchestration.go` / `endpoints.go` 共享接线文件的封存快照或合并变体，完成版都在 main。**结论：截至 `d6d4cd46` 没有丢失的代码；不在 main 的只有 MCP-6 在途一票与有意排除的几处。**量法脚本在 `%TEMP%`（`classify-branches.ps1` / `code-only-on-branches.ps1` / `unreachable-audit.ps1`），若要长期用应进 `scripts/`，待用户定。

### 23:1x：pc-gaps/10 进 main

MCP-6 `390c4f53` done（23:0x）：`mcp6-pcgaps10` 基 `9379c716`，六笔，跳过清点笔 `ac6b5241`。main 自其基线起只多 .md（`git diff --name-only 9379c716 main` 无非 .md），与分支零重叠，重放到 `5fd4d942` 零冲突：`76fb081a→3ce4615f`、`91aa3c85→0932d6c3`、`709a87c1→053da518`、`44e80389→2b4ae6aa`、`c6cefab9→7a1ce11f`；清点在 `7a1ce11f` 重生成 `534493d4`（partycommercial 90→92 / 92→94 / 端口 30→31，合计 841→843 / 789→791；迁移 PC 26→27，合计 151→152；端口声明 361→362），**与 `ac6b5241` 逐字节同**。验证：重放 tip 的非 .md 树对 MCP-6 已含 DSN 全仓验过（100 ok / 0 FAIL，9m33s）的 `ac6b5241` 零差 → 按「代码同一全仓验仍有效」不重跑全仓；本侧钉 `534493d4`：gofmt 空、build/vet 0、清点门零差、探针 PC postgres `-run 'SourceDataAmendment|OpenParentRow'` 无 DSN SKIP 5 / 含 DSN PASS 5、含 DSN `-p 1` 跑 partycommercial/** + cmd/parcel-commercial + cmd/parcel-api + architecture + migrations 全 ok（83s）。共享 main 由 `5fd4d942` 快进到 `534493d4`，**已推**（推前 ls-remote = 5fd4d942）。MCP-6 树（status 零行、23 件对 main 零差）交它自拆并改名 `merged/`；远端 `mcp6-pcgaps10` 由它删。**PC 批（07→08→09→10）至此全部进 main**；pc-gaps 目录待 MCP-6 补票 10「进 main 记录」与 spec 对齐。

**解阻**（MCP-6 点名，只报不做 PS）：ps-port-remainder/02 余段——PS 消费适配器读 PC 声明（`AdoptedStageOwner.AcceptanceRulePackageFor` → `pcports.SourceDataAmendmentAllowanceView.LoadSourceDataAmendmentAllowance`，found=false → NotDeclared；`content.AllowanceFor(资料组, 阶段, 意图)` 三值一对一译到 `ports.SourceDataAmendmentAllowance`；阶段/意图用 PC 的 `DeclaredAmendmentStage` / `DeclaredAmendmentIntent`，`String()` 与 PS 原词逐字相等，认领人在 PS 适配器旁加跨侧比对测试钉住，ADR-0120 决定五）；admin-write-faces/12 可加 `sourceDataAmendment` 一节。**可派**：ps-port-remainder/02 余段（PS 地盘空）、tf/11、sa/04 draft。

**在途（23:1x）**：MCP-4 `82bc2586` CI（tmpfs 那一笔 run 34135451794 在跑）；其余四通道空闲。

### 23:2x：CI 修复进 main；21:5x 节的成因判断改口

MCP-4 `82bc2586` done：`mcp4-ci-p1` 四笔只动 `ci.yml`——`9814569f→f5bb9a85`（Test 行加 `-p 1`）、`d40e10e6→f06336dd`（postgres service `--tmpfs /var/lib/postgresql/data:size=2g`，与本机 compose.yaml 同口径）、`57b00a6d→874849ce`（timeout 依据改成 CI 实测：最重 job 406s = 15 分的 45%，上限与分片不动）、`44fc6fc6→e682291f`（注释改口）。重放到 `b2cb71fc` 零冲突；ci.yml 与分支 tip 零差、`i/lf w/lf` 无 BOM；WSL PyYAML safe_load 回显三项；`test-shards.sh check` 116 包过；无 Go 改动不跑测试。快进并推，**远端 main = `e682291f`**（推前 ls-remote = b2cb71fc）。MCP-4 自拆树、改名 `merged/`。

**成因改口**（21:5x 节写「片内并发锁等」只对了一半）：MCP-4 取数——`-p 1` 磁盘库那次 network-visibility 串行独占库仍 865s 被掐，真库包比健康日慢 2 倍多、纯内存包分毫不差 → **主因是 runner 落盘慢**：pgtest 每用例建库/删库，DROP 强制 checkpoint，耗时 = 用例数 × fsync 延迟；本机 compose 是 tmpfs，CI 用磁盘库对着本机数推 timeout，口径错在这里。`-p 1` 消掉的是叠在上面的第二笔账（真库包并发时慢三成、且不可复现）。tmpfs 三个 run 四片 Test 步 268–372s，逐包稳定（customs 两包 126/113s 三次同），cached 0 / FAIL 0 / DATA RACE 0。**切换「作者自落 main」的计数从 `e682291f` 这一跑起：main 连绿三次。**

顺带记：`b2cb71fc`（pc-gaps/10 代码 + 两笔 .md，ci.yml 还是磁盘库那版）的 run 34137392455 又红在 customs-transport——`customscompliance/adapters/postgres` 撞 go test 10 分默认上限，与 `f69b35b9` 那次同形，代码与本机验过的 `534493d4` 同一；这是 tmpfs 修复之前最后一次红的样本。MCP-4 补第五笔（注释：数以 tmpfs 为前提）`eac8c948` 与取数分支 `mcp4-ci-tmpfs-nop1`（PR #2，不进 main）在途。

### 23:3x–23:4x：用户「你看看如何调整」→ 弱项逐条落地（用户已授权自决）

按 23:5x 审查的六条弱项，能今晚落的都落了，全部可逆：

1. **合入前独立评审**：写进 `parallel-sessions.md`「合入前独立评审（2026-09-08 起）」——非作者、空闲通道评；隔离检出跑 `/code-review <基线>` 两轴（Standards / Spec）；写进票面 Comments「阻断 / 非阻断 / 无发现」三格，每条锚文件 + 符号，20 分钟时限；推送方只在「无阻断」时重放；纯 .md / 清点 / 封存笔 / 一行 ci.yml 自审。**从下一张代码票起执行。**
2. **度量**：同文件「度量：tasks.md 每节末尾五个数」——CI main 绿/总、集成时延中位数、重放笔数、重复开发次数、会话重置次数，量法逐条写明；本节末尾第一次落数（见下）。
3. **状态从 git 算**：新增 `scripts/branch-state.ps1`（状态页：main 与 origin、各树未提交、在途分支及推没推 origin、`-Path` 查地盘半成品、`-Classify` 判 ABSORBED、`-Audit` 游离提交两级核）与 `scripts/owner-review-queue.ps1`（把 ADR / 票面里的越权风险点与 needs-info / blocked 票汇成一页）。两份都只读；`.ps1` 带 BOM（PS 5.1 读中文要它，先例 `.scratch/admin-remainder-mechanism-batch/t2-census.ps1`）。AGENTS.md「Parallel sessions」加一句：多会话开工第一眼看它算出的状态页，不读自报；`parallel-sessions.md` 派单第 0 步与拆树节改为调它。
4. **owner 复核队列**：`scripts/owner-review-queue.ps1 -Out .scratch/owner-review-queue.md` 首次生成（钉 `ade810bd`）：11 篇 ADR 含越权风险点（0027 / 0029 / 0055 / 0113 / 0114 / 0116 / 0119 / 0120 / 0123 / 0124 / 0125）、8 张 needs-info / blocked 票、13 张票面提到越权点。owner 复核完在原文旁写「owner 复核 YYYY-MM-DD 认可」，下次生成会标出。
5. **机器把门的替代**：`scripts/git-hooks/pre-push` 写好（推 main 前核 `.git/landing.lock` 里的 SHA），**未安装**，切换日装。
6. 单机单库吞吐与文档拆层：未动——前者随 PR/CI 落地自然缓解；后者（`parallel-sessions.md` 500+ 行拆「现行规矩 / 历史复盘」两层）需要一次整篇重排，留给白天有余量时做，先记在这里。

### 23:4x–23:5x：用户「你来安排派发」→ 点名后四单派满，MCP-6 余量将满不派

- MCP-4 补报：第五笔 `eac8c948`（纯注释：数以 tmpfs 为前提）cherry-pick 进 main 为 `d1e6c094`，**已推**（推前 ls-remote = 6c465c53）；「tmpfs 不带 -p 1」取数 run 34138585558：四片 176/232/215/288s，比带 -p 1 快 20–70%，但逐包仍互相拖（customs 两包 200/181 对 126/113）——**-p 1 留**（最重片 45–46% 在六成线内，逐包耗时不再取决于邻包）。`82bc2586` 收口。
- 23:4x 点名（广播里写的「截止 01:38」沿用了各通道自报的时钟，本机实际 23:48）：MCP-2/3/5 空闲·充足，MCP-6 空闲·**将满**，MCP-4 补报里自报转空闲。第 0 步 `scripts/branch-state.ps1`：PS / PC / TF 地盘均无在途分支、无未提交现场。基线一律 `d1e6c094`。号：ADR-0121（tf/11 备）、0126/0127（awf/08）；迁移 TF 0019、PC 0028（awf/08）/ 0029（wbr 若需）、PS 0019。

| 通道 | 单号 | 内容 | 独占地盘 |
|---|---|---|---|
| MCP-2 | `5307fbdc` | ps-port-remainder/02 余段（PS 消费适配器读 PC 资料修订允许声明 + 跨侧词比对测试 + 接真装配）→ wbr/01 → wbr/02 | PS、parcel-api PS 组、基线 PS 段 |
| MCP-3 | `5f716c71` | wiring-baseline-remainder/03（PC→SA 信用依据缝）→ 04 → 05：接线或剪 | PC ports/postgres 三票点名的符号、SA adapters/partycommercial（仅 03）、基线 PC 段 |
| MCP-5 | `cc7313e8` | admin-write-faces/08：三问建模 → ADR-0126（备 0127）→ 服务端按册规范化 + 摘要（首例信用政策）→ 待批准载体 + 批准动作（PC 0028）→ 预览口进端点表；解阻 09–17 | PC domain/application/http/translate、parcel-api PC 组（endpoints.go 本波只有它动）、PC CONTEXT |
| MCP-4 | `c2003926` | tf/11 撤回控制的更正让参与关系失效（ADR-0112 决定四；需则 ADR-0121、TF 0019） | TF、parcel-api TF 组 |
| MCP-6 | — | 不派：余量将满。`save_progress` 后待命做本波第一个独立评审；换会话后接 tf/12–14 三条输入缝建模 | — |

**形状差别**：每张派单都带「合入前独立评审」——完工报后由 MCP-1 点一个非作者通道 20 分钟内评（阻断/非阻断/无发现），无阻断才重放；作者可能被点名评别人的票，优先于自己下一票。MCP-3 与 MCP-5 同在 PC，按文件分界并对 `ports.go` 占号；`endpoints.go` 本波只给 MCP-5。评审配对预案：MCP-2↔MCP-3、MCP-4↔MCP-5，MCP-6 余量允许时接第一个小的。

- 23:5x **投递错位一次**：`5f716c71`（toChannel=3）同时落进通道 6 队列，MCP-6 报了 working、建了空分支，但**动代码前先问归属**（09 那次之后要的形）；裁定归 3，MCP-6 零代码退出、树与分支删。00:0x 用户报 **MCP-3 crash、手动改指 MCP-6**：`5f716c71` 结 failed「承接方换人」，重派 **`4c5a1215`**（通道 6，原文一字不改，分支名 `mcp6-wbr03-05`，余量将满则逐票交、每票 save_progress）。MCP-1 首跑取数：`ade810bd` run 34138109343 四片 296/304/274/362s 绿——「作者自落 main」连绿三次的第 1 次；`b2cb71fc`（旧 ci.yml）run 34137392455 是今日第四红（customs 608s），与新 ci.yml 首绿成一组前后对照。

### 00:0x–00:1x：MCP-4 crash，tf/11 现场封存，接管单预派通道 4

用户 00:0x 报 MCP-4 crash（截图：todo 停在「逐条定要定的」已完、正在改领域 red 测试）。量现场：`mcp4-tf11@503dfcc6`（票面裁「要定的」四条——失效版本是链上带 voided 标的一版、失效后当前有效控制为无、继承离场三件、在场与关段都不数它；**不立 ADR-0121，号释回**）已推 origin；树内未提交只有 `actual_fulfillment_segment_test.go` +7/−18（mtime 00:02:49）。推送方代封存为 `chore(salvage)` **`6e5c8a0c`** 并推 origin（非集成候选，接手人重切）。`c2003926` 结 failed「承接方换人」；接管单 **`a8e6a834`** 预派通道 4（从 `mcp4-tf11` 接着做、第 1 步先读封存 diff 报 ≤10 行、封存笔不进 main），用户重启通道 4 即接；若改指别的通道，我改派。本波在途：MCP-2 psr/02 余段、MCP-5 awf/08（ADR-0126 已落，0127 释回）、MCP-6 wbr/03–05；通道 3/4 空（crash）。评审配对改为 MCP-2↔MCP-6、MCP-5↔（4 回来后）。

**本节五数**（21:2x 接手 → 23:4x）：CI main 绿/总 7/9（2 红均为 tmpfs 修复前 customs-transport 真库包撞 go test 10 分上限：`f69b35b9`、`b2cb71fc`；5 次 concurrency 顶替的 cancelled 不计；`ade810bd` 在跑）· 集成时延中位数 65 分（3 票：sa/03 37 / pc-gaps/10 65 / CI 75——后两票的等待大半是推送方在答用户问）· 重放笔数 14（另 cherry-pick 3 笔纯 .md 票面记录）· 重复开发 1 次（pc-gaps/09）· 会话重置 3 次（通道 1 21:2x、MCP-6 21:2x、MCP-3 21:5x）。

## 2026-09-08 10:2x 通道 1 新会话接续（接手时 `main = origin/main = 96558acd`）

用户只交代「监听队列、正常回复、保持循环」，随后问「工作树干净吗 / temp 里有没有 replay / 需要 Replay 吗」。上一节末（00:1x）到接手之间 main 无提交；队列里无未消费的完工报。

### 10:3x 只读盘点（钉 `96558acd`）

- 共享树干净（status 零行，无 CRLF 幻影，无 stash）。`worktree list` 七棵：主树 + `idp-parcel-mcp2-psr02`[mcp2-psr02-tail] / `idp-verify-psr02`（detached，同 SHA，MCP-2 自验树）/ `idp-parcel-mcp4-tf11` / `idp-parcel-mcp5-awf08` / `idp-parcel-mcp6-wbr03-05` / `idp-tf03`，全部干净。
- 四支在途分支对 main 的 `.go/.sql` 零重叠（main 自各自基线起只多 tasks.md 两笔）：
  - `mcp2-psr02-tail@e2a51ba8`：psr/02 余段两笔代码（`2e085cad` PS 消费适配器 `DeclaredSourceDataAmendmentAllowance`、`5e5e2c05` parcel-api 装配接真）+ 分支清点笔。MCP-2 自验含 DSN 全仓 **100 ok / 0 FAIL / 16 无测试 / 0 cached**（`%TEMP%\psr02-fulltest.log`，00:17）。票 02 面在分支上仍 in-progress、无完成记录；独立评审未做。
  - `mcp6-wbr03-05@c1dcbd90`：`db723eb1` 剪 `ValidateBeforeDecision`（PC domain 纯删 −217 + 基线）→ 05 resolved；`c1dcbd90` 03/04 补理由行并转 **blocked**，各卡一格等 MCP-1 裁（03 多候选选择器是解析语义改口、要 ADR，MCP-6 倾向甲；04 UC-PC-003 第四项改名或拆两问）。独立评审未做。
  - `mcp5-awf08@c003c845`：ADR-0126 + 首例信用政策规范化/摘要已落，待批准载体 + 批准动作（PC 0028）→ 预览口未做，在途。
  - `mcp4-tf11@6e5c8a0c`：只有封存笔 + 票面裁决，不进 main，等接管单 `a8e6a834`。
- **远端 CI 自 `e682291f` 起 main 连绿 6 次**（`ade810bd` run 34138109343 起，cancelled 不计）——22:4x 定的「作者自落 main：PR + CI」切换条件已满足，切不切归用户；未切之前仍走重放。
- `scripts/branch-state.ps1` 在 PS 5.1 下「在途分支」段被 `git cat-file -e` 的 NativeCommandError 打断，`-Classify` 输出为空（`2>$null` 拦不住 stderr 转异常）；本节候选判断为手算。修法归脚本地盘，未动。

### 10:4x：acceptance-review-read-face/01 收口快进入 main

MCP-2 10:3x 占号（只动一件票面 .md，`git log --all --not main` 对该目录为空）→ 核准（钉 `96558acd`：文件仍 handed-off、`df51ce0` 是 main 祖先、四支在途分支对该目录 diff 为 0）→ 10:42 完工报 `mcp2-arrf01-close@a4f31eeb`（父 = `96558acd`，1 件 +25/−2，无 `.go/.sql`，已推 origin）。纯 .md 簿记按 09-08 规矩自审、不点评审人、不走隔离树验；共享 main **快进**到 `a4f31eeb`（SHA 原样），本笔 tasks.md 之后一并推（推前 ls-remote = 96558acd）。MCP-2 树 `idp-parcel-mcp2-arrf01` 由它自拆、指针改 `merged/`。

**在途（10:4x）**：MCP-2 psr/02 余段（等它答「算不算完工」再点 MCP-6 评）、MCP-5 awf/08、MCP-6 wbr/03–05（05 待评审，03/04 等 MCP-1 裁两格）；通道 3/4 空。**归用户**：是否切「作者自落 main」；needs-info 四张、ftr/03、ve-claims/04 三问、ADR-0109/0111 复核、tenant-implementation-01 三份不变。

### 10:4x–11:2x：用户「继续解决 replay」→ 首两张按「合入前独立评审」走完的票进 main

- 10:4x 派评审（配对照 00:1x 预案）：MCP-6 评 psr/02（`e2a51ba8`，基线 `d1e6c094`）、MCP-2 评 wbr/05（`c1dcbd90`，基线 `4524cfd4`）；同时要 MCP-2 补票 02 完成记录。10:5x 两份都到、**均无阻断**：psr/02 ← 通道 6 · Standards 0 阻断 / 4 非阻断（最重：`unconfigured_source_data_amendment.go` 文件尾变更说明注释）· Spec 0 / 2（封闭零格仅钉一格）；wbr/05 ← 通道 2 · Standards 0 / 3（最重：staleness 测试新头注指错文件，闭包形态用例在 `application/validate_commercial_basis_test.go` 不在 `reference_closure_test.go`；「只有唯一解析能被重解」那格删后无人钉，可另立票）· Spec 0 / 0。非阻断随票记不挡合入。MCP-2 票 02 完成记录 `80f11fa2`（转 resolved）+ 评审原文落票 `46ada017`。
- 重放（隔离 detached 树 `idp-replay-psr02` 基 `04198347`，cherry-pick 零冲突成一条链）：psr/02 `2e085cad→0e46a9a0`、`5e5e2c05→a936509c`；清点在 `a936509c` 干净检出重生成 **`230366c4`**（parcelshipment 生产 140→141 / 测试 139→140，合计 843→844 / 791→792；消费缝 PS→PC 12→13，合计 53→54；与分支清点笔 `e2a51ba8` 逐字节同，分支清点笔不重放）；票面 `80f11fa2→7a6762f4`、`46ada017→115cd44e`；wbr/05 `db723eb1→1ebd7e4b`（剪 `ValidateBeforeDecision` + 八条专属用例，基线两法同得 7→6 / PC 5→4）、`c1dcbd90→5df16244`（03/04 理由行 + 转 blocked）。wbr 那两笔不改文件数，清点门在终 tip 零差。
- 验证钉 **`5df16244`**：gofmt 空；build/vet 退 0；清点工具 vet/test 退 0；清点门零差；非 .md 树对 `e2a51ba8` 只差 wbr/05 那六件（PC domain 四 .go + 基线两 .txt），对 `c1dcbd90` 只差 psr/02 那六件；含 DSN `go test -p 1 -count=1 ./...` 退 0，**100 ok / 0 FAIL / 16 无测试 / 0 cached**（559s）；探针 `cmd/parcel-api -run Amendment` 无 DSN SKIP 5 / 含 DSN PASS 5；改动 `.go` 全 `i/lf w/lf`（两份基线 .txt 为 text=auto，索引 LF）。共享 main 由 `04198347` **快进**到本笔（推前 ls-remote = 04198347）。
- 裁决两格（MCP-6 wbr/03、04 停下报的，owner 授权自决口径，已发通道 6，落票面由 MCP-6 另笔）：**03 裁甲**——闭包解析加 `resolveCreditPolicyBasis` 一步（镜像 `resolveSettlementPolicyBasis`），`ResolveCreditPolicy` 作四维选择器、`CreditBasis` 随 Resolution 交出；理由：价格规则与结算政策已在同一闭包里用类别专属选择器解析（ADR-0044），信用政策是第三个，乙会让同一闭包两套解析语义并存且逼租户按（法人、等级）拆范围；**取号 ADR-0127**（0126 在 `mcp5-awf08`，0127 全 ref 无文件）。**04 裁「谁有权」形态、不拆两问、不改名而并入 `Authorize`**——`domain.Authorize(grants, delegations, request)` 已是动作无关的三值裁定，`ManualReviewAction` 与主动拒绝同路；`ManualReviewRequirementFor(grants, scope, at) bool` 无法人无等级无三值，答不了「谁有权」也不该答「要不要」（单一权威在 `ManualReviewDirective`），改名后仍是 ADR-0042 要拦的并格；PS 侧复核授权端口与 `ActiveRejectionAuthorizer` 同形、适配器调同一裁定用例带 `ManualReviewAction`，谓词在 PS 端口接真那笔删、基线条目同笔消；不立新 ADR（ADR-0042 已定归类），改 UC-PC-003 第四项记裁决。PS 侧端口另派 MCP-2。
- 树：`idp-replay-psr02` 推后拆；`idp-replay-wbr05`（预验用，内容已在链上）拆；`idp-parcel-mcp2-psr02` / `idp-verify-psr02` / `idp-parcel-mcp6-wbr03-05` 各归其主自拆并改名 `merged/`（wbr 分支上 03/04 后续笔基新 tip 另起）。

**在途（11:2x）**：MCP-5 awf/08（待批准载体 + 批准动作 + 预览口）；MCP-6 落 03/04 裁决 → ADR-0127 + 03 判据 2–3（余量将满，可能只交裁决落票）；MCP-2 空闲——**可派**：wbr/04 PS 侧复核授权端口（与 `ActiveRejectionAuthorizer` 同形，PS 地盘）、wbr/01→02（原 `5307fbdc` 后两段）；tf/11 等通道 4。**归用户**：是否切「作者自落 main」（main 连绿 6 次）。

## 2026-09-08 11:1x 通道 1 新会话（接管时 `main = origin/main = a107ff60`）

- 前会话推完 `a107ff60` 后未发新 main SHA 对照——11:17 查各通道 `messages.json`，`a107ff60` 全零命中；本节首条广播补发。前节五数前会话未量，不代记。
- 接管时队列里 MCP-6 两条：wbr/03–05 落裁决与评审 `857acf78`（纯 .md 四件）+ `790f4b70`（staleness 测试头注一行）已于 11:0x 推到 origin `mcp6-wbr03-05`——在冻结通知之后到、不在重放链里；并交单（余量尽，`save_progress` 已存）。问 (a) 推送方顺手重放 / (b) 广播新 tip 后由它另起分支。

### 11:1x–11:2x：wbr 分支后两笔按 (a) 重放，远端 main = `f742a261`

- 选 (a)：两笔纯簿记，MCP-6 已交单，(b) 要等通道 6 下一会话再绕一圈。
- 隔离 detached 树 `idp-replay-wbr0304docs` 基 `a107ff60`，cherry-pick 零冲突：`857acf78→4dd653f8`、`790f4b70→f742a261`；两笔所动五件对分支 tip `git diff 790f4b70 f742a261 -- <五件>` 为空。
- 验证钉 `f742a261`：gofmt 空；`go build ./...` 退 0；`go vet ./internal/partycommercial/...` 退 0；`go test -count=1 ./internal/partycommercial/...` 四包 ok（无 DSN，postgres 包跳过——本笔只改注释）；清点在该树重生成、porcelain 空；改动 .go `i/lf w/lf`。按 09-08 规矩纯簿记自审、未重跑全仓，CI 由 push 兑底。
- 共享 main 由 `a107ff60` **快进**；推前 ls-remote = `a107ff60`，`push f742a261:main`，推后 ls-remote = `f742a261`。树推后拆（porcelain 空、`.git` 链接与 `worktree list` 登记均无）。
- 11:2x 广播 SHA 对照（含前会话那条链）并解冻：`mcp2-psr02-tail@46ada017` / `mcp6-wbr03-05@790f4b70` 内容已全在 main，各归其主自拆、指针改 `merged/`；新分支基 `f742a261`。

**在途（11:2x）**：MCP-5 awf/08；wbr/03 下一段（ADR-0127 + 判据 2–3，跨 PC/SA）待派，接手者先 `load_progress` 通道 6 存档；wbr/04 PS 侧复核授权端口待派（PS 地盘）；tf/11 等通道 4。**归用户**：是否切「作者自落 main」。

## 2026-09-08 12:3x 通道 1 新会话（接手时 `main = origin/main = 5aedbb0f`）

用户先问「ready-for-agent 9 张全在 admin-write-faces 属实吗」——核 `Status:` 首值：10 张，多出 wiring-baseline-remainder/04（11:0x 转 ready，唯一不受 08 阻塞）；admin-write-faces 09–17 九张全 Blocked by 08（draft）、无一在前沿。用户随即裁「先解决 08，全部你自己干，不要派工」。

### 12:4x–13:5x：awf/08 由通道 1 自办，接 MCP-5 半成品

- 第 0 步查地盘：`mcp5-awf08@c003c845`（基 `d1e6c094`，两笔：ADR-0126 + 首例信用政策规范化/摘要 + NOT_ACCEPTED 对账门）00:12 后无提交、树干净；`task-cc7313e8` 结 failed「承接方换人（用户指示 MCP-1 自办）」并广播占号（PC 地盘 + parcel-api PC 组 + PC 0028）。`d1e6c094..5aedbb0f` 与那两笔无 `.go/.sql` 重叠，cherry-pick 到 `mcp1-awf08`（基 `5aedbb0f`）零冲突：`db9a05ac→6152b9f5`、`c003c845→5baf3348`；MCP-5 指针留作封存出处。
- 余步按 `/implement` › `/tdd` 逐片红绿，每片一笔即推 origin：`e8045f6b` 领域（`PublicationDraft` 三格只向前、`ApprovalDutyRule` 批准门、`OperatorSubject`、快照文档往返、重建门）→ `81b9a622` 端口新文件 + 四用例（发布用例收具体 `*PublishCommercialAuthorityHandler`）→ `c55127c0` PC 迁移 0028 + 两只适配器 + 真库六组 → `9e23eb3f` 传输面（一份载荷两口同段解码、逐格问题、预览口、载体三口）→ `ed61d8e3` parcel-api 四行 + 第五族装配 + 真库装配用例；门禁跟进：PBC-08 补三个写口负向证据、`CurrentPublicationCanonicalizationVersion`（MCP-5 导出、全仓无生产调用点）删 → `d2b131e1` 清点重生成 → `5b04c874` 票 08 resolved + 伞票 07 表。
- 实施中量到两处、记进票 08 完成记录：区间未开的载体发布答 `DRAFT_AWAITS_EFFECTIVE_START`（发布用例对挂在已计划生效版本上的声明整项拒，载体永远带正文）；「同一次录入」= 摘要 + 壳（换范围再录是修订，否则新范围静默丢）。
- 验证钉 `ed61d8e3`（隔离 detached 树）：gofmt 空、build/vet 0、architecture 全绿；含 DSN `go test -p 1 -count=1 ./...` **100 ok / 0 FAIL / 16 无测试 / 0 cached**（565s）；探针 `TestTheWiredPublicationDraftPath` 无 DSN SKIP / 含 DSN PASS。
- 合入前独立评审：用户指示不派工，按 09-08 规矩「没有空闲通道时推送方自己用 /code-review 的隔离子代理跑」——两个隔离子代理各跑一轴（Standards / Spec），检出 `idp-review-awf08@5b04c874`、基线 `5aedbb0f`，13:47 起跑。

### 14:0x：用户「replay」→ 评审未回先落 main，远端 main = `5b04c874`

- 用户 13:5x 问「replay 了吗」、14:0x 裁「replay」——评审两轴尚未交报告，按用户指示先落。**这一次没有重放**：main 本地 = 远端 = `5aedbb0f` 正是分支基线，共享树只有本文件未提交，`git merge --ff-only 5b04c874` 快进，SHA 与分支逐笔相同。推前 ls-remote = `5aedbb0f`，`push 5b04c874:main`，推后 ls-remote = `5b04c874`。
- 验证凭据：分支 tip 前两笔（清点、票面）是纯 .md，代码 tip `ed61d8e3` 的隔离检出含 DSN 全仓 100 ok / 0 FAIL（见上节）；`5b04c874` 对 `ed61d8e3` 只差 `MECHANISM-INVENTORY.md` 与两张票面。
- 评审结果到后追记进票 08 Comments：非阻断随票记；若有阻断，在 main 上另笔修（不改写已推的 SHA）。
- 广播 SHA 对照与地盘释出；`mcp5-awf08` 内容已全在 main（两笔 cherry-pick 内容逐字节同），指针改名 `merged/mcp5-awf08`；树 `idp-parcel-mcp5-awf08` 归通道 5 自拆。`idp-parcel-mcp1-awf08` 与 `idp-review-awf08` 待评审收口后拆。

**在途（14:0x）**：wbr/03 下一段（ADR-0127 + 判据 2–3，跨 PC/SA）待派；wbr/04 PS 侧复核授权端口待派；tf/11 等通道 4；admin-write-faces 09–17 九张进入前沿、可派（同在 PC + admin-web，按册分文件；`endpoints.go` 若加读面行要占号）。**归用户**：是否切「作者自落 main」（连绿次数见 CI）；ADR-0126 越权风险点四条复核。

## 2026-09-08 14:0x 通道 1 新会话（接手时 `main = origin/main = 0ef63897`）

用户先要一次自带 `ask_question` 的调用测试，随后「监听队列、正常回复、保持循环」。接手时队列一条：前会话 14:0x 的 main 前进广播回声（`5b04c874`，awf/08 落地）。前会话 13:47 起跑的 awf/08 两轴隔离评审随会话结束**未交报告**——票 08 Comments 里没有评审条目，`5b04c874` 已在 main；补评归待办（非作者通道空出后派，或推送方再起隔离子代理）。

### 14:0x 用户「08 解决了，09–17 九张能不能并行开干」→ 答：能并行，但不是九张独立开

钉 `0ef63897` 核票面与代码，四处共享面：① 前端公共半边不存在——`apps/admin-web/src` 里四个新端点零命中，九张票完成判据都是同一条五步流程，08「未做」把它归给子票但没指定谁；② Go 两文件九张共改相邻行——`domain/publication_canonicalization.go`（`PublicationContent` 一格、`CanonicalizePublicationContent` kind 不符一句 + switch 一支、`IsRegisterCanonicalized`、`canonicalPublicationDocument` 一格、`RehydratePublicationContent` 一支）与 `adapters/http/publication_draft_payload.go`（`CommercialPublicationPayload` 一格、`Publication()` 一段）；③ 页面归属——09/10/11 各独占一页，12–17 六张全在 `CommercialPoliciesPage.tsx`，`party/api.ts` 九张共写；④ 词表读口不存在且是新裁决——12/13/15/17 写「下拉由服务端词表读口供」，`endpoints.go` 无词表端点，「一册一口还是一个通用口」按伞票口径该先裁。容量：六通道里 1 是推送方、2/4/6 各有在途分支。**建议三波**：第 0 波 16（信用政策，08 首例，Go 已接）连带前端公共半边先落 + 裁词表读口；第 1 波 09/10/11 三张并行（页面独占，不依赖词表读口）；第 2 波 12/13/14/15/17 按空闲通道 2–3 张一批（共写政策页，每册表单独立成文件、页面只加一行挂载）。用户 14:1x「按你的建议开工」。

### 14:1x 点名 → 14:1x 四单派满

- 第 0 步 `branch-state.ps1 -Path internal/partycommercial` / `-Path apps/admin-web/src/pages/party`：两块地盘上无 09–17 半成品、无未提交现场；PC 地盘上唯一「main 尚无」的两笔是 `mcp6-wbr03-05` 的 `db723eb1` / `790f4b70`，`git cherry main` 四笔全 `-`（已按 (a) 重放，见 11:1x 节），前会话写「归其主改名」未做，本会话 `git branch -m` → `merged/mcp6-wbr03-05`。
- 14:13 广播 `[点名 ← 通道 1 · 截止 14:18]`；14:15 前 MCP-3/4/5/6 应答空闲·无地盘·充足；MCP-2 截至 14:16 未应答（`mcp2-psr02-tail` 内容已在 main，树干净）。四张票四个应答，不等截止即派。基线一律 `0ef63897`；不加迁移（PC 0029 未占）、不动 `ports.go` / `endpoints.go`。

| 通道 | 单号 | 票 | 独占地盘 | 分支 |
|---|---|---|---|---|
| MCP-5 | `2e9b1361` | awf/16 信用政策表单 + **前端公共半边**（约定落点 `party/publication-draft-api.ts` / `publication-draft-flow.ts` / `PublicationDraftFlow.tsx`，骨架一出即广播） | `apps/admin-web/src/pages/party/` 新文件三件 + `CommercialPoliciesPage.tsx`；预期不动 Go | `mcp5-awf16` |
| MCP-3 | `7fc5a960` | awf/09 服务产品版本表单（本册无正文，先在票面补「本册规范化判断」再写代码） | Go 两共享文件各一格 + 新文件 `*_service_product.go`；`ServiceProductsPage.tsx` + 新表单文件 | `mcp3-awf09` |
| MCP-4 | `39a6f0cd` | awf/10 客户合同表单（两层分两节） | 同上模式 `*_customer_contract.go`；`PartyContractsPage.tsx` + 新表单文件 | `mcp4-awf10` |
| MCP-6 | `662822ef` | awf/11 供应商协议表单 | 同上模式 `*_supplier_agreement.go`；`SupplierAgreementsPage.tsx` + 新表单文件 | `mcp6-awf11` |

**形状**：09/10/11 先 Go 再表单本体（组载荷的纯函数 + 测试），流程组件等 MCP-5 的落点广播后接；三张共改 Go 两文件相邻行，改前占号、只加自己一格、本册正文 / 文档 / 载荷节全放新文件，重放按完工先后、纯加行冲突推送方解；九张都不碰 `party/api.ts`。每张带合入前独立评审（完工报后点非作者通道 20 分钟）。

**待派**：第 2 波 12/13/14/15/17（等 16 的公共半边进 main + 词表读口裁决——形状候选：一个通用读口按 kind 答词表 vs 一册一口，归 owner 或授权自决，未裁）；18 留 draft 等读面票；wbr/03 下一段、wbr/04、tf/11 不变。**待办**：awf/08 补评。**归用户**：是否切「作者自落 main」；ADR-0126 越权风险点四条复核。

### 14:2x–14:5x：awf/08 补评（MCP-2）→ 一条竞态窗口 → 作者另笔修 → 进 main；三路占号；公共半边落点

- 14:2x MCP-2 晚到应答（空闲、主动接 08 补评），派 `469aeab7`（非作者，钉 `5b04c874` 基 `5aedbb0f`，用前会话留下的 `idp-review-awf08` 树，20 分钟）。14:33 交：**两轴无阻断**；Standards 三条非阻断——(1) `PublicationDrafts.AdvanceDraft` UPDATE 只钉 `status + content_digest`，而 `SubmitDraft` 修订会重写壳五列与 `submitter_ref / submitted_at` 且摘要不变，批准者读到与写回之间录入者换范围重录，批准落在没看过的壳上、自批门对旧录入者判；(2) `已批准`后同内容同壳再录答 `DRAFT_REPLAYED`，ADR-0126 Decision 三字面可两读；(3) 0028 `content_document jsonb` 归一化后列上字节非摘要所盖字节，头注失配。Spec 零条。评审记录 `mcp2-awf08-review@d795307c`（基 `446389a6`，纯 .md）快进推，**远端 main = `d795307c`**。
- 作者（本会话）修 (1)(3)，隔离树 `idp-parcel-mcp1-awf08fix` 分支 `mcp1-awf08-fix` 基 `d795307c`：`c84230ce` WHERE 再钉 `submitter_ref / submitted_at / scope_ref / effective_starts_at / effective_ends_at`（`IS NOT DISTINCT FROM`）/ `declared_references`（`::jsonb`）六列，与 ON CONFLICT 修订谓词同一组列；真库用例 `TestApprovalLandsOnlyOnTheSubmissionTheApproverRead` 先在旧代码上跑（`git stash` 生产文件）**实测答 ADVANCED**、修后 SUPERSEDED 且行不动；(3) 不改已施加迁移（`migrate` 有校验和漂移门，`runner.go ErrChecksumDrift`），改为 `publication_draft.go` 文件头写明 jsonb 归一化与「两面一致靠重建门重算」。`d70a1f32` 票 08 完成记录补「补评后修补」，(2) 记为读法（同一次录入 = 摘要 + 壳）不改 ADR。验证（修补树，含 DSN）：gofmt 空；build / vet 0；`go test -p 1 -count=1` PC 四包（postgres 61.8s）+ parcel-api + parcel-commercial + architecture 全 ok；无新文件、端点 / 端口 / 迁移不变，清点不重生成。MCP-2 复核修笔 Standards 一轴**无阻断**（timestamptz µs 精度两侧同、已批准→已发布那步壳列不可能再变故不误拦）。共享 main 由 `d795307c` **快进**到 `d70a1f32`（推前 ls-remote = d795307c），**远端 main = `d70a1f32`**，已广播（四支在途分支不碰该文件，不必 rebase）。
- 拆树三棵（`status --untracked-files=all` 均零行、不带 `--force`）：`idp-parcel-mcp1-awf08fix`、`idp-parcel-mcp1-awf08`、`idp-review-awf08`；指针 `mcp1-awf08` / `mcp1-awf08-fix` 改名 `merged/`。`idp-parcel-mcp5-awf08`[merged/]、`idp-parcel-mcp6-wbr03-05`[merged/]、`idp-parcel-mcp2-psr02` + `idp-verify-psr02` 仍归其主未拆。
- 14:2x–14:3x 三路占号到（MCP-3 `mcp3-awf09`、MCP-4 `mcp4-awf10`、MCP-6 `mcp6-awf11`，均基 `0ef63897`）。派单没点到、4/6 量出来的两处共享点：应用层 `publish_commercial_authority.go publicationContentOf` 一 case 与 `publication_draft.go declarationsOfContent` 一支，各加自己那一支；以及 seed `publish-batch.json` 各册旧式 `sha256:syn-…` 串接进规范化后会 `NOT_ACCEPTED`（SERVICE_PRODUCT 2 项、CUSTOMER_CONTRACT 1 项、SUPPLIER_AGREEMENT 2 项），各改自己那几行——已转告 3/6。MCP-3 对 09「无正文」裁：`IsRegisterCanonicalized(SERVICE_PRODUCT)=true`，表单路摘要 = `PCC-1:<h{canonicalization,kind}>`；批文路无本册正文通道，两处走 default 即答案，对账门不开，seed 与 `publish_batch_test` / `syn_pc_seed_test` 不动；跨路重发答 `CONTENT_CONFLICT` 而非带两串的 `NOT_ACCEPTED` 记为后果，归伞票收口「旧式串何时拒收」一起裁。
- 14:3x MCP-5 广播公共半边落点 `mcp5-awf16@283aa7d3`（基 `0ef63897`，纯 admin-web）：`party/publication-draft-api.ts`（四口客户端 + `CommercialPublicationPayload` 类型——各册在此加一格正文类型 + outcome 标签表）、`publication-draft-flow.ts`（+test，纯逻辑）、`PublicationDraftFlow.tsx`（五步组件，render-prop 按 JSON 路径挂 problems，`localProblems` 只限编不进类型的格）。已广播接法：3/4/6 把 `283aa7d3` cherry-pick / rebase 进自己分支、完工报点名「借了」、**重放时跳过它（16 先落）**；MCP-5 不得改写该笔。

### 15:0x–15:5x：第 0 波 + 第 1 波四票逐个评审、重放、进 main；票 19 立

四张完工报 15:0x–15:1x 内齐到（16 MCP-5 `7d204fd5` / 11 MCP-6 `4fe52a35` / 09 MCP-3 `f4be0bc9` / 10 MCP-4 `7377bd23`），每张按 09-08 规矩点一个非作者空闲通道评（16←MCP-2、11←MCP-5、09←MCP-2、10←MCP-3），四份**两轴均无阻断**；评审记录各基当时 main 只提票面 .md，重放时排在作者完成记录之后（票面末尾同位冲突四次，都是「完成记录在前、Comments 在后」手工并）。重放顺序 16 → 11 → 09 → 10，前两张各自快进一次，后两张与 16 追笔、票 19 合成一条链一次快进：

| 远端 main | 内容 |
|---|---|
| `f1e37065` | **16**：`283aa7d3→dff3331a`、`5e2aa3d1→799e0ed9`、`7d204fd5→cf4bd709`（零冲突）；评审 `edea6c7f→f1e37065`。验证钉 `cf4bd709`：tsc 0、run-tests 91/91、architecture ok、清点零差。MCP-2 非阻断四条：`unconfiguredNote` 错指 PAR-INT-01、`DRAFT_SUBMITTED` 文案写死不同主体（作者追笔修）、`fieldPaths` 静态精确匹配装不下可加行子表（09 的 `serviceProductFieldPaths(draft)` 动态生成是现成解法）、助手同形不共用（判断题） |
| `83970dcf` | **11**：跳过 merge 笔 `f94c13a2`，`c6028a2e→934e8916`、`eadac23d→7b817cb9`、`33bda25f→4e51c75b`、`c4ca7004→78a002bf`、`d756dd1e→bf419f4d`、`a2a44679→2c7bee12`、`4fe52a35→61d994d6`（零冲突）；清点 `dccffb80`（PC 生产 100→102 / 测试 99→102）；评审 `a1401b84→83970dcf`。验证钉 `dccffb80` 含 DSN：PC 四包 + parcel-api + parcel-commercial + architecture 全 ok、tsc 0、98/98。MCP-5 非阻断：if 链宜改 switch、`normalizeMoment` 跨页导入、Status 早于进 main 转 resolved；Spec：目录读面不显 0021 正文列 → **票 19** |
| 本笔 | **09**：`69cc885e→5d455fbe`、`9fad4185→7cb76512`、`3f43525e→e0a56d36`、`063b0c22→c68f039f`、`07d7b5da→8ddfbcef`、`6d5dea48→3954ed00`、`f4be0bc9→f7652e7f`；`publication_canonicalization.go` 三处与 11 冲突手工并（版本注释两句并列、`Rehydrate` 供应商块早返回后接 `registerHasNoBody`、`IsRegisterCanonicalized` 并成 switch 每册一 case）、`publication_draft_payload.go` 一处注释并；清点 `af22c9a1`（PC 生产 102→103 / 测试 102→105）；评审 `e408d8d6→d371ba6e`。MCP-2 关键判断：无正文册选 (a) + 对账门不开 **不需改 ADR**——文档形状属 ADR-0126 能力边界下放子票；但「已接」被用成两义（`IsRegisterCanonicalized` = 表单路可算；门 = 它 ∧ `publicationContentOf` 在场），是 08 的门本就两段，伞票 07 收口时与「旧串何时拒收」一并裁。**16 追笔** `9dacdbad→abdc5ef9`（两处文案）。**票 19** `mcp6-awf19@8a9856f9→06d918ea`（供应商协议目录读面镜像 `supplierAgreementBody` 各键 + 正文列，ready-for-agent，非伞票子票）。**10**：跳过借来的 `1e7b9f41`，`fe8a522f→2aab84c4`、`ab771dec→e81dd49e`、`4e9d0970→54391c9c`、`3f4d4b53→19f01f33`、`7377bd23→0d03ae86`；域 / 应用两文件 / http 载荷 / `publication-draft-api.ts` 四处与 09/11 冲突手工并（各加一格一支、`IsRegisterCanonicalized` 四册四 case）；清点 `b25942a5`（PC 生产 103→105 / 测试 105→108）；评审 `76c09410→dc32c314`。MCP-3 取证 seed `SYN-CONTRACT-01` 的 PCC-1 串确由 `CanonicalizePublicationContent` 算出（隔离树临时探针）；非阻断：`Rehydrate` 「正文缺席」判散在两处宜按 kind 分派、`PreAcceptanceControlRequirementNamed` 是第二份枚举、合同级声明一节永远送让主路径表达不了「不作声明」（归伞票裁各册一致）。票 16 Status 转 resolved；伞票 07 子票表 09/10/11/16 四行转 resolved；09/10/11 票面末尾补「进 main 记录」 |

验证钉 `b25942a5`（隔离 detached 树 `idp-replay-wave1`；本笔只多 .md）：gofmt 空；build / vet 退 0；清点在 tip 重生成零差；含 DSN `go test -p 1 -count=1 ./...` 退 0，**100 ok / 0 FAIL / 16 无测试 / 0 cached**（554s）；admin-web tsc 0、run-tests **111/111**（node_modules 走 junction，验完 rmdir）。改动 `.go` / `.ts` / `.md` 全 `i/lf`。

**四票共同量到、归伞票 07 收口的**：(a) 无正文册「已接」与「开门」拆开 + 旧式 `sha256:` 串何时拒收（09 后果：跨路重发答 `CONTENT_CONFLICT` 而非带两串）；(b) `RehydratePublicationContent` 的「正文缺席」判散在各册早返回里，宜按 kind 分派；(c) 各册可缺声明节「留空 = 不作声明」还是「永远送」要一致（10）；(d) `normalizeMoment` 等表单助手跨页导入的所有权（11/16）。**第 2 波前置**仍是词表读口形状（归用户）。

树：`idp-parcel-mcp5-awf16` / `idp-parcel-mcp6-awf11` 已由作者拆并改名 merged/；`idp-parcel-mcp3-awf09`、`idp-parcel-mcp4-awf10`、`idp-parcel-mcp6-awf19`、`idp-parcel-mcp5-awf16-followup`、`idp-parcel-mcp2-awf08rv` 与四棵 review 树等本笔广播后各归其主拆；`idp-replay-wave1` 推后拆。分支 `mcp3-awf09` / `mcp4-awf10` / `mcp5-awf16-followup` / `mcp6-awf19` / 五条 `*-review` 待改名 merged/（内容全在 main）。

## 2026-09-08 16:2x 通道 1 新会话（接手时 `main = origin/main = dc1f0c07`；前会话与各通道 15:5x 后全部 crash）

用户问「工作树干净吗、都回放了吗」。`branch-state.ps1`：共享树 0 行未提交，15 棵树全干净。逐条在途分支核两样——`git cherry main <分支>`，加「分支每笔新增行是否都在 main 同文件里」（`cherry` 的 `+` 不等于内容不在，见 09-07 findings）：

- **全在 main**：`mcp3-awf09`（8 笔）、`mcp4-awf10`（6 笔）——`+` 的五笔只是 patch-id 变了，main 缺的行全是被 awf/10、awf/11 手工并或 `8ddfbcef` 重构改写的注释 / 结构体对齐行；`mcp2-awf09-review`（41/41 行）、`mcp3-awf10-review`（36/36 行）；`mcp5-awf16-followup`、`mcp6-awf19`（全 `-`）；`mcp2-psr02-tail`（5 笔全 `-`，只差 `MECHANISM-INVENTORY.md` 后来三次重生成）。
- **不在 main、按规矩不进**：`mcp4-tf11@6e5c8a0c` 封存笔；`mcp4-tf03@44808f31`（`D:/tops/idp-tf03`，未推）封存笔——加的行全在 main、删的行也已从 main 消失（只剩 `if err != nil {` 一类通行句），即被后续工作整体吸收。
- **不在 main、该进而漏了**：`mcp4-tf11@503dfcc6` 票 11 裁决（Status → in-progress + 六条裁决 + 0121 释回）。main 上票 11 仍 `draft` / `Blocked by: 10`，谁读 main 都会重裁一遍。

### 16:4x 用户「你帮我决策，怕丢了又重复开发」→ 三条各一句

| 分支 | 裁 | 动作 |
|---|---|---|
| `mcp4-tf11` | 裁决进 main，现场留分支 | `503dfcc6 → bd5ccb9e` cherry-pick 干净（票 11 自 `d1e6c094` 起 main 未动）；本笔改 Status 点明「实施现场在 `mcp4-tf11@6e5c8a0c`，接管从分支接着做、不从 main 重开」并补 Comment。分支、树 `idp-parcel-mcp4-tf11` 不动，等接管单 `a8e6a834` |
| `mcp4-tf03` | 已被吸收，留指针不留名 | 改名 `salvage/mcp4-tf03` 并推 origin（此前只在本机）；树 `D:/tops/idp-tf03` 归用户拆 |
| `mcp2-psr02-tail` | 内容全在 main | 改名 `merged/mcp2-psr02-tail`；树 `idp-parcel-mcp2-psr02`、`idp-verify-psr02` 待拆 |

票 19 由 MCP-6 16:3x 起做（地盘 `apps/admin-web/src/pages/party/`，基线 `dc1f0c07`，与 tf/11 不相交）。远端 main `dc1f0c07 → 2c0008c3`（`bd5ccb9e` + 本笔，纯 .md），已广播 2–6。

### 16:46 点名 → 16:48 四应答 → 三单派满、一单代结；merged 树统一拆

用户「crash / in-progress 的能不能 dispatch 继续收工」。在途清单：`Status: in-progress` 的票四张——awf/07（伞票，第 2 波 12–17 仍卡「词表读口形状」归用户）、tf/09（父票，子票 12/13 另计）、tf/11（crash 现场）、wbr/03（11:0x 裁甲后判据 2–3 待做）；wbr/04 `ready-for-agent`（11:0x 裁「并入 Authorize」，PS 端口与 PC 半边同笔）。`query_tasks pending` 两单：`a8e6a834`（tf/11 接管，00:0x 预派通道 4，从未开工）、`4c5a1215`（wbr/03→05，MCP-6，已实际做完 05 与 03/04 理由行）。

- 16:46 广播 `[点名 ← 通道 1 · 截止 16:50]`，同条问「正在用哪棵 merged 树」；16:48 前 MCP-3 / 5 / 2 / 4 均答空闲·无地盘·充足，且都答「列出的树未在用、可拆」；MCP-6 截至 16:50 未应答（在做 awf/19，正常形态）。
- `git diff --stat d1e6c094 main -- internal/transportfulfillment migrations/transport_fulfillment` 为空：tf/11 分支不必 rebase。

| 通道 | 单号 | 票 | 分支 | 独占地盘 |
|---|---|---|---|---|
| MCP-4 | `a8e6a834`（原单，催认领） | tf/11 接管，从 `mcp4-tf11@6e5c8a0c` 接着做，第 1 步读封存 diff 报 ≤10 行 | `mcp4-tf11`（树在） | `internal/transportfulfillment/**`、TF 迁移 0019 |
| MCP-3 | `5031a8a1` | wbr/03 下一段：ADR-0127 + 判据 2（闭包解析 `resolveCreditPolicyBasis`、`CreditBasis` 随 Resolution、SA 消费适配器）+ 判据 3（基线剪行第二种） | `mcp3-wbr03` 基 `2c0008c3` | PC domain `commercial_resolution.go` / `credit_policy.go`、`settlementaccounting/adapters/partycommercial/**` + SA 装配、`docs/adr/0127` |
| MCP-2 | `449f4e64` | wbr/04 判据 2–3：PS 复核授权端口（三值）+ 适配器调 `AdjudicateCommercialAuthorizationHandler` 带 `ManualReviewAction`，同笔删 `ManualReviewRequirementFor` 与基线条目 | `mcp2-wbr04` 基 `2c0008c3` | PS ports / `adapters/partycommercial` / 消费点、PC domain `authority_grant.go` / `acceptance_content.go`（仅该符号）、`cmd/*` PS 装配行 |
| MCP-5 | — | 本轮非作者评审待命 | — | — |

**共享点**：`production_wiring_baseline.txt` PC 段 MCP-3 与 MCP-2 各删一行（纯删行，改前广播，冲突推送方解）；两家都在 `internal/partycommercial/domain/` 但不同文件，派单里互列「不碰」。`4c5a1215` 由推送方代结 done（结果写明已进 main 的三笔与两张新单）。spec 状态行三票都完后推送方改一次。

**树与分支**：拆 `idp-parcel-mcp2-awf09rv` / `mcp2-psr02` / `mcp3-awf09` / `mcp3-awf10-review` / `mcp4-awf10` / `mcp5-awf08` / `mcp5-awf16-followup` / `mcp6-wbr03-05` / `idp-replay-wave1` / `idp-review-awf09` / `idp-verify-psr02` 共 11 棵（拆前逐棵 `status --porcelain --untracked-files=all` 零行、不带 `--force`；`mcp4-awf10` 与 `mcp5-awf16-followup` 两棵 git 注销后目录删不净——.git 文件已去、只剩干净检出的副本——`Remove-Item -Recurse -Force` 补删）；改名 merged/：`mcp2-awf09-review`、`mcp3-awf09`、`mcp3-awf10-review`、`mcp4-awf10`、`mcp5-awf16-followup`。**不动**：`idp-parcel-mcp6-awf19`[`mcp6-awf19` 已长到 `a21ef0c3`，MCP-6 在用]、`idp-parcel-mcp4-tf11`（接管现场）、`D:/tops/idp-tf03`（归用户）。剩余在途分支只剩 `mcp3-wbr03` / `mcp4-tf11` / `mcp6-awf19`（+ MCP-2 待建 `mcp2-wbr04`）。远端旧名（`origin/mcp1-awf08*`、`mcp2-awf08-review`、`mcp2-awf16-review`、`mcp5-awf11-review`、`mcp5-awf08`、`mcp5-awf16`、`mcp6-awf11`、`mcp6-wbr03-05` 等）未删，只是指针。

### 16:5x–17:0x：awf/19 完工 → MCP-5 评审无阻断 → 进 main；三路占号

- 16:5x MCP-4 认领 `a8e6a834`（report working），交现场分析：封存笔只动 `actual_fulfillment_segment_test.go`，Covers 改口与票面裁决 5（`ErrCorrectionWithdrawsControl` 退场）、1/4 一致，删的断言在裁决 5 下必红，辅助 `withdrawingCorrection` 未定义故 domain_test 红——**用**；接下来补辅助 + void 用例 red → 领域 → 0019 → PG → 编排 → HTTP。
- 16:5x MCP-6 交 awf/19 完工报（`mcp6-awf19@3d31f910`）；16:5x 派 MCP-5 非作者两轴评审 `875e90a3`（20 分钟）。MCP-6 报「立票笔 8a9856f9 不在 main」有误——它 = `06d918ea`（`cherry` 全 `-`、blob 同）；MCP-6 复核后认：三点 diff 量的是 merge-base→分支，没问 main 上有没有那个 blob。重放序列定为 `b470bfe6 → d6fdeacc → a21ef0c3 → 3d31f910`，跳 `8a9856f9` 与 merge 笔 `d572e554`。
- 17:0x MCP-5 交评审：Standards 阻断 0 / 非阻断 2（`col` 与 Row 同 policy-rows.ts 同形——接受作者「第三个消费者再抬」；只有壳一例手写正文格 id，nit），Spec 阻断 0 / 非阻断 0；自跑 tsc 0 / run-tests 116/116 / 过期话零命中含阳性对照；评审笔 `db42a887`（`mcp5-awf19-review` 基 `62bf6504`）。
- **重放**（隔离 detached 树 `idp-replay-awf19`，评审回来前已预演）：`62bf6504` 上 `b470bfe6→d8e713e0`、`d6fdeacc→cc86a911`、`a21ef0c3→e73d4963`、`3d31f910→797b52a2` 零冲突；`db42a887→84033830` 在票 19 EOF 撞一次（完成记录与 Comments 同在文末追加），手工并成完成记录在前、Comments 在后，评审段逐行在场。树等价：`3d31f910` 对 `797b52a2` 排除 tasks.md 只剩票 tf/11 .md（main 自己改的）。验证钉 `797b52a2`（之后只多 .md）：tsc `--noEmit` 0、run-tests 116/116、过期话零命中；`.go` / `.sql` 零变动，未跑 Go 门禁与清点（同 awf/16 先例）。本笔：票 19 Status → resolved、逐笔表 `8a9856f9` 行改正、补「进 main 记录」；`--ff-only` 进 main 后推。
- **占号**：16:57 MCP-2 wbr/04（`mcp2-wbr04` 基 `62bf6504`）：PS `ports.go` 加复核授权端口、`adapters/partycommercial/manual_review_authorization*.go` 新建、`application/complete_manual_review.go`+test（命令去掉 Authority、编排先问 PC）、`cmd/parcel-api/assemble_review.go`+test + `main.go` 一行、`cmd/parcel-dispatch/manual_review_resume_loop_test.go` 几行、PC domain `authority_grant.go`+test / `acceptance_content.go`（仅 `ManualReviewRequirementFor`）、基线 PC 段删一行 + 理由行改历史注 + 头注记数。17:0x MCP-3 wbr/03（`mcp3-wbr03` 基 `2c0008c3`）：README 只加 0127 一行；票面之外多三处**新增不改签名**——SA domain（`CreditBasis` + `CreditStanding.WithAuthorizedLimit`）、SA ports（`CreditBasisView`）、SA application `apply_pre_acceptance_control.go` Deps 加一项（nil 沿旧路，因 PS 夹具直构 Deps 而不碰 PS）；装配点 `cmd/parcel-dispatch/assemble.go` 的 `acceptanceFinancialControl`；基线 PC 段剪 `ResolveCreditPolicy` 一行 + `production_type_reachability_baseline.txt` 剪 `CreditBasis` 一行。两家文件零重叠；共享点只有基线 PC 段各删一行。

### 17:2x–17:4x：tf/11 完工 → MCP-6 评审无阻断 → 进 main；wbr/04 完工 → MCP-5 评审中

- 17:28 MCP-4 交 tf/11 完工报：`mcp4-tf11@21ee3af0`（基 `d1e6c094`，TF 地盘零漂移未 rebase；封存笔重切进领域笔，原指针留本地 `salvage/mcp4-tf11-red`）。五笔：`3559f5d5` 领域（Voided/失效版本/重建门）、`59b0bb1e` 编排（退 `CORRECTION_WITHDRAWS_CONTROL`，失效链尾答 `OBJECT_NOT_IN_SEGMENT`）、`244ad85d` 迁移 0019 + PG 在场谓词、`2d77d7de` 票面 resolved + spec + CONTEXT 一句、`21ee3af0` 清点。作者自验含 DSN 100 ok（7424 PASS）。17:3x 派 MCP-6 评审 `d2ca32a2`；同时隔离树预演重放并起含 DSN 全仓测试。
- 17:39 MCP-6 交评审（`mcp6-tf11-review@7e4e5fc5` 基 `a69c16f0`）：Standards 阻断 0 / 非阻断 3（跨文件计数头注一处 + 同族五处；替身各长一个同形 `active()`；中间笔 `3559f5d5` 单独不可编）、Spec 阻断 0 / 非阻断 2（越权 ④ 在 ADR-0114 决定二字面内但正文「两种」没跟成三种，归 owner；越权 ② 已进 TF CONTEXT 词条，owner 复核连看）。自跑 TF 六包含 DSN ok。
- 17:3x MCP-2 交 wbr/04 完工报：`mcp2-wbr04` 代码 tip `8e52f413`、票面笔 `04472784`（基 `62bf6504`）；逐笔 `67ca20d5` 消费点量法 → `f4e9a9e2` PS 端口 `ManualReviewAuthorizer` + 适配器（多一道 `ErrUntranslatableAnswer` 守卫）→ `80d100a2` 消费点接真 + 装配 + 同笔删 `ManualReviewRequirementFor` 与基线条目 → `8e52f413` 清点。自验含 DSN 100 ok 两遍。17:4x 派 MCP-5 评审 `baec1ba4`。作者自识两条地盘外非阻断（admin-web `ManualReviewCompletionOutcome` 联合少两取值；UC-PC-003 首切「二值」过期）记票面。
- **tf/11 重放**（隔离 detached 树 `idp-replay-tf11`）：`a69c16f0` 上 `3559f5d5→290299e3`、`59b0bb1e→383e65f2`、`244ad85d→090f64cd` 零冲突；`2d77d7de→5e8eb710` 票 11 Status 与 Comments 各撞一次（main 上 16:4x 那笔改过），取分支侧后本票对分支零差；清点笔 `21ee3af0` 不带（基 `d1e6c094` 缺 PC 0028），tip 重生成 `80b4b41f`（TF 迁移 18→19、合计 153→154）；评审 `7e4e5fc5→c02ccbe2`（Comments 末同 hunk，两者都留）。TF 地盘对 `21ee3af0` 零差。验证钉 `80b4b41f`：gofmt 空、build/vet 0、含 DSN 全仓 **100 ok / 0 FAIL / 16 无测试 / 0 cached**（17:35→17:45）。本笔：票 11 补「进 main 记录」+ 本节；`--ff-only` 进 main 后推。中间笔不可编按原样重放，不并笔。
- **接下来**：wbr/04 评审回来后重放（基线 PC 段与头注同 wbr/03 撞，先到先进、后到者冲突由推送方解）；wbr/03（MCP-3）在途；03/04 都进后 spec 状态行改一次。

### 17:4x–18:0x：MCP-3 crash → wbr/03 由 MCP-4 接手；wbr/04 评审无阻断 → 进 main

- 17:4x 用户报通道 3 crash，指 MCP-4 接手 wbr/03。MCP-4 17:46 取证：树 `idp-parcel-mcp3-wbr03` status 零行、HEAD = origin/mcp3-wbr03 = `eed205aa`（基 `2c0008c3` 上八笔：`85c4209f` PC 闭包解析 → `f0caf25a` PC postgres → `fdf6819e` ADR-0127 → `a532b500` SA 端口/领域 → `4162b421` SA→PC 适配器 → `84191cb7` parcel-dispatch 装配 → `84b9a313` 基线剪两行 → `eed205aa` 清点）；崩溃前终端截图：detached 检出 `eed205aa` 全仓含 DSN 100 ok，最后一步拆验证树退 255 后 crash。未做：票面完成记录/Status、完工报。原单 `5031a8a1` 结 failed「承接方换人」；接管单 **`3ffeb230`** 派通道 4（同分支接着做：隔离重验、写完成记录、Status resolved、完工报；代码不动除非重验红）。
- 17:5x MCP-5 交 wbr/04 评审（`mcp5-wbr04-review@90795093` 基 `a69c16f0`）：Standards 阻断 0 / 非阻断 2（`judgment_continuation.go` `ErrUnexpectedAuthorizationOutcome` 头注计数变旧；`ErrUntranslatableAnswer` 名义错位接受），Spec 阻断 0 / 非阻断 1（UC-PC-003 第四项「将来时」归 docs 收口）；守卫判在裁决字面内（走 error，端口仍三值）。自跑三包组含 DSN ok。
- **wbr/04 重放**（隔离 detached 树 `idp-replay-wbr04`，评审回来前已预演）：`6ae24ae2` 上 `67ca20d5→0a426d8b`、`f4e9a9e2→aed6d756`、`80d100a2→9d93f74f`、`04472784→9fd38552` 零冲突；清点笔 `8e52f413` 不带（基 `62bf6504` 缺 TF 0019），tip 重生成 `cd1a1fdb`（PS 141→142 / 140→141、缝 13→14、端口 365→366）；评审 `90795093→f68afec3`（Comments 末同 hunk，作者两条在前、评审在后）。验证钉 `cd1a1fdb`：gofmt 空、build/vet 0、含 DSN 全仓 **100 ok / 0 FAIL / 16 无测试 / 0 cached**（17:48→17:58）。本笔：票 04 补「进 main 记录」+ 本节；`--ff-only` 进 main 后推。计数头注那条非阻断交作者另笔 `mcp2-wbr04-followup`。
- **接下来**：wbr/03 收尾（MCP-4，`3ffeb230`）完工报到后点 MCP-6 或 MCP-5 评审；重放时基线 PC 段与头注对 wbr/04 撞由推送方解；03/04 都进后 spec 状态行改一次。MCP-6 空闲待评审。

### 18:0x–18:3x：MCP-2/3/6 再 crash；MCP-5 验证协议 .md 与 branch-state 修复进 main；wbr/03 评审无阻断 → 进 main

- 18:0x MCP-5 交纯 .md：`docs/agents/parallel-sessions.md` 验证协议加「全量只跑一次」（用户经通道 5 裁定「同意」：作者 build/vet 全仓 + 动过的包及反向依赖 + architecture，不跑两遍；评审只跑点名包组；推送方重放后带 DSN 全量一次；再省要动 pgtest 夹具——模板库，先量四段再改，是一张票的量）。推送方自审后 `18f62899→199bb73c`、`de715499→140bce84` 直接进 main；`mcp5-verify-tiers` 改名 merged/，树已拆。
- 18:00 MCP-4 代 MCP-3 交 wbr/03 完工报（代码 `eed205aa`、簿记 `94eda949` + `05e0e80f`；隔离重验含 DSN 100 ok，7548 PASS）；`3ffeb230` 结 done。评审先派 MCP-6（`d7be7d8a`）。
- 18:1x 用户报 **MCP-2 / 3 / 6 全 crash**（MCP-5 只读清点：所有树干净、无丢失——wbr/04 已在 main、mcp3-wbr03 本地=origin=`05e0e80f`、MCP-6 无在途）。`d7be7d8a`（MCP-6 评审）结 failed「换人」→ 改派 MCP-5 `f9db19ec`；`1d7276ee`（MCP-2 followup）结 failed「换人」→ 改派 MCP-4 `7befa278`（纯注释一笔 + 代拆 `idp-parcel-mcp2-wbr04`）。MCP-4 已自拆 `idp-parcel-mcp4-tf11`（先比内容）。
- MCP-5 顺手报 `scripts/branch-state.ps1` 一个 bug：在途分支节 `git cat-file -e main:<f>` 对分支新增而 main 无的文件往 stderr 写 fatal，`$ErrorActionPreference='Stop'` 下 `2>$null` 也挡不住，整节中断、在途分支一行不印（实测于 `mcp3-wbr03` 六个新文件）——推送方改为 `rev-parse -q --verify`（与文件内另两处同写法），`bfde8108` 进 main，重跑状态页在途节正常印出。
- 18:3x MCP-5 交 wbr/03 评审（`mcp5-wbr03-review@e53bd152` 基 `140bce84`）：Standards 阻断 0 / 非阻断 1（parcel-dispatch 接 `NewCreditBasis` 三行 cmd 测试零断言、nil 合法故漏接不红，随 SA contract 段收）；Spec 阻断 0 / 非阻断 0；越权四条：① ④ 字面内，② ③ 字面外不相悖。自跑三包组含 DSN ok；基线数字三检出复核一致。
- **wbr/03 重放**（隔离 detached 树 `idp-replay-wbr03`）：`140bce84` 上按 `2c0008c3..05e0e80f` 全列（MCP-4 的逐笔漏了 `95193261` ADR 立篇与 `05e0e80f`）十笔——七笔零冲突；`84b9a313` 在 `production_wiring_baseline.txt` 撞 wbr/04：头注两段各留 + 推送方重放注（钉父 `02918e11`：剪前 5 / 剪后 4，PC 3→2），PC 段两条目都出名单、两条「曾在这里」注都留；清点笔 `eed205aa` 不带，tip 重生成（SA 78→80 / 57→60、PC 测试 108→109、缝 1→2、端口 366→367）；architecture 门禁 ok；含 DSN 全仓钉 `075c71eb` **100 ok / 0 FAIL / 16 无测试 / 0 cached**（18:11→18:21）；随后 rebase 到 `bfde8108`（只多 .ps1）；评审 `e53bd152` cherry-pick（Comments 末同 hunk，作者两笔在前、评审在后）。本笔：票 03「进 main 记录」+ spec 状态行（PC 三条全进、PS 二条仍 draft）+ 本节；`--ff-only` 进 main 后推。
- **在途只剩**：wbr/04 纯注释 followup（MCP-4 `7befa278`），到后直接重放。**归用户**：awf 第 2 波前置「词表读口形状」；tf/11 越权 ①–④；wbr/03 越权 ①–④ 与「未落三件」（PS 登记面二维、SA contract 段 + 账本行政策引用、比例基数 BD）；PS 二条 wbr/01/02 待派；MCP-5 提的 pgtest 模板库是否开票；`D:/tops/idp-tf03` 拆留；远端旧名分支未删。

### 18:3x–19:3x：通道 1 代裁词表读口形状 → 立票 20；会话断在提交前，19:3x 新会话重放补记

- 18:3x 用户对「awf 第 2 波前置『词表读口形状』」回「你自决」，通道 1 代裁：**一口按 kind 答**（`GET /commercial-publication-vocabularies?kind=`），只答码不答中文，集合权威在 PC 领域枚举（`PublicationVocabulary(kind)` 从同一枚举列词、测试双向钉住与规范化器同词），坏 kind 答 `PayloadProblemRecord`，前端 `fetchPublicationVocabulary(kind)` 一处取；理由（五步路径已是一套载荷带 kind 分册，一册一口是四人改 `endpoints.go` 的撞点）与派单顺序记在伞票 07 Comments 同刻那条。立票 `admin-write-faces/20`（ready-for-agent，Blocked by 无：08 与 16 已进 main）；07 子票表加 20 一行；12/13/15/17 的 Blocked by 加 20（落点广播前下拉写占位、不内置枚举）。六个文件写完未提交、本节未记，会话即断。
- 19:3x 通道 1 新会话重放：`branch-state.ps1` 状态页——main = origin = `ca978fff`、在途分支零、共享树未提交 6 行（`--ignore-cr-at-eol` 核过是真改动，内容即上条）；`D:/tops/idp-tf03` 树已不在磁盘与 `worktree list`（18:30 快照与 MCP-4 收尾截图都还登记着它，之后被拆），`salvage/mcp4-tf03@44808f31` 本地与 origin 都在、无损。用户回「提」，本笔把六个文件 + 本节一笔进 main 后推。
- **接下来**：点名（截止 3 分钟）→ 只派应答者：**20 + 14 先开**（14 不依赖词表）；20 落点广播后 12/13/15/17 点名派。**归用户**（不变）：tf/11 越权 ①–④；wbr/03 越权 ①–④ 与「未落三件」；PS 二条 wbr/01/02 待派；pgtest 模板库是否开票；远端旧名分支未删。
- 19:37 点名（截止 19:40，目标 2–6），19:39 前 5 个应答全到、均「空闲 · 地盘无 · 余量充足」；派单第 0 步 `branch-state.ps1` 在途分支零，无半成品可撞。派 **awf/20 → 通道 4**（`b916b2d3`，分支 `mcp4-awf20`，基 `5a209f70`）、**awf/14 → 通道 2**（`7afbd72a`，分支 `mcp2-awf14`，基 `5a209f70`）；3/5/6 留作评审与第 2 波，已告知。两单都写明：共享文件（`endpoints.go`；`publication_canonicalization.go` 与其测试；`CommercialPoliciesPage.tsx`）改前占号、只加自己一格；验证按「全量只跑一次」作者层；20 落点广播给全部通道。
- 19:4x 顺手补簿记：psr/02 余段 11:2x 已重放进 main 但票面 Status 仍写「待重放」、无进 main 记录——补齐 `c135049b` 进 main 已推。通道 2 awf/14 占号到（共享文件各一格；地盘外 `party/api.ts` 口径两格 + `policy-rows.ts` 口径列——完成判据「含口径列」所要，准做，单独成笔只加不改）；通道 4 awf/20 占号到（`endpoints.go` 一行挂字面量 `UnconfiguredIntake{}`——认形不认「同预览口」那句理由：词表不要租户；真理由是唯一消费者是四口喂的表单、且它不读任何存储不满足 ADR-0078 放行判据，挂进 `commercialCatalogueIntake` 会让 `TestIsolatedReadAdmissionSwitchesOnlyOperationsReadLines` 二分多出第三形态，另一票的量；要求注释写真理由、完成记录单列判断点归 owner）。
- 19:5x 通道 4 广播 [落点 awf/20]（领域半边 `mcp4-awf20@0fb66cd8`）：`domain.PublicationVocabulary(kind)` / `VocabularySet{Name, Codes}`；`GET /commercial-publication-vocabularies?kind=` 答 `{outcome: PUBLICATION_VOCABULARY_LISTED, kind, sets}`，坏 kind 400 MALFORMED_REQUEST，未配置 403；TS `fetchPublicationVocabulary` / `VocabularySetRecord` / `vocabularyOptions(codes, labels)` 在 `publication-draft-api.ts` 文件末尾。19:54 点名 3/5/6（截止 19:57），19:55 三应答均空闲；派 **awf/12 → 通道 3**（`8325f4ee`，`mcp3-awf12`）、**awf/13 → 通道 5**（`e7836296`，`mcp5-awf13`）、**awf/15 → 通道 6**（`70599fbd`，`mcp6-awf15`），基 `c135049b`；三单都写：Go 先做、表单纯函数、等通道 4 推出 TS 那笔后 cherry-pick 借用（同 09/10 借 16）；共享文件占号只加一格。**17 等第一个空下来的通道**。此刻六通道全在干活、无人空闲作评审——完工报到时若仍无人，推送方按规矩用 `/code-review` 隔离子代理评，不由作者自评。

## 2026-09-08 20:2x 通道 1 由通道 2 的会话接听（接手时 `main = origin/main = 0f9eaa1e`；同一窗口仍是 awf/14 的作者会话）

用户 20:2x 在通道 2 的队列里写「请调用 idp-mcp-1 的 check_messages 开始监听」；`list_sessions` 显 MCP-1 `waiting=false`（无在线监听者，上次活动约 20 分钟前）→ 本窗口转听 idp-mcp-1。**利害写明**：本会话恰是 awf/14 的作者（通道 2 完工报 20:2x 刚发），按「合入前独立评审」它不能自评自己那张票。接手时队列里：awf/20 完工报（MCP-4，`mcp4-awf20@802ae400`）、awf/14 完工报（MCP-2，`mcp2-awf14@44f2b79d`，代码 tip `6bfd2f1b`）、MCP-4 应答接 awf/17（基 `802ae400`，20 进 main 后 rebase）、12/13/15/17 四路占号广播、awf/20 释号。六通道此刻全在干活，无人空闲作评审。

### 20:3x–20:5x：awf/20 评审（推送方自查）→ 重放 → 进 main；awf/14 等非作者评审

- 起两个 `/code-review` 隔离子代理（20 与 14 各一）——**都在第一步鉴权错死掉**（transcript 一行 `Authentication error`），与票 16 完成记录记的同一 harness 症状。awf/20 改由推送方（非作者）按 skill 正文串行自查：两轴无阻断，Standards 3 条非阻断（共享 TS 文件 import 行改了一行、按 kind 第三处 switch 判断题、`closedCodes` 扫值域记性能不是问题）、Spec 2 条非阻断（`PRICE_RULE` 三个封闭集今天答 `sets: []` 属票面正确、条目一行 + 注释几行已自报）；记录在票 20 Comments。**awf/14 不由本会话评**：等第一个空出来的非作者通道（预计 MCP-4 做完 17），届时派 20 分钟评审，评审无阻断再重放。
- **awf/20 重放**（隔离 detached 树 `idp-replay-awf20`）：`0f9eaa1e` 上六笔零冲突 `509b67ce→fa1f249a`、`0fb66cd8→fdb3043a`、`d894751a→00a122d0`、`4eb2492f→2acf6107`、`17a962a5→794ec2dc`、`802ae400→c29b50d2`；清点笔 `b0667d3b` 不带，tip 重生成 `c05ee34d`（PC 生产 105→107 / 测试 109→111、端点 106→107）。验证钉 `c05ee34d`：gofmt 空、build / vet 0、含 DSN 全仓 `-p 1 -count=1` **100 ok / 0 FAIL / 16 无测试 / 0 cached**（20:44→20:54，581s）、tsc 0、run-tests 123/123、探针 PG 用例 PASS。本笔：票 20 Comments 补评审 + 进 main 记录、伞票 07 子票表 20 行转 resolved、本节；`--ff-only` 进 main 后 `ls-remote` 核 `0f9eaa1e` 再推。曾先按「20 + 14 一起重放」建过 `idp-replay-awf2014`（`cae4c98d`，两票零冲突、`publication-draft-api.ts` 两段互不相扰、tsc 0 / 134 tests），含 DSN 全量跑到一半时意识到 14 无评审不得推，杀掉那一跑（连同孤儿 `go test` / `postgres.test.exe` 进程）改跑 20 单独的 tip——那棵树留着等 14 评审回来重建。
- **接下来**：awf/14 评审派给第一个空出来的非作者通道（备选：MCP-4 完成 17 后）；17 基 `802ae400`（20 的分支 tip），20 进 main 后作者 rebase 到 main；12/13/15/17 完工报到后同法。**归用户**（不变）：tf/11 越权 ①–④；wbr/03 越权 ①–④ 与「未落三件」；PS 二条 wbr/01/02 待派；pgtest 模板库是否开票；远端旧名分支未删；awf/20 判断点「词表读口隔离读态不放行」。
## 2026-09-08 21:1x 通道 1 由通道 4 的会话接听（接手时 `main = origin/main = 0f9eaa1e`；同一窗口是 awf/20 与 awf/17 的作者会话）

用户 21:1x 在通道 4 的队列里写「请调用 idp-mcp-1 的 check_messages 开始监听」→ 本窗口转听 idp-mcp-1。**利害写明**：本会话是 awf/20 与 awf/17 的作者，两票都不能由本会话评审；20 的评审与含 DSN 全量已由前一任（通道 2 窗口）在 20:5x 做完，本会话只推它验过的 SHA。接手时队列里：awf/17 完工报（MCP-4，自己的，`mcp4-awf17@9799785a` 基 `802ae400`）、awf/15 完工报（MCP-6，`mcp6-awf15@d79093d4`，已变基到 `802ae400`）、awf/13 由通道 6 接手（通道 5 crash，其树六份未提交 mtime 停 20:12:13，通道 6 先封存再接）、awf/12 通道 3 crash（`mcp3-awf12@febfcd2e` 五笔已推、树干净无现场）。前一节（20:2x 通道 2 窗口）会话断在 `--ff-only` 推 main 之前，末尾五数未写。

### 21:2x：推 awf/20；派 17 评审 + 12 续单；14 / 15 评审起隔离子代理

- **awf/20 进 main**：`ls-remote` 核 `0f9eaa1e` 未动 → `git push origin 3ca75da6:main` → 远端 `3ca75da6`，共享树 ff。推的是前一任验过的链（代码 tip `c05ee34d` 含 DSN 100 ok；`3ca75da6` 只多 .md），`git diff 802ae400 3ca75da6 -- internal cmd apps` 为空。两棵遗留树：`idp-replay-awf20`（`3ca75da6`，已进 main，可拆）、`idp-replay-awf2014`（`cae4c98d`，20 的另一套 SHA + 14，作废——20 已按另一链进 main，14 评审后从 main tip 重建）。
- 派单：**awf/17 评审 → 通道 2**（非作者，20 分钟，报告发通道 1 由推送方写票面），完成后**接 awf/12**（从 `mcp3-awf12@febfcd2e` 接着做，树自己开 `mcp2-awf12`）（`2b69f06d`）。**awf/14 / awf/15 评审**起两个隔离子代理（本会话对两票都非作者；前一任的子代理鉴权错，本次起成了），报告回来写进票面。
- **接下来**：评审无阻断者按 main tip 重放（17 / 15 取 `802ae400..tip`，14 取 `5a209f70..44f2b79d`），tip 重生成清点，含 DSN 全量一次，`<sha>:main`。**归用户**（不变）：tf/11 越权 ①–④；wbr/03 越权 ①–④ 与「未落三件」；PS 二条 wbr/01/02 待派；pgtest 模板库是否开票；远端旧名分支未删；awf/20 判断点「词表读口隔离读态不放行」。

## 2026-09-08 21:2x 通道 1 新会话接续（接手时 `main = origin/main = f722d9f0`；用户报「所有通道都 crash 了，现在恢复了」）

用户先要一次自带 `ask_question` 调用，随后「监听队列」→「看看各通道工作树的现场，如何继续，需要回放吗」。前一节（通道 4 窗口）断在派单之后，14 / 15 的两个隔离评审子代理随之死亡、未交报告；五数未写。接手时队列一条：MCP-5 新会话自报空闲、不再碰 `mcp5-awf13`。

### 21:2x 只读盘点（`scripts/branch-state.ps1` 钉 `f722d9f0`）

- main = origin = `f722d9f0`；共享树 0 行未提交；9 棵树全干净。唯一动静：`%TEMP%\idp-review-awf17`（detached `9799785a`）21:21 有一个未跟踪探针 `zz_review_seed_digest_test.go`、21:27 已消失——通道 2 新会话在做 17 的评审（树 21:19 建），不动。
- 六支在途分支全部推到 origin；`mcp4-awf12@febfcd2e` 与 `mcp3-awf12` 同 SHA、零独有提交、未推——来历不明的空树，可拆。
- 票况：20 已进 main；**17 / 15 / 14 三张 resolved、等非作者评审后回放**（14 与 main 只在 `publication-draft-api.ts` 同文件，前任试过零冲突；15 / 17 基 `802ae400`）；**12 in-progress**（`mcp3-awf12@febfcd2e` Go 四笔 + seed；admin-web 与完成记录未做；台账 `8325f4ee`（通道 3）与 `2b69f06d` 第二件（通道 2）两张单都指向它）；**13 in-progress**（`mcp6-awf13@86b55e86`：封存通道 5 现场 + 应用层一笔；admin-web、清点、完成记录未做；台账 `e7836296` 仍 pending 在通道 5，通道 6 是新会话无记忆）。
- 答用户「需要回放吗」：要，但此刻不能——14 / 15 / 17 卡在评审门，12 / 13 未完工；没有一笔代码丢失。

### 21:28–21:4x：awf/17 评审到（通道 2，无阻断）→ 重放 → 进 main；12 承接方定为通道 2

- 21:28 通道 2 交 17 评审（钉 `9799785a`）：Standards 0 阻断 / 5 非阻断（最重：`authorizationRuleFieldPaths` 认领节根不渲染其 `Problems`）、Spec 0 阻断 / 1 非阻断（tsc/run-tests 评审树未复跑）；四问全成立，seed 摘要独立算得同值。全文落票 17 Comments。通道 2 随即按 `2b69f06d` 第二件从 `febfcd2e` 开 `mcp2-awf12` 接 awf/12——**12 归通道 2 就此定死**，`8325f4ee`（通道 3）结 failed「承接方换人」，另给通道 3 一条「勿认领、勿碰 mcp3-awf12 树」。
- **17 重放**（隔离 detached 树 `idp-replay-awf17` 基 `f722d9f0`）：跳清点笔 `e9ffa685`，八笔零冲突 `3264f362→6148548e`、`8c89f282→3b15236c`、`e2302ecf→9660a360`、`d756e2d9→a895f003`、`a0c126e4→772c9339`、`f5fc7117→a954ac0f`、`fcf3d703→ec6289f7`、`9799785a→a9d1af33`；代码树对分支 tip 零差。清点在 tip 重生成 `1d6ac983`（PC 生产 107→109 / 测试 111→114、声明面 20→21），与 `e9ffa685` 逐字节同。验证钉 `1d6ac983`：gofmt 空、build/vet 0、含 DSN `go test -p 1 -count=1 ./...` **100 ok / 0 FAIL / 16 无测试 / 0 cached**（587s）、探针 PC http `-run AuthorizationRule` PASS 2、admin-web tsc 0 / run-tests 131/131。本笔：票 17 Comments 评审 + 进 main 记录、伞票 07 子票表 17 行 resolved、本节；`--ff-only` 进 main，推前 `ls-remote` 核 `f722d9f0`。
- **接下来**：点名后派 **15 → 3/4/5 之一**（不能 6）、**14 → 3/4/5/6 之一**（不能 2）各 20 分钟评审；**13 → 通道 6** 从 `mcp6-awf13@86b55e86` 接着做（`e7836296` 结 failed「换人」、`mcp5-awf13` 改名 `salvage/`）；评审无阻断者按到达先后重放（15 取 `802ae400..d79093d4` 跳清点笔 `23a4ad59`；14 取 `5a209f70..44f2b79d` 跳清点笔 `c7635837`），共享 Go 文件与 `CommercialPoliciesPage.tsx` 相邻行冲突按第 1 波手工并；拆 `mcp4-awf12` 空树；`mcp4-awf17` 改名 `merged/`。**归用户**（不变）：tf/11 越权 ①–④；wbr/03 越权 ①–④ 与「未落三件」；PS 二条 wbr/01/02 待派；pgtest 模板库是否开票；远端旧名分支未删；awf/20 判断点；17 非阻断 (1) 是否要作者另笔修。

### 21:5x–22:0x（本段由 09-09 的接手会话据现场补记，前一任断在提交簿记之前、这一段它自己没写下）

- 14 / 15 评审派出并收回：**14 ← 通道 5 · 钉 `44f2b79d` · 21:52**，两轴无阻断（Standards 非阻断 2、Spec 非阻断 1）；**15 ← 通道 3 · 钉 `d79093d4` · 21:56**，两轴无阻断（Standards 非阻断 2、Spec 非阻断 1）。全文已写进两票 Comments。
- 15 八笔 + 14 七笔重放到隔离 detached 树 `%TEMP%\idp-replay-awf1514`（基 main `6b1e0d63`），清点在 tip 重生成 `ef6cb914`；两票「进 main 记录」与伞票 07 子票表两行写好**未提交**，含 DSN 全量未跑（或跑了没记）、未推。会话在此断掉。

## 2026-09-09 10:4x 通道 1 新会话接续（接手时 `main = origin/main = 6b1e0d63`；用户先要一次自带 `ask_question`，再「监听队列」）

接手时队列一条：另一会话的盘点结论「12/13/14/15 未回放但都能回放、无代码丢失、阻塞在评审门」。核过：四支分支本地 = 远端、领先数 8/16/8/16、均不在 main；`mcp3-awf12` = `salvage/mcp4-awf12` = `febfcd2e`。补一处：`idp-replay-awf1514@ef6cb914` 树上躺着 3 份未提交票面（07 / 14 / 15）——读了才知道那正是前一任收到的 14 / 15 评审报告与写好的进 main 记录，上一节据此补记。

### 10:46 点名（截止 10:49）→ 派 12 / 13 评审；14 / 15 误派后撤回

- 10:46 广播点名（目标 2–6），10:49 前 **4 个应答**：2 / 3 / 4 / 5 均「空闲 · 地盘无 · 余量充足」；**通道 6 截至 10:49 未应答**。
- 10:5x 派四份评审：**12 → 通道 3**（`d0714738`，钉 `76c35c4a`，基线 `f722d9f0`）、**14 → 通道 4**（`922a4194`）、**13 → 通道 5**（`f7a9f921`，钉 `2599816f`，基线 `802ae400`——分支基 20 的 tip，不用 merge-base 否则把 20 的活算进 diff）、**15 → 通道 2**（`1ae5d58f`）。随即读到回放树里 14 / 15 已有评审 → **`922a4194` / `1ae5d58f` 撤回**（重复派单，各发通道 4 / 2 一条撤回令、结 failed）。派错的原因：先答用户「四张都无评审」时只看了分支上的票面，回放树的未提交改动那时还没读。
- 回放树复核（本会话不信前任自报）：`git range-diff 802ae400..d79093d4 6b1e0d63..6de76d44` 与 `5a209f70..44f2b79d 6de76d44..44008e42`——非共享文件各笔 `=`，六份共享文件各笔 `!` 且差异全在上下文行（邻册 17 / 20 / 15 的加行），加行本身逐字相等；`publication-draft-api.ts` 上 15 / 14 的新接口块落位后移一段（纯加块换位，语义无关）。
- 验证钉 `ef6cb914`（Docker Desktop 本机未起，起后 `compose up -d`）：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；admin-web `tsc --noEmit` 退 0（node_modules 用 junction 借共享树的）、`run-tests` 151/151；含 DSN `go test -p 1 -count=1 ./...` **99 ok / 1 FAIL / 16 无测试 / 0 cached**（555s）。**红的一条**：`cmd/parcel-commercial` `TestAPublishedSettlementPolicyIsAdoptedOnlyOnTheExactSixDimensions`——夹具 `settlementBatchBody` 声明 `sha256:settlement-1`，本册接进 PCC-1 后对账门开了答 NOT_ACCEPTED（算出 `PCC-1:cab5c83b…`）。在作者分支 tip `d79093d4` 上带 DSN 单跑同红，**是 15 自带的缺陷**，作者自验与评审都只跑 PC 包组 + `cmd/parcel-api`，这一包无 DSN 时 `t.Skip` 所以两边都绿。同法查 13：`mcp6-awf13@2599816f` 上 `TestAPublishedPreAcceptanceFinancialControlPolicyIsReadBackByTheContentView` 同症（`sha256:fcp-1`，算出 `PCC-1:07cd1a96…`）；12（`76c35c4a`）该包带 DSN ok。
- 11:1x 派 **通道 6（作者）**：15 / 13 各自分支各补一笔改夹具摘要、带 DSN 跑 `cmd/parcel-commercial`（`14915d93`，11:35 前）；知会通道 5 把 13 这条写进评审作阻断。**11:2x 用户告知：通道 6 根本没有会话**——点名未应答的真相；`list_sessions` 对它显 `online / idle`，这一格不可信。`14915d93` 结 failed「承接方换人」，改派 **15 补笔 → 通道 4**（`ee13fad5`；非作者、非其评审者）、**13 补笔 → 通道 5**（`c549b9b9`；13 的评审者，它自己在补记里请缨）。两棵 `mcp6-*` 树都干净、分支已在 origin，接手方直接在树上接着做、不另起。15 的补笔到后 cherry-pick 到回放链 tip、只重跑 `cmd/parcel-commercial`（其余包与 `ef6cb914` 同码），再推 main。
- 11:03–11:08 四份评审到齐：**12 ← 通道 3 · 钉 `76c35c4a`**：两轴无阻断、Spec 非阻断 3（有效期格被立成独立节 / 票面错列 `pendingRoutingBasis` / seed 摘要未运行验）。**13 ← 通道 5 · 钉 `2599816f`**：Standards 0 阻断 4 非阻断、Spec 起初 0 阻断 2 非阻断，11:08 补记把推送方实测的 cmd 夹具红列为 **1 阻断**→回作者（即上面的改派）。**14 ← 通道 4**、**15 ← 通道 2**：都是撤回令到前已完工的重复评审，同 tip 同结论「无阻断」，各多两条判断题；两单保持 done，台账记「第二份非作者评审」。全文各落票面 Comments。
- 11:1x–11:2x **12 重放上链**（评审无阻断，不等 15 的补笔——补笔只碰 `cmd/` 夹具，链上顺序无关）：`f722d9f0..76c35c4a` 跳清点笔 `9c7e6da6` 后七笔 `3c339029→2c05a8a4`、`94df574c→2cb23a35`、`d8ba1669→768ca5cc`、`3f07466c→233ac1b1`、`b439a3a0→36cd0b18`、`9a473cf0→85d8efdd`、`76c35c4a→2c823378`；六份共享文件各撞一次 17 / 15 / 14 的相邻加行，逐 hunk 手工并，每份核过「零删行、加行多重集与分支上那笔逐行相等」；链 tip 对 `76c35c4a` 的差恰为 17 / 15 / 14 / 20 文件集 + `publish_commercial_authority_test.go`（那几票改配样例）+ `publish-batch.json`（15 / 17 的 seed 摘要）。链上 `go vet` 0、PC 四包 + `cmd/parcel-api` + architecture 含 DSN 全 ok、tsc 0、run-tests 161/161；清点重生成 `6edabc5c`（PC 生产 113→115 / 测试 120→123、http 适配器 23→24）。
- 11:2x 两笔补笔到：**15 ← 通道 4** `mcp6-awf15@60494959`（+3/−1，只换那一格摘要 + 头注两行；带 DSN 该包 ok、`-v` 核 SKIP = 0）；**13 ← 通道 5** `mcp6-awf13@da2736e5`（代码 `e58a085a` 同法；票面加「评审后补一笔」）。15 的补笔 cherry-pick 上链 `60494959→a67a2419`，**最终代码 tip `a67a2419`**。
- **验证钉 `a67a2419`**（15 + 14 + 12 三票共一次）：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；含 DSN `go test -p 1 -count=1 ./...` **100 ok / 0 FAIL / 16 无测试 / 0 cached**（548s）；admin-web `tsc --noEmit` 退 0、`run-tests` **161/161**。
- 推送方自查出第二个漏：派 13 评审时让通道 5 跳过 `896994ac`（salvage 笔），但那一笔装的是本票 Go 领域 / 载荷的主体（`publication_canonicalization_pre_acceptance_financial_control_policy.go` +157 及 _test +240、`publication_canonicalization.go` +20、`pre_acceptance_financial_control_policy.go` +26/−12、http 载荷 +76），后五笔建在它上；且按「换过会话的同一通道也算同一人」通道 5 对它是作者。**13 补评 → 通道 3**（`0b952bbb`，钉 `da2736e5`，重点封存笔四份 Go + 补笔，其余五笔抄查接缝），无阻断后 13 另起一链从新 main tip 重放。
- 本笔：票 12 / 14 / 15 Comments（四份评审 + 12 的一份）与「进 main 记录」、伞票 07 子票表 12 / 13 / 14 / 15 四行、本节；之后 `--ff-only` 进共享 main，推前 `ls-remote` 核 `6b1e0d63`。分支 `mcp2-awf14` / `mcp6-awf15` / `mcp2-awf12` 内容已全在 main → 改名 `merged/`；`mcp3-awf12`（旧来源，= `salvage/mcp4-awf12`）改名 `salvage/`。树：`idp-review-awf1[245]` 各评审者已自拆；`idp-replay-awf1514` 推完拆；`D:/tops/idp-parcel-mcp6-awf15` 无主（通道 6 无会话）、干净、内容全在 main → 推送方拆，指针留；`mcp2-awf12` / `mcp2-awf14` 两树归通道 2 自拆。**归用户**（不变）：tf/11 越权 ①–④；wbr/03 越权 ①–④ 与「未落三件」；PS 二条 wbr/01/02 待派；pgtest 模板库是否开票；远端旧名分支未删；awf/20 判断点；17 非阻断 (1) 是否要作者另笔修；**新增**：共享树 70 个 CRLF ` M` 文件怎么处置（`git add --renormalize` 或 `checkout --`）；同形票的完成判据是否加「带 DSN 跑 `cmd/parcel-commercial` 发布用例」（通道 5 建议）；15 评审两条判断题（客户相对方解析册未裁、壳引用与发布未决排序门）归伞票 07。
- 共享树此刻 70 个文件 ` M` 但 `git diff` 为空、全是 CRLF（mtime 全停 10:18:10）——某工具整批改了行尾，不是谁的活；不动，归用户。

## 2026-09-09 12:0x 通道 1 新会话接续（接手时 `main = origin/main = 568cc50f`；用户先要一次自带 `ask_question`，再「监听队列」→「继续」）

接手时队列两条：通道 2 报 `mcp2-awf12` / `mcp2-awf14` 两树已拆（核过：`worktree list` 无、目录不存在、`merged/` 两指针在、main = origin = `568cc50f`，回执）；另一会话的盘点结论「12/13/14/15 未回放」——已过时（`6b1e0d63..568cc50f` 正是 15 / 14 / 12 的重放链），只剩 13。

### 11:5x–12:xx：awf/13 接着重放 → 进 main（伞票 07 第 2 波至此全部进 main）

- 现场：`%TEMP%\idp-replay-awf13`（detached，基 `568cc50f`）躺着 `896994ac` 的 cherry-pick 冲突现场：两份共享文件 `UU`，`publication_canonicalization.go` 一处 hunk 的 `<<<<<<<` 已被前一任删掉、`=======` / `>>>>>>>` 还在（mtime 11:43:19，比其余五份晚一分钟）——前一任解到一半即断，本会话据 git 现场接着解，未凭自报。评审门核过三份：通道 4（09-08 22:03，钉 `2599816f`）、通道 5（11:04 + 11:08 补记，钉 `2599816f`；补记的 1 阻断由 `e58a085a` 修平）、通道 3 补评（11:36，钉 `da2736e5`，封存笔 Go 主体）——均「无阻断，可重放」；全文从通道 1 队列 `channels/1/messages.json` 取回落票 13 Comments（前一任没来得及写，`saved_progress` 停在 09-08 19:2x 也没有）。
- **13 重放上链**：`802ae400..da2736e5` 跳清点笔 `bdebd840` 后十笔 `896994ac→53480472`、`1a87d061→ea1a6cc1`、`86b55e86→17050991`、`eac0f65b→bab8b213`、`c4bbed53→dbc930aa`、`b74f9808→7eccadcf`、`5d56ad2a→7a6385fd`、`2599816f→c4378298`、`e58a085a→8c8c068b`、`da2736e5→71a6983d`（封存笔照进——它是本票 Go 主体，非纯簿记）。六份共享文件共十处 hunk 撞 12 / 14 / 15 / 17 的相邻加行（`publication_canonicalization.go` 2、`publication_draft_payload.go` 2、`publication_draft.go` 1、`publish_commercial_authority.go` 1、`publication-draft-api.ts` 1、`CommercialPoliciesPage.tsx` 3），逐 hunk 手工并，本册一律排在 12 那一册之后；每份核过「零标记、零删行、加行多重集与分支上那笔逐行相等」。链 tip 对 `da2736e5` 的差恰为 main 自 `802ae400` 起前进的 58 个文件、无一多余；八份本票与 main 都动过的文件上，链上本票净差 = 分支上本票净差（逐行）。清点重生成 `c4b71675`（PC 生产 115→117 / 测试 123→126、http 适配器 24→25，增量与分支清点笔同）。
- **验证钉 `c4b71675`**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；admin-web `tsc --noEmit` 退 0（node_modules junction 借共享树；注意 `npx tsc` 会解析到 npm 上那个占位包「This is not the tsc command you are looking for」且退 0——要跑 `node node_modules/typescript/bin/tsc --noEmit`）、`run-tests` **172/172**（161 + 本票 11）；含 DSN `go test -p 1 -count=1 ./...` **100 ok / 0 FAIL / 16 无测试 / 0 cached**（572s）。
- 本笔：票 13 Comments（三份评审 + 推送方注）与「进 main 记录」、伞票 07 子票表 13 行、本节；之后 `--ff-only` 进共享 main（共享树 70 个 CRLF ` M` 文件与本链 19 个文件零重叠，ff 不碰它们），推前 `ls-remote` 核 `568cc50f`。分支 `mcp6-awf13` 内容全在 main → 改名 `merged/`；`mcp5-awf13`（封存现场、`896994ac` 的来源）改名 `salvage/`；树 `idp-replay-awf13` 推完拆（先 rmdir junction 再 `worktree remove`，避开通道 2 遇到的 node_modules 残留）；`D:/tops/idp-parcel-mcp6-awf13` 无主（通道 6 无会话）、干净、内容全在 main → 拆，指针留。
- **接下来**：伞票 07 子票 08–17 + 20 全部进 main，只剩 18（draft，等用户裁管理台读面票要不要立）——07 转 resolved 卡在 18 一张。**归用户**（不变）：tf/11 越权 ①–④；wbr/03 越权 ①–④ 与「未落三件」；PS 二条 wbr/01/02 待派；pgtest 模板库是否开票；远端旧名分支未删；awf/20 判断点；17 非阻断 (1) 是否要作者另笔修；共享树 70 个 CRLF ` M` 文件怎么处置；同形票完成判据是否加「带 DSN 跑 `cmd/parcel-commercial` 发布用例」（13 / 15 两票的评审与推送方各撞一次）；15 评审两条判断题归伞票 07；`branch-state.ps1 -Classify` 把已重放的分支误报 `ABSORBED` / 未推 origin（另一会话实测），要不要修；**新增**：12 / 13 / 15 三票评审都点名的两处 Duplicated Code——各册 `*Named` 反查与 awf/12 泛型 `closedCodeNamed` 合一、表单私有 `Field` / `useLoaded` / 整数解析抬 party 共享层——要不要立票。
### 12:3x 用户「你是系统和业务专家，你自决，我的目标是全部解决」→ 通道 1 按 owner 授权代裁一批（本笔只动 .md）

- **CI 真相（`gh run list -b main`）**：自 09-08 02:45Z（10:45 本地）起 main 上 **34 个 run 全部 failure、零 step 跑过**，注解原文「The job was not started because recent account payments have failed or your spending limit needs to be increased」——是 GitHub 组织 `idp-xyz` 的 Actions 计费，不是代码；09-08 起所有「CI 红三次」「切换条件连绿三次」的账都没有基准。**归用户（仓外动作，任何通道做不了）**：到组织 Billing & plans 处理付款 / 提高 spending limit；处理完 `gh run rerun` 最新 main run 即可拿到 `3d90130c` 的真结果。这一天半里进 main 的每一笔都只靠推送方本机含 DSN 全量守着。
- 顺手清掉的两件：共享树 70 个 CRLF ` M` 文件——`git diff` 零内容差，`git checkout --` 恢复索引里的 LF 版本，状态零行；远端 31 支冗余分支（6 支旧名 + 25 支 `merged/*`，每支本地都有同 SHA 指针）`push --delete` 清掉，远端只剩 main + `mcp3-awf12` / `mcp5-awf13` / `salvage/mcp4-tf03` 三支 salvage（防丢，留）。
- **owner 复核认可**（写在各原文旁，`owner-review-queue.ps1` 下次能标出）：tf/11 越权 ①–④；wbr/03 越权 ①–④；awf/20 判断点「词表读口隔离读态不放行」；ADR-0114 自己的四条 + 决定二「被替代或已离场」自 tf/11 起覆盖已失效的复核记录。
- **裁决落文**：PC CONTEXT 结算方式那条规则末尾补两句（客户相对方 = 业务参与方不是货主客户账户；六维合同同时作壳引用）；parallel-sessions「验证」节补「反向依赖里的 `cmd/*` 包一律带 DSN 跑」及 13 / 15 的实测理由；wbr/01「要先裁的一格」代裁（两件都要、接管记录在前、缺格各落其未决、决定记录分格，ADR-0128 由实施者落）；wbr/02 取 (b) 删；wbr/03 未落三件各立票。
- **立票九张**（全部 ready-for-agent，阻塞边写在票面）：awf/21 客户服务规则册读面（18 的前置；18 Blocked by 改指 21）· awf/22 九张表单私有件抬 party 共享层 + 17 非阻断 (1) 节根问题渲染（Blocked by 21、18）· awf/23 结算政策相对方从业务参与方册选 + 六维合同镜像壳引用（Blocked by 22）· awf/24 各册 `*Named` 合一到 `closedCodeNamed` · wbr/08 PS 登记面补信用二维（取补两维）· wbr/09 SA contract 段 + 暴露账本行政策引用 + PS 夹具替身 · wbr/10 比例额度基数由政策正文声明（封闭集成员交 `/domain-modeling`，ADR-0129）· pgtest-template-database/01 先量四段再模板库 · agent-docs-local-env/02 `branch-state -Classify` 换「加行多重集包含」判据。伞票 07 子票表加 21–24 四行，07 收口判据不变（08–18）。
- **未复核、待派或自审**：其余 ADR 越权风险点（0113 / 0116 / 0119 / 0120 / 0123 / 0124 / 0125 / 0126；0027 / 0029 / 0055 命中的是「越权探测」这个领域词，不是风险点，脚本误收）与 16 张票面里的越权点——按 ADR 分四组派非作者通道预审（每条 认可 / 不认可 + 依据 ≤ 80 字），通道 1 复核后写认可行。
- **本节五数**：CI main 绿/总 **0/34（全部计费未启动，非代码）** · 集成时延中位数 —（13 完工报 09-08 21:5x → 推出 09-09 12:xx，横跨评审门、一次补笔与三次会话重置，1 票）· 重放笔数 10 · 重复开发 0 次 · 会话重置 1 次（通道 1 自己）

## 2026-09-09 14:0x 通道 1 新会话接续（接手时 `main = origin/main = 74ef0da8`；用户先要一次自带 `ask_question`，再「监听队列」→「各通道原来的工作全部中断，如何继续，是否重派」）

### 12:5x–13:08（本段由 14:0x 的接手会话据现场补记，前一任断在提交之前、这一段它自己没写下）

- 12:59–13:03 派五单（基 `74ef0da8`，每单一条专用 worktree）：**wbr/01 → 通道 2**（`0ad35a6d`，`mcp2-wbr01`，含 ADR-0128、PS 迁移号预给 0021）· **awf/21 → 通道 3**（`2306e16d`，`mcp3-awf21`，完成后同分支接 awf/18）· **wbr/08 → 通道 4**（`6eb31b85`，`mcp4-wbr08`，PS 迁移号 0020）· **pgtest/01 → 通道 5**（`cc7f42b9`，`mcp5-pgtest01`）· **awf/24 → 通道 6**（`057b6dc3`，`mcp6-awf24`）。派单原文用户留在仓根 `d_tasks.txt`（未跟踪，归用户，不入库）。
- 13:06–13:08 通道 1 自己在共享树写 owner 复核认可：十篇 ADR（0112 / 0113 / 0116 / 0119 / 0120 / 0122 / 0123 / 0124 / 0125 / 0126）各加「owner 复核记录」节、十二张票面 Comments 各加一行「owner 复核 2026-09-09 认可」、`owner-review-queue.ps1` 匹配词收窄为「越权风险点|越权点」（只匹「越权」把 0027 / 0029 / 0055 的领域词「越权探测」误收，09-09 实测三篇）。写完未提交，约 13:09 全部通道（含通道 1）会话中断。

### 14:0x 只读盘点（`scripts/branch-state.ps1` 钉 `74ef0da8`）

- main = origin = `74ef0da8`；五支在途分支全部已推 origin（同 SHA）。各树：`mcp2-wbr01@a9566d7b` 只有认领笔、树干净；`mcp3-awf21@1d4a308c` 认领笔 + **树上四份未提交**（`party/api.ts` / `policy-rows.ts` / `policy-rows.test.ts` / `presentation.ts`，+278/−5，mtime 13:05–13:08:51——客户服务规则册读面的类型、列集、三格单元格函数与五条测试已写成，未跑 tsc / run-tests、未提交）；`mcp4-wbr08@fe7c7253` 只有认领笔、干净；`mcp5-pgtest01@788e226c` 认领笔 + 打点笔（`IDP_PARCEL_PGTEST_TIMING` 四段 TSV，新文件 `pgtest/timing.go`），干净，量数未跑；`mcp6-awf24@cc22b9b0` 只有认领笔、干净。**没有一行代码丢失**；共享树 23 份未提交是上段通道 1 自己的活（核过全部为合法 UTF-8、每篇 ADR 复核节完整）。
- 台账五单仍 pending，无一 report working；`list_sessions` 对 2–6 显 `running / idle`、lastActivity 全停 13:04–13:10——这一格不可信（11:2x 那次对无会话的通道 6 也显 online）。
- 答用户「是否重派」：**要**——新会话无在途记忆，队列里的原单已被旧会话消费；但不是从头派，是接续单：树与分支都在，写明「从 `<分支>@<SHA>` 接着做、不另起树」，通道 3 那张多一句「先读树上四份未提交、跑 tsc + run-tests，能过就先提一笔」。原单结 failed「会话重置，接续单另派」，接续单里指回原单 id（新会话 `query_tasks` 能读到原单全文，接续单只写差异）。

### 14:05 点名（截止 14:08）→ 五张接续单全部派出

- 14:05 广播点名（目标 2–6），14:07 前 **5 个应答全到**：2 / 3 / 4 / 5 / 6 均「空闲 · 地盘无 · 余量充足」。派单第 0 步已在上节：五张票各有一条半成品分支，全部按「从 `<分支>@<SHA>` 接着做、不另起」派回原通道（树名带通道号，换通道只会让树名说谎）。Postgres 门禁 `idp-parcel-postgres-gate` 在跑（55432 healthy）。
- 14:07–14:10 原单五张各结 failed（result 写现场 SHA 与「接续单另派」），接续单：**wbr/01 → 通道 2**（`27028f77`，从 `a9566d7b`，先落 ADR-0128）· **awf/21 → 通道 3**（`134c240c`，从 `1d4a308c` + 四份未提交：先按票面判据自列该钉什么、再读 diff 与五条测试对照、跑 tsc + run-tests，绿就先提一笔；不许 checkout/stash 抓掉现场）· **wbr/08 → 通道 4**（`39ed9dec`，从 `fe7c7253`，/tdd 第一层起）· **pgtest/01 → 通道 5**（`c3d1a7ce`，从 `788e226c`，先核 `timing.go` 只加打点、再量；量前对 2–6 广播量数窗口）· **awf/24 → 通道 6**（`bb38aeda`，从 `cc22b9b0`，表驱动测试起；与通道 3 同包，动 domain 前占号）。五单都写：不 worktree add、不另起分支、不再提认领笔、开工先核 status + log 对得上现场那句；报法与时限照原单，`report_task` 收接续单 id。
- 本笔：上段 23 份文件 + 本节，pathspec 提交进 main；推前 `ls-remote` 核 `74ef0da8`。纯 .md / .ps1，不跑 Go 门禁；`owner-review-queue.ps1` 改后跑过一遍能生成、0027 / 0029 / 0055 不再命中。
- **接下来**：五张完工报到后各派非作者评审（20 分钟）→ 无阻断者重放进 main（PS 两票 wbr/01 与 wbr/08 都动 PS，迁移 0020 / 0021 两号已分开，先到先进）；awf/21 进 main 后通道 3 接 18；pgtest/01 若转 needs-info 报数就停。**归用户**（不变）：CI 计费（`idp-xyz` 组织 Actions 付款 / spending limit，仓外动作）；其余 ADR 越权点预审派法（上节「未复核」那段，本节已由前一任直接写了认可行——十篇 ADR + 十二张票，owner 若不认可走 supersede）；`d_tasks.txt` 留仓根还是删。
- **本节五数**：CI main 绿/总 未量（计费未解，run 不起）· 集成时延中位数 —（本节 0 票进 main）· 重放笔数 0 · 重复开发 0 次 · 会话重置 **6 次**（13:0x 通道 1–6 同时中断，含通道 1 自己）

## 2026-09-09 15:0x 通道 1 新会话接续（接手时 `main = origin/main = 8f020eff`；用户先要一次自带 `ask_question`，再「监听队列」→「各通道原来的工作全部中断，如何继续，是否重派」）

### 14:2x–14:4x（本段由 15:0x 的接手会话据现场补记，前一任断在 14:21 之后、这一段它自己没写下）

- 14:21 前一任收 awf/21 完工报（`mcp3-awf21@4de2d30a`，代码 `4f956057`），裁了通道 2 的 wbr/01 相悖报取 **B**（观察代数加第六格 `HandoffObservationChannelUnconfigured` → 未决原因 `CHANNEL_NOT_CONFIGURED`，其余五格不动；ADR-0055 同形；通道 2 14:19 已读），14:22 建了 `%TEMP%\idp-review-awf21`（detached @ `4de2d30a`，未用）。此后无记录。
- 各通道在 14:2x 继续：通道 6 `363544ef`（14:16）/ `60cdca52`（14:23）/ `ff442245`（14:26，awf/24 resolved），**完工报未发出**；通道 4 `920bab06`（14:19，wbr/08 第一层）+ 第二层未提交（mtime 至 14:24，含新文件迁移 0020）；通道 5 `8a81aa28`（14:19，pgtest/01 取证：迁移 93.6%，停下条件不触发）；通道 2 域层落 B（`production_handoff.go` mtime 14:21）+ 端口 / 编排改到一半（`NewSubmitShipmentRequestHandler` 加参，`cmd/parcel-api/assemble_submission.go` 与 PS http 测试两处调用点未跟上，build 红），六份未提交 +637/−13；通道 3 在 `4de2d30a` 把 18 转 in-progress 后一行未写。
- **14:48:57 全部会话中断**——共享树 70 份文件与 `mcp2-wbr01` 树上三份同一秒被重写成 CRLF（内容零差），Docker Desktop 同时掉了；`list_sessions` 对 2–6 显 `running / idle`、lastActivity 停 14:10–14:25，这一格照旧不可信。

### 15:0x 只读盘点 → 点名 → 五张接续单（第二张）+ 一张评审

- `branch-state.ps1` 钉 `8f020eff`：main = origin；五支在途分支全部已推 origin（同 SHA）；**没有一行代码丢失**。共享树 70 个 CRLF ` M` 内容零差（`--ignore-cr-at-eol` 0 件）→ `git checkout --` 复原，状态零行（`d_tasks.txt`、`docs/api/` 两件 untracked 归用户）。Docker Desktop 15:01 拉起、`compose up -d`，`idp-parcel-postgres-gate` healthy。
- 15:00 点名（截止 15:04），15:00:53–15:01:02 **5 个应答全到**（2–6 均空闲 · 无地盘 · 余量充足）。第一轮接续单五张各结 failed（现场 SHA 写进 result）；awf/24 的 `bb38aeda` 据分支结 **done**。
- 15:07–15:08 派：**wbr/01 → 2**（`cf833fc9`，从 `a9566d7b` + 六份未提交，裁决 B 全文写进单里，先补两处调用点、绿了分层提交）· **wbr/08 → 4**（`c608979f`，从 `920bab06` + 第二层未提交 + 迁移 0020，带 DSN 跑 PS postgres + migrations 绿了同一笔提）· **pgtest/01 → 5**（`0864c5d3`，从 `8a81aa28`，做法第 2 步模板库起）· **awf/18 → 3**（`5985bf37`，同分支 `mcp3-awf21@4de2d30a` 往上做，新 `*Named` 直接用 `closedCodeNamed`）· **awf/21 评审 → 6**（`e40acaa0`，非作者，钉 `4de2d30a` 代码 `4f956057` 基线 `1d4a308c`，复用 `idp-review-awf21`，20 分钟）。全部写明「不 worktree add、不另起分支、不再提认领笔、不许 checkout/stash 抓掉现场」。
- **awf/24 评审由通道 1 自己跑**（`Task` 子代理认证失败 → 按 skill 退到串行两遍，检出 `%TEMP%\idp-review-awf24` @ `ff442245`）：Standards 0 阻断 / 1 非阻断（`TaxDisposition.valid` 与 `NewTaxCaliber` default 同一道边界判，作者已留待）；Spec 0 阻断 / 2 非阻断（`CommercialObjectKindNamed` 按地盘留旧循环、交下一个动 `publication_canonicalization.go` 的人；表驱动测试成员表是手写快照）。九集接受判据逐一与旧循环等价核过；20 个 `*Named` 全进表，`-v` 20 子测试 PASS，vet 0。**可重放。**
- **24 重放上链** `%TEMP%\idp-replay-2124`（基 `8f020eff`）：`363544ef→550b89c9`、`60cdca52→8eefc6ac`、`ff442245→e609d8a4`（票面 Status 行与 main 上未认领的原句撞一次，取分支版；认领笔 `cc22b9b0` 未重放，票面终态与分支逐字同）。链上 gofmt 空、build/vet 0、PC 四包 + architecture ok（无 DSN）。**含 DSN 全量与推送等 21 评审到后一并做**——两票一跑，省一次占实例 10 分钟。
- **接下来**：21 评审到 → 无阻断则 `1d4a308c / 4f956057 / 4de2d30a` 重放到链上 → tip 重生成清点 → 广播占实例 → 含 DSN `-p 1 -count=1 ./...` 一次 → `push <sha>:main` → 广播 → 24 / 21 票面补「进 main 记录」+ 评审 Comments，`mcp6-awf24` 改名 `merged/`（`mcp3-awf21` 还在途，18 在同分支）。其余四单完工报到后各派非作者评审。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；14:48:57 那次「整批 CRLF 重写 + 全会话中断 + Docker 掉线」的成因（本机层面，仓内查不出）。
- **本节五数**：CI main 绿/总 未量（计费未解）· 集成时延中位数 —（本节尚 0 票进 main）· 重放笔数 3（未推）· 重复开发 0 次 · 会话重置 **6 次**（14:48 通道 1–6 同时中断，含通道 1 自己）

## 2026-09-09 15:3x 通道 1 新会话接续（接手时 `main = origin/main = 8f020eff`；用户先要一次自带 `ask_question`，再「监听队列」→ 截图 →「各通道原来的工作全部中断，如何继续，是否重派」）

### 15:1x–15:25（本段由 15:3x 的接手会话据现场补记，前一任断在 21 上链之后、这一段它自己没写下；上节末两条「接下来」与「五数」是它 15:16 写的，未提交，本笔一并入库）

- 15:13 通道 6 报 awf/21 评审（`e40acaa0` 结 done，全文在台账 result）：两轴无阻断；Standards 非阻断 2（`policy-rows.test.ts` 夹具头注跨文件计数；`api.ts` 头注逐字列 `ClaimDeadlineKind` 三值）、Spec 非阻断 1（票面「缺口」写 days、HTTP 行体是 `durationDays`）；隔离树复核 tsc 0 / run-tests 177。前一任据此把 `mcp3-awf21` 三笔重放到链上：`1d4a308c→35bddd70`、`4f956057→4dab1d46`、`4de2d30a→e8566de7`，链 tip `e8566de7`（基 `8f020eff`，前三笔是 24）。未做：清点重生成、含 DSN 全量、推。
- 各通道 15:1x 继续：通道 2 落未配置适配器 `adapters/productionhandoff/unconfigured_channel.go`(+_test)（mtime 15:17–15:18）、补 `cmd/parcel-api/assemble_submission.go` 一处调用点（15:18:49），九份未提交；通道 3 抽 `domain/customer_service_rule.go` 帮助函数（+48/−19，15:18:03），一份未提交；通道 4 **15:14 提第二层 `821282dc`**（迁移 0020 + store + `creditSelectorFromRow`）并推，树干净；通道 5 写模板库 `pgtest/template.go`（新）+ `database.go`（36/36，15:19:02），两份未提交。
- **约 15:19 全部会话第三次中断**（各树最后 mtime 15:18:04–15:19:02）。15:25:57–58 共享树同一批 70 份文件再次被重写成 CRLF（内容零差；`git diff --name-only` 只列 `tasks.md`，` M` 全靠 `status` 才看得见）——这个时刻是 Cursor 重开工作区的时刻，不是崩的时刻；与 14:48:57 那次同一批文件、同一形态。

### 15:3x 只读盘点 → 复原 → 点名 → 接续单第三张 + adle/02 → 24 / 21 上链

- `branch-state.ps1` 钉 `8f020eff`：main = origin；五支在途分支全部已推 origin（同 SHA）；**没有一行代码丢失**。70 份 CRLF 幻影 `git checkout --` 复原（取列表用 `status --porcelain`，`diff --name-only` 看不见它们），共享树只剩 `tasks.md`（前一任 18 行）与两件归用户的 untracked。Docker 门禁 healthy（未掉）。
- 旧接续单四张各结 failed（result 写 15:3x 现场：分支 tip、未提交文件 numstat 与 mtime）。15:40 点名（截止 15:44），15:41–15:43 **5 个应答全到**：2 / 4 / 5 / 6 空闲无地盘；通道 3 新会话已自行按 `5985bf37` 接上 awf/18（现场核对一致，`customer_service_rule.go` 是它自己的在途）。
- 15:45–15:48 派：**wbr/01 → 2**（`1eef363a`，从 `a9566d7b` + 九份未提交，第二张 `cf833fc9` 的裁决 B 全文指回去读）· **awf/18 → 3**（`47ef95d8`，只换台账号，三条差异：新 id / 55432 窗口 / 写到可提就提）· **wbr/08 → 4**（`18658c56`，从 `821282dc` 树干净：对判据自列第三层缺什么、验证、完成记录）· **pgtest/01 → 5**（`171e3119`，从 `8a81aa28` + 两份未提交：先自列模板库该钉什么再读 diff，pgtest 自测绿先提一笔）· **adle/02 → 6**（`8fef2107`，`branch-state.ps1 -Classify` 换判据，新树 `mcp6-adle02` 基 `8f020eff`，零交集；预告可能被 wbr/08 评审打断一次）。五单都写「不 worktree add、不另起分支、不再提认领笔、不许 checkout/stash 抓掉现场、**写到可提的点就先提**」。
- 24 / 21 上链续做：链 tip `e8566de7` 上重生成清点 → `56ed4111`（partycommercial 测试文件 126→127、合计 828→829；21 只动 admin-web 不入清点口径）。15:47 广播「窗口开」占 55432，隔离树 `%TEMP%\idp-replay-2124` 上 gofmt 空、build/vet 0、含 DSN `go test -p 1 -count=1 ./...` 一次——结果与推送见下一条。
- **归用户**：CI 计费（不变）；`d_tasks.txt` / `docs/api/` 两件 untracked（不变）；**全会话中断的成因本机层面**，本节多两条取证：(a) 三次中断 13:09 / 14:48 / 15:19 各间隔 100 / 31 分钟，六个通道同秒停，是 Cursor 进程级不是会话级；(b) 70 份 CRLF 重写发生在**重开工作区**那一秒（15:25:57–58），git 不是写入者（`.gitattributes` 对 `*.go/*.md/*.sql` 是 `eol=lf`，git 写盘只会写 LF），且两次都是同一批 70 份——它们更像「Cursor 重开时恢复的编辑器标签」而不是随机文件。可核一件：重开后编辑器里开着的标签是不是就是这 70 份；若是，工作区设置 `"files.eol": "\n"` 能让恢复时不再写成 CRLF。仓内不改（`.vscode/` 归用户）。

### 15:5x–16:0x（本段由 16:1x 的接管会话据现场补记，前一任断在推完 21 / 24 之后、簿记写了没提）

- 15:57 前一任广播「窗口关」：远端 main = `56ed4111`（基 `8f020eff` 七笔：24 三笔 + 21 三笔 + 清点），隔离树含 DSN 全量 100 ok / 0 FAIL / 16 无测试，9.5 分钟；`mcp6-awf24` 拆树改名 `merged/`。票 21 / 24「进 main 记录」+ 评审 Comments 与本文件 15:0x / 15:3x 两节写在共享树上**未提交**（`68d9efa6` 由接管会话原样入库）。
- 15:5x 通道 4 报 wbr/08 完工（`mcp4-wbr08@10e90845`，五笔，含 DSN 22 包 PASS 1990 / 0 / 0）；通道 3 在 awf/18 上写到 16 份未提交（mtime 15:18–15:55）；通道 5 16:04 广播量数窗口开（≤ 3 分钟）后**再无活动、未关窗**；通道 6 15:51 起做 adle/02。
- **16:0x 通道 1 与通道 3 会话 crash**（用户 16:1x 报「1 crash 了」「mcp-3 做到如图 crash 了」）。这次不是全通道同秒停：2 / 4 / 6 活着，5 无活动待查。

## 2026-09-09 16:1x 通道 1 由通道 2 的会话接管（接手时 `main = origin/main = 56ed4111`；用户先要一次自带 `ask_question`，再「监听 mcp-2」→ 通道 2 做 wbr/01 到 16:1x →「1 crash 了……你现在开始接管 mcp-1」）

- 接管前在通道 2 的活（wbr/01 接续 3，`1eef363a`）：三笔已推 `6a80a6da` / `6f52fffc` / `d7f912ce`，树干净，`save_progress` 在通道 2 封现场；接管后在通道 1 收尾——含 DSN（`pg_stat_activity` 核零客户端连接后跑）cmd/parcel-api、cmd/parcel-commercial、cmd/parcel-dispatch、tests/bentocontract、PS postgres + 四个单元包 + architecture：PASS 1567 / 0 / 0，10 包 ok；完成记录 `a5bff461`；`1eef363a` 结 done；09-08 的 PS 批单 `5307fbdc` 收口 done（psr/02 余段与 wbr/01 已完，wbr/02 另派）。
- `branch-state.ps1` 钉 `56ed4111`：main = origin；五支在途全部已推 origin（同 SHA）；**没有一行代码丢失**。通道 3 树核过：16 份未提交（11 改 5 新，全在 PC domain / application / adapters/http），gofmt 空、build/vet 0、PC 四包 ok（无 DSN）。
- 16:1x 点名（截止 16:28）：通道 4 16:2x 应答（空闲，wbr/08 释号）；通道 6 16:3x 应答（空闲，adle/02 完工 `mcp6-adle02@36d9d161`）；通道 5 未应答（16:04 后无活动）；通道 3 crash；通道 2 窗口无人（本会话）。
- 派：**awf/18 接续 4 → 4**（`b061d8a9`，用户 16:2x 指定；换通道，做法全文写进单里——原单在通道 3 台账它读不到；现场 16 份逐份钉 numstat + mtime；先收 Go 半边带 DSN 跑 `cmd/parcel-commercial` 后分层提，再 admin-web）；通道 3 的 `47ef95d8` 结 failed。通道 4 16:2x 开工，估 100–120 分。**wbr/01 评审 → 6**（`54851708`，20 分钟，钉 `a5bff461` 基 `74ef0da8`）。**wbr/08 评审 → 通道 1 隔离子代理**（5 / 6 当时都未应答；检出 `%TEMP%\idp-review-wbr08` @ `10e90845`）。
- **接下来**：两份评审到 → 无阻断者重放进 main（wbr/08 与 wbr/01 无文件重叠，可同链：wbr/08 动 PS adapters/partycommercial + postgres + 迁移 0020 + cmd/parcel-commercial/translate.go；wbr/01 动 PS domain / ports / application / productionhandoff / http 测试 / cmd/parcel-api + 三处夹具 + 两份基线 + ADR）→ tip 重生成清点 → 含 DSN `-p 1 -count=1 ./...` 一次 → `push <sha>:main` → 广播 → 票面「进 main 记录」+ 评审落 Comments → `mcp4-wbr08` / `mcp2-wbr01` 改名 `merged/`。adle/02（纯 .ps1，`mcp6-adle02@36d9d161` 基 `8f020eff`）推送方自审后重放。通道 5 待用户看窗口；pgtest/01（`171e3119`）若 crash 按现场（树干净 @ `fc2b473b`）写接续。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；会话中断成因本机层面。

### 16:2x–16:3x（本段由 16:4x 的接手会话据现场补记，前一任断在派完三单之后、本节上面几条写了没提）

- 通道 4 接 awf/18（`b061d8a9`）后分层收了 Go 半边三笔并推：`e8958fce` domain / `e33aa81e` application + http / `fc51e407` cmd 夹具换 PCC-1（16:29）；接着写 admin-web 半边到四份未提交（`customer-service-rule-form.ts` + test、`publication-draft-api.ts` +35、`presentation.ts` +9，mtime 16:35–16:38:20）。通道 6 为 wbr/01 评审建了 `%TEMP%\idp-review-wbr01`（detached @ `a5bff461`），评审未交。wbr/08 的子代理评审树没建。
- **约 16:38–16:4x 全部会话再次中断**（第五次；用户报「mcp 通道 crash」）。前一任 16:32 已把 15:3x 会话的簿记入库为 `68d9efa6`，**未推**；本节上面 14 行写在共享树上未提交。

## 2026-09-09 16:4x 通道 1 新会话接续（接手时本地 `main = 68d9efa6` 领先 `origin/main = 56ed4111` 一笔纯簿记；用户先要一次自带 `ask_question`，再「监听队列」→「mcp 通道 crash 后各地盘工作树都进不了 main，是不是工作方式有问题；各通道完成情况；如何继续、是否重派」）

- `branch-state.ps1` 钉 `68d9efa6`（16:45）：五支在途分支全部已推 origin（同 SHA）；**没有一行代码丢失**。各树：`mcp2-wbr01@a5bff461` 干净（wbr/01 完工待评审）；`mcp4-wbr08@10e90845` 干净（wbr/08 完工待评审）；`mcp5-pgtest01@fc2b473b` 干净（模板库已提，差再量 / 全仓 / 完成记录）；`mcp6-adle02@36d9d161` 干净（完工，纯 .ps1）；`mcp3-awf21@fc51e407` + 四份 admin-web 未提交。共享树这次**没有 CRLF 幻影**，只有 `tasks.md`（前一任 14 行）与两件归用户的 untracked。Docker 门禁 healthy（Up 2 hours，未掉）。`list_sessions` 对 2–6 显 lastActivity 16:45–16:46——是新会话开听写的心跳，不是在途证据。
- **答用户「工作方式有没有问题」**：保活那半没问题（隔离 worktree + 分支 + 每笔推 origin，五次崩溃零丢失，实测）。进 main 那半有结构性问题：只经一个推送方，而通道 1 是崩得最多的通道（今天七任会话），每任重新定向 20–30 分钟；「评审 → 重放 → 全量 → 推」约 40 分钟的串行流水线没有持久化，崩在中途下一任只能从 tasks.md 找位置，而 tasks.md 常没来得及提交（16:1x 那节即是）。三张完工票等进 main 30–60 分钟，不是活没做完，是推送方一直在重启。解法已写在 parallel-sessions「落 main 的下一形态」（作者自落，PR + CI 把门，通道 1 缩成仲裁簿记），切换条件 CI 连绿三次，**卡在 CI 计费未启（归用户）**。崩溃本身是 Cursor 进程级，仓内治不了。
- 16:50 点名（截止 16:54）：16:51–16:53 **2 / 3 / 4 / 5 应答**（均空闲 · 无地盘 · 余量充足；通道 5 顺带代发了「窗口关」并实测无 55432 连接；通道 2 在帮用户清 `~/.cursor/idp-mcp` 队列目录，不占代码地盘可接票）；**通道 6 截至 16:54 未应答**。
- 旧单结 failed（result 写现场）：`b061d8a9`（awf/18 → 4）、`171e3119`（pgtest/01 → 5）、`54851708`（wbr/01 评审 → 6）。16:57–16:58 派四单：**awf/18 接续 5 → 4**（`e62ba34f`，从 `fc51e407` + 四份未提交，只写差异、指回 `b061d8a9` 读全文；先让四份到可提就提一笔，再组件与页面一签）· **pgtest/01 接续 4 → 5**（`8f187cbb`，从 `fc2b473b` 树干净：先对判据核 `fc2b473b` 那一笔，再量 → 全仓同码 → 残留 → 完成记录；两次占 55432 都先广播、跑完关窗）· **wbr/01 评审 → 3**（`dcb03b95`，改派自 6，全文重写，复用 `idp-review-wbr01`，到 17:2x）· **wbr/08 评审 → 2**（`1a9f4f2c`，新单，钉 `10e90845`，到 17:2x）。四单都写「不碰 55432——通道 5 要量数」。
- 本笔：前一任 14 行 + 本节，pathspec 提交进 main；连同 `68d9efa6` 一起 `push <sha>:main`（两笔都是纯 .md，不跑 Go 门禁；推前 `ls-remote` 核 `56ed4111`）。
- 17:0x 推 `3c9a41bd:main`（`68d9efa6` + 本节簿记，纯 .md），广播派单图。通道 6 16:5x 晚到应答按纪律不算认领，17:08 单独点名（截止 17:12）应答后派 **wbr/02 → 6**（`2a4dfaba`，新树 `mcp6-wbr02` 基 `origin/main = 3c9a41bd`；写明基线文件与 wbr/01 同剪、头注记数钉自己父提交、链 tip 由推送方重数），通道 6 估 35–45 分。通道 4 17:0x 报 awf/18 首笔 `c31f3fda` 已推（四份未提交 + Status 接续句；tsc 0 / run-tests 188），估 60 分；通道 5 17:02–17:05 量数窗口（PS postgres 再量 5.3 s / 5.8 s vs 改前 43.9 s / 44.1 s）、17:08–17:10 全仓窗口（`mcp5-pgtest01@db66ea02` 含 DSN 全量 101 ok / 0 FAIL / 15 无测试，**118 s**，改前 9–10 分）。
- **评审两份到**：wbr/08 ← 通道 2 17:04（Standards 0 阻断 / 2 判断项；Spec 0 阻断 / 2 非阻断，其一是「无 DSN 全 SKIP，推送方复跑」）；wbr/01 ← 通道 3 17:08（Standards 0 / 1；Spec 0 / 3，其一「HTTP 拆出物无票，推送前立票」）。两份全文落各票 Comments。
- **重放链** `%TEMP%\idp-replay-1650`（基 `3c9a41bd`）：adle/02 自审无阻断三笔 `1b39313c→dabe2261` / `18489aeb→83d38658` / `36d9d161→00163b45`；wbr/08 五笔 `fe7c7253→c587a66b` / `920bab06→9661ce3b` / `821282dc→42d84aaa` / `9e509489→678fd8f6` / `10e90845→8dad9960`；wbr/01 五笔 `a9566d7b→ac866a63` / `6a80a6da→1f6572f6` / `6f52fffc→b43066af` / `d7f912ce→ae3c5751` / `a5bff461→8a1c403b`；全部零冲突、内容与分支逐文件零差。评审点名的拆出物立票 **wbr/11**（HTTP 归属视图对 Other 决定欠未决原因 / 续办引用 / 确认引用；`62e6df10`，Blocked by 01）。清点重生成 `62e19b1b`（parcelshipment 生产 142→143 / 测试 141→142、迁移 19→20，合计 154→155；端口声明 367→368）。链 tip gofmt 空、build/vet 0。
- 17:2x 广播占 55432，隔离树含 DSN `go test -p 1 -count=1 ./...` 一次：**101 ok / 0 FAIL / 16 无测试，568 s**（比 `56ed4111` 多一个 ok：新包 `adapters/productionhandoff`）；探针 `TestACreditKeyResolvesAgainstTheClosure` 含 DSN PASS / 无 DSN SKIP。`ls-remote` 核 `3c9a41bd` 未动 → ff → `push 62e19b1b:main`。**17:3x 远端 `main = 62e19b1b`**，广播窗口关 + SHA 对照。三张票面「进 main 记录」+ 评审 Comments 与本节随下一笔簿记入库。
- **接下来**：`mcp4-wbr08` / `mcp2-wbr01` / `mcp6-adle02` 改名 `merged/`、拆树（先比内容再 `worktree remove`，不加 `--force`）；拆 `idp-review-wbr01` / `idp-review-wbr08` / `idp-replay-1650` 三棵临时树。awf/18（通道 4，估 18:0x）、pgtest/01（通道 5，差残留核查与完成记录）、wbr/02（通道 6，估 17:5x）完工报到后各派非作者评审——**pgtest/01 进 main 后全仓从 9.5 分掉到 2 分，之后每票的集成时延主要就剩评审那 20 分**。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；会话中断成因本机层面。
- 17:4x 三张完工报几乎同时到：**pgtest/01**（`mcp5-pgtest01@7a906a86`，六笔；再量 PS postgres 43.9 s → 5.3 s；全仓同码 101 ok / 118 s；残留稳态一个模板库按设计）· **awf/18**（`mcp3-awf21@ed842637`，4de2d30a 后六笔：Go 三 + admin-web 三；tsc 0 / 188 pass；托传「CommercialObjectKindNamed 已改委托 closedCodeNamed」给通道 6）· **wbr/02**（`mcp6-wbr02@58c409a9`，三笔；基线记数钉 e38bc18d 4→3、预写链 tip 由推送方重数）。17:4x–17:5x 直接派三份评审（四通道十分钟内都刚报过状态，不再点名）：**pgtest/01 → 3**（`a4fb6bd8`）· **awf/18 → 6**（`0532630c`）· **wbr/02 → 4**（`80da867c`）。
- 评审回：wbr/02 ← 通道 4 17:38（0 阻断；Standards 1 判断项：旧句作历史引文保留）；pgtest/01 ← 通道 3 17:48（0 阻断；Spec 非阻断 2：`ALLOW_CONNECTIONS false` 加固一行、判据措辞改口；两条合成收尾小票另派作者）。**awf/18 ← 通道 6 约 17:40 crash**，分析做了报告没发（用户截图）——`0532630c` 结 failed，**改派通道 5**（`b0b1b481`，复用 `idp-review-awf18`，加一句「写到一半先发半成品」）。
- **第二条重放链** `%TEMP%\idp-replay-1750`（基 `135af96b`）：wbr/02 三笔 `e38bc18d→4ba719c8` / `eecd05d4→8c38f27c` / `58c409a9→903fda7a`——`eecd05d4` 在 `production_wiring_baseline.txt` 头注处与 wbr/01 那一剪冲突，按意图并（两段头注都留 + 推送方重放注：3→2，PS 组清空；暂存 blob LF 无 BOM，architecture 门禁 ok）；pgtest/01 六笔 `f321b9c9→7a95b701` / `788e226c→5b1e8d44` / `8a81aa28→8c9d8ed1` / `fc2b473b→bb930f89` / `db66ea02→64e95b6b` / `7a906a86→63fe4445` 零冲突零差；清点 `f9bcaa6e`（platform 15→17 / 15→16）。17:4x 广播占 55432，含 DSN 全量一次：**102 ok / 0 FAIL / 15 无测试，120 s**（上一轮 568 s——模板库进 main 的效果当场量到）；探针 PASS / SKIP。`ls-remote` 核 `135af96b` 未动 → ff → `push f9bcaa6e:main`。**17:48 远端 `main = f9bcaa6e`**。
- 17:4x 后：wbr/02 / pgtest/01 票面「进 main 记录」+ 评审 Comments 入库 `506bbfd2`；`mcp6-wbr02` / `mcp5-pgtest01` 改名 `merged/`，五棵树拆净（先比内容），远端旧分支已删。pgtest/01 收尾小票派 **通道 5**（`db4a2e40`：`ALLOW_CONNECTIONS false` 加固一行 + 票面判据措辞改口；排在它的 awf/18 评审之后）。
- **awf/18 评审 ← 通道 5 17:5x**（先发半成品再发全文，照单）：Standards 0 阻断 / 2 非阻断（父行三格门两份、三处过时注释改口属地盘外未列）；Spec 0 阻断 / 3 非阻断（判据「只加一格」口径写窄归伞票、原句「起算事件封闭集」半错、days 留空文案「收到 0」）；补跑 cmd/parcel-commercial 含 DSN PASS 123 / 0 / 0。**第三条链** `%TEMP%\idp-replay-1810`（基 `506bbfd2`）：六笔 `e8958fce→918ef3e0` / `e33aa81e→76710c17` / `fc51e407→6f3c76df` / `c31f3fda→e7a7d784` / `dbd16d10→81285be0` / `ed842637→83c6d81d` 零冲突零差；清点 `c2a965c9`（partycommercial 117→119 / 127→130、http 25→26）。17:5x 广播占 55432，含 DSN 全量一次 **102 ok / 0 FAIL / 15 无测试，117 s**；main 树 ff 后 admin-web tsc 0 / run-tests 188。`push c2a965c9:main`。**17:58 远端 `main = c2a965c9`**。票 18 进 main 记录 + 评审 Comments、伞票 07 子票表 18 / 21 两行转 resolved、22 行阻塞边标已解除——随本笔入库。
- **12:59 派出的五张票至此全部进 main**（wbr/01 · awf/21 + 18 · wbr/08 · pgtest/01 · awf/24），外加 adle/02 与 wbr/02；今天 main 从 `74ef0da8` 走到 `c2a965c9`。
- 18:0x–18:1x：票 18 簿记入库 `158791dc`（18:02）；`mcp3-awf21` 改名 `merged/`、拆树（`worktree remove` 退 255 「Directory not empty」——parallel-sessions 记过的 node_modules 半拆坑，三样核过（链接文件已删、登记已无、指针在）后 `\\?\` 长路径清壳）。**pgtest/01 收尾**（通道 5，`mcp5-pgtest01-tail@2e576218`，一笔：`forbidConnections` + 新测钉 55000 + `dropDatabaseOn` 收三处 + 票面改口）→ 通道 3 十分钟核（`2a2df8ca`，18:0x——评审自报「18:33」与到达时刻不符，0 阻断 / 1 判断题）→ 第四条链 `%TEMP%\idp-replay-1835`（基 `158791dc`）`2e576218→15611900`，清点无变动，含 DSN 全量 102 ok / 0 FAIL / 115 s → `push 15611900:main`。**18:10 远端 `main = 15611900`**。`mcp5-pgtest01-tail` 改名 `merged/`、三棵树拆净、远端分支删；远端只剩 main + 两支 salvage 镜像 + `salvage/mcp4-tf03`。
- **接下来**：在途为零，各通道空闲。前沿可派：awf/22（阻塞边已全解除，admin-web 全页重构，一票独占 `CommercialPoliciesPage.tsx` 与九张表单文件）、awf/23（Blocked by 22）、wbr/09 / wbr/10（SA / PC+SA，ready）、wbr/11（HTTP 归属视图，ready，PS）。下一任派前先点名。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/`；会话中断成因本机层面（本节两例：通道 1 16:3x、通道 6 17:40 单通道）。
- **本节五数**：CI main 绿/总 未量（计费未解）· 集成时延中位数 **25 分**（7 票：wbr/08 15:5x→17:3x ≈ 95、wbr/01 16:25→17:3x ≈ 65、adle/02 16:23→17:3x ≈ 67、pgtest/01 17:33→17:48 ≈ 15、wbr/02 17:33→17:48 ≈ 15、awf/18 17:33→17:58 ≈ 25（含通道 6 评审 crash 重派）、pgtest/01 收尾 18:04→18:10 ≈ 6；前三票被 16:3x 崩溃 + 重启点名 + 评审重派吃掉，后四票是本流水线不中断时的真值——评审与推送方并行、全量 2 分，时延只剩重放 + 全量 + 推那几分钟；本节各条 17:4x–18:1x 时刻先前按估写偏了 20 分左右，18:2x 按 `git log` 作者时刻与台账 createdAt 改正；完工报到达时刻取评审派单的 createdAt 前一分钟）· 重放笔数 **29**（不含清点与立票）· 重复开发 0 次 · 会话重置 **7 次**（16:3x–16:4x 通道 1–6 同时中断 6 次 + 通道 6 17:40 单独 1 次）

## 2026-09-09 19:3x 通道 1 新会话接续（接手时 `main = origin/main = d5a35960`；用户先要一次自带 `ask_question`，再「监听队列」→「继续：已改正簿记时刻（d5a35960），18:22 点名准备派 awf/22 · wbr/09 · wbr/11；wbr/10 等 09/22 进 main 后再派。已经做了一部分，如图」）

### 18:2x–18:5x（本段由 19:3x 的接手会话据通道 1 chat_history、任务台账、各分支 `git log` 作者时刻与树上 mtime 补记；前一任断在点名收到一半，上节「接下来」之后一行没写）

- 18:22 前一任点名（截止 18:26）：18:22–18:23 收 4（空闲）、5（空闲、余量将满，只接小票 / 评审）、3；2 / 6 未到。**通道 1 18:23 后无活动（第八任中断）**。18:27 用户经队列改令「现在只有 mcp-3/4 了，你们来派 awf/22 · wbr/09 · wbr/11；wbr/10 等 09/22 进 main 后再派」。
- 通道 3 18:29 自点名 → 18:31 派 **awf/22 → 4**（`task-7a3944f8`，台账 status 至今 pending——通道 4 没 report working）；自领 **wbr/11**（`mcp3-wbr11` 基 `d5a35960`：`07f99781` 18:34 认领 / `1c10b9b6` 18:37 http 归属视图读决定上的交接评估格 / `52b53ddb` 18:39 parcel-api 装配「其他权威 + 未配置通道」一例 / `a0b3339f` 18:41 票面 resolved + 完成记录；已推 origin 同 SHA；作者含 DSN 跑 PS http + application + parcel-api + architecture 四包 ok，`-v` 计 PASS 98 / 0 / 0；完成记录自报「非作者评审留通道 4 或用户」）；再自领 **wbr/09**（`mcp3-wbr09` 基 `d5a35960` 零提交、未推；树上一份未提交 `internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control_test.go` +62/−19，mtime 18:47:46——做法第 1 步「PS 夹具补 `CreditBasisView` 替身接进三处 `Deps` 构造」写到一半，用户截图左半就是它）。
- 通道 4 awf/22（`mcp4-awf22` 基 `d5a35960`：`21c40771` 18:34 认领 / `a2f0cc2a` 18:46 共享层落地——新文件 `publication-form-shared.ts` +60 与 `PublicationFormFields.tsx` +297、五个 `*-form.ts` 改从共享层导入；已推 origin 同 SHA）；树上三份未提交（`CustomerServiceRulePublicationForm.tsx` +5/−132、`PreAcceptanceFinancialControlPolicyPublicationForm.tsx` +7/−132、`SupplierAgreementPublicationForm.tsx` 0/−118；mtime 18:49–18:51——十张表单组件的私有件裁到共享层，做了三张，用户截图右半就是它）。
- **约 18:5x 全部会话第八次中断**（3 / 4 两树 mtime 停在 18:47 / 18:51；19:3x 重启后 2–6 心跳 19:36–19:37 均为新会话）。wbr/11 完工报没有到过通道 1 的队列（自派流程里报的是通道 3 自己，而那个会话已不在）。

- `branch-state.ps1` 钉 `d5a35960`（19:35）：main = origin；共享树 72 份 ` M` **全是 CRLF 幻影**（`git diff --ignore-cr-at-eol --stat` 为空），两件归用户的 untracked 不变；三支在途 `mcp3-wbr11@a0b3339f`（已推）/ `mcp4-awf22@a2f0cc2a`（已推 + 3 未提交）/ `mcp3-wbr09@d5a35960`（未推、领先 0 + 1 未提交）；两支 salvage/ 镜像。**没有一行已提交代码丢失**；两份未提交现场在各自隔离树上原样在。文件面：wbr/11（PS http 两文件 + parcel-api 测试）、awf/22（admin-web `party/`）、wbr/09（PS `adapters/settlementaccounting` 测试）三者两两不相交，可同链。Docker 门禁 healthy（Up 5 h），55432 零客户端。
- 19:39 点名（截止 19:47）：19:4x **2 / 3 / 4 / 5 / 6 全部应答**（均空闲 · 无地盘 · 余量充足——都是新会话）。19:43–19:44 派三单：**awf/22 接续 2 → 4**（`42893be6`，从 `mcp4-awf22@a2f0cc2a` + 三份未提交接着做，原单 `7a3944f8` 仍在其名下一并收；三份到 tsc 绿先提一笔，余七张每 2–3 张一笔推）· **wbr/09 接续 → 3**（`de74738a`，从 `mcp3-wbr09@d5a35960` + 一份未提交接着做，第一笔就推；迁移与 `migrations.go` / `plan.go` 同笔、改前占号；带 DSN 前广播占 55432）· **wbr/11 评审 → 2**（`68149649`，钉 `a0b3339f`，检出 `%TEMP%\idp-review-wbr11`，到 20:0x；作者是通道 3 上一任，同通道换会话也算同一人故不派 3）。通道 5 / 6 待命，留给 awf/22 与 wbr/09 完工时的两份评审。
- **接下来**：wbr/11 评审到 → 无阻断者在隔离树重放到 `d5a35960` tip（四笔，含票面完成记录）→ 清点重生成 → 广播占 55432（与通道 3 的 SA 带 DSN 错开）含 DSN `-p 1 -count=1 ./...` 一次 → `ls-remote` 核 `d5a35960` 未动 → ff → `push <sha>:main` → 广播 → 票面「进 main 记录」+ 评审 Comments → `mcp3-wbr11` 改名 `merged/`、拆树（先比内容）。awf/22 / wbr/09 完工报到 → 派 5 / 6 评审 → 同法进 main → 之后派 **wbr/10**（PC + SA + admin-web `credit-policy-form.ts`，与 09 / 22 都有文件面交叠，故等它们进 main）与 **awf/23**（Blocked by 22）。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；会话中断成因本机层面（本节一例：18:5x 全通道第八次）。

## 2026-09-09 20:0x 通道 1 新会话接续（接手时 `main = origin/main = 94893c35`；用户先要一次自带 `ask_question`，再「监听队列」）

### 19:4x–19:55（本段由 20:0x 的接手会话据通道 1 队列、任务台账、`git log` 作者时刻与树上 mtime 补记；前一任断在 19:55 改完票 11 Status 一行之后，本段它自己没写下）

- 19:45 通道 4 报 awf/22 接续 2 开工（`42893be6` → working；现场核对一致，估约 2.5 小时：三份接共享层推第一笔 20 分 / 余七张 90 分 / 判据 3 测试 + 完成记录 40 分）。19:48 用户经队列问六个隔离树目录做完是否可删，前一任答：三棵在途（wbr09 / awf22 / wbr11）进 main 后按流程拆；两棵 salvage（`mcp3-awf12` / `mcp5-awf13`）内容已在分支、目录现在就没用、等用户点头；`idp-parcel-wt-h14` 是 08-14 剩的空壳（零文件无 `.git`）已删。
- 19:48:39 **wbr/11 评审 ← 通道 2**（`68149649` 19:49 结 done，钉 `a0b3339f`）：两轴无阻断；Spec 非阻断 1（parcel-api 新例「不建单」只钉响应体、可补 outbox 零信封）、Standards 非阻断 2（两替身 `DecideProductionOwnership` 六处同形；未决分支 `present == false` 不可达）；判断题三道均同意作者。全文 20:2x 落票 11 Comments。
- 19:5x 前一任在 `%TEMP%\idp-replay-1950`（基 `efcdeabc`）重放 wbr/11 四笔 `07f99781→d9958e1f` / `1c10b9b6→411d89f4` / `52b53ddb→f6268470` / `a0b3339f→94893c35`，**19:54 推 `94893c35:main`**；19:55:01 在共享树改票 11 Status 一行（加「19:54 进 main」）后中断。推前有没有跑含 DSN 全量、有没有广播窗口与 SHA 对照，队列与 chat_history 里都没有——按「写证据不写结论」记为无记录，不记为没跑。
- 19:52:10 **wbr/09 估算 ← 通道 3** 到队列，前一任没读到（20:03 由本会话消费）。

### 20:0x 只读盘点 → 回执 wbr/09 估算 → 补 main 真值 → 收 wbr/11 尾

- `branch-state.ps1` 钉 `94893c35`（20:03）：main = origin；在途 `mcp3-wbr09@5bead280`（19:53，领先 1，已推同 SHA）/ `mcp4-awf22@e32cb604`（19:50，领先 3，已推同 SHA，树上 4 行未提交）/ `mcp3-wbr11@a0b3339f`（已进 main，待改名拆树）；两支 salvage/ 镜像。共享树 73 行 ` M`/`??` 里 70 份是 CRLF 幻影、2 件归用户的 untracked、1 份真改动（票 11 Status 一行，mtime 19:55:01，前一任的活，本笔一并入库）。**没有一行代码丢失**。Docker 门禁 healthy（Up 5 h），55432 零客户端。清点在 `94893c35` 干净检出重生成零差。
- 20:08 回执通道 3 的 wbr/09 估算（75–90 分五步；`de74738a` 台账仍 pending——它没 `report_task` working，估算消息即当开工报）：第 1 步已完（`5bead280` 夹具 +62/−19 + 票面，已推）；主干前进到 `94893c35` 与本票零交集不必 rebase；**SA 迁移号 0017 归它**（main 上 SA 到 `0016`，三支在途分支无 SA 0017；`migrations.go` 用 `all:settlement_accounting` 嵌目录、`SettlementAccounting()` 按模块读，加 SQL 不碰两份共享接线文件——它的判断对，19:4x 派单里「迁移与 `migrations.go` / `plan.go` 同笔、改前占号」那句作废）；构造点核对：`NewApplyPreAcceptanceControlHandler` 生产只有 `cmd/parcel-dispatch/assemble.go` 一处（不是 parcel-api），测试 PS 夹具 3 + SA 两份测试 6 + 1，共 11 处；SA application 今天没有任何构造器返 error，全仓唯一先例 `internal/accessidentity/mint.go` 的 `ErrNilDependency`；改口两处：作者不跑全量（只跑票面判据包 + 反向依赖 `cmd/*` 带 DSN + architecture，全量归推送方）、带 DSN 前照旧广播占 55432。
- 20:14–20:18 广播占 55432，在 `idp-replay-1950`（detached @ `94893c35`，干净）补 main 真值：gofmt -l 空、build / vet 0、含 DSN `go test -p 1 -count=1 ./...` **102 ok / 0 FAIL / 15 无测试 / 0 cached，101 s**；探针 `TestFreezeScopesAreInvisibleToEachOther` `-v` PASS。广播窗口关 + 结果。
- wbr/11 收尾：`git diff a0b3339f 94893c35 -- <四文件>` 空、四对 patch-id 相等、全树差只有 main 多的 tasks.md 一份 → `D:/tops/idp-parcel-mcp3-wbr11` `worktree remove`（未加 `--force`，退 0、目录已无）→ `mcp3-wbr11` 改名 `merged/mcp3-wbr11` → 远端 `mcp3-wbr11` 删（远端剩 main + `mcp3-wbr09` / `mcp4-awf22` 两支在途 + `mcp3-awf12` / `mcp5-awf13` / `salvage/mcp4-tf03` 三支 salvage）；`idp-replay-1950` 跑完状态零行后 `worktree remove`。票 11 补「进 main 记录」+ 评审 Comments。
- 本笔：票 11（Status 一行 + 两节）+ 本节，pathspec 提交进 main；纯 .md 不跑 Go 门禁；推前 `ls-remote` 核 `94893c35` 未动、`git log 94893c35..HEAD` 只有本笔。
- **接下来**：wbr/09（通道 3，估 21:1x–21:2x 完工）/ awf/22（通道 4，估 22:1x）完工报到 → 各派非作者评审（5 / 6 空闲）→ 同法进 main → 之后派 **wbr/10**（等 09 / 22 进 main）与 **awf/23**（Blocked by 22）。两棵 salvage 树目录等用户点头再拆。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；会话中断成因本机层面（本节一例：19:55 通道 1 单独）。
- **本节五数**（含上一节没来得及写的 wbr/11 那一票）：CI main 绿/总 未量（计费未解）· 集成时延中位数 **73 分**（1 票：wbr/11 完工 18:41 → 推出 19:54，含 18:5x 全通道中断、19:39 重启点名与评审 20 分）· 重放笔数 4（wbr/11 四笔，前一任 19:5x 重放）· 重复开发 0 次 · 会话重置 **1 次**（通道 1 自己，19:55）

## 2026-09-09 20:2x 通道 2 接管推送方（通道 1 20:2x crash；接手时 `main = origin/main = 4b847a22`；用户先要一次自带 `ask_question`，再「监听 mcp-2」→「广播一次吧，点个名」→「mcp-01 crash，你接管」）

- 20:15 点名（截止 20:20）：20:1x **4 / 5 / 6 应答**（均空闲 · 无地盘 · 余量充足）；**3 截至 20:20 未应答**（在做 wbr/09，树上 mtime 一直在走，正常形态）；通道 1 20:20:30 才读到、按纪律不补应答，20:21 回「协调」一条（推送方不接票；现场 wbr/09 → 3、awf/22 → 4；wbr/10 与 09 / 22 文件面交叠等两票进 main；awf/23 Blocked by 22）——**那是它最后一条消息**，用户 20:2x 报 crash（今天第九任）。20:0x 那节它自己写的「本笔」已入库为 `4b847a22` 并推（`ls-remote` 核同）。
- `branch-state.ps1` 钉 `4b847a22`（20:25）：main = origin；共享树 72 行 ` M`/`??`（上节核过的 CRLF 幻影 + 两件归用户的 untracked，本任没逐份重核）；在途 `mcp3-wbr09@d45a7a34`（20:21，领先 2，已推同 SHA；**树上 8 件未提交 mtime 20:22–20:25**：SA postgres 两文件 + application 两文件 + domain 三文件 + 新文件 `migrations/settlement_accounting/0017_credit_exposure_policy_reference.sql`——第 3 步在做，人活着）/ `mcp4-awf22@e32cb604`（19:50，领先 3，已推同 SHA；**树上 4 件未提交 mtime 19:53:17–19:55:24，到 20:26 已停 31 分**：CreditPolicy / PricePolicy 私有件删尽、`PublicationFormFields.tsx` +24、SettlementPolicy 半张——它的 Field / useLoaded / vocabularyPlaceholder / ReferencePicker 四个私有函数还在）；两支 salvage/ 镜像。**没有一行已提交代码丢失**。
- **awf/22 判孤儿在途**：证据两条——树上 mtime 停在 19:55 而通道 4 20:1x 应答「空闲 · 地盘无」；`list_sessions` 通道 4 online/idle。前一任 20:2x 广播里还按 19:45 的名单写「awf/22 在通道 4」，那份名单已过期（parallel-sessions「派发前先点名」那节的形状）。
- 20:27 广播接管：完工报 / 评审报告 / 占号释号 / 占 55432 一律发通道 2，不发 1。
- 20:30 派 **awf/22 接续 3 → 4**（`5c5a179a`；从 `mcp4-awf22@e32cb604` + 四份未提交接着做——先 tsc 找红、SettlementPolicy 半张收完 4 件一笔推，再 CustomerContract / AuthorizationRule 一笔、AcceptanceRulePackage 连 RowFrame 一笔，再判据 3 的节根 Problems + `claimedPaths ⊆ renderedPaths` 测试、完成记录；ServiceProduct 核一眼本就没有私有件还是已接。main 到 `4b847a22` 对 admin-web 零改动，不必 rebase。报通道 2。估 22:3x）。原单 `42893be6` / `7a3944f8` 仍在通道 4 名下（它 `query_tasks` 读得到），本任未结它们——那两单由通道 1 派，通道 2 的台账看不见。
- **接下来**：wbr/09（通道 3，估 21:1x–21:2x）/ awf/22（通道 4，估 22:3x）完工报到 → 各派非作者评审（5 / 6 空闲待命；同通道换会话也算同一人，awf/22 不派 4、wbr/09 不派 3）→ 无阻断者在隔离树重放到 main tip → 清点重生成 → 广播占 55432、含 DSN `-p 1 -count=1 ./...` 一次 → `ls-remote` 核 tip 未动 → ff → `push <sha>:main` → 广播 SHA 对照 → 票面「进 main 记录」+ 评审 Comments → 分支改名 `merged/`、拆树（先比内容、不加 `--force`）。两票都进 main 后派 **wbr/10**（PC + SA + admin-web `credit-policy-form.ts`）与 **awf/23**（Blocked by 22）。wbr/09 带 DSN 前会广播占 55432，与本任的全量错开。两棵 salvage 树目录等用户点头再拆。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；会话中断成因本机层面（本节一例：通道 1 20:2x）。

## 2026-09-09 21:3x 通道 1 新会话接续（接手时 `main = origin/main = 55528895`；用户先要一次自带 `ask_question`，再「监听队列」→ 四个目录现状 +「awf12 / wbr09 已经 replay 到 main 了，看看当前的情况」）

### 20:4x–21:2x（本段由本会话据任务台账、`%TEMP%` 两棵重放树的 mtime 与 `list_sessions` 补记；前一任是 20:2x 接管推送方的通道 2 会话，中途切到通道 1 接听，21:19 之后没有动静，本段它自己没写下）

- 20:41 派 wbr/09 非作者评审 → 通道 5（task-4f833b6f）；21:02 派 awf/22 非作者评审 → 通道 6（task-2945ad8a）。21:07 通道 6 交 awf/22 评审：**两轴无阻断**（Spec 非阻断 4 / Standards 非阻断 2，全文在台账 result，本会话已落票 22 Comments）。21:09 通道 5 自 20:27 无活动记 failed；21:16 wbr/09 评审改派通道 6（task-66a3fe55，**pending**）。
- 前一任建了两棵重放树：`%TEMP%\idp-replay-2055`（20:42，wbr/09 五笔重放到 `55528895`，tip `84c62c3c`，清点重生成零差）与 `%TEMP%\idp-replay-awf22`（awf/22 八笔重放到 `55528895`，tip `5f9a33a7`，21:19 跑过 run-tests）——两条链互相独立，都没 ff、没推。21:19 之后无动静，`list_sessions` 通道 2 最后活动 20:38。
- 通道 3 wbr/09 完工：`512ada47` 20:37（完工报有没有进过队列无记录，它也没 `report_task`，接续单 de74738a 至今 pending）；通道 4 awf/22 完工：`0c005ed5` 20:56（作者随后 crash，完工报没发出，由用户截图 + origin 分支确认——task-2945ad8a context 原话）。

### 21:3x 只读盘点 → 21:4x 接回推送方 → 两次 ff → 拆四棵树

- 用户 21:2x 经队列：两棵 salvage 树（`mcp3-awf12` / `mcp5-awf13`）已拆、指针保留；「awf12 / wbr09 已经 replay 到 main」；「看看当前的情况」。量到与之不符的一格：**main = origin = `55528895`，wbr/09 / awf/22 都没进 main**（branch-state 两支在途；SA 0017 与 `PublicationFormFields.tsx` main 树上无；main reflog `55528895` 之后无 ff）——replay 停在 %TEMP% 两棵树里。salvage 两棵：目录已无、指针 `salvage/mcp3-awf12` / `salvage/mcp4-awf12` @ `febfcd2e`、`salvage/mcp5-awf13` @ `feaf5bbb`、远端 `mcp3-awf12` / `mcp5-awf13` 同 SHA；awf/12、awf/13 票面在 main 上 resolved（经 `mcp2-awf12` / `mcp6-awf13` 进的，`git cherry` 剩的 `+` 是接手时改写过的 SHA）。**零丢失**。共享树 72 行 ` M`/`??`：`git diff --ignore-cr-at-eol --stat` 0 行，全为 CRLF 幻影 + 两件归用户的 untracked。本机无 `go test` 进程、55432 零客户端、门禁容器 healthy。
- 21:3x 问通道 2 是否在推送流程里；它回：新会话、不持推送方角色、没碰两棵重放树、在回话前不动 main。21:4x 广播「通道 1 接回推送方 · 占 55432」。
- **wbr/09 进 main**：`84c62c3c` 对 `512ada47` 十六文件 diff 空、五对 `patch-id --stable` 相等；清点在 `84c62c3c` 干净检出重生成零差；gofmt 空 / `go build` `go vet` 0 / 含 DSN `go test -p 1 -count=1 ./...` **102 ok / 0 FAIL / 15 无测试 / 0 cached，105 s**；探针 `TestCreditExposureLedgerRoundTripsSeparately` 带 DSN PASS / 不带 SKIP；共享树幻影文件与本票文件面零交集（ff 不必归一任何工作副本）；`ls-remote` 核 `55528895` 未动、`55528895..84c62c3c` 只有五笔 → ff 21:43 → `push 84c62c3c:main`。分支→main SHA 对照见票 09「进 main 记录」。
- **awf/22 进 main**：`idp-replay-awf22` 上 `rebase --onto 84c62c3c 55528895` 零冲突（与 wbr/09 文件面零交集）→ `4209520b`；八对 patch-id 相等、二十四文件 diff 空；`84c62c3c..4209520b` 的 `.go`/`.sql` 零变动（Go 真值沿用上一跑）；admin-web `tsc -b --force` 退 0（`--listFiles` 计 165 份 `src/` 含两件共享层——先前不带 `--force` 2 秒退 0，怕是增量缓存才多量了这一眼）、`node scripts/run-tests.mjs` 195 / 195；清点零差；`ls-remote` 核 `84c62c3c` 未动、`84c62c3c..4209520b` 只有八笔 → ff 21:45 → `push 4209520b:main`。**远端 main = `4209520b`**。21:5x 广播两票 SHA 对照 + 释 55432。
- **评审门如实记**：awf/22 有通道 6 无阻断评审在手（台账 result）；wbr/09 台账里只有「5 未响应 → 改派 6 pending」，本会话队列里没有它的评审原文——推送方据用户口述「5 / 6 评审已过」推了，wbr/09 按**合入后补评**处理（21:5x 已致通道 6：单 66a3fe55 仍有效，检出 `512ada47` 或 `84c62c3c` 皆可，有阻断在 main 上往前修不回滚）。这是本节唯一一处对「无阻断才重放」的让步，票 09 与此处各写一遍。**21:56 补评到**：通道 6 钉 main `84c62c3c` 两轴无阻断、main 不动（Spec 非阻断 1：票面第 3 步「两格」与「同形」自相矛盾，代码取一列对；Standards 非阻断 3：`IsZero()` / `Deps` 头注后半是变更史 / `CreditStanding.Policy()` 生产零调用——都落 SA 地盘，随 wbr/10 顺手收），`task-66a3fe55` done；全文与推送方处置落票 09 Comments。通道 5 21:4x 自证从未评过这两票（前一任会话 20:27 应答后在 `check_messages` 上挂死，未消费到派单），现为 21:27 新会话、空闲。让步已补平。
- 收尾：票 09 / 22 各补「进 main 记录」（22 同时落通道 6 评审 Comments；09 落判断题 ④ 的推送方裁定：不加单独装配探针）；伞票 07 子票表 22 行转 resolved、23 行阻塞边解除；票 23 `Blocked by` 标已进 main。四棵树（两棵 D: 在途 + 两棵 %TEMP% 重放）：本票文件对 `origin/main` diff 空、`status --untracked-files=all` 零行；两棵 awf22 树的 `apps/admin-web/node_modules` 是指向共享树的 **junction**，先 `cmd /c rmdir` 只摘链接再 `worktree remove`（未加 `--force`；四棵退 0、目录与 `.git` 链接文件均无、共享树 `node_modules` 完好——不先摘链接就 remove 会顺着 junction 删共享树的依赖，这一格 parallel-sessions「带 node_modules 的工作树」那节没写，本节先记着）；`mcp3-wbr09` → `merged/mcp3-wbr09`、`mcp4-awf22` → `merged/mcp4-awf22`；远端两支删。branch-state 钉 `4209520b`：**在途（无）**，远端只剩 main + 三支 salvage。
- 本笔：票 09 / 22 / 07 / 23 + 本节，pathspec 提交进 main；纯 .md 不跑 Go 门禁；推前 `ls-remote` 核 `4209520b` 未动、`git log 4209520b..HEAD` 只有本笔。
- 22:0x 用户经队列「点头」→ **22:09 点名**（截止 22:12）：**5 个应答**（2 / 3 / 4 / 5 / 6 均空闲 · 地盘无 · 余量充足，全部截止前到）。派单第 0 步：`branch-state -Path` 四块地盘（SA / PC / admin-web party / PC 迁移）在途分支无、无未提交现场——两票都**另起**；ADR-0129 号空（main 到 0128）、PC 迁移号 0029（main 到 0028）。**22:11 派 wbr/10 → 通道 3**（`task-519de030`；分支 `mcp3-wbr10` 基 `90c025ca`；七步各一笔：`/domain-modeling` 定封闭集 + ADR-0129 / PC 领域 `CreditRatioBase` + 迁移 0029 / 发布路 + 词表 / SA 折算 + 顺手收 09 的三条非阻断 / admin-web 一格 / seed / 清点；占号点名 `publication_draft_payload.go`、`publication-draft-api.ts`、ADR README；估 2.5–3 小时）。**22:13 派 awf/23 → 通道 4**（`task-23fabd10`；分支 `mcp4-awf23` 基 `90c025ca`；六步：载荷镜像壳引用 / 相对方选单换业务参与方册 / Go 载荷层一道问题路径 / 读面 / seed / 票面；不碰 wbr/10 的两份公共文件；`cmd/parcel-commercial` 带 DSN；估 1.5–2 小时）。两票文件面：wbr/10 的 `publication_draft_payload.go` 与 awf/23 的 `publication_draft_payload_settlement_policy.go` 同目录不同文件，共用夹具可能碰——两单都写了占号。2 / 5 / 6 留作非作者评审（09 / 22 的评审都是 6 出的，下一轮先派 2 / 5）。通道 3 的 wbr/09 接续单 `de74738a` 推送方代结 done（作者会话未自报）。
- 22:1x 通道 3 / 4 开工报到（wbr/10 估 2.5–3 h；awf/23 估 23:5x，它点出第 3 步要在 `publication_draft_payload.go` 的 `Publication` 里加一行、resolution-key seed 的 `settlement.counterparty` 要随相对方同步改）。22:2x 致 3 / 4 三条约定：共用文件各落自己的 hunk、后到的由推送方 rebase、非平凡冲突退回作者重验。22:3x 两边占号广播都到（4 占 `publication_draft_payload.go` 只加一行；3 占 ADR README 加 0129 一行）。
- 22:2x 用户问「2 / 5 / 6 现在空闲无需干活对吗」→ 答：是，ready 队列只剩刚派的 10 / 23，其余开票全 blocked / draft / needs-info，2 / 5 / 6 留作评审；若授权代裁可派四张 draft 的 `/domain-modeling`。伞票 07 表 24 行漏转 resolved 顺手补（`91df9aaa`）。用户「做这个吧」→ **22:30 点名 2 / 5 / 6**（截止 22:33，三应答全到）→ 第 0 步 `-Path internal/transportfulfillment` / `docs/domain` / `docs/adr`：只有 wbr/10 / awf/23 两支在途且不碰 TF、不碰这几份 CONTEXT——四票都另起。**ADR 号预留**：0130 tf/12、0131 tf/13、0132 sa/04、0133 tf/14（用不上就空着）。**22:36 派 tf/12 → 通道 2**（`task-ddb77473`，分支 `mcp2-tf12` 基 `91df9aaa`）、**22:38 派 tf/13 → 通道 5**（`task-8f6d8f94`，`mcp5-tf13`）、**22:40 派 sa/04 → 通道 6**（`task-103327c7`，`mcp6-sa04`）——三张都是「只裁不码」：答票面「要裁的」、落 ADR + CONTEXT 词条、票面 draft → ready-for-agent 并写出做法 / 完成判据；各估 60–90 分。**tf/14 未派**：它第 2 问「对象怎么走到合同」要用 tf/12 对「PS 按什么键答」的裁决，等 tf/12 完工报到再派给先空出来的通道。共享文件占号点名：PS CONTEXT（2 与 6 各加一节）、ADR README（3 / 2 / 5 / 6 各加一行）。
- **接下来**：等 wbr/10（估 00:4x–01:1x）/ awf/23（估 23:5x）完工报 → 各派非作者评审 → 同法进 main（两票若同时到，先 awf/23 后 wbr/10，wbr/10 重放要在 awf/23 之后核 `publication_draft_payload*` 夹具是否合得上）；tf/12 / 13 / sa/04 完工报到（估 23:4x–00:1x）→ 纯 .md 推送方自审重放 → tf/14 派出。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；会话中断成因本机层面（本节：前一任推送方会话 21:19 后无动静、通道 2 新会话自报无记忆）。
- **本节五数**：CI main 绿/总 未量（计费未解）· 集成时延中位数 **58 分**（2 票：wbr/09 `512ada47` 20:37 → 21:43 推出 66 分、awf/22 `0c005ed5` 20:56 → 21:45 推出 50 分；两票完工报都没进过本会话队列，起点按分支 tip 作者时刻）· 重放笔数 **13**（wbr/09 五 + awf/22 八，无清点笔）· 重复开发 0 次 · 会话重置 **2 次**（通道 1 本会话、通道 2 自报新会话）

## 2026-09-09 22:47 通道 4 窗口接听通道 1 队列并代推送方（通道 1 22:4x crash；接手时 `main = origin/main = 8cc79b3d`；用户先经通道 4 队列送「请调用 idp-mcp-1 的 check_messages」一次——本窗口按开场绑定拒了、答「通道 1 是推送方队列不代消费」，再送「mcp-1 crash，请调用 idp-mcp-1 的 check_messages」才接）

- **本窗口是 awf/23 的作者会话。** 接手时刻 awf/23 已交活（完工报 22:3x 发通道 1）、通道 1 已重放（`%TEMP%\idp-replay-awf23` @ `f8fa0398`，基 `8cc79b3d`）并在 22:4x 广播占 55432 跑含 DSN 全量，随后崩；那一跑的结果与它派的隔离子代理评审都随会话丢失（`%TEMP%\idp-review-awf23` @ `10851199` 干净、无产物）。接手取证（git，不是记忆）：`ls-remote` main = `8cc79b3d`；本机无 `go` / `*.test` 进程（22:47），55432 的占号按无进程视为释出并广播；八棵树：共享树、两棵 %TEMP%、五棵 D: 在途（`mcp2-tf12@c14511a4` / `mcp3-wbr10@b9f5a2b1` / `mcp4-awf23@10851199` / `mcp5-tf13@91df9aaa` / `mcp6-sa04@cf8a3c4a`）。
- **awf/23 进 main**：六对 `patch-id --stable` 逐对相等（`a9d7ef7b→5d504401` / `e906e012→125e9fd3` / `7cd71479→f2d65463` / `08af0cf1→00ed2fce` / `9216dfff→295c66a2` / `10851199→f8fa0398`）；`idp-replay-awf23` 钉 `f8fa0398` 重跑含 DSN `go test -p 1 -count=1 ./...` **exit 0 · 103 s · 102 ok / 0 FAIL / 15 无测试 / 0 cached**，探针 `TestFreezeScopesAreInvisibleToEachOther` 带 DSN PASS；22:5x 广播占 / 释 55432 各一次。**推送方与作者同会话，自己不评**：22:5x 派非作者评审 → 通道 2（`task-08584691`，tf/12 刚交、唯一空闲非作者；5 / 6 在代裁票上）；22:5x 通道 2 交：**两轴无阻断**（Standards 非阻断 3：最重是 `reconcileShellReference` 空白口径 `== ""` 与 `requireField` 的 `TrimSpace` 不一致、纯空格壳引用会被报两次——随下一张碰本册文件的票一行收；Spec 非阻断 5：最重是 seed 摘要评审未能独立复算）→ 推送方按它的要求在 tip 上跑临时探针（未提交、已删）`ReconcileDeclaredDigest` 答 nil。全文与处置落票 23「进 main 记录」+ Comments；伞票 07 子票表 23 行转 resolved。
- **tf/12 进 main**（通道 2 代裁完工报 22:4x 到、随会话丢，22:5x 重发一份；`task-ddb77473` done）：三笔纯 .md 重放到 `f8fa0398` 之上，`89d2326f→b2feccd0` / `290280d9→3fb5e30c` / `c14511a4→9efb33d1`，三对 patch-id 相等，零冲突；推送方自审：`git diff --check` 唯一命中是 PS CONTEXT 新词条标题行尾两空格——量过该文件 38 条粗体词条标题行 38 条都以两空格收尾，是硬换行约定不是杂误；ADR-0130 + README 一行 + GLOSSARY 一条 + PS CONTEXT 词条与 Rules 各一条 + 票 12 面 + 新票 ps-port-remainder/06 + spec 一行。
- **sa/04 进 main**（通道 6 代裁完工报 22:50 到；`task-103327c7` done）：三笔纯 .md 重放到 `9efb33d1` 之上，`d85711ea→6995afe7` / `cf8a3c4a→79d1662b` / `543a9ac1→f53c1a06`；**`docs/adr/README.md` 撞一次同点插入**（0130 与 0132 都插在 0128 行后）——按号序 0130 前 0132 后解，只删三行冲突标记，逐文件 numstat 与原笔相同（README 1+/0-、CONTEXT 5+/1-），所以中间那对 patch-id 不等是冲突上下文所致，不是内容差；PS CONTEXT 两票各动不同节自动合上。tip 上 README 0128 → 0130 → 0132 各一行。清点在 tip 干净检出重生成零差（三票都不动 .go/.sql，Go 真值沿用 `f8fa0398` 那一跑）。
- 本笔：票 23（Status + 进 main 记录 + Comments）+ 伞票 07 表 23 行 + 本节，pathspec 提交在 `f53c1a06` 之上；纯 .md 不跑 Go 门禁；推前 `ls-remote` 核 `8cc79b3d` 未动、`git log 8cc79b3d..HEAD` 只有本任重放的 12 笔 + 本笔；共享树 72 行幻影与本链 18 份改动文件零交集，ff 不必归一任何工作副本。**远端 main = 本笔 tip**（SHA 在推后的广播里；本节写在推之前，写不出自己的 SHA）。
- **接下来**（本任只代推送方，不改派单纪律）：wbr/10（通道 3，估 00:4x–01:1x；它 22:5x 广播要占 `cmd/parcel-dispatch/assemble.go` 一行与 PS `settlementaccounting` 适配器测试夹具——地盘外两处，完工报里要有理由）、tf/13（通道 5，估 23:4x–00:1x）完工报到 → 各派非作者评审（wbr/10 不派 3、tf/13 纯 .md 自审）→ 同法进 main；**tf/14 按前一任「等 tf/12 完工报到再派给先空出来的通道」现在可派**，通道 2 空闲——归下一任或本任按用户是否继续要本窗口代推送方而定；三棵已合树（`mcp2-tf12` / `mcp4-awf23` / `mcp6-sa04`）先比内容再拆、指针改名 `merged/`，两棵 %TEMP%（`idp-replay-awf23` / `idp-review-awf23`）同拆——`mcp4-awf23` 树的 `apps/admin-web/node_modules` 是 junction，先 `cmd /c rmdir` 再 remove。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；会话中断成因本机层面（本节：通道 1 22:4x，今天第十任）。
- **本节五数**：CI main 绿/总 未量（计费未解）· 集成时延中位数 **约 25 分**（3 票：awf/23 完工报 22:3x → 推出 23:0x 约 30 分、tf/12 22:4x → 23:0x 约 20 分、sa/04 22:50 → 23:0x 约 15 分；含通道 1 崩掉的那 10 余分钟）· 重放笔数 **12**（awf/23 六 + tf/12 三 + sa/04 三，无清点笔）· 重复开发 0 次 · 会话重置 **1 次**（通道 1 22:4x）

### 23:0x–23:2x 续（同一窗口）：拆树、派 tf/14、通道 3 / 5 又崩——wbr/10 接续单、tf/13 封存进 main

- 23:03 推 `dec37d78` 后：三棵已合树（`mcp2-tf12` / `mcp4-awf23` / `mcp6-sa04`）先比内容（本票文件对 `origin/main` 只有 main 多出的行：票 23 进 main 记录、README / PS CONTEXT 里兄弟票的行）、`status --untracked-files=all` 零行，`mcp4-awf23` 树先 `cmd /c rmdir` 摘 junction（共享树 `node_modules` 完好）再 remove；两棵 %TEMP%（`idp-replay-awf23` / `idp-review-awf23`）同拆——replay 树 remove 退 255、`.git` 链接已删、登记已无、剩半棵树，按 workflow.md 三格核过后以 `\\?\` 长路径前缀清壳。三支指针改名 `merged/`、远端三支删。**23:0x 派 tf/14 → 通道 2**（`task-f5521768`，`mcp2-tf14` 基 `dec37d78`，ADR-0133；前一任「等 tf/12 完工报到再派给先空出来的通道」，2 刚交 tf/12）；23:1x 开工报到，估 00:2x。
- **通道 3 崩**（用户 23:1x 报；`list_sessions` offline）：wbr/10 五笔全在 `origin/mcp3-wbr10@f733374b`、树干净、mtime 23:07 后无动静——零丢失；剩第 6–7 步（seed 核对 / 最后一轮 cmd/* 带 DSN / 清点核零差 / 票面 resolved + 完成记录 / 完工报）。**23:1x 派接续单 3 → 6**（`task-431cc4a0`；6 刚交 sa/04、`list_sessions` running/idle、地盘无——据 git 与刚收的完工报判，没再点名；只加笔不改它的五笔），23:1x 开工报到，估 23:50–00:00。评审届时不派 3 / 6。
- **通道 5 崩**（用户 23:1x 报；offline）：tf/13 分支 `mcp5-tf13` 零提交、远端无分支，**四件产出全在未提交现场**（ADR-0131 / NR CONTEXT 一条 / NR 窄读口新票 nr-route-evidence-views/03 / 票 13 面，mtime 22:41–22:43）——按「未提交现场」那节封存：`chore(salvage)` 一字不改入库 `mcp5-tf13@f73c481b` 并推远端，提交信写证据（mtime、offline、用户报）不写结论。现场读下来是完整的（ADR Status Accepted 五条越权风险点收尾、票 13 裁决 / 做法 / 完成判据 / Comments 齐、NR 小票 ready-for-agent），差两样：作者 Comments 写「GLOSSARY 加一行」而现场无 GLOSSARY 改动；README 0131 行（作者原计划单独一笔占号）没写。无空闲通道可派接续（2 在 tf/14、6 在 wbr/10 接续），**推送方按纯 .md 代裁自审重放**：`f73c481b→029352a0`（patch-id 相等，零冲突），自己另起一笔补 README 0131 行（按号序插在 0130 与 0132 之间）与票 13 Comments 一条如实记（GLOSSARY 未落归 nr-route-evidence-views/03 或下一位碰 GLOSSARY 的 NR 裁决顺手补）；`git diff --check` 空、四件 + README 内 .md 相对链接 158 条逐一解析存在。未作语义评审，票 13 与 ADR-0131 正文以作者会话写下的为准。
- 本笔：README 一行 + 票 13 Comments 一条 + 本段，pathspec 提交在 `029352a0` 之上；纯 .md 不跑 Go 门禁；推前 `ls-remote` 核 `dec37d78` 未动。**远端 main = 本笔 tip**（SHA 在推后的广播里）。
- **接下来**：wbr/10 接续（6，估 00:00）完工报到 → 派非作者评审（2 若已交 tf/14 就派 2；否则推送方隔离子代理）→ 重放（`publication_draft_payload.go` 与 awf/23 那一行不相邻、README 0129 行按号序插在 0128 后、PC CONTEXT 两句不同行；它随迁移重生成过清点，重放后在 tip 干净检出再生成一次兑底）→ 含 DSN 全量 → ff 推；tf/14（2，估 00:2x）纯 .md 自审重放。`mcp3-wbr10` 树归通道 6 在用；`mcp5-tf13` 树本笔进 main 后拆（比内容再拆、指针改名 `merged/`）。**归用户**：CI 计费；两件 untracked；会话中断成因本机层面——本节里 1 / 3 / 5 三个通道各崩一次，今天第十一、十二任。
- **本节五数（续）**：会话重置累计 **3 次**（通道 1 22:4x、通道 3 23:1x、通道 5 23:1x）；tf/13 集成时延不可量（作者没发出完工报，起点按现场 mtime 22:43 → 推出 23:2x 约 40 分）；重放笔数累计 **13**（+tf/13 一笔封存）；其余同上。

## 2026-09-10 00:0x 通道 2 窗口接听通道 1 队列并代推送方（通道 1 会话 00:0x 再崩；接手时 `main = origin/main = 4cfb725a`；用户经通道 2 队列送「mcp-1 crash，请调用 idp-mcp-1 的 check_messages」——本窗口是 tf/12 / tf/14 的作者会话、awf/23 的非作者评审会话）

- **接手取证**（git，不是记忆；00:0x）：`ls-remote` main = `4cfb725a`（前一任 23:2x 推的 tf/13 封存 + README 0131 行 + tasks.md 节；本窗口 00:0x 那条接听广播里写的「远端 main = dec37d78」是旧值，以本行为准）；远端在途 `mcp2-tf14@a6e013cf`（本会话，纯 .md，完工报 00:0x 已进通道 1 队列、`task-f5521768` done）、`mcp3-wbr10@a2e02e52`（本机树 `D:/tops/idp-parcel-mcp3-wbr10` 已到 `87e3cfc0` = 票面 resolved + 完成记录，比远端多、干净——通道 6 接续的最后几笔还没推或完工报还没到）；`mcp5-tf13` 远端已无（前一任封存进 main 后删）；另有 `mcp3-awf12` / `mcp5-awf13` / `salvage/mcp4-tf03` 三支老指针在远端，`origin/main..` 各自的关系未量，不在本任范围。开工前排一次通道 1 队列：空；通道 4 窗口对「若仍在接听通道 1 队列请喉一声」无应答，按不再接听处理。
- **tf/14 进 main**（作者与推送方同会话；纯 .md 按前两任「代裁自审重放」先例办，未作第二人语义评审，票 14 Comments 如实记）：三笔在 `%TEMP%\idp-replay-tf14` 重放到 `4cfb725a` 之上零冲突，`829e5999→70a7c669` / `3b941f4b→2db719f2` / `a6e013cf→25102207`；后两对 `patch-id --stable` 相等，第一对不等的成因是 README 0131 行已在 0132 之前、hunk 上下文变了——`git diff a6e013cf 25102207 -- docs/adr/README.md` 只多 main 的 0131 那一行，`--stat` 全树差只有 main 自己多出的六份文件（tf/13 四件 + README 一行 + tasks.md）。tip 上 README 0128 → 0130 → 0131 → 0132 → 0133；`git diff --check` 唯一命中是 PC CONTEXT 新词条标题行尾两空格（该文件每条粗体词条同样的硬换行约定）；五份新改文件内 .md 相对链接逐一解析存在。不动 .go/.sql，Go 真值沿用 `f8fa0398` 那一跑。
- 本笔：票 14 Comments 一条（SHA 对照）+ 本节，pathspec 提交在 `25102207` 之上；纯 .md 不跑 Go 门禁；推前 `ls-remote` 再核 `4cfb725a` 未动。**远端 main = 本笔 tip**（SHA 在推后的广播里）。
- **接下来**：wbr/10 接续（通道 6）完工报到 → 非作者评审归本窗口（2 不是 wbr/10 的作者；推送方评他人的活不违「不自评」）→ 重放（`publication_draft_payload.go` 与 awf/23 那一行不相邻、README 0129 行按号序插在 0128 后、PC CONTEXT 信用政策句与本任「交付条件」词条不同节；它随迁移重生成过清点，重放后在 tip 干净检出再生成一次兑底）→ 含 DSN 全量（占 55432 广播）→ ff 推。`mcp2-tf14` 树本笔进 main 后拆（比内容再拆、指针改名 `merged/`、远端删）；`%TEMP%\idp-replay-tf14` 同拆。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；会话中断成因本机层面——本节：通道 1 00:0x，今天第十三任。
- **本节五数**：CI main 绿/总 未量 · 集成时延 tf/14 完工报 00:0x → 推出 00:1x **约 15 分** · 重放笔数 **3**（无清点笔）· 重复开发 0 次 · 会话重置 **1 次**（通道 1 00:0x）

## 2026-09-10 10:3x 通道 1 新会话接续（接手时 `main = origin/main = 160a1def`；用户先要一次自带 `ask_question`，再「监听队列」→「这个全部完成了，可以回放了吧：D:\tops\idp-parcel-mcp3-wbr10」）

- **接手取证**（git，不是记忆；10:4x）：`ls-remote` main = `160a1def` = 本地 main；共享树 72 行全是 CRLF 幻影（`git diff --ignore-cr-at-eol --stat` 空）+ `d_tasks.txt` / `docs/api/` 两件 untracked 归用户；队列空。两棵树：`D:/tops/idp-parcel-mcp3-wbr10` @ `87e3cfc0` = `origin/mcp3-wbr10`，干净（票面 resolved + 完成记录，通道 6 完工报 23:26 已在台账 task-431cc4a0 done）；**`%TEMP%\idp-replay-wbr10` @ `26e64d1c` detached，干净——前一任已把八笔 cherry-pick 到 `160a1def` 之上（committer 时刻 09-09 23:36–23:38），没 ff、没推、没清点笔、票面无评审记录**，之后会话中断。上一节标「00:0x」的几件事在 git 里是 23:21–23:29（`25102207` / `160a1def` committer 时刻），以 git 为准。
- **重放核对**（不重做，核过再用）：48 件文件面与 `90c025ca..87e3cfc0` 一致；`git diff 87e3cfc0 26e64d1c` 全树只 3 件 +10/−0，全是 main 自己多出的（README 0131 / 0133 行、PC CONTEXT「交付条件」词条、`publication_draft_payload.go` awf/23 那一行——两侧都改、hunk 不相邻、自动合并干净）；README 0127→0133 号序对；`patch-id --stable` 七对相等、`b9f5a2b1→ca597f57` 那对不等只因上下文。
- **评审**：10:45 点名（截止 10:49），10:46 前 5 应答（2/3/4/5/6 全「空闲 · 地盘无 · 余量充足」，`list_sessions` 五个 running 的 lastActivity 都停在昨夜 23:2x–23:4x，点名答得出、它答不出）；派**通道 5**（task-fecd651a，非作者；3/6 是作者），10:49 交：两轴无阻断，Standards 2 非阻断（`priorPeriodConfirmedChargesBase` 同刻决胜靠 `statement_number` 字串序；ADR-0129 Consequences 漏列 pending carrier 存量）、Spec 1 非阻断（票外顺手收 wbr/09 三条，已登记）。全文与推送方处置落票 10 Comments。
- **验证**（隔离树钉 `26e64d1c`）：`gofmt -l` 零、`go build` / `go vet` 全仓 0；机制清点在 tip 重生成零差（无需清点笔）；占 55432 广播后 **含 DSN** `go test -p 1 -count=1 -v ./...`：102 ok / 0 FAIL，`--- PASS` 7839 / `--- SKIP` 1（仅 `pgtest.TestHelperTemplateOwnerProcess`，设计上只由子进程驱动）/ `--- FAIL` 0，122 s（与 09-09 20:1x 那跑 101 s 同量级），探针 `TestFreezeScopesAreInvisibleToEachOther` PASS，释号广播；admin-web 在隔离树 junction 主树 `node_modules`：`tsc -b --force` 0 错、`run-tests` 198/198，`.tmp-test` / `tsbuildinfo` / junction 随手清（`rmdir` 只删链接，主树 `node_modules` 完好）。日志 `%TEMP%\wbr10-replay-full.log`，仓内无残留。
- **ff + push**（10:5x）：共享树无一件与 48 件相交的脏文件，`merge --ff-only` `160a1def`→`26e64d1c`；`git push origin 26e64d1c:main`，`ls-remote` 核 = `26e64d1c`；广播「远端 main = 26e64d1c」带八对 SHA 对照。pre-push 钩子未装、`landing.lock` 不在（切换条件未到，仍走推送方直推）。
- **拆树**：`mcp3-wbr10` 对 main 48 件只差 main 自己多出的加行 → `git branch -m` 成 `merged/mcp3-wbr10`（指针 `87e3cfc0` 留着）；远端 `mcp3-wbr10` 删（远端还剩 `mcp3-awf12` / `mcp5-awf13` / `salvage/mcp4-tf03`，前两支仍「等用户点头再拆」）；`D:/tops/idp-parcel-mcp3-wbr10`（先 `rmdir` junction、清两件 ignored 产物，`status --ignored --untracked-files=all` 空）与 `%TEMP%\idp-replay-wbr10` 两棵 `git worktree remove` 均未加 `--force`，`.git` 链接 / `worktree list` / 分支指针三样各查过。
- 本笔：票 10 Comments 两条（评审全文 + 进 main 记录）+ 本节，pathspec 提交在 `26e64d1c` 之上；纯 .md 不跑 Go 门禁、自审；推前 `ls-remote` 再核 `26e64d1c` 未动。**远端 main = 本笔 tip**（SHA 在推后的广播里）。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；会话中断成因本机层面——本节：通道 1 今晨新会话，前一任（通道 2 窗口代推送方）重放到 23:38 后中断。
- **CI 取证**（11:0x，`gh run list -b main` + check-run annotations）：main 最近四跑 `4cfb725a` / `160a1def` / `26e64d1c` / `f80befa6` 全 failure，**每个 job 2–3 s 即失败、零 step、无 runner**，annotation 原话「The job was not started because recent account payments have failed or your spending limit needs to be increased」——不是代码红，是 GitHub 账单 / 消费上限没起 runner；main 真值今天只来自本机隔离树那一跑。「落 main 的下一形态」切换条件（`main` 连绿三次）在账单修好之前量不出来。归用户。
- **用户 11:0x「接下来做什么」→ 11:1x「你是开发管理专家，你自决」**。前沿算自票面（`Status:` 行扫全部 `.scratch/*/issues`）：ready-for-agent 且 `Blocked by: 无` 的五张——psr/06、psr/07（建议排 06 后）、nr/03、pc-gaps/11、sa/04，全是昨晚四个裁决票（ADR-0130–0133）拆出的落地票；tf/12 / 13 / 14 是它们的第二波，tf/09 父票随之收；`wiring-baseline-remainder` 十一张全 resolved；awf 子票 08–18 全 resolved → **伞票 awf/07 本笔转 resolved**（本笔改它的 Status 行 + Comments 一条）。第 0 步：`branch-state.ps1 -Path` 对 internal/{parcelshipment,networkrouting,partycommercial,transportfulfillment} 各查一遍，在途分支无、半成品无。
- **派单记录**（基 `84e89dc7`）：11:13 点名（截止 11:16），11:14 前 5 个应答（2/3/4/5/6 全空闲）。派：通道 2 → psr/06（task-0dab9f5a；完工后不等进 main，同链 `mcp2-psr07` 接 07）；通道 5 → nr/03（task-a8cbfc83）；通道 4 → pc-gaps/11（task-8359632c；PC 迁移 0030；交付方式封闭集 vs 开放引用那一处「先裁」归作者，拿不准取开放引用）；通道 6 → sa/04（task-276950ce；PS 迁移 0021，取号前广播）；通道 3 留评审位（send_to_session 说明 + 预读 ADR-0130–0133 与 tf/12），不派实施票。派法理由：2 / 5 / 6 各是对应裁决的代裁人，上下文最近；PS 地盘 2 与 6 同期在途，派单里点名了共享点（ports.go / adapters/postgres 既有文件 / *_baseline.txt / cmd/parcel-api 装配改前占号，新文件避同名，0021 取号先广播先得）。每票进 main 前非作者评审照 09-08 规矩，推送方重放。
- **本节五数**：CI main 绿/总 **0/2**（两跑都没起，成因见上一条）· 集成时延 wbr/10 完工报 23:26 → 推出 10:5x **约 690 分**（1 票，跨夜：重放 23:38 做好后会话中断、今晨用户 10:3x 催起才接；不剔除）· 重放笔数 **8**（无清点笔）· 重复开发 0 次 · 会话重置 **1 次**（通道 1 今晨）

## 2026-09-10 11:3x 通道 1 新会话接续（接手时 `main = origin/main = 6a387b89`；用户先要一次自带 `ask_question`，再「监听队列」→「你原来派发的任务，后来全部 crash 了，你看看情况，继续开始工作吧」，附六窗截图）

- **接手取证**（git，不是记忆；11:39）：`ls-remote` main = `6a387b89` = 本地 main（上一节那笔 .md 已推）；共享树 72 行 ` M` 仍全是 CRLF 幻影（`git diff --ignore-cr-at-eol --stat` 空）+ `d_tasks.txt` / `docs/api/` 两件 untracked 归用户。四棵派单树都在、分支都在，**四个作者会话在 11:18–11:23 之间同时停笔**（各树未提交文件 mtime 的上界），与用户「全部 crash」相符：
    - `mcp2-psr06@1d7affa5`（= origin，一笔票面 in-progress）+ **四份未跟踪** `internal/parcelshipment/domain/delivery_place_{reference,resolution}{,_test}.go`（mtime 11:18–11:22）；我在该树量：`go build ./...` 0、`go vet` PS domain 0、`go test -count=1` PS domain **ok**——绿的中间态。
    - `mcp4-pcgaps11@d9a1f571`（= origin，一笔票面 in-progress）干净，零半成品。
    - `mcp5-nr03@6edb83b6`（= origin，两笔：票面 + 第 1 步 `PlannedLegReference` / `LegAt`）+ 未提交 `ports/planned_leg_window_read.go`、`adapters/postgres/planned_leg_window{,_test}.go`、`production_wiring_baseline.txt` 加一段（`ParsePlannedLegReference` 登记，写明票 13 接线那天剪）；`go build` / `go vet ./internal/networkrouting/...` 0、domain 测试 ok，postgres 那份要 DSN 未跑。
    - `mcp6-sa04@84e89dc7`（零笔、**未推 origin**）+ 未提交 red：`authorized_disposition_test.go`（新，28 KB）与 `operator_registration_wait_test.go` 两行改口（`ResumeByAuthorizedDisposition` 进四路互异表 + 上界探针）；`go vet` PS domain 红在 `undefined: ResumeByAuthorizedDisposition`——第 1 步 red 写完、实现未起。
- 台账（`query_tasks`）：pc-gaps/11 `task-8359632c` **working**（11:3x 刚报，通道 4 已自己接上）；psr/06 / nr/03 / sa/04 三单 pending；另两单陈旧 working（tf/13 代裁 `task-8f6d8f94`、wbr/10 `task-519de030`）实际早已进 main，本节代收 done 并写明「作者会话已崩未报、通道 1 代收」。
- **点名**（11:42 广播 2–6，截止 11:46；不开 `activeOnly`）。11:46 前全到，五种答法：通道 4 以占号广播代答——**已自行续做 pc-gaps/11**、占 PC 迁移 0030、地盘同派单；通道 5 新会话、无在途记忆，**已按 git 现场接 nr/03**（从 `6edb83b6` + 未提交现场，不另起），并已重占 55432 把 `./internal/networkrouting/... ./internal/architecture/... ./cmd/...` 带 DSN 跑绿、释号；通道 6 新会话，**已按 git 现场接 sa/04**（`report_task` working，估第 1 步 60–90 分、全票 4–6 h），PS 迁移 0021 仍归 6、点名 psr/06 若要迁移取 0022；通道 3 空闲 · 地盘无 · 余量充足；**通道 2 空闲、崩后新会话、对 psr/06 无在途记忆**——四张里只有这一张没人接。
- **续派**：11:4x **psr/06 → 通道 2**（`task-422c077d`，原单 `task-0dab9f5a` 报 failed 并写明「本单作废不是工作失败」）。派单里写死「从 `mcp2-psr06@1d7affa5` + 树上四份未提交接着做，不另起」、四份的 mtime 与我量的绿、接手纪律（先写自己第一片 red 再读对方四份，对判据不对断言条数；核过即成笔并推）、迁移取 0022（0021 归 6）、同期在途 sa/04 的共享点、07 同链接做。通道 3 仍留评审位（送了预读清单：ADR-0130–0133 + 四票面 + parallel-sessions「合入前独立评审」）；通道 5 补发台账单号 `task-a8cbfc83`（前任没报 working）。
- **这次崩掉的教训一条，落在派单措辞里不另开文档**：四个作者在 11:14–11:23 之间各写了 5–9 分钟就崩，只有通道 5 在崩前提了一笔（第 1 步）并推了，其余三个的活全在未跟踪文件里靠盘活着——「写到可提的点就先提」在 5–10 分钟一崩的节奏下要更细：**第一片 red 绿了就是可提的点**。续派单里点了名。
- 本笔：本节 + 上述台账动作；pathspec 提交在 `6a387b89` 之上；纯 .md 不跑 Go 门禁、自审；推前 `ls-remote` 再核 `6a387b89` 未动。**归用户**（不变）：CI 计费；`d_tasks.txt` / `docs/api/` 两件 untracked；会话中断成因本机层面——本节：通道 1–6 六个会话 11:2x 同时中断，今天第二次全员重置。
- **接下来**：nr/03（5）最先到完工报 → 派通道 3 评审 → 重放 → 含 DSN 全量（占 55432 广播）→ ff 推；psr/06（2）/ pc-gaps/11（4）/ sa/04（6）完工报到后同法；psr/06 进 main 后 tf/12 可派（通道 3 若仍空闲接它）；sa/04 与 psr/06 若重叠 .go，后进的一方重验。

### 11:5x–12:3x 同一会话：nr/03 完工 → 评审改道 → 进 main；通道 4 / 3 各再崩一次

- **nr/03 完工报 11:5x**（通道 5，tip 先 `f32311ea`、追补清点笔 `90c0595c`、票面补表 `6e4e1f9d`，基 `84e89dc7`，代码 tip `4ace04d6`；带 DSN NR + architecture + cmd/* 19 包 ok 探针 PASS）。11:5x 派通道 3 评审（task-f8baa675，截止 12:20）；同时 `%TEMP%\idp-replay-nr03` 在 `c59892e3` 上 cherry-pick 六笔无冲突（`c3f711df→15df570d` / `6edb83b6→ca02dbef` / `4ace04d6→7ca08d29` / `f32311ea→e76d0245` / `90c0595c→f78fc500` / `6e4e1f9d→85d238ce`），清点在 `f78fc500` 重生成零差，gofmt / build / vet 0，占 55432 广播后带 DSN 全量 **102 ok / 0 FAIL / 15 无测试 · PASS 7848 / SKIP 1 / FAIL 0 / 0 cached · 118 s**，探针 PASS，释号。
- **通道 4 二次 crash**（用户 12:1x 报，附截图：崩前仍在读 ADR-0120 先例）：树 `mcp4-pcgaps11@d9a1f571` 干净、无半成品；原单 task-8359632c 报 failed（作废非失败）；**pc-gaps/11 改派通道 5**（task-4649070b，分支 `mcp5-pcgaps11` 基 `d9a1f571`，PC 0030 随转；5 12:1x 报 working，估 90–120 分，走开放引用）；`mcp4-pcgaps11` 树拆（干净、未加 --force）、指针改 `salvage/mcp4-pcgaps11`（d9a1f571 已是 mcp5-pcgaps11 的祖先，进 main 走后者）。
- **通道 3 crash**（用户 12:2x 报，附截图）：评审在 Step 2/3 中途，隔离检出 `%TEMP%\idp-review-nr03@90c0595c` 已建、gofmt / vet / domain 测试 / architecture 门禁跑过，两轴未交。2 / 5 / 6 各在票上，无空闲非作者通道 → **推送方自跑 /code-review**（09-08 规矩「没有空闲通道时推送方自己用 /code-review 的隔离子代理跑」）；子代理 `Task` 报 `Authentication error`，两轴改由本窗口串行跑、互不引用（skill「Otherwise run them serially」）。结论 **Standards 0 阻断 / 1 非阻断（基线加行未记数）· Spec 0 阻断 / 1 非阻断（`planByVersion` 多行报错分支无用例，且 `plan_version` 无唯一约束、可达）**，全文落票 03 Comments。今天第三、四次会话重置。
- **进 main**：推送方簿记一笔在 `85d238ce` 之上（票 03 Comments 两条 + 基线 NR 段重放注「在 85d238ce 上实测 3」+ 本节）；`./internal/architecture/...` 重跑一遍（动了基线注释行）；`ls-remote` 核 `c59892e3` 未动 → ff → `push <sha>:main`；SHA 见推后广播。拆树：`mcp5-nr03` 比内容后改 `merged/`、远端删；`idp-replay-nr03` / `idp-review-nr03` 两棵 %TEMP% 树拆（未加 --force）。
- **接下来**：psr/06（2，已推两笔 eed326c5 / 2dc0dd0d，第三笔 adapters/postgres 在写）/ sa/04（6，第 1 步 e4883039 已推、第 2 步在写）/ pc-gaps/11（5）完工报到后各派非作者评审（通道 3 / 4 若重起接评审位；都不在则推送方自跑）→ 重放 → 全量 → ff 推。psr/06 与 nr/03 都动 `production_wiring_baseline.txt`（不同段 + 各自计数注），重放时后进的重数。**归用户**：CI 计费；两件 untracked；会话中断成因——本节又两次（4、3），今天累计 8 次。
- **本节五数**：CI main 绿/总 未量（账单未修，跑不起）· 集成时延 nr/03 完工报 11:5x → 推出 12:3x **约 40 分**（1 票；含评审改道一次）· 重放笔数 **6**（含作者清点笔 1）· 重复开发 0 次 · 会话重置 **8 次**（11:2x 通道 1–6 各一 + 12:1x 通道 4 + 12:2x 通道 3）

### 12:3x–12:5x 同一会话：psr/06 完工 → 推送方自评 → 进 main

- **psr/06 完工报 12:3x**（通道 2，task-422c077d done：tip `c2a119c6` 基 `84e89dc7`，代码 tip `906f2f0f`；七笔；PS adapters/postgres PASS 151、domain + architecture + cmd/* PASS 1079，SKIP 0；`DPR-1:<租户>/<委托>/<资料组>[;<包裹>]/<baseline|version;<版本>>`，原词 `DELIVERY_PLACE`）。3 / 4 仍未重起（`list_sessions` lastActivity 停在崩前），5 / 6 在票上 → 推送方自跑 /code-review（非作者），两轴串行：**Standards 0 阻断 / 2 非阻断**（基线计数在 tip 重数为 4、重放注补；「Consequences 第 N 条」按序位指别文件无标签列表项，与行号同构，随 tf/12 或另起小笔）· **Spec 0 阻断 / 1 非阻断**（判据 2 字面「ports 替身实现」以 domain 四格 + 真库四格作答，认可不补）。四道判断题（替身落点 / `SourceDataVersionAnchor` 分型 / 歧义上抛 / Parse 进基线）全部认可，理由落票 06 Comments。
- **重放**：`%TEMP%\idp-replay-psr06` 在 `346a6532` 上 cherry-pick 六笔（`1d7affa5→53469892` / `eed326c5→43919344` / `2dc0dd0d→70745894` / `5a30093d→ad1529aa` / `906f2f0f→60576319` / `c2a119c6→fcabde8e`）；作者清点笔 `42bf6570` 与 nr/03 清点同文件冲突——生成物「谁的笔在后谁的数字盖前面」——**跳过**，推送方在 tip 重生成为 `b734f8f5`。本票十件与分支逐字相同。gofmt / build / vet 0；基线在 tip 实测 4。占 55432 广播后带 DSN 全量 **102 ok / 0 FAIL / 15 无测试 · PASS 7897 / SKIP 1 / FAIL 0 / 0 cached · 120 s**，探针 PASS，释号。
- **进 main**：推送方簿记一笔在 `b734f8f5` 之上（票 06 Comments 两条 + 基线 PS 段重放注「在 b734f8f5 上实测 4」+ 本节）；`./internal/architecture/...` 重跑；`ls-remote` 核 `346a6532` 未动 → ff → `push <sha>:main`，SHA 见推后广播。`mcp2-psr06` 树与指针**暂留**：通道 2 要在 `c2a119c6` 上开 `mcp2-psr07`，等它确认开出后再改 `merged/`、拆树、删远端；`idp-replay-psr06` 拆。
- **接下来**：psr/07（2，同链基 `c2a119c6`，推送方按序重放——其父链六笔已在 main 换了 SHA，重放时只 pick 07 自己那几笔）/ sa/04（6，第 3 步应用层，占了 `cmd/parcel-dispatch/assemble.go` 一段 + `NewAdvanceFinancialControlJudgmentHandler` 加参签名变更，只在隔离树）/ pc-gaps/11（5）。psr/06 进 main 后 **tf/12 可派**：3 / 4 任一重起即派它（原计划通道 3 接）；都不起则等 2 交 07。**归用户**：CI 计费；两件 untracked；会话中断成因。
- **本节五数**：CI main 绿/总 未量 · 集成时延 psr/06 完工报 12:3x → 推出 12:5x **约 20 分**（1 票）· 重放笔数 **7**（六笔作者 + 推送方清点 1；作者清点笔跳过不计）· 重复开发 0 次 · 会话重置 0 次（本小节内）

### 12:5x–13:2x 同一会话：psr/06 树收尾 → psr/07 完工 → 自评 → 进 main；tf/12 派通道 2

- **psr/06 收尾补记**：上一小节写「`mcp2-psr06` 暂留」，12:5x 查 `git worktree list` 时通道 2 已在 `c2a119c6` 上开出 `mcp2-psr07`（`merge-base --is-ancestor` 核过），于是当场收：本票非簿记文件对 main 逐字相同 → 树拆（干净、未加 --force）、指针改 `merged/mcp2-psr06`、远端删。
- **psr/07 完工报 13:0x**（通道 2，send_to_session：分支 `mcp2-psr07` 基 `c2a119c6` 同链，tip `6043923a`，四笔 + 清点笔；带 DSN PS domain + postgres + architecture + cmd/* PASS 1246 / SKIP 0）。3 / 4 仍未重起、5 / 6 在票上、2 是作者 → 推送方自跑 /code-review：**Standards 0 阻断 / 1 非阻断**（「Consequences 第一条」序位引用，同 06 那条不重开）· **Spec 0 阻断 / 1 非阻断**（判据 1「集运单元引用 → 没有」在 `DeclaredParcelID` 键的口上不可表达，归 tf/14 适配器分流——写进 tf/14 派单）。四道判断题（替身落点 / 抽 helper 改 06 文件 / 内部测试钉不可达格 / `(id, bool, error)` 形）全部认可，全文落票 07 Comments。
- **重放**：`%TEMP%\idp-replay-psr07` 在 `8bdab82e` 上只 pick 07 自己的四笔（`5018962b→21342b50` / `18b628cf→35fde705` / `1e8e2bc0→ee497195` / `6043923a→1767a9d5`），全干净；作者清点笔 `526f0cdf` 与 main 同文件冲突→跳过，tip 重生成为 `637bb3a2`。gofmt / build / vet 0、architecture ok；占 55432 广播后带 DSN 全量 **102 ok / 0 FAIL / 15 无测试 · PASS 7910 / SKIP 1 / FAIL 0 / 0 cached · 121 s**，探针 PASS，释号。
- **tf/12 派通道 2**（13:0x，`task-b5823d32`，基 `8bdab82e`）：psr/06 进 main 即解阻塞；`branch-state -Path internal/transportfulfillment` 无在途无半成品→另起；13 / 14 都未开工，执行器生产入口归本票；`待复核`落法默认甲；点名了通道 6 此刻正占 cmd/parcel-api 几份接线文件，动前先排队列再占号。原计划「通道 3 接 tf/12」因 3 崩改为 2（代裁人，上下文最近）。
- **进 main**：推送方簿记一笔在 `637bb3a2` 之上（票 07 Comments 两条 + 本节）；`ls-remote` 核 `8bdab82e` 未动 → ff → `push <sha>:main`，SHA 见推后广播。`mcp2-psr07` 比内容后改 `merged/`、远端删、树拆（2 已转 tf/12 用新树 `mcp2-tf12`）；`idp-replay-psr07` 拆。
- **接下来**：sa/04（6，第 3 步 HTTP 半边在跑）/ pc-gaps/11（5，首笔 582e9f12 已推）/ tf/12（2）。tf/13（NR 缝）现在也可派——nr/03 已在 main；3 / 4 任一重起给它，否则等 2 交 tf/12 后接。tf/14 Blocked by pc-gaps/11（psr/07 半边今起已解）。**归用户**：CI 计费；两件 untracked；会话中断成因。
- **本节五数**：CI main 绿/总 未量 · 集成时延 psr/07 完工报 13:0x → 推出 13:2x **约 15 分**（1 票）· 重放笔数 **5**（四笔作者 + 推送方清点 1）· 重复开发 0 次 · 会话重置 0 次（本小节内）

### 13:0x–13:5x 同一会话：pc-gaps/11 完工 → tf/13 派通道 5 → 自评 → 进 main

- **tf/12 第 5 步答通道 2**（13:4x）：取甲（端点 `/transport-fulfillment-delivery-dispatch-triggers` + `UnconfiguredIntake{}`，照 dispatch-task-registrations 先例；拍频归调用方即「作配置不作默认」）；TF adapters/http 新增一件端点文件准（地盘外一件，完成记录记一句）；乙要新开 TF 工作面读口 + postgres，越地盘另票。
- **pc-gaps/11 完工报 13:0x**（通道 5，task-4649070b done：分支 `mcp5-pcgaps11` 基 `d9a1f571`，tip `3e513732`，六笔含清点；带 DSN 23 包 ok / PASS 2201 / SKIP 0；含迁移 0030）。**tf/13 派通道 5**（13:3x，`task-247c5d4d`，基 `c0cdebba`；nr/03 已在 main 解阻塞；TF 地盘 branch-state 无在途——但 tf/12 同期在 2 手上，派单写死顺序：先做不碰共享文件的适配器新包 + 测试，再问 2 之后才动 ports/delivery_requirement.go / 执行器 / cmd/parcel-api，三步法 + 开窗；生产入口归 tf/12）。
- **pc-gaps/11 评审**（推送方自跑，非作者；3 / 4 未回、2 / 6 在票上）：**Standards 0 阻断 / 1 非阻断**（`SaveDeliveryConditions` 以 error 表达业务拒件——ADR-0031 三格外第四种落点，作者判断题 1，归 owner 复核）· **Spec 0 阻断 / 1 非阻断**（产品版本未采用不单列一格 vs ADR-0133 决定二字面，作者判断题 5，归 owner 复核）。开放引用裁法认可（CONTEXT / GLOSSARY 未动）。全文落票 11 Comments。
- **重放**：`%TEMP%\idp-replay-pc11` 在 `c0cdebba` 上 pick 六笔（`d9a1f571→d656b99b` / `582e9f12→545b2483` / `d7bd4132→e0edc701` / `697c9dc5→1ad8d81f` / `7f18b156→08084300` / `3e513732→92a059dc`）——第一遍漏了基底 `d9a1f571` 致票面笔冲突，重置重来后全干净（教训：改派票的基底笔也是这张票的一笔，pick 时从分支的 merge-base 起数，别从「作者的第一笔代码」起数）；作者清点笔 `aff5537d` 跳过，tip 重生成为 `fce1faa9`。gofmt / build / vet 0、architecture ok、0030 LF 无 BOM；占 55432 广播后带 DSN 全量 **102 ok / 0 FAIL / 15 无测试 · PASS 7939 / SKIP 1 / FAIL 0 / 0 cached · 122 s**，探针 PASS，释号。
- **进 main**：推送方簿记一笔在 `fce1faa9` 之上（票 11 Comments 两条 + 本节）；`ls-remote` 核 `c0cdebba` 未动 → ff → `push <sha>:main`，SHA 见推后广播。`mcp5-pcgaps11` 比内容后改 `merged/`、远端删、树拆；远端 `mcp4-pcgaps11`（d9a1f571，已是 main 祖先）同删；`idp-replay-pc11` 拆。
- **接下来**：sa/04（6，第 5 步 HTTP 读面）/ tf/12（2）/ tf/13（5）。**tf/14 两个 Blocked by 至此都在 main**（pc-gaps/11 + psr/07）——3 / 4 任一重起给它；都不起则等 2 或 5 交票后接（tf/14 与 12 / 13 共用执行器与 cmd/parcel-api，派前要看 12 / 13 进到哪）。owner 复核队列新增两条（ADR-0031 第四落点、ADR-0133 决定二产品未采用那一格），`scripts/owner-review-queue.ps1` 下次生成会带出。**归用户**：CI 计费；两件 untracked；会话中断成因。
- **本节五数**：CI main 绿/总 未量 · 集成时延 pc-gaps/11 完工报 13:0x → 推出 13:5x **约 45 分**（1 票；评审这张最大、约 20 分）· 重放笔数 **7**（六笔作者 + 推送方清点 1）· 重复开发 0 次 · 会话重置 0 次（本小节内）

### 14:3x–15:0x 同一会话：sa/04 完工 → 自评 → 进 main（今晨派出的四张落地票全部收口）

- **sa/04 完工报 14:3x**（通道 6，send_to_session + task-276950ce：分支 `mcp6-sa04` 基 `84e89dc7`，tip `3df82d62`，九笔 + 票面；带 DSN 34 包 ok / PASS 2280 / SKIP 0；含迁移 0021；余量将满）。3 / 4 仍未重起、2 / 5 在票上 → 推送方自跑 /code-review：**Standards 0 阻断 / 1 非阻断**（0021 受限项两列 CHECK「同在或同缺」vs 票面字面，判断题 7，认可记一句）· **Spec 0 阻断 / 1 非阻断**（处置授权翻译适配器未建、只建未配置授权器，判断题 4，认可；PC 落词那天同笔换）。十二道判断题全部认可；硬句「人工处理不得绕过硬规则」由 `TestADispositionRecordNeverPassesTheFinancialControlGroup` 钉住。全文落票 04「进 main 记录」节。
- **重放**：`%TEMP%\idp-replay-sa04` 在 `62c87e73` 上 pick 九笔全干净（`e4883039→6a5535e2` / `8c05797e→3c1f54bf` / `ca2c44e3→0ce6461e` / `f686e4bd→a974e6b3` / `33fbfb6d→b995f683` / `1aa5b6e3→1e589f66` / `5f34f67b→f1644897` / `f66af0d0→1f45af10` / `3df82d62→779f841e`）；作者清点笔 `99ddc8f5` 跳过，tip 重生成为 `fb38e66a`。gofmt / build / vet 0、architecture ok、0021 LF 无 BOM；占 55432 广播后带 DSN 全量 **102 ok / 0 FAIL / 15 无测试 · PASS 8011 / SKIP 1 / FAIL 0 / 0 cached · 125 s**，探针 PASS，释号。
- **进 main**：推送方簿记一笔在 `fb38e66a` 之上（票 04「进 main 记录」+ 本节）；`ls-remote` 核 `62c87e73` 未动 → ff → `push <sha>:main`，SHA 见推后广播。`mcp6-sa04` 比内容后改 `merged/`、远端删、树拆；`idp-replay-sa04` 拆。
- **今晨四张落地票全部进 main**：nr/03（346a6532）→ psr/06（8bdab82e）→ psr/07（c0cdebba）→ pc-gaps/11（62c87e73）→ sa/04（本笔）。五次评审全由推送方自跑（3 / 4 崩后未重起、其余通道各在票上），十条非阻断里四条归 owner 复核（ADR-0031 第四落点、ADR-0133 决定二产品未采用那一格、「Consequences 第 N 条」序位引用两处）。
- **接下来**：tf/12（2，cmd/parcel-api 接线中，sa/04 进 main 后要对新 main 干跑）/ tf/13（5，ports 三步法中）；两票完工后 tf/14 可派（3 / 4 若重起先给它）；nr/03 补测试小笔（5，tf/13 后）。**归用户**：CI 计费；两件 untracked；会话中断成因（今天 8 次）。
- **本节五数**：CI main 绿/总 未量 · 集成时延 sa/04 完工报 14:3x → 推出 15:0x **约 30 分**（1 票）· 重放笔数 **10**（九笔作者 + 推送方清点 1）· 重复开发 0 次 · 会话重置 0 次（本小节内）

### 15:0x–15:2x 同一会话：tf/12 完工 → 通道 5 评审（今天第一次非推送方评审）→ 进 main

- **tf/12 完工报 15:0x**（通道 2，task-b5823d32 done：分支 `mcp2-tf12` 基 `8bdab82e`，tip `9cd329b4`，七笔含清点；带 DSN TF 全树 + parcel-api + parcel-dispatch + architecture PASS 1707 / SKIP 0；生产装配停点 DELIVERY_PLACE_SOURCE_NOT_WIRED → DELIVERY_WINDOW_SOURCE_NOT_WIRED；一处纪律失手自报：shell 残留 DSN 碰 55432 约 7 s 未占号，只慢不错）。**评审派通道 5**（task-58bb1010，截止 15:35）——它的 tf/13 下一步本就等 tf/12 进 main，这 20 分不白等；15:1x 交：**Standards 0 阻断 / 2 非阻断**（CU 经 `NewDeclaredParcelID` 铸成包裹身份去问 PS，类型上一句假话但今天无种类维、ADR-0130 决定二归 PS 答，可接受；未定 / 没有两分支同返回，分开为让甲可见）· **Spec 0 阻断 / 2 非阻断**（判据 2「续办引用非空」作者改断为「MISSING 不留续办」——评审判作者对；待复核取甲归 owner）。推送方处置：判据 2 那半句以进 main 记录改口；「对象种类维」与 psr/07 评审那条合成一句写进 tf/14 派单。全文落票 12 Comments。
- **重放**：`%TEMP%\idp-replay-tf12` 在 `cc2c5f6f` 上 pick 六笔全干净（`972d4867→5ce50558` / `a06f8145→bae1820a` / `12ce0ed3→5c3ce7cf` / `2b22c9b2→1f95f72a` / `c33ac8bd→2e1dbe25` / `9cd329b4→6428eaf9`）；cmd/parcel-api 四份接线文件与 sa/04 的加行不相邻、自动合上，build / vet / architecture 0——parallel-sessions「只改自己那几行不动邻行」这条今天在两票同文件上兑现了一次；作者清点笔 `e6f10315` 跳过，tip 重生成为 `c0ef50d3`。占 55432 广播后带 DSN 全量 **103 ok / 0 FAIL / 15 无测试 · PASS 8035 / SKIP 1 / FAIL 0 / 0 cached · 122 s**，探针 PASS，释号。
- **进 main**：推送方簿记一笔在 `c0ef50d3` 之上（票 12 Comments 三条 + 本节）；`ls-remote` 核 `cc2c5f6f` 未动 → ff → `push <sha>:main`，SHA 见推后广播。`mcp2-tf12` 比内容后改 `merged/`、远端删、树拆；`idp-replay-tf12` 拆。
- **接下来**：tf/13（5，rebase 到新 main 后填 Windows 一格 + cmd/parcel-api 接线 + 装配测试停点改 DELIVERY_CONDITION_SOURCE_NOT_WIRED）；通道 2 空闲 → **派 tf/14**（两个 Blocked by 都在 main；2 是 ADR-0133 代裁人；派单写清：对象种类维今天没有、集运单元照 tf/12 经提供方答没有；`pcpostgres.NewDeliveryConditions(db)` 装配；Deps.Conditions 那一格；与 tf/13 在 cmd/parcel-api 同文件、按 13 → 14 顺序落或各自加行不相邻）。通道 6 待命不派（余量将满）。**归用户**：CI 计费；两件 untracked；会话中断成因。
- **本节五数**：CI main 绿/总 未量 · 集成时延 tf/12 完工报 15:0x → 推出 15:2x **约 20 分**（1 票；含非作者评审 15 分）· 重放笔数 **7**（六笔作者 + 推送方清点 1）· 重复开发 0 次 · 会话重置 0 次（本小节内）

### 15:2x–16:0x 同一会话：tf/14 派通道 2 → tf/13 完工 → 自评 → 进 main

- **tf/14 派通道 2**（15:2x，`task-1d5f0b15`，基 `0c9b846f`）：两个 Blocked by（psr/07 + pc-gaps/11）都在 main；生产入口归 tf/12 不重立，只填 `Deps.Conditions`；两个「没有」默认甲；对象种类维今天没有、集运单元照 tf/12 经 PS 答没有（psr/07 与 tf/12 两条评审合成一句）；与 tf/13 在 `assemble_delivery_dispatch{,_test}.go` 同文件——先做适配器半边、再按谁先推谁先落。2 已接（估 60–80 分）。
- **tf/13 完工报 15:5x**（通道 5，task-247c5d4d done：作者已 rebase 到 `0c9b846f`，tip `dab85498`，七笔含清点；带 DSN TF 全包 + 反向依赖 + parcel-api + parcel-dispatch + architecture 13 包 ok / PASS 1877 / SKIP 0；装配停点后移到 DELIVERY_CONDITION_SOURCE_NOT_WIRED；顺手改了 tf/12 http 替身签名一行）。2 在 tf/14、6 余量将满、3 / 4 未起 → 推送方自跑 /code-review：**Standards 0 阻断 / 1 非阻断**（`ports/delivery_requirement.go` 头注「PS 那一侧……归票 12……才落适配器」在 rebase 后过时，tf/14 改同段头注时一并改准）· **Spec 0 阻断 / 0 非阻断**。八道判断题全部认可。全文落票 13 Comments。
- **重放**：`%TEMP%\idp-replay-tf13` 在 `0c9b846f` 上 pick 六笔全干净（`4e5d34a9→b8c3d0c1` / `30a49640→0363b088` / `0d3dd6a3→31a71b22` / `e400e8ac→eb18af94` / `9a068d2d→0c99d362` / `dab85498→636366c3`）；作者清点笔 `bdb1224c` 跳过，tip 重生成为 `1cb827b0`。gofmt / build / vet 0、architecture ok、基线实测 3；占 55432 广播后带 DSN 全量 **104 ok / 0 FAIL / 15 无测试 · PASS 8048 / SKIP 1 / FAIL 0 / 0 cached · 124 s**，探针 PASS，释号。
- **进 main**：推送方簿记一笔在 `1cb827b0` 之上（票 13 Comments 两条 + 本节）；`ls-remote` 核 `0c9b846f` 未动 → ff → `push <sha>:main`，SHA 见推后广播。`mcp5-tf13` 比内容后改 `merged/`（昨晚裁决指针已占 `merged/mcp5-tf13`，本笔用 `-impl` 后缀）、远端删、树拆；`idp-replay-tf13` 拆。
- **接下来**：tf/14（2）完工后 → 评审（5 空闲、非作者，派它）→ 重放 → 全量 → ff；tf/14 进 main 后 **tf/09 父票收口**（三条缝全接，执行器停点走到 OpenDispatchTask）由推送方一笔 .md。nr/03 补测试小笔（5，`mcp5-nr03-tail`）随到随收、自审直推。3 / 4 至今未起——今天 15:2x 后再无新崩，剩余会话稳定。**归用户**：CI 计费；两件 untracked；会话中断成因（今天 8 次，最后一次 12:2x）。
- **本节五数**：CI main 绿/总 未量 · 集成时延 tf/13 完工报 15:5x → 推出 16:0x **约 15 分**（1 票）· 重放笔数 **7**（六笔作者 + 推送方清点 1）· 重复开发 0 次 · 会话重置 0 次（本小节内）
- **16:1x nr/03 补测试小笔进 main**：`mcp5-nr03-tail@1b0ab5e0`（通道 5，基 `0c9b846f`，纯测试 +27 行）→ 重放 `dbd9d49d`；推送方自审不派评审（14:2x 说定）；带 DSN 全量 **104 ok / PASS 8049 / SKIP 1 / FAIL 0 · 122 s**，新例 PASS；簿记随本笔，ff 推。`mcp5-nr03-tail` 改 `merged/`、远端删、树拆。

### 16:3x–16:5x 同一会话：tf/14 完工 → 通道 5 评审 → 进 main；tf/09 父票收口——三条派送要求缝全接

- **tf/14 完工报 16:3x**（通道 2，task-1d5f0b15 done：作者已 rebase 到 `ffdf1d5c`，tip `47e1f13b`，五笔含清点；带 DSN TF 全树 + parcel-api + parcel-dispatch + architecture PASS 1739 / SKIP 0；**三缝齐后生产装配下执行器再无 NOT_WIRED 停点**，空库一拍走到三个所有者答 REQUIREMENT_MISSING [DELIVERY_PLACE DELIVERY_WINDOW DELIVERY_CONDITION]）。**评审派通道 5**（task-2eddaa4a，非作者），16:4x 交：**Standards 0 阻断 / 2 非阻断**（头注一句「今天 PC 没有这一族」在 pc-gaps/11 进 main 后过时——推送方簿记笔改一句；同包三上下文 import 是 ADR-0133 选的形，归 owner）· **Spec 0 阻断 / 2 非阻断**（两个「没有」取甲归 owner；判据 2 改口同 tf/12）。全文落票 14 Comments。
- **重放**：`%TEMP%\idp-replay-tf14` 在 `8d38a0e5` 上 pick 四笔全干净（`12020372→89d601d5` / `e7cd7272→e58e1483` / `595c659c→de1f9de2` / `47e1f13b→a731e362`；第一笔 pick 的是 rebase 前同内容对象 `1e22410f`，逐字核同）；作者清点笔 `1accc36d` 跳过，tip 重生成为 `95b07c3f`。gofmt / build / vet 0、architecture ok；占 55432 广播后带 DSN 全量 **104 ok / 0 FAIL / 15 无测试 · PASS 8067 / SKIP 1 / FAIL 0 / 0 cached · 125 s**，探针 PASS，释号。簿记笔另改 `delivery_condition_source.go` 一句注释（评审非阻断 1），改后 gofmt / build / vet / 该包重跑 ok——全量是在 `95b07c3f` 跑的，簿记笔与它只差这一行注释与 .md。
- **tf/09 父票收口**：12（0c9b846f）/ 13（ffdf1d5c）/ 14（本笔）全 resolved 进 main，按 issue-tracker「Complete a parent」Status → resolved、Blocked by → 无，随本笔落。`tf-segment-lifecycle-closure` 这一目录自此 09 / 12 / 13 / 14 四张全收；三条派送要求缝的提供方半边（psr/06、psr/07、nr/03、pc-gaps/11）与消费方半边（tf/12、13、14）今天一天从 ready-for-agent 到 main。
- **进 main**：推送方簿记一笔在 `95b07c3f` 之上（票 14 Comments 两条 + 注释一句 + 票 09 Status / Blocked by + 本节）；`ls-remote` 核 `8d38a0e5` 未动 → ff → `push <sha>:main`，SHA 见推后广播。`mcp2-tf14` 比内容后改 `merged/`、远端删、树拆；`idp-replay-tf14` 拆。
- **接下来**：前沿要重算（`Status:` 行扫全部 `.scratch/*/issues` 找 ready-for-agent 且 Blocked by 无）——今天派出的九张（psr/06 / 07、nr/03 + tail、pc-gaps/11、sa/04、tf/12 / 13 / 14）全部进 main，2 / 5 空闲、6 待命将满、3 / 4 未起。owner 复核队列今天新增：ADR-0031 第四落点（pc-gaps/11）、ADR-0133 决定二产品未采用那一格（pc-gaps/11）、「Consequences 第 N 条」序位引用（psr/06 / 07）、ADR-0130 / 0133 越权风险点 1 两个甲（tf/12 / 14）、tf adapters/partycommercial 三上下文 import 的形（tf/14）。**归用户**：CI 计费（main 今天 12 次推送一次 run 都没起）；两件 untracked；会话中断成因。
- **本节五数**：CI main 绿/总 未量 · 集成时延 tf/14 完工报 16:3x → 推出 16:5x **约 20 分**（1 票；含非作者评审）· 重放笔数 **5**（四笔作者 + 推送方清点 1）· 重复开发 0 次 · 会话重置 0 次（本小节内）

## 2026-09-10 14:1x 通道 1 新会话接续（接手时 `main = origin/main = 5dda0fb2`；用户先要一次自带 `ask_question`，再「监听队列」→ 通报「九张全部合入 main」→「重新扫描 .scratch/*/issues 找出 ready-for-agent 且无阻塞的前沿任务，再对通道进行派单」）

- **时刻口径**：本节按本机 `Get-Date`（+08:00）记。上一节写的 16:xx 与其对应提交的 git 时刻（`5dda0fb2` = 13:37 +0800）相差约 +3h，属那一任会话的时钟口径，不改写；读两节对不上时以 git 提交时刻为准。
- **接手取证**（14:1x）：`branch-state.ps1`——本地 main = origin/main = `5dda0fb2`，**在途分支无**（未加 merged/ salvage/ 前缀的一条都没有），共享树 71 行 ` M` 经 `git diff --ignore-cr-at-eol --name-only` 全为 CRLF 幻影 + `d_tasks.txt` / `docs/api/` 两件 untracked 归用户。**前沿重算**（逐文件读 `.scratch/*/issues/*.md` 的 `Status:` 首词）：**260 resolved / 4 needs-info / 4 blocked / 3 draft / 2 done / 1 superseded / 0 ready-for-agent / 0 in-progress**。非 resolved 的十四张逐张看过：needs-info 四张（nr-views/01、ps-external-mark-relations/01、syn-wall-door-audit/01、ve-008/04）与 blocked 两张（auto-reroute/02、first-tenant-runway/03）都卡实例半边（PAR-NET-14 / PAR-INT-01 / S0 证据）或排期决定，不可派；draft 三张里 lc/20 / lc/22 等第一家真源（无租户即无真源，留 draft），ve-claims-read-seams/04 是四问待裁的裁决票；**blocked 另两张 psr/01 / psr/03 的 Blocked by 是「PC 半边等 pc-gaps 批」，而 pc-gaps/09（ADR-0119，`838b283e` / `d06192ed` / `d58ceecc`）与 pc-gaps/08（ADR-0116，`0cfe3571` / `31903cc5` / `02ff5205` / `44bcac4d`）09-07 已进 main、越权点 owner 09-09 认可，两张票没人回头转状态**——票面状态比 main 慢了三天。代码侧核过：`cmd/parcel-api/assemble_customer_amendment.go` 已装 `UnconfiguredSourceDataAmendmentAuthorizer{}`（psr/04 的生产入口），换真适配器就是 psr/03 PS 半边；`NewJudgeLabelServiceFinalHandler` 在 `cmd/` 仍零调用方（label-channel/11 三张接线票至今未立），psr/01 PS 半边适配器落地后无生产装配点，如实写进票面与派单。
- **簿记一笔 `dd5ed934`**（在 `5dda0fb2` 之上，纯 .md，自审；`ls-remote` 核未动后 `push dd5ed934:main`，推后 = `dd5ed934`）：psr/01 / psr/03 Status → ready-for-agent、Blocked by → 无，Comments 各一条把 PC 落地件逐符号对号、PS 剩余件列清；`tf-segment-lifecycle-closure/spec.md` 按 issue-tracker「Complete a parent」→ resolved（01–14 十四张逐文件读过均 resolved）。
- **点名**（14:20 广播 2–6，截止 14:24；不开 `activeOnly`）：14:24 前**五个全到**——2 / 3 / 4 / 5 / 6 均「空闲 · 地盘无 · 余量充足」（今天 15:2x 后无新崩，3 / 4 已重起）。`list_sessions` 同刻 2–6 running / 7–11 offline。
- **派单**（14:3x，基 `dd5ed934`）：**psr/03 → 通道 2**（`task-0450f54b`；立票人；适配器照撤回那只 + `buildCustomerAmendmentOrchestration` 换真 + 剪基线两条 `RequestedByCustomerAccount` / `RequestedByOperatorRole`；RequestSource 留 nil 实例半边；停点从未配置后移到授权未形成是唯一可观察的生产变化）· **psr/01 → 通道 6**（`task-45b75eb2`；适配器 `label_validity_rule.go` 六例；无生产装配点，派单明写「不为接上而在 cmd/ 装没人调的编排」）· **只读重核 → 通道 4**（`task-f985c4d0`；把 `unresolved-review-20260904/remaining-work-a3a4814.md` 各条在 `dd5ed934` 上逐条判已落 / 仍余 / 已被替代，产出 `remaining-work-dd5ed934.md`，「仍余且机制半边且无票」单列——那是下一轮立票名单；40 分）。**3 / 5 留评审位**。两张 PS 票同包 `adapters/partycommercial` 各建新文件、都不碰既有文件；psr/03 独占 cmd/parcel-api 与接线基线。
- **待用户**：**ve-claims-read-seams/04 要不要授权 VE owner 口径代裁**（锚点从哪来 / 键按什么维 / 要不要 `CustomerContractObject` / 登记入口放哪；四问答完才有实施面；派法照 09-09 ADR-0130–0133 那四张——代裁票 + 拆落地票）；label-channel/11 那三张接线票要不要立（「择优是运营端点还是面单交易编排的前置步，是产品流程决定」，remaining-work 第 1 / 2 条）——等通道 4 的表回来一并问。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因。
- **接下来**：psr/03（2）/ psr/01（6）完工报到 → 派 3 / 5 非作者评审 → 重放 → 含 DSN 全量（占 55432 广播）→ ff 推；两票若都动 `adapters/partycommercial` 包内测试夹具同名，后进的重验。通道 4 的表回来 → 立票名单交用户过一眼 → 下一波。
- **14:4x 用户问「mcp-3/5 空闲，没有其他的活了吗」**——答：代码票没有了是票面没跟上，不是活没了；把挖票的活派出去。**通道 3 → 立票**（`task-b5dba034`）：lc/11「不在本票」三个 `JudgeLabelServiceFinalHandler` 调用方各一张接线票 lc/25 / 26 / 27（命令形状已定，票面以代码为准取证：TF 首次有效收寄信封 → PS inbox 消费者照 VE 那只的形；交易定案那一拍同事务 / 后置的代价列「要裁的」；受控关闭决定口无写入方可能再多一条 Blocked by）+ lc/28 draft 记择优组合根的产品决定（`SelectChannelCandidate` 在 cmd/ 零命中）；同笔补 lc spec 子票表四行。**通道 5 → 只读扫描**（`task-a5b220d1`）：09-05 以来 resolved 票面里「另立票 / 不在本票 / 归 owner 复核 / 待裁 / 等 X 落地后剪」四类句子逐条判有票 / 无票 / 已复核 / 待复核 / 痕迹可剪，产出 `followup-inventory-20260910/report.md`——与通道 4 那份（09-04 清单）合成下一轮立票名单。`git grep -c '另立票|不在本票|另起小笔'` 在 issues 下命中 130+ 份，这一格以前没人收。三份父 spec 顺手对齐：`party-commercial-context-gaps` / `wiring-baseline-remainder` 十一张子票各自全 resolved → spec resolved；`ps-port-remainder` spec Status 行补今天 01 / 03 解阻分派与 06 / 07 进 main。
- **本节五数**：CI main 绿/总 未量（账单未修）· 集成时延 —（本小节无票进 main）· 重放笔数 0 · 重复开发 0 次 · 会话重置 **1 次**（通道 1 本会话 14:1x 新起；上一任 16:5x 那节之后无接续记录）

### 14:4x–14:57 同一会话：通道 4 重核表进 main → psr/01 完工 → 推送方自评 → 进 main

（**时刻校准**：本节起到 15:52 的五个小节，标题与「完工报 → 推出」里的时刻先前按估写、偏快 20–60 分；16:0x 按 `git log` 各笔提交时刻改准——推出时刻取簿记笔提交时刻，完工报到达时刻取作者分支 tip 提交时刻之后、我下一步动作之前的区间；各通道自报的时刻（如「16:2x 交」）是它们的时钟，同样改成到达时的本机时刻。上面 14:1x 那一节开头那句「上一任 16:xx 比 git 晚约 3h」的教训，本会话自己也犯了一半。）

- **通道 4 重核表**（task-f985c4d0 done）：`mcp4-rework` 作者报 `43435787`、origin 上是 amend 后的 `091e5398`（同父同题差六行——把我 14:3x「lc 第 1 / 2 条归通道 3」那句写进去了），取 origin 版重放到 `9dddaf65` 上为 `3f485e97`，文件逐字同，纯 .md 自审直推。结论：09-04 清单一至三节 21 条全部已落或已被替代、六节簿记 19 条已落、七节树况 4 条已落、四节 7 条实例阻断状态未变；**仍余且机制半边且无票 8 条**（1 / 2 归通道 3 立票；3 PC `DeclaredResponsibilityOutcome` 面单渠道两行；4 保价三落点 + PC「保价条件」声明族；5 SA inbox 两件；6 CC 四件机制；7 VE 两册登记口；8 E2 Excel 工具）。顺手订正 a3a4814 一处：`RehydrateContinuedAttemptRegister` 早在 `0d492b8f` 出基线。**再派通道 4 立票**（task-9880bbc9，60 分）：第 3 条 → pc-gaps/12（同笔把我 14:4x 刚收口的 pc-gaps spec 改回 in-progress）；第 5 + 6 条 → 新目录 `sa-cc-funds-and-credential-seams/`（spec + 一件机制一张）；第 7 条 → awf/25 或 ve-disclosure-policy-view 下一号；第 4 保价要先 /domain-modeling、第 8 归用户侧，先不立。
- **psr/01 完工报 14:4x**（通道 6，task-45b75eb2 done：分支 `mcp6-psr01` 基 `dd5ed934`，tip `14db4a1a`，五笔；两份新 .go、未改任何既有文件；PS adapters/partycommercial + application + architecture ok；未带 DSN——无 .sql 无 cmd 改动，反向依赖四包对新类型零引用）。3 / 4 / 5 各在立票 / 只读任务上、2 在 psr/03 → **推送方自跑 `/code-review`**（子代理 `Task` 仍报认证错，两轴串行、互不引用）：**Standards 0 阻断 / 0 非阻断** · **Spec 0 阻断 / 1 非阻断**（零值锚答 error 而非「没结果就没失效」，认可作者口径不改）；判断题四道全认可，「两口而非三口」判等价（`FinalContentFor` 就是那两跳，夹具走真实路径）。全文落票 01 Comments。
- **重放**：`%TEMP%\idp-replay-psr01` 在 `3f485e97` 上 pick 五笔全干净（`560f51a7→0a603cc8` / `c9ba714f→a557485a` / `1a23f5b2→bc67de03` / `ff9af9bd→e9d23fd1` / `14db4a1a→25fc0e82`）；作者清点笔**未跳过**——`dd5ed934`→`3f485e97` 之间 main 只动 `.scratch/**`，清点在 tip 重生成零差，沿用即可（与今天前几票「跳过再重生成」不同，成因是这一票之前 main 没进过代码）。gofmt / build / vet 0；占 55432 广播后带 DSN 全量 **104 ok / 0 FAIL / 15 无测试 / 0 cached · 119 s**，探针 PASS，新包 8 例 PASS，释号。
- **进 main**：推送方簿记一笔在 `25fc0e82` 之上（票 01 Status 行 + 评审 + 进 main 记录 + 本节）；`ls-remote` 核 `3f485e97` 未动 → ff → `push <sha>:main`，SHA 见推后广播。`mcp6-psr01` 比内容后改 `merged/`、远端删；树由通道 6 拆。
- **接下来**：psr/03（2，基线已剪、cmd 装配在做）完工报到 → 自评或派 3 / 5（若已交立票 / 扫描）→ 重放 → 全量 → ff；通道 3 lc/25–28、通道 4 pc-gaps/12 + 新目录、通道 5 余工表交回 → 我核「要裁的」为零的转 ready-for-agent → 下一波派 3 / 4 / 5 / 6。**待用户**（不变）：ve-claims/04 代裁授权；lc/28 择优组合根产品决定；保价要不要开 /domain-modeling。
- **本节五数**：CI main 绿/总 未量 · 集成时延 psr/01 完工报 14:4x → 推出 14:57（`5e4c8cfc`）**约 10 分**（1 票；含推送方自评）· 重放笔数 **6**（五笔作者含清点 + 通道 4 重核表 1）· 重复开发 0 次 · 会话重置 0 次（本小节内）

### 14:5x–15:08 同一会话：psr/03 完工 → 通道 6 评审（今天第二次非推送方评审）→ 进 main；lc/25–28 与余工表进 main；3 / 4 / 5 再派

- **三件同刻交回（14:5x–15:0x）**：psr/03 完工报（通道 2，task-0450f54b done：分支 `mcp2-psr03` 基 `dd5ed934`，tip `276e3770`，代码三笔 + 票面两笔；带 DSN 7 包 PASS 1085 / SKIP 0；RequestSource 交坐标而非整份 AuthorizationRequest 那一处自报）；lc/25–28 立票（通道 3，`mcp3-lc-tickets@e0ae4196`，四张全 draft、要裁的 1/2/1/2，**核心发现：lc/11 与余工表写的「`external-carrier-tracking` 信封 = 首次有效收寄」是误读**，TF 十种出向事件无收寄一种，25 要先等 TF 侧收寄判断执行器那张票；27 要等关闭 / 重开决定写面那张票；两张上游票都没立过）；09-05 以来余工表（通道 5，`mcp5-followups@500541e0`，57 张 resolved 票 790 句逐句核：无票且机制半边 22 组 / 待 owner 复核 8 项 / 痕迹可剪 7 处 / 未核 8 条）。两笔纯 .md 直接 cherry-pick 进共享 main（`51674f4b` / `93349f83`），推送时一次 TLS 抖动重试即过。共享树上 `git cherry-pick` 撞了一处 CRLF 幻影（lc/11 票面），`git checkout -- <file>` 还原后再 pick——幻影不只是噪音，它会挡重放。
- **psr/03 评审派通道 6**（task-b4a2553d，14:5x；6 刚交 psr/01、非作者、空闲）→ 15:04 交：**Standards 0 阻断 / 2 非阻断**（两处注释对 PC 封闭集 / 本包文件的计数措辞；三适配器同形四格 switch 为既有纪律不建议改）· **Spec 0 阻断 / 1 非阻断**（RequestSource 交坐标有票面红线与派单③依据，认可；代价是本包两种 RequestSource 形状并存）；派单五点逐核无发现，作者五道判断题全认可。全文由推送方代落票 03 Comments。
- **重放**：`%TEMP%\idp-replay-psr03` 在 `93349f83` 上 pick 四笔全干净（`39c1206d→f9852a3f` / `07fe7930→ee1e8cc1` / `9c6ccd4c→bb0abf2f` / `276e3770→5cdac262`）；作者清点笔 `27607caa` 与 psr/01 之后的同文件冲突→跳过，tip 重生成为 `19f323f1`。gofmt / build / vet 0、architecture ok（基线 PC 组两行剪掉在 tip 成立）；占 55432 广播后带 DSN 全量 **104 ok / 0 FAIL / 15 无测试 / 0 cached · 120 s**，探针 `cmd/parcel-api -run Amendment` 五例 PASS，释号。
- **进 main**：推送方簿记一笔在 `19f323f1` 之上（票 03 Status 行 + 评审 + 进 main 记录 + 本节）；`ls-remote` 核 `93349f83` 未动 → ff → `push <sha>:main`，SHA 见推后广播。`mcp2-psr03` 比内容后改 `merged/`、远端删；树由通道 2 拆。
- **再派**：**通道 5 → 痕迹清剪七处**（task-14712757，立票 followup-inventory-20260910/issues/01 即认领，纯注释 / 测试名，0028 迁移头注那处先查校验和纪律再动）；**通道 3 → 立票 awf/25 交付条件管理台写面 + 读回 + 载荷 + 折进 PCC-1**（task-910498f2；pc-gaps/11 收口三处点名的那张 awf 票；要裁的为零就直接开工，否则报我）；**通道 4 交回九张票**（`mcp4-tickets@bc470b8a`：pc-gaps/12 要裁 3、`sa-cc-funds-and-credential-seams/` spec + 01–07 共要裁 13、ve-disclosure-policy-view/02 要裁 3——**九张各至少一条要裁的，无一张可直接转 ready**），随下一笔重放。
- **前沿此刻的形状（给用户）**：代码票只剩 psr/03（本节进 main）与通道 3 若开工的 awf/25；**新立的 13 张票（lc/25–28、pc-gaps/12、sa-cc 01–07、ve-disclosure/02）全部卡在裁决上**，加上 ve-claims/04 四问与保价建模，**不授权代裁就没有下一波代码票**。09-09 那四张（ADR-0130–0133）走的正是「用户经队列授权 owner 口径代裁 → 落 ADR + 拆落地票 → 越权风险点单列供复核」，今天已复核认可两篇。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因。
- **本节五数**：CI main 绿/总 未量 · 集成时延 psr/03 完工报 15:2x → 推出 15:4x **约 20 分**（1 票；含通道 6 评审约 12 分）· 重放笔数 **7**（四笔作者 + 推送方清点 1 + 通道 3 / 5 纯 .md 各 1）· 重复开发 0 次 · 会话重置 0 次（本小节内）

### 15:4x–16:0x 同一会话：通道 4 九张票进 main → 2 / 6 再派 → 通道 5 痕迹清剪进 main

- **通道 4 九张票**（`mcp4-tickets@bc470b8a` 三笔）直接 cherry-pick 进共享 main（`083c1159` / `29ed489b` / `0e8d048a`）；pc-gaps spec 由它如实改回 in-progress（我 14:4x 刚收的，加了 12 就该回去）。指针 mcp2-psr03 / mcp4-tickets / mcp4-rework / mcp3-lc-tickets / mcp5-followups 改 `merged/`、远端五条删。
- **再派**：**通道 6 → sa/05 admin-web 授权处置队列页与处置操作**（task-c9aaf964；sa/04 票面「边界」划归 admin-web 另票的那张；两口都在 main；照 AcceptanceReviewPage 的形；要裁的为零就开工）；**通道 2 → psr/08 PS 评审尾巴**（task-ca3d37b0；psr/03 评审 Standards ① 两处注释 + 判断题 5 历史头注 + wbr/04 观察「ActiveRejectionAdapter 没有对称的动作守卫」；零待裁立票即认领）。通道 3 awf/25 立票中（15:4x 状态：伞票那道「何时拒收旧串」在它这条路上避得开，倾向不列要裁的，16:1x 前推立票笔）。
- **通道 5 痕迹清剪完工 15:5x**（task-14712757 done：`mcp5-residue@2a8df802` 基 `93349f83`，三笔；11 件只改注释 / 测试头注 +25/−21；0028 迁移头注**不剪**——`checksumOf` 对全文件含注释算校验和、`ErrChecksumDrift` 会拦住所有已施加 0028 的库，先例 wbr/03——理由成立）。推送方自审：隔离树 `%TEMP%\idp-replay-residue` 在 `0e8d048a` 上 pick 三笔全干净（`ae0b26b2→84a5f939` / `13ea493d→82a998c5` / `2a8df802→637d77da`），`git diff -U0` 过滤注释后零条；gofmt / build / vet 0；占 55432 后带 DSN 全量 **104 ok / 0 FAIL / 0 cached · 120 s**；admin-web tsc 0、run-tests **198 / 198**（junction 借 node_modules，验完拆）；释号。簿记一笔在 `637d77da` 之上（票 01 进 main 记录 + 本节），ff → `push <sha>:main`。`mcp5-residue` 改 `merged/`、远端删；树由通道 5 拆。
- **接下来**：通道 3 awf/25、通道 6 sa/05、通道 2 psr/08 完工报到后各派非作者评审（4 / 5 空闲可接）→ 重放 → 全量 → ff；**十三张新票 + ve-claims/04 + 保价的裁决等用户答**（15:4x 已把三种走法与两张上游票的问题送队列）。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因。
- **本节五数**：CI main 绿/总 未量 · 集成时延 痕迹清剪完工报 15:5x → 推出 16:0x **约 10 分**（1 票，自审）· 重放笔数 **6**（通道 4 三笔 + 通道 5 三笔）· 重复开发 0 次 · 会话重置 0 次（本小节内）

### 16:0x–16:3x 同一会话：awf/25 裁乙开工 → psr/08 完工 → 通道 4 评审 → 进 main

- **awf/25 立票交回一条要裁的**（通道 3，16:0x）：读回落在哪一面——甲 政策页新一册 `?kind=DELIVERY_CONDITION` / 乙 折进两张版本目录行（LEFT JOIN 0030）。**推送方代裁取乙**（属管理台呈现落点，不属领域归属）：伞票 07「写签跟着读签走」；交付条件挂在版本上（ADR-0133 决定四）与 0012 合同正文 LEFT JOIN 进合同行同形；不动 cmd/parcel-api。地盘扩到 ports.go 两行可缺格 + adapters/postgres 目录读两条查询：认可。通道 3 已按乙开工。**sa/05 零待裁**（通道 6，16:0x）：页面位置照复核页、字段照 Go 键名、去向只 REJECT / CUSTOMER_SUPPLEMENT（ADR-0132 决定一），已开工。
- **psr/08 完工报 16:0x**（通道 2，task-ca3d37b0 done：`mcp2-psr08@f2c96f02` 基 `0e8d048a`，三笔；①② 纯注释，③ `active_rejection.go` 加与复核那只同形的动作守卫 + 一正一反用例——有行为变化；撤回那只不加，PC 无撤回动作）。**评审派通道 4**（task-a7df43ca，空闲非作者），16:2x 交：**Spec 0 阻断 / 1 非阻断**（撤回头注「借现有动作占位」是推论非引文）· **Standards 0 阻断 / 1 非阻断**（Covers 句「第 2 条」序位，非本票引入）；四点逐核属实（守卫位置同形、生产 RequestSource nil 守卫不可达、反格真证不问提供方、撤回不加理由成立）。全文由推送方代落票 08。
- **重放**：`%TEMP%\idp-replay-psr08` 在 `d3b82200` 上 pick 三笔全干净（`29b08395→d731b6c2` / `9decab60→0806a7e6` / `f2c96f02→c5b3edc1`）；gofmt / build / vet 0；占 55432 后带 DSN 全量 **104 ok / 0 FAIL / 0 cached · 120 s**；释号。簿记一笔在 `c5b3edc1` 之上（票 08 评审 + 进 main 记录 + 本节），ff → `push <sha>:main`。`mcp2-psr08` 改 `merged/`、远端删；树由通道 2 拆。
- **接下来**：通道 3 awf/25、通道 6 sa/05 完工后派 4 / 5 评审 → 重放 → 全量 → ff；通道 2 空闲——**PS 地盘再无零待裁的票**（psr/01 / 03 / 08 今天全收；ve-claims/04 是 VE、lc/26 要裁），等用户对裁决走法的答复。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因。
- **本节五数**：CI main 绿/总 未量 · 集成时延 psr/08 完工报 16:0x → 推出 16:3x **约 25 分**（1 票；含通道 4 评审约 12 分）· 重放笔数 **3** · 重复开发 0 次 · 会话重置 0 次（本小节内）

### 16:3x–16:5x 同一会话：sa/05 完工 → 通道 5 评审 → 进 main

- **sa/05 完工报 16:3x**（通道 6，task-c9aaf964 done：`mcp6-sa05@5213ef4a` 基 `0e8d048a`，六笔，只动 `apps/admin-web/src`；tsc 0 / run-tests 208 pass 含新十例；决定集类型上只两值、403 停点原词直显、复核页 items[] 两格显出、导航接线纯加 14 行）。**评审派通道 5**（task-d8b6c412，空闲非作者、刚清过 admin-web 注释），16:5x 交：**Standards 0 阻断 / 2 非阻断**（三处「三组」跨文件计数、两条文案函数尾段同形）· **Spec 0 阻断 / 2 非阻断**（版本 subtitle 边外一根、未配置态 tone 同普通 outcome）；六点逐核属实（无通过 / 放行动作、键名逐字对 Go、不自判权限、`SubmissionVersion` 是编排真入参但端点 `UnconfiguredIntake{}` 今天不解体、抽共用后既有页逐行同、`occupationFormed` / `jointPassCondition` 两格确在「不做的」内）；六道判断题全同意。全文由推送方代落票 05。
- **重放**：`%TEMP%\idp-replay-sa05` 在 `48ccbb2f` 上 pick 六笔全干净（`2f7c3a6d→fe239830` / `b45941d4→9243c9e8` / `85583479→796fa8ff` / `15fc0f75→96e8c890` / `ca1233e7→767e4146` / `5213ef4a→46c11d02`）；本票文件与分支逐字同；tsc 0、run-tests **208 / 208**（junction 借 node_modules，验完拆）；无 Go 改动不跑 Go 全量。簿记一笔在 `46c11d02` 之上（票 05 评审 + 进 main 记录 + 本节），ff → `push <sha>:main`。`mcp6-sa05` 改 `merged/`、远端删；树由通道 6 拆。
- **接下来**：通道 3 awf/25（PC 四包 + admin-web 两张表单，最大的一张）完工后派 4 / 5 / 6 之一评审 → 重放 → 全量（含 DSN，cmd/parcel-commercial 夹具那格）→ ff。2 / 4 / 5 / 6 空闲——**零待裁的机制活已尽**：今天由余工表挖出的三张（痕迹清剪、psr/08、sa/05）全收，下一波 13 张票 + ve-claims/04 + 保价全等裁决，用户尚未答复走法。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因。
- **本节五数**：CI main 绿/总 未量 · 集成时延 sa/05 完工报 16:3x → 推出 16:5x **约 20 分**（1 票；含通道 5 评审约 15 分）· 重放笔数 **6** · 重复开发 0 次 · 会话重置 0 次（本小节内）

## 2026-09-10 16:3x 通道 1 新会话接续（接手时 `main = origin/main = 1c07ef50`；用户先要一次自带 `ask_question`，再「监听队列、正常回复、循环」）

- **时刻口径**：本节按本机 `Get-Date`（+08:00）记。上一任最后一节标题写 16:3x–16:5x，其簿记笔 `1c07ef50` 的 git 时刻是 15:52 +0800——那一任自己在共享树上留了一份**未提交**的时刻校准（`.scratch/tasks.md` mtime 16:07:44，8+/6−：把 14:5x 起几节的标题与「完工报 → 推出」按 `git log` 改准、并写明「先前按估写、偏快 20–60 分」），改到 sa/05 那节之前会话就断了；通道 3 报数时也量到前会话广播里写 17:xx 而 `03b9b52e` 提交时刻是 16:12:08。那份校准按「未提交现场」那节的做法原样入库、单独成笔（见下「进 main」），本节不替它改 sa/05 那节的时刻；两节对不上时以 git 提交时刻为准。
- **接手取证**（16:31）：`branch-state.ps1`——本地 main = origin/main = `1c07ef50`（`ls-remote` 核过）；在途分支只 `mcp3-awf25@c8243f4f`（= origin，领先 main 10 笔，main 上从未有过的代码文件 11 件）；三棵树：共享树 71 行 ` M`（与上一任同一批 CRLF 幻影 + `d_tasks.txt` / `docs/api/` 两件 untracked，归用户，未动）、`%TEMP%\idp-review-awf25` detached @`03b9b52e` 干净（来历不明，无人认领——按时刻推是上一任 16:12–16:14 之间建的评审树，未派出去会话就断了；本节交通道 4 直接用）、`D:/tops/idp-parcel-mcp3-awf25` 干净。**报数**（16:3x，各通道主动报到，非本节点名）：2 / 3 / 4 / 5 均「新会话、空闲、地盘无、余量充足」；6 未报（`list_sessions` 显 running / idle）。`query_tasks pending` 里躺着一条陈旧派单 `task-f8baa675`（12:1x 派通道 3 评 nr03，那票早已进 main）——待清，不影响本节。
- **awf/25 完工报 16:30**（通道 3，task-910498f2 done；分支 `mcp3-awf25` 基 `93349f83`，代码 tip `cd95f3a2`，分支 tip `c8243f4f`，十笔：立票 / 裁乙 / 领域 / 应用两向 + cmd 夹具串 / 载荷 / 读回 / admin-web / 测试头注 / 清点 / 票面 resolved；39 文件 +2330/−169；作者自验带 DSN 13 包 PASS 1093 / SKIP 0、tsc 0、run-tests 204）。推送方核：`93349f83..1c07ef50`（main 这段 26 笔）与本票 `.go/.sql/.ts/.tsx` **零重叠**（`Compare-Object` 两侧 `git diff --name-only` 量的），按「别人分支上的活怎么进 main」那节可直接重放、不必作者重验。
- **评审拆两半派 4 / 5**（16:36，task-5d0d7a78 → 通道 4 Go 半边 `internal/partycommercial/{domain,application,adapters/http,adapters/postgres,ports}` + `cmd/parcel-commercial/publish_batch_test.go`；task-134a77b2 → 通道 5 admin-web 半边 `apps/admin-web/src/pages/party/**`；各 20 分，截止 16:55）。拆的理由：39 文件 20 分钟一人看不完，两半在语言与地盘上天然可分（Go / TS 零共享文件），两份报告都落票面 Comments、各自三格。通道 4 沿用 `%TEMP%\idp-review-awf25`（checkout 到 `c8243f4f`），通道 5 另建 `idp-review-awf25-web`。
- **重放**（与评审并行，评审无阻断才推）：`%TEMP%\idp-replay-awf25` 在 `1c07ef50` 上 pick 九笔全干净——`92fb79af→2d0139a4` / `7578a9b9→0b07371c` / `554dffac→bebe0c7b` / `b5533242→67400b94` / `4108cc3a→a6b4accc` / `bec9b3e9→9b960f1b` / `c286be66→988fb8e3` / `cd95f3a2→6636dad2` / `c8243f4f→cf16d10d`；作者清点笔 `03b9b52e` 与 main 上 psr/03 之后的同文件 `19f323f1` 冲突 → `--skip`，tip 重生成为 `852bf7c4`（PC 生产 121→123 / 测试 133→138 / http 26→27 / 合计 905→907 / 861→866 / 122→123，与作者在 `cd95f3a2` 上量得的数同）。本票全部文件（清点除外）`git diff c8243f4f cf16d10d -- <files>` 零行。gofmt / build / vet 0；占 55432 广播后带 DSN 全量 **104 ok / 0 FAIL / 15 无测试 / 0 cached · 121 s**，探针 `cmd/parcel-commercial -run TestAPublishedDeliveryConditionIsAnsweredByTheResolutionKeyedReadFace` **PASS** 非 SKIP（本票完成判据 4 那格）；admin-web tsc 0、run-tests **214 / 214**（main 208 + 本票 6；junction 借共享树 node_modules、验完 `rmdir` 拆）；16:43 释号。
- **评审结果**（两份 16:40 / 16:42 到，均在截止前）：**通道 5 admin-web 半边**——Standards 0 阻断 / 3 非阻断（最重：`deliveryConditionRenderedPaths` 别名化认领表，票 22 那道 JSX 漂移比对对本节恒真；另 `key={method}`、前端头注描述后端 SQL）· Spec 0 阻断 / 3 非阻断（最重：票面路径表多列 `.methods` 一项而实现与 Go 都不点它；另 trim 口径与先例不同、`declaredAt` 不显）；判断题 (2)(5)(6)(7)(8) 全同意。**通道 4 Go 半边**——Standards 0 阻断 / 2 非阻断（最重：`json_agg … ORDER BY method_ref` 按库 collation 排，与领域 Go 字节序可能不同，加 `COLLATE "C"` 可钉死；另头注计数冗余）· Spec 0 阻断 / 2 非阻断（最重：PCC-1 不换号靠结构推出、未钉字面摘要串；另 cmd 夹具串评审无 DSN 未独立重算——推送方探针已覆盖）；重点核 ①–⑨ 无发现；判断题 (1)(3)(4) 全同意。**四轴 0 阻断，可合入。** 全文由推送方代落票 25 Comments；可改的四条非阻断记在「进 main 记录」尾，可攒一张 awf 小票。
- **进 main**：推送方簿记一笔在 `852bf7c4` 之上（票 25 Status 行 + 两份评审 + 进 main 记录 + 本节）；其上再一笔 `chore(salvage)` 把上一任留在共享树的 tasks.md 时刻校准原样入库（ff 到这里被它挡了一次：`git merge --ff-only` 报 `.scratch/tasks.md` 会被覆盖——共享树 `--ignore-cr-at-eol` 后唯一的真改动就是它；取 `git diff` 成 patch 在隔离树上 `git apply`、单独成笔、再 `git checkout -- .scratch/tasks.md` 让共享树回到 HEAD 后 ff）；`ls-remote` 核 `1c07ef50` 未动 → ff → `push <sha>:main`，SHA 见推后广播。首次 push 撞一次 `schannel … SSL/TLS connection failed`（今天第二次，上一次 15:0x），重试即过。`mcp3-awf25` 比内容后改 `merged/`、远端删；`idp-replay-awf25` 与 `idp-review-awf25` 由推送方拆、`idp-review-awf25-web` 由通道 5 拆、`D:/tops/idp-parcel-mcp3-awf25` 由通道 3 拆。
- **接下来**：2 / 3 / 4 / 5 / 6 全空闲；上一任「零待裁的机制活已尽」的判断不变——下一波 13 张票（lc/25–28、pc-gaps/12、sa-cc 01–07、ve-disclosure/02）+ ve-claims/04 四问 + 保价建模全等用户答裁决走法（15:4x 那条队列消息未见回复）。可派的只读活：清 `task-f8baa675` 陈旧派单；awf/25 评审非阻断若有可改的，攒成一张小票。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因；裁决走法。
- **本节五数**：CI main 绿/总 未量 · 集成时延 awf/25 完工报 16:30 → 推出 16:4x **约 18 分**（1 票；含 4 / 5 并行双评审各约 6 分、与重放 + 全量并行）· 重放笔数 **9**（作者九笔含票面两笔；推送方清点 1 另计）· 重复开发 0 次 · 会话重置 **5 次**（通道 1 本会话 16:3x 新起 + 2 / 3 / 4 / 5 各自报「新会话、无在途记忆」）

### 16:5x–17:0x 同一会话：用户问「下一步」→ 派裁决分类 + awf/26 → 用户授权「你自决」

- **前沿重算**（`66cad4c4`，逐文件读 `.scratch/*/issues/*.md` 的 `Status:` 首词）：**266 resolved / 16 draft / 4 needs-info / 2 blocked / 2 done / 1 superseded / 0 ready-for-agent / 0 in-progress**。16 张 draft：lc/20、lc/22 等第一家真源（实例半边，无租户即无解，留 draft）；lc/25–28、pc-gaps/12、sa-cc 01–07、ve-disclosure/02 十三张 + ve-claims/04 四问全卡「要裁的」。needs-info 四张与 blocked 两张与 14:1x 一节判的相同，都卡实例半边或排期。**瓶颈是裁决不是人手。**
- **答用户「下一步应该做什么」**（16:5x）：三件不等他先派——① **裁决分类只读活**派 4（lc/25–28 + pc-gaps/12 + ve-disclosure/02 + ve-claims/04，task-f7df8b7e）/ 5（sa-cc spec + 01–07，task-b941ce87），把每条「要裁的」归 **A 推送方可代裁**（呈现落点、落位、命名、同形先例）/ **B owner 口径代裁**（领域归属、边界、不变式、事务边界、跨上下文形状——要用户授权一次，照 09-09 ADR-0130–0133 走 ADR + 拆落地票 + 越权风险点单列）/ **C 只有用户能答**（产品流程、实例半边、排期），17:25 回，目的是把用户要拍的板压到最少；② awf/25 评审尾巴攒一张零待裁小票派通道 3（task-87e19668）；③ 清陈旧派单 `task-f8baa675`（12:1x 派 3 评 nr03，那票早已进 main；工具无 cancelled，用 failed 收口并写明非失败）。要他拍的：裁决走法（建议先看分类表再对 B 一次授权、C 逐条问）、ve-claims/04 授权、lc/28 择优组合根产品决定、保价要不要开 /domain-modeling、老三件。
- **通道 3 抢先立了 26**（17:00，`mcp3-awf26@1254f8f0` 纯 .md，Status ready-for-agent，四项 + 一条带默认的要裁的，不认领、树已拆）——与我同刻发的 task-87e19668 撞了一下。以它立的票为准：**裁「`ListAcceptanceRulePackages` 那两处 ORDER BY 本票不动」**（取默认；那两处无票面「与领域同序」要求，改前要先核那一册领域有没有排序不变式，不是本票的题；完成记录点名同形先例待另票）；**补第五项**钉两串在 66cad4c4 上算出的字面 `PCC-1:…`（通道 4 Spec (1)：把「不换号」从结构推论变成断言）；**派通道 3 接着做 26**（它说「谁做你派」；对这五处最熟，作者收评审尾巴不犯规矩，非作者评审照旧另派）。
- **用户授权**（17:0x，经队列原话：「你是业务和系统超级专家，你自决，按你建议的来吧」）。**读法**（写下来供复核，不扩权）：按我 16:5x 建议的走法执行——A 类由推送方直接裁进票面「裁决」节；B 类按各上下文 owner 口径代裁、落 ADR + 拆落地票、拿不准的单列「越权风险点」供 owner 事后复核（09-09 ADR-0130–0133 的形，Status 行写明「用户授权代裁 + 能力边界」）；C 类里的**产品流程决定**由我按开发主线「机制半边现在做、实例半边留空」的口径定（能做成两条路都走得通的，不替产品选路，把选择留成配置或显式组合根）；**实例半边不在授权内**——真实参数 / 真源 / 租户实例按红线仍拒默认值，那不是裁决是门禁。授权覆盖本节起的十三张 draft 与 ve-claims/04；保价那件要先 /domain-modeling 再立票，另报。
- **接下来**：等 4 / 5 分类表（17:25）→ A 类我直接裁、B 类按上下文分组派 2 / 6（此刻空闲）与交表后的 4 / 5 写 ADR + 拆落地票 → 转 ready-for-agent → 派实现。通道 3 做 awf/26。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因；ADR 越权风险点事后复核（`scripts/owner-review-queue.ps1` 汇总）。

### 17:0x–17:2x 同一会话：分类表提前到 → 29 条全部拍 → 四篇 ADR 分派 + 两张票的 A 类裁决

- **分类表**（4 / 5 各 ~17:00 交，早于截止）：通道 4 七张 16 条 **A 8 / B 6 / C 2**；通道 5 sa-cc 七张 13 条 **A 7 / B 5 / C 1**。合计 29 条：A 15 / B 11 / C 3。分类表全文在两条 report_task 里，不抄进本文；下面只记裁决。
- **推送方裁决（全部 29 条，17:1x）**：**lc/28-1（C）取乙**，带形状约束「06 编排输入 = 择优结果对象（候选 + 七类依据引用 + 评价痕迹引用），编排不关心谁产生它」——首发只接系统择优组合根，运营人工择优作叠加项不立票（越权点：是否需要人工择优归产品 owner）；**lc/28-2 另立 lc/29** 翻译适配器票，28 Blocked by 29。**lc/26-1（B）取乙**（后置一拍：ADR-0090 回执不压 N 次判断、失败有重试载体、三触发共用一只处理方适配器、06 头注「只留缝」仍真）；**26-2 两拍都触发**。**lc/27-1（B）立两半**：lc/30 PS 写面 + pc-gaps/13 PC 授权动作加「关闭」「重开」两格（ADR-0116「加格不要 ADR」），27 Blocked by 两者。**lc/25-1（B，TF）立 lc/31**，形照「有效交付」那一路（TF 在轨迹事实之上一次显式判断形成的事实，自有登记册与事件），走 /domain-modeling 改 TF CONTEXT + ADR。**pc-gaps/12 三条（A）**：`LABEL_SERVICE_COMPLETED` / `LABEL_SERVICE_FAILED`、改词条正文不动 Rules、不换号——已写进票面并转 ready-for-agent（本笔）。**ve-disc/02**：02-1 取 B（CLI + 端点 + 管理台，口径「同族一致」，越权点 VE owner 复核）、02-2 读面另立 ve-disc/03、02-3 并回 `CatalogRegistration`。**ve-claims/04**：04-1 锚点按事实取 = 委托接受时刻经 PS 商业解析回指（ADR-0133 同族）、04-2 键 = （租户，回指）合同由闭包解（ADR-0080 / 0079 / 0133 的形，VE 不登合同维）、04-3 `CustomerContractObject` 列进必需依据、04-4 入口放 `parcel-ve-register`；Blocked by 03 已过期改无。**sa-cc**：01-1 取 (c) + 立 sa-cc/08（UC-SA-002 步 2 编排）；02-1 租户 / 资金事实引用、02-2 同型带回指；03-1 按键取走只读口（信封所指版本）；04-1 一册（凭证门禁判断登记册，就绪判断只引用）、04-2 评估请求驱动不后台重算；05-1 租户 / 申报范围、05-2 立 sa-cc/09（SA 消费付款核对 → `assess_advance_recovery`）；06-1 折法是登记进来的规则（三态接受集合形、无默认）、06-2 加核对版本引用列；07-1 取 B「同族一致」、07-2 去掉 CC 人工补录口（单一铸造口在 SA，越权点 SA owner）、07-3 立 sa-cc/10 三册读面。**实例半边一条没碰**：规则取值、键值、映射、承运商状态码全留实例半边。
- **分派（17:1x–17:2x，均纯 .md、基 `062f5228`、隔离分支、每笔推 origin、约 60 分至 18:15）**：**通道 2 → ADR-0134**（PS owner 口径：三触发后置一拍 + 处理方适配器唯一 + 06 事务边界 + 两拍触发）+ lc/26 转 ready + lc/27 裁决 + 立 lc/30 与 pc-gaps/13（task-138ab1c9）；**通道 6 → ADR-0135**（TF owner 口径：/domain-modeling 首次有效收寄 → TF CONTEXT + ADR + 立 lc/31 + lc/25 Blocked by 31；task-f05bc5a3）；**通道 4 → ADR-0136**（VE owner 口径：索赔规则解析键 = 接受时回指 + 04 转 ready + ve-disc/02 三条写入 + 立 ve-disc/03 + 02 转 ready；task-d6660969）；**通道 5 → ADR-0137**（CC owner 口径、决定四连 SA：逐门禁判断登记册 + 请求驱动 + 门禁规则登记形 + 资金事实单一铸造口 + 01–07 十三条写入 + 立 08 / 09 / 10 + 转 ready；task-9a2ff746）；**通道 3 → awf/26 之后接 lc/28 裁决写入 + 立 lc/29**（task-bc04bfc2）。ADR 号预分配 0134–0137 避免撞号；`docs/adr/README.md` 与各 spec 子票表是共享索引，各加自己一行、重放时我合。每单都要求：Status 行写「用户授权代裁 + owner 口径 + 能力边界」、末尾 `## 越权风险点`、对着 CONTEXT / ADR 核推送方倾向、矛盾就报不硬写。
- **接下来**：四篇 ADR + 票面 18:15 前陆续回 → 我逐条核「越权风险点」写全、CONTEXT 只改自己那句、票面 Status 合规 → 纯 .md 重放进 main（README / spec 子票表冲突手合）→ 转 ready 的票派实现（预计首批：pc-gaps/12、sa-cc/02 / 05、ve-disc/02 步一、lc/26；实现方从 2 / 3 / 4 / 5 / 6 交表后挑非作者）。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因；四篇 ADR 的越权风险点事后复核。

### 17:2x–17:4x 同一会话：awf/26 完工 → 推送方自评 → 进 main

- **awf/26 完工报 17:20**（通道 3，task-87e19668 done：`mcp3-awf26@05a240df` 基 `66cad4c4`，代码 tip `a30f6d67`，八笔；五处：① `deliveryConditionColumns` 方式聚合 `COLLATE "C"` + 真库一例（改前就绿——测试库 C.UTF-8 与字节序同，钉的是部署库换 locale 那一格）；② `deliveryConditionRenderedPaths` 照 JSX 独立列出、测试改集合比对（变异去 `methods[i]` 红 3 例）；③ 票 25 路径表去 `.methods`；④ `ServiceProductsPage.tsx` 头注不描述后端 SQL；⑤ 钉两串字面 PCC-1（先红后钉）。作者带 DSN 8 包 PASS 1678 / SKIP 0、tsc 0 / 214 pass、清点零差。作者对派单的一处更正：「在 JSX 去掉 Field 必须红」红不出来——票 22 那份测试比两份 TS 声明不渲染 JSX，变异改落渲染表；派单写错了，票面判据已改准。
- **评审**：2 / 4 / 5 / 6 全在写 ADR，`/code-review` 子代理 `Task` 仍报认证错 → **推送方自评**两轴（读 diff 150 行全文）：Standards 0 阻断 / 1 非阻断（新例头注陈述 compose.yaml 的 locale 是另一份文件的环境事实，可钉时点）· Spec 0 阻断 / 1 非阻断（渲染表顺序无机器比对，票 22 既有空白非本票引入）；五道判断题全同意。全文落票 26 Comments。
- **重放**：`%TEMP%\idp-replay-awf26` 在 `8313c23f` 上 pick 八笔全干净（`1254f8f0→47802a95` / `1acf5066→a70fc772` / `bd6ba2b7→637e6326` / `d7a6d4ce→72a9447d` / `f88680d5→8e33e2f9` / `814450f5→61928a5e` / `a30f6d67→e6624896` / `05a240df→1ddaa384`）；`66cad4c4..8313c23f` 只动 `.scratch/**`，零重叠；清点重生成零差。gofmt / build / vet 0；占 55432 后带 DSN 全量 **104 ok / 0 FAIL / 15 无测试 / 0 cached · 126 s**；tsc 0、run-tests 214 / 214；释号。簿记一笔在 `1ddaa384` 之上（票 26 Status + 自评 + 进 main 记录 + 本节），`ls-remote` 核 `8313c23f` 未动 → ff → `push <sha>:main`。`mcp3-awf26` 改 `merged/`、远端删；树由通道 3 拆（先 rmdir junction）。
- **接下来**：不变——等四篇 ADR（18:15）；通道 3 接 lc/28 + 立 lc/29（task-bc04bfc2）。
- **本节五数**：CI main 绿/总 未量 · 集成时延 awf/26 完工报 17:20 → 推出 17:4x **约 20 分**（1 票；推送方自评 + 全量）· 重放笔数 **8** · 重复开发 0 次 · 会话重置 0 次（本小节内）

### 17:3x–17:5x 同一会话：四篇 ADR + 29 条裁决落票面进 main → ready-for-agent 0 → 17

- **四单全在 18:15 前回**（17:36 通道 4 · 17:38 通道 5 · 17:4x 通道 6 · 17:4x 通道 2），加通道 3 的 lc/28 + 29（17:34 / 17:41）与通道 5 的 08 / 10 / 11 续笔（17:47）。**两处通道对推送方倾向的有理出入，均接受**：ADR-0136 决定四「VE 不立键登记面，实例半边落 PS `commercial_resolution_key`」（ADR-0079 决定五「两口取 = 两次解析」，比我派单里「VE 仍登五样」更贴单一权威；04-4 随之消解）；ADR-0134 决定三「06 不加 Transactor，同事务由 `RequireExecutor` 既有约束守、事务由组合根壳开」+ 三件多定（一封一包裹 / 入队失败回滚 / 两拍一类型）——通道 2 把「实施时核」量成了结论。四篇 ADR 结构逐篇核：Status 行含授权与能力边界、`## 越权风险点`（0134 六条 / 0135 六条 / 0136 五条 / 0137 六条）、Links、README 各一行。**又冒出的要裁的已即时裁完**：lc/29 四条（全取默认 + 两条补强：择优结果对象由 29 定义并产出免与 28 成环；ResponsibilityBasis 格 = 接受时回指，照 PC CONTEXT「再固定该次决定实际使用的……并保留与接受时快照的关系」原句）；sa-cc/08 两条（SA→PP 走信封不同步调用；评价请求铸造 ID + 自然键唯一）与 10 一条（三读签各挂同族页：凭证 → customs-cases、核对 / 协作 → customs-restrictions）；pc-gaps/13 三条（首发机制不给 AuthorityLevel 算序、由授权规则登记内容承担 + 越权点；PS 侧核同一账户；例外支留位）与 lc/30 一条（`AuthoritativeCutoffBoundary` = 关闭决定标识）——后两张的写入与新立 lc/32（06 `Establish` 核登记册关闭边界，硬句代码无守卫）派回通道 2 续笔（task-3ebcdc45），下一笔落。**PP 一角被量出来**：sa-cc/08 的 PP 侧消费者无先例，通道 5 立 sa-cc/11（draft，PP owner 一条）作 08 的 Blocked by。
- **重放**：`%TEMP%\idp-replay-adr` 在 `65d0ec23` 上 pick 20 笔纯 .md（通道 6 三笔 `3771a688` / `15aca5a3` / `71acc178` → 通道 4 三笔 `3e622cb6` / `e9c0894e` / `ae99e024` → 通道 5 五笔 `22b21ccf` / `7aa2f4b1` / `d7a82d7a` / `fef1ce4d` / `6e8014a2` → 通道 3 四笔 `4879f309` / `473a7a0a` / `6a7a59c1` / `221576cc` → 通道 2 五笔 `88e88283` / `a9274ff5` / `844002a2` / `bbe0ec40` / `4ad6b221`；新 SHA 见推后广播）。冲突五处全在共享索引：`docs/adr/README.md` 三次（四行各插 0133 之后，解法 = 两侧都留、按 ADR 号排）、lc spec 子票表两次（28 / 29 / 30 / 31 四行三方各加，解法 = 按行号合、行内容取改过它的那一方）。**一次误操作，已回退**：解 README 的脚本头两行用了中文注释，Windows PowerShell 按 GBK 读脚本把中文行与下一行并掉，`$text` 从未赋值、`WriteAllBytes` 写出 0 字节 README 并被 `cherry-pick --continue` 带进三笔——`git show HEAD:docs/adr/README.md` 核到 0 字符后 `reset --hard` 到前一笔重做；脚本改为 ASCII 注释 + 读空即退出 + 结果短于原文一半即退出。这正是 workflow.md「本机环境」那条坑换了一张脸：**不止 `Set-Content` 写乱码，`powershell -File` 读无 BOM 的中文脚本也会并行**。簿记笔顺手对齐 lc spec：状态行 29 → 31 张、「清单 28 张」→ 31 张、26 行 → ready-for-agent、27 行 → `30` + pc-gaps/13、28 行取回通道 3 的版本（blocked｜29）。
- **前沿（重放 tip 上重算）**：**resolved 267 / ready-for-agent 17 / draft 7 / needs-info 4 / blocked 3**。17 张 ready：lc/26 · lc/29 · lc/31 · pc-gaps/12 · sa-cc/01–10（03 等 02、07 步二等 10、08 等 11、09 等 05）· ve-claims/04 · ve-disc/02（步一）· ve-disc/03。draft 7：lc/20 / 22（等真源）、lc/27（等 30 + 13）、lc/30（等 13）、pc-gaps/13（续笔转 ready）、sa-cc/11（PP owner 一条）、ve-claims/—（无）；blocked 3：lc/28（等 29）、auto-reroute/02、first-tenant-runway/03。
- **接下来**：ff → push → 派实现首波（地盘互不重叠、非作者评审照旧）：lc/26 → 通道 2（ADR-0134 作者，PS inbox / handoff）· lc/29 → 通道 3（立票人，PS domain + adapters/partycommercial）· lc/31 → 通道 6（ADR-0135 作者，TF）· sa-cc/02 → 通道 5（SA handoff）· pc-gaps/12 → 通道 4（PC 封闭集加两格，小）。第二波候选：ve-disc/02 步一 / 03、ve-claims/04、sa-cc/05 / 04 / 06、pc-gaps/13。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因；**四篇 ADR 共 23 条越权风险点 + 票面里的另 9 条事后复核**（`scripts/owner-review-queue.ps1 -Out .scratch/owner-review-queue.md` 汇总）。
- **本节五数**：CI main 绿/总 未量 · 集成时延 —（本小节无代码票；四篇 ADR 派 17:1x → 全部到 17:4x，重放推出 17:5x）· 重放笔数 **20**（全纯 .md）· 重复开发 0 次 · 会话重置 0 次（本小节内）

### 17:5x–18:1x 同一会话：实现首波派出 → 通道 4 crash → 通道 2 续笔进 main

- **实现首波**（17:59–18:00，五单，地盘互不重叠，每单照旧隔离 worktree + 每笔推 origin + 完工报到派非作者评审）：**lc/26 → 通道 2**（task-02bb25bb，续笔之后接）· **lc/29 → 通道 3**（task-8e799b96）· **pc-gaps/12 → 通道 4**（task-eff393d5）· **sa-cc/02 → 通道 5**（task-c947d9a1）· **lc/31 → 通道 6**（task-f3f0d59a）。共享接线文件点名：`cmd/parcel-dispatch/assemble.go`（26 / 31 都可能接）、`migrations/migrations.go` + `plan.go`（31 新迁移模块四件同笔）、`operate_label_transaction.go`（26 与日后 32 同文件）。3 / 5 / 6 各自拆完 ADR 树后开工。
- **通道 4 crash**（用户 18:0x 告知，「原来的工作完成了」）：其 ADR-0136 三笔与 main 逐文件零差，`D:/tops/idp-parcel-mcp4-adr0136` 干净，推送方替它 `worktree remove`（不加 --force），`merged/mcp4-adr0136` 指针留。刚派的 pc-gaps/12 留在通道 4 队列等重启接手；不重启则待 3 / 5 / 6 任一交活改派。
- **通道 2 续笔进 main**（`6f9b701c`，18:0x 报）：pc-gaps/13 三条裁决写入 → ready-for-agent；lc/30 要裁的清零（`AuthoritativeCutoffBoundary` = 关闭决定标识）、仍 blocked by 13；新立 **lc/32**「06 `Establish` 核继续尝试登记册的关闭边界」→ ready-for-agent（Blocked by 30）——通道 2 在我裁的形上多定三件，都是 06 既有代数的照抄：用 `Judge(currentFinalPresent)` 而不只看 StandingClosure（CONTEXT「开放」定义含「无当前有效终局」）、结果代数加一格 `PARCEL_CONTINUED_ATTEMPT_CLOSED` 纯加法、重放先于核册；认可。**它如实记下一格不关的窗口**：Establish 读册与 30 的关闭 Save 各在自己事务里，窄格拦不住；CONTEXT 为此备了「关闭期间若仍发现已实际提交的边界后交易……形成违反截断边界的业务判断」——**那是后继无票**，需建立决定与关闭决定间的稳定领域顺序，归下一轮立票（候选 lc/33），不在 32。重放：lc spec 冲突一处（30 行改 + 32 行加 vs 我的对齐版）——取 main 版为底、从 `6f9b701c` 只取 30 / 32 两行；状态行与「清单」改 32 张。`mcp2-adr0134` 六笔全进，改 `merged/`、远端删。
- **前沿**：ready-for-agent **19**（上节 17 + pc-gaps/13 + lc/32），其中 5 张在做（26 / 29 / 31 / sa-cc/02 / pc-gaps/12 待通道 4 重启）。
- **本节五数**：CI main 绿/总 未量 · 集成时延 —（无代码票进 main）· 重放笔数 **1** · 重复开发 0 次 · 会话重置 **1 次**（通道 4 crash）

## 2026-09-10 18:5x 通道 1 新会话接续（接手时 `main = origin/main = dede3c2e`；用户先要一次自带 `ask_question`，再「监听队列、正常回复、循环」）

- **接手取证**（18:56，`branch-state.ps1` + `ls-remote`）：main = origin = `dede3c2e`；共享树 70 行 ` M`（与前几任同一批 CRLF 幻影）+ `d_tasks.txt` / `docs/api/` 两件 untracked，归用户，未动。在途分支四支、全部 = origin：`mcp2-lc26@0a7ea17d`（领先 9，票面 resolved）、`mcp6-lc31@579b89ab`（领先 9，票面 resolved）、`mcp3-lc29@2d19bc32`（领先 4，票面 in-progress）、`mcp5-sacc02@0ba22316`（领先 1，认领笔；树上 7 件未提交 mtime 18:04–18:09）。队列里：lc/26 完工报（通道 2，task-02bb25bb done）+ 通道 2 / 6 各自的占号 / 释号 / 作者验广播；**lc/31 作者验广播齐全但 report_task 从未到**（作者会话 18:43 推出 resolved 笔后重置）。
- **点名**（19:00，截止 19:04，目标 2–6）：19:03 前 **5 个应答全到**，全部「空闲 · 地盘无 · 余量充足」——即 2 / 3 / 4 / 5 / 6 **全是新会话、无在途记忆**（通道 2 自报「新会话，作为 lc/26 作者通道不接该票评审」）。用户 18:4x–19:0x 间重启了全部通道，本节按此处置：
  - **lc/26 评审 → 通道 6**（task-f8c78f66，20 分）；**lc/31 评审 → 通道 2**（task-21e07f77，25 分；通道 2 19:1x 报队列里没有这单——派单消息可能被它上一次重启前的会话消费，`send_to_session` 重发要点，任务记录本就在它名下）；**lc/29 接管 → 通道 3**（task-6cda77fa，同分支接着做，原单 task-8e799b96 代结 failed「承接方会话重置」）；**sa-cc/02 接管 → 通道 5**（task-eaecfdd6，第 1 步原样封存 7 件成笔再续做，原单 task-c947d9a1 代结 failed）；**pc-gaps/12 → 通道 4**（原单 task-eff393d5 仍 pending，`send_to_session` 让它照原单开工、只把基线改成当前 main；19:1x 通道 4 report working 认领，分支 `mcp4-pcgaps12`）；**lc/31 原单 task-f3f0d59a 推送方代结 done**（完工以分支为准，写明作者未报的原因）。
- **lc/26 重放**（与评审并行，隔离 detached 树 `%TEMP%\idp-replay-lc26`）：`76932b38..dede3c2e` 只动 `.scratch/**`、与本票文件零重叠 → 在 `dede3c2e` 上 pick 八笔全干净（`93842381→bd06b91a` / `f172a972→45fa80d9` / `8a76c6bb→1a236fd3` / `fea57cea→2a52355d` / `63581373→4333543a` / `bc0e36df→b3b06226` / `d6d50e30→81c0598d` / `0a7ea17d→009ec866`）；作者清点笔 `f8013a36` 不带，tip 重生成 `83ab2032`（与作者那份逐字节同）；本票文件对分支 tip 零差。**验证钉 `83ab2032`**：gofmt 空、build / vet 0；占 55432 广播后带 DSN `go test -p 1 -count=1 ./...` **105 ok / 0 FAIL / 15 无测试 / 0 cached**（19:01:41→19:03:42，121 s；`1c48e4fe` 那笔写 106，把随后探针那一行 `ok` 也数进去了，19:5x 按全量段单独重数改正——三跑同法重数都是 105）；探针 PS postgres `-run LabelTransactionJudgment -v` PASS 2 / SKIP 0；19:04 释号。
- **lc/26 评审到**（通道 6，19:1x，钉 `0a7ea17d`，评审侧五包 + postgres 探针带 DSN 全 PASS）：**Standards 0 阻断 / 4 非阻断**（跨文件计数进注释三处；`parcelFinalAdoptionChain` 头注理由与代码不一致；`judgmentBeat` 取 `saved.Revision()+1` 把仓储 `Save` 的「+1」抬进应用层；`NewLabelTransactionHandler` 不校验依赖到写时才响亮）· **Spec 0 阻断 / 2 非阻断**（共享文件重排超出票面地盘字面但逐步等价成立；`labelfinal.ErrParcelTargetNotFound` 与 `pstf.` 那只是两个哨兵同一恢复格）；重点核 ①–⑦ 属实、多定四件全同意。全文由推送方代落票 26 Comments。
- **进 main**：簿记一笔在 `83ab2032` 之上（票 26 Status + 评审 + 进 main 记录、lc spec 26 行 → resolved、本节）；`ls-remote` 核 `dede3c2e` 未动 → ff → `push <sha>:main`。`mcp2-lc26` 改 `merged/`、远端删；树 `D:/tops/idp-parcel-mcp2-lc26` 干净、通道 2 已是新会话 → 推送方比内容后拆；`idp-replay-lc26` 推完拆。
- **lc/31 预演**（隔离 detached 树 `%TEMP%\idp-replay-lc31`，叠在 `83ab2032` 之上）：代码七笔 + 认领笔全干净（`internal/architecture/partition_subject_registry_test.go` 26 / 31 两行分属 PS / TF 两段，git 自动合、gofmt 对齐后仍空）；票面笔 `579b89ab` 在 lc spec 子票表撞 `dede3c2e` 的对齐版——取 main 版为底、31 行取分支版，其余行不动；作者清点笔 `27bb2dc1` 不带，tip 重生成 `58480cf2`（对作者那份只差 lc/26 的 8 行）。带 DSN 全量在跑（19:08 占号），评审（通道 2）无阻断后 rebase 到 lc/26 落地的 main tip 再 ff。
- **接下来**：lc/31 评审 ≤ 19:40 → 重放 → ff；lc/29（通道 3）/ sa-cc/02（通道 5）/ pc-gaps/12（通道 4）完工报到后各派非作者评审（此刻能评的：6 空下来后、2 评完 31 后）。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因（本节 2–6 全重启一次）；四篇 ADR 越权风险点事后复核；lc/26 评审非阻断六条是否攒小票。
- **本节五数**：CI main 绿/总 未量 · 集成时延 lc/26 完工报 18:4x → 推出 19:1x **约 30 分**（1 票；其中等点名与重启后的通道到位约 10 分、评审 12 分与重放 + 全量并行）· 重放笔数 **8**（推送方清点 1 另计）· 重复开发 0 次 · 会话重置 **6 次**（通道 1 本会话新起 + 2 / 3 / 4 / 5 / 6 各一次，点名自报）

### 19:2x–19:30 同一会话：lc/29 完工 → 通道 6 评审；通道 2 评 lc/31 到中间态即 crash → 以中间态为评审结论进 main（本小节与下一小节的时刻在 19:4x 按 git 提交时刻校准过：原写 19:5x / 20:0x 偏快 20–30 分）

- **通道 2 的 lc/31 评审**：19:1x 通道 2 报「队列里没有这单」（派单消息被它重启前的会话消费），`send_to_session` 重发要点后 19:2x 认领开工；**19:2x 交中间态**（评审自标 19:37，与本机 `Get-Date` 不合，按到达顺序在 19:30 那笔提交之前）：主评已核完领域 / 迁移 0020 / application / postgres / http / cmd 装配六处，**无阻断**，Spec 非阻断两条语义格（待确认 → 已形成那一步不再问首次判据；轨迹来源自动派生的 `corrects` 在链不存在时一律 `BASIS_NOT_CURRENT`，首条被判版本若本身是更正代则永远形成不了收寄）、Standards 非阻断两条（`rederive` 未用形参、中间笔不可编），只剩评审侧自己的测试 pass 在跑。**19:2x 用户告知通道 2 crash**（「它没有什么工作的吧」——它正在评 lc/31，终报没到）。推送方裁：中间态已覆盖全部待评文件且明写无阻断，缺的那一半（评审侧测试）本就不是评审义务，**以中间态为评审结论**；两条 Spec 非阻断推送方对着 `Judge` / `resolveBasis` 核过属实、都是保守失败（拒绝而非误形成），不挡合入，归作者另立后继票。全文代落票 31 Comments；task-21e07f77 代结 done。
- **lc/31 进 main**：预演链（`58480cf2`，见上节）在 lc/26 落地后 `rebase --onto 1c48e4fe 83ab2032` 零冲突 → `037d88b8`，代码树对 `58480cf2` 逐字节同：`0b89f99e→51d5f6d9` / `5b6ce319→887a5f2e` / `14608699→9286562e` / `55bfdf15→0c76fc66` / `8795a1b6→4ae53900` / `ce8afd68→788c6473` / `1d9a24c0→d79b65a6` / `579b89ab→0e5ef0ce` / 清点 `037d88b8`。**验证钉 `58480cf2`**：gofmt 空、build / vet 0；带 DSN 全量 **105 ok / 0 FAIL / 15 无测试 / 0 cached**（19:10:22→19:12:29，127 s）；`platform/migrate` -v 五例 PASS；TF postgres 链用例 -v PASS 2 / SKIP 0（在 lc/29 预演那跑上量，同一代码树）。簿记一笔在 `037d88b8` 之上（票 31 Status + 评审 + 进 main 记录、票 26 与本节 106 → 105 改正、本小节）；`ls-remote` 核 `1c48e4fe` 未动 → ff → `push <sha>:main`。`mcp6-lc31` 改 `merged/`、远端删；树 `D:/tops/idp-parcel-mcp6-lc31` 干净、通道 6 已是新会话 → 推送方比内容后拆；`idp-replay-lc31` 推完拆。
- **lc/29 完工报 19:2x**（通道 3 接管单 task-6cda77fa done：分支 tip `b1d8bc40`、代码 tip `2d19bc32` 一行未改，清点 `03ce7c1b`，票面 resolved + 完成记录；判据 1–7 先写期望再读全部对上；带 DSN PS 19 包 + cmd/* 11 包 + 四反向依赖 + architecture 全 ok，-v 下 PS postgres 168 / partycommercial 195 / domain 705 / application 325 / parcelpricing 16 / cmd 三包 67 / 146 / 93，SKIP 全 0；三道判断题：`AuthorizationFor` 交回 `ChannelAccountUseSelection{Authorization, Grantee}`、停点比票面多四格、`ErrUntranslatableAnswer` 六处无用例能走到）。**评审派通道 6**（task-0509b519，20 分；派单里写的截止 20:10 是按偏快的时钟算的，实际给了约 45 分）。**预演**：`%TEMP%\idp-replay-lc29` 叠在 lc/31 链 `037d88b8` 上 pick 五笔全干净（`68c3e72a` / `18f79dda` / `944e97b7` / `2d19bc32` / `b1d8bc40`），清点笔 `03ce7c1b` 不带、tip 重生成 `85b5898b`（5/5）；本票文件对分支 tip 零差。**验证钉 `85b5898b`**：gofmt 空、build / vet 0；带 DSN 全量 **105 ok / 0 FAIL / 15 无测试 / 0 cached**（19:24:19→19:26:25，126 s）；PS adapters/partycommercial -v PASS 189 / SKIP 0。评审无阻断后 rebase 到 lc/31 落地的 main tip 再 ff。
- **在途**：pc-gaps/12（通道 4，作者验 19:3x 占号中）、sa-cc/02（通道 5，作者验 20 包 ok，收尾中）。通道 2 crash 后无在途（评审已按中间态收）。

### 19:3x 同一会话：lc/29 评审无阻断 → 进 main（19:34）

- **lc/29 评审到**（通道 6，19:3x，钉 `b1d8bc40`，评审侧五包无 DSN ok）：**Standards 0 阻断 / 2 非阻断**（`NewChannelSelectionBasisTranslator` 必填的两个 PC 读口不在构造期拒 nil；`authorizationOf` / `agreementOf` 自分「已撤销 / 不在有效期」的边界谓词与 PC 内部那句是第二份）· **Spec 0 阻断 / 3 非阻断**（`cost_bridge.go` 超地盘字面但有裁决 (3) 支撑；三处 `deps.X == nil` 分支无用例；`Handle` → `Select` 窄面等价核过）；判据 1–7 逐项 ✓、三道判断题全同意、三条越权风险点字面内。全文代落票 29 Comments。
- **进 main**：预演链 `85b5898b` 在 lc/31 落地后 `rebase --onto 03474d87 037d88b8` 零冲突 → `58c59daf`，代码树对 `85b5898b` 逐字节同：`68c3e72a→df6d486b` / `18f79dda→2e8b7f48` / `944e97b7→74ee7ca9` / `2d19bc32→e53db985` / `b1d8bc40→50910105` / 清点 `58c59daf`。验证钉 `85b5898b`（见上小节）。簿记一笔在 `58c59daf` 之上（票 29 Status + 评审 + 进 main 记录、lc spec 29 行 → resolved 并注明 `28` 解除阻塞、本小节）；`ls-remote` 核 `03474d87` 未动 → ff → `push <sha>:main`。`mcp3-lc29` 改 `merged/`、远端删；树 `D:/tops/idp-parcel-mcp3-lc29` 归通道 3 拆（在线）。
- **前沿变化**：lc/26 / 29 / 31 三张今晚进 main；**lc/28**（择优组合根）Blocked by 29 解除 → 可转 ready-for-agent 派实现（它接 `SelectedChannelBasis`，前置步 `Select` → `TranslateSelectedCandidate`，三源装 nil；同时装 `LabelTransactionDeps` 与事务壳，lc/26 诚实句那格由它补上）；**lc/25**（PS 半边）Blocked by 31 解除，票面已回填。候选后继票：lc/31 评审两条 Spec 语义格（归通道 6 立）；lc/26 评审非阻断六条 + lc/29 评审非阻断五条（可攒一张 lc 小票或随 lc/28 顺手修）。
- **本节五数**（19:15–19:34 段，按 git 提交时刻）：CI main 绿/总 未量 · 集成时延 lc/31 完工（分支 18:43 推出、未报）→ 推出 19:30 **约 47 分**（含全通道重启后重派评审与评审方 crash）、lc/29 完工报 19:2x → 推出 19:34 **约 12 分**；两票中位 **约 30 分** · 重放笔数 **13**（lc/31 八笔 + lc/29 五笔；推送方清点 2 另计）· 重复开发 0 次 · 会话重置 **1 次**（通道 2 于 19:2x crash，本段内）

### 19:3x–19:4x 同一会话：lc/28 派出；sa-cc/02 与 pc-gaps/12 完工 → 评审交叉派 6 / 5 → sa-cc/02 进 main

- **lc/28 → 通道 3**（19:3x，task-049e6dcd，分支 `mcp3-lc28v2` 基 `32778486`，估 90 分）：29 进 main 即解阻，票面「裁决」乙 + 形状约束照旧；派单点名了 lc/26 评审 Standards 4（组合根断言 `Judgments` 非 nil）与 lc/26 诚实句（`LabelTransactionHandler` 的事务壳与生产调用方由本票补）；`operate_label_transaction.go` 与日后 lc/32 同文件、动前占号。
- **sa-cc/02 完工报 19:3x**（通道 5 接管单 task-eaecfdd6 done：分支 tip `c8812ea1`、代码 tip `1af6a226`；封存笔 `43f7c3e4` 判「用」+ 接管方补并发重放用例 `1af6a226` + 清点 `0384f10d` + 票面；带 DSN 20 包 ok、四包 -v PASS 503 / SKIP 0；cmd/ 下无 `MapExternalFundsHandler` 装配点故不新增壳）。**评审派通道 6**（task-b04b728c）。**预演**：`%TEMP%\idp-replay-sacc02` 在 `32778486` 上 pick 四笔全干净（`0ba22316→affb20f6` / `43f7c3e4→8c7b893e` / `1af6a226→47600539` / `c8812ea1→b8f3fd75`），`partition_subject_registry_test.go` 对 main 恰 +1 SA 行；清点笔 `0384f10d` 不带、tip 重生成 `cdd6504c`。**验证钉 `cdd6504c`**：gofmt 空、build / vet 0；带 DSN 全量 **105 ok / 0 FAIL / 15 无测试 / 0 cached**（19:41:16→19:43:26，130 s）。
- **pc-gaps/12 完工报 19:4x**（通道 4 task-eff393d5 done：分支 tip `0051a246`、代码 tip `c5afe453`，基 `dede3c2e`，九笔：CONTEXT 一句 → 领域两值 + 不换号摘要字面钉基线 → 迁移 0031 放宽 CHECK + 库读口反查改走领域 → PS 适配器逐格翻译、删「今天没有行」短接 → `cmd/parcel-commercial/translate.go` 反查改走领域 → 清点 → 自评三处 → 票面；带 DSN 27 包 26 ok / 1 无用例、-v PASS 3276 / SKIP 0；三道判断题）。**评审派通道 5**（task-e6dc2cc6，非作者）。**预演**：`%TEMP%\idp-replay-pcgaps12` 叠在 sa-cc/02 链 `cdd6504c` 上 pick 八笔全干净（清点笔 `1675f830` 不带），tip 重生成 `392fedcb`（PC 迁移 30→31）；本票文件对分支 tip 零差；带 DSN 全量在跑。
- **sa-cc/02 评审到**（通道 6，19:4x，钉 `c8812ea1`，封存笔七件逐读；评审侧 SA 全包 + parcel-dispatch + architecture 带 DSN ok）：**Standards 0 阻断 / 3 非阻断**（`MapExternalFundsDeps.FactHandoff` 必填口不校验、漏装即 panic——潜伏，归 mech/06 SA 在线面那笔；`handOffFact` / `existingFact` 与既有一对同形不抽；`ExternalFundsFactIntent` 头注偏长）· **Spec 0 阻断 / 3 非阻断**（**推送方派单【重点核】③ 写「入队失败整步回滚」是写错的**——票面原句是「版本已落、意图未入队 → 技术未形成 + 续办引用」，代码按票面且与同文件既有形一致，与 PS lc/26 的 ADR-0134 回滚形是两个上下文各自的选择；「重放不交」在编排层只在观察面成立、真库那格才证不翻倍；`cmd/parcel-api` 装配处经核不存在、票面写实）；六道判断题全同意。全文代落票 02 Comments。
- **sa-cc/02 进 main**：簿记一笔在 `cdd6504c` 之上（票 02 Status + 评审 + 进 main 记录；sa-cc spec 02 行注明进 main、03 行 Blocked by 清零；票 03 Blocked by 行改「无（02 已进 main）」并给信封形状；票 29 / 31 与本节上两小节的时刻按 git 提交时刻改正；本小节）；`ls-remote` 核 `32778486` 未动 → ff → `push <sha>:main`。`mcp5-sacc02` 改 `merged/`、远端删；树 `D:/tops/idp-parcel-mcp5-sacc02` 归通道 5 拆（在线，评 pc-gaps/12 中）；`idp-replay-sacc02` 推完拆。pc-gaps/12 链随后 `rebase --onto` 新 main。
- **前沿变化**：**sa-cc/03**（CC inbox 消费者收资金事实信封）Blocked by 02 解除、ready-for-agent 可派（CC 地盘；PP 无关）。候选后继：sa-cc/02 评审 Standards 1（`FactHandoff` 构造期断言）随 mech/06 或另一笔小改。

### 19:5x 同一会话：pc-gaps/12 评审无阻断 → 进 main（今晚第五张）

- **pc-gaps/12 评审到**（通道 5，19:5x，钉 `0051a246`，读全部 18 文件 diff；评审侧七包无 DSN ok，PC postgres 真库依作者 + 推送方全量）：**Standards 0 阻断 / 3 非阻断**（PS application 测试 Covers 注释引了不存在的字面 `FINAL_RULE_NOT_SATISFIED_YET`（真字面无 `_YET`）；票面判断题 (c) 先例误引 `0026`、真先例是 `0017` / `0019` / `0025`；`translate.go` 与库读口反查同形一行包、接受）· **Spec 0 阻断 / 2 非阻断**（判据 7 前端半边是代码取证非运行验；判据 5 应用层无「已登缺行 → 未决」用例、只在适配器层）；**不换号由评审在 `dede3c2e` 临时检出上独立算出同一串**；三道判断题全同意（(b) 换格正确、PS 文案不必跟；(c) 不算改已施加迁移，checksum 门按文件字节）。全文代落票 12 Comments。
- **进 main**：预演链 `392fedcb`（叠在 sa-cc/02 链上，见上小节；验证钉它：带 DSN 全量 105 ok / 0 FAIL / 0 cached，19:45:08→19:47:14；PC postgres + migrate -v PASS 278 / SKIP 0）在 sa-cc/02 落地后 `rebase --onto 5f70a985 cdd6504c` 零冲突 → `5635ba08`，代码树逐字节同：`ba6e823f→ac07aa87` / `544bad5f→fdf0de17` / `366a7178→b10f672f` / `217d153c→3b3d9544` / `e54f8d3d→26c55cac` / `876831b7→177df226` / `c5afe453→be2eb21d` / `0051a246→0d09d849` / 清点 `5635ba08`。簿记一笔在 `5635ba08` 之上（票 12 Status + 评审 + 进 main 记录、本小节）；`ls-remote` 核 `5f70a985` 未动 → ff → `push <sha>:main`。`mcp4-pcgaps12` 改 `merged/`、远端删；树 `D:/tops/idp-parcel-mcp4-pcgaps12` 归通道 4 拆（在线）；`idp-replay-pcgaps12` 推完拆。
- **通道 6 两次「释号补记」**：其 lc/26 评审包组（19:07:59–19:08:18）与 sa-cc/02 评审包组（19:41:16–19:41:41）各与推送方一跑重叠十几秒——占号广播在它交报告后才被读到，是「占号广播拦不拦得住人取决于收的人什么时候读」那一格；两跑都全绿，无偶发红。每用例独立物理库，重叠只影响时长不影响结果。
- **收口状态**：首波五单全部进 main——lc/26（19:15）、lc/31（19:30）、lc/29（19:34）、sa-cc/02（19:4x）、pc-gaps/12（19:5x）；在途只剩 **lc/28**（通道 3，估 21:1x 前）。空闲：4 / 5 / 6（2 crash 无会话）。**可派**：sa-cc/03（CC inbox 消费者，ready）、sa-cc/05 / 04 / 06（CC，ready）、ve-disc/02 步一 / 03、ve-claims/04、pc-gaps/13；lc/31 两条 Spec 语义格后继票归通道 6 立；lc/26 + lc/29 评审非阻断攒小票。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因（本节 2–6 全重启 + 通道 2 再 crash）；四篇 ADR 越权风险点事后复核；pc-gaps/12 判断题 (b) PS 文案是否跟（评审判不必）。
- **本节五数**（19:4x–19:5x 段，按 git 提交时刻）：CI main 绿/总 未量 · 集成时延 sa-cc/02 完工报 19:3x → 推出 19:4x **约 15 分**、pc-gaps/12 完工报 19:4x → 推出 19:5x **约 15 分**；两票中位 **约 15 分** · 重放笔数 **12**（sa-cc/02 四笔 + pc-gaps/12 八笔；推送方清点 2 另计）· 重复开发 0 次 · 会话重置 0 次（本段内）

### 20:0x 同一会话：第二波三单派出（4 / 5 / 6），在途四张

- 派单第 0 步：`branch-state.ps1` 在途分支**零**（首波五支全改 merged/），无半成品可撞；4 / 5 / 6 各在 15 分内交过评审或完工报、无在途票，未再点名，派单末尾各留「若已在做别的事回一句我改派」。基线一律远端 main `b40b1804`。
  - **pc-gaps/13 → 通道 4**（task-46cd2d7a，`mcp4-pcgaps13`）：PC 授权动作加「受控关闭」「重开」两格，三条裁决在票面；**迁移序号重取为 `0032`**（票面写的 0031 已被 pc-gaps/12 用掉）。地盘 PC domain / postgres / 迁移 / `cmd/parcel-commercial/translate.go` / CONTEXT 一句；不动 PS。估 45–60 分。
  - **sa-cc/03 → 通道 5**（task-764b20a1，`mcp5-sacc03`）：CC inbox 消费者收资金事实信封 → `ReceiveFundsFact`；按裁决在 SA `ports` 补按键只读视图（取信封所指那一版）、CC 消费侧适配器、`cmd/parcel-dispatch` 路由一行（占号）、CONTEXT-MAP 一条边。估 60 分。
  - **lc/33 立票 + 实现 → 通道 6**（task-ea5ac39e，`mcp6-lc33`；号开工时重取）：lc/31 评审两条 Spec 语义格的后继（待确认 → 已形成补问首次判据；自动派生 `corrects` 在链不存在时视作首次判断），作者自立自做，地盘只 `judge_carrier_first_effective_pickup*.go`；碰 ADR-0135 决定句就写「要裁的」报我当场裁。估 30–45 分。
- **在途四张**：lc/28（3，19:3x 派，估 21:1x）、pc-gaps/13（4）、sa-cc/03（5）、lc/33（6）。**无空闲评审者**（2 crash 无会话）——完工报到时若四家都在忙，按规矩推送方用 `/code-review` 隔离子代理评或自评（非作者）；预计第一张完工在 20:4x。**归用户**（不变）：CI 计费；两件 untracked；会话中断成因；四篇 ADR 越权风险点事后复核；lc/26 + lc/29 评审非阻断攒小票；pc-gaps/12 判断题 (b) PS 文案。

### 20:0x–20:2x 同一会话（本机时钟；各通道自标 21:xx 偏快约一小时）：通道 3 / 6 又 crash；pc-gaps/12 随后小笔进 main；sa-cc/03 一条 B 类当场裁；pc-gaps/13 与 lc/33 完工 → 交叉评审 → lc/33 进 main

- **pc-gaps/12 随后小笔**（通道 4，`mcp4-pcgaps12-followup@d59767ee`：Covers 注释字面串改符号名 + 票面判断题 (c) 先例更正）：纯注释 + .md，推送方自审（gofmt / vet 0、PS application 包 ok），`d59767ee→0e575072` 直接进 main（20:0x）；分支改 merged/、远端删。
- **用户 20:1x 告知通道 3 crash**：核过它从未开工 lc/28（origin 无 `mcp3-lc28v2`、无树、无半成品）；task-049e6dcd 仍 pending，给通道 3 下一会话留言「认领前先 report working、若已改派就不接」；先空下来的通道优先接。推送方拆了它的 `merged/mcp3-lc29` 树（干净）。
- **sa-cc/03 要裁的（通道 5 20:1x 报）**：CC `ExternalFundsFactRegistration.Payer` 在 SA 事实里没有来源（SA domain / 表 / 采用命令均无付款人，CC 拒空 + NOT NULL）。**推送方裁 A：SA 外部资金事实携带来源提供的付款人，可缺席（显式「来源未提供」，列 nullable）**——口径：CC CONTEXT 把付款人列为「来源提供或真实程序要求」的维度，是事实自带属性；ADR-0137 决定四单一铸造口在 SA，SA 不登 CC 就永远拿不到；「来源提供」故 SA 不得因缺席拒采用；与 SA「付款方身份」（代垫判断输入）不同名不同义。CC 侧不动（无付款人 → 未受理是诚实停点）；CC 放宽（「来源未提供且程序不要求的维度要明确记录」）另立 sa-cc 新票 draft 归 CC owner。越权风险点两条入票 03。通道 5 据此在 SA 加 `FundsPayerReference` + 迁移 SA 0018 `payer_ref NULL` + 只读视图 `AdoptedFundsFactView`（`84da8df5`，SA postgres + migrate 带 DSN PASS 134 / SKIP 0）。
- **用户 20:2x 告知通道 6 crash**（「工作完成了，等通道 4 释号期间先跑不带 DSN 的包组，并准备完成记录」）：`mcp6-lc33@2ed45954` 三笔已在 origin、树干净；票面无完成记录。推送方预演到 main `0e575072`（三笔全干净，清点零差，tip `1d1d405c`），**验证钉 `1d1d405c`**：gofmt 0 / build 0 / vet 0；带 DSN 全量 **105 ok / 0 FAIL / 15 无测试 / 0 cached**（20:18:37→20:20:42）；TF application -v PASS 408 / SKIP 0。
- **pc-gaps/13 完工报**（通道 4，20:1x 到，task-46cd2d7a done：分支 tip `61008b80`、代码 tip `80cb3ab8`（Go / SQL 与 `a02cad7f` 逐字节同），八笔；带 DSN 25 包 24 ok / 1 无用例 / -v PASS 2316 / SKIP 0；spec 收口 13 张全 resolved；三道判断题 (a)(b)(c)）。**评审**：无空闲非作者通道（5 忙、2 / 3 / 6 crash）→ 按规矩起两个隔离只读子代理（Standards / Spec 各一，隔离检出 `%TEMP%\idp-review-pcg13@61008b80`），结果回来落票面。**预演**：`%TEMP%\idp-replay-pcg13` 叠在 lc/33 链 `1d1d405c` 上 pick 七笔全干净（清点笔 `8a27c7d5` 不带，tip 重生成 `537171bb`：PC 迁移 31→32）；**验证钉 `537171bb`**：gofmt 0 / build 0 / vet 0；带 DSN 全量 test_exit 0（20:21:53→20:24:00）；PC postgres + migrate -v PASS 278 / SKIP 0。
- **lc/33 评审 → 通道 4**（task-dbb45a16，20:1x 派，20:2x 交）：**Standards 0 阻断 / 2 非阻断**（一处恒真条件可无条件调用；**行为位移未在票面点名**——`reconsiderPending` 先问首次判据，「链尾待确认 + 在控 + 身份仍未在册」从旧 `PICKUP_PENDING` 变 `NOT_FIRST`，两者都不落版本）· **Spec 0 阻断 / 1 非阻断**（完成判据 5 缺——作者 crash）；三入口都补、`correctsDerived` 区分在字段上成立、决定六原句未动、显式指名优先。**lc/33 进 main**：完成记录由推送方代写（分支三笔、判据逐项、能力边界「红未单独可见、判断题没来得及写」、行为位移补记）、评审 + 进 main 记录、lc spec 33 行 → resolved、本小节；`cca9dca1→27695ab6` / `a55dbedc→2e3405ab` / `2ed45954→1d1d405c`；`ls-remote` 核 `0e575072` 未动 → ff → `push <sha>:main`。`mcp6-lc33` 改 `merged/`、远端删；树 `D:/tops/idp-parcel-mcp6-lc33` 干净、通道 6 无会话 → 推送方拆；`idp-replay-lc33` 推完拆。pc-gaps/13 链随后 `rebase --onto` 新 main，等两个子代理的评审。
- **在途**：sa-cc/03（5，作者验 20:2x 占号中）；lc/28 待改派（3 无会话）；pc-gaps/13 等子代理评审。空闲：4。

### 20:3x 同一会话：lc/28 改派通道 4；两个评审子代理鉴权错 → 推送方自评 pc-gaps/13 → 进 main；lc/30 转 ready

- **lc/28 改派**：task-049e6dcd（通道 3，从未开工即 crash）代结 failed，新单 **task-03394bb6 → 通道 4**（`mcp4-lc28` 基 `53173062`，估 90 分），20:3x 通道 4 认领。派单里点名 lc/29 评审的「必填两口构造期不拒 nil、装配时必须真装上」与 lc/26 的 `Judgments` 断言非 nil、事务壳由组合根开。
- **pc-gaps/13 评审**：两个隔离只读子代理都在第一步 `Authentication error` 死掉（09-08 20:3x 与今天 sa-cc/02 作者自报的同一 harness 症状——**三次里两次死、一次成**，不能当默认路径）。推送方按规矩自评（非作者）：读完生产 diff 与三份测试的用例名单，grep 证 `AuthorityLevel` 零比较、`0003` / `0025` 零改动、`contract_delegation` CHECK 未扩、`resolveDecider` 的客户拒绝确在 `grant.permits` 命中之后。**Standards 0 阻断 / 2 非阻断（判断：同一枚举两个谓词各答一问不并；「各加过一次」会变旧）· Spec 0 阻断 / 0 非阻断**，判据 1–7 逐项 ✓；三道判断题：(a) 同意现状（红线要求）、(b) 同意另立票归 PC owner、(c) 留。全文落票 13 Comments。
- **pc-gaps/13 进 main**：预演链 `537171bb`（验证钉它：带 DSN 全量 105 ok / 0 FAIL / 0 cached，20:21:53→20:24:00；PC postgres + migrate -v PASS 278 / SKIP 0）在 lc/33 落地后 `rebase --onto 53173062 1d1d405c` 零冲突 → `fd55e1fb`，代码树逐字节同：`7251de45→d44b8999` / `a68318cd→3fc306bd` / `3b738658→17ad88d7` / `29b487d5→ac48afd7` / `a02cad7f→7d35de84` / `80cb3ab8→dfd08f03` / `61008b80→cd43bcf7` / 清点 `fd55e1fb`。簿记一笔在 `fd55e1fb` 之上（票 13 Status + 自评 + 进 main 记录；**lc/30 → ready-for-agent**（票 30 Status / Blocked by + lc spec 30 行——pc-gaps/13 落地即转，Status 行原句）；本小节）；`ls-remote` 核 `53173062` 未动 → ff → `push <sha>:main`。`mcp4-pcgaps13` 改 `merged/`、远端删；树 `D:/tops/idp-parcel-mcp4-pcgaps13` 归通道 4 拆（在线，做 lc/28）；`idp-replay-pcg13` / `idp-review-pcg13` 推完拆。
- **今晚进 main 七张**：lc/26 · lc/31 · lc/29 · sa-cc/02 · pc-gaps/12（+ 随后小笔）· lc/33 · pc-gaps/13。**前沿**：lc/30（PS 关闭 / 重开写面）ready 可派；lc/32 Blocked by 30；sa-cc/03 在途（5）；lc/28 在途（4）。**归用户**（累计）：CI 计费；两件 untracked；会话中断成因（今晚 2 / 3 / 6 各 crash 一到两次，全在写完成记录或评审终报前——**完成记录写在代码笔之后、单独成笔，是这一晚三次丢报告的共同形**，建议下一版派单把「每笔推完先 report working 一句」写成硬要求）；四篇 ADR 越权风险点事后复核 + pc-gaps/13 两条（`AuthorityLevel` 定序、既有两格对客户请求方不对称）；子代理评审不可依赖。
- **本节五数**（20:0x–20:3x 段，本机时钟）：CI main 绿/总 未量 · 集成时延 lc/33 完工（分支 20:11 推出、未报）→ 推出 20:2x **约 15 分**、pc-gaps/13 完工报 20:1x → 推出 20:3x **约 20 分**；两票中位 **约 18 分** · 重放笔数 **10**（lc/33 三笔 + pc-gaps/13 七笔；推送方清点 1、随后小笔 1 另计）· 重复开发 0 次 · 会话重置 **2 次**（通道 3、通道 6 各 crash 一次，本段内）

## 2026-09-10 21:1x 通道 1 新会话接续（接手时 `main = origin/main = 9220ab06`；用户先要一次自带 `ask_question`，再「监听队列」→「这些也都完成可以重放了吗：`D:\tops\idp-parcel-mcp4-lc28` / `-mcp4-rework` / `-mcp5-sacc03`」）

- **接手取证**（21:1x，git，不读自报）：`ls-remote` main = `9220ab06` = 本地 main；共享树 70 行 ` M`（CRLF 幻影）+ `d_tasks.txt` / `docs/api/` 两件 untracked 归用户、未动。三棵被问的树：`D:/tops/idp-parcel-mcp4-lc28 @ a35bc375` = `origin/mcp4-lc28`，干净，票面 resolved（作者 20:58 推出）；`D:/tops/idp-parcel-mcp5-sacc03 @ b2b174b0` = `origin/mcp5-sacc03`，干净，票面 resolved（作者 20:30 推出）；**`D:\tops\idp-parcel-mcp4-rework` 是空壳目录**——0 件文件、无 `.git` 链接、不在 `git worktree list`，分支 `merged/mcp4-rework@091e5398`（14:42）早已 ABSORBED，**无物可重放**，本节收尾时删壳。`%TEMP%` 里三棵 detached：`idp-replay-sacc03@343b997b`（通道 1 前会话 20:39 把四笔叠到 `9220ab06` + 清点，随后中断——评审没派、台账没记）、`idp-review-sacc03@b2b174b0`（干净）、`idp-replay-lc28@83124308`（通道 4 前身兼推送方 20:59 把七笔叠到同一个 `9220ab06`，跑过一次**无头**全量 105 ok——哪个 SHA、DSN 设否日志证不出，不计；随后 crash，通道 4 21:2x 直信答清三件）。队列里只有通道 4 前身的「占 55432」广播，到达时已过时。
- **重放核对**（不重做，核过再用）：sa-cc/03 23 件 `git diff b2b174b0 343b997b -- <files>` 只差清点 4 行；lc/28 11 件 `git diff a35bc375 83124308 -- <files>` 只差清点 + main 自己改的 lc spec 30 行（`53173062..9220ab06` 与本票同文件只有这两件）；两条链只在生成物 `MECHANISM-INVENTORY.md` 上重叠 → 顺序叠、一次推。
- **叫停 + 点名**：21:19 直信通道 4「不 ff / 不 push——你是 lc/28 作者，自评不算，进 main 由通道 1 做」，21:2x 通道 4 答「不动 main」并空闲。21:19 点名（截止 21:23）：2 / 3 / 5 / 6 四应答全「空闲 · 地盘无 · 余量充足」；4 截止后到（不算认领）。派：**sa-cc/03 评审 → 通道 6**（task-5fa114f6，作者 5）、**lc/28 评审 → 通道 3**（task-220c3eb5，作者 4），各 20 分、不带 DSN；2 / 4 / 5 空闲待命**未派**——用户本轮只问重放，不扩范围。
- **链 A**（sa-cc/03，`343b997b`）：`gofmt -l` 空、build / vet 0；21:25 占 55432 广播后带 DSN `go test -p 1 -count=1 ./...` **107 ok / 0 FAIL / 15 无测试 / 0 cached**（21:25:41→21:27:31，110 s）——比今晚此前各跑的 105 多两包，正是 sa-cc/03 新增的 `customscompliance/adapters/inbox` 与 `adapters/settlementaccounting`。
- **链 B**（lc/28 叠在 A 上）：在 `idp-replay-lc28` 树 `checkout --detach 343b997b`，`cherry-pick 9220ab06..75a15059` 六笔零冲突 → `002d3135`；tip 重生成清点 → **`784ad076`**（作者清点 `3c50706a` 与通道 4 前身清点 `83124308` 都不带）。lc/28 Go 文件对分支 tip 零差；链 B 对 `83124308` 除清点外恰为 sa-cc/03 那 22 件。`gofmt -l` 空、build / vet 0；带 DSN 全量 **107 ok / 0 FAIL / 15 无测试 / 0 cached**（21:27:45→21:29:38，113 s）；`-v` 探针：SA postgres `-run AdoptedFundsFact` PASS 3 / SKIP 0、`cmd/parcel-api -run LabelChannelChain` PASS 2 / SKIP 0（lc/28 两条真库用例在推送方这跑实跑）、CC inbox PASS 5 / SKIP 0；21:31 释号。日志 `%TEMP%\sacc03-full-343b997b.log` / `%TEMP%\lc28-chain-full-784ad076.log`，仓内无残留。
- **评审到**：**sa-cc/03 ← 通道 6**（21:3x，钉 `b2b174b0`）：Standards 0 阻断 / 5 非阻断（最重 `adoptDigest` 加付款人对存量摘要不兼容、靠 0018「存量零」兜住）· Spec 0 阻断 / 3 非阻断（最重：更正版本到 CC 侧落成 `内容冲突`，CC 登记无版本维，建议另立 CC 票与 12 并列）；七道判断题全同意。**lc/28 ← 通道 3**（21:3x，钉 `a35bc375`）：Standards 0 阻断 / 4 非阻断（最重：并列 / 无人参选那对分类在三处各写一遍，根因 `Select` 把业务答案当 error 交回）· Spec 0 阻断 / 3 非阻断（最重：`buildLabelChannelOrchestration` 无生产调用方而完成记录写「生产可达」）；判断题 (a)(c)(d) 同意、**(b) 不同意作者读法**——推送方采纳：本票交付「可装配、未触发」，触发面归后继票待立。两份全文代落票 03 / 28 Comments。
- **进 main**：簿记一笔在 `784ad076` 之上（票 03 Status + 评审 + 进 main 记录、sa-cc spec 03 行 → 进 main、票 28 Status + 评审 + 进 main 记录、lc spec 28 行 → 已进 main、本节）；纯 .md 自审；`ls-remote` 核 `9220ab06` 未动 → `merge --ff-only` → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后的广播里）。SHA 对照：sa-cc/03 `0ecb72c3→f6884cb7` / `84da8df5→06e69916` / `f96169d2→0caa03d7` / `b2b174b0→00e21321` / 清点 `343b997b`；lc/28 `b7602123→f3d938ac` / `c2f44ba8→7019f51f` / `7f821cdd→dd54ad63` / `d96e4793→38324ce1` / `05afb2d1→32eecea3` / `a35bc375→002d3135` / 清点 `784ad076`。
- **拆树**：`mcp5-sacc03` / `mcp4-lc28` 改 `merged/`（指针留）、远端删；`D:/tops/idp-parcel-mcp5-sacc03` 与 `D:/tops/idp-parcel-mcp4-lc28` 干净（后者只一件 ignored `parcel-api.exe`，先删）、作者通道都已是新会话 → 推送方比内容后拆；`idp-replay-sacc03` / `idp-review-sacc03` / `idp-replay-lc28` 推完拆（均不加 `--force`，三样各查）；空壳 `D:\tops\idp-parcel-mcp4-rework` 21:1x 核在（0 件）、21:4x 动手时已不在——不是我删的，谁删的没证据。**更正上面「接手取证」一句**：那件 ` M` 在 **`idp-replay-sacc03`** 而不在 `idp-review-sacc03`（我把两棵树的输出看串了），而且**不是幻影**——是通道 1 前会话 20:43:05（mtime）写了一半的票 03 Status 行：「20:4x 通道 1 推送方重放进 main（非作者评审：隔离子代理鉴权错死掉，改由推送方按 /code-review 两轴串行自评，0 阻断）」，一行、未提交、会话随后中断。它被本节落地版（真有非作者评审 ← 通道 6）取代，内容已如实抄在此，按死现场处置：不封存、`checkout --` 丢弃后拆树。`idp-replay-lc28` 拆时 `.git` 链接与登记已摘、目录因本 shell 的 cwd 在里面而 `Permission denied` 留空壳（0 件），`cd` 出来后 `Remove-Item` 清掉——与 parallel-sessions「删一半就停」那条同形，三样各查过。
- **候选后继（本节不立，归 owner / 下一派单）**：CC 更正版本的入向登记 / 重核对（sa-cc/03 评审 Spec ①，与票 12 并列）；lc/28 触发面（`main` 启动时装配以 fail-fast，动共享 `main.go` 须占号；Spec ② / 判断题 (b)）；建立重放在决定册留第二条 SELECTED 的对账 + `Select` 把并列 / 无人参选改成结果格（Spec ③ / (c)(d)，PS owner，可合一张）；`adoptDigest` 头注一句 + `NewDutyPaymentReconciliationHandler` 构造期拒 nil（CC 小改）。**前沿**：lc/30 ready 可派；lc/32 Blocked by 30；2 / 4 / 5 空闲。
- **归用户**（累计）：CI 计费；两件 untracked；会话中断成因——本节新增两例都在**重放做好之后、簿记之前**（通道 1 前会话 20:39 后、通道 4 前身 21:0x），与 20:3x 节「完成记录单独成笔丢报告」同一个形，只是这次丢的是推送方那半；sa-cc/03 越权风险点 (a)(b) 与四篇 ADR 越权风险点事后复核；子代理评审仍不可依赖（本节没用，直接派人）。
- **本节五数**（21:1x–21:4x 段，本机时钟）：CI main 绿/总 未量 · 集成时延 sa-cc/03 完工（分支 20:30 推出、report 未到）→ 推出 21:4x **约 70 分**（跨两次会话中断，不剔除）、lc/28 完工（分支 20:58 推出、report 未到）→ 推出 21:4x **约 45 分**；两票中位 **约 58 分** · 重放笔数 **10**（sa-cc/03 四笔 + lc/28 六笔；推送方清点 2 另计）· 重复开发 0 次 · 会话重置 **2 次**（通道 1 本会话新起 + 通道 4 自报新会话；2 / 3 / 5 / 6 点名只答空闲、未自报新旧，不计）

### 21:4x–21:5x 同一会话：用户「下一步应该做什么」→「派」——第三波四单派出（2 / 4 / 5 / 6），3 留评审位

- **前沿算自票面**（`Status:` / `Blocked by:` 行扫全部 `.scratch/*/issues`，main `9ddbafcf`）：ready 且 Blocked by 无的有 lc/30、sa-cc/01 / 04 / 05 / 06 / 10、ve-claims/04、ve-disc/03；in-progress 零、在途分支零、工作树只剩 main。其中 sa-cc/01（三件引用从哪来，SA owner）、04（一格还是一册）、05（分区主体）、06（三态怎么折）票面「要裁的」未清，**本波不派、等 owner 一句或授权代裁**；ve-claims/04 无地盘节，先补票面再派。派单第 0 步 `branch-state.ps1`：在途分支零，无半成品可撞。
- **未再点名**：五个通道 21:19 点名全应答空闲，3 / 6 在 21:3x 刚交过评审报告，派单末尾各留「若已在做别的事回一句我改派」（同 20:0x 节的做法）。基线一律远端 main `9ddbafcf`。
  - **lc/30 → 通道 2**（task-5b540f27，`mcp2-lc30`，估 60–90 分）：PS 受控关闭 / 重开写面，要裁的已清零；派单点名四件从 PC 答复取、`授权未决`非拒绝、ADR-0134 尾段、必填口构造期拒 nil、基线在干净检出上重数。
  - **ve-disc/03 → 通道 6**（task-646f7c8e，`mcp6-vedisc03`，估 45–60 分）：VE 两册目录读面 + 管理台两签，规则与策略不混；`endpoints.go` 不动。
  - **sa-cc/10 → 通道 5**（task-7e1d15e1，`mcp5-sacc10`，估 60 分）：CC 三册伴生列表读口 + http + 管理台两页三签（裁决已在票面：凭证进 `customs-cases`、协作 / 核对进 `customs-restrictions`）。
  - **四张后继票 → 通道 4**（task-d00c5556，`mcp4-followups`，只写票面）：sa-cc/13 CC 更正版本入向登记、lc/34 lc/28 触发面（回填票 28「票号待立」句）、lc/35 建立重放对账 + `Select` 结果格（顺手逐条核 lc/26 / lc/29 评审非阻断仍未修的）、sa-cc/14 两处小改。
  - **三张实施票共享点已在派单里点名**：`cmd/parcel-api/endpoints.go`（lc/30 与 sa-cc/10）、`unwired_orchestration.go` / `isolated_read_test.go`（ve-disc/03 与 sa-cc/10 若需）、探针 / 放行表（lc/30）——各加自己那块、动前占号；无新迁移；admin-web 两票分落 `pages/visibility/` 与 `pages/customs/`。**每笔推完先 `report_task` working 一句**写进了四张派单（20:3x 节归用户那条建议，本波起当硬要求）。
- **通道 3 留评审位**，不派实施票；第一张完工报到即派它评。预计第一张完工 22:4x 前后。**归用户**（新增）：sa-cc/01 / 04 / 05 / 06 四句 owner 裁决（或授权代裁）；其余不变。
- **21:5x 用户告知通道 2 / 6 crash**（2 未接到 lc/30 即 crash；6 未开工 ve-disc/03），「只有 3 空闲，你自决」。git 核：origin 无 `mcp2-lc30` / `mcp6-vedisc03`、无树、无半成品。处置：task-5b540f27 / task-646f7c8e 推送方代结 failed；**lc/30 改派通道 3**（task-a0f51893，`mcp3-lc30` 基 `cb467233`）——评审位随之空出，lc/30 / sa-cc/10 完工时由 4（后继票写完即空）/ 5 交叉评，作者不评自己；**ve-disc/03 回到 ready 待派**，等 2 / 6 重启或 4 空下来。4 / 5 已 report working（5 第 1 笔 `444d10fe` 票面 in-progress 已推）。会话重置本小节 +2（通道 2、通道 6）。
- **lc/30 要裁的（通道 3 22:0x 报，推送方当场裁，用户授权代裁；通道 3 落笔进票 30「裁决」节）**：① 地盘随做法 3 / 4 扩三处（`adapters/identity/` 新签发器、`adapters/postgres/` 新 `OutboxContinuedAttemptDecisionHandoff`——事件类型名逐字照 lc/27 票面、`adapters/http/` 新端点文件 + `UnconfiguredIntake` 两个加法方法）**准**，只加新文件不改既有行。② 重开时认原关闭「属货主指令」：**取 (a)**——关闭命令带封闭三值「货主指令 / 运营操作 / 其他」（PILOT-SCOPE 原词），编排合成带种类段的责任来源引用（照 `CONTINUED-ATTEMPT-CLOSURE/<决定标识>` 先例），合成 / 解析收在一对函数；**认不出种类段 → 不放行、不默认「其他」**；否 (b)（用 Requester 代推，与 GLOSSARY「三者分别记录」相悖、渠道方请求也带 Requester）。越权风险点三条归 owner：种类由命令声明 vs 由 PC「关闭责任来源规则」答（PC / PS owner）；domain 应否显式带种类格（PS owner）；重开决定应否保存货主新授权证据引用（本票只核不存，PS owner）。③ `AuthorityRole` 取请求坐标等级（PC `Authorization` 不暴露 grant 等级，`permits` 要求两值相等）：接受，头注按符号名指到该谓词并写明是暂行；PC 暴露 grant 等级归后继候选。

## 2026-09-11 10:3x 通道 1 新会话接续（接手时 `main = origin/main = e8fe7bf9`；用户先要一次自带 `ask_question`，再「监听队列」→「这些工作树完成了吗，可以回放了吗：`D:\tops\idp-parcel-mcp5-sacc10` / `-mcp4-followups` / `-mcp3-lc30`」）

- **接手取证**（10:34，`branch-state.ps1` + git，不读自报）：main = origin = `e8fe7bf9`；共享树 73 行 ` M`（CRLF 幻影）+ `d_tasks.txt` / `docs/api/` 两件 untracked 归用户、未动。三棵树全干净、全 = origin：`mcp3-lc30@e4498b6c`（领先 9 笔，票 30 resolved——通道 4 接续通道 3 收口，用户 22:2x 经队列直接指示，**无派单记录**；作者验 107 ok 含真库）、`mcp4-followups@3520dd1a`（4 笔纯 .md，四票 13 / 34 / 35 / 14）、`mcp5-sacc10@49ef9d5f`（领先 7 笔，票 10 resolved）。**`%TEMP%\idp-replay-batch@affdedcd`**：通道 1 前会话 23:09 已把三链叠到 `e8fe7bf9`（sa-cc/10 六笔 → 后继四票 → lc/30 八笔，两笔作者清点都跳过），随后中断——未验、**未派评审**、未簿记；`query_tasks` 22:11 之后零派单，两票 Comments 无「评审 ←」。与 21:1x 节同形（重放做好之后、簿记之前断）。核过内容：`git diff mcp3-lc30 affdedcd` 只多 sa-cc/10 那 22 件 + 清点；`git diff mcp5-sacc10 affdedcd` 只多 lc/30 那 21 件 + 四票 .md + 清点；四票 .md 对 `mcp4-followups` 零差 → 沿用，不重叠。
- **tip 兑清点 + 全量**：`affdedcd` 上重生成清点 → `3cf48524`（CC 78→83、PS 169→174、端点 112→117、端口 384→390 = 两作者各 +3）；`gofmt -l` 空、build / vet 0；10:41 占号，带 DSN `go test -p 1 -count=1 ./...` **107 ok / 0 FAIL / 15 无测试 / 0 cached**（10:41:04→10:43:11，127 s）；`-v` 探针 PS `postgres` + `cmd/parcel-api -run ContinuedAttemptDecision` PASS 5 / SKIP 0、CC `postgres -run 'Credential|DutyReconciliation|DutyPayment'` PASS 10 / SKIP 0；admin-web（junction 主树 `node_modules`）`tsc --noEmit` 0、`run-tests` 220 / 220；10:47 释号。
- **点名 + 派评审**：10:40 点名（截止 10:44），2 / 3 / 4 / 5 / 6 五应答全「空闲 · 地盘无 · 余量充足」（5 自报是 sa-cc/10 作者不评它）。lc/30 作者是 3 + 4 两个通道，sa-cc/10 作者 5。10:44 派：**lc/30 评审 → 通道 6**（task-a72a805e，钉 `e4498b6c`，基线 `cb467233`）、**sa-cc/10 评审 → 通道 2**（task-09f4b036，钉 `49ef9d5f`，基线 `9ddbafcf`），各 20 分不带 DSN。后继四票纯 .md 推送方自审：四笔只动 `.scratch/**`，Status / Blocked by 如报，相对链接逐一在。
- **评审到 · lc/30 ← 通道 6（10:4x，钉 `e4498b6c`）**：Standards 0 阻断 / 5 非阻断；**Spec 1 阻断** / 4 非阻断——22:0x 裁决 ②「认不出种类段 → 不放行、不默认『其他』」被反向实现：`application/form_continued_attempt_decision.go` `shipperReauthorizationGate` 对 `closureResponsibilitySourceKindOf(...) != ShipperInstructionResponsibilitySource` 一律放行，头注明写「认不出时也不拦」；无用例。推送方在重放树核过代码，成立。另 Spec 非阻断 ②：票 30 分支版「裁决」节**没有** 22:0x 三条（只在本台账），评审只能按派单转述对。**处置**：按「有阻断回作者在同一分支修」，10:50 回通道 4（task-0bff9595，同分支 `mcp3-lc30`，只修这一处 + 票面两句：裁决节补三条、判据 2 原句加改口注；不加新 .go 文件；11:35 前），修完通道 6 只重评 Spec 轴。全文待代落票 30。
- **三链拆两批**：lc/30 要等修 + 重评，sa-cc/10 与四票不等它。**批 1** = `72a247ff`（= `e8fe7bf9` + sa-cc/10 六笔 + 四票，直接取重放链中段，不重 pick）另建 `%TEMP%\idp-replay-sacc10`，tip 重生成清点 → **`99ceb975`**（CC 78→83、端点 112→115、端口 384→387）；`gofmt -l` 空、build / vet 0；10:51 占号，带 DSN 全量 **107 ok / 0 FAIL / 15 无测试 / 0 cached**（10:51:10→10:53:15，125 s）；CC 真库 PASS 10 / SKIP 0、`cmd/parcel-api` PASS 70 / SKIP 0；admin-web `tsc` 0、220 / 220；10:55 释号。**批 2** = lc/30 八笔（重放链 `72a247ff..affdedcd`，已适配在 sa-cc/10 之后）+ 作者修笔，修好重评无阻断后叠到批 1 之上、再兑一次清点、再全量。占号广播两条把时刻标成「11:08」，实为 10:51——推送方估错了钟，已广播更正。
- **评审到 · sa-cc/10 ← 通道 2（10:54，钉 `49ef9d5f`）**：Standards 0 阻断 / 3 非阻断（最重：`query_duty_collaborations.go` `dutyCollaborationBody` 等三处注释「CONTEXT 硬句 212 / 214」是行号引用，AGENTS.md 明令不写；沿包内旧例非新造）· Spec 0 阻断 / 3 非阻断（最重：真库与 admin-web 评审侧未验——推送方全量已补验）；作者自报三处站得住；裁决落点、端点各立入口、`application/**` 零改动逐核。评审者起的两个 `/code-review` 隔离 pass 截止前未回报，本报为评审通道自做两轴。全文代落票 10 Comments。
- **批 1 进 main**：簿记一笔在 `99ceb975` 之上（票 10 Status + 评审 + 进 main 记录、sa-cc spec 10 行 → 进 main、票 07 Blocked by 句 → 「10 已进 main」、本节）；纯 .md 自审；`ls-remote` 核 `e8fe7bf9` 未动 → `merge --ff-only` → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后的广播里）。SHA 对照：sa-cc/10 `444d10fe→264eeeeb` / `3dba4fd9→630d427d` / `3654d711→df78a173` / `0d8cd298→32311c76` / `0017f49b→e6132fce` / `49ef9d5f→24d6a376` / 清点 `99ceb975`；四票 `585af1c5→9a77be16` / `17370740→bcb676d0` / `2240d826→6e31da11` / `3520dd1a→72a247ff`。行号引用三处推送方不代改作者代码，归 CC owner 一笔换引文（候选并入 sa-cc/14 同类小改或下一号）。
- **拆树（批 1）**：`mcp5-sacc10` / `mcp4-followups` 改 `merged/`（指针留）、远端删；`D:/tops/idp-parcel-mcp5-sacc10`（作者已 rmdir junction）与 `D:/tops/idp-parcel-mcp4-followups` 干净 → 比内容后拆；`%TEMP%\idp-review-sacc10`（通道 2 未拆，供复核）推完拆；`%TEMP%\idp-replay-sacc10` 推完拆；`%TEMP%\idp-replay-batch`（含 lc/30 链）**留到批 2**。均不加 `--force`。
- **批 1 已推**：10:58 `push a6cdba51:main` 成，**远端 main = `a6cdba51`**；已广播 SHA 对照；`mcp5-sacc10` / `mcp4-followups` → `merged/`、远端删；三棵树拆（`idp-review-sacc10` 通道 2 已自拆）。通道 5 直信确认不在其树里。
- **lc/30 回修到（通道 4，11:0x，task-0bff9595 done）**：`5681d97d` fix——`shipperReauthorizationGate` 三路（认不出 → `NOT_ACCEPTED · RESPONSIBILITY_SOURCE_UNKNOWN`，签发标识之前拒；货主指令照旧核；运营 / 外部硬限制照旧放行；`!found` 归 `Append`），新用例 `TestReopeningAClosureWhoseResponsibilitySourceKindCannotBeReadIsNotAccepted` 四子例（先 red 四例皆 `FORMED` 坐实现状，后 green），不加新文件；`6e7f0ebf` 票面（裁决节补三条、判据 2 改口括注、完成记录补回修段）。作者提一处与派单出入——「授权口零调用」做不到（共用路径授权先于读册与门，重排会让未授权者探到册状态），改断言恰一次并说明，交通道 6 判。推送方核修笔只动三文件、门形如述。11:0x 派通道 6 只重评 Spec 轴（task-3e39a3a6，钉 `6e7f0ebf`）。
- **批 2 建树 + 全量**（不等重评，先验）：`%TEMP%\idp-replay-lc30` detached `a6cdba51`，`cherry-pick 72a247ff..affdedcd`（八笔，重放链里已适配在 sa-cc/10 之后的版本）+ `5681d97d` `6e7f0ebf`，十笔零冲突 → `2655a405`；本票 Go 文件对分支 tip 只差共享接线四处 sa-cc/10 的 64 行加行、票面零差；tip 兑清点 → **`6a5fb181`**（与 `3cf48524` 那份逐字同：PS 169→174、端点 115→117、端口 387→390）；`gofmt -l` 空、build / vet 0；11:04 占号，带 DSN 全量 **107 ok / 0 FAIL / 15 无测试 / 0 cached**（11:04:34→11:06:39，125 s）；PS `postgres` + `cmd/parcel-api` + `application -run 'ContinuedAttemptDecision|Reopening|ControlledClosure'` `-v` PASS 20 / SKIP 0；admin-web `tsc` 0、220 / 220；11:09 释号。
- **重评到 · lc/30 ← 通道 6（11:10，钉 `6e7f0ebf`，只 Spec 轴）**：**原阻断关闭**；阻断无、非阻断无新增；四子例真钉住裁决 ②；票面三条与本台账逐字同；「授权口恰一次」判接受、计无发现（裁决 ② 管放不放行不管问不问 PC；读册挪到授权前会让未授权者探到册状态，与 `ports.go` 否定读不区分那条纪律同）。前轮 Spec 非阻断 ①（第三值 `EXTERNAL-RESTRICTION` vs 裁的「其他」）归推送方拍：**接受**——封闭集取 CONTEXT 原词是合法来源、不设门在头注写明，改名不换机制且丢事实；「先由来源责任方形成有效解除」无落点随判断题 (d) 归 PS owner 立票。两份全文代落票 30 Comments。
- **批 2 进 main**：簿记一笔在 `6a5fb181` 之上（票 30 Status + 两次评审 + 进 main 记录、lc spec 30 行 → 已进 main、本节）；纯 .md 自审；`ls-remote` 核 `a6cdba51` 未动 → ff → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后的广播里）。SHA 对照：`b2aa505d→09bd687a` / `f6cf9d03→25364e7f` / `10bcc753→806ca95e` / `851fe41a→3fd924cc` / `18efa74e→d9dfb10a` / `73a166d6→6fc5b745` / `892ffa3f→a100a34c` / `e4498b6c→2e4253e1` / `5681d97d→6af8eed4` / `6e7f0ebf→2655a405` / 清点 `6a5fb181`。
- **拆树（批 2）**：`mcp3-lc30` 改 `merged/`（指针留）、远端删；`D:/tops/idp-parcel-mcp3-lc30`（通道 4 报干净 = origin）比内容后拆；`%TEMP%\idp-replay-batch`（先 `rmdir` admin-web `node_modules` junction）与 `%TEMP%\idp-replay-lc30`（同）推完拆；`%TEMP%\idp-review-lc30` 通道 6 报完自拆、核在不在。均不加 `--force`。
- **候选后继（本节不立，归 owner / 下一派单）**：**PS**——lc/30 评审 Standards ②③（`ports/continued_attempt_decision.go` 查询头注讲错 Requester 去向、`AuthorityRole` 头注未标暂行，一笔改口）、①⑤ + 判断题 (c)（`adjudicate` 构造期拒 nil、关闭责任来源种类抬进 `domain/**` 成类型并把合成 / 解析收成一对）、判断题 (d)（外部硬限制来源重开的「先有效解除」无落点）、偏离 ④（货主重授权证据引用落册的领域格）；**CC**——sa-cc/10 Standards ①（`query_duty_collaborations.go` / `query_duty_verifications_test.go` / `CustomsRestrictionsPage.tsx` 三处「CONTEXT 硬句 212 / 214」行号引用换引文，可并入 sa-cc/14 同类小改）。**前沿**：lc/34、sa-cc/14 ready 可派；lc/32 Blocked by 30 → 30 已进 main，可转 ready；sa-cc/13、lc/35 draft 待 owner 裁；ve-disc/03 ready 待派；2 / 3 / 4 / 5 / 6 全空闲。
- **归用户**（累计）：CI 计费；两件 untracked；会话中断成因——本节又一例在**重放做好之后、簿记之前**（前会话 23:09 建 `idp-replay-batch` 后断），连同 21:1x 节两例已是同形三例，且这次连评审都没派就断了——「重放前先派评审、评审结论先落 task result 再动树」也许比「簿记单独成笔」更早一步保住证据；lc/30 由用户 22:2x 直接经队列指示通道 4 接续、无派单记录（票面 Status 里有，台账此前没有）；sa-cc/01 / 04 / 05 / 06 四句 owner 裁决仍未到；lc/30 三条越权风险点与 sa-cc/03 两条仍归 owner 事后复核。
- **本节五数**（10:3x–11:1x 段，本机时钟）：CI main 绿/总 未量 · 集成时延（完工报→推出，不剔除跨夜中断）sa-cc/10 09-10 22:21→10:58 **约 12 时 37 分**、后继四票 22:16→10:58 **约 12 时 42 分**、lc/30 23:0x→11:1x **约 12 时 10 分**；三票中位 **约 12 时 37 分**（几乎全是前会话 23:09 中断到用户 10:32 来问之间的空档，本会话内三票从取证到推出各 26 / 26 / 45 分）· 重放笔数 **20**（sa-cc/10 六 + 四票四 + lc/30 十；推送方清点 2 另计）· 重复开发 0 次 · 会话重置 **1 次**（通道 1 本会话新起；2–6 点名只答空闲、未自报新旧，不计）

### 11:2x 同一会话：用户「下一步做什么呢」→「派」——第四波五单派出（2 / 3 / 4 / 5 / 6），不留评审位

- **前沿算自票面**（`Status:` / `Blocked by:` 行扫全部 `.scratch/*/issues`，main `2c7326ef`）：ready 且 Blocked by 已解 / 无、要裁的清零的有 lc/32（Blocked by 30 → 30 已进 main）、lc/34、sa-cc/07（步二 Blocked by 10 → 10 已进 main）、sa-cc/14、ve-disc/03、ve-disc/02 步一；draft 但要裁的已清零且 Blocked by 已解的有 lc/27（30）、lc/25（31，但票面「要裁的」1 仍写着 TF 票立不立，需先按 ADR-0135 改口）。in-progress 零、在途分支零、工作树只剩主树（`branch-state.ps1`），无半成品可撞；远端只余 `mcp3-awf12` / `mcp5-awf13` / `salvage/mcp4-tf03` 三个旧指针（归用户点头删）。
- **不派**：sa-cc/01 / 04 / 05 / 06（要裁的归 owner，等一句或授权代裁）；sa-cc/12 / 13、lc/35（draft，要裁的归 owner）；ve-claims/04（四问已裁落 ADR-0136，但无地盘节，先补票面）；lc/25（同上改口后下一波）。
- **未再点名**：10:40 点名五应答全空闲，2 / 4 / 5 / 6 在 10:5x–11:1x 刚交过评审 / 回修，3 自 10:42 应答后未被派任何事；派单末尾各留「若已在做别的事回一句我改派」（同 21:4x 节做法）。基线一律远端 main `2c7326ef`。撞文件的串在同一通道或点名对方：
  - **lc/34 → 通道 4**（task-c7390f8a，`mcp4-lc34`，估 30 分）：`main` 启动时装配 `buildLabelChannelOrchestration` 以 fail-fast + 三处头注改口；`main.go` 占号；完工后另单接 **lc/27**（消费者半边；30 已进 main，写侧尾段已在；先把 Status 转 ready）。
  - **lc/32 → 通道 3**（task-287429ec，`mcp3-lc32`，估 60–90 分）：`Establish` 前 `register.Judge(currentFinalPresent)` 核册、拒建立加一格 `PARCEL_CONTINUED_ATTEMPT_CLOSED`、先 `FindByID` 再核册、后四步不核；与 34 共用 `assemble_label_channel.go`（不同块，占号）。
  - **sa-cc/14 → 通道 5**（task-f955ead7，`mcp5-sacc14`，估 30 分）：`adoptDigest` 头注 + `NewDutyPaymentReconciliationHandler` 构造期拒 nil、调用点三处；`cmd/parcel-dispatch/assemble.go` 占号；**先落**——07 步二要装配它的新签名。
  - **sa-cc/07 → 通道 2**（task-e04c2d5a，估 90–120 分）：两步两分支——步一 CLI `mcp2-sacc07-cli` 基 `2c7326ef` 现在做、完工先进 main；步二端点 + 管理台写签 `mcp2-sacc07-web` 等 14 进 main 后基新 main（`endpoints.go` 占号）。
  - **ve-disc/03 → 通道 6**（task-7491f646，`mcp6-vedisc03`，估 45–60 分）：VE 两册目录读面 + 管理台两签（规则与策略一处不混）；`unwired_orchestration.go` 占号（与 34 的头注不同块）。
  - 五单都写进：每笔推完 `report_task` working；共享接线文件（`main.go` / `assemble_label_channel.go` / `unwired_orchestration.go` / `endpoints.go` / `cmd/parcel-dispatch/assemble.go`）动前占号、各加自己那块；注释引 CONTEXT 用引文不用行号（sa-cc/10 评审刚点过）；新增构造器构造期拒 nil。
- **评审位不留**：五单陆续完工，谁先空谁交叉评（作者不评自己；lc/32 与 lc/34 互为最熟的评审人选但 4 是 28 作者、3 是 30 前半作者，都不是对方票的作者，可互评）。**归用户**（新增）：sa-cc/01 / 04 / 05 / 06 四句 owner 裁决（或授权代裁）；lc/25 改口与 ve-claims/04 补地盘节要不要派人做（纯票面，各 15 分）；远端三个旧指针。

### 11:3x–11:5x 同一会话：第四波第一批进 main（lc/34 + sa-cc/14）；lc/27 派出；07 步一与 14 的调用点相撞定序

- **完工到**：lc/34 ← 通道 4（11:3x，`mcp4-lc34@a4c12d4e`，代码 `38f2322c`，三笔）；sa-cc/14 ← 通道 5（11:3x，`mcp5-sacc14@356b027c`，代码 `62420c95`，五笔）；sa-cc/07 步一 ← 通道 2（12:0x 自标、机时 11:4x，`mcp2-sacc07-cli@2069bace`，三笔；**它新增的 `cmd/parcel-customs-register/main.go` `buildRegistrar` 调用 `NewDutyPaymentReconciliationHandler`，与 14 的签名改动相撞**）；ve-disc/03 ← 通道 6（11:5x，`mcp6-vedisc03@51c6b79d`，代码 `f0bcb66f`，五笔，清点零差）。
- **lc/27 → 通道 4**（task-c2983660，`mcp4-lc27` 基 `b68baddf`）：lc/34 完工即派；作者按 inbox 惯例自写事件类型常量并用测试钉与 PS postgres 导出常量相等（八个既有消费者同形），推送方接受。作者占 `cmd/parcel-dispatch/assemble.go` 路由表 + lc/26 块（抽共用核），与通道 5 的 `receiveExternalFundsFactConsumer` 块隔开。
- **评审**：无空闲通道时先试隔离子代理评 lc/34——**鉴权错**（与 09-10 20:4x 同），弃；排给第一个空下来的通道 5（task-f9da570e，检出 `%TEMP%\idp-review-lc34` 推送方先建好给它用）：**两轴 0 阻断**，Standards 2 非阻断（最重：`main.go` 头注跨文件计数「六个」，AGENTS.md 明令；归 PS owner 与 lc/30 头注改口合一笔）。sa-cc/14 → 通道 2（task-77ebc993，它步二正等 14 进 main）：**两轴 0 阻断**，Spec 1 非阻断（真库未验，推送方全量闭合）。ve-disc/03 → 通道 5（task-c4404a99，12:15 前）。07 步一的评审等作者 rebase 后再派。
- **定序（07 步一 vs 14）**：14 先进 main → 通道 2 把 `mcp2-sacc07-cli` rebase 到新 main、补 `buildRegistrar` 接错误一行、`--force-with-lease` 重推、报新 tip → 再评 07 步一 → 重放 → 通道 2 开步二 `mcp2-sacc07-web`。改的是作者自己分支的 SHA 不是 main，与「共享树上绝不 rebase main」不冲突。
- **批进 main（lc/34 + sa-cc/14）**：`%TEMP%\idp-replay-w4a` detached `b68baddf`，`cherry-pick 2c7326ef..mcp4-lc34` + `2c7326ef..mcp5-sacc14` 八笔零冲突 → `72a7ef49`；两票 Go 文件对各自 tip 零差；清点 tip 重生成**零差**（两票都不增删文件），无清点笔；`gofmt -l` 空、build / vet 0；11:44 占号，带 DSN 全量 **107 ok / 0 FAIL / 15 无测试 / 0 cached**（11:44:02→11:46:11，129 s）；探针 LabelChannel + 路由装配 + 构造期拒 nil PASS 7 / SKIP 0；11:46 释号。簿记一笔在其上（票 34 / 14 Status + 评审 + 进 main 记录、lc spec 34 行、sa-cc spec 14 行、本节）；`ls-remote` 核 `b68baddf` 未动 → ff → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后广播里）。SHA 对照：lc/34 `6cf854a9→2db3eb5f` / `38f2322c→a173cddb` / `a4c12d4e→ce278a4a`；sa-cc/14 `5690bd33→054c898f` / `f3104dd8→af0f23c0` / `8de0fdcd→d1b3b95f` / `62420c95→f6da16fd` / `356b027c→72a7ef49`。
- **拆树**：`mcp4-lc34` / `mcp5-sacc14` 改 `merged/`、远端删；`D:/tops/idp-parcel-mcp4-lc34`（通道 4 已在 lc27 树）与 `D:/tops/idp-parcel-mcp5-sacc14`（通道 5 报不在其中写）比内容后拆；`%TEMP%\idp-review-lc34`、`%TEMP%\idp-replay-w4a` 推完拆；`idp-review-sacc14` 通道 2 自拆。均不加 `--force`。
- **推送方自己的差错**：本节多条占号 / 释号广播把时刻估成「12:1x / 12:2x」，实为 11:2x–11:4x，已广播更正；此后每条前先 `Get-Date`。通道 2 / 5 也各有一次估错并自纠。
- **在途**：lc/32（3，代码笔 `f7a1e1be` 已推、自审中）、lc/27（4，笔 2 `7bcbacc7` 已推、路由表在做）、sa-cc/07 步一（2，等 14 进 main 后 rebase）、ve-disc/03 评审（5）。

### 11:5x–12:0x 同一会话：ve-disc/03 进 main；lc/32 / lc/27 完工并派评审

- **已推（上小节那批）**：11:53 `push a1b19433:main` 成，**远端 main = `a1b19433`**；`mcp4-lc34` / `mcp5-sacc14` → `merged/`、远端删；四棵树拆（通道 5 直信确认不在 sacc14 树里；它自拆了 `idp-review-vedisc03`）。
- **完工到**：lc/32 ← 通道 3（11:50 机时，`mcp3-lc32@683da565`，代码 `5dd8db59`；作者自报票面时刻估错后补一笔纯簿记更正为 git 时间戳）；lc/27 ← 通道 4（11:5x，`mcp4-lc27@745b7dd8`，代码 `868725ca`，清点 `70d77d91`：PS 生产 174→176 / 测试 167→169、消费适配 28→29、直投路由 19→20；路由表 gofmt 重排 map 对齐，作者已广播提醒后续加行者）。
- **评审到 · ve-disc/03 ← 通道 5（11:50，钉 `51c6b79d`）**：Standards 0 阻断 / 2 非阻断（最重：新注释延续「八法 / 七法」跨文件计数；② 票 02 步一 CLI 命令名要对齐 03 落下的 `kind` 原词）· Spec 0 阻断 / 0 非阻断（真库与 tsc 评审侧未验，推送方全量闭合）。全文代落票 03。
- **派评审**：lc/32 → 通道 4（task-3af3e0c9，钉 `683da565`；4 是 lc/28 组合根作者、不是 32 作者）；lc/27 → 通道 5（task-914031d5，钉 `745b7dd8`）；各 12:20 前。07 步一等通道 2 rebase 重推后再派（候选 3 或 6）。
- **ve-disc/03 进 main**：`%TEMP%\idp-replay-vedisc03` detached `a1b19433`，`cherry-pick 2c7326ef..mcp6-vedisc03` 五笔零冲突（唯一重叠 `unwired_orchestration.go`：lc/34 头注块 vs 本票占位两法块，不同块）；本票文件对分支 tip 只差那两行头注；清点零差、无清点笔；`gofmt -l` 空、build / vet 0；11:58 占号，带 DSN 全量 **107 ok / 0 FAIL / 15 无测试 / 0 cached**（11:58:51→12:01:00，129 s）；VE postgres + http 探针 PASS 16 / SKIP 0；admin-web `tsc` 0、220 / 220；12:01 释号。簿记一笔在其上（票 03 Status + 评审 + 进 main 记录、票 02 Blocked by 句 → 「03 已进 main」+ 命令名对齐一句、本节）；`ls-remote` 核 `a1b19433` 未动 → ff → `push <sha>:main`。**远端 main = 本笔 tip**。SHA 对照：`b939c1d3→063744d6` / `c9f9bf78→4fa83d08` / `0b786fa6→38492882` / `f0bcb66f→6ec6df6c` / `51c6b79d→53370bb6`。`mcp6-vedisc03` → `merged/`、远端删；`D:/tops/idp-parcel-mcp6-vedisc03`（作者报干净、junction 已 rmdir）与 `%TEMP%\idp-replay-vedisc03` 拆。
- **在途**：lc/32 评审（4）、lc/27 评审（5）、sa-cc/07 步一 rebase（2）。**归用户**（累计不变）+ 新增：VE 注释计数一族（旧「六法」+ 新「八法 / 七法」）归 VE owner 一笔改口。

### 12:0x–12:1x 同一会话：lc/32 + lc/27 进 main；07 步一重推并派评审

- **已推（上小节）**：12:02 `push 6f2e4942:main` 成，**远端 main = `6f2e4942`**（ve-disc/03）；`mcp6-vedisc03` → `merged/`、远端删；两棵树拆（作者树只余 ignored `apps/admin-web/.tmp-test/`）。
- **07 步一重推 ← 通道 2（11:57 机时）**：`mcp2-sacc07-cli` rebase 到 `a1b19433`（含 14）并 `--force-with-lease`：`c2c87e23→c4e4c70c` / `37acb7b1→5621b0b5` / `2069bace→40f3d6d3`（`buildRegistrar` 接新签名 amend 进同笔，让每一笔在新 main 上可编译）；带 DSN `cmd/parcel-customs-register` 35 PASS / 0 SKIP。通道 2 随即开步二 `mcp2-sacc07-web` 基 `a1b19433`（不基步一分支，两支独立进 main；步二若复用 `registrationjson` 译装等步一进 main 后 rebase 接上）。**07 步一评审 → 通道 6**（task-8543732a，钉 `40f3d6d3`，基线 `a1b19433`，12:30 前）。
- **评审到 · lc/32 ← 通道 4（11:59，钉 `683da565`）**：Standards 0 阻断 / 3 非阻断（最重：`NewLabelTransactionHandler` 只拒两新口、头注理由不自洽；三处「读终局 → 读册 → 空册」同形可抽 helper；error 前缀中英混杂）· Spec 0 阻断 / 3 非阻断（最重：`FindByID` 重放提前于 `priorLink` 的行为变化未记；`label_transaction_judgment_handoff_test.go` 连带不止签名；判据 4 只钩两步）。**lc/27 ← 通道 5（12:00，钉 `745b7dd8`）**：Standards 0 阻断 / 2 非阻断（「三路 / 六口」计数；每路各装一只核）· Spec 0 阻断 / 1 非阻断（一条龙验收要有票号或余工表一句——已采纳，写进 lc spec 27 行）。两份全文代落票 32 / 27。
- **批进 main（lc/32 + lc/27）**：`%TEMP%\idp-replay-w4b` detached `6f2e4942`，`cherry-pick 2c7326ef..mcp3-lc32` 五笔 + lc/27 `510ce2cb` `7bcbacc7` `868725ca` `745b7dd8` 四笔零冲突（重叠：`assemble_label_channel.go` lc/34 头注 vs 32 deps 块、`cmd/parcel-dispatch/assemble.go` 14 块 vs 27 路由表 + 26 块——各不同块）；两票 Go 文件对各自 tip 只差对方已进 main 的那几行。清点：先跳过作者笔 `70d77d91` 在 tip 重生成，与作者那份**逐字同**（PS 176 / 169、消费适配 29、直投路由 20）→ 改为直接 `cherry-pick 70d77d91` 保留作者出处、再重生成核零差 → tip **`6bf9861b`**。`gofmt -l` 空、build / vet 0；12:06 占号，带 DSN 全量 **107 ok / 0 FAIL / 15 无测试 / 0 cached**（12:06:49→12:08:57，128 s）；探针 PS inbox + `cmd/parcel-dispatch` + `cmd/parcel-api` + PS application `-run 'ContinuedAttempt|LabelChannel|ControlledClosure|Reopening|ClosedParcel|Establish'` PASS 55 / SKIP 0（两票评审侧「未验」的真库在此实跑）；12:11 释号。簿记一笔在其上（票 32 / 27 Status + 评审 + 进 main 记录、lc spec 32 / 27 行（27 行加一条龙余工句）、本节）；`ls-remote` 核 `6f2e4942` 未动 → ff → `push <sha>:main`。**远端 main = 本笔 tip**。SHA 对照：lc/32 `be0c4bfb→f43729ac` / `f7a1e1be→cb5ddeb4` / `5dd8db59→671d4881` / `2c82a7ff→b55f0968` / `683da565→08d50985`；lc/27 `510ce2cb→6d7079f5` / `7bcbacc7→91ed7951` / `868725ca→6cc918af` / `745b7dd8→d98ee9d1` / `70d77d91→6bf9861b`。
- **拆树**：`mcp3-lc32` / `mcp4-lc27` → `merged/`、远端删；`D:/tops/idp-parcel-mcp3-lc32`（作者报保留到拆树指令）与 `D:/tops/idp-parcel-mcp4-lc27`（作者报干净、shell 已退出）比内容后拆；`%TEMP%\idp-replay-w4b` 推完拆；`idp-review-lc32` / `idp-review-lc27` 评审者自拆。均不加 `--force`。
- **候选后继（本节累计，归 owner）**：**PS 一张小票**收下今天四份评审点的同族头注 / 构造器项——lc/30 S①②③⑤、lc/34 S①（「六个」计数）、lc/32 S①②③、lc/27 S①（「三路 / 六口」计数）+ lc/30 头注「导出给 27 引」改口；**PS**：lc/27 一条龙验收（lc spec 27 行余工句）、lc/30 (c)(d) / 偏离 ④、lc/32 并发窄格「违反截断边界的业务判断」；**VE**：注释「六法 / 八法 / 七法」计数一族；**CC**：sa-cc/10 行号引用三处。
- **在途**：07 步一评审（6，12:30 前）、07 步二（2）。3 / 4 / 5 空闲。

## 2026-09-11 13:2x 通道 1 新会话接续（接手时 `main = origin/main = 2dcffbb3`；用户先要一次自带 `ask_question`，再「监听队列」→「这两个可以 replay 了吗：`D:\tops\idp-parcel-mcp2-sacc07-cli` / `-web`」→「继续」）

- **接手取证**（13:2x，`git worktree list` + 逐树 `status` / `log main..HEAD` / `merge-base`，不读自报）：main = origin = `2dcffbb3`；共享树仍是那 73 行 ` M`（CRLF 幻影）+ `d_tasks.txt` / `docs/api/` 两件 untracked，归用户、未动。**cli**：`mcp2-sacc07-cli@40f3d6d3` 干净、= origin，基 `a1b19433` 三笔，动 6 件，与 main 自基线起 17 笔**零文件重叠**；**web**：`mcp2-sacc07-web@a1b19433` **零提交**（就是旧 main）、远端无此分支，树上只有一份未跟踪的 `internal/customscompliance/adapters/http/register_credential_and_duty_test.go`（22.8 KB、UTF-8 正常、mtime 12:03:31、`go vet` 报 `undefined: customshttp.NewRegisterRegulatoryCredentialEndpoint`——步二 TDD 的 red 半边，处理器 / `endpoints.go` 行 / 管理台写签一个没有）。答用户：cli 可重放只差评审门；web 没有东西可重放，不拆不动。
- **评审门**：`query_tasks` 核 task-8543732a（07 步一评审 → 通道 6，12:05 派、12:30 止）**status 仍 pending、updatedAt = createdAt**——通道 6 从未认领，逾时一小时。按「到时未交，推送方改派或自跑，不等」自跑：13:3x 直信通道 6 撤回；起 `/code-review` 隔离子代理→ **鉴权错**（与 09-10 20:4x、09-11 11:3x 同一失败面，第三次；子代理评审这条路在本机今天走不通，下回直接自跑不再试）；两轴由通道 1 会话直读 diff（通道 1 非作者）：**Standards 0 阻断 / 1 非阻断**（跨包计数「十三格」四处，与 lc/34「六个」/ lc/27「三路 / 六口」/ sa-cc/14「四口」同族，归 CC owner）· **Spec 0 阻断 / 1 非阻断**（核对子命令无一条经 `execute` 到编排 NOT_ACCEPTED 的用例，映射已由 `TestDutyReconciliationAnswerCoversEveryOutcome` 钉住）。全文落票 07 Comments。
- **重放 + 清点 + 全量**（评审前先做，与评审无依赖）：`%TEMP%\idp-replay-sacc07` detached `2dcffbb3`，`cherry-pick a1b19433..mcp2-sacc07-cli` 三笔零冲突，本票文件对分支 tip 零差 → `3ec86aaa`；tip 兑清点 → **`fa386a29`**（`cmd/` 测试 82→83，其余不变）；`gofmt -l` 空、build / vet 0；13:31 占号（已广播），带 DSN `go test -p 1 -count=1 ./...` **107 ok / 0 FAIL / 15 无测试 / 0 cached**（13:31:33→13:33:41，128 s）；`-v` 探针 `cmd/parcel-customs-register` + `registrationjson` PASS 35 / SKIP 0（与作者自报同数）、CC `adapters/postgres` 凭证 / 协作 / 核对 PASS 13 / SKIP 0；13:3x 释号（已广播）。admin-web 未跑：本票零 `apps/` 改动。
- **进 main（07 步一）**：簿记一笔在 `fa386a29` 之上（票 07 Status + 步一完工代记 + 评审全文 + 进 main 记录、sa-cc spec 07 行 → 步一已进 main / 步二在途、本节）；纯 .md 自审；`ls-remote` 核 `2dcffbb3` 未动 → `merge --ff-only` → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后的广播里）。SHA 对照：`c4e4c70c→a5c6523f` / `5621b0b5→138edb11` / `40f3d6d3→3ec86aaa` / 清点 `fa386a29`。
- **拆树**：`mcp2-sacc07-cli` 改 `merged/`（指针留）、远端删；`D:/tops/idp-parcel-mcp2-sacc07-cli`（干净 = origin）比内容后拆；`%TEMP%\idp-replay-sacc07` 推完拆。**`D:/tops/idp-parcel-mcp2-sacc07-web` 不拆**——未提交现场，通道 2 在做步二。均不加 `--force`。
- **候选后继（累计，归 owner）**：**CC**——sa-cc/07 步一 Standards ①「十三格」四处 + sa-cc/10 三处行号引用，合一笔改口；其余同上节（PS 小票、VE 计数一族）。**归用户**（累计）：CI 计费；两件 untracked；通道 6 对派单无响应一次（task-8543732a 全程 pending）——点名答「空闲」不等于会认领，派评审后 5 分钟没转 working 就该改派，不等 20 分钟；隔离子代理鉴权错第三次，归本机环境。
- **在途**：07 步二（2，`mcp2-sacc07-web`，red 测试一份未提交）。3 / 4 / 5 / 6 状态未点名。
- **本节五数**（13:2x–13:4x 段，本机时钟）：CI main 绿/总 未量 · 集成时延（完工报→推出）sa-cc/07 步一 12:0x（作者自标）→13:4x **约 100 分**（其中 12:05→13:2x 是评审派出后无人认领、通道 1 前会话结束到用户 13:2x 来问的空档；本会话内从取证到推出约 25 分）；1 票 · 重放笔数 **3**（清点 1 另计） · 重复开发 0 次 · 会话重置 **1 次**（通道 1 本会话新起）

### 13:5x–14:0x 同一会话：用户「下一步做什么呢」→「派」——第五波五单派出（2 / 3 / 4 / 5 / 6）

- **前沿算自票面**（`Status:` / `Blocked by:` 行扫全部 `.scratch/*/issues`，main `51ca1270`）：308 张——resolved 283 / ready-for-agent 8 / draft 7 / needs-info 4 / blocked 2 / in-progress 1（sa-cc/07 步二）。ready 且要裁的清零、Blocked by 已解：sa-cc/01 / 04 / 05 / 06 / 08（08 只挡判据 4，登记册 + 发信封半边可先落）、ve-claims/04（**缺「地盘」节**）、ve-disc/02（步一步二都不阻，03 已进 main）；等一步：sa-cc/09（等 05）、sa-cc/11（等 08）、lc/25（31 已进 main，「要裁的」1 待按 ADR-0135 改口）；draft 归 owner 一句：lc/35、sa-cc/12 / 13。`branch-state.ps1`：在途只 `mcp2-sacc07-web`——通道 2 已把它挪到 `51ca1270`（13:45，领先 0、未推）。
- **半成品核**（派单第 0 步）：`git branch --list` 与 `ls-remote --heads` 对 vedisc02 / sacc04 / sacc05 / sacc08 / sacc01 / sacc06 零命中；远端只余 `mcp3-awf12` / `mcp5-awf13` / `salvage/mcp4-tf03` 旧指针（归用户点头删）。
- **点名**：13:57 广播（截止 14:01），**13:58 五应答全到**：2「空闲（sa-cc/07 步二未开工：树已快进到 51ca1270，只有一件未提交 red 测试，**本会话无派单**）· 地盘 `D:/tops/idp-parcel-mcp2-sacc07-web`」、3 / 4 / 5 / 6「空闲 · 地盘无 · 余量充足」。通道 2 那句坐实了一件事：task-e04c2d5a 是前会话的派单，新会话没有步二的记忆——**两步票要在步一进 main 时给步二重新派一单**，不能靠原单撑到底。
- **派单（14:0x，基线一律远端 main `51ca1270`）**：
  - **sa-cc/07 步二 → 通道 2**（task-6a228ee9，估 90 分）：接着 `mcp2-sacc07-web` 树上那份 red 做；CC `adapters/http` 三个 `*Registrar` + 处理器 + 封闭响应形、`endpoints.go` 三行（占号）、`assemble_customs*.go`、`pages/customs/` 三册写签挂 10 的读签旁；答案代数照 CLI `dutyReconciliationAnswer` 归法转 HTTP；`kind` 缺席不在译装拒。
  - **ve-disc/02 → 通道 3**（task-a2cb7477，`mcp3-vedisc02`，估 120 分）：两步一起——`CatalogRegistration` 两方法 + 三处替身、`parcel-ve-register` 两命令（命令名对齐 03 的 `kind` 原词）、`adapters/http/register_catalog.go` 两 Registrar、`endpoints.go` 两行（占号）、`assemble_ve_registration.go`、`pages/visibility/` 两写签。
  - **sa-cc/04 → 通道 4**（task-42663c03，`mcp4-sacc04`，估 90 分）：独立「凭证门禁判断」登记册（ADR-0137 决定一 / 二），新编排调 `JudgeCredentialApplicability`、ports 写读、postgres、**迁移钉 `customs_compliance/0018`**；装配点先核在哪；**完工后同通道接 sa-cc/06**（同目录串着做）。
  - **sa-cc/05 → 通道 5**（task-3de4ab39，`mcp5-sacc05`，估 60 分）：`DutyPaymentVerificationHandoff` + postgres handoff，`VerifyPayment` 形成格同事务发 `customs-compliance.duty-payment-verification.formed`，分区主体 租户 / 申报范围；`DutyPaymentReconciliationDeps` 加 `Handoff` 口 → 两处装配点（`cmd/parcel-dispatch/assemble.go`、`cmd/parcel-customs-register/main.go`）+ 三处夹具；迁移若需取 `0019`。
  - **sa-cc/08 → 通道 6**（task-b5cadbb5，`mcp6-sacc08`，估 90–120 分）：评价请求登记册（铸造 ID + 自然键唯一、排序后摘要）、`RequestBuyEvaluation` 同事务发 `settlement-accounting.evaluation-request.submitted`、`EvaluationRequestHandoff`、**迁移钉 `settlement_accounting/0019`**、装配点先核、CONTEXT-MAP 一条边；判据 4 标「等 11」。
- **撞点已在各单写明**：`endpoints.go` / `apps/admin-web` / `internal/architecture` 管理台基线——2 与 3；`NewDutyPaymentReconciliationHandler` 签名（05 加口）vs `cmd/parcel-api` 新调用点（07 步二）——谁后进 main 谁适配；CC `ports` / `adapters/postgres` / 迁移序号——4 与 5；`cmd/parcel-dispatch/assemble.go` / `unwired_orchestration.go`——4 / 5 / 6 各加自己那一块、占号。
- **不派**：sa-cc/01（与 08 撞 `assemble.go`，留下一波）；sa-cc/06（04 完工后同通道接）；sa-cc/09 / 11（等 05 / 08）；ve-claims/04、lc/25（通道 1 自己先补票面：地盘节 / ADR-0135 改口，转 ready 备下一波）；lc/35、sa-cc/12 / 13（要裁的归 owner）。**评审位不预留**：完工时点名当下空闲者；派出后 5 分钟未转 working 就改派，不等 20 分钟（13:2x 节教训）。
- **在途**：07 步二（2）、ve-disc/02（3）、sa-cc/04（4）、sa-cc/05（5）、sa-cc/08（6）。通道 1 接下来：ve-claims/04 地盘节、lc/25 改口、三张注释改口小票（PS / VE / CC）。
- **通道 1 顺手（14:1x–14:2x，纯 .md 自审，直接落 main）**：① ve-claims/04 补「地盘」节（从「要做什么」1–4 抄出，不加宽），下一波可派；② lc/25 draft → **ready-for-agent**：要裁的 1 标已裁并抄 ADR-0135 答案、Blocked by 改「31 已进 main」、失效版本那一格从裁决末段抬成要裁的 2（归 PS owner；怎么停按红线定为显式未决 / 未决哨兵——A 类代裁，怎么重派生待裁），lc spec 25 行同步；③ 立三张注释改口小票并直接 ready：**sa-cc/15**（CC「硬句 NNN」行号引用 76 + 2 + admin-web 6 处——`0b027ab8` 实测 CC CONTEXT 217 行是 ADR-0137 插句，其后 218 / 219 一族已指错一行；加 `parcel-customs-register`「八格 / 十三格」跨包计数；软阻等 04 / 05 / 07 步二进 main）、**ve-disc/04**（VE「硬句 NNN」38 处 + 「六法 / 七法 / 八法」计数；软阻等 ve-disc/02 进 main）、**lc/36**（lc/34 S① / lc/27 S① / lc/32 S①③ / lc/30 S①②③⑤ + 「导出给 27」头注改口，不含 lc/32 S② 与 lc/30 S④；无阻，可即派）。sa-cc spec 15 行、lc spec 36 行同步。**前沿 ready 8 → 11**（+ lc/25、sa-cc/15、ve-disc/04、lc/36，− 无）；下一波候选：lc/36（任何空闲通道）、ve-claims/04、lc/25（PS，与 lc/36 不同文件）、sa-cc/01、sa-cc/06（4 接）。
- **五单 5 分钟内全转 working**（14:1x：2 / 3 / 4 / 5 / 6 各报建树或读现场）。**通道 4 crash（用户 14:3x 报，附截图）**：git 实测 `D:/tops/idp-parcel-mcp4-sacc04@51ca1270` 已建、零提交、`status` 干净、未推——什么都没做；截图末步是 `rg` CommandNotFound（本机无 ripgrep）。**重派 sa-cc/04 → 通道 4**（task-f9d6cd97，接替 task-42663c03；派单写明「接着那棵树用、不再建」与「搜代码用 `git grep` / `Select-String`」）。会话重置本节 +1（通道 4）。
- **通道 6 crash（用户 14:4x 报，附截图，问「全部完成了吗」）**：**没有**。git 实测 `mcp6-sacc08@96d1d9dd`（只推了认领笔）；树上五件**未跟踪、未推**：`domain/evaluation_request.go`（+ `_test`）、`ports/evaluation_request.go`、`application/request_buy_evaluation.go`（+ `_test`），mtime 14:21–14:27，约 49 KB；截图末步是 domain + application 用例刚跑绿。未做：postgres 登记册与 handoff、迁移 0019、装配点、CONTEXT-MAP 边、真库、清点、票面。推送方核 gofmt 空 / build / vet 0 / 两包 ok 后**代提 `chore(salvage)` 一笔 `6dda71fd` 并推分支**（一字未改）——这是「每笔提交后推分支」纪律的第 N 个反例：没推，crash 就丢；这次靠推送方手工救回约 20 分钟现场。**重派 sa-cc/08 → 通道 6**（task-da7f289a，接替 task-b5cadbb5；从 `6dda71fd` 接着做、先读那五件）。会话重置本节 +1（通道 6，累计 2）。
- **通道 2 crash（用户 14:5x 报，附截图）**：这一次纪律**有效**——已推两笔 `1bb68a9c`（票面开工）/ `0fedd53c`（CC `adapters/http` 三册端点 + 569 行测试，按 ADR-0022 状态码），只丢一件未提交的 `cmd/parcel-api/assemble_customs_registration.go`（+132/−16，mtime 14:29）。推送方核 build / vet 0、CC http ok（gofmt 未过）后**代提 `chore(salvage)` `5104cd47` 并推**。未做：`endpoints.go` 三行 + `main.go` / `endpoints_test.go` / `unwired_orchestration.go` 各一块、真库装配用例、admin-web 三册写签、清点、票面。通道 2 14:2x 的 `cmd/parcel-api` 占号随 crash 失效（通道 3 已在自己树上加 VE 两行，不阻）。**重派 07 步二 → 通道 2**（task-564ffa0f，接替 task-6a228ee9；从 `5104cd47` 接着、第一笔 gofmt、动 `endpoints.go` 前重占号）。会话重置本节 +1（通道 2，累计 3：4 / 6 / 2，半小时内）。
- **通道 3 crash（用户 15:0x 报，附截图）**：步一 `fdc3d1eb`（794 行，CLI 带 DSN 已绿）已推安全；步二九件未提交（VE `adapters/http` 四件、`cmd/parcel-api` 五件，+377/−28，mtime 14:29–14:35）。推送方核 build / vet 0、VE http + `cmd/parcel-api` 不带 DSN ok（gofmt 未过一件）后**代提 `chore(salvage)` `ced1ca72` 并推**。未做：admin-web 两册写签、architecture、真库装配用例核、清点、票面。**重派 ve-disc/02 → 通道 3**（task-ce406503，接替 task-a2cb7477；从 `ced1ca72` 接着）。会话重置本节 +1（通道 3，累计 4：4 / 6 / 2 / 3，一小时内；只有 5 没倒）。
- **归用户（新增）**：四个通道一小时内各 crash 一次（4 建树后、6 写了 20 分钟未提交、2 与 3 各在改 `cmd/parcel-api` 装配文件时），crash 点都在读写大文件 / 跑测试之后——成因归本机环境，与今天早上子代理鉴权错同源与否不知。「每笔提交后推分支」在 2 / 3 有效（各只丢一件 / 九件未提交）、在 6 无效（五件全未提交）、在 4 未到那一步；四次都靠推送方 `chore(salvage)` 手工救回。建议把「建树后立刻提一笔认领并推」+「每个 red→green 循环成笔即推」抬成开工纪律写进 `docs/agents/parallel-sessions.md`，归你拍。

### 14:56 用户「暂停同坐，交接给下次工作的提示词」——本会话收口（状态全在 git，不在任何人记忆里）

- **main = origin/main = `851b4540`**（本会话推了 8 笔：`51ca1270` 07 步一进 main、`0b027ab8` 派单簿记、`5d6fda3f` 顺手三件、`86c00fd8` / `57237a03` / `96175fc6` / `851b4540` 四次 crash 簿记）。共享树 ` M` 73 行仍是 CRLF 幻影 + `d_tasks.txt` / `docs/api/` 两件 untracked，归用户、未动。
- **在途五树五分支（`branch-state.ps1` 实测 14:56，全部 dirty 0、全 = origin）**：
  - `mcp2-sacc07-web@5104cd47`（3 笔：票面 / http 三端点 + 测试 / salvage 装配文件）——**sa-cc/07 步二**，重派 task-564ffa0f 给通道 2，**尚未见 working**；剩 `endpoints.go` 三行 + main / endpoints_test / unwired 各一块、真库装配用例、admin-web 三册写签、清点、票面。第一笔要 `gofmt -w assemble_customs_registration.go`。
  - `mcp3-vedisc02@ced1ca72`（3 笔：票面 / 步一 CLI 794 行 / salvage 步二九件）——**ve-disc/02**，重派 task-ce406503 给通道 3，**尚未见 working**；剩 admin-web 两册写签、architecture、真库装配用例核、清点、票面。第一笔 `gofmt -w register_catalog_test.go`。
  - `mcp4-sacc04@51ca1270`（0 笔、空树、**未推**）——**sa-cc/04**，重派 task-f9d6cd97 给通道 4，**尚未见 working**；从零做。完工后同通道接 sa-cc/06。
  - `mcp5-sacc05@27b30cd8`（基 `5d6fda3f`；3 笔：票面 / 代码 `353597a2` / 清点 `27b30cd8`）——**sa-cc/05**，通道 5 **唯一没 crash**，隔离树带 DSN 六包 PASS 659 / SKIP 0、gofmt / build / vet 0，**完工报（票面完成记录 + `report_task` done）截至 14:56 未到**——下一会话第一件事大概率就是收它：点名派非作者评审 → 重放（注意它给 `DutyPaymentReconciliationDeps` 加了 `Handoff` 口，07 步二在 `cmd/parcel-api` 的新调用点谁后进 main 谁适配）。
  - `mcp6-sacc08@6dda71fd`（2 笔：票面 / salvage 五件 domain + ports + application）——**sa-cc/08**，重派 task-da7f289a 给通道 6，**尚未见 working**；剩 postgres 登记册 + `evaluation_request_handoff` + 迁移 SA `0019` + 装配点 + CONTEXT-MAP 边 + 真库 + 清点 + 票面。
- **`query_tasks` 上仍挂着的旧单**（被接替、不必再报）：task-42663c03（04）、task-b5cadbb5（08）、task-6a228ee9（07 步二）、task-a2cb7477（ve-disc/02）；task-8543732a（07 步一评审）pending 已作废。
- **前沿 ready 11**：lc/36（无阻，任何空闲通道）、ve-claims/04、lc/25（PS）、sa-cc/01（等 08 的 `assemble.go` 块落了再进）、sa-cc/06（4 接）、sa-cc/15（软阻等 04 / 05 / 07 步二）、ve-disc/04（软阻等 ve-disc/02）+ 在途五张。draft 归 owner 一句：lc/35、sa-cc/12 / 13；sa-cc/09 等 05、sa-cc/11 等 08。
- **迁移序号已预钉**：CC `0018` → 04、`0019` → 05 若加表（05 实际未加迁移：`353597a2` 只 +handoff）；SA `0019` → 08。
- **归用户（本会话累计）**：① 一小时四 crash（4 / 6 / 2 / 3）成因归本机；纪律建议见上条；② 隔离子代理评审鉴权错第三次——评审只能派通道或推送方直读；③ 通道 6 对 task-8543732a 全程未认领（派评审后 5 分钟未 working 就改派）；④ 派单里 HTTP 状态码口径我写错一次（4xx / 409），通道 2 按 ADR-0022 纠正，已同意——下回派端点票直接写「按 ADR-0022：形成 201、其余答案 200 原名、无答案 500」；⑤ 两件 untracked、远端三个旧指针（`mcp3-awf12` / `mcp5-awf13` / `salvage/mcp4-tf03`）待点头删、CI 计费。
- **本节五数**（13:2x–14:56 全段，本机时钟）：CI main 绿/总 未量 · 集成时延（完工报→推出）sa-cc/07 步一 12:0x→13:4x **约 100 分**（1 票；本会话内 25 分）· 重放笔数 **3**（清点 1 另计；另 `chore(salvage)` 代提 4 笔在四条分支上，不算重放）· 重复开发 0 次 · 会话重置 **5 次**（通道 1 新起 1 + 通道 4 / 6 / 2 / 3 各 1）

### 15:0x–15:1x 同一会话（用户已暂停，监听未断）：sa-cc/05 完工报到 → 推送方自评 → 进 main

- **完工到**：sa-cc/05 ← 通道 5（15:0x，`mcp5-sacc05@33265d0c`，代码 `353597a2`，四笔；基 `5d6fda3f` 而非派单写的 `51ca1270`——开工时远端已到，其间两笔只动 `.scratch/`，作者自判基新 tip，对）。作者带 DSN 六包 PASS 659 / SKIP 0；四条判断项写在票 Comments。
- **评审**：15:00 点名（截止 15:12）**只有作者通道 5 应答**，2 / 3 / 4 / 6 crash 后未回来（用户已暂停，未重启）；不等到截止——四个通道已知不在，等是零信息——按「没有空闲通道时推送方自己」办，**通道 1 直读重放树自评**（子代理今天鉴权错三次，不再试）：Standards 0 / 0；Spec 0 阻断 / 2 非阻断（① `已存在`不重发 + 续办引用无重投路径——作者判断项 ①，库侧失败同事务论证成立、票面字面支持，不挡，改不改回既有形归 CC owner 一句；② CLI 答复面未透出 `HandoffReference()`——并入 sa-cc/15）。作者判断项 ② 口名段接受（ADR-0074 决定二先例）、④ 接通断言接受。全文落票 05 Comments。
- **重放 + 全量**：`%TEMP%\idp-replay-sacc05` detached `c9d5f93e`，`cherry-pick 5d6fda3f..mcp5-sacc05` 四笔零冲突（13 件 vs main 五笔 tasks.md 零重叠），本票文件对分支 tip 零差；作者清点笔在 tip 重生成**零差**、沿用；`gofmt -l` 空、build / vet 0；15:01 占号，带 DSN 全量 **107 ok / 0 FAIL / 15 无测试 / 0 cached**（15:01:39→15:03:47，128 s）；探针 CC postgres handoff PASS、cmd 两包 PASS 10 / SKIP 0；15:04 释号。
- **进 main**：簿记一笔在 `4ab49c65` 之上（票 05 Status + 评审 + 进 main 记录、sa-cc spec 05 行 → 进 main / 09 行 Blocked by → 已解、票 09 Blocked by 句、本节）；纯 .md 自审；`ls-remote` 核 `c9d5f93e` 未动 → ff → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后的广播里）。SHA 对照：`f4713583→22362733` / `353597a2→aeb9c6a9` / `27b30cd8→522f65b6` / `33265d0c→4ab49c65`。
- **拆树**：`mcp5-sacc05` → `merged/`、远端删；`D:/tops/idp-parcel-mcp5-sacc05`（作者报干净、树保留等重放）比内容后拆；`%TEMP%\idp-replay-sacc05` 推完拆。均不加 `--force`。
- **下游**：sa-cc/09 解阻（票面 Blocked by 已改，Status 仍 ready）；**07 步二（通道 2）rebase 后要多传 `Handoff` 口**——推后广播点名。**在途**变为四树：2 / 3 / 4 / 6 各一张重派单，均未见 working（用户暂停中）。前沿 ready 12（+ sa-cc/09）。
- **本节五数**（15:0x–15:1x 段）：CI 未量 · 集成时延 sa-cc/05 15:0x→15:1x **约 12 分**（1 票）· 重放笔数 **4**（含作者清点笔，无推送方清点笔）· 重复开发 0 · 会话重置 0

## 2026-09-11 15:2x 通道 1 新会话接续（接手时 `main = origin/main = 7c37253f`；用户先要一次自带 `ask_question`，再「监听队列」→ 队列里是用户手写的 `handoff.md`）

- **接手取证**（15:27，`branch-state.ps1` + git，不读自报）：main = origin = `7c37253f`——handoff.md 写的「暂停时 main = c9d5f93e」「最先要收的多半是 sa-cc/05」已过时：上一会话在用户 14:56 暂停之后监听未断，15:0x–15:1x 把 05 收进了 main（上节），`mcp5-sacc05` 已 `merged/`（@`33265d0c`）、`D:/tops/idp-parcel-mcp5-sacc05` 与 `%TEMP%\idp-replay-sacc05` 均已不在。在途四树四分支全干净、全 = 已推指针：`mcp2-sacc07-web@5104cd47` / `mcp3-vedisc02@ced1ca72` / `mcp4-sacc04@51ca1270`（空、未推）/ `mcp6-sacc08@6dda71fd`。共享树仍是 73 行 CRLF 幻影 + `d_tasks.txt` / `docs/api/` 两件 untracked，归用户、未动。`load_progress` 与 git 一致。
- **`query_tasks`**：四张重派单 task-564ffa0f（2）/ task-ce406503（3）/ task-f9d6cd97（4）/ task-da7f289a（6）**全 pending、updatedAt = createdAt**——派出 30–60 分钟无人认领（派出时四个通道都 crash 着，用户暂停中）；`list_sessions` 15:26 显示 2 / 3 / 4 / 6 最近一分钟内有活动、5 最后活动 15:09。
- **点名**（15:29 广播，截止 15:33）：15:31 前五应答全到——**2 直接 `report_task` working**（task-564ffa0f；自报 git 实测树干净 = origin、已 rebase 到 `7c37253f` 零冲突 `d77b408c` / `7ee71b20` / `f40e92df`，顺序 gofmt → 接 05 的 `Handoff` 口 → `endpoints.go` 占号 → 其余）；3 / 4 / 5 / 6 答「空闲 · 地盘无 · 余量充足」。3 / 4 / 6 各有一张 pending 重派单却答了空闲——队列不留副本，它们读到过派单原文没有不知，只记事实。**处置**：15:31 直信 3 / 4 / 6 各一条（单号、树、tip、剩什么、先 rebase 到 `7c37253f`、`query_tasks` 读全文、立即 working；5 分钟未 working 改派）。15:33 `branch-state` 已见 `mcp4-sacc04` 树快进到 `7c37253f`、`mcp2-sacc07-web@4ffb6ba5` 领先 5 笔已推 + 1 件未提交。
- **派 sa-cc/09 → 通道 5**（15:36，task-c4d63735，`mcp5-sacc09` 基 `7c37253f`，估 60–90 分）：5 是 05 信封作者、唯一真空闲。派单第 0 步 `branch-state.ps1 -Path` 核 SA `adapters/inbox` / `adapters/customscompliance` 在途零提交、`git branch` / `ls-remote` 无 `*sacc09*`。取证写进派单：SA 无 inbox / customscompliance 目录、`NewAssessAdvanceRecoveryHandler` 在 `cmd/` 零调用点、CC 读口 `DutyVerificationStore.FindVerification` 已在、CC 事件类型常量未导出（照 lc/27 惯例消费者自写串 + 测试对照）、SA 迁移最大 0018 而 **0019 已钉给 08，09 若加表取 0020**。撞点：`cmd/parcel-dispatch/assemble.go`（与 08 各加一块、占号）、CONTEXT-MAP 各加一条边（不同小节）。
- **在途**：07 步二（2，working）、ve-disc/02（3，催认领中）、sa-cc/04（4，催认领中）、sa-cc/08（6，催认领中）、sa-cc/09（5，刚派）。**归用户**：累计不变（四 crash 成因、开工纪律写进 parallel-sessions.md 否、两件 untracked、远端三旧指针、CI 计费）+ 新增：handoff.md 里写死的 SHA 与「最先要收 05」在暂停后继续跑的那 20 分钟里已失效——交接词写 SHA 就会过期，写「跑 `branch-state.ps1` + 读 tasks.md 末节」不会；下次交接只要这两句。

### 15:3x–16:0x 同一会话：五单全 working；旧单收口；sa-cc/09 一句代码不符裁决；sa-cc/07 步二进 main

- **五单全 working**（15:3x–15:4x）：2 task-564ffa0f（rebase 到 `7c37253f`、接 05 的 `Handoff` 口、占号 `cmd/parcel-api` 四文件 + 55432）；3 task-ce406503（rebase 强推 `a63230f1→b65c651c` / `fdc3d1eb→8a76df61` / `ced1ca72→000d0740`，gofmt 笔 `c51b7a8d`）；4 task-f9d6cd97（树 `reset --hard origin/main` → `7c37253f`）；6 task-da7f289a（rebase → `724dfbc5`，占号 `cmd/parcel-api/main.go` 一块 + `internal/architecture/partition_subject_registry_test.go` SA 块 + CONTEXT-MAP 一条边 + 迁移 SA `0019`；**不碰 `cmd/parcel-dispatch/assemble.go`**）；5 task-c4d63735（sa-cc/09，建树 `mcp5-sacc09`）。簿记 `db0d2963` 15:38 已推。
- **旧单收口**（15:4x，`report_task` 由派发方对自己派的单可标终态）：task-8543732a（07 步一评审，通道 6 全程未认领）/ 42663c03 / b5cadbb5 / a2cb7477 / 6a228ee9（四张被接替的原单）→ failed 作废；task-ea5ac39e（lc/33，09-10 派，作者 crash 后完成记录由推送方代写、已进 main `53173062`）→ done 补收。通道 6 名下另一张 task-a52fb3da 是通道 5 派的，不在通道 1 名下，不代动。
- **sa-cc/09 票面与代码不符 → 裁**（15:5x，通道 5 报）：`AssessAdvanceRecoveryHandler.Assess` 要四值裁决 + 正金额 + 币种，CC 信封与 `FindVerification` 读口都不带金额、消费者也不能代判——票面做法 1「译成 `AssessAdvanceCommand` 一格交 `Assess`」走不通（立票人 Comments 自认没读字段）。**裁：不拦，按通道 5 设计**——同一只 handler 加 UC-SA-001 步 2「结算输入采用」一格 `AdoptDutyPaymentVerification`（新文件，`AdvanceOutcome` 不动、自带小结果代数）、`Deps` 加一口采用登记册、新册只存核对版本引用 + 采用时刻（迁移 `0020`，`0019` 是 08 的）、消费侧读口只包 CC `FindVerification` 只读半边、本步零代垫判断用例钉住。三条边：① 「核对时刻（业务时间）」默认不存（是 CC 事实不是引用），要存须头注写明它属版本身份；② 小结果代数用仓内既有词，`Deps` 口名带 UC 原词，拒 nil 照该构造器既有惯例；③ 地盘扩到 SA `ports` / `adapters/postgres` 新文件 + `0020`，只加新文件不碰 6 的 `evaluation_request*`；票面做法 1 同笔改口、Comments 记不符原句 + 本裁决；「步 4 带金额的 Assess 由谁调」写成候选后继归 SA owner。另直信 5：CONTEXT-MAP 新边插在 `customs-compliance → settlement-accounting` 之后、`settlement-accounting → customs-compliance` 之前（6 插在后者之后，中间隔一行不撞）。
- **完工到 · sa-cc/07 步二 ← 通道 2**（15:51，`mcp2-sacc07-web@e427747d`，代码 `bf8bd5ea`、清点 `56674625`，基 `7c37253f`；rebase 后前三笔 `d77b408c` / `7ee71b20` / `f40e92df` + 七笔新）。git 实测：分支 = origin、作者树干净；main 自 `7c37253f` 起只多 tasks.md 一笔 → 零文件重叠。
- **评审**：3 / 4 / 5 / 6 全在票上无空闲，子代理不再试，**通道 1 直读两轴**（非作者）：**Standards 0 阻断 / 2 非阻断**（① 跨包计数「八格 / 三格」三处，与步一「十三格」同族并入 sa-cc/15；② 真库装配用例三子测试共库且顺序依赖未钉，归作者随手）· **Spec 0 阻断 / 1 非阻断**（既有签名「登记门禁目录」改「登记」，票面没要）。核过的关键点：`UNDECIDED` 分两半的转写方向——`DutyReconciliationReason` 今天只有 `DutyObligationBasisAbsent` 一种业务未决，其余三种全是存储不可用 → 500，正确；提示句封闭词与 `registrationjson` `case` 逐字同；`RegistrationPanel.tsx` 只增不改。作者判断项 ①（`HandoffReference()` 未透出）**不并入 sa-cc/15**——15 零行为、这是答复形改动，归 CC owner 另立小票；**更正 15:0x 节「② 并入 sa-cc/15」那句**，同题同处置。全文落票 07 Comments。
- **重放 + 全量**：`%TEMP%\idp-replay-sacc07web` detached `db0d2963`，`cherry-pick 7c37253f..mcp2-sacc07-web` 11 笔零冲突 → `25377eec`，本票文件对分支 tip 零差；作者清点笔 `b45d0bd6` 在 tip 重生成**零差**、沿用；`gofmt -l` 空、build / vet 0；15:56 占号，带 DSN 全量 **107 ok / 0 FAIL / 15 无测试 / 0 cached**（15:56:26→15:58:33，127 s）；探针 `cmd/parcel-api` + CC http PASS 50 / SKIP 0（新真库装配用例三子测试 PASS）；admin-web `tsc` 0（`npx tsc` 在隔离树找不到编译器、退出码却是 0——**假绿**，要用 `node node_modules/typescript/bin/tsc --noEmit -p .` 真跑）、220 / 220；15:59 释号。
- **进 main**：簿记一笔在 `25377eec` 之上（票 07 Status + 评审 + 进 main 记录、sa-cc spec 07 / 15 行、票 15 软阻句 → 只剩 04、本节）；纯 .md 自审；`ls-remote` 核 `db0d2963` 未动 → ff → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后的广播里）。SHA 对照：`d77b408c→bb526763` / `7ee71b20→b09ac62a` / `f40e92df→86ba8e5a` / `75d10c78→58ac6faa` / `4ffb6ba5→9f391d37` / `3bc1f850→e7553ce2` / `52a8c322→88fe4ccd` / `6f886498→c32e2102` / `bf8bd5ea→9b142bf7` / `56674625→b45d0bd6` / `e427747d→25377eec`。
- **拆树**：`mcp2-sacc07-web` → `merged/`（指针留）、远端删；`D:/tops/idp-parcel-mcp2-sacc07-web`（作者报干净保留到拆树指令）比内容后拆（先 `rmdir` admin-web `node_modules` junction）；`%TEMP%\idp-replay-sacc07web`（同）推完拆。均不加 `--force`。
- **下游**：sa-cc/15 软阻只剩 04；`endpoints.go` 撞点——通道 3（ve-disc/02）加 VE 两行时 rebase 到新 main 后逐块核（2 的三行已进 main）。**在途**四张：ve-disc/02（3）、sa-cc/04（4）、sa-cc/09（5）、sa-cc/08（6）。**候选后继（归 owner）**：CC——`HandoffReference()` 透出到 CLI 与 HTTP 答复面一张小票（05 Spec ② + 07 步二判断项 ①）；SA——09 的「步 4 带金额的 Assess 由谁调」。
- **本节五数**（15:3x–16:0x 段，本机时钟）：CI 未量 · 集成时延 sa-cc/07 步二 15:51→16:0x **约 12 分**（1 票）· 重放笔数 **11**（含作者清点笔，无推送方清点笔）· 重复开发 0 · 会话重置 0（2 / 3 / 4 / 6 本节应答的都是重启后的新会话，重置已计在 14:0x 节的 crash 里，不重复计）

### 16:0x–16:3x 同一会话：lc/36 派出；四票同时完工（ve-disc/02 / sa-cc/04 / 08 / 09）→ 交叉评审；sa-cc/04 + 08 批进 main

- **派 lc/36 → 通道 2**（16:08，task-f20edb3f，`mcp2-lc36` 基 `807de571`；派单第 0 步核无 `*lc36*` 分支、PS `application` 在途零提交；撞点写明 `cmd/parcel-api/main.go` 是 3 / 6 / 36 三方热点，条 1 放最后一笔）。16:09 working；**16:24 完工**（`mcp2-lc36@65a2fbd3`，代码 `b5a7fb81`，清点 `614cda04` 非零差：PS 测试 169→170，条 8 的包内往返用例文件）。
- **完工到（15:53–16:2x，四票）**：ve-disc/02 ← 3（`mcp3-vedisc02@d7b5d5d0`，与刚进 main 的 07 步二同在 `cmd/parcel-api` 四文件不同块 → 按「有 .go 重叠让作者重验」16:1x 请 3 rebase；3 报 16:05 广播后已自行做了大半，16:12 前 rebase 到 `807de571` 零冲突逐块核 + 清点重生成 + 带 DSN 四包 PASS 351 + admin-web，`--force-with-lease` 钉旧 tip：`d7b5d5d0→87449edf`，代码 `ab03d4cd`，清点 `7e6c3d94`）；sa-cc/04 ← 4（16:1x，`mcp4-sacc04@4c764f40`，代码 `d69e18c7`，清点 `3a2454d9`；判据 1 如实记「编排有、装配点今天无」）；sa-cc/08 ← 6（16:2x，`mcp6-sacc08@bedb2a24`，代码 `4ea6a8b4`；6 随后自行 rebase 到 `807de571` 并 `--force-with-lease`：`bedb2a24→70176b0d`，代码 `e81e1240`，清点 `7dfba37d`——推送方核 08 自有文件内容与旧 tip 一致）；sa-cc/09 ← 5（16:10，`mcp5-sacc09@d8535e99`，代码 `17167aa8`，清点 `11da0858`，注释笔 `cb04084e`；带 DSN 九包 PASS 847；迁移 0020；判断项六条，含地盘外 `adapters/partycommercial/contract_responsibility.go` 显式未配置口）。
- **交叉评审（未再点名——四份完工报都是刚到的自报空闲，同 11:2x 节做法；每单「若在做别的回一句我改派」）**：sa-cc/04 → 通道 6（task-93b306bb）、sa-cc/08 → 通道 4（task-5ff84cf2）截止 16:38；ve-disc/02 → 通道 5（task-294871ee）、sa-cc/09 → 通道 3（task-cc969d63）截止 17:05；lc/36 → 通道 6（task-55d2671d，04 评审交完后）截止 17:00。全部 5 分钟内转 working。
- **评审到 · sa-cc/04 ← 通道 6（16:2x，钉 `4c764f40`）**：Standards 0 阻断 / 2 非阻断（最重：`CredentialGateDigest` 用枚举整数入指纹，枚举中间插格旧指纹静默失配，换 `String()`）· Spec 0 阻断 / 2 非阻断（判据 1 装配半边如实缺席，符合「编排或装配」；做法 2「身份 / 版本」折一列，实现对）；五条判断题全站得住，④ 补一句「后继就绪绑引用必须拒绑 UNDECIDED 版」归 CC owner。**处置**：指纹 `String()` 随 sa-cc/06 同通道同目录一并改（派单写明）。全文落票 04。
- **评审到 · sa-cc/08 ← 通道 4（16:20，钉 `70176b0d`）**：Standards 0 阻断 / 3 非阻断（最重：`NewRequestBuyEvaluationHandler` 四口裸 `fmt.Errorf` 不包 `ErrNilDependency`；`EvaluationRequests.Save` 不核键与对象一致；`inserted_at` 从不读回）· Spec 0 阻断 / 3 非阻断（判据 2「技术未形成」措辞、CONTEXT-MAP 新边写的是 11 落地后的形、`SOURCE_NOT_ACCEPTED` 词形窄于格）；四道判断题全对。**处置**：Standards ①② **归 SA owner 一笔小票或随下一张 SA 票收**；其余随票记。全文落票 08。
- **评审到 · ve-disc/02 ← 通道 5（16:24，钉 `87449edf`）**：Standards 0 阻断 / 2 非阻断（`CatalogRegistry` 头注「三口」数别处；`application.CatalogRegistry` 与 `ports.CatalogRegistry` 两包同名不同型）· Spec 0 阻断 / 1 非阻断（地盘外旧「六」字注释改不计数，作者取舍 ④ 已自报，接受）；四条取舍全同意。**可重放**，等 09 / lc/36 评审一起下一批。
- **批进 main（sa-cc/04 + 08）**：评审前先重放（与评审无依赖）：`%TEMP%\idp-replay-w5a` detached `807de571`，先 `cherry-pick` 04 五笔（跳过作者清点 `3a2454d9`）→ `258ff7b3`，再 08 的 rebase 后六笔（跳过作者清点 `7dfba37d`）→ `e0eb667f`，零冲突；两票自有文件对各自分支 tip 零差、互相代码零重叠（只重叠 `spec.md` / 清点两件簿记）。**跳过作者清点、tip 重生成一笔**（两票各自的清点在各自底座上生成、叠加后不再成立）→ **`dcb74785`**（CC 85→88、迁移 17→18；SA 83→89、迁移 18→19、outbox 交接 8→9；`cmd/` 测试 84→85；端口 391→397）。`gofmt -l` 空、build / vet 0；16:17 占号，带 DSN 全量 **108 ok / 0 FAIL / 15 无测试 / 0 cached**（16:17:14→16:19:23，129 s；包数 107→108 是 08 的新包 `adapters/identity`）；探针 `-run 'CredentialGate|EvaluationRequest'` PASS 21 / SKIP 0（两份评审侧「未验」的真库用例在此实跑）；16:19 释号。簿记一笔在其上（票 04 / 08 Status + 评审 + 进 main 记录、sa-cc spec 04 / 08 / 11 / 15 行、票 11 硬阻已解句、票 15 软阻解除句、本节）；`ls-remote` 核 `807de571` 未动 → ff → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后的广播里）。SHA 对照：04 `d59015d3→cb9eec19` / `a6874e0f→2dd947fd` / `2a503c00→f3305906` / `d69e18c7→a32c80eb` / `4c764f40→258ff7b3`；08 `c8e07b5f→ae0cfd24` / `d93d208c→8b1ac98f` / `352dfcfe→c9aae85f` / `d09aed17→8d75bfc7` / `e81e1240→0459dfdf` / `70176b0d→e0eb667f`；推送方清点 `dcb74785`。
- **拆树**：`mcp4-sacc04` / `mcp6-sacc08` → `merged/`（指针留）、远端删；`D:/tops/idp-parcel-mcp4-sacc04` / `D:/tops/idp-parcel-mcp6-sacc08` 比内容后拆；`%TEMP%\idp-replay-w5a` 推完拆。均不加 `--force`。
- **下游**：sa-cc/06 → 通道 4 同通道接（派单带上 04 评审 Standards ① 指纹 `String()`）；sa-cc/15 软阻三票全进 main，可派；sa-cc/11 硬阻已解、仍 draft 等 PP owner 裁入口形状；sa-cc/01 撞点 `cmd/parcel-dispatch/assemble.go` 现在归 09（08 没碰它），等 09 进 main。**下一批**：ve-disc/02（评审已清）+ sa-cc/09（3 评审中）+ lc/36（6 评审中）在新 main 上叠一栈。**候选后继（归 owner）**：SA——08 评审 S①②③（`ErrNilDependency` 包装、`Save` 核键、`inserted_at`）一笔小票；CC——04 评审 ④「就绪绑引用拒绑 UNDECIDED 版」写进后继票；VE——02 评审 S②（`CatalogRegistry` 跨包同名）归 VE owner；PP——11 入口形状一条待裁。
- **本节五数**（16:0x–16:3x 段，本机时钟）：CI 未量 · 集成时延（完工报→推出）sa-cc/04 16:1x→16:3x **约 20 分**、sa-cc/08 16:2x→16:3x **约 12 分**；两票中位 **约 16 分** · 重放笔数 **11**（04 五 + 08 六；推送方清点 1 另计）· 重复开发 0 · 会话重置 0

### 16:3x–17:0x 同一会话：第六波三单派出（06 / 15 / ve-claims/04）；通道 2 / 6 crash；ve-disc/02 + sa-cc/09 + lc/36 批进 main

- **第六波派单**（16:3x，基线 `620f7fed`，未再点名——2 / 4 / 5 刚交完工报 / 评审即派）：**sa-cc/06 → 通道 4**（task-18848394，`mcp4-sacc06`；同目录接 04；派单带上 04 评审 S① 指纹 `String()` 顺手改；迁移钉 CC `0019`）；**sa-cc/15 → 通道 5**（task-0fb34872，`mcp5-sacc15`；软阻已解；撞点 06 的五件放最后）；**ve-claims/04 → 通道 2**（task-2fd800be，`mcp2-veclaims04`；派单第 0 步核无 `*veclaims*` 分支、VE `adapters/partycommercial` 与 `assemble_claims.go` 在途零提交）。4 / 5 五分钟内 working；**2 未见 working**。
- **通道 2 / 6 crash（用户 16:5x 报，问「工作都做完了吧」）**：git 实测——2：lc/36 已完工且安全（`mcp2-lc36@65a2fbd3` = origin，树干净），ve-claims/04 **从未开工**（无树、无分支、无 working 报），无现场可救；6：08 已进 main，手上只剩 lc/36 评审（task-55d2671d），`%TEMP%\idp-review-lc36` 不存在、无回报。**处置**：lc/36 评审改派通道 5（task-f76246c1，17:0x working，17:02 交）；task-55d2671d / task-2fd800be → failed 作废；ve-claims/04 等空闲通道重派。会话重置本节 +2（2 / 6）。
- **sa-cc/15 完工 ← 通道 5（16:51，`mcp5-sacc15@70083dfc`，58 文件 +118 −117 纯注释；`硬句 NNN` 84 处 + 同病「生命周期 NNN」7 处 + 裸 219 一处；行号偏移不止 218 / 219：偏一行十处、偏两行两处，逐处按代码语义核；06 撞点五件放末笔 `04ac89f1`）**；作者如实报一处纪律失误：16:46 跑 CC 包时 shell 残留 09 那票的 DSN，CC postgres + inbox 实际带 DSN 跑了、未占号、与推送方 16:44–16:47 全量撞同一只 55432——两边都绿，票面按判据 4 记「真库未跑」。**评审 → 通道 3**（task-f10ddd5a）：16:59 交，两轴 0 阻断；Standards 1（26 处引文折行，单行 grep 搜不中）+ Spec 1（129 处引文抽 116 逐字在 CONTEXT、13 处是转述，6 处在本 diff 碰过的行）非阻断，同源「引文要能被搜到」。**处置**：本票价值就是引文可搜，两条不修等于白做一半——请通道 5 在 lc/36 评审交完后同分支补一笔（不折行 + 6 处换原句 + 三地盘内同症 7 处一并收），再让通道 3 只重核 Spec 轴那一笔；不挡 02 + 09 + 36 这批。
- **评审到 · lc/36 ← 通道 5（17:02，钉 `65a2fbd3`）**：Standards 0 阻断 / 1 非阻断（`ContinuedAttemptDecisionRequestCoordinates` 类型头注一句未动的旧话「随查询进 PC」与条 6 刚改对的 ports 头注矛盾，归 PS owner 一句）· Spec 0 / 0；两条判断项全同意。全文落票 36。
- **批进 main（ve-disc/02 + sa-cc/09 + lc/36）**：评审前先重放：`%TEMP%\idp-replay-w5b` detached `620f7fed`，`cherry-pick` 02 七笔（作者已 rebase 到 `807de571` 的那一组，跳过清点 `7e6c3d94`）→ 09 五笔（跳过清点 `11da0858`；唯一冲突 `sa-cc spec.md` 09 行与 main 已改的 08 行相邻，纯 .md 推送方按意图解——**第一次解漏了 `<<<<<<<` 一行、第二次漏了 `>>>>>>>` 一行，都在 `--continue` 之后才发现**，两次都 `reset --hard` 回 02 之后重做，第三次用 `checkout --ours` 取 main 侧整文件再只换 09 行、核标记为 0 再 `--continue`——教训：解冲突后 `Select-String '^(<<<<<<<|=======|>>>>>>>)'` 数到 0 再动，不凭肉眼）→ 36 八笔（跳过清点 `614cda04`），代码零冲突 → `8216366b`；三票自有文件对各自分支 tip 零差（差异全归属：08 已进 main 的 `main.go` 块与 CONTEXT-MAP 边、36 的两句头注、09 的路由表与装配函数）。**跳过三笔作者清点、tip 重生成一笔** → **`dfd3c3f5`**（端点 120→122、消费缝 23→24、SA→PC 2→3、消费适配 29→30、直投路由 20→21、SA 89→96、迁移 165→166、PS 测试 169→170、端口 397→399 且基线口径缺 14→13 / 精确口径缺 7→6）。`gofmt -l` 空、build / vet 0；16:44 占号，带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（16:44:37→16:46:47，130 s；包数 108→110 是 09 的两个新包）；探针 PASS 126 / SKIP 0；admin-web `tsc` 0（`node node_modules/typescript/bin/tsc`）、220 / 220；16:47 释号。簿记一笔在其上（票 02 / 09 / 36 Status + 评审 + 进 main 记录、票 09 地盘句坐实「作者自扩、评审与推送方认可」、sa-cc spec 09 行、lc spec 36 行、ve-disc/04 软阻解除句、本节）；`ls-remote` 核 `620f7fed` 未动 → ff → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后的广播里）。SHA 对照：02 `1c88e85a→d312059a` / `c0da5eca→ec1df997` / `389086f4→3a4a8e56` / `cde1be2f→5a617d8b` / `44e9790d→8a235e5d` / `ab03d4cd→3abf1962` / `87449edf→413c4f0c`；09 `c7bbe952→69a1585b` / `e83fd5bd→2c62310d` / `17167aa8→a2832262` / `cb04084e→ff86844b` / `d8535e99→14556746`；36 `a3e130ab→f3b3257a` / `8a4ab2b8→bcaa00bc` / `7b396803→62cba201` / `e0acde49→4dd7b142` / `0c57084f→ee3c3e0b` / `35fc65d8→481e5af4` / `b5a7fb81→efc0c4b5` / `65a2fbd3→8216366b`；推送方清点 `dfd3c3f5`。
- **拆树**：`mcp3-vedisc02` / `mcp5-sacc09` / `mcp2-lc36` → `merged/`（指针留）、远端删；三棵作者树比内容后拆（`mcp2-lc36` 作者已 crash，推送方代拆；`mcp3-vedisc02` 先 `rmdir` admin-web junction）；`%TEMP%\idp-replay-w5b` 推完拆。均不加 `--force`。
- **下游**：ve-disc/04 软阻解除可派；sa-cc/01 撞点 `assemble.go` 已由 09 落定可派；lc/25 撞点同（09 已进）可派；ve-claims/04 重派；sa-cc/11 仍 draft 等 PP owner。**在途**：sa-cc/06（4）、sa-cc/15（5，补一笔后重核）。**候选后继（归 owner）**：SA——08 S①②③ + 09 S①②（`ErrNilDependency` 包装、`Save` 核键、`inserted_at`、空租户核、`ErrUntranslatableReference` 原样上抛）一张小票；PS——36 评审 S① 一句旧头注；VE——02 评审 S②（`CatalogRegistry` 跨包同名）；CC——15 评审点的三地盘外 7 处转述引文。**归用户**：通道 2 / 6 再 crash（今天累计 2 / 3 / 4 / 6 各两次 → 4 / 6 / 2 / 3 / 2 / 6）；通道 5 带残留 DSN 未占号跑真库一次（自报）。
- **本节五数**（16:3x–17:0x 段，本机时钟）：CI 未量 · 集成时延（完工报→推出）ve-disc/02 15:53→17:0x **约 70 分**（含 16:1x 请作者 rebase 与等两份评审）、sa-cc/09 16:10→17:0x **约 55 分**、lc/36 16:24→17:0x **约 40 分**；三票中位 **约 55 分** · 重放笔数 **20**（02 七 + 09 五 + 36 八；推送方清点 1 另计）· 重复开发 0 · 会话重置 **2**（通道 2 / 6 crash）

## 2026-09-11 17:5x 通道 1 新会话接续（原通道 1 17:4x 前 crash；用户先在通道 2 起本会话、要一次自带 `ask_question`、让它监听 idp-mcp-2 → 收到收工令、通道 2 名下残留 working 的 sa-cc/07 单补记 done → 17:5x 改令接管通道 1；接手时 `main = origin/main = 45b9c468`，状态全按 git + `query_tasks` 实测，不靠记忆）

### 17:0x–17:4x 原通道 1 最后一段（从 git 与广播反推，原会话无台账）

- **sa-cc/06 完工 ← 通道 4（17:11，`mcp4-sacc06@2747ee94`，代码 `455cacbd`）** → 重放 `%TEMP%\idp-replay-w5c` detached `45b9c468`：七笔 + 推送方清点 **`9c0b7223`**（作者清点 `d4f1a1e6` 旧底座跳过）；17:16:27→17:18:36 带 DSN 全量 110 ok（17:19 释号广播）。评审派通道 3（task-642a917c，17:15）。
- ve-claims/04 重派通道 3（task-0a43458e，17:10，**pending 至今未认领**）；lc/25 派通道 4（task-c7991ebb，17:25，认领笔 `3034099f` 已推，代码零动）。
- 17:3x 收工令广播。通道 5 的 15 第二笔补笔未成，原通道 1 代提 `wip(sa-cc/15)` **`0b4da637`** 到 `mcp5-sacc15` 并推 origin；起 `%TEMP%\idp-replay-15on06`，在 `9c0b7223` 上 pick 15 十笔（含 wip）→ `11fa6a70`；随后 crash（最后一动是查 spec 06 / 15 行与票 15 尾部，用户把那条命令原样交给本会话重跑）。通道 3 17:48 报 06 评审**未开工**（新会话、无检出、无台账 working），单置 failed。

### 17:5x–18:4x 本会话：接管核、06 评审自跑、06 + 15 进 main

- **接管核**：`git worktree list` 五棵树（共享树 + w5c + 15on06 + `mcp4-lc25` / `mcp4-sacc06` / `mcp5-sacc15`）；`mcp5-sacc15` = origin `0b4da637`、`mcp4-sacc06` = origin `2747ee94`、`mcp4-lc25` = origin `3034099f`；`git range-diff 45b9c468..0b4da637 9c0b7223..11fa6a70` 十笔八 `=`、切片 1 / 4 各少一 hunk（06 已改同两处头注，重放取 06 版）；票 15 判据 1 / 2 三个 grep 在 `11fa6a70` 零。**决定 wip `0b4da637` 不进 main**（未成笔、无评审；纯注释、build / vet 过，但进 main 的门是评审不是编译），15on06 退到 **`af40abf7`**。
- **06 非作者评审**：三路评审子代理全「Authentication error」（与通道 4 16:4x–17:1x 同），推送方按 `/code-review` 正文串行两轴自跑、互不见结论：**Standards 0 阻断 / 3 非阻断**（两处 CONTEXT 引文非原句：『』内层引号、括注去掉无省略号；`attachTo`「七列」；`findings_digest` 列名不再描述内容）· **Spec 0 阻断 / 3 非阻断**（做法 1 程序维实做无、`RegisterGateFinding` 对这一道关门是 ADR-0137 决定三推论但无独立理由格、规则登记面缺席让「规则未配置」今天在生产无解但编排无装配点）——全文落票 06。**15 补笔 `94c87bb6` Spec 重核**代通道 3：46 处引文全部逐字命中、零非注释行、零地盘外文件。
- 18:0x 占号；`af40abf7` 上 `gofmt -l` 空、build / vet 0、带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（18:32:31→18:34:25，113 s）；探针 `ReleaseGate|DutyPaymentGateRule|CredentialGate` CC 包组 PASS 27 / SKIP 0；清点在 tip 重生成 porcelain 空（15 零行为，06 的清点已在 `9c0b7223`，不另成笔）；admin-web `tsc` 0、220 / 220（junction 已 rmdir）。18:4x 释号。
- 簿记一笔在其上（票 06 Status + 评审全文 + 进 main 记录、票 15 Status + 进 main 记录 + wip 处置、sa-cc spec 06 / 15 行、本节）；`ls-remote` 核 `45b9c468` 未动 → ff → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后广播）。SHA 对照：06 `810fbd09→ac6471d4` / `23b7f671→92161d31` / `da4d8369→4136b6e1` / `664116c9→22e99fee` / `99608e4b→76a7fdd0` / `455cacbd→612e9afb` / `2747ee94→a89cd823`；15 `c0134511→f061b834` / `459cf68e→747c8870` / `d4616677→9e90e88b` / `66d0ac75→caad3e20` / `c4e9adf5→3098cf61` / `a1d21ada→54abb795` / `94c87bb6→839e76dc` / `0802f192→7f45c45b` / `74bb70cf→af40abf7`；推送方清点 `9c0b7223`。
- **拆树**：`mcp4-sacc06` → `merged/`（指针留）、远端删、作者树 `D:/tops/idp-parcel-mcp4-sacc06` 比内容后代拆（通道 4 收工报已说树干净等评审）；`mcp5-sacc15` **不改名不拆**（wip 在途）；`%TEMP%\idp-replay-w5c` / `idp-replay-15on06` / `idp-review-sacc06` / `idp-review-sacc15b` 推完拆。均不加 `--force`。
- **在途 / 下游**：lc/25（通道 4，只到认领笔，下次从 `3034099f` 接着做）；ve-claims/04（task-0a43458e pending，通道 3 未认领，下次重派前先 `branch-state.ps1 -Path internal/visibilityexception`）；15 wip 归通道 5（rebase 到新 main 收成正式补笔 + Spec 重核）；sa-cc/11 draft 等 PP owner。**候选后继（归 owner）**：CC——06 评审 S① 两处非原句引文 + S② `attachTo`「七列」+ S③ `findings_digest` 列名一句、Spec ② `RegisterGateFinding` 拒这一道无独立理由格；规则行登记面（CLI / 端点 / 管理台）归 07 家族。**归用户**：原通道 1 crash 一次；评审子代理鉴权错持续一天，非作者评审三次退化为推送方自跑（05 / 07 步一 / 06）——不是纪律问题，是工具问题。
- **本节五数**（17:5x–18:4x 段，本机时钟）：CI 未量 · 集成时延（完工报→推出）sa-cc/06 17:11→18:4x **约 95 分**（含等通道 3 评审未交 + 通道 1 crash）、sa-cc/15 16:51→18:4x **约 115 分**（含 06 先后与补笔）；两票中位 **约 105 分** · 重放笔数 **16**（06 七 + 15 九；推送方清点 1 另计）· 重复开发 0 · 会话重置 **2**（通道 1 crash；通道 3 17:48 自报新会话）

## 2026-09-11 19:2x 通道 1 新会话接续（接手时 `main = origin/main = 8557afd7`；用户先要一次自带 `ask_question`，再「监听队列」→ 通道 4 lc/25 来信 →「这些还在，现在可以 replay 了吗：`D:\tops\idp-parcel-mcp5-sacc15` / `-mcp1-sacc15-tail` / `-mcp4-lc25`」→「需要」）

### 18:4x–19:0x 原通道 1 最后一段（从 git 反推，原会话无台账）

- 上节写「15 wip 归通道 5 收成」，实际原通道 1 19:00 自己收成：新分支 `mcp1-sacc15-tail` 基 main tip `8557afd7` 一笔 **`da52715a`**（wip `0b4da637` 22 件 + 通道 1 收尾 9 件，零行为只注释，含 06 评审 Standards ① 两处非原句引文），未推 origin、未广播、无票面记录，随后会话重置。本节补记，票 15 Comments 19:2x 条同。

### 19:2x–19:3x 来信与接管核

- **lc/25 ← 通道 4**：票面（lc/26 写的 `labelfinal` 包头注「那一段日后加在本包」）与门禁 `TestBusinessModulesDoNotReachIntoEachOther` 不符——`isCrossContextAdapter` 只认 `internal/<consumer>/adapters/<provider>/`，`labelfinal` 不是模块名。核过门禁读法属实、`parcelshipment/adapters/transportfulfillment/` 已存在、三处工作树无未提交；回信同意其按票面 + ADR-0025 落 `adapters/transportfulfillment/` 新文件、调 `labelfinal.ParcelJudgmentCore.JudgeParcel`，并顺手改头注那一句。19:3x 收其占号广播，与通道 1 地盘零重叠，未答。
- **接管核**（`branch-state.ps1 -Classify`，不读自报）：四棵树。`mcp1-sacc15-tail@da52715a` 基 main tip、领先 1 笔、未推、NOT-ABSORBED 31 件；`mcp5-sacc15@0b4da637` 领先 10 笔，`74bb70cf` 及之前已吸收于 `af40abf7`，只剩 wip；`mcp4-lc25` 在途（核对期间 tip `3034099f→1d80eb64`，不动）。共享树「76 行未提交」`--ignore-cr-at-eol` 后真改动 0 件，全是 CRLF 幻影。

### 19:3x–19:5x sa-cc/15 补笔二进 main

- **包含性核**：`%TEMP%\idp-replay-check-15tail` detached `8557afd7` pick `0b4da637` 零冲突 → 与 `da52715a` 比只差通道 1 收尾 9 件，wip 22 件字节一致；核毕拆。
- **判据复跑于 `da52715a`**：1 / 2 三个 grep（含 `CONTEXT\s*NNN`）零；3 地盘外 0、`git diff -U0` 非注释行 0；4 `gofmt -l` 空、build / vet 0、不带 DSN CC + CLI + architecture ok、admin-web `tsc` 0 / 220 pass（junction 已 rmdir）；5 清点 tip 重生成 porcelain 空。**Spec 重核代评审**（按 18:4x 补笔一先例，用户 19:4x「需要」裁可）：`+` 行引文段 74 处、71 处在 docs / .scratch 逐字命中；未命中 3 处（「端点各立三个入口」「一层的译装接到另一层的端点上」「这个请求来自哪个租户」）为基线已有的票面裁决转述、本笔只并行未改字。Standards 轴未做。
- 19:4x 占号广播（55432 + 两地盘）；带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（19:41:18→19:43:25，127 s；CC `adapters/postgres` 8.9 s 真跑）。
- 簿记一笔在其上（票 15 Status + 补笔二进 main 记录、sa-cc spec 15 行、本节）；`ls-remote` 核 `8557afd7` 未动 → 共享树 `merge --ff-only` → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 推后广播）；代码 `da52715a` 不换号，重放 0 笔 cherry-pick。
- **拆树**：`mcp5-sacc15` → `merged/mcp5-sacc15`（wip `0b4da637` 作封存出处，指针留）、远端删、树 `D:/tops/idp-parcel-mcp5-sacc15` 拆；`mcp1-sacc15-tail` → `merged/mcp1-sacc15-tail`、树拆。两树均干净，均不加 `--force`。`mcp4-lc25` 不动。
- **在途 / 下游**：lc/25（通道 4 在做，头注一句已同意）；ve-claims/04（task-0a43458e 是否仍 pending 本节未查）；sa-cc/11 draft 等 PP owner；06 评审余下非阻断（S② `attachTo`「七列」、S③ `findings_digest` 列名、Spec ② `RegisterGateFinding` 理由格）仍归 CC owner——S① 两处已随本笔收。**归用户**：原通道 1 19:0x 成笔后再重置一次（无台账）。
- **本节五数**（19:2x–19:5x，本机时钟）：CI 未量 · 集成时延 sa-cc/15 补笔二 19:00（`da52715a` 成笔，无完工报）→ 19:5x 推出 **约 50 分**（1 票，含原会话重置与用户裁可等待）· 重放笔数 **0**（纯 ff）· 重复开发 0 · 会话重置 **1**（通道 1 本会话新起）

## 2026-09-11 20:4x 通道 1 新会话接续（接手时 `main = origin/main = 1cc61efe`；用户先要一次自带 `ask_question`，再「监听队列」→「可以 replay 了吗：`D:\tops\idp-parcel-mcp4-lc25`」）

### 19:5x–20:0x 原通道 1 最后一段（从 `query_tasks` 记录反推，原会话无台账）

- lc/25 今晚三换手，全在完成记录之前：**通道 4**（task-c7991ebb）19:47 crash，现场 `mcp4-lc25@14b53f57`（票面 in-progress + PS inbox 消费者）+ 两件未跟踪 TF 处理方；用户改派 **通道 3**（task-b28862e4，19:5x）→ `chore(salvage)` 封存两件成 `ea0ba2ec`，随后继续做了装配四件；19:56 用户报通道 3 crash → 改派 **通道 5**（task-252049e0，20:0x）→ 通道 5 20:00 收回：树在 19:56:35 / 19:57:34 落了 `c03eea37`（装配）/ `cbd0ed23`（清点，在 `%TEMP%\idp-lc25-verify` 干净检出重生成）并 rebase 到 `1cc61efe`——持树者（通道 3）当时还在干活，不是死现场；20:0x 用户确认通道 3 成清点笔后才 crash。原通道 1 `force-with-lease` 把 `cbd0ed23` 推上 origin 保现场，在 `cbd0ed23` 上跑了 gofmt 空 / build 0 / vet 0 / 六包不带 DSN ok，20:0x 发 **收尾单 task-c1dd4fa8 → 通道 5**（判据 1–6 自核 + 带 DSN 六包 + 票面完成记录 + 完工报，估 20:45）；通道 5 20:06 认领。原通道 1 之后重置。

### 20:4x–21:1x 本会话：接手核 → 验证 → 点名派评审 → 收尾代写 → 阻断回修 → ff 进 main

- **接手核**（20:47，git + `query_tasks` + `list_sessions`，不读自报）：`mcp4-lc25@cbd0ed23` = origin，干净，19:57 后无写入；merge-base = `1cc61efe` = main = origin/main → **纯 ff、零 cherry-pick**；10 文件 +1239/−36 与共享树 76 行未提交（CRLF 幻影 + `d_tasks.txt` / `docs/api/` 两件 untracked，归用户、未动）零重叠。票 25 Status 仍 in-progress、无完成记录、无评审。**task-c1dd4fa8 working 中但零动静**：MCP-5 最后活动 20:06（接单那一刻）、本机无 go / test 进程、树零改动、自估 20:45 已过。
- **验证先做**（与评审无依赖）：`%TEMP%\idp-replay-lc25` detached `cbd0ed23`：`gofmt -l` 空、build / vet 0、清点 tip 重生成 porcelain 空；20:52 占号广播后带 DSN `go test -p 1 -count=1 ./...` **110 ok / 0 FAIL / 15 无测试 / 0 cached**（20:53:20→20:55:31，131 s）；`-v` 探针 PS `adapters/inbox` + `adapters/transportfulfillment` `-run 'CarrierPickup|PickupRoute'` PASS 31 / SKIP 0（inbox 那包真库实跑）、`cmd/parcel-dispatch` PASS 96 / SKIP 0（含 `TestACarrierFirstEffectivePickupRegisteredReachesTheConsumerThroughTheRouteTable`）；20:56 释号。零 `apps/` 改动，admin-web 不跑。
- **点名 + 催 5**（20:52，截止 20:56）：2 / 3 / 4 / 6 四应答全「空闲 · 地盘无 · 余量充足」（4 自注作者不评）；直信通道 5 限 20:57 报到哪一步 → 20:5x 通道 5 **释单**（task-c1dd4fa8 → failed：20:06 接单的会话已丢失、本会话新开、零进度、树无未推）。**收尾归通道 1**。
- **派评审**（20:5x，两轴拆两人并行、互不引用）：**Spec → 通道 6**（task-cd1f7160；它写过 ADR-0135 与 lc/31，最清楚这封信的形状）、**Standards → 通道 2**（task-e1a7bfa5），各钉 `cbd0ed23`、截止 21:20、自建只读检出、不带 DSN；两单 5 分钟内 working。
- **收尾代写**（21:0x，author tree 上直接成笔——三位持树者都已无会话，通道 5 明言不再动树）：`6e10cd8f` 票 25 完成记录（五笔归属、判据 1–6 逐项对用例名、验证数、地盘 / 红线 / 能力边界）+ Blocked by 两处过时句括注更正：**psr/01 已进 main → `Deps.Validity` 随 `labelFinalJudgmentCore` 实接 `DeclaredLabelValidityRule`**（装配头注已如实写）；**PC 面单渠道两格已由 pc-gaps/12 于 09-10 进 main**（PS `service_stage_rules.go` 逐格翻译），今天的停点是租户未登记面单渠道终局规则 → `FinalRuleUnconfigured`（PAR-COM-17，实例半边），与交付那一路同一个诚实停点。每笔推 origin。
- **评审到**：**Spec ← 通道 6（21:05）**：0 阻断 / 5 非阻断（替代版本 v2 缺直接用例；Validity 过时句；`judge_parcel.go` 头注不在票面地盘（ADR-0134 后才有的包，派单已扩入）；UC-PS-004 括注另笔；判据 6 完成记录待落——后三条已由 `6e10cd8f` 收），重点核 ①–⑨ 全 ✓。**Standards ← 通道 2（21:10）**：**1 阻断**——`judge_on_carrier_first_effective_pickup_test.go` 两条用例头注「CONTEXT 规则 1 / 2」按序号指 PS CONTEXT 无编号列表（AGENTS.md 禁）；4 非阻断（文件头「三路 / 四个读口」计数；类型串钉字面而 TF 常量未导出；「ADR-0049 第三条」写法不一；与交付适配器同形只记不抽）。
- **阻断回修**（推送方，同分支）：`a710a0e7`——两处头注换 PS CONTEXT 原句（「除已经形成的有效取消结果外，实际承运商首次有效收寄即形成终局」前半；「实际承运商首次有效收寄即形成终局」先于「否则，只有在当前受控关闭已经生效且未被重开」那条关闭路径），三处引文单行各命中 1 次、`CONTEXT 规则` 归零，零行为；gofmt 空 / vet 0 / 包 ok；推 origin。21:07 再占号，`a710a0e7` 上 gofmt 空 / build / vet 0 / 带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（21:07:49→21:09:39）/ 清点零差；21:10 释号。**重评 ← 通道 2（21:14，钉 `a710a0e7`）：阻断关闭**，其余四条维持。两轴阻断清零。
- **进 main**：簿记一笔在 `a710a0e7` 之上（票 25 Status → resolved + 两份评审 + 回修 + 重评 + 进 main 记录 + 地盘句更正、lc spec 25 行 → 已进 main、本节）；`ls-remote` 核 `1cc61efe` 未动 → 共享树 `merge --ff-only mcp4-lc25` → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后广播里）；代码 `c03eea37` / 清点 `cbd0ed23` / 完成记录 `6e10cd8f` / 回修 `a710a0e7` **不换号，重放 0 笔 cherry-pick**。
- **拆树**：`mcp4-lc25` → `merged/mcp4-lc25`（指针留）、远端删；`D:/tops/idp-parcel-mcp4-lc25` 干净 = origin、持树者 4 / 3 / 5 均无会话 → 推送方比内容后拆；`%TEMP%\idp-lc25-verify`（通道 3 清点用的干净检出 `c03eea37`，死现场、干净）与 `%TEMP%\idp-replay-lc25` 拆；`idp-review-lc25-spec` 通道 6 已自拆、`idp-review-lc25-std` 归通道 2 自拆。均不加 `--force`。
- **候选后继（归 owner，本节不立）**：**PS**——替代版本 v2 直接用例（Spec 1）；`judge_on_carrier_first_effective_pickup_test.go` 文件头「三路 / 四个读口」计数 + `assemble.go`「ADR-0049 第三条」写法（Standards 1 / 3，可并入 lc/36 同族的下一张）；UC-PS-004 依据表括注「（外部承运轨迹事实）」改口（ADR-0135 越权风险点 5）；`CarrierTrackingFactReference` 是否改名；**「要裁的」2 失效版本重派生**（停在 `ErrVoidedCarrierPickupRederivationUndecided`，ADR-0117 采用版本链是可循的形）。**TF**——导出 `carrierFirstEffectivePickupEventType` 让消费方测试对照（Standards 2）。**前沿**：lc/11「不在本票」三路（25 / 26 / 27）今晚全部进 main；ve-claims/04（task-0a43458e 通道 3 从未认领，本节未重派）、sa-cc/11 draft 等 PP owner；2 / 3 / 4 / 5 / 6 全空闲。
- **归用户**（累计）：一张票一晚三换手（4 → 3 → 5）都断在**代码成笔之后、完成记录之前**，与 09-10 20:3x 节「完成记录单独成笔丢报告」同形——建议把「完成记录随最后一笔代码同笔提交、完工报先于清点」写进 `docs/agents/parallel-sessions.md`，归你拍；原通道 1 20:0x 派出收尾单后再重置（今晚第三次）；票面写死的「未进 main 前填 nil」「PC 两行未立票」两句在被引票进 main 后没人回改——这类跨票状态句是 AGENTS.md「计数与行号同构」那条的又一形；CI 计费、两件 untracked、远端三个旧指针（`mcp3-awf12` / `mcp5-awf13` / `salvage/mcp4-tf03`）不变。
- **本节五数**（20:4x–21:1x，本机时钟）：CI 未量 · 集成时延 lc/25 19:57（清点成笔，无完工报）→ 21:1x 推出 **约 80 分**（1 票；其中 19:57→20:47 是通道 3 / 5 / 原通道 1 接连失去会话到用户来问的空档，本会话内从接手到推出约 30 分）· 重放笔数 **0**（纯 ff，五笔作者 + 两笔推送方收尾 / 回修不换号）· 重复开发 0 · 会话重置 **2**（通道 1 本会话新起；通道 5 自报「20:06 接单的会话已丢失」）

### 21:1x–21:3x 同一会话：用户「下一步我们做什么呢」→「派」——第七波五单派出（2 / 3 / 4 / 5 / 6）

- **前沿算自票面**（`Status:` / `Blocked by:` 行扫全部 `.scratch/*/issues`，main `02e1dfc4`）：311 张——resolved 293 / ready-for-agent **3** / draft 6 / needs-info 4 / blocked 2 / done 2 / superseded 1；in-progress 零、在途分支零（`branch-state.ps1`）、工作树只剩主树；`git branch` / `ls-remote --heads` 对 sacc01 / veclaims / vedisc04 / lc37 / sacc16 零命中，远端仍只余 `mcp3-awf12` / `mcp5-awf13` / `salvage/mcp4-tf03` 三个旧指针。ready 三张的阻塞都已在今晚解掉：sa-cc/01 撞点 `cmd/parcel-dispatch/assemble.go` 由 08 / 09 落定；ve-disc/04 软阻 ve-disc/02 17:0x 进 main；ve-claims/04 地盘节 14:1x 已补、派过两次（2、3）从未开工。
- **答用户**：五单可派（三张 ready + 两张评审尾巴小票即立即做），另列要他拍的四件：① lc/25「要裁的」2（失效版本重派生，B 类，09-10 17:0x 授权覆盖的是当时 13 张 draft、这一格是 ADR-0135 之后长出的）要不要授权按 PS owner 口径代裁落 ADR-0138；② sa-cc/11 / 12 / 13 与 lc/35 四张 draft 各一条要裁的归 owner（09-11 新立、不在旧授权内）；③ `parallel-sessions.md` 加「完成记录随最后一笔代码同笔提交、完工报先于清点」；④ 老三件。用户答「派」，未答授权。
- **点名**（21:21 广播，截止 21:24）：2 / 3 / 4 / 5 / 6 五应答全到、全「空闲 · 地盘无 · 余量充足」。基线一律远端 main `02e1dfc4`。
- **派单（21:2x）**：
  - **sa-cc/01 → 通道 5**（task-35caf74b，`mcp5-sacc01`，估 60–90 分）：PP `evaluation.recorded` 信封的 SA inbox 消费者，裁决 (c)「信封到未决」；取证写进派单——票面「`adapters/inbox/` 新目录」已过时（09 建了，新增文件不动 `duty_payment_verification_consumer`）、`git grep evaluation.recorded -- cmd/` 仍零、08 已进 main 但「形成」路仍不在本票。
  - **ve-claims/04 → 通道 4**（task-c28ddfb5，`mcp4-veclaims04`，估 60–90 分）：ADR-0136 落地——VE→PS 回指窄读缝 + `ClaimServiceRules` 三段 + 退役 `RuleResolutionKeySource` + `assemble_claims.go`；判据 5「先核 PC 闭包快照落不落客户服务规则版本」列为开工第一件事，不对称先报不绕；派单明写「建树即提认领笔」（前两次没起）。
  - **ve-disc/04 → 通道 3**（task-9f7982ab，`mcp3-vedisc04`，估 30–45 分）：VE 注释行号 / 计数改口；取证——`硬句 NNN` 在 `02e1dfc4` 已从票面的 38 处降到 **10 处**、`adapters/partycommercial/**` 下零；**不碰 `adapters/partycommercial/**`**（4 在重写 `claim_service_rules.go`）；02 评审 S② 改名题不并入。
  - **lc/37 立 + 做 → 通道 2**（task-c778df52，`mcp2-lc37`，估 45 分）：lc/25 两轴非阻断五条——替代版本 v2 直接用例、测试文件头「三路 / 四个读口」计数、`ADR-0049 第三条` 写法、UC-PS-004 依据表括注改口（ADR-0135 越权风险点 5）、TF 导出 `carrierFirstEffectivePickupEventType` 供对照（门禁不许 inbox 测试 import TF 时对照放 `cmd/parcel-dispatch/assemble_test.go`）；不做改名与要裁的 2。
  - **sa-cc/16 立 + 做 → 通道 6**（task-63bd3adc，`mcp6-sacc16`，估 30–45 分）：08 / 09 评审 Standards 五条——`NewRequestBuyEvaluationHandler` 包 `ErrNilDependency`、`EvaluationRequests.Save` 核键、`inserted_at` 二选一（不改 0019；(a) 新迁移 `0021` 去列 / (b) 头注写明审计列，推送方倾向 (b)）、`AdoptDutyPaymentVerification` 空租户先拒、`ErrUntranslatableReference` 上抛路径只核不改。
- **撞点已写进各单**：`cmd/parcel-dispatch/assemble.go`（5 路由行 + 装配函数 vs 2 一句头注，不同块、占号）；`internal/visibilityexception/**`（3 只注释且**不碰 `adapters/partycommercial/**`** vs 4 只动该包 + `cmd/parcel-api/assemble_claims.go`）；SA（5 `adapters/inbox/` 新文件 vs 6 `application/` 两件 + `adapters/postgres/evaluation_request.go` + `adapters/customscompliance/` 一件）。五单都写进：**完成记录随最后一笔代码同笔提交、完工报先于清点**（今晚 lc/25 的教训，本波起当硬要求）；建树后立即 working + 认领笔 + push；共享接线文件动前占号；注释不写行号 / 计数。
- **评审位不留**：谁先完工谁交叉评（作者不评自己；2 是 lc/25 Standards 评审者，评 lc/37 算作者不评；6 是 08 作者评 16 算作者不评）。**归用户**（累计）：上面四件 + 老三件。

## 2026-09-11 22:2x 通道 1 新会话接续（本会话先绑通道 2：用户要一次自带 `ask_question` → 监听 idp-mcp-2 →「这些都完成了吗，可以 replay 了吗，只有你自己一个通道」五树问答 →「全面解决」→ 22:2x「mcp-1 crash，请你接管」改听 idp-mcp-1；接手时 `main = origin/main = 6a4fd7c8`，状态全按 git + `branch-state.ps1` 实测，不靠记忆）

### 21:3x–22:1x 原通道 1 最后一段（从 git 与 `%TEMP%` 残留反推，原会话无台账）

- 21:3x 派单簿记 `6a4fd7c8` 推出后：**21:45 封存通道 4 现场**——`mcp4-veclaims04@3730ef63` `chore(salvage)`（`claim_service_rules{,_test}.go` 三段改写中 + 退役 `RuleResolutionKeySource`、`assemble_claims.go` 新 `buildClaimEligibilityRules`、`assemble_claims_test.go` 改写中途未编译；mtime 21:36–21:42）；**21:5x 裁 sa-cc/16 要裁的 1「准」**（票面已记，通道 6 补笔 `ffbe1f14` 21:50）；建 `%TEMP%\idp-review-lc37@345f4aff` 与 `%TEMP%\idp-review-sacc16@ffbe1f14` 两棵评审检出（无任何评审记录）；在 `%TEMP%\idp-replay-sacc16`（detached `6a4fd7c8`）`cherry-pick main..mcp6-sacc16` 六笔到 `3049ca1b`（未全量、未 ff、未推）；随后 crash。通道 2 / 4 / 5 / 6 的会话也在 21:5x–22:1x 间陆续结束（用户 22:2x「没有任何其他的通道了」）；通道 5 把 sa-cc/01 八件暂存后没来得及提交（见下）。

### 22:1x–22:2x 五树问答（在通道 2 上答）

- `branch-state.ps1` + 逐树 `status` / `log main..tip` / `merge-base`：**lc/37**（`mcp2-lc37@345f4aff`，六笔 = origin，干净，票面完工待评审）与 **sa-cc/16**（`mcp6-sacc16@ffbe1f14`，六笔 = origin，干净，已重放到 `3049ca1b`）可 replay、只差非作者评审；**sa-cc/01**（`mcp5-sacc01@0f8757ee` 只有认领笔，树上八件 780 行**已暂存未提交**、票面写的「代码 + 完成记录同笔…分支已推 origin」那一笔从未落地；gofmt / build / vet 过）差一步；**ve-claims/04**（`3730ef63` 封存的未编译半成品）与 **ve-disc/04**（`mcp3-vedisc04@4377f28c` 只有 Status → in-progress 一笔）无可 replay。另报：**CI main 计费红**（最近 5 次 run 全 failure、1–4 秒即挂，`gh run view` 注解「recent account payments have failed or your spending limit needs to be increased」——机器门不在，门只剩本机带 DSN 全量）；共享树 76 行未提交全 CRLF 幻影（`--ignore-cr-at-eol` 真改动 0），6 件 untracked（`d_tasks.txt` / `handoff.md` 空文件、`docs/api/` 一件、三个 logo svg）归用户未动。用户答「全面解决」+ 改令接管通道 1。

### 22:2x–22:5x 本会话：sa-cc/01 封存 → 点名 → 三份评审 → 叠栈 → 清点 → 全量 → 簿记 → ff → push

- **sa-cc/01 封存**（22:28）：在 `D:/tops/idp-parcel-mcp5-sacc01` 核索引 == 工作副本、无未跟踪件、mtime 21:35:12–21:48:49 后无写入、通道 5 无会话 → `chore(salvage)` **`ddc88003`** 原样入库一字不改（提交信写证据不写结论），推 origin。
- **点名**（22:30 广播，截止 22:33）：`list_sessions` 2 / 4 / 5 / 6 仍显 online 但均无会话；**通道 3 应答空闲**（22:31）；用户 22:32 报「通道 2 crash」。三份评审的分法：lc/37 与 sa-cc/16 由通道 1 直读两轴（作者 2 / 6 无会话，无他人空闲；子代理评审今天鉴权错四次，不再试）；**sa-cc/01 评审 → 通道 3**（task-933c2f59，钉 `ddc88003`，22:55 止），22:3x working、**22:42 交**（13 分钟）。
- **评审结论**：lc/37 ← 通道 1：Standards 0 阻断 / 2 非阻断（`assemble.go` 对 ADR-0049「第三条 / 决定三」两种写法并存、测试文件头「五值」计数——归 PS owner）· Spec 0 / 2（括注改口与「红」的证法，均接受）。sa-cc/16 ← 通道 1：Standards 0 / 1（`ErrNilDependency` 文本「pre-acceptance control」对 buy-evaluation 编排是错话，归 SA owner）· Spec 0 / 0。sa-cc/01 ← 通道 3：Standards 0 / 2（「三件引用 / 五种结果」跨文件计数；PP 夹具第三份）· Spec 0 / 3（清点未补——推送方兑；判据 2「应用层」改「处理方」；(c) 窗口期 `consumer_undecided` 留库信封需后继票带重放动作）；三道判断题全表态，判断题 3 点出 `UNRATABLE` 与 AT-SA-175 的张力要写进形成路后继票。三份全文各落票面 Comments。
- **叠栈**：`%TEMP%\idp-replay-sacc16@3049ca1b` 上 `cherry-pick main..mcp2-lc37` 六笔 → `cherry-pick main..mcp5-sacc01` 两笔，**零冲突**（`assemble.go` / `assemble_test.go` 两票 hunk 逐一核为不同块；`sa-cc spec.md` 16 行与 01 行不同行）→ 批 tip **`3c57811d`**（14 笔）；三票自有文件对各自分支 tip 只差同批他票的块；全树差 = 三分支文件的并集（23 件），无外来件。
- **清点**：先在 main 干净 detached 检出（`%TEMP%\idp-inv-main`）重生成核与已提交版本**逐字同**（main 的清点是现行的），再在批 tip 生成两次结果一致 → 推送方清点笔 **`fe975e1b`**（SA 生产 96→98 / 测试 76→78、合计 962→964 / 916→918、SA→PP 消费缝 1→2、消费适配器 31→32、直投路由表 22→23——全是 sa-cc/01，lc/37 与 sa-cc/16 零差）。**一处自己的差错**：第一次读 `%TEMP%\inv-diff.txt` 看到的是 15:5x 某会话留下的旧文件（CC 84→85、端点 117→120），差点当成本批数字——`%TEMP%` 里复用文件名的取证不可信，此后每次用带时刻的文件名。
- **验证**：`3c57811d` 上 `gofmt -l` 空、`go build ./...` / `go vet ./...` 0；22:41 占号广播，**`fe975e1b` 带 DSN 全量 `go test -p 1 -count=1 ./...` 110 ok / 0 FAIL / 15 无测试 / 0 cached**（22:41:35→22:43:49，134 s）；`-v` 探针九包 PASS 59 / SKIP 0 / FAIL 0（`cmd/parcel-dispatch` 与 SA `adapters/postgres` 真库实跑）；22:44 释号广播。零 `apps/` 改动，admin-web 不跑。
- **进 main**：簿记一笔在 `fe975e1b` 之上（票 37 / 16 / 01 Status + 三份评审全文 + 处置 + 进 main 记录、lc spec 37 行、sa-cc spec 01 / 16 行、本节）；纯 .md 自审；`ls-remote` 核 `6a4fd7c8` 未动 → 共享树 `merge --ff-only` → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后广播）。SHA 对照：sa-cc/16 `2cef827c→a11abb96` / `9d48e8ca→33f2272f` / `0d25b898→c9d6f6e7` / `2aa0c463→324aa474` / `dab69b4f→4776b1b8` / `ffbe1f14→3049ca1b`；lc/37 `b89540cc→7a0b4827` / `5cd5c099→3e9ed3a6` / `aa1a570b→73d729c7` / `ca228065→86736403` / `ed64901a→0c0c10ae` / `345f4aff→8c56b6b1`；sa-cc/01 `0f8757ee→5cdd85ad` / `ddc88003→3c57811d`；推送方清点 `fe975e1b`。
- **拆树**：`mcp2-lc37` / `mcp6-sacc16` / `mcp5-sacc01` → `merged/`（指针留）、远端删；三棵作者树比内容后拆（持树者均无会话）；`%TEMP%\idp-replay-sacc16` / `idp-review-lc37` / `idp-review-sacc16` / `idp-inv-main` 推完拆。`mcp3-vedisc04`（只有认领笔）→ `salvage/mcp3-vedisc04`、树拆；`mcp4-veclaims04@3730ef63`（封存半成品）**不改名不拆**，等重派「从 `3730ef63` 接着做」。均不加 `--force`。
- **在途 / 下游**：ve-disc/04（重派候选：通道 3，基新 main 另起分支）、ve-claims/04（重派候选：从 `3730ef63` 接着，需先读封存的三件半成品）；sa-cc/11 draft 等 PP owner。**候选后继（归 owner）**：**PS**——`assemble.go` 其余「ADR-0049 第三条」统一为带引文的「决定三」+ `judge_on_carrier_first_effective_pickup_test.go` 文件头「五值」计数、`CarrierTrackingFactReference` 改名、lc/25 要裁的 2 失效版本重派生；**SA**——`ErrNilDependency` 文本去「pre-acceptance control」限定；sa-cc/01 注释「三件引用 / 五种结果」计数换点名；形成路后继票要（i）带窗口期 `consumer_undecided` 留库信封的重放动作、（ii）让 `UNRATABLE` / `CONFLICT` 落入账（AT-SA-175）；**PP**——`syntheticPricingEvaluation` 第三份同形夹具抽成导出合成夹具。
- **归用户**（累计）：CI 计费（今晚 main 五次 run 全未起跑，机器门不在）；六件 untracked（新增 `handoff.md` 空文件与三个 logo svg）；远端三个旧指针；原通道 1 与通道 2 / 4 / 5 / 6 21:5x–22:3x 间全部失去会话（本节会话重置计法见下）；作者「已暂存未提交」这一格是今晚新的一种丢法——暂存与提交之间断掉，`git status` 里看着比未暂存更像「做完了」，仍靠推送方手工封存。
- **本节五数**（22:2x–22:5x，本机时钟）：CI main 绿/总 **0/5（五次全为计费未起跑，非代码红）** · 集成时延（完工报→推出）lc/37 21:4x→22:5x **约 70 分**、sa-cc/16 21:5x→22:5x **约 60 分**、sa-cc/01 22:0x（作者票面自标，无完工报）→22:5x **约 50 分**；三票中位 **约 60 分**（其中 21:5x→22:2x 是原通道 1 crash 到用户来问的空档；本会话内从接手到推出约 35 分）· 重放笔数 **14**（16 六 + 37 六 + 01 二；推送方清点 1 另计）· 重复开发 0 · 会话重置 **2**（通道 1 本会话新起；通道 2 用户 22:32 报 crash）——通道 4 / 5 / 6 的会话结束由用户 22:2x 一句「只余通道 1」带过、未逐个报，不入数、记在此

### 22:5x–23:2x 同一会话：ve-disc/04 派出 → 完工 → 推送方两轴 → 纯 ff 进 main

- **派单**（22:5x，task-7309fe70 → 通道 3，未再点名：它 22:42 刚交完 sa-cc/01 评审并报 done；基远端 main `609e334b`，另起 `mcp3-vedisc04-2`，旧同名分支已 `salvage/`）。派单取证里我写的「`硬句 NNN` 在 `609e334b` 已降到 10 处 / 9 文件」**是错数**：用 PowerShell 管道 `ForEach-Object { ($_ -split ':')[1] } | Group-Object` 数 `git grep` 的中文输出，GBK 解码把多行并成少行（[workflow.md 的本机环境] 点名过的坑，读过照样踩），真值 38 处 / 12 文件与票面 `0b027ab8` 同数——作者开工第一件事就量出来纠正了。此后取数一律 UTF-8 写文件再数、模式走 `-f` 文件。
- **完工到**（23:08，`mcp3-vedisc04-2@67be7173`，两笔：认领 `1e2eab1c` + 代码与完成记录同笔 `67be7173`；18 件 +86/−95 全注释；38 处行号 **38 处全部指错行**，偏移分四族 18 / 20 / 21 / 27 行，另 3 处原始编号即错、2 处引文不逐字；同症裸行号 13 处一并收；10 处计数改点名；三条判断项）。作者树干净 = origin。
- **评审 ← 通道 1**（非作者；钉 `67be7173`）：Standards 0 阻断 / 2 非阻断（判断项 1 四处跨文件计数归 VE owner 与 02 评审 S② 合一张；判断项 2 同文件计数不算违反）· Spec 0 / 2（真库由推送方闭合；同症 13 处补收超出「缺口」但同病同红线，接受）。脚本核：181 条改动行全 `//` 起头；19 件全在 VE + 票面；`adapters/partycommercial/` 与 `CONTEXT.md` 零；`+` 行「」去重 49 条中 44 条逐字命中 CONTEXT、5 条是票面完成标准名与普通词；无折行；判据 1 / 2 与同症两式 UTF-8 模式文件在 tip 全零、同式在 main 38 命中。全文落票。
- **进 main**：`merge-base = main` → **纯 ff、零 cherry-pick、SHA 不换**。`%TEMP%\idp-replay-vedisc04b` detached `67be7173`：`gofmt -l` 空、build / vet 0；23:10 占号，带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（23:10:55→23:13:07，132 s）；清点 porcelain 空；23:13 释号。簿记一笔在其上（票 04 Status + 评审 + 进 main 记录、本节）；`ls-remote` 核 `609e334b` 未动 → ff → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后广播）。`mcp3-vedisc04-2` → `merged/`、远端删；作者树与 `%TEMP%\idp-replay-vedisc04b` 比内容后拆。
- **下游**：ve-claims/04 → 通道 3 另单（从 `mcp4-veclaims04@3730ef63` 接着做，树 `D:/tops/idp-parcel-mcp4-veclaims04` 在；`assemble_claims_test.go` 未编译点 `buildClaimsOrchestrationWith` 已删仍被引）。**候选后继（归 VE owner）**：判断项 1 四处跨文件计数（「七件」「六件必备」「五个装载口」「两组表」）+ 02 评审 S②。**归用户**（新增）：本会话推送方一次错数（上）。
- **本节五数**（22:5x–23:2x）：CI 未起跑（计费）· 集成时延 ve-disc/04 23:08→23:2x **约 15 分**（1 票）· 重放笔数 **0**（纯 ff）· 重复开发 0 · 会话重置 0

### 23:1x–23:5x 同一会话：ve-claims/04 接手派出 → 中途报 PS 闸 → 裁 (a) + 立 ps-port-remainder/09 → 完工 → 评审 → 重放进 main

- **派单**（23:18，task-6afdcc42 → 通道 3，刚交 ve-disc/04 即派）：从 `mcp4-veclaims04@3730ef63`（21:45 封存的通道 4 半成品）接着做，先 rebase 到 `262e8c0a`；判据 5「PC 闭包快照对称否」列为第一步，不对称先报不绕；接手别人在途实现先写自己的 red 再读对方（parallel-sessions.md「镜像测试」）。通道 3 23:2x working：rebase 零冲突 `a41e1669→f0a454cf` / `3730ef63→3d54438e`，接手笔 `5cead8f5`。
- **中途报**（23:3x 机时）：判据 5 **对称**（`documentOfClosure` / `closure()` 逐项通写通读，`RehydrateCommercialClosure` 只校三类附件）；封存三件已按做法 1–3 落齐、与它先写的 red 逐格同形（哨兵名都一致）；**堵点是 PS 地盘**：解析键登记面不收 `CUSTOMER_SERVICE_RULE`——`0007` 立、`0008` 重加的 CHECK `commercial_resolution_key_registration_bases_closed` 白名单无此格 + `commercialKindFrom` 名集无 `CustomerServiceRuleObject`（通道 1 复核 `git grep` 于 `262e8c0a` 零命中）。ADR-0136 越权风险点 2「可登不必登」的前提不成立，是不可登。
- **裁 (a)**（23:3x）：「已登记」态闭包由测试直给 PC 造、经 PC 真解析库落，PS 真委托仓储 Decide → Save 形成回指，三段读路全真库；「未采用 CSR → 未登记」态真走 PS 登记面（只含合同）；票面判据 3 划掉留痕改口；ADR-0136 不改，归 owner 补一句实测；**立 [ps-port-remainder/09](ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md)**（两道闸 + 「必登否」归 PS owner，机制半边 ready）直接推 main **`aa2e6a07`**（23:32，纯 .md 自审）。不取 (b) 挂起：机制与开闸互不依赖。
- **完工到**（23:39，`mcp4-veclaims04@3dc07230`，四笔：`f0a454cf` / `3d54438e` / `5cead8f5` / `3dc07230`；+1146/−412 于 VE `adapters/partycommercial` 三件 + `cmd/parcel-api/assemble_claims*` + 票面；带 DSN 三包 ok；判据 1–6 逐项，3 改口；三条判断项）。作者树干净 = origin。
- **评审 ← 通道 1**（非作者，钉 `3dc07230`）：Standards 0 阻断 / 2 非阻断（`buildClaimEligibilityRules` 头注「四个协作方」跨文件计数；`NewClaimServiceRules` 拒 nil 裸 `fmt.Errorf` 不包哨兵——沿既有写法，与 sa-cc/16 ① 同族，归 VE owner）· Spec 0 / 3（判据 3 改口；负断言绊线**留**、翻线写进 09 的做法 4 与地盘；两份适配器测试并存留；重建门造闭包接受）。结果代数逐格对 ADR-0136 决定二第三段 / 决定三核过；没有时钟、键、默认范围；退役五件零命中。全文落票。
- **重放**：`%TEMP%\idp-replay-veclaims04` detached `aa2e6a07`，`cherry-pick 262e8c0a..mcp4-veclaims04` 四笔零冲突 → `bf722146`，本票文件对分支 tip 零差；`gofmt -l` 空、build / vet 0；清点 tip 重生成 → 推送方清点笔 **`637245e7`**（VE 测试 92→93、合计 918→919；票面预报的「VE→PS 消费缝 +1」按生成器不成立——窄口落在 `adapters/partycommercial` 包内，清点按目录归 VE→PC 缝，如实记）。23:45 占号，`637245e7` 带 DSN 全量与探针见下一条。
- **验证结果 + 进 main**：`637245e7` 带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（23:45:02→23:47:12，130 s）；`-v` 探针 `cmd/parcel-api` + VE `adapters/partycommercial` `-run 'ClaimServiceRules|WiredClaims|NewClaimServiceRules'` **PASS 40 / SKIP 0 / FAIL 0**（三态真库用例实跑）；23:47 释号。簿记一笔在其上（票 ve-claims/04 Status + 判据 3 改口留痕 + 裁决末条 + 评审 + 进 main 记录、ps-port-remainder/09 做法 4 / 地盘 / 软阻解除、本节）；`ls-remote` 核 `aa2e6a07` 未动 → 共享树 `merge --ff-only` → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后广播）。SHA 对照：`f0a454cf→943d6f2c` / `3d54438e→e4814324` / `5cead8f5→876e4280` / `3dc07230→bf722146`；推送方清点 `637245e7`。`mcp4-veclaims04` → `merged/`、远端删；作者树 `D:/tops/idp-parcel-mcp4-veclaims04`（通道 3 已退出）与 `%TEMP%\idp-replay-veclaims04` 比内容后拆。**今晚五树至此全部收口**：lc/37、sa-cc/16、sa-cc/01、ve-disc/04、ve-claims/04 五票进 main；新立 ps-port-remainder/09 ready。**前沿 ready**：ps-port-remainder/09（PS，任何空闲通道）；draft 归 owner：sa-cc/11 / 12 / 13、lc/35；lc/25 要裁的 2 待授权。**候选后继（归 owner）**：见各票「进 main 记录」；汇总——PS：`assemble.go`「ADR-0049 第三条」统一 + 「五值」计数、`CarrierTrackingFactReference` 改名、lc/25 要裁的 2；SA：`ErrNilDependency` 文本、sa-cc/01 注释计数、形成路后继票带重放动作 + UNRATABLE / CONFLICT 落入账；VE：ve-disc/04 判断项 1 四处计数 + 02 评审 S②、`assemble_claims.go`「四个协作方」计数、`NewClaimServiceRules` 包哨兵、ADR-0136 越权风险点 2 补实测；PP：`syntheticPricingEvaluation` 第三份夹具。
- **漏了一笔、补叠（23:49–23:5x，推送方自己的差错）**：通道 3 在 23:39 完工报之后按我 23:3x 的四条条件又成了一笔 **`98b8cd19`**（23:48 推 origin，同时发了「替代 23:39 那份」的完工报与 23:49 的提醒）；我 23:44 重放的是 `3dc07230` 为止的四笔，23:49:04 `push 4e969ef9:main` 之前**没有再排一次队列**——那两条消息当时就躺在队列里。这是「落笔前再排一次，尤其是共享接线文件」那一格的推送方版本：完工报到手与 push 之间隔了十分钟（评审 + 重放 + 全量），窗口正好够作者再成一笔。随后 `git branch -m` 与远端删都做在了 `98b8cd19` 上（拆树前作者树干净 = 已提交，所以内容没丢，只是没进 main）。**补救**：`%TEMP%\idp-replay-veclaims04b` detached `4e969ef9`，`cherry-pick 98b8cd19` → 代码零冲突、票面三处冲突按意图解（保留「已进 main」前缀与推送方评审 / 进 main 记录，取作者更新的 23:4x 段、判据 3 改口与 Comments 条；作者「裁决」5 与推送方同题 bullet 合成一条，标记数到 0 才 `--continue`）→ **`98b8cd19→aa5bf548`**；该笔推送方直读：四条条件逐条落地（态二真解析器 + `fixedClock{at: key.Anchor.At()}`、态一真走登记面、退役 `seedClaimRuleClosure`、`acceptOnRealAssemblyWith` 参数化、头注引 09），0 阻断。`aa5bf548` 的 gofmt / build / vet / 带 DSN 全量 / 探针 / 清点零差见下一行；再 ff → `push <sha>:main`。**处方（写给下一个推送方）**：`push` 之前的最后一步是 `check_messages(waitMs 0)` + `git ls-remote --heads origin <作者分支>` 核 tip 是否仍是你重放的那一个——两秒钟，挡的就是这一格。
- **补叠验证 + 进 main**：`aa5bf548` 上 `gofmt -l` 空、`go build ./...` / `go vet ./...` 0；23:54 占号，带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（23:54:18→23:56:30）；探针 `cmd/parcel-api -run WiredClaims` PASS 2 / SKIP 0（三态用例按改形后的夹具真库实跑）；清点 tip 重生成零差（不增删文件）；23:57 释号。簿记一笔在其上（票 04 进 main 记录补叠段、本节两条）；`ls-remote` 核 `4e969ef9` 未动 → 共享树 `merge --ff-only` → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 在推后广播）。SHA 对照补一行：`98b8cd19→aa5bf548`。`merged/mcp4-veclaims04`（tip `98b8cd19`）自此与 main 内容一致。
- **本节五数**（23:1x–23:5x）：CI 未起跑（计费）· 集成时延 ve-claims/04 23:39→23:49 **约 10 分**（首批四笔）、第五笔 23:48→23:5x **约 10 分**（1 票，两段）· 重放笔数 **5**（四 + 补叠一；推送方清点 1 另计）· 重复开发 0 · 会话重置 0 · 推送方漏叠 **1 次**（上）

### 00:1x–00:2x 通道 1 由原通道 3 会话接管（原通道 1 crash）：ps-port-remainder/09 点名零应答 → 用户裁「作者自评进 main」→ 纯 ff → main

- **接管**（00:1x，用户经队列告知「mcp-1 crash，请你接管」）：本会话此前是通道 3，刚以作者身份交完 ps-port-remainder/09（`mcp3-psr09@9fa2ddc6`，00:09 完工报）。接管即申明：**本会话不能自评自放**，评审要派别人。
- **点名**（00:14 广播、截止 00:17）：`list_sessions` 只有通道 1 / 3 在线（同一会话），通道 2 与其余全部离线；**0 个应答**。非作者评审派给通道 2 排队（task-50cd7049，钉 `9fa2ddc6`）——它上线即接。用户 00:2x：「现在只有你自己了」。
- **重放先做不依赖评审的半边**：`merge-base(main, mcp3-psr09) = main` → 纯 ff。`%TEMP%\idp-replay-psr09` detached `9fa2ddc6`，清点在其上重生成 **`590282b0`**（PS 测试 172→173、合计 919→920；迁移 parcel_shipment 21→22、合计 167→168；生产 / 端口 / 消费缝 / 路由表零差）。`590282b0` 上 `gofmt -l` 空、`go build ./...` / `go vet ./...` 0；00:18 占号，带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（00:19:22→00:21:30）；00:21 释号。
- **无非作者评审怎么办 → 问用户**：三选一（自评进 main 并如实记 / 挂起等他人 / 用户自己评）。用户选 **「作者两轴自评、票面与 tasks.md 如实记『作者自评、无他人会话』、留后继等他人补非作者评审」**。作者两轴自评全文在票 09「进 main 记录」：Standards 0 阻断 / 0 非阻断（迁移形、白名单完整、0007 / 0008 零 diff、ADR 引文逐字、名集对齐、注释纪律、不预填键）；Spec 0 阻断 / 1 非阻断（判断项 1 两客户共用 VE 声明——VE 册既有键形，接受）。**证据层级**：这是「作者自评」，与「非作者评审 0 阻断」不是同一级，票面 Status 与本节都写成这个词，不写成「评审 0 阻断」。
- **进 main**：簿记一笔在 `590282b0` 之上（票 09 Status 加「已进 main」前缀 + 进 main 记录；ve-claims/04「裁决」5 追「后继已落」句；本节）；`ls-remote` 核 `1b06bb18` 未动 → 共享树 `merge --ff-only` → `push <sha>:main`。**远端 main = 本笔 tip**（SHA 推后在下一节或 Comments 补）。SHA 不换：`7348bce6` / `9fa2ddc6` 原样进 main。
- **拆树**：`mcp3-psr09` → `merged/`（指针留本地、远端删）；作者树 `D:/tops/idp-parcel-mcp3-psr09`（干净）与 `%TEMP%\idp-replay-psr09` 比内容后拆。
- **后继**：① task-50cd7049 留在通道 2 队列——后补非作者评审（钉 `9fa2ddc6`，已进 main；若有发现按回修票走）；② PS owner：09「要裁的」1「必登」；③ ADR-0136 owner：越权风险点 2 补 `262e8c0a` 实测一句（可登 → 不可登，09 已开闸）；④ 昨夜候选后继表原样（tasks.md 23:1x–23:5x 节）。
- **本节五数**（00:1x–00:2x）：CI 未起跑（计费）· 集成时延 ps-port-remainder/09 00:09→00:2x **约 15 分**（1 票）· 重放笔数 **0**（纯 ff；推送方清点 1 另计）· 重复开发 0 · 会话重置 **1**（通道 1 crash，由通道 3 会话接管）· **非作者评审缺席 1 次**（用户裁，后补）

## 2026-09-14 08:4x 通道 1 新会话（用户要一次自带 `ask_question` → 监听 idp-mcp-1 →「ready-for-agent 为零，下一步还剩下什么」→「派吧」；接手时 `main = origin/main = cdf17834`，2026-09-12 00:27 之后零提交，在途分支零；状态全按 git + `.scratch` 全量 `Status:` 扫描实测）

### 08:4x–08:5x 盘点：ready-for-agent = 0 之后剩什么

- **`.scratch` 全量 `Status:` 分布**（`Select-String` 逐文件首个 `Status:`，UTF-8）：resolved 330 · in-progress 7（四个 spec + tenant-implementation-01 三件）· done 7 · superseded 6 · **draft 6**（lc/20、lc/22、lc/35、sa-cc/11、12、13）· **needs-info 4**（nr-route-evidence-views/01、ps-external-mark-relations/01、syn-wall-door-audit/01、ve-008/04）· **blocked 2**（auto-reroute/02、first-tenant-runway/03）· open 1（ve-ops-tracking/03 的旧状态行残留，首行已 resolved，不算）。**ready-for-agent 0。**
- **四类**：① 六张 draft 各差 owner 一句裁决（sa-cc/11 PP 入口形；sa-cc/12 付款人是规则格还是 `PAR-CUS-*`；sa-cc/13 版本维 + 触发重核对；lc/35 重放对账甲/乙；lc/20、22 等第一家真源）；② 已进 main 的尾巴（psr/09 非作者评审欠着；psr/09「必登」与 lc/25 要裁的 2 归 PS owner；ADR-0136 越权风险点 2 补实测；昨夜候选后继表的 A 类）；③ 纯簿记（ps-port-remainder spec 九子票全 resolved 未关）；④ 六张 needs-info / blocked 全是实例半边（`PAR-NET-14` / `PAR-INT-01` / 真实租户接入），等租户不等工程。
- **过期任务记录**：`query_tasks all` 里五条 pending / working 全是 09-11 晚 crash 会话留下、工作已进 main 的单（task-f72d6362 ve-claims/04 重派、task-55995e48 lc/37 评审、task-ad449e18 ve-disc/04 重派、task-35caf74b sa-cc/01、task-0a43458e ve-claims/04 首派）——**推送方逐条 `report_task done` 代关**，result 写进 main 的 SHA 与「不要再接」；不关的话新会话上线一读队列就会把 ve-disc/04 / ve-claims/04 再做一遍（09-07 pc-gaps/09 那种孤儿）。task-50cd7049（psr/09 非作者评审 → 通道 2）仍 pending，是真的。
- **归用户（沿昨夜）**：CI 计费红；六件 untracked（`d_tasks.txt` / `handoff.md` 空、`docs/api/` 一件、三个 logo svg）；共享树 76 行 CRLF 幻影（`--ignore-cr-at-eol` 真改动 0，本会话复核）。

### 08:5x 用户裁「派吧」→ 点名 → 三件派出 → 收口 ps-port-remainder spec

- **点名**（08:52 广播、截止 08:55）：`list_sessions` 通道 2 / 3 / 4 running-idle，5–11 offline；**3 个应答**（2 / 3 / 4，全「空闲 · 地盘无 · 余量充足」，均在截止前到）。`branch-state.ps1`：在途分支零、只有共享树 76 行幻影。
- **派单三件**（分法按地盘不撞）：**通道 2 ← psr/09 非作者两轴评审**（沿用 task-50cd7049 不另开单，`send_to_session` 刷新指针：分支已改名 `merged/mcp3-psr09`、diff = `1b06bb18..9fa2ddc6`、只读不带 DSN、截止 09:20；作者是原通道 3 会话，通道 2 非作者）。**通道 3 ← task-0962cb6b VE 尾巴合票自立自做**（`ve-disclosure-policy-view/05`，分支 `mcp3-vetails` 基 `cdf17834`；五条：ve-disc/04 判断项 1 四处跨文件计数、ve-disc/02 S② `application.CatalogRegistry` 改名、`assemble_claims.go`「四个协作方」计数、`NewClaimServiceRules` 拒 nil 包哨兵、ADR-0136 越权风险点 2 追加补记不改写）。**通道 4 ← task-65ce5275 PS / SA / PP 尾巴两票一分支**（lc/38 + sa-cc/17，分支 `mcp4-tails`；六条：`ADR-0049 第三条`→带引文「决定三」四处（推送方 `git grep` 实测 `cmd/parcel-dispatch/assemble.go` 两处 + `internal/platform/dispatch/direct_publisher_test.go` 两处，`assemble_test.go` 零）、`judge_on_carrier_first_effective_pickup_test.go`「五值」、`CarrierTrackingFactReference` 改名判（lc/25 裁决已把判权交实施者）、`ErrNilDependency` 文本去限定、sa-cc/01「三件引用 / 五种结果」、PP `syntheticPricingEvaluation` 第三份抽成导出合成夹具）。PS + SA + PP 合派一个通道的理由：三家都撞 `cmd/parcel-dispatch/assemble.go` / `assemble_test.go`。**明写不在本批**：sa-cc/01 处置 ③「形成路后继票」（设计题）、lc/25 要裁的 2、psr/09「必登」、lc/37「只记不抽」的 helper。
- **收口 ps-port-remainder spec**（本笔）：九子票 `Status:` 逐文件核全 resolved → spec `Status: resolved`，子票表补 08 / 09 两行（此前只列到 07）；只关状态不改票面正文；owner 尾巴（09 必登、09 评审后补、02 消费适配器段票面自记 Blocked by PC）留在票里。
- **本节五数**（08:4x–08:5x）：CI 未起跑（计费）· 集成时延 —（本节无票进 main）· 重放笔数 0 · 重复开发 0（五条过期任务代关，堵的就是这一格）· 会话重置 **1**（通道 1 本会话新起；原会话 09-12 00:2x 后结束）· 非作者评审缺席 —（psr/09 后补已派）

### 09:0x psr/09 非作者评审后补到手 → 落票 → 尾巴追加给通道 4

- **评审 ← 通道 2**（09:03，钉 `9fa2ddc6`，只读不带 DSN，约 8 分）：Standards 0 阻断 / 1 非阻断（`commercialKindFrom` 头注「名集与白名单是同一集合的两份镜像」被代码反证——名集含 `PriceRuleObject`、0022 白名单无 `PRICE_RULE`，那格由 `validate` 与 CHECK 另拦）· Spec 0 / 1（两客户共用一份 `RegisterClaimEligibility` 与生产同形，接受）。全文落票 09 Comments；Status 首句改口为「非作者评审已后补」，「进 main 记录」后继 ① 划掉；ps-port-remainder spec 状态行同步。**证据层级**：psr/09 自此是「非作者评审 0 阻断」，昨夜「作者自评」那一格补齐。
- **处置**：Standards ① 是 PS 地盘纯注释 → 不另开票，`send_to_session` 追加为 lc/38 第 7 条（通道 4 正在 `internal/parcelshipment/**` 上，同一分支顺手收）；Spec ① 接受不改。
- **在途**：通道 3 task-0962cb6b（VE 五条）、通道 4 task-65ce5275（PS / SA / PP 六条 + 追加 1）均 working；通道 2 空闲——两份完工报到手后各派对方评审。（上一稿这里写「3 ↔ 4 互评会撞『作者评自己』」是错话：各评对方的票就是非作者，不撞；留痕改口。）

### 09:1x–09:4x 通道 2 交活后 crash → 通道 3 做完未成笔即 crash → 封存 → 推送方直读两轴 → 重放进 main

- **通道 2**（用户 09:1x 报「做完然后 crash」）：评审全文 09:03 已到手并落票进 main（`3ee64735`），只读检出 `%TEMP%\idp-review-psr09-0855` 已自拆，`git worktree list` 无残留；零损失。
- **通道 3**（用户 09:3x 报「做完了，然后 crash」）——**不信自报，查 git**：`mcp3-vetails@66c6168c` = origin，五笔（立票 + 条 1–4）已推；树上 **2 件已修改未提交**（条 5 ADR-0136 补记 + 票面 Status → resolved）、0 件未跟踪、mtime 09:25:32 / 09:27:33 在最后一笔 09:24:15 之后、完成记录节仍「待填」、task-0962cb6b 仍 working、无完工报。**与 09-11 晚 sa-cc/01 同一种丢法**（代码到手、完成记录没成笔）。封存：`chore(salvage)` **`b9fbd0dd`** 原样入库一字不改（提交信写证据），推 origin。
- **非作者评审 ← 通道 1 直读**（通道 2 crash、通道 4 在做自己的票；钉 `b9fbd0dd`）：Standards 0 阻断 / 2 非阻断（`TestAnETADemandsItsSevenParts` 用例名带「Seven」——标识符不在条 1「只许注释行」红线内，作者不动是对的；`NewCatalogRegistration` 拒 nil 文本「catalog registry is required」没随 `CatalogRegistries` 改口）· Spec 0 / 2（完成记录缺席由推送方代写、判据 5「只有追加行」按行数不成立但原句是新行逐字节前缀）。脚本核：判据 1–4 零命中 / 同数 / PASS；`-U0` 非注释差 0；`ports.CatalogRegistry` 引用 3 = 3；`production_wiring_baseline.txt` 零差。全文落票 05 Comments。
- **重放**：`merge-base = cdf17834`，main 已前进两笔纯 .md → `%TEMP%\idp-replay-vetails-0933` detached `3ee64735`，`cherry-pick cdf17834..mcp3-vetails` 六笔零冲突 → tip **`a34f439c`**；`gofmt -l` 空、build / vet 0；09:35 占号，带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（09:37→09:39，133 s）；探针 PASS 四子例；清点 `tools/mechanism-inventory`（自带 go.mod，要在它目录里 `go run . -dir ../..`——仓根 `go run ./tools/...` 报「main module does not contain package」）干净检出重生成 porcelain 空；09:4x 释号。**完成记录由推送方代写**（lc/25 先例）：只写 git 与脚本能证的，作者判断项只取提交信与 Comments 里作者写过的。簿记一笔在其上（票 05 Status / 完成记录 / 评审 / 进 main 记录；psr/09 后继 ③ 划掉；本节）→ `ls-remote` 核 `3ee64735` 未动 → ff → `push <sha>:main`。SHA 对照见票 05「进 main 记录」。`mcp3-vetails` → `merged/`、远端删；作者树与重放树比内容后拆。
- **候选后继（归 VE owner）**：`TestAnETADemandsItsSevenParts` 改名 + `NewCatalogRegistration` 拒 nil 文本随名改口，两处一笔。**归用户（新增）**：通道 2、3 今早各 crash 一次（均在交活之后 / 最后一笔之后），会话重置累计 3。
- **本节五数**（09:1x–09:4x）：CI 未起跑（计费）· 集成时延 ve-disc/05 09:27（树上最后一次写入）→09:4x **约 20 分**（1 票）· 重放笔数 **6**（推送方封存 1 含在内）· 重复开发 0 · 会话重置 **2**（通道 2、通道 3）· 非作者评审缺席 0（推送方直读，非作者）· **作者完成记录缺席 1**（推送方代写）

### 09:4x–10:0x 通道 4 完工（lc/38 + sa-cc/17）→ 追派条补笔 → 推送方直读两轴 → 九笔重放进 main；今早三件全部收口

- **完工报**（09:4x，task-65ce5275 done）：八笔 = origin，树干净，两票 resolved + 完成记录在 `96bd5d3a`。**少一条**：推送方 09:0x `send_to_session` 追派的 lc/38 第 7 条（psr/09 评审 Standards ①）**从未进通道 4 队列**（通道 4 自报「队列里确实没有」，首次见到是完工报回执里的重发）→ 通道 4 10:0x 补笔 **`c569ea36`**（`commercialKindFrom` 头注 + 同文件 `FormResolutionKey` 同句错话一并改口，只注释）。**处方**：追派用 `dispatch_task`（有任务记录、可 `query_tasks` 对账），不用裸 `send_to_session`——裸消息丢了两边都不知道。
- **通道 4 报的与派单前提不符处（全部成立、接受）**：条 1 实得**十四处四文件**（派单写四处两文件；`assemble.go` 十处 + `assemble_test.go` 一句 + PS `advance_acceptance_chain_test.go` 一句 + 平台两句）；条 5 派单点名的 `buy_evaluation_recorded_consumer_test.go` 零计数（推送方写错）；条 6 三份夹具**形同值不同**（价表 12.5 / 10、取整 None 1kg / Ceiling 0.5kg、金额取整三态、引用带不带指纹），「零参数共用」与「值一字不变」不可兼得 → 取「规格显式、零默认值」；`pptest` 是普通包 → 清点记 **PP 生产 +1** 不是「测试 +1」（推送方预报口径错，`filecensus` 按后缀）。**推送方派单前的重量又错了三处**——派单里写的数字一律是取证时刻的观察，作者开工第一件事重量是纪律不是客气。
- **55432 撞号一次**：推送方 09:35 占号与通道 4 09:35 占号广播同分发出，双方各在本轮结束才读到对方；通道 4 实跑 09:36:01→09:36:10 落在推送方 09:37→09:39 之前，未真正重叠；两侧都绿。教训同「落笔前再排一次」：**占号前先 `check_messages(waitMs 0)` 排一次队列**。
- **非作者评审 ← 通道 1 直读**（钉 `c569ea36`）：lc/38 Standards 0 / 2（`Covers:` 引文嵌套「」是原文自带；`newRequiredValue` 两串口名是错误文本变化）· Spec 0 / 2（收平台 + PS application 两处非地盘 `Covers:`、收生产文件头注——同句只注释，接受）；sa-cc/17 Standards 0 / 3（`pptest` 记生产 +1 的语义；08 那批仍有五处计数归 SA owner；`pptest.Value` 三份同形归 PP owner）· Spec 0 / 2（值不变证据是作者未入库的探针，以带 DSN 全量 + 探针旁证；完成记录随下一笔）。脚本核：`ADR-0049 第三条` 0、旧名 0（TF / VE `ExternalCarrierTrackingFact` 族 38 处是另一类事实未动）、`pre-acceptance control dependency` 0、sa-cc/01 那批计数 0、`pptest` 非测试 import 0、`production_wiring_baseline.txt` 零差。全文落两票 Comments。
- **重放**：`merge-base = cdf17834`，main 已到 `7b4cffc1` → `%TEMP%\idp-replay-tails-0958` detached `7b4cffc1`，`cherry-pick cdf17834..mcp4-tails` 八笔 + `c569ea36` 零冲突 → tip **`9cf30fab`**；`gofmt -l` 空、build / vet 0；10:0x 占号（这次先排了队列），带 DSN 全量 **110 ok / 0 FAIL / 16 无测试 / 0 cached**（133 s；无测试 15→16 是 `pptest` 包）；`-v` 探针四例 PASS；清点重生成 **PP 生产 88→89、合计 964→965** → 推送方清点笔 **`69406e6d`**；释号。簿记一笔在其上（两票 Status / 评审 / 进 main 记录；psr/09 Standards ① 标已落；本节）→ `ls-remote` 核 `7b4cffc1` 未动 → ff → `push <sha>:main`。SHA 对照九笔在 lc/38「进 main 记录」。`mcp4-tails` → `merged/`、远端删；作者树与重放树比内容后拆。
- **今早三件全部收口**：psr/09 非作者评审（通道 2）、ve-disc/05（通道 3 + 推送方封存代写）、lc/38 + sa-cc/17（通道 4）。**前沿 ready = 0**。**候选后继（归 owner，全是小活）**：VE——`TestAnETADemandsItsSevenParts` 改名 + `NewCatalogRegistration` 拒 nil 文本随 `CatalogRegistries` 改口；SA——08 那批五处「三件引用 / 五种结果」换点名；PP——`pptest.Value` / `evaluationValue` / `saTestValue` 三份同形合一。**等裁决**：lc/35、psr/09 必登、sa-cc/11 / 12 / 13、lc/25 要裁的 2（建议裁法已 09:4x 经队列给用户）。
- **本节五数**（09:4x–10:0x）：CI 未起跑（计费）· 集成时延 lc/38 + sa-cc/17 09:45（`96bd5d3a`）→10:0x **约 20 分**（2 票，同分支）· 重放笔数 **9**（推送方清点 1 另计）· 重复开发 0 · 会话重置 0 · 非作者评审缺席 0（推送方直读）· **通道间消息丢失 1 次**（追派条）· **推送方派单取证错数 3 处**（条 1 处数、条 5 第四份、条 6 清点口径）

### 10:1x–10:2x 用户「授权代裁」→ 六条裁决落票、五张转 ready、一张新立 → A 类尾巴第二批派通道 4

- **先派再裁**（10:1x，task-c0000fd2 → 通道 4，唯一在线通道）：今早三批评审留下的 A 类尾巴合票自立自做——ve-disc/06（`TestAnETADemandsItsSevenParts` 改名、`NewCatalogRegistration` 拒 nil 文本随名改口）+ sa-cc/18（SA 08 那批五处计数换点名、PP 三份 `Value` 助手合一），分支 `mcp4-tails2` 基 `6bbf2bf0`。与裁决票零重叠（它不碰待裁票面）。
- **代裁六条**（用户 10:1x 一句「授权代裁」，我按 09:4x 给过的建议裁法写票，各票「要裁的」标「已裁」+ 新增「裁决」小节 + Status 转 ready-for-agent）：
  - **lc/35** 取甲（建立重放先查交易已存在则跳择优）；乙不做——「决定写下时交易还没落」是票 23 读面的语义题，等它有真实消费者再裁；多一次读的代价认下。
  - **psr/09 必登** 取 (a) 不必登（理由照作者完成记录：租户可合法没有客户服务规则版本，ADR-0104 决定三那一类；必登会让接受流整行拒登）。零代码——今天就是 (a)；ADR-0136 越权风险点 2 补记末尾追一句「（二）已裁 (a)」。
  - **sa-cc/12** 「真实程序要求付款人」= 登记的规则一格（形照 ADR-0137 决定三规则型目录行），不是 `PAR-CUS-*`；三停格：要求而未提供 → 待确认点名缺付款人；不要求 → 显式「未提供」照常核对；未登记要不要 → 未配置点名缺规则。「来源未提供」取领域上一格不取 `(string, bool)`。
  - **sa-cc/13** 要版本维（一版本一行，形作者定，`0016` 不改，幂等键（引用 + 版本））；触发重核对不在本票，只到「登记册看得见新版本与回指」，作者完成记录写清后继票的形，推送方据此立。
  - **sa-cc/11**（先量再裁）：`NewEvaluatePricingHandler` 非测试零调用、`Handle` 只收已成形 `domain.EvaluationRequest`、PP ports 无「按（租户、范围、方向、目的、时点）解析在用价卡」读口、`EvaluationRequest` 无回指 → 走票面 (b)：立「按评价请求形成评价」入口（复用 `EvaluatePricingHandler.Handle` 不改它）+ 评价上「评价请求回指」一格（不进 semanticDigest）+ 在用价卡解析读口三格（唯一 / 未配置 / 适用冲突，不种选择规则）+ 输入取值不可得则显式未决点名缺哪只读口 + PP 以消费者形式接、CONTEXT-MAP 加 SA→PP 边；**范围比原估大一倍以上，允许拆前置票（回指 + 解析读口），不允许绕未决格**。
  - **lc/25 要裁的 2** 不一句裁——**立 lc/39**（ready-for-agent，Type: grilling → ADR 草案）：`/domain-modeling` 三场景（失效后无新收寄 / 又来替代版本 / 已有受控关闭）、候选甲（循 ADR-0117 形成回指前版的新采用判断）/ 乙（终局不退、追加「依据已失效」标记交人核）各写反方，落 `docs/adr/0138-*`（`Proposed`，序号开工重取）；实现另立票；哨兵一字不动。lc spec 子票表加 39 行。
- **证据层级**：六条全是「用户授权、推送方以 owner 口径代裁」，各票「裁决」小节首行都写了这一句；不是 owner 亲裁。sa-cc spec 状态行同步。
- **前沿 ready**（五张）：lc/35（PS）、lc/39（PS，ADR 草案）、sa-cc/11（PP，大）、sa-cc/12（CC）、sa-cc/13（CC）。在途：通道 4 task-c0000fd2。**下一轮派单**：通道 4 交活后点名；只有它在线时按「小 → 大」派：sa-cc/12 → sa-cc/13（同 CC 地盘、迁移序号要错开，串行为宜）→ lc/35 → lc/39 → sa-cc/11；若用户开新通道，CC 两张与 PS 两张可并行（地盘不撞），sa-cc/11 单独一人。
- **本节五数**（10:1x–10:2x）：CI 未起跑（计费）· 集成时延 —（本节无票进 main，只有簿记）· 重放笔数 0 · 重复开发 0 · 会话重置 0 · **代裁 6 条**（其中 1 条改为立 ADR 票不裁）

### 10:2x–10:4x 通道 4 交 tails2（ve-disc/06 + sa-cc/18）→ 直读两轴 → 五笔重放进 main；同时派 sa-cc/12

- **完工报**（10:2x，task-c0000fd2 done）：五笔 = origin，树干净，两票完成记录各随该票最后一笔代码同 commit（上一张的那一格这次守住了）。条 1（SA 08 那批计数）重量实得十三句七文件——派单写六处，又多两处同族（推送方取证第四次错数，同一种病：取证时刻的观察被当成清单）。`saTestValue` 判留（十七处全喂 SA / CC 类型，`pptest` 不该当跨上下文通用工具）。
- **先派再评**（10:3x，task-d6d4f149 → 通道 4）：sa-cc/12（CC，裁决已写实；分支 `mcp4-sacc12` 基 `1ca125a5`，迁移序号钉 `customs_compliance/0020`，规则目录形照 0019 那一族；真库必跑，占号前先排队列）。地盘查过 `branch-state.ps1 -Path internal/customscompliance`：在途零。
- **非作者评审 ← 通道 1 直读**（钉 `a4da9748`）：ve-disc/06 两轴 0 / 0；sa-cc/18 Standards 0 / 0 · Spec 0 / 1（`request_buy_evaluation.go` 头注数同文件字段却改了，与 `domain/evaluation_request.go`「不动」不一致——接受，边界写进票面）。作者四条判断项全接受。
- **重放**：`%TEMP%\idp-replay-tails2-1035` detached `1ca125a5`，`cherry-pick 6bbf2bf0..mcp4-tails2` 五笔零冲突（sa-cc spec.md 我的状态行改动与它新增的 18 行不同 hunk，自动合）→ tip **`cb440073`**；`gofmt -l` 空、build / vet 0；10:3x 先排队列再占号，带 DSN 全量 **110 ok / 0 FAIL / 16 无测试 / 0 cached**（132 s）；探针两例 PASS；清点重生成 porcelain 空；10:4x 释号。簿记一笔在其上（两票 Status / 评审 / 进 main 记录；本节）→ `ls-remote` 核 `1ca125a5` 未动 → ff → `push <sha>:main`。SHA 对照五笔在 ve-disc/06「进 main 记录」。`mcp4-tails2` → `merged/`、远端删；两树比内容后拆。
- **前沿**：ready 四张（lc/35、lc/39、sa-cc/11、sa-cc/13）；在途 sa-cc/12（通道 4）。今早自 08:4x 起进 main 的票：psr/09 评审后补、ve-disc/05、lc/38、sa-cc/17、ve-disc/06、sa-cc/18 六张 + 六条裁决簿记。
- **本节五数**（10:2x–10:4x）：CI 未起跑（计费）· 集成时延 tails2 10:20（`a4da9748`）→10:4x **约 25 分**（2 票，同分支；其中约 10 分在写六条裁决）· 重放笔数 **5** · 重复开发 0 · 会话重置 0 · 非作者评审缺席 0 · **推送方派单取证错数 1 处**（条 1 处数）

### 12:0x–12:1x 用户问 sa-cc/12 能否重放 → 三门未过；随后报通道 4 crash → 现场封存 `7f5db322`、接管单预派通道 4

- **11:5x 用户问「`idp-parcel-mcp4-sacc12` 可以 replay 了吗」**：不能。树上五份未提交（+85/−31：`reconcile_duty_payment.go`、`registrationjson/translate.go`、三处 `cmd/*` 装配各一行）；tip `97ba925a` 题头自标「之一」；票面 `in-progress`、无完工报、无非作者评审。顺手核：分支 = origin；merge-base `1ca125a5` 以来 main 前进 7 笔（ve-disc/06、sa-cc/18 + 簿记），main 侧 `.go`/`.sql` 全在 SA / PP / VE，与分支 13 份 CC 文件零重叠，未提交五份亦零重叠——到时可直接重放，不必作者重验。
- **12:0x 用户报通道 4 crash**。量现场：五份 mtime 停在 10:52:06–10:53:23（tip 提于 10:50:04），自此未动；`go build ./...` 0；`application` 十例红同一因（夹具未装 `PayerRules`，构造门拒「payer requirement rule view」）、`domain` 绿。封存内容是「之二」：`VerifyDutyPaymentCommand` 加 `Procedure`、deps 加 `PayerRules ports.PayerRequirementRuleView` 进构造门、`VerifyPayment` 两道前置后读规则出三停格（未决 `PayerRequirementNotConfigured` / `PayerRequiredNotProvided` / `PayerRequirementViewUnavailable`）、`procedureRef` 必填入 JSON 译装、三处装配接 `PayerRules`——**已按 `32ec44bd` 改口落**（未决 + reason，不加新 outcome）。
- **推送方代封存** `chore(salvage)` **`7f5db322`**（原样入库一字不改；提交信写 mtime 与用户报时，不写「已死」）并推 origin；树干净、= origin。`task-d6d4f149` 结 failed「承接方换人（crash）」；**接管单 `task-8d2df3e3`** 预派通道 4（high）：第 1 步读封存 diff 报 ≤10 行接不接受其形；接受则 `reset --soft` 拆回随测试成正式笔、封存笔留 `salvage/` 指针不进 main；剩余七项列明（夹具 → 三停格 + 读口故障各一例 → `procedureRef` 译装例 → 真库 0020 往返 + 规则册三格 → cmd 装配测试 → 判据 1–3 → 完工报）。用户重启通道 4 即接；改指别的通道我改派。
- **原单一处与实做不符、接受**：派单写「不碰 `assemble.go`」，但构造门对每一口一视同仁，三处装配各一行 `PayerRules: reconciliation` 是必需。
- **在途**：sa-cc/12（接管单待通道 4 上线）。前沿 ready 四张不变（lc/35、lc/39、sa-cc/11、sa-cc/13）；sa-cc/13 与 12 同 CC 地盘，迁移序号待 12 落后再钉、串行派。
- **本节五数**（12:0x–12:1x）：CI 未起跑（计费）· 集成时延 —（无票进 main）· 重放笔数 0 · 重复开发 0 · **会话重置 1**（用户报通道 4 crash）· 封存 1 笔

### 12:2x–13:1x 用户改指「不要考虑 mcp-4 了，你看看怎么干」→ 推送方自办 sa-cc/12 之二 / 之三 → 重放进 main（非作者评审缺席，用户授权自决 → 放行 + 补评审）

- **改指**（12:2x）：`task-8d2df3e3` 接管单结 failed「用户改指推送方自办」（从未开工）。树 `D:/tops/idp-parcel-mcp4-sacc12` 上切 `mcp1-sacc12` 基 `7f5db322`，`reset --soft HEAD~1` 把封存拆回工作区随用例重切；`mcp4-sacc12` 指针留原位。本会话既是推送方又是作者——利害在此写明。
- **之二 `5ef53c6e`**：封存那半（`VerifyDutyPaymentCommand.Procedure`、`PayerRules` 进构造门、`VerifyPayment` 两道前置后读规则出三 reason、`procedureRef` 必填、三处装配）+ 本会话续写：application 双替身加规则读口、`TestThePayerDimensionIsJudgedByTheProcedureRule` 三格 + `TestThePayerRuleIsReadAfterBothPrerequisitesAndNamesItsOwnFailure`（读口故障 / 程序空白 / 前置先于规则）；HTTP `businessUndecided` 把付款人两格业务未决归 200 带 reason（ADR-0022：形成了的答案不折 5xx）+ 转写四格；CLI `TestExecuteDutyPaymentVerificationPayerGridsKeepTheirExitCodes` + `procedureRef` 缺席 / 空白拒；SA 消费侧替身 `unreachedDutyStores.LoadPayerRequirement` 守「入向登记不读规则」；`cmd/parcel-api` 装配测试 JSON 补 `procedureRef`。夹具修绿的那十来例同一因（构造门拒缺口），一处 `PayerRules: store` 了事。
- **之三 `2ed944c4`**：裁决 1 的「登记面」——`RegisterCaseConfigurationHandler.RegisterPayerRequirement`（形照 0019 那一格：同键同值`已存在`、换值`内容冲突`不顶替、缺格`未受理`、依赖故障未决）+ `RegisterCaseConfigurationDeps.PayerRules` / `PayerRuleView` 在 `cmd/parcel-api` 与 `cmd/parcel-customs-register` 组合根接上（该 handler 无构造门，测试零改）。与 0019 一样今天没有 HTTP / CLI 子命令收它，归后继。
- **`9e3d6434`**：票面取 main 版再落 Status resolved + 完成记录（判据 1–3 逐项；判据 3 复核 sa-cc/03 越权风险点 (b)：不对称撤销、CC 取「明确记录」，理由 CC CONTEXT「来源提供**或**真实程序要求的」——入向登记那步没有真实程序可对，拒收等于替所有程序预设「要」）+ 判断项四条（监管程序由调用方交、不核与案件一致；`procedureRef` 必填是裁决 1 后果、`admin-web` `presentation.ts` 键名帮助文本因此过时归后继一行；登记面无 HTTP / CLI 口；核对记录不带付款人维）+ 三处注释计数换点名（`dutyReconciliationAnswer` 头注「三种存储不可用」因本票加一格读口故障**已变假**——AGENTS「不用计数」那条的活例）。
- **验**：每笔 gofmt / build / vet 0；不带 DSN CC 七包 + cmd 三包 + architecture 全 ok；带 DSN（`2ed944c4`）同组 + migrate 12 ok，探针 `TestAFundsFactWithoutAPayerRoundTripsAsExplicitlyNotProvided` / `TestPayerRequirementRulesRoundTripPerProcedure` PASS 非 SKIP。
- **非作者评审缺席**：`/code-review` 隔离子代理（generalPurpose × 4、explore × 1）连续「Authentication error」；MCP-2/3 `running` 但最后活动 8:5x / 9:3x、MCP-4 已弃。作者两轴自查写进票面并明标不是非作者评审。用户 13:0x「你作为业务和系统专家，请你自决」→ **放行 + 补评审**：等的一侧没有 ETA 且 sa-cc/13 同地盘排在后面；推的一侧带 DSN 全仓绿、之一与之二编排半边出自另一会话而由本会话逐行读过、当前无租户、新停格只在「未提供」或规则未登时触发、票面如实标注。补评审条目写在票面「进 main 记录」下（基线 `bf1ae577`，范围 `831cfbef..1ec18258`），子代理或任一通道恢复即跑。
- **重放**：`%TEMP%\idp-replay-sacc12` detached `bf1ae577`，`cherry-pick 1ca125a5..mcp1-sacc12` 五笔零冲突（票面 .md 与 `32ec44bd` 两侧同文自动并）→ `3ede6a27→48cf37dd` / `97ba925a→831cfbef` / `5ef53c6e→f3957007` / `2ed944c4→8de78629` / `9e3d6434→1ec18258`；清点重生成 → **`699c9b49`**（CC 生产 90→92、测试 91→93、端口 40→41；合计 965→967；迁移 168→169、CC 19→20；端口声明 402→404）；gofmt / build / vet 0；13:0x 先排队列再占号，带 DSN 全仓 `-p 1 -count=1` **110 ok / 0 FAIL / 16 无测试 / 0 cached**（131 s）；释号。簿记一笔在其上（票 12 Status「已进 main」+ 进 main 记录 + 补评审条；spec.md 12 行；本节）→ `ls-remote` 核 `bf1ae577` 未动 → ff → `push <sha>:main`。`mcp1-sacc12` → `merged/`、`mcp4-sacc12` → `salvage/`（`7f5db322` 不进 main），远端两支删；作者树与重放树比内容后拆。
- **前沿**：ready 四张（lc/35、lc/39、sa-cc/11、sa-cc/13）；sa-cc/13 与 12 同 CC 地盘、迁移序号钉 `customs_compliance/0021`，可派。在途零。**后继两条**：sa-cc/12 补评审（子代理 / 通道恢复即跑）；`admin-web` 核对键名帮助文本补 `procedureRef`（一行，归 admin-web）。
- **本节五数**（12:2x–13:1x）：CI 未起跑（计费）· 集成时延 sa-cc/12 12:5x（`9e3d6434`）→13:1x **约 20 分**（1 票；作者 = 推送方，无队列等待）· 重放笔数 **5**（清点 1 另计）· 重复开发 0 · 会话重置 0（上节已记通道 4 那次）· **非作者评审缺席 1**（子代理不可用，用户授权自决放行）· 子代理调用失败 5 次

### 13:2x–14:0x 用户「请你自决」→ 推送方自办 sa-cc/13（CC 入向登记版本维）→ 进 main；立后继 19 / 20

- **盘点后选票**（13:1x 用户问「还有未完成的任务吗」→ 在途 0、ready 四张、后继两条、draft 两张、blocked 两张卡 `PAR-NET-14`；用户「自决」）：按「小 → 大」且与刚进 main 的 12 同 CC 地盘，先 sa-cc/13。分支 `mcp1-sacc13` 基 `ace35ac3`，树 `D:/tops/idp-parcel-mcp1-sacc13`，迁移钉 `0021`。
- **形先定再动手（认领笔 `c90d8606`）**：**版本子表**而不是主键加版本——`0016` 的 `duty_payment_verification` 外键钉在 `external_funds_fact (tenant_id, fact_ref)` 上，「没有接收的资金事实就没有核对」是事实身份层面的话；主键加版本会拆掉它，子表让身份行不动、版本各占一行，外键原样成立（`0021` 头注写为何）。
- **之一 `e833355d`**（`/tdd`：先红在编排该拒 / 该比的两处，再绿）：领域 `FundsFactVersion`；端口 `ExternalFundsFactRegistration.Version` / `Corrects` + `ListFundsFactVersions` + `LoadFundsFact` 头注写实「最近接收的那一版」；迁移 `0021`（版本子表 + 存量非零 `RAISE` 交人 + 身份表退成身份列）；postgres 两次 DO NOTHING、两读口共用一只扫描器；编排幂等键（引用 + 版本）、`sameFundsFactVersion` 连回指一起比、版本空白 / 回指自己`未受理`、写口说已登记读口列不出 → 未决；SA 消费侧译出版本 / 回指，`ReceiveOnAdoptedFundsFactAdapter` 版本取信封所指、回指取事实本体；旧头注「付款人缺席答未受理」（12 起已假）一并改口。替身：application / SA 消费侧版本键 + 接收顺序、HTTP stub、CLI fake、dispatch 字面加 Version。真库例两版并存 / 身份 1 行版本 2 行 / 乱序各占一行 / CHECK 四格。
- **之二 `bab22328`**：dispatch 正例扩一格（v2 经信封落第二行回指 v1）；消费者头注改指实际落点（判据 4）。**实施中发现**：SA `external_funds_fact` 主键 `(tenant_id, fact_id)` 一事实一行、`ExternalFundsFacts.Save` DO NOTHING、`CorrectAmount` **无生产调用点**、`AdoptedFundsFactView` 头注自己写「将来一事实多行」——**SA 自己也存不下第二版**，02 裁决 2「更正再发一封」在 SA 侧没有落地路径。正例因此另用一条事实：v1 经 CC 写口预铺、SA 只存 v2，CC 一侧仍走生产依赖图；不在用例里绕 SA。
- **`8dbd49e2`**：完成记录（判据 1–4 逐项；判据 3「与 12 的交叉」：12 的 `payer_ref` 连内容列一起搬进子表，机制逐条保留、12 真库例改读子表仍 PASS；**后继票的形**按裁决 2 写清——先裁三轴与依据从哪来、触发在消费侧适配器之后另起一只编排、要读四口、核对按版本读、与 05 交接不变；判断项五条）。
- **验**：每笔 gofmt / build / vet 0；不带 DSN CC 七包 + cmd 三包 + architecture ok；带 DSN CC postgres + migrate + cmd 三包 ok，新真库例 `-v` PASS。
- **非作者评审缺席**（子代理 13:5x 第六次「Authentication error」）：按 12 同一裁法放行 + 补评审，票面「进 main 记录」留补评审条（`e833355d..8dbd49e2`，基线 `ace35ac3`）。
- **进 main**：分支基 `ace35ac3` = main tip、`ls-remote` 核未动 → 不必 cherry-pick，SHA 原样；清点重生成 → **`0bd86d42`**（CC 生产 92→93；迁移 169→170、CC 20→21）；gofmt / build / vet 0；14:0x 先排队列再占号，带 DSN 全仓 `-p 1 -count=1` **110 ok / 0 FAIL / 16 无测试 / 0 cached**（130 s）；释号。簿记一笔在其上（票 13「已进 main」+ 进 main 记录 + 补评审条；spec.md 13 行 + 19 / 20 两行；本节）→ ff → `push <sha>:main`。`mcp1-sacc13` → `merged/`、远端删、树拆。
- **立票两张**（推送方按 13 裁决 2 与判断项 ①）：**sa-cc/19** draft「资金事实新版本到达 → 形成新核对版本」（要裁三轴与依据从哪来：复用前版标 PENDING / 只登待重核对事项，归 CC owner）；**sa-cc/20** draft「SA 一事实一行存不下更正版本」（要裁形与入口，归 SA owner）。
- **前沿**：ready 三张（lc/35、lc/39、sa-cc/11）；draft 新增 19 / 20（各有要裁的）；后继：12 与 13 补评审（子代理 / 通道恢复即跑）、`admin-web` `procedureRef` 一行。在途零。
- **本节五数**（13:2x–14:0x）：CI 未起跑（计费）· 集成时延 sa-cc/13 13:5x（`8dbd49e2`）→14:0x **约 10 分**（1 票；作者 = 推送方）· 重放笔数 **0**（分支基 tip，ff 直进；四笔 + 清点 1）· 重复开发 0 · 会话重置 0 · **非作者评审缺席 1** · 子代理调用失败 1 次（累计 6）· 立票 2

### 14:4x–15:1x 新会话（通道 1；前两任：lc/35 作者崩于 13:4x、接手方 14:0x 写完成记录后亦止）：用户问 lc35 树能否 replay → 点名 → 通道 3 评审（今天首次非推送方评审到位）→ 进 main

- **lc/35 前两任留下的**（按 git 与票面完成记录复述，本会话未重写）：之一 `2ff3cbe5`（做法二 `Select` 结果对象 + 裁决取甲 建立重放先查交易；作者会话 13:4x 崩）、之二 `35257c3c`（「归本票」五条：`Save` 交回落成版本、翻译器构造期拒两口 nil、三处注释去数改句；前一任的未提交现场由 14:0x 接手会话逐块核 diff 后原样提交）、`b293a921`（票面 resolved + 完成记录 + spec 35 行）。分支 `mcp1-lc35` = origin，基 `7d664915` = main tip，树干净；隔离评审检出 `$env:TEMP\idp-review-lc35` @ `b293a921` 14:07 建好但没有报告出来。tasks.md 此前没有 lc/35 的节。
- **14:4x 用户问「`idp-parcel-mcp1-lc35` 现在上面状况了，可以 replay 了吗」**：量得重放为纯 ff、零重叠，但票面无任何「评审 ← 通道 N」条、`query_tasks` 无评审单——门未过，答「还不能」。`list_sessions` MCP-6 online（14:40 有活动），2 / 3 / 4 / 5 running 但 lastActivity 停 13:51 / 13:59 / 10:47 / 09-11，这一格照旧不可信 → **14:46 点名**（截止 14:50）：**4 个应答**（2 / 3 / 4 / 5 全报空闲、地盘无、余量充足；6 未应答）。派**通道 3**（task `819bdc5e`，high；lc/28 评审人）——评审单点了甲的事务边界、三格「决定已记」、并列 / 无人参选语义、`Save` 答复版本、拒 nil、注释不数别处、真库例是否真读决定册、地盘；20 分限时。2 / 4 / 5 一条收口广播。
- **14:57 评审到**（11 分）：**Standards 0 阻断 / 3 非阻断**（两处测试头注跨文件计数；「壳只管事务」一句两处复述；那一问读口故障无段标、评审自判可不改）· **Spec 0 阻断 / 2 非阻断**（① 裁决 1 末句「同一事务内」字面不成立且不可能成立——`Lookup` 走 `ReadExecutor` 自动提交、择优与建立本就两笔；并发两次 `Establish` 落在读与 `Insert` 之间各记一条 SELECTED，「一笔交易一条 SELECTED」只对顺序重放成立；② 评审未跑 DSN 与 architecture）。逐项 ✓ 判据 1–5、裁决四点、红线四条、地盘。评审原文落票 35 Comments；Spec ① 由推送方以 PS owner 口径收进 进 main 记录：**裁决 1 末句按「事务边界照旧」读、并发窗口是认下的残差**，归乙路候选后继；Standards ① ② → 候选尾巴票，随下一批 A 类合票。
- **进 main**：`fetch` 核 main = origin = `7d664915` 未动 → 不必 cherry-pick，SHA 原样；清点在 `b293a921` 重生成**零差**（19 件全是既有文件）→ 不出清点笔；gofmt / build / vet 0。**Docker Desktop 已掉**（55432 不通、daemon pipe 不在，容器 `idp-parcel-postgres-gate` Exited(255)）：15:0x 拉起 Docker Desktop；`compose up -d` 在本树撞 `container_name` 冲突（compose 项目名随目录名变，旧容器属共享树那个项目）→ 改 `docker start idp-parcel-postgres-gate`，healthy、端口映射 `127.0.0.1:55432->5432` 在。15:0x 先排队列再占号，带 DSN 全仓 `-p 1 -count=1` **110 ok / 0 FAIL / 16 无测试 / 0 cached**（122 s）；判据 2 真库子例与 `TestAStaleLabelTransactionSaveAnswersRevisionConflict` 单跑 `-v` PASS 非 SKIP；释号。簿记一笔在其上（票 35「已进 main」+ 评审 + 进 main 记录；spec.md 35 行；本节）→ ff → `push <sha>:main`。`mcp1-lc35` → `merged/`、远端删、两棵树拆（`idp-review-lc35`、`idp-parcel-mcp1-lc35`）。
- **前沿**：ready 两张（lc/39、sa-cc/11）；draft 19 / 20；后继：12 / 13 补评审（今天通道 2–5 都答了空闲，可派）、`admin-web` `procedureRef` 一行、lc/35 评审 Standards ① ② 尾巴。在途零。
- **本节五数**（14:4x–15:1x）：CI 未起跑（计费）· 集成时延 lc/35 完工报 14:0x（`b293a921`，前任写）→ 推出 15:1x **约 70 分**（1 票；其中 14:0x–14:4x 是会话空档，本会话 14:4x 起算约 30 分）· 重放笔数 **0**（分支基 tip，ff 直进；三笔）· 重复开发 0 · **会话重置 1**（本会话新起、无在途记忆）· 非作者评审缺席 0（通道 3 到位，今天首次）· 子代理调用 0 次

### 15:2x–15:5x 通道 1 会话 crash → 原通道 2 会话接管 → 重点名 → 派 12 / 13 补评审（通道 3 / 4）→ 12 补评审到 → 派 lc/39（通道 3）

- **接管现场**（15:2x 用户「mcp-1 crash，请你接管」）：本会话原是通道 2（14:38 起在通道 2 值守，答过 14:46 与 15:16 两轮点名），改听 `idp-mcp-1`，通道 2 从此无人值守。先量后动：通道 1 队列非阻塞排一次 → 空（15:16 点名的应答随崩掉的会话丢了）；`query_tasks` 无 pending / working → **崩前四件一件未派**，无孤儿派单；`branch-state.ps1` main = origin = `f04ad816`、在途分支 0、共享树 6 行全是未跟踪文件（`--ignore-cr-at-eol` 空）→ 四票地盘无半成品；`list_sessions` 3 / 4 / 5 running、6 online、7–11 offline。
- **15:25 重点名**（按 parallel-sessions「派任务是按名单做的决定，派之前重取一次」；只发 3 / 4 / 5 / 6，7–11 offline 不发免留过期点名）：截止 15:29 前应答 **2 个**（通道 3 / 4 空闲、地盘无、余量充足）；5 / 6 未应答；用户 15:2x 同时指明「目前只有 mcp-3 / 4 可派」。
- **派单**（15:30–15:31，两单都预建隔离检出以省评审人时间）：**sa-cc/12 补评审 → 通道 3**（task `fc95e0f8`，`$env:TEMP\idp-review-sacc12` @ `1ec18258`，基线 `bf1ae577`，24 件；通道 4 旧会话是「之一」作者不能评）；**sa-cc/13 补评审 → 通道 4**（task `e65b3d52`，`$env:TEMP\idp-review-sacc13` @ `8dbd49e2`，基线 `ace35ac3`，16 件；通道 4 旧会话只立过 13 票面未动代码，可评）。四件两通道，评审先派（欠的债、短、代码已在 main 早发现早立票），lc/39 与 sa-cc/11 各在评审交后接续派，一通道一次一单不叠派。
- **15:47 sa-cc/12 补评审到**（16 分）：**Standards 0 阻断 / 3 非阻断**（① CONTEXT 引文归属错位——「未提供或不适用必须明确记录，规则要求但缺失时保持未决」在 CC CONTEXT Rules 只在监管处置决定一条，税费付款核对词条没有这句，代码注释、`0020` 头注与裁决 2 改口都引错了词条；② 核对记录不带 Procedure、事后看不出付款人维按哪个程序判；③ 「同上一格」相对引用可不改）· **Spec 0 阻断 / 3 非阻断**（① 判断项 ① 评非阻断：错报 `procedureRef` 可绕「要求而缺失保持未决」，但登记方本就断言全部维度、信任模型未变，堵它需新读口非补丁；② 地盘一行未随裁决 3 更新；③ 评审未跑 DSN）。逐项 ✓ 判据 1–3、裁决 1–4、登记面、读规则位置。评审原文落票 12 Comments 进 main 记录之下；处置：Standards ① → 候选文档票；Standards ② + Spec ① → 候选后继票「加程序读口核一致 + 核对记录带程序」；两张等 13 补评审到齐一并立。
- **派 lc/39 → 通道 3**（15:5x，task `ea9c8da8`，normal；分支 `mcp3-lc39` 基 `f04ad816`，ADR 取 `0138`（现最高 0137），估 60–90 分，到 17:20 报位置）：/domain-modeling 三场景两候选，纯 docs；派单点了票面作者自认没读的那半（`FinalOutcomeStore` 采用记录形、VE / SA 消费路径、受控关闭的形）要先量。
- **前沿**：在途 lc/39（通道 3）、sa-cc/13 补评审（通道 4，到 15:50）；ready sa-cc/11（等通道 4 交评审后派）；draft 19 / 20；候选票两张（12 评审的 Standards ① / ②+Spec ①）待 13 评审到齐；`admin-web` `procedureRef` 一行；lc/35 评审 Standards ① ② 尾巴。
- **本节五数**（15:2x–15:5x）：CI 未起跑（计费）· 集成时延 —（本节无票进 main）· 重放笔数 0 · 重复开发 0 · **会话重置 1**（通道 1 crash，由通道 2 会话接管；通道 2 因此空置）· 非作者评审缺席 0（12 的补评审兑清 1 笔）· **点名名单丢失 1 次**（15:16 那轮随会话丢，重点名一次补回）

### 15:5x–16:1x 13 补评审到（通道 4）→ 派 sa-cc/11（通道 4）→ 立 21 / 22 → 簿记进 main

- **15:3x 通道 4 sa-cc/13 补评审到**（派后不到 10 分；报告 15:5x 到队列）：**Standards 0 阻断 / 3 非阻断**（1 两包测试替身同名同形，第三份出现再抽；2 `HandleAdoptedExternalFundsFact` 版本取信封、回指取回查内容，但回查交回的 `content.Version` 未与信封所指比对——加一句相等校验；3 `assemble_test.go` 正例头注「落一行」自 0021 起已旧）· **Spec 0 阻断 / 2 非阻断**（1 两件替身桩在票面地盘外未记账——`cmd/parcel-customs-register/duty_registers_test.go`、`adapters/http/register_credential_and_duty_test.go`，请推送方补一句；2 核对身份缺资金版本——v2 到达后同三轴同依据重核落`已存在`，给 19 作完成判据）。逐项 ✓ 判据 1–4、裁决 1–4、红线四条、判断项 ①–⑤（③「不校验前版已到」评为不该阻断：校验即链连续性 = 复制 SA 版本链，且会拒掉乱序到达）。评审原文落票 13 Comments；地盘两件由推送方在处置段补记；Standards 2 / 3 → 候选尾巴（与 lc/35 Standards ① ② 同批）；Spec 2 → 追写 19 Comments。
- **派 sa-cc/11 → 通道 4**（16:0x，task `e480bd94`，normal；分支 `mcp4-sacc11` 基 `01974923`；估 120–180 分，到 17:30 报位置）：**dispatch_task 建了记录但往通道 4 队列写通知时 `EPERM`（messages.json 被占用）**——`query_tasks pending` 核到记录只有一条，未重派，改 `send_to_session` 补通知带 task ID 让其 `query_tasks` 取全文。派单点了裁决 4 的量（合格来源引用能否指到包裹主体 / 分区 / 计费重量 / 业务时点）为第一件事、量完 ≤ 10 行回报；拆前置票号从 23 起；PP 迁移钉 `0010`；共享接线文件（`assemble.go`、CONTEXT-MAP）动前占号；真库占号前排队列。
- **立票两张（12 补评审后继，等 13 到齐一并立）**：**sa-cc/21** draft chore「CC CONTEXT 税费付款核对没有『未提供必须明确记录、规则要求而缺失保持未决』那句，付款人一格的注释 / `0020` 头注 / 12 裁决 2 引的都是监管处置决定词条：补句 + 改正引文归属」（`git grep` 核过那句在 CONTEXT 只命中监管处置决定一处；要裁原词，归 CC owner）；**sa-cc/22** draft enhancement「核对的监管程序由调用方断言、不核与案件一致、核对记录不带程序」（`VerifyDutyPaymentCommand.Procedure` 头注自认；`DutyVerificationKey` / `0016` 主键无程序；`CustomsCase.Procedure` 在但编排不读案件；要裁程序从哪来 + 核对身份要不要带程序，归 CC owner；与 19 同一张键两维，票面互指）。spec.md 子票表 +2 行。
- **簿记进 main**：票 12 / 13 Comments + 19 Comments 一句 + 21 / 22 + spec.md 两行 + 本节；纯 .md，`ls-remote` 核未动 → ff → push。（**17:5x 下一任更正**：这一笔写下时并没有推出——本节连同 13 / 19 Comments、21 / 22、spec 两行在共享树上未提交躺到 17:5x，main 停在 `01974923`；由 17:5x 接续的通道 1 会话原样提交推出，见下一节。）
- **前沿**：在途 lc/39（通道 3，到 17:20 报位置）、sa-cc/11（通道 4，到 17:30 报位置）；ready 0；draft 19 / 20 / 21 / 22（各有要裁的，归 CC / SA owner）；候选尾巴一批（lc/35 Standards ① ②、13 Standards 2 / 3、12 Standards ③ 评审自判可不改不计）；`admin-web` `procedureRef` 一行已并入 22 做法 4。今天欠的两笔补评审全部兑清。
- **本节五数**（15:5x–16:1x）：CI 未起跑（计费）· 集成时延 —（本节无代码票进 main）· 重放笔数 0 · 重复开发 0 · 会话重置 0 · 非作者评审缺席 0（13 的补评审兑清 1 笔）· **派单通知写入失败 1 次**（EPERM，记录在、通知丢，`send_to_session` 补发）· 立票 2

### 18:0x–18:2x 通道 1 接续会话（本节由 19:0x 会话按任务台账 `query_tasks all` 的 createdAt / updatedAt 与 git 作者时刻反推补记；该会话未写本节即结束）

- **接手**：上一任写完 15:5x–16:1x 节未提交即 crash，本任把共享树上的现场原样提交为 `09596d9a`（作者时刻 18:00:54）并在上节末加更正句，ff 推出；main = origin = `09596d9a`。
- **派单四件**（18:06–18:07）：**通道 4 ← task-28474d4c** sa-cc/11 续做（前任通道 4 16:2x 报完裁决 4 的量后 crash，树上只有认领笔 `24b73f66`；原单 task-e480bd94 结 failed「承接方换人」）；**通道 3 ← task-31d06b90** lc/39 续做（前任通道 3 15:52 提交 `0a25bb2a` 后 crash；原单 task-ea9c8da8 结 failed；补 README 行 / 0135 与 lc/25 追句 / spec 行 / 完成记录）；**通道 2 ← task-86d29daf** ADR-0138 非作者两轴评审（钉 `0a25bb2a`，只读不带 DSN）；**通道 5 ← task-80f5ff33** A 类尾巴合票 lc/40 + sa-cc/23（lc/35 评审 Standards ① ② + sa-cc/13 评审 Standards 2 / 3，分支 `mcp5-tails3`）。
- **收**：18:13 通道 2 评审到（Standards 0 阻断 / 6 非阻断 · Spec 0 阻断 / 2 非阻断；全文后由通道 3 照录票 39）；18:15 通道 3 lc/39 续做 done（tip `30130fda`）；18:17 **task-0d6941be → 通道 3** lc/39 评审收口（①–⑤ 改稿 + 照录，high）；18:19 通道 5 未回执即 crash（推送方按 git 量零产物）→ task-80f5ff33 结 failed → 18:21 **task-e79cc5a9 → 通道 2** 改派（分支 `mcp2-tails3` 基 `09596d9a`）；18:22 通道 2 回执「worktree 已建」。
- **该会话此后无记录**（台账最后一次写入 18:22；下一任 19:09 起）。**台账里的时刻标签偏早约 20 分**（task-0d6941be createdAt 18:17 而题头写「18:4x」、task-31d06b90 完工报 18:15 而票面写「18:3x」），与 09-13 那次「按估写偏」同病——本节与下节时刻全按 createdAt / git 作者时刻。
- **本节五数**（18:0x–18:2x，反推）：CI 未起跑（计费）· 集成时延 —（无票进 main，只有 `09596d9a` 簿记）· 重放笔数 0 · 重复开发 0 · **会话重置 2**（通道 1 上一任崩、通道 5 派单后即崩）· 非作者评审缺席 0（0138 评审到位）· 派单 5 张（含 1 张改派）

### 19:0x–19:3x 通道 1 新会话：用户问三棵树能否 replay → 量 → 三通道自查汇报 → sa-cc/11 笔 C 封存 + 续做改派 → lc/39 进 main

- **接手**（19:09 用户经队列：`idp-parcel-mcp2-tails3` / `-mcp3-lc39` / `-mcp4-sacc11` 可以 replay 了吗，不行就让各通道自查汇报、由推送方重派、全部解决）：无在途记忆，全按 git + `branch-state.ps1` + `query_tasks all` 量：main = origin = `09596d9a`；`mcp2-tails3@09596d9a` 领先 0 笔、树干净、未推（task-e79cc5a9 working 自 18:22 零产物）；`mcp3-lc39@ebea1678` = origin 领先 6 笔纯 .md、**树上 1 件未提交**（0138 一行，mtime 18:26:28）、task-0d6941be pending 未回执但它要的两笔 `2a1493f5` / `ebea1678` 已在；`mcp4-sacc11@28915f3b` = origin 领先 3 笔、**树上 7 件未提交**（笔 C，mtime 18:34–18:39:24）、task-28474d4c working 自 18:33 无回执。`list_sessions` 2–6 全 running 但 lastActivity 停在 18:07–18:32——这一格照旧答不出谁还在。**答用户**：三棵都还不能——2 无物可放、3 差一行未提交、4 在途未完且无评审。
- **19:1x 三通道点名式问询**（各一条 `send_to_session`，写明我量到的与要它答的、截止 19:25）+ 点名 5 / 6：19:2x 前 **5 个应答**——通道 2 / 3 / 4 **全是新会话无在途记忆**（三通道在 18:2x–18:4x 之间各崩一次），5 / 6 空闲。通道 2 按我给的路接续 task-e79cc5a9（`query_tasks` 读全文、时限顺延 20:15）；通道 4 未碰树上七件、请我封存；通道 3 把那一行成笔。
- **sa-cc/11 封存 + 改派**：推送方在 `D:/tops/idp-parcel-mcp4-sacc11` 逐路径 `add` + pathspec `commit` → **`0c368e45`** chore(salvage)（七件 954 行原样入库；提交信写 mtime / 自报 / 量得 gofmt 空、build vet 0、PP application + identity + domain 无 DSN ok——只作取证不作判据），推 origin；本地指针 `salvage/mcp4-sacc11-c` 留在它上；task-28474d4c 结 failed；**task-d47fcaaa → 通道 4**（high）：先写自己第一片 red 再读封存七件、≤ 15 分回接不接受，接受则 `reset --soft` 拆回随测试重切、`--force-with-lease` 推一次；剩余笔 C → D（消费侧读口 + inbox 消费者）→ E（dispatch 路由 / 真库装配例 / CONTEXT-MAP / 清点）→ 完成记录同笔；20:30 报位置、21:30 未交先 `save_progress`。撞点 `cmd/parcel-dispatch/assemble_test.go`（通道 2 一处头注）写进两单。
- **lc/39 进 main**：通道 3 19:19 成笔 **`4c6c4e86`**（一行括注四处调用方，树内 grep 复核）并 `report_task done`；推送方隔离检出 `%TEMP%\idp-replay-lc39-1915` @ `09596d9a`，`cherry-pick f04ad816..4c6c4e86` 七笔零冲突 → 代码 tip **`bb5a6297`**（对照见票 39 进 main 记录）；`git diff mcp3-lc39 bb5a6297 -- docs/adr .scratch/label-channel-service-first-release` 空；`go build ./...` 0、gofmt 空；清点在 `bb5a6297` 重生成零差 → 不出清点笔；纯 .md 不带 DSN 跑全仓（lc/35 先例）。**评审门**：通道 2 钉 `0a25bb2a` 两轴 0 阻断；`0a25bb2a` 后五笔推送方自审（`2a1493f5` 对照评审 ①–⑤ 逐条与 diff 17+/17−，Decision 八条取舍未改；`4c6c4e86` 四符号在 main 上 grep 均在；余三笔簿记）——写进票 39 进 main 记录。簿记一笔在其上（票 39 Status「已进 main」+ 进 main 记录；lc spec 39 行；本节与上节补记）→ `ls-remote` 核 `09596d9a` 未动 → ff → `push <sha>:main`。`mcp3-lc39` → `merged/`、远端删；作者树 `D:/tops/idp-parcel-mcp3-lc39` 与重放树比内容后拆。
- **前沿**：在途 tails3（通道 2，task-e79cc5a9，到 20:15）、sa-cc/11 续做（通道 4，task-d47fcaaa，20:30 报位置）；5 / 6 空闲留作两票的非作者评审；ready 0；draft 19 / 20 / 21 / 22；**归 PS owner**：ADR-0138 接受与否 + 实现票 + 判断项 1–8。**归用户**：会话崩溃今天再累计 4（通道 1 的 18:0x 会话、通道 2 / 3 / 4 各一次，均在 18:2x–19:0x）。
- **本节五数**（19:0x–19:3x）：CI 未起跑（计费）· 集成时延 lc/39 完工报 18:15（`30130fda`）→ 推出 19:3x **约 80 分**（1 票；其中 18:2x–19:09 是通道 1 会话空档，本会话起算约 30 分）· 重放笔数 **7** · 重复开发 0 · **会话重置 4**（通道 1 本会话新起 + 通道 2 / 3 / 4 新会话自报）· 非作者评审缺席 0（0138 评审到位；后五笔自审如实标注）· 封存 1 笔 · 派单 1 张

### 19:3x–19:5x 通道 2 交 tails3（lc/40 + sa-cc/23）→ 派通道 6 评审 → 全量一次 → 三笔重放进 main

- **完工报**（19:3x，task-e79cc5a9 done）：`mcp2-tails3@c03088b8` = origin，树干净，基 `09596d9a`，三笔（立票 `8ab6c61c` / lc/40 两条 `42f524f4` / sa-cc/23 三条 + 两票完成记录 `c03088b8`），11 文件 +155/−9；作者带 DSN cmd 两包 PASS 129 / SKIP 0，占 / 释 55432 与 `assemble.go` 各一条广播；实测与派单不符两点（读口实名 `LoadAdoptedFundsFact`；`assemble.go` 头注一句越地盘字面）均属实、接受。通道 4 `mcp4-sacc11` 截至 `c03088b8` 未碰 `cmd/parcel-dispatch/`，无撞。
- **先重放再评审（并行省时）**：隔离检出 `%TEMP%\idp-replay-tails3` @ `07eaafc5`，`cherry-pick 09596d9a..c03088b8`——**lc spec.md 撞两次**（main 侧 39 行已改「已进 main」，分支侧在相邻处加 40 行、再改 40 行状态），按意图解：保 main 的 39 行、取分支的 40 行，正则去标记后 `cherry-pick --continue`；结果 `git diff mcp2-tails3 f1be6687 -- cmd internal .scratch/sa-cc-* issues/40-*` 空、lc spec 只差 39 行那一处。SHA 对照 `8ab6c61c→1b3a430c` / `42f524f4→a86a678f` / `c03088b8→f1be6687`。**评审 → 通道 6**（task-f75ff579，high，预建检出 `%TEMP%\idp-review-tails3` @ `c03088b8`，基线 `09596d9a`，20 分）——通道 5 曾被派这张票（零产物即崩）仍避，留给 sa-cc/11。
- **全量一次**（推送方）：19:4x 先 `check_messages(waitMs 0)` 排队列（空）再广播占 55432，`f1be6687` 带 DSN `go test -p 1 -count=1 ./...` **110 ok / 0 FAIL / 16 无测试 / 0 cached**（131 s）；探针 `TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable` / `TestASourceAnsweringAnotherVersionIsRefusedBeforeRegistering` `-v` PASS 非 SKIP；释号广播。清点在 `f1be6687` 重生成零差 → 不出清点笔。
- **评审到**（19:4x，派后约 10 分）：**Standards 0 阻断 / 1 非阻断**（`ErrChannelBasisTranslationStopped` 头注跨包计数，既有非本票引入，另笔零行为）· **Spec 0 阻断 / 1 非阻断**（lc/40 判据 2 grep 口径限 `.go`）；ADR-0029「不同必须分」判新哨兵不进未决名单对、ADR-0025 判校验放消费处理方对；作者九条判断项全接受（lc/40 ② 即 Standards ①）。全文落两票 Comments；两条非阻断处置写进 lc/40 进 main 记录。
- **进 main**：簿记一笔在 `f1be6687` 之上（两票 Status「已进 main」+ 评审 + 进 main 记录；lc spec 40 行、sa-cc spec 23 行；本节）→ `ls-remote` 核 `07eaafc5` 未动 → ff → `push <sha>:main`。`mcp2-tails3` → `merged/`、远端删；作者树与重放树比内容后拆。顺手拆了两棵 15:3x 遗留的评审检出 `%TEMP%\idp-review-sacc12` / `-sacc13`（均干净、评审已落票）。
- **前沿**：在途 sa-cc/11 续做（通道 4，task-d47fcaaa，19:4x 接手判「接受其形（小改），五格都在」，20:30 报位置）；通道 2 / 3 / 5 / 6 空闲——sa-cc/11 交后评审派 5（从未碰 PP / sa-cc/11）；ready 0；draft 19 / 20 / 21 / 22。**候选尾巴（归 PS owner，零行为）**：`establish_selected_label_transaction.go` `ErrChannelBasisTranslationStopped` 头注「三个实例半边源」→ 点名。
- **本节五数**（19:3x–19:5x）：CI 未起跑（计费）· 集成时延 tails3 完工报 19:3x（`c03088b8`）→ 推出 19:5x **约 20 分**（2 票，同分支；含评审 10 分与全量 131 s 并行）· 重放笔数 **3** · 重复开发 0 · 会话重置 0 · 非作者评审缺席 0（通道 6）· 冲突解 2 处（同文件相邻行，簿记）

### 19:5x–20:0x 原通道 1 最后一段（本段由 20:1x 接手会话按 `query_tasks` 记录、`%TEMP%` 两棵树的 HEAD 与 git 反推；原会话未写下即 crash）

- 通道 4 20:0x `report_task done` task-d47fcaaa：`mcp4-sacc11@3062d485` = origin，六笔（认领 + 笔 A/B/C/D/E），基 `01974923`，作者带 DSN cmd/parcel-dispatch + PP 全部 PASS 513 / 0 / 0，占 / 释 55432 与 assemble*.go / CONTEXT-MAP 占 / 释号各一条广播。
- 原会话建了两棵检出：`%TEMP%\idp-review-sacc11` @ `3062d485`（评审用，预备派通道 5——19:5x 节写的「sa-cc/11 交后评审派 5」）与 `%TEMP%\idp-replay-sacc11` @ `6876e10f`（六笔 `cherry-pick` 到 `9d720a2f` 之上已完成、零冲突、树干净）。评审单未派出、全量未跑、簿记未写，会话断在这里。此后通道 2 / 3 / 5 / 6 相继 crash，用户 20:1x 报「只剩 1 / 4」。

## 2026-09-14 20:1x 通道 1 新会话（原为通道 3 的会话，用户经队列「mcp-1 crash，请监听 idp-mcp-1」→「`D:\tops\idp-parcel-mcp4-sacc11` 现在可以回放了吗，现在只有 mcp-1 / 4 通道了」；接手时 `main = origin/main = 9d720a2f`）

### 20:1x–20:4x：sa-cc/11 量 → 推送方自评（子代理鉴权错）→ 全量一次 → 六笔进 main

- **量**（全按 git，不读自报）：`branch-state.ps1 -Path` 对 `D:/tops/idp-parcel-mcp4-sacc11` 报路径在仓外（脚本的 `-Path` 收的是仓内相对地盘，不是另一棵树的绝对路径——量法误用，不是脚本坏；改用 `git -C` 直接量）。`mcp4-sacc11@3062d485` = origin、树干净、领先 main 6 笔；`git range-diff 01974923..3062d485 9d720a2f..6876e10f` 六笔全 `=`——前任的重放原样可用，不必重来；main `01974923..9d720a2f` 与本分支重叠文件只 `cmd/parcel-dispatch/assemble.go` / `assemble_test.go`（通道 2 两处注释，不同块）。共享树 76 行未提交经 `--ignore-cr-at-eol` 量得全是 CRLF 幻影、无真改动；未跟踪六件归用户。**答用户**：重放本身已在，缺的只是评审门。
- **评审门**：票 11 Comments 只有作者自跑两轴，无非作者评审；除 1 / 4 外无空闲通道，按「没有空闲通道时推送方自己用 /code-review 的隔离子代理跑」起两个只读子代理（Standards / Spec，钉 `3062d485` 基线 `01974923`）——**两个都在起跑前 `Authentication error`**（09-07 20:3x 同一故障，当时也是推送方转自评），本会话按同一 brief 逐文件读 diff 自评（推送方 ≠ 作者，满足「非作者」）。结论 **Standards 0 阻断 / 2 非阻断**（`assemble.go` 哨兵头注「都停在前三格之一」与装配例相反：有卡停第四格；S/R/P 封闭集 switch 在 adapter 与 application 各一份）· **Spec 0 阻断 / 2 非阻断**（裁决 7 字面被 `completeCatalogueReadings` / `withSeriesReadings` 改走 `WithInput` 打破——为裁决 2 所必需、接受，但顺带让「卡绑序列 + 目录、序列步留说明、目录读数挂上」这一组合新评价的解释多出序列说明、语义摘要随之变，范围内无用例钉，归 PP owner；形成前三格 `PRICE_CARD_NOT_CONFIGURED` / `…APPLICABILITY_CONFLICT` / `PRICING_INPUT_UNAVAILABLE` 与 CONTEXT「取不到价卡、事实缺失属待判断」用词不同——裁决 3 / 4 明写、CONTEXT 那句分的是已形成评价的格，接受，是否补 CONTEXT 一句归 PP owner）。全文落票 11 Comments。
- **全量一次**（与自评并行）：`6876e10f` 上 `gofmt -l` 空、`go build` / `go vet` 0；20:1x `check_messages(waitMs 0)` 空 → 广播占 55432 → 带 DSN `go test -p 1 -count=1 ./...` **113 ok / 0 FAIL / 16 无测试 / 0 cached**（128 s；比 `f1be6687` 多三包 = PP 三个新适配器包）；探针 `TestASubmittedEvaluationRequestStopsHonestlyAtThePricingInputSeamThroughTheRouteTable` 与 `TestASubmittedRequestEnvelopeIsProcessedExactlyOnce` `-v` PASS 非 SKIP；释号广播。清点在 `6876e10f` 重生成零差 → 不出清点笔。
- **进 main**：簿记一笔在 `6876e10f` 之上（票 11 Status「已进 main」+ 评审 + 进 main 记录；sa-cc spec 11 行；本节与上节补记）→ `ls-remote` 核 `9d720a2f` 未动 → ff → `push <sha>:main`。`mcp4-sacc11` → `merged/`、远端删；作者树 `D:/tops/idp-parcel-mcp4-sacc11` 与重放树比内容后拆；`idp-review-sacc11` 拆。
- **前沿**：ready 0；draft 19 / 20 / 21 / 22；在途分支 0。**候选尾巴（A 类零行为）**：`assemble.go` 哨兵头注一句 + S/R/P switch 归一（PP）；`establish_selected_label_transaction.go` `ErrChannelBasisTranslationStopped` 头注计数换点名（PS，19:5x 节已记）。**归 PP owner**：Spec 两条非阻断；三只输入读口的后继票归 TF / NO·PS / PS。**归用户**：只剩两个通道时评审门只能推送方自评——子代理鉴权错今天第二次（09-07、09-14），是否修子代理或接受「推送方自评」为常态；会话崩溃今天累计 8（18:2x–20:1x 间 1 / 2 / 3 / 4 各一次 + 20:0x–20:1x 间 1 / 2 / 5 / 6 各一次）。
- **本节五数**（20:1x–20:4x）：CI 未起跑（计费）· 集成时延 sa-cc/11 完工报 20:0x（`3062d485`）→ 推出 20:4x **约 40 分**（1 票；其中 20:0x–20:1x 是通道 1 会话空档，本会话起算约 30 分，含自评约 15 分与全量 128 s 并行）· 重放笔数 **6**（前任 cherry-pick、本会话核对）· 重复开发 0 · **会话重置 5**（通道 1 本会话由通道 3 转来 + 用户报 1 / 2 / 5 / 6 crash）· 非作者评审缺席 0（推送方自评，子代理鉴权错如实标注）

### 20:4x–21:0x 同一会话：用户「下一步呢」→「派吧」→ 通道 4 也 crash → 推送方自做 A 类尾巴（sa-cc/24 + lc/41）直接快进 main

- **前沿**（20:4x，全量 `Status:` 扫描 + `branch-state.ps1`）：ready 0 · in-progress 0 · 在途分支 0 · draft 6（sa-cc 19 / 21 / 22 归 CC owner、20 归 SA owner；lc 20 / 22 等第一家真源）。给用户三条路：① A 类零行为尾巴合票派通道 4；② 推送方立三张输入读口后继票；③ 用户授权代裁 sa-cc 19–22。用户「派吧」（①+②）。
- **点名**（20:33 广播、截止 20:38）：零应答；用户 20:4x「现在只有你了，现在都 crash」——通道 4 亦崩。① 改为推送方自做（09-08 awf/08「全部你自己干」同一形态）：隔离树 `%TEMP%\idp-parcel-mcp1-tails4`，分支 `mcp1-tails4` 基 `eca6dba1`，自立 sa-cc/24（`assemble.go` 哨兵头注「前三格」一句改点名三格；`EvidenceKind.Declared()` 导出，adapters/settlementaccounting `declaredEvidence` + application `accepted` + **开工重量又带出一处既有的** adapters/http 回放译码——三份 S/R/P switch 归领域一处）+ lc/41（PS `ErrChannelBasisTranslationStopped` 头注「三个实例半边源」→ 点名 `ChannelSelectionBasisTranslatorDeps` 的 `Accounts` / `Agreements` / `Resolutions`）。一笔 **`5eb3caee`**（九文件 +17/−23，生产语句只有等价重构）推 origin。验：gofmt 空、build / vet 0、六包无 DSN ok；占 55432 → `5eb3caee` 带 DSN 全仓 **113 ok / 0 FAIL / 16 无测试**（129 s）→ 释号。清点不重生成（不增删文件）。
- **进 main**：main 自 `eca6dba1` 未动 → 簿记一笔在 `5eb3caee` 之上（两票 Status「已进 main」+ 进 main 记录；两 spec 行；本节）→ `ls-remote` 核 → **直接快进、不重放、SHA 不换** → `push <sha>:main`。**评审门**：单通道无非作者可派、子代理鉴权错，零行为注释 + 等价重构按「推送方自审」处置，两票票面如实写。`mcp1-tails4` → `merged/`、远端删、树拆。
- **② 进行中**：三张输入读口后继票立在新目录 `.scratch/pp-pricing-input-seams/`（TF 发生项成员对象只读视图 / NO 实测 + PS 申报的实重尺寸读口 / PS 起讫邮编路线读口），纯票面 draft，各有「要裁的」归 TF / NO·PS / PS owner——见下节。
- **本节五数**（20:4x–21:0x）：CI 未起跑（计费）· 集成时延 自做无完工报，立票 20:4x → 推出 21:0x **约 20 分**（2 票同笔）· 重放笔数 **0**（快进）· 重复开发 0 · **会话重置 1**（用户报通道 4 crash）· 非作者评审缺席 **1**（单通道自审，如实计）

## 2026-09-14 20:5x 通道 1 新会话（用户经队列指 `%TEMP%\idp-parcel-mcp1-tails4\.scratch\tasks.md`「现在各通道已经恢复了」；接手时 `main = origin/main = db480695`）

### 20:5x–21:0x：量 → tails4 收尾 → 点名（今天首次五通道齐答）→ 派 24 / 41 补评审（通道 2）+ PP 三张输入读口票（通道 3）

- **量**（无在途记忆，全按 git，不读自报）：`fetch` 后 main = origin = `db480695`——上一任 21:0x 的簿记笔已推出（用户截图里 `5eb3caee..db480695 mcp1-tails4 -> mcp1-tails4` 那一推是分支，main 亦已 ff）；`mcp1-tails4@db480695` 领先 0、树干净；`branch-state.ps1` 在途分支只此一条；共享树 76 行未提交经 `git diff --ignore-cr-at-eol --stat` 量得全是 CRLF 幻影、无真改动，未跟踪六件归用户；`query_tasks` pending / working 皆空。**上一任断在**「`mcp1-tails4` → `merged/`、远端删、树拆」与「② 立三张票」之前——`.scratch/pp-pricing-input-seams/` 两棵树上都不存在，上节末句「见下节」所指的那一节从未写下。
- **tails4 收尾**：先比内容（七份代码文件 `git diff 5eb3caee origin/main` 空；四份 .md 只差 `db480695` 追的「进 main 记录」，属预期）、`status --short` 与 `--untracked-files=all` 零行 → `git branch -m mcp1-tails4 merged/mcp1-tails4`、`push origin --delete mcp1-tails4`、`worktree remove`（不加 `--force`）；核三样：树里 `.git` 链接无、`worktree list` 无、目录无，指针 `merged/mcp1-tails4` = `db480695` 在。
- **点名**（20:53 广播 2–6，截止 20:57；7–11 `list_sessions` offline 不发免留过期点名）：**5 个应答**（2 / 3 / 4 / 5 / 6 全报空闲、地盘无、余量充足，全部在截止前到）——今天自 08:4x 起第一次五个通道同轮齐答。`list_sessions` 那一格这次与应答一致，但仍按点名不按它。
- **派单两件**（20:57–20:58）：**通道 2 ← task-293ce358**（high）sa-cc/24 + lc/41 补评审——钉 `5eb3caee` 基线 `eca6dba1`，推送方预建检出 `%TEMP%\idp-review-tails4`，只读 20 分、21:20 前交；作者是通道 1，任一通道都非作者；派单点了五处要专看（`EvidenceKind.Declared()` 导出会不会外泄封闭集语义、三处 switch 是否逐格等价、`assemble.go` 新头注与真库装配例那一格是否一致、lc/41 点字段名不点接口名、有无越票面）。**通道 3 ← task-620bc8e7**（normal）立 PP 三张输入读口后继票——新目录 `.scratch/pp-pricing-input-seams/`：spec + 01 TF 发生项成员对象只读视图 / 02 NO 实测 + PS 申报的实重尺寸读口 / 03 PS 起讫邮编路线读口，纯 .md draft、各带「要裁的」归 TF / NO·PS / PS owner、不裁；出处 sa-cc/11 裁决 4 + 判据 4 的量（`01974923`）；分支 `mcp3-pp-seams` 基 `db480695`，隔离树；估 45–60 分，21:45 报位置。4 / 5 / 6 一条收口广播、留作后手。
- **簿记** `bccb60a1`（本节到此为止那半）→ ff → push。
- **用户 21:0x「现在 mcp-4/5/6 空闲」→ 再派三件裁决前取证**（21:0x–21:1x；同一轮点名名单，用户又确认过；`branch-state.ps1` 在途分支零、CC / SA 地盘无半成品）：sa-cc 19–22 四张 draft 的「要裁的」全归 owner、用户未授权代裁，但裁决要的**事实**可以先量——按 parallel-sessions「只读会话要派具体任务」派成三件只读取证，产物是各票 Comments 一条「取证 ← 通道 N · 钉 `bccb60a1`」（命令 → 结果、两路各自的实测后果只列不选、能力边界），不裁、不提方案、不动代码。**通道 4 ← task-2cc063bd** sa-cc/19 + 22（同一张核对键两维：有效性轴有无 PENDING、范围→案件是否一对一、`duty_payment_verification` 改键会拆到谁、`corrects` 缺席能否在既往核对之后到达；分支 `mcp4-sacc19-22-evidence`，21:45 报位置）；**通道 5 ← task-a41fb596** sa-cc/20（哪些 SA 表外键到 `external_funds_fact`、首版从哪个入口进、ADR-0137 决定四去掉的是哪一口、02 的 handoff 参数带不带版本；分支 `mcp5-sacc20-evidence`，21:40 报位置）；**通道 6 ← task-edcfe85c** sa-cc/21（CONTEXT 两词条原文、引错处以 grep 为准逐符号列全、三停格代码原词、两候选各自要换哪些引文；分支 `mcp6-sacc21-evidence`，21:35 报位置）。三通道写同目录不同文件，撞点写进三单。
- **21:0x 24 / 41 补评审到**（通道 2，派后约 15 分）：**Standards 0 阻断 / 3 非阻断**（① `Declared()` 头注「封闭集多一格时只有这里要改」过宽——三句错误文本仍字面枚举 S / R / P，收窄句子零行为、改错误文本属行为归另票；② `assemble.go` 新句三分漏 `ErrEvaluationRequestNotVisible`，可不改、改则加限定「到得了价卡解析的」；③ 判断项 ①「本仓 `valid()` 一律未导出」全仓不成立——另有七处导出 `Valid`，取名 `Declared` 仍接受、依据改语义半边）· **Spec 0 阻断 / 1 非阻断**（末句第三枚以「这一格」指代，判满足、记一笔）；无发现项逐条 ✓（三处 switch 逐格等价含零值路径、错误文本逐字同、lc/41 字段名点名判可、地盘零 diff、无越票面）。评审全文落 24 Comments，41 摘本票那半。**处置**：Standards ① 前半 + ② + Spec ① → 一张 A 类尾巴票派通道 2 自立自做（零行为两处注释）；Standards ① 后半（错误文本从领域取）→ 归 PP owner；Standards ③ → 判断项 ① 半句在处置条里收窄，原文不改写。评审门补齐：上节「非作者评审缺席 1」就此兑清。检出 `%TEMP%\idp-review-tails4` 干净、拆。
- **前沿**：在途——task-620bc8e7（通道 3，PP 三票，21:45 报位置）、task-2cc063bd（通道 4）、task-a41fb596（通道 5）、task-edcfe85c（通道 6）、通道 2 接 A 类尾巴票（见下条）；ready 0；draft sa-cc 19 / 20 / 21 / 22（取证到齐后等裁）；lc 20 / 22 等第一家真源。**归用户**：sa-cc 19–22 是否授权代裁（取证到齐后授权即可当场写裁决转 ready 再派实现）；pp-seams 01–03 三张的「要裁的」归 TF / NO·PS / PS owner（同一问题：授权代裁否）；**归 PP owner 三条**：sa-cc/11 Spec ① ②（`WithInput` 顺带保住序列说明的解释语义变化要不要补一条单元例、形成前停格在 PP CONTEXT 要不要一句话）、sa-cc/24 评审 Standards ① 后半（三句错误文本字面枚举 S / R / P 要不要改从领域取）、pp-seams spec「不在本目录」第二条（`EvaluationSubjectKind` 要不要加集运单元一种，ADR-0111 四种里没有）。
- **派通道 2 ← task-4edf2e4d**（21:1x）：自立自做 sa-cc/25（A 类零行为尾巴：`Declared()` 头注收窄 + `assemble.go` 哨兵末句加限定「到得了价卡解析的」并写第三枚符号名；三句错误文本一字不动，那半归 PP owner）；分支 `mcp2-tails5` 基 `0bea3610`；`assemble.go` 共享接线文件动前占号；评审门推送方自审（纯注释）；21:50 报位置。24 / 41 评审簿记 `0bea3610` 已推。
- **21:17 通道 3 交 PP 三票**（task-620bc8e7 done，派后约 20 分）：`mcp3-pp-seams@0a9808ec` = origin，一笔基 `db480695`，四文件 +267 纯 .md（`.scratch/pp-pricing-input-seams/` spec + 01 TF 发生项成员对象只读视图 / 02 NO·PS 实重尺寸读口 / 03 PS 起讫邮编路线读口，全 draft，「要裁的」3 / 4 / 4 条各归 TF / NO·PS / PS owner，02-2 与 03-3 与 PP owner 共）。**实测与派单不符五条，全部接受**：① NO 那半连实际测量的领域对象、登记册与迁移都没有（NO 自己的 HTTP 查阅用例头注已如实记「没有表就没有读法」），缺的不是一只口是口后面的登记册；② PS 非测试代码与迁移无任何结构化邮编字段，`DeliveryPlaceReferenceView` 只交引用、CONTEXT 说的「持引用方按引用向本上下文取」那只口不存在——它的头注早预告了「第二个消费方……届时另立」，03 就是那个届时；③ NR CONTEXT 无「分区」一词，分区解析归 PP（PP CONTEXT「计价参考目录」+ ADR-0109 决定四），03 不列为要裁的、只记防重开——派单让查 NR 是我写偏，票面纠正得对；④ 用词：「计费重量」在 GLOSSARY 归 SA、PP 词是「计价重量」、提供方给原始量——三票统一写「实重 / 尺寸」，引既有权威不是裁决；⑤ 顺带量到 PP `EvaluationSubjectKind` 四种（ADR-0111）无集运单元，发生项成员是集运单元时评价对象无处落——记在 spec「不在本目录」，**归 PP owner**（本节末「归用户」加一条）。推送方读四文件：取证 18 条逐命令钉 `db480695`；红线自查项（未裁、无行号、计数只在数即论点处且钉 SHA、未复制 11 裁决正文）逐条属实；「不在本目录」把 PP 消费侧适配器（实现 `PricingInputResolver`）与 CONTEXT-MAP 三条消费边写清、Blocked by 三票全部进 main。
- **进 main**：隔离检出 `%TEMP%\idp-replay-ppseams` @ `0bea3610`，`cherry-pick 0a9808ec` 零冲突（main `db480695..0bea3610` 三笔只动 tasks.md / 24 / 41，与新目录零重叠）→ **`f94c8cd1`**，`git diff 0a9808ec f94c8cd1 -- .scratch/pp-pricing-input-seams` 空；清点在 `f94c8cd1` 重生成零差（`tools/mechanism-inventory` 不数 `.scratch/`）；纯 `.scratch` .md 不跑 go。共享 main `merge --ff-only f94c8cd1` → 簿记一笔在其上（本条）→ `ls-remote` 核 `0bea3610` 未动 → `push <sha>:main`。**评审门**：纯 .md 票面，推送方自审（上条读四文件即是）。`mcp3-pp-seams` → `merged/`、远端删；作者树与重放树比内容后拆。
- **21:2x 四件同时到**：通道 2 sa-cc/25 完工（`mcp2-tails5@b0463e1a` 基 `0bea3610`，四文件 +54 −3，两份 .go 每一改行 `//`；判据 1–4 ✓；三条措辞判断项——「那些」让主语落在信封、冒号改分号、末尾写「正例断的正是」——全接受）；通道 5 sa-cc/20 取证（`5fab18e0`，+33：**SA 没有任何表外键到 `external_funds_fact`**——`funds_mapping.fact_id` / `settlement_application.fact_id` / `duty_payment_verification_adoption.funds_ref` 全是裸 text，CC 0021「子表保住外键」那条理由在 SA 无前提；**首版今天没有生产入口**——`NewMapExternalFundsHandler` 在 cmd/ 零命中、SA inbox 只听 PP / CC 两类信封、SA http 全是 query、只有测试直写；ADR-0137 决定四去掉的是 CC 侧运维补录子命令、保留 SA `map_external_funds.go` 采用；handoff 版本取自 `Record.Fact` 无独立参数）；通道 4 sa-cc/19 + 22 取证（`cbdcb360`，+73：**有效性轴今天有 PENDING**（`FundsFactPending`，0016 CHECK 同词）但 `DutyPaymentGateReading.valid()` 与 0019 规则表都拒 PENDING——(a) 路形成的 PENDING 版成为「当前」后放行门禁答未决；`DutyVerificationStore` 只有 Find / Save 两法；**范围→案件这条边在库上不存在**——`customs_case` 无 `scope_ref`，主键 (租户、辖区、方向、程序、义务范围)，`procedure_ref` 在案件主键里；`procedureRef` 必填只在 registrationjson，**HTTP 端点挂 `UnconfiguredIntake{}` 不读报文体**；核对五维在 CC 0016 / 0019 gate_verification / SA 0020 / 信封载荷 / 信封 ID 五处各存一份字串、无外键——**主键加维而指纹不变会让 SA 主键与信封五维都不再唯一**，「不改 SA」在那条改法上成立不了，折进 `verificationDigest` 则五处形状不变；不带回指的新版本可以在既往核对之后落地，「有回指」与「已有既往核对」互不蕴含）；通道 6 sa-cc/21 取证（`a42bdbfb`，+38：引那句的地方**付款人维 13 处**（票面列 6）+ 非付款人维 8 处兑平 24 行命中（钉 `bccb60a1`）；13 处里 3 处自称引「CONTEXT 税费付款核对 + Rules 一句」、10 处只写「CONTEXT」；**税费付款核对词条今天没有任何一句讲「未提供怎么记」**；顺带核出票 12「语言从哪里来」把一句记为 CC CONTEXT 而出处其实是 UC-CC-009「启动条件」；候选 A 同形复用 → 13 处零改引文，候选 B 付款人专句 → 7 处改引文 + 4 处改归属 + 2 处补词条名）。四份取证推送方逐条读过：命令→结果、只列不选、能力边界都在。
- **进 main（五笔一批）**：隔离检出 `%TEMP%\idp-replay-tails5` @ `84e37713`，`cherry-pick b0463e1a cbdcb360 5fab18e0 a42bdbfb` 零冲突 → `58ef74bd` / `f248a0b9` / `af75066e` / **`09833d67`**，四处 `git diff` 内容零差；`gofmt -l` 空、`go build` / `go vet` 0；21:3x `check_messages(waitMs 0)` 排队列（收到四件完工报，无占号）→ 广播占 55432 → `09833d67` 带 DSN 全仓 `-p 1 -count=1` **112 ok / 1 FAIL / 16 无测试 / 0 cached**（129 s）→ 释号广播。**那 1 FAIL 是 main 既有 flaky、与本批零关系**：CC postgres `TestFundsFactVersionsAccrueAsRowsThatPointBack`「v1 走样」——该包与 CC 迁移在 `84e37713..09833d67` 零 diff；成因：用例以 map 迭代登记 v1 / v2（小 map 迭代起点随机）、每版各自一笔事务故 `received_at` 不同，`ListFundsFactVersions ORDER BY received_at ASC` 在 v2 先登时如实把 v2 排前、用例却断言 `versions[0]` 是 v1；同 tip 单跑 `-count=40` **36 PASS / 4 FAIL**（≈ 1/8，与 Go 小 map 反序概率同量级）。自 sa-cc/13 `e833355d` 起就在 main 上，今天此前五次全仓绿是运气（P ≈ 0.5）。**处置**：不重跑凑绿；点名 + 立票 sa-cc/26 派通道 3 修（用例登记顺序显式化，两种到达顺序各钉一格）。共享 main `merge --ff-only 09833d67` → 簿记一笔在其上（票 25「已进 main」+ 进 main 记录；spec 25 行；本条）→ `ls-remote` 核 `84e37713` 未动 → `push <sha>:main`。四条分支 → `merged/`、远端删；四棵作者树与重放树比内容后拆。簿记 `5bf6eb84` 已推。
- **派通道 3 ← task-e73efbe2**（21:4x，high）：自立自做 sa-cc/26（bug）——把 `TestFundsFactVersionsAccrueAsRowsThatPointBack` 的 map 迭代登记改显式顺序，另加「先 v2 后 v1」确定性一格（`ListFundsFactVersions` 如实列 [v2, v1]、`LoadFundsFact` 交回最近接收的 v1）；生产零 diff；判据 `-count=40` 40/40；分支 `mcp3-sacc26` 基 `5bf6eb84`；22:10 报位置；完工后派非作者评审再重放。
- **前沿**（21:4x）：在途 sa-cc/26（通道 3）；2 / 4 / 5 / 6 空闲；ready 0；draft sa-cc 19 / 20 / 21 / 22（**取证全部到齐在票面**，等裁）、pp-seams 01 / 02 / 03（等裁）；lc 20 / 22 等第一家真源。远端遗留三条非本会话分支 `mcp3-awf12` / `mcp5-awf13` / `salvage/mcp4-tf03`（本地对应 `salvage/` 指针；按纪律 salvage 不推远端——归下次清理，不动）。
- **21:41 通道 3 交 sa-cc/26**（派后约 5 分完工报；`mcp3-sacc26@5d9907aa` 基 `5bf6eb84`，一笔三文件 +103 −10，测试 +34 −10、生产零 diff）：red 基线 `-count=40` 36 / 4 → green 两格各 40 / 40。**判断项 1 接受、是我派单写偏**：反序格（`SYN-FUNDS-02` 先 v2 后 v1）在用例尾部**本来就是**切片确定登记的，随机的只是正序格的 map；作者把反序段抽成独立用例 `TestFundsFactVersionsArrivingOutOfOrderListByReceiptAndLoadTheLatestReceived` 并补上 `LoadFundsFact` 交回最近接收 v1 的断言（那条一次即绿、如实写无自己的 red）。**评审 → 通道 4**（task-fea10163，预建检出 `%TEMP%\idp-review-sacc26`，15 分）；与评审并行重放 `%TEMP%\idp-replay-sacc26` @ `3373f7cf` → **`2b2685c3`** 零冲突、内容零差，占 55432 → 带 DSN 全仓 **113 ok / 0 FAIL / 16 无测试 / 0 cached**（130 s），探针两格 PASS 非 SKIP → 释号。**21:5x 评审到**（约 10 分）：Standards 0 阻断 / 1 非阻断（反序格头注只引了「最近接收」口径的前半、没点出它是 19 做法 3 要退役的——转记进 19 Comments，19 作者落地时一并改口）· Spec 0 阻断 / 1 非阻断（反序格三条顺序断言依赖两笔事务 `now()` 严格递增——微秒时钟依赖非随机源、基线既有非回归、红线「不 Sleep」已堵便宜解，接受为残差）；无发现项逐条 ✓（正序断言与基线逐行同、反序段新断言严格超集、生产 / 迁移 / cmd / apps 零 diff）。全文落票 26 Comments。**进 main**：`merge --ff-only 2b2685c3` → 簿记一笔在其上（票 26「已进 main」+ 评审 + 进 main 记录；spec 26 行；19 Comments 一句；本条）→ `ls-remote` 核 `3373f7cf` 未动 → `push <sha>:main`。`mcp3-sacc26` → `merged/`、远端删；作者树 / 评审检出 / 重放树拆。
- **前沿**（22:0x）：在途 0；2 / 3 / 4 / 5 / 6 全空闲；ready 0；draft sa-cc 19 / 20 / 21 / 22（取证到齐等裁）、pp-seams 01 / 02 / 03（等裁）；lc 20 / 22 等第一家真源。**归用户**同上条不变：授权代裁与否是眼下唯一能让五个通道再动起来的开关。
- **本节五数**（20:5x–22:0x）：CI 未起跑（计费）· 集成时延——24 / 41 补评审无完工报不计；PP 三票 完工报 21:17 → 推出 21:3x **约 15 分**；sa-cc/25 完工报 21:2x → 推出 21:4x **约 20 分**（等四份取证凑一批）；四份取证 21:2x → 21:4x **约 20 分**；sa-cc/26 完工报 21:41 → 推出 22:0x **约 20 分**（含评审 10 分与全量 130 s 并行）——**中位数约 20 分（4 批 9 票）** · 重放笔数 **7**（0a9808ec、b0463e1a、cbdcb360、5fab18e0、a42bdbfb、5d9907aa 六笔 + 无清点笔；tails4 收尾那笔是上节的）· 重复开发 0 · **会话重置 1**（通道 1 本会话新起、无在途记忆；2–6 是上节已计过的那几次 crash 之后的恢复，不重计）· 非作者评审缺席 0（24 / 41 欠账兑清；25 纯注释自审如实写；26 通道 4 评）· 派单 **8**（评审 2、立票 1、取证 3、A 类尾巴 1、bug 1）· **flaky 点名并修掉 1**（`TestFundsFactVersionsAccrueAsRowsThatPointBack`，≈ 1/8 翻红概率，自 09-14 `e833355d` 起在 main）

### 22:0x–22:2x 用户「你是业务和系统专家，自决」→ 推送方代裁七张 draft（sa-cc 19 / 20 / 21 / 22 + pp-seams 01 / 02 / 03）转 ready → 五通道各派一单

- **代裁**（22:0x–22:2x，全按三份取证与三张票面自带取证，每票「裁决」节末写能力边界，裁结构不裁具体不变式）：**22**（CC）程序仍调用方交、不核一致（范围→程序这条边在 CC 库上不存在，造它是新领域事实），程序进记录列 + 折进 `verificationDigest`、不加主键列（五处键形不变、SA 零改动）；**19**（CC，Blocked by 22、同一作者同分支）(a′) 形成新核对版本——覆盖轴承前版（UC-CC-009「保留原覆盖判断」原句）、差额 / 有效性显式 `PENDING`、依据 / 程序承前、资金版本取新到那版并进指纹 + 记录列；触发只看「该事实已有既往核对」不看 `corrects`；`PENDING` 当前版让放行门禁答未决——正是要的效果；`LoadFundsFact`「最近接收」退役；**20**（SA）版本子表（照 CC 0021；SA 无外键但身份 / 版本分层让 `funds_mapping` / `settlement_application` 的事实引用继续指到确定的东西、且与 CC 同形），入口 = 既有采用用例加「更正」格（`Corrects` 须等于链头、否则未受理——SA 是铸造方）、复用同一交接口再发一封、**不开新面**（首版今天也没有生产入口，登记面另立 draft 归 SA）；**21**（CC 文档）A-1 同形复用落 Rules 税费段、主语是「付款人、金额、币种、业务时间等由来源提供或真实程序要求的维度」、补第三格、13 处引用零改动、UC-CC-009 启动条件补三格、12 两处归属追一句；**pp-seams 01**（TF）裸引用窄口、按 `ChargeOccurrenceKey` 精确到版本、不展开集运单元；**02**（NO·PS）拆：本票留 PS 申报口（锚法照收件地点引用），NO 半边拆新票 04（登记册 + 仍有效实测清单口、不派生「当前有效」、登记入口留后继），两源规则「实测优先、无实测用申报并标来源、目的拒用申报保持可配置」归 PP 消费侧票写进 PP CONTEXT；**03**（PS）邮编成 PS「地址要素」一格（要素名封闭集、条目列表不重构、既有快照如实答缺）、一口两段各带锚、起点先答寄件人（节点邮编另票归 NR / NO）、不过授权查询作用域。两 spec Status 行、pp-seams 子票表 +04 行、「不在本目录」+3 行。簿记一笔推出（本条上半）。
- **下半从未写下**：本节标题写的「五通道各派一单」实际只派出一单（通道 2 ← sa-cc/22+19，task-f3329ba3，22:2x），通道 2 建了树没落一笔，随后全体 crash（09-15 09:5x 接手会话按 `query_tasks` 与 `branch-state.ps1` 反推，见下节）。

## 2026-09-15 09:5x 通道 1 新会话（用户先要一次自带 `ask_question`，再经队列「原 mcp-1 crash，还没有派发任务，你来计划和安排派单，mcp-1/2/3/4/5/6 现在各通道就绪」；接手时 `main = origin/main = 3a21dab7`）

### 09:5x–10:1x：量 → 点名 → 七张 ready-for-agent 全部派出（五单）

- **量**（无在途记忆，全按 git，不读自报）：`branch-state.ps1` + `ls-remote`——main = origin = `3a21dab7`（昨 22:15 代裁簿记笔，即上一任最后一笔）；共享树 76 行 ` M` 经 `git diff --ignore-cr-at-eol --stat` 量得全是 CRLF 幻影、无真改动；未跟踪六件（`d_tasks.txt` / `docs/api/` / `handoff.md` / 三份 logo svg）归用户，未动。在途分支只 `mcp2-sacc22-19@3a21dab7`（领先 0、树 `%TEMP%\idp-parcel-mcp2-sacc22-19` 干净、未推 origin）；`query_tasks pending` 里躺着 **task-f3329ba3**（昨 22:2x 上一任派通道 2 sa-cc/22+19，pending、无 working 记录）——上一任昨晚只派出这一单，其余四单从未派出。`docker ps`：`idp-parcel-postgres-gate` 55432 健康。前沿：ready-for-agent 七张（sa-cc 19 / 20 / 21 / 22、pp-seams 01 / 02 / 03），全部昨晚代裁转 ready；in-progress 0；pp-seams/04（NO 半边）票面尚未立（02 裁决 1 写「由 NO 半边作者自立」）。
- **点名**（10:00 广播 2–6，截止 10:04；不开 `activeOnly`）：10:01 前 **2 / 3 / 4 / 6 四个应答**，全部「空闲 · 地盘无 · 余量充足」；**5 截至 10:04 未应答**（`list_sessions` 显 running / idle、lastActivity 昨 22:2x——与 3 / 4 / 6 同值而它们都答了，这一格照旧不可信）。10:05 单点 5 再点一次（截止 10:10），**10:08 应答**「空闲 · 地盘无 · 余量充足」。
- **派单五件**（10:02–10:09，全部基 `3a21dab7`，地盘互不重叠，每单照旧隔离 worktree + 每笔推 origin + 完工报到派非作者评审）：
  - **通道 2 ← sa-cc/22 + 19**（CC，同分支两笔 22 先落）：沿用 task-f3329ba3 不另建记录，`send_to_session` 只补三件——树已在不再 `worktree add`、时限从现在起算（12:00 报位置）、撕点对手是通道 6。
  - **通道 6 ← sa-cc/20**（task-173daa46，high）：SA 版本子表照 CC 0021 + `CorrectFundsFactCommand` 走 `MapExternalFundsHandler` + 复用 handoff 再发一封 + `cmd/parcel-dispatch` 正例改回同一事实两版都在 SA + 自立 draft 27「SA 采用与更正的登记面」；11:30 报位置。
  - **通道 3 ← sa-cc/21**（task-7d92552e，纯 docs）：CC CONTEXT Rules「税费、放行与案件闭环」段第 ② 条之后补同形句 + 第三格、UC-CC-009 启动条件三格、票 12 Comments 追两句、13 处引用逐条核零改动；10:50 报位置；**完工后留作评审位**（先评 pp-seams/01）。
  - **通道 4 ← pp-seams/01**（task-aadedbce）：TF 发生项成员窄只读口，形照 `FailedAttemptSource`，键 `ChargeOccurrenceKey` 精确到有效性版本，`ChargeOccurrences` 加方法，不迁移；11:00 报位置。
  - **通道 5 ← pp-seams/02 + 03**（task-9cc0b1a6，high；5 第二次点名应答后派）：PS 申报重量 / 尺寸口 + 邮编口（一口两段 + CONTEXT「地址要素」），同分支两笔 02 先落、共用「按包裹身份解析寄 / 收件资料范围版本锚」内部步骤——合成一单的理由：两票裁决同一套锚法与封闭答格，分两人做会在 PS `adapters/postgres` 各写一份；不加列不加迁移；11:30 报位置。
  - **撕点写进相关单**：`cmd/parcel-dispatch/assemble_test.go` `TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable` 通道 2（19 判据 (2) 扩一格）与通道 6（20 判据 (3) 第二格改回两版都在 SA）都要改——动前排队列 + 占号写明用例与段、后进 main 的一方 rebase、建议单独成笔；`migrations/migrations.go` / `plan.go` 通道 6 新迁移接线、动前占号；CC CONTEXT 通道 3 独占、通道 2 要加句先问 3；两份 spec 子票表各改自己一行、重放时我合。
- **接下来**：21（3）约 10:50 交 → 自审 → 进 main；01（4）约 11:00 交 → 派 3 评 → 重放 → 全量 → ff；22 首笔到可先评；20 / 19 / 02+03 陆续交 → 评审位从已交活的通道里挑非作者。pp-seams/04（NO：登记册 + 迁移 `node_operations/0005` + 仍有效实测只读口，先自立票面照 02 裁决 1）等 3 或 4 空出后派。**归用户**（不变）：CI 计费；六件 untracked；会话中断成因（昨 22:2x 全体 crash 一次、通道 1 今天新起）；四篇 ADR 越权风险点事后复核（`scripts/owner-review-queue.ps1`）。
- **本节五数**（09:5x–10:1x）：CI 未起跑（计费）· 集成时延 —（本小节无票进 main）· 重放笔数 0 · 重复开发 0 · **会话重置 6**（通道 1 本会话新起 + 2 / 3 / 4 / 5 / 6 均无在途记忆；昨晚 22:2x 全体 crash 记 1 次事件）· 非作者评审缺席 0 · 派单 **5**（代码 4、docs 1；覆盖七张 ready）

### 10:1x–10:2x 同一会话：接单确认 → sa-cc/21 完工 → 推送方自审 → 进 main

- **接单确认**（10:1x）：通道 2（task-f3329ba3 working；报「建 worktree」——已提醒树与分支已在、不要重建）、通道 4（task-aadedbce working）。簿记 `4bbe0a9c` 推出后广播「远端 main = 4bbe0a9c，只动 tasks.md，各分支基 3a21dab7 不必动」。
- **sa-cc/21 完工报 10:1x**（通道 3，task-7d92552e done，派后约 10 分）：`mcp3-sacc21@efd6549f` = origin，三笔（认领 `97b05cf8` / 之一 `08eb6d59` / 完成记录 `efd6549f`），五件 +47 −3 全在 `docs/` 与 `.scratch/`；判据 (1) grep 1→2、(2) `internal/ migrations/ apps/` 零 diff、(3) UC-CC-009 三格 + `CC-RULE` 指向、(4) 票 12 两句；13 处引用逐条 Read 核过零改动清单；判断项四条（落点未挪 / UC 指向用既有引用键 / UC-CC-008 对称句归 CC owner / 票 12 判定与取证一致）。
- **评审门**：纯 .md → 推送方自审，读全部五件 diff：**Standards 0 阻断 / 0 非阻断 · Spec 0 阻断 / 0 非阻断**（CONTEXT 新句与裁决 1 逐字同、落第 ② 条后；UC 句指向用既有 `CC-RULE` 键，认可；13 处清单与取证条逐处对上）。全文落票 21 Comments。
- **重放**：`%TEMP%\idp-replay-sacc21` @ `4bbe0a9c`，`cherry-pick 3a21dab7..efd6549f` 零冲突 → `c4932992` / `ac39f9a5` / **`b62bab28`**；`git diff efd6549f b62bab28 -- docs .scratch/sa-cc-*` 空。纯 docs 不跑 go、清点不重生成。共享 main `merge --ff-only b62bab28` → 簿记一笔在其上（票 21 Status「已进 main」+ 进 main 记录；spec 21 行；本条）→ `ls-remote` 核 `4bbe0a9c` 未动 → `push <sha>:main`。`mcp3-sacc21` → `merged/`、远端删；重放树拆；作者树由通道 3 比内容后拆。
- **前沿**：在途 22+19（2）/ 20（6）/ 01（4）/ 02+03（5）；通道 3 空闲 → **评审位**（等 pp-seams/01 约 11:00）。ready 0；draft lc 20 / 22（等真源）。
- **本节五数**（10:1x–10:2x）：CI 未起跑（计费）· 集成时延 sa-cc/21 完工报 10:1x → 推出 10:2x **约 10 分**（1 票，自审）· 重放笔数 **3** · 重复开发 0 · 会话重置 0 · 非作者评审缺席 0（纯 .md 自审如实计）

### 10:2x–11:0x 上一任通道 1（未写台账即 crash，本节按 git 与任务台账反推）：首波四票评审到齐 → 隔离重放 → 全量 113 ok → 本地 ff → **未推、未提交簿记** → crash

- **反推依据**（不读自报）：`query_tasks` 里 10:31–10:57 五单——task-5c5e6165（通道 3 接手 pp-seams/01 封存现场 `3b91cb5a`，10:44 done）、task-ccd57329（评 sa-cc/22 → 通道 6，10:45 done，0 阻断）、task-a9e4e4d8（评 sa-cc/20 → 通道 3，10:51 done，0 阻断 / Standards 2 非阻断 / Spec 3 非阻断）、task-e2d98535（评 pp-seams/01 → 通道 6，10:56 done，0 阻断）、task-eaefa42a（评 pp-seams/03 → 通道 6，11:01 done，0 阻断）；pp-seams/02 推送方自评（票面 Comments 11:1x 条）。隔离树 `%TEMP%\idp-replay-wave1` @ `c657a5e7` = 首波批 tip（pp-seams/02 两笔 → sa-cc/22 三笔 → pp-seams/01 三笔含封存笔 → sa-cc/20 六笔 → 清点 `c657a5e7`，共 15 笔在 `bd919b72` 之上，`merge-base --is-ancestor` 成立）；10:48 广播「`c657a5e7` 带 DSN `-p 1 -count=1 ./...` 113 ok / 0 FAIL / 16 无测试 / 0 cached（131 s）」——本任（当时通道 6）亲收。共享 `main` 已 `ff` 到 `c657a5e7`；远端仍 `bd919b72`（11:04 `ls-remote`）。六件簿记（票 20 / 22 / pp 01 / 02 的 Status「已进 main」+ 评审代落 + 进 main 记录、两份 spec 行）已写在共享树、**未提交**——`git diff --ignore-cr-at-eol --stat` 量得只此六件是真改动，其余 76 行 ` M` 仍是 CRLF 幻影；逐件读末行确认写完整、无半句。

## 2026-09-15 11:0x 通道 1 再次新会话（用户经队列「mcp-1 crash」把本窗口从通道 6 改绑通道 1；接手时 main 本地 `c657a5e7`、远端 `bd919b72`）

### 11:0x–11:1x：接上一任断点——七件簿记同笔 → 推首波

- **角色重叠如实记**：本任原是通道 6——sa-cc/20 的**作者**、sa-cc/22 / pp-seams/01 / pp-seams/03 的**评审人**。首波四票的评审门都由非作者完成（02 推送方自评并如实写、22 ← 6、01 ← 6、20 ← 3），门在接手前已过；本任只做推送方那半（簿记、`ls-remote`、push、改名分支），不改任何票的评审结论。
- **处置**：不重跑全量——`c657a5e7` 是不动的对象，10:48 那一跑就是它的证据；本笔只加 `.md`（上一任写好的六件 + 本节），纯簿记不跑 go。七件 `git commit --trailer "Co-authored-by: Cursor <cursoragent@cursor.com>" -- <路径>` 同笔在 `c657a5e7` 之上 → `ls-remote` 核 `bd919b72` 未动 → `push <sha>:main` → 广播远端 main。分支：`mcp6-sacc20` / `mcp3-ppseams01` / `mcp4-ppseams01` → `merged/`、远端删；`mcp2-sacc22-19`（19 在做）与 `mcp5-ppseams02-03`（03 待第二波）不改名不删。树：`idp-replay-wave1` 与五棵 `idp-review-*` 由本任比内容后拆；`mcp6-sacc20` 作者树是本任自己的，比内容后拆；`mcp3-ppseams01` / `mcp4-ppseams01` 两棵作者树（3 / 4 都 crash 过）由本任比内容后拆。
- **第二波待办**：pp-seams/03 两笔 `2a637107` / `3209c4b3`（`mcp5-ppseams02-03`，评审 ← 通道 6 两轴 0 阻断 / 各 1 非阻断，评审全文待代落票 03 Comments）→ 重放到新 tip → 清点重生成 → 带 DSN 全量一次 → ff → push。
- **前沿**：在途 sa-cc/19（通道 2，`mcp2-sacc22-19`，22 已进 main、19 在其上）；2 以外各通道空闲；ready 0；draft sa-cc/27（SA 登记面，归 SA owner）、pp-seams/04（NO 登记册，归 NO）、pp-seams/05（PS 留内容，归 PS owner）；lc 20 / 22 等真源。**归用户**：CI 计费；六件 untracked（`d_tasks.txt` / `docs/api/` / `handoff.md` / 三份 logo）；会话中断成因（今天通道 1 已重置两次、4 一次）；四篇 ADR 越权风险点复核。
### 11:1x–11:2x：第二波 pp-seams/03 进 main

- **首波推出**（11:1x）：七件簿记同笔 `1e74aaaf` 在 `c657a5e7` 之上（上一任写好的六件 + tasks.md 两节），`ls-remote` 核 `bd919b72` 未动 → `push 1e74aaaf:main`；广播远端 main。分支 `mcp6-sacc20` / `mcp3-ppseams01` / `mcp4-ppseams01` → `merged/`、远端删；九棵树（三棵作者树 + `idp-replay-wave1` + 五棵 `idp-review-*`）逐棵 `status` 空、内容与 `origin/main` 比过（SA / TF 代码路径零 diff；`assemble_test.go` 只差 22 那一行）后 `worktree remove`（不加 `--force`，全部退 0、目录不存在）。
- **第二波**：`%TEMP%\idp-replay-wave2` @ `1e74aaaf`，`cherry-pick 2a637107`（撞清点 → 取 main、后重生成）→ `1bd50208`；`cherry-pick 3209c4b3`（撞 pp-seams `spec.md`：01 / 02 行取 main、03 / 05 行取分支）→ `6e712d64`；03 地盘内容与作者 tip 零 diff。清点重生成 `444513c7`（PS 生产 182→186 / 测试 175→178 / 适配器 33→34、端口 410→411、缺数不变，与作者预报同）。`gofmt` 空、build / vet 0。11:1x `check_messages(waitMs 0)` 排队列（通道 2 刚释 55432、sa-cc/19 五包 484 PASS）→ 占 55432 → `444513c7` 带 DSN `-p 1 -count=1 ./...` **113 ok / 0 FAIL / 16 无测试 / 0 cached**（131 s）、探针 PASS 非 SKIP → 释。共享 main `ff` → 本簿记（票 03 Status + 评审代落 + 进 main 记录、spec 03 行、本条）→ `ls-remote` 核 `1e74aaaf` → push。
- **通道 2 在途**（11:1x 广播）：sa-cc/19 除 `assemble_test.go` 那一格外已验绿（五包 484 PASS），占了 `assemble.go` 两处注释 / 哨兵；等 20 进 main 后 rebase 再写那一格——20 已在 `1e74aaaf`，可以开始。
- **前沿**：在途 sa-cc/19（2）；3 / 4 / 5 / 6 空闲；ready 0；draft sa-cc/27、pp-seams/04 / 05；lc 20 / 22 等真源。**归用户**：同上节。
- **本节五数**（11:0x–11:2x）：CI 未起跑（计费）· 集成时延——首波四票完工报 10:41 / 10:44 / 10:54（20 / 01 / 02+22）→ 推出 11:1x **约 25–35 分**（含上一任 crash 空档约 15 分）；03 完工报 10:54 → 推出 11:2x **约 30 分**——中位数约 30 分（5 票 2 批）· 重放笔数 **17**（首波 15 + 第二波 2）· 重复开发 0 · **会话重置 1**（通道 1 本会话由通道 6 改绑）· 非作者评审缺席 0（02 推送方自评如实计）

## 2026-09-15 11:4x 通道 1 又一次新会话（无上一任在途记忆；通道 2 同刻也报「新会话、无 sa-cc/19 上下文」——今天通道 1 第三次重置、通道 2 第一次）

### 11:4x–12:0x：接尾巴——mcp5 分支收尾 · 答「现在可以回放了吗」

- **量**（全按 git）：main = origin/main = `7160fe67`。通道 5 报拆树（`%TEMP%\idp-parcel-mcp5-ppseams02-03` 已不在、porcelain 干净），留分支给通道 1 收尾：自核 `mcp5-ppseams02-03@3209c4b3` 自 merge-base `3a21dab7` 改过的 25 件里，`internal/parcelshipment` / `docs/domain/parcel-shipment/CONTEXT.md` / 接线基线 / issues/05 对 main **零 diff**，只差 issues/02 / 03 Status 行 + 追加记录、spec 子票表、清点重生成——全簿记 → `git branch -m` 为 `merged/mcp5-ppseams02-03`（指针留、`--unset-upstream`）、`push origin --delete mcp5-ppseams02-03`。
- **用户问「现在可以回放了吗」**（两棵树 mcp5 / mcp2）：mcp5 无可回放之物；`mcp2-sacc22-19@a0cb6fef` 两笔 sa-cc/19 树干净、已推、与 main 自 `1e74aaaf` 以来的 21 件改动**零文件重叠**、cherry-pick 预期零冲突——但缺两道门：作者完成记录未写（分支没碰票 19 .md，原通道 2 会话已换）、非作者评审未做。答：可以起链，不能直接 ff；用户「全部你自决」。

### 12:0x–12:2x：第三波 sa-cc/19 进 main（点名 → 双派 → 隔离重放 → 全量 → ff → push）

- **点名**（11:59 广播、截止 12:03）：2 / 3 / 4 / 5 / 6 五个应答全在截止前、全报空闲 · 地盘无 · 余量充足。派两单：**task-3b395d37 → 通道 6** sa-cc/19 两轴评审（只读，钉 `a0cb6fef` 基线 `1e74aaaf`，隔离检出 `%TEMP%\idp-review-sacc19`，不占 55432）；**task-d230f095 → 通道 2** 接手作者半边——只写完成记录 + 带 DSN 六包验证、不改代码、只碰票 19 .md（spec / tasks / 清点留给推送方，避免撞）。两单并行、互不等。
- **并行本侧**：`%TEMP%\idp-replay-wave3` @ `7160fe67`，`cherry-pick 6ab8e41c a0cb6fef` 零冲突 → `e0201ca6` / `07341b8b`，CC 地盘与作者 tip 零 diff；`gofmt` 空、build / vet 0。
- **两路回报**（12:1x–12:2x）：通道 2 占 55432 六包 `-p 1 -count=1 -v` **1698 PASS / 0 FAIL / 0 SKIP** 释号；通道 6 评审两轴 0 阻断 / Standards 2 非阻断（适配器替身无序、夹具手拼键）/ Spec 3 非阻断（找不到版本答 `FundsFactNotReceived` 非`未受理`、重派路续办引用无读者、`verifiedAt` 编排时钟），三条不变式逐条读代码写出，结论可进 main；通道 2 完成记录笔 `dbff7f5f`（只票 19 .md，+64 −1，判断项五条含信封 ID 128 字节）。
- **推送方**：清点重生成 `b6e7bd34`（CC 生产 93→94 / 测试 93→94 / 适配器 17→18；迁移 173→174，customs_compliance 22→23）→ 占 55432 → `b6e7bd34` 带 DSN `go test -p 1 -count=1 ./...` **113 ok / 0 FAIL / 16 无测试 / 0 cached**（133 s）、探针 `TestVerificationsRoundTripTheFundsFactVersionTheyJudgedAndListByFundsFact` PASS 非 SKIP → 释。`cherry-pick dbff7f5f` → `1f3ed3d4`（票面与作者 tip 零 diff）。簿记同笔（票 19 Status「已进 main」+ 评审代落 + 进 main 记录含推送方补判断项 ⑥、sa-cc spec 19 行、本节）→ 共享 main `merge --ff-only` → `ls-remote` 核 `7160fe67` 未动 → `push <sha>:main`。分支 `mcp2-sacc22-19` → `merged/`、远端删；作者树由通道 2 比内容后拆。
- **同时派出的三单**（用户 12:1x「mcp-3/4/5 空闲了，还有任务吗」；按票面 Status 量前沿：ready-for-agent 两张、draft 三张、needs-info / blocked 六张不派）：**task-93aa833b → 通道 3** pp-seams/06（A 类尾巴：TF 去「三件」+ 成员顺序进契约；PS 地址要素空白值三处同口 + 口头注收窄；分支 `mcp3-ppseams06` 基 `7160fe67`）；**task-1b28ece5 → 通道 4** sa-cc/28（零行为，SA 三处注释；`mcp4-sacc28`）；**task-4e80c3bb → 通道 5** sa-cc/27 裁决前只读取证（不裁不提方案不动代码；`mcp5-sacc27-evidence`）。三单都被告知不占 55432。
- **前沿**：在途 pp-seams/06（3）、sa-cc/28（4）、sa-cc/27 取证（5）；2 / 6 空闲；ready 0；draft pp-seams/05（PS owner 四条要裁的，可授权代裁）、sa-cc/27（取证中）；**待立票**（CC owner）：05 信封 ID 128 字节 + 重派路续办引用无读者 + 19 评审 Standards 两条尾巴；lc 20 / 22 等真源。**归用户**：CI 计费；六件 untracked（`d_tasks.txt` / `docs/api/` / `handoff.md` / 三份 logo）；会话中断成因（今天通道 1 三次、2 一次、4 一次）。
- **本节五数**（11:4x–12:2x）：CI 未起跑（计费）· 集成时延——19 代码完工（作者 `a0cb6fef` 11:22 推出）→ 进 main 12:22 **约 60 分**（其中约 25 分是原作者会话消失、完成记录无人写的空档；点名到两路回报约 20 分）· 重放笔数 **3**（代码 2 + 完成记录 1）· 重复开发 0 · **会话重置 2**（通道 1、通道 2）· 非作者评审缺席 0

### 12:2x–12:3x：第四批 sa-cc/28 + pp-seams/06 + sa-cc/27 取证进 main

- **三单回报**（12:1x–12:2x，全在派出后 10–25 分内）：通道 4 sa-cc/28 `mcp4-sacc28@88ca0a2e`（代码 `d6b1f773` 三文件九增两删全 `//`、清点零差、判断项六条）；通道 3 pp-seams/06 `mcp3-ppseams06@69189208`（单笔六件 +51 −11 零增删；缺口 3 取票面默认口，`AddressElementsOf` `TrimSpace(v) != ""` → `v != ""` 先 red 后绿；判断项七条）；通道 5 sa-cc/27 取证 `mcp5-sacc27-evidence@af2b59db`（票 27 Comments +39，只列不选；最要紧一句：要裁的 3「构造期拒 nil 统一形」前提不成立——SA application 十二只 `New*Handler` 两只拒 nil、十只不校验，且与要裁的 1 耦合）。通道 2 12:4x 报 sa-cc/19 作者树已拆（CC / cmd / admin-web / 迁移对 `49ffc96c` 零 diff，票 19 只差 Status 前缀与两条 Comments）。
- **评审门**：sa-cc/28 纯注释 → 推送方自审（`git diff -U0 7160fe67 d6b1f773 -- internal/` 滤 `^[+-]\s*//` 后为空，措辞对做法 1–3，判断项 ①–⑥ 接受，票面如实写）；sa-cc/27 取证纯 .md → 自审；pp-seams/06 有一处读值分支 → **task-b8929141 → 通道 4**（12:4x 交活时自报空闲即派，未另点名）两轴评审：0 阻断 / Standards 1 非阻断（`AddressElements` 头注「只认这一种缺席」略宽）/ Spec 2 非阻断（`AddressElementsOutcome` 头注宽口归 05；成员序权威在库 collation，PP 消费侧进指纹前自排字节序），行为改动副作用核过——`AddressElementsOf` 今天零非测试调用方、运行时不可达；判断项 ①–⑦ 全同意。
- **重放**：`%TEMP%\idp-replay-wave4` @ `49ffc96c`：`cherry-pick d6b1f773 88ca0a2e` → `981a6f15` / `ee3ec953`；`cherry-pick 69189208` → `4ca2ccac`；`cherry-pick af2b59db` → `676cc09b`（全部零冲突、各自地盘与作者 tip 零 diff）。清点在 `4ca2ccac` 重生成**零差**（本批零增删文件）。**验证（推送方全量一次，`4ca2ccac`；其后只叠一笔纯 .md）**：`gofmt` 空、build / vet 0；占 55432 → 带 DSN `go test -p 1 -count=1 ./...` **113 ok / 0 FAIL / 16 无测试 / 0 cached**（134 s）、探针 `TestAddressElementsAreReadByClosedEntryNameOnly` PASS → 释（评审尚未回时先跑，评审 0 阻断故不重跑）。簿记同笔（票 28 / 06 Status「已进 main」+ 评审代落 / 自审 + 进 main 记录、sa-cc spec 27 / 28 行、pp-seams spec 06 行、本节）→ 共享 main `merge --ff-only` → `ls-remote` 核 `49ffc96c` 未动 → `push <sha>:main`。分支 `mcp4-sacc28` / `mcp3-ppseams06` / `mcp5-sacc27-evidence` → `merged/`、远端删；三棵作者树由各通道比内容后拆。
- **前沿**：全通道空闲；ready 0；draft pp-seams/05（PS owner 四条要裁的）、sa-cc/27（取证已落，SA owner 三条要裁的——取证量得要裁的 3 前提不成立，裁时先看那条）；**待立票**（CC owner）：05 信封 ID 128 字节 + 重派路续办引用无读者 + 19 评审 Standards 两条尾巴；**残差待收**（PS owner 随 05）：`AddressElements` 头注「只认这一种缺席」、`AddressElementsOutcome` 头注宽口；（PP 消费侧票）成员序进指纹前自排；（TF owner）`charge_occurrence_registry.go` `LoadMembers` 头注「三件」；lc 20 / 22 等真源。**归用户**：CI 计费；六件 untracked；会话中断成因（今天通道 1 三次、2 一次、4 一次）；两张 draft 要不要授权代裁。
- **本节五数**（12:2x–12:3x）：CI 未起跑（计费）· 集成时延——28 完工报 12:1x → 进 main 12:35 **约 20 分**（等 06 同批）；06 完工报 12:2x → 进 main 12:35 **约 10 分**；27 取证报 12:3x → 12:35 约 5 分 · 重放笔数 **4**（28 两笔 + 06 一笔 + 27 一笔）· 重复开发 0 · 会话重置 0 · 非作者评审缺席 0（28 / 27 纯注释 / 纯 .md 自审如实计）

### 12:4x：立两张后继票（用户问「下一步呢，还有工作吗」）

- 三棵作者树 12:4x 前全拆（3 / 4 / 5 各报对 `origin/main` 零 diff；`worktree list` 只剩 main、`prune` 无残留）。
- **立票**（推送方处置时点名「归 CC owner 立票」那一件 + 19 评审 Standards 两条尾巴）：**sa-cc/29** draft「CC 三只交接信封 ID 超框架上限被 `EnqueueOnce` 拒收 + `handOffVerification` 吞错留续办引用、重派路上无读者」——缺口逐符号、做法候选 ID 形状三条 × 响法三条只列不选、四条要裁的归 CC owner，能力边界写明框架上限源码未读；**sa-cc/30** ready-for-agent「`rederiveStores` 替身无序 + `duty_registers_test.go` 手拼键」只测试文件零生产改动。两份 spec 行随笔。纯 .md，共享 main 直接一笔、`ls-remote` 核 `36fb5437` 未动后 push。
- **前沿**：ready **1**（sa-cc/30，可派任一空闲通道）；draft 三张——pp-seams/05（PS owner 四条）、sa-cc/27（SA owner 三条，取证已落）、sa-cc/29（CC owner 四条）——**要动它们只差 owner 裁决或用户授权代裁**；PS 残差两处随 05；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**：CI 计费；六件 untracked；三张 draft 是否授权代裁。
- sa-cc/30 → **task-38d5d3a7 → 通道 2**（12:4x 拆树时自报空闲即派）。

### 12:5x：用户「代裁」→ 三张 draft 转 ready

- **授权**：用户经队列一句「代裁」，对应上节归用户那一格；三张各按其 owner 口径代裁，裁决节都写明依据与能力边界，作者量到与代码不符以代码为准写判断项。
- **pp-seams/05（PS owner 口径）**：只留封闭要素不留原文；做法 1（两份既有 jsonb 快照各加可缺席内容子段，零迁移，旧快照如实答缺；重开条件写明）；内容与摘要由 `CanonicalizeSubmissionPayload` 一次调用成对产出、两条命令一起带、应用层不重算、不设领域构造门（结构上只留一条产出路径）；CONTEXT「保留原始请求」仍指指纹、词条末加一句留封闭要素内容；06 评审两条 PS 残差顺带收进；完成判据五条写实。
- **sa-cc/27（SA owner 口径）**：先裁要裁的 2——ADR-0085 读作「算」，判据是它治的那张脸（「尚未接线」与「未配置」同脸）而非 ADR 没画的「配置 / 事实」边，先例 CC 协作 / 核对；入口两者都要分两步、本票只做第一步 `cmd/parcel-settlement-register` + `adapters/registrationjson` + 事务壳 Outbox 同 db + 六口全接真，第二步端点 + 管理台随完成记录立 ready；`NewMapExternalFundsHandler` 单只改拒 nil，十只统一形不立法（取证量得 2 / 10 并存、有生产装配的四只两拒两不拒——没有可照的形）；退出码归格原则五条；完成判据七条写实。
- **sa-cc/29（CC owner 口径）**：先量实上限——`idp-bento-go@v0.1.0-rc.2` `eventing/types.go` `MaxEventIDLength = 128`、`Envelope.Validate` 确定性拒（补票面「没读」那一格）；三只 `*EventID` 一次同改成「口名前缀 + `sha256` 十六进制」定长形（唯一不依赖实例半边长度就能保证的路），载荷五维照旧，`Subject` / `PartitionKey`（上限 512）作者量能否超；重派路交接失败分两格整笔回滚——依赖不可用归未决重投、信封不合法归硬失败（新哨兵不进未决集合，`dispatcher.go`「硬失败人动手」那格），不登续办（登要改词条 / 日志不是结构）；人重核路 05 兜底不动；SA 靠采用登记册五维主键守幂等、真库用例钉住；完成判据六条写实。
- **簿记**：三票 Status 转 ready、两份 spec 三行、本节；纯 .md 共享 main 一笔、`ls-remote` 核 `e1ab9fb5` 未动后 push。
- **派单**：点名后派 05 / 27 / 29 给空闲通道（2 在做 30）。撞点：29 碰 `receive_on_adopted_funds_fact.go`（生产）与 `assemble_test.go`，30 碰同目录 `receive_on_adopted_funds_fact_test.go` 与 `duty_registers_test.go`——29 的作者不动 `rederiveStores` 替身、新增用例另起或等 30 进 main 后 rebase；05 在 PS、27 在 SA + 新 `cmd/`，与 29 / 30 零重叠。
- **前沿**：ready 0（三张转 ready 即派）；在途 sa-cc/30（2）+ 三票；draft 0；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**：CI 计费；六件 untracked。

### 13:0x–13:1x：时刻校正 → 点名派三票 → 第五批 sa-cc/30 进 main

- **时刻校正**（`bb7268f0`）：本会话前几笔簿记的 HH:Mx 是估的、比提交时间晚 10–30 分（19 进 main 写 12:3x 实为 12:22，第四批写 12:5x 实为 12:35，立票写 13:0x 实为 12:43，代裁写 13:1x / 13:2x 实为 12:57）；按 `git log --date` 逐条改回，两条集成时延随之改小。教训：写时刻先 `Get-Date`，不估。
- **点名**（13:00 广播、截止 13:03）：3 / 5 / 6 / 4 / 2 五个应答全在截止前、全空闲。派三单：**task-6dbb178b → 通道 5** pp-seams/05（02 / 03 作者，自己立的 draft；分支 `mcp5-ppseams05` 基 `bb7268f0`）；**task-43a6ae28 → 通道 3** sa-cc/27 第一步（`mcp3-sacc27`；同笔立后继票 31 第二步端点 + 管理台）；**task-2cc18a20 → 通道 6** sa-cc/29（19 的评审人、非作者；`mcp6-sacc29`）。撞点写进单：29 动 `receive_on_adopted_funds_fact_test.go` 前先 rebase 到含 30 的 main、不改替身排序。2 / 4 留作评审人。
- **第五批 sa-cc/30**（通道 2 12:5x 交活 `mcp2-sacc30@c9c9b8f5`，单笔三文件 +100 −11、零生产改动、做法 2 做了并 red / green）：评审门推送方自审（只测试文件，逐 hunk 读，判断项四条接受）。`%TEMP%\idp-replay-wave5` @ `bb7268f0`，`cherry-pick c9c9b8f5` 撞票 30 .md（main 侧时刻校正 vs 分支侧 Status → resolved）→ 按意图解取分支 Status、立票时刻取 main → `3dc8f94c`，测试文件与作者 tip 零 diff；清点零差。**验证（推送方全量一次，`3dc8f94c`）**：`gofmt` 空、build / vet 0；占 55432 → 带 DSN `-p 1 -count=1 ./...` **113 ok / 0 FAIL / 16 无测试 / 0 cached**（134 s）、探针新格 PASS → 释。簿记同笔（票 30 Status「已进 main」+ 自审 + 进 main 记录、sa-cc spec 30 行、本节）→ 共享 main `ff` → `ls-remote` 核 `bb7268f0` → push。分支 `mcp2-sacc30` → `merged/`、远端删；作者树由通道 2 比内容后拆。
- **前沿**：在途 pp-seams/05（5）、sa-cc/27（3）、sa-cc/29（6）；2 / 4 空闲留评审；ready 0；draft 0；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**：CI 计费；六件 untracked。
- **本节五数**（13:0x–13:1x）：CI 未起跑（计费）· 集成时延——30 完工报 12:5x → 进 main 13:1x **约 15 分** · 重放笔数 **1** · 重复开发 0 · 会话重置 0 · 非作者评审缺席 0（30 只测试文件自审如实计）

## 2026-09-15 14:1x 通道 1 新会话（今天通道 1 第四次重置；上一任 13:3x–13:40 在分支 `mcp3-sacc27` 上代写 27 完成记录后消失、本节前半由它未及簿记；用户先用自带 `ask_question` 验通道，再问「已经全部完成，可以 replay 了吗」）

### 13:1x–13:4x 补记（上一任通道 1 未及写入 tasks.md；按 task 台账 result 字段、分支 git log 与广播补，时刻取提交时间）

- **sa-cc/29 两次改派**：13:0x 派通道 6（task-2cc18a20）→ 用户 13:1x 报 crash，推送方核 `mcp6-sacc29` = `bb7268f0` 零提交零改动、树拆、空分支删 → 13:1x 改派通道 2（task-6ad990ad）→ 用户 13:2x 报 crash（截图末句「30 树已拆、回到监听」，尚未读到单），核无分支无树 → 13:2x 二次改派通道 4（task-7d4346c0，基 `cbffc269`，撞点写明 30 已在 main、不改替身排序）。通道 4 13:36 / 13:4x / 13:47 三笔逐笔推，带 DSN CC 全部包 + dispatch + SA adapters/customscompliance + architecture **886 PASS / 0 / 0**，14:1x 释号广播「转入完成记录」。
- **sa-cc/27 作者 crash、推送方封存 + 代写**：通道 3 13:11–13:27 五笔代码推齐、13:2x 广播 904 PASS / 0 SKIP，13:3x 写完成记录时 crash（用户报）；上一任通道 1 把树里未提交的两件（票 27 Status 行 + 后继票 31 全文）原样封存 `5fb936a3`，按 `git diff bb7268f0 aa48912e` 代写完成记录 `03e6b951`（13:39），随即发现替换脚本把票 31 全文的「2」换成了「0」（36 处）、从封存笔恢复 `bb82df21`（13:40）；task-43a6ae28 记 failed（result 写明缺「完成记录节（已代写）+ 非作者评审」）。此后上一任无任何动作——会话消失于 13:40–14:1x 之间。
- **pp-seams/05**：通道 5 13:13 认领笔、13:2x–13:43 四笔代码逐笔推；13:5x 广播占 55432 跑真库后会话结束——未释号、未留数、未写完成记录；task-6dbb178b 始终 pending（作者从未 `report_task`）。

### 14:1x–14:2x：量状态 → 答「不能 ff、可以起链」→ 点名 → 派三单评审 → 催两份完成记录

- **量**（全按 git，不读任何人自报）：`main = origin/main = cbffc269`；共享树 `git status` 76 处改动经 `--ignore-cr-at-eol` 全是 CRLF 幻影、真改动 0、六件 untracked 归用户；三棵作者树——`mcp3-sacc27@bb82df21` 干净已推（27 记录已代写、**缺评审**）、`mcp4-sacc29@b2229462` 两件 .md 未提交（Status→resolved + 05 一句，完成记录节未写；task working）、`mcp5-ppseams05@a893999a` 干净已推（Status 仍 in-progress、无完成记录、无带 DSN 数；task pending）。三票地盘互不重叠，与 main 自 `bb7268f0` 起两笔（30 的两份 CC 测试 + .md）零文件重叠。
- **干跑重放**：`%TEMP%\idp-replay-wave6` detached `cbffc269`，`cherry-pick bb7268f0..mcp3-sacc27`（8）→ `bb7268f0..mcp5-ppseams05`（5）→ `cbffc269..mcp4-sacc29`（3）全部零冲突，tip `d1ebc8c1`，三块地盘对作者 tip 零 diff；`gofmt -l` 空、build / vet 0。**答用户**：还不能 ff / push——三票代码齐、两道门都没过（27 缺评审；29 记录半截 + 缺评审；05 无记录 + 缺评审）；已起链。
- **点名**（14:19 广播、截止 14:23）：3 / 2 / 5 / 4 / 6 五个应答全在截止前，**全报「空闲 · 地盘无」**——含正在做 29 的通道 4 与做 05 的通道 5，即两者都已是无在途记忆的新会话（今天的会话重置形态）。派三单评审（只读、不占 55432、20 分钟）：**task-8cedf7c9 → 通道 2** sa-cc/27（钉 `aa48912e`）、**task-9fe20d5b → 通道 6** sa-cc/29（钉 `b2229462`；它评过 19，这张正是它 Spec ② 量出来的）、**task-70b9cc3d → 通道 3** pp-seams/05（钉 `a893999a`；它做过 06）。定向催通道 4 按 diff 补写 29 完成记录同笔提交推送、催通道 5 报定稿 tip / 重跑带 DSN / 写完成记录并 `report_task`。
- **两位作者新会话接手**：通道 5 14:2x 占号重跑 05 受影响范围 → **2219 PASS / 0 / 0**（24 s）→ 释号 → 完成记录 `f7580e47`（判断项九条；⑤ 修订路无生产填写方等 PAR-INT-01、⑥ admin-web 提交页未长四格为越地盘残差）→ `report_task done`；通道 4 14:28 完成记录 `5d2a6807`（判断项九条；③ Subject / PartitionKey 能超 512 未哈希、靠硬失败兜；清点预报零差；Status 时刻改实际）→ `report_task done`。

### 14:2x–14:4x：第六批 sa-cc/27 + pp-seams/05 + sa-cc/29 进 main

- **三路评审**（14:28 / 14:38 / 14:4x 回报，全在 20 分钟内）：**27 ← 通道 2**：两轴 0 阻断 / Standards 2 非阻断（夹具币种 `EUR` 宜 `XTS`；`registrationjson` 头注「采用四格」计数）/ Spec 2 非阻断（**SA 信封 ID `分区键 / 版本` 无长度上界**，同 29 那种 128 字节确定性拒 + `handOffFact` 折续办，29 裁决不覆盖 SA；判断项 ⑤「钉全覆盖」措辞过）→ 可进。**29 ← 通道 6**：两轴 0 阻断 / Standards 2 非阻断（`assemble_test.go` 头注把三只哨兵数成两只；`fingerprintEventID` 头注引 138 字节没锚 SHA）/ Spec 5 非阻断（Subject / PK 能超 512 未哈希——要求完成记录写明「能超、为何不哈希」，推送方核判断项 ③ ⑨ 已写；判据 (4) 真库格落 `assemble_test.go` 走生产路更强；默认格把确定性装配缺陷也归未决、实际到不了；**其余五只 `*EventID` 仍串接形**；判据 (6) 05 句随记录笔落），**19 评审 Spec ② 在新代码上不再成立** → 可进。**05 ← 通道 3**：Standards 0 阻断 / 1 非阻断（收件夹具真邮编 `10115` / `2009x` / `DE`，03 已有先例）/ Spec 1「阻断」= 判据 (4) 清点未在 tip 重生成（PS 186→187 / 178→181）→ 推送方答：这正是本仓流程里推送方在重放 tip 重生成、单独成笔那一步（27 也增七件、作者同样未重生成），不回作者。
- **重放与清点**：wave6 续 `cherry-pick 5d2a6807`（→ `cbe83f8a`）、`f7580e47`（→ `30113519`），票面对分支 tip 零 diff；清点在 tip 重生成 `de822820`（PS 186→187 / 178→181、SA 98→99 / 79→80、`cmd/` 61→63 / 85→88、CC 零差——与通道 3、27 记录、通道 4 三方预报逐格相符；重生成一次后 porcelain 空）。SHA 对照全表见三票「进 main 记录」。
- **验证（推送方全量一次，`de822820`）**：`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；14:34 占号 → 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**（14:34:48→14:37:04，136 s；比上批多两包 = `cmd/parcel-settlement-register` + `registrationjson`）；`-v` 探针 17 PASS / 0 SKIP（`TestSettlementRegisterVerticalOnRealPostgres` / `TestTheFundsHandlerRefusesANilDependencyAtConstruction` / `TestOverlongReferencesStillProduceAnEnvelopeIDWithinTheFrameworkLimit` / `TestARederivedVerificationWhoseEnvelopeIsRejectedRollsBackLoudlyInsteadOfCommittingHalf` / `TestAFormedDutyPaymentVerificationReachesTheSettlementInputThroughTheRouteTable` / `TestARejectedDutyVerificationHandoffIsNotRegisteredAsUndecided` / `TestAnOldShapeSnapshotWithoutTheElementsSegmentAnswersNotProvided` 等）→ 14:37 释号。
- **推送方对评审的处置**（写进三票「进 main 记录」）：27 Spec ① → 立 **sa-cc/32**（bug，draft，SA owner：SA 信封 ID 指纹形 + 不合法信封不折续办）；27 Standards ① ② + Spec ② 与 29 Standards ① ② → 合立 **sa-cc/33**（A 类尾巴，ready）；29 Spec ④ → 立 **sa-cc/34**（bug，draft，CC owner：其余五只 `*EventID`）；05 Standards ① → 立 **pp-seams/07**（chore，ready，PS owner：夹具 `SYN-`），05 判断项 ⑥ 记 pp-seams spec 残差；27 判断项 ⑤ 措辞本笔改；29 Spec ① 推送方核记录判断项 ③ ⑨ 满足评审条件。**31 Blocked by 解**（27 已进 main），可派。
- **簿记同笔**（本笔，纯 .md 自审）：三票 Status「已进 main」+ 评审代落 + 进 main 记录；票 31 Blocked by；sa-cc spec 27 / 29 行 + 新行 31 / 32 / 33 / 34；pp-seams spec 05 行 + 新行 07；新票四张；本节 → 共享 main `merge --ff-only` → `ls-remote` 核 `cbffc269` 未动 → `push <sha>:main`。分支 `mcp3-sacc27` / `mcp4-sacc29` / `mcp5-ppseams05` → `merged/`、远端删；三棵作者树由 3 / 4 / 5 比内容后拆。
- **前沿**：ready **3**——sa-cc/31（第二步端点 + 管理台，SA `adapters/http` + `cmd/parcel-api` 共享接线占号 + admin-web）、sa-cc/33（A 类尾巴）、pp-seams/07（A 类尾巴）；draft **2**——sa-cc/32（SA owner 三条要裁的）、sa-cc/34（CC owner）；在途 0；全通道空闲；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**：CI 计费；六件 untracked（`d_tasks.txt` / `docs/api/` / `handoff.md` / 三份 logo）；两张 draft 是否授权代裁；**会话中断成因**——本节窗口内六次重置（见五数），每一张票都至少换过一次手，活全靠「每笔推分支」与推送方按 diff 代写留住。
- **本节五数**（13:1x–14:4x，含补记段）：CI 未起跑（计费）· 集成时延——27 代码推齐 13:27 → 进 main 14:4x **约 80 分**（其中约 35 分是通道 1 会话消失的空档、约 20 分是代写与评审）；29 代码推齐 13:47 → 14:4x **约 60 分**（完成记录由新会话 14:28 补）；05 代码推齐 13:43 → 14:4x **约 65 分**（真库重跑 + 完成记录由新会话补）；**中位数约 65 分（3 票）** · 重放笔数 **18**（27 八笔 + 05 六笔 + 29 四笔，不含清点与本笔）· 重复开发 0 · **会话重置 6**（通道 6 13:1x、通道 2 13:2x、通道 3 13:3x、通道 1 13:4x–14:1x、通道 4 与通道 5 各一次——14:2x 点名两者都报「空闲 · 地盘无」）· 非作者评审缺席 0

### 14:5x–15:1x：用户「自决」→ 点名派五单 → 第七批 sa-cc/33 + pp-seams/07 + 32 取证进 main

- **三棵作者树拆完**（3 / 4 / 5 各报地盘对 `origin/main` 零 diff；4 / 5 的 `worktree remove` 退 255 Permission denied、按 workflow.md 本机环境四步清壳），`worktree list` 只剩主树。
- **用户「下一步做什么呢，请你自决吧」**。**点名**（14:55 广播、截止 14:58）：4 / 2 / 3 / 5 / 6 五应答全在截止前、全空闲。派五单（全基 `93840328`）：**task-cbea007d → 通道 3** sa-cc/31 实现（27 作者；SA 第二步端点 + `cmd/parcel-api` 共享接线占号 + admin-web 写签；唯一用 55432 的一单）；**task-c3f6cfd9 → 通道 4** sa-cc/33（29 作者，两处是它自己的注释）；**task-f70d20a8 → 通道 5** pp-seams/07（05 / 03 作者）；**task-22481ee4 → 通道 2** sa-cc/32 裁决前只读取证（它在 27 评审里量出的）；**task-a2f70396 → 通道 6** sa-cc/34 裁决前只读取证（它在 29 评审里量出的）。撞点写进单：31 不碰 33 的两处、34 取证只读 `external_result_handoff.go`。
- **三单回报**（15:0x，派出后 10–15 分）：通道 4 sa-cc/33 `mcp4-sacc33@60721f60`（单笔六件零行为；138 字节锚 `a0cb6fef`；判断项四条）；通道 5 pp-seams/07 `mcp5-ppseams07@0b3e5de9`（单笔六件 62 行 / 65 处字面，国家码无格式门故全 `SYN-`，票面漏列三处一并换；判断项六条；提醒 PowerShell 会剥 `"DE"` 引号、核国家码用 `-w`）；通道 2 sa-cc/32 取证 `mcp2-sacc32-evidence@d2af6ba1`（最要紧：`Store.Enqueue` 先 `Validate` 再 SQL、`handOffFact` 不分种类折续办，单事务 CLI 下「行已落信封未出」只由 Go 侧确定性错误到达，27「重跑补发」在它唯一能到的因下恒假；CC 消费门有 ID 之外的幂等键；`fingerprintEventID` 未导出且跨包引用被 `boundaries_test` 挡，共用位先例 `internal/platform/outboxintent`）。
- **评审门**：33 / 07 零行为 → 推送方自审（33：生产两文件滤 `//` 后空、测试 hunk 逐行；07：生产零 diff、每条加行含 `SYN-`、`-w` grep 真码零命中）；32 取证纯 .md → 自审。
- **重放**：`%TEMP%\idp-replay-wave7` @ `93840328`：`cherry-pick 60721f60` → `3e0f1230`；`0b3e5de9` → `e1840144`；`d2af6ba1` → `c84e81bd`（全部零冲突、各自地盘对作者 tip 零 diff）。清点在 tip 重生成**零差**。**验证（推送方全量一次，`c84e81bd`）**：`gofmt -l` 空、build / vet 0；15:06 占号（广播里点名通道 3 等释号）→ 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**（136 s）；`-v` 探针 PS content / elements + settlement-register 52 PASS / 0 SKIP → 15:09 释号。簿记同笔（票 33 / 07 Status「已进 main」+ 自审 + 进 main 记录、sa-cc spec 32 / 33 行、pp-seams spec 07 行、本节）→ 共享 main `ff` → `ls-remote` 核 `93840328` → push。分支 `mcp4-sacc33` / `mcp5-ppseams07` / `mcp2-sacc32-evidence` → `merged/`、远端删；三棵树由 4 / 5 / 2 比内容后拆。
- **前沿**：在途 sa-cc/31（3，目标 90 分）、sa-cc/34 取证（6）；**待通道 1 代裁**：sa-cc/32（取证已落，SA owner 口径）、sa-cc/34（等取证）；ready 0；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**：CI 计费；六件 untracked。
- **本节五数**（14:5x–15:1x）：CI 未起跑（计费）· 集成时延——33 完工报 15:0x → 进 main 15:1x **约 10 分**；07 完工报 15:0x → 15:1x **约 8 分**；中位数约 9 分（2 票）· 重放笔数 **3**（33 一笔 + 07 一笔 + 32 取证一笔）· 重复开发 0 · 会话重置 0 · 非作者评审缺席 0（33 / 07 零行为自审如实计）

### 15:2x：代裁 sa-cc/32（用户「自决」授权；SA owner 口径）→ ready，Blocked by 31

- **依据**：通道 2 取证 (1)–(6)（钉 `93840328`）。最要紧的一条：单事务入口下「行已落、信封未出、续办引用非空」只由 Go 侧确定性错误到达（`Store.Enqueue` 先 `Validate` 再 SQL；依赖故障把事务打进中止态 → CLI 未决 3 且版本行不落），所以 27 的「重跑补发」在它今天唯一能到的因下恒假。
- **裁**：ID 指纹化（口名前缀 `funds-fact` + `sha256(租户 / 事实 / 版本)`），helper 按 `outboxintent` 头注的 rule-of-three 提炼为导出 `FingerprintEventID`、CC 三只改引删私有（零行为，34 五只同引）；`Subject` / `PartitionKey` 不哈希（照 29 判断项 ③）；`handOffFact` 只对 `ErrInvalidEnvelope` 分格——包 `ports.ErrFundsFactHandoffRejected` 返错整笔回滚、CLI 用法格 1 点名原因（不归未决 3：什么都没登记、要改的是输入，重跑到预算耗尽正是 29 的反例）、端点 4xx；其余错误照旧折续办（对依赖故障那句是真的，事务入口下到不了→判断项写实，不删 `FundsResult` 那一格）；CC 消费门幂等靠 0021 主键 + `已存在`，真库钉。**为什么 Blocked by 31**：CLI 与端点两口对同一哨兵的映射一次做齐，不让端点先以 500 把确定性拒折成「不知道」。完成判据七条写实。
- 纯 .md，共享 main 直接一笔、`ls-remote` 核 `d02fd926` 未动后 push（`2f698d2b`）。**前沿**：在途 31（3）、34 取证（6）；ready 1——sa-cc/32（Blocked by 31）；待代裁 34（等取证）。

### 15:3x：34 取证进 main → 代裁 sa-cc/34（CC owner 口径）→ ready 即派

- **取证 ← 通道 6**（`mcp6-sacc34-evidence@aec424a6`，纯 .md，推送方直接 `cherry-pick` 到共享 main → `760332c7`）。最要紧：五只里**只有 `externalResultEventID` 有生产装配**（`/customs/external-results`）且其事件类型零消费者；五只都不分格、全折续办、都不在重派路；两只把 ID 直接当分区键；有消费者的两只 → VE 有 `FactKey` + 摘要幂等；grep 漏两只（follow-up 四维串接第六只、restriction 单引用无上界）。
- **裁**：范围扩到**七只**一次同改（单引用与零装配的也改——「留一只等于留一张脸」）；`FingerprintEventID` 提炼到 `outboxintent` 导出**由本票做**、29 三只改引删私有，32（Blocked by 31、晚于本票）只引——32 票面已改注；ID 与分区键解耦：两只 `PartitionKey: eventID` 的改取原可读串接一字不变（分区是顺序语义，`partition_subject_registry_test` 登的就是它），其余五只分区键本来另算；确定性拒分格**只做外部结果口**那一路（照 32 裁决 2，`ErrHandoffEnvelopeRejected` 返错回滚 → 4xx），其余六只入口未接不动、残差记 spec；VE 幂等真库钉两只有消费者的。完成判据七条写实。
- 纯 .md 两笔（取证 cherry-pick + 代裁）共享 main 直接落，`ls-remote` 核 `2f698d2b` 未动后 push（`760332c7` / `0b3f0831`）。三棵尾巴树（2 / 4 / 5）已拆。**派单**：task-78fa0373 → 通道 6 sa-cc/34 实现（取证人即作者，未另点名、按 15:0x 自报空闲）。**前沿**：在途 31（3）、34（6）；ready 1——sa-cc/32（Blocked by 31）。

### 15:2x–15:4x：31 交活 → 派评审 → 立 35（main 上 admin-web `tsc` 既有红）

- **31 交活**（通道 3 15:2x）：`mcp3-sacc31` 六笔，代码 tip `08e82a88`、票面 tip `17067468`；带 DSN `cmd/parcel-api` + SA http / postgres + architecture **474 PASS / 0 / 0**；清点预报 SA 生产 99→100 / 测试 80→81、cmd 63→64 / 88→89、端点 122→124；判断项 8 条——续办引用格折 **500 `HANDOFF_NOT_SENT`** 不进 2xx、`FUNDS_UNDECIDED` → 500、`ADOPTED` 201 其余 200；做法 3 实际动四件（占号一笔 `9c065fb0`）；**(7b) `tsc --noEmit` 仓级在基线已红**。**派评审 task-d159b7a2 → 通道 2**（评过 27、取证过 32；按 15:1x 自报空闲，未另点名），要它顺带在基线检出上复跑 `tsc` 证 (7b) 既有。
- **(7b) 推送方复现**：`0b3f0831` 上 `apps/admin-web` `tsc --noEmit` 退 2，唯一报错 `register-rows.test.ts` `verification()` 缺 `fundsVersion`；`git log -S fundsVersion` 指 19 的代码笔 `e0201ca6`（12:22 进 main）；`ci.yml` 无 admin-web 类型检查步 → 三个多小时无人看见。**立 sa-cc/35**（ready，A 类：夹具加 `fundsVersion`，不改类型）；「CI 加不加 `tsc` 步」归用户（计费）。
- **前沿**：在途 31 评审（2）、34（6）；ready 2——sa-cc/35（即派）、sa-cc/32（Blocked by 31）。

## 2026-09-15 16:0x 通道 1 新会话（今天通道 1 第五次重置；上一任 15:27 推完 `15433da2`、15:30 建第八批重放树后消失，15:4x–15:5x 由原通道 2 会话代行推送方到释号后亦消失；用户先用自带 `ask_question` 验通道，再问「这些全部完成了吗，可以 replay 了吗」→「replay」）

### 15:2x–15:5x 补记（两任推送方都未及写入 tasks.md；按任务台账 result、分支 git 时间、队列里两条广播补，时刻取 git / 台账）

- **35 两次派**：15:2x 派通道 4（task-b6892242）→ 用户报 crash、核无分支无树零开工 → 15:30 改派通道 5（task-1d4e6bef）→ 15:33 单笔 `2a1f4138` 推 origin、`report_task done`（缺席的必填字段是两个：`fundsVersion` + `procedure`，tsc 一次只报第一个，红自 22 `2e411f63` 起；判断项三条）；作者随后在 push 完成、junction 收回之后 crash（用户 16:0x 截图报），树 `status` 空、无损失。
- **31 评审到**：15:29 通道 2 交 task-d159b7a2 done——**两轴 0 阻断、Standards 0 非阻断、Spec 1 非阻断**（admin-web 词条归 32）；(7b) 复跑证既有。送到的是随后 crash 的通道 1 会话，票面未落。
- **第八批重放树**：上一任 15:30:55 在 `%TEMP%\idp-replay-wave8`（detached `15433da2`）重放 31 六笔到 `e9c2f7f8` 并起全量后消失；原通道 2 会话 15:4x 广播「接手 + 占号」（只读 git 与票面、无上一任内存），15:37:53 叠 35 → `0943f7da`、清点重生成零差、带 DSN 全量 **115 ok / 0 FAIL / 16 无测试 / 0 cached**（15:38:41→15:40:36）、探针 PASS、tip 上 `tsc` 退 0，15:4x 广播「释号 · 转簿记 → ff → push」——**之后无任何动作**：簿记未写、main 未 ff、未 push；该会话 16:04 前结束（通道 2 16:04 起是新会话）。与 09-10 夜、今晨 14:1x 节同形：**重放做好之后、簿记之前断**——今天第三例。
- **34**：15:23 派通道 6（task-78fa0373，始终 pending 未 report）；15:27 `ed1da549`（裁决 2：`FingerprintEventID` 提炼到 `outboxintent`）、15:38 `f07d65dd`（裁决 1：七只信封 ID 定长形，标「1 / 3」）两笔推 origin；树内三文件未提交（`customscompliance/application/receive_external_result.go` + 同名 `_test` + `adapters/http/receive_external_result_test.go`，mtime 15:57:23，裁决 3 在途）；会话在 15:57 之后、16:0x 之前结束。

### 16:0x–16:1x：量状态 → 转投 → 身份澄清 → 答「31 / 35 已重放、34 未完、evidence 是空壳」

- **接手取证**（只按 git 与台账，不读自报）：`ls-remote` main = `15433da2`；`branch-state.ps1`：共享树 76 处未提交（CRLF 幻影为主，六件 untracked 归用户）；`mcp3-sacc31@17067468` / `mcp5-sacc35@2a1f4138` 干净、已推、内容分别与 `e9c2f7f8` / `0943f7da` 零 diff（`git patch-id --stable` 七笔逐笔同）；`mcp6-sacc34@f07d65dd` 两笔已推 + 三文件未提交，merge-base `0b3f0831`；`idp-replay-wave8@0943f7da` 干净；`%TEMP%\idp-parcel-mcp6-sacc34-evidence` 是空目录（0 项，14:59 建、15:23 末改，内容早在 `760332c7` 进 main）。
- **通道 5 / 6 状态报告落到通道 1**（两者都是新会话、无内存；6 量到 34 半成品、未接指令不动树），本会话先误判「接手推送方在通道 2」转投两条 → 通道 2 澄清它也是 16:04 起的新会话、不据此行动。结论：**推送方身份无人持有**，等用户裁。用户 16:1x 问四棵树是否全完成、可否 replay → 答见上节标题；用户 16:1x「replay」。

### 16:1x–16:2x：第八批 sa-cc/31 + sa-cc/35 进 main

- **验证（本会话重跑，不沿用前任的数；钉 `0943f7da`）**：`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；16:19 占号广播 → 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**，退出码 0（16:19:34→16:21:28，114 s）；`-v` 探针 `TestTheWiredExternalFundsFactRegistrationsRecordAgainstARealDatabase` PASS（0.42 s）非 SKIP、`TestAnAdoptedFactWhoseHandoffDidNotLeaveIsNotReportedAsAdopted` / `TestIsolatedReadIntakeCannotServeExternalFundsFactRegistration` PASS；admin-web（`node_modules` junction 指主树）`tsc --noEmit` 在 tip 退 0，**阳性对照**：共享树基线 `15433da2` 同命令退 2、唯一报错 `register-rows.test.ts(86,3)` TS2322 `fundsVersion` → 35 的红确在基线、确在 tip 消；16:2x 释号广播。
- **评审门**：31 ← 通道 2 无阻断（15:29，代落票面 Comments，措辞照台账 result）；35 零行为推送方自审（`git show --stat 2a1f4138` 两件，测试 +2 −0，`api.ts` / 页面 / CI 零 diff）。
- **簿记本笔**（纯 .md 自审，在 `0943f7da` 之上）：票 31 Status「已进 main」+ 评审代落 + 进 main 记录；票 35 Status + 进 main 记录；票 32 Blocked by → 「无——31 已进 main」+ 评审 Spec ① 归它；sa-cc spec 31 / 35 行 → 进 main、32 行解阻；本节。→ `ls-remote` 核 `15433da2` 未动 → `push <本笔>:main` → 共享 main `merge --ff-only`（共享树 76 处未提交与本批文件零重叠才 ff 得动，否则只推不 ff 并在此记明）。**远端 main = 本笔 tip**（SHA 在推后的广播里）。
- **SHA 对照**：31 `4c466db4→25d2855a` / `a698f961→25ba4e24` / `9c065fb0→51279e51` / `8ae3a9a4→a0ddcc80` / `08e82a88→a12e9dc2` / `17067468→e9c2f7f8`；35 `2a1f4138→0943f7da`；簿记本笔在其上。分支 `mcp3-sacc31` / `mcp5-sacc35` → `merged/`、远端删；作者树 `idp-parcel-mcp3-sacc31` / `idp-parcel-mcp5-sacc35` 归 3 / 5 比内容后拆（两树 `status` 均空）；`idp-replay-wave8` 推完拆（先删 junction）；`idp-parcel-mcp6-sacc34-evidence` 空壳归用户点头删。
- **前沿**：在途 sa-cc/34（通道 6 树 `f07d65dd` + 三文件未提交、task-78fa0373 pending、原会话已无）——**派单归推送方，两种写法各是答案**：(a) 派通道 6 新会话「从 `mcp6-sacc34@f07d65dd` 接着做，第 1 步先读三文件未提交 diff 报 ≤10 行、判用不用」；(b) 三文件先原样封存 `chore(salvage)` 再另起。未派，等用户一句或下一轮自决；ready 1——sa-cc/32（Blocked by 解）；draft 0；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**（累计）：CI 计费与要不要加 admin-web `tsc` 步；六件 untracked；`mcp6-sacc34-evidence` 空壳；**会话中断成因——今天第三例「重放做好、簿记之前断」，且这次是两任连断**：15:30 上一任建树起全量后断，15:4x 接手会话验完释号后断。两任都把最贵的一步（带 DSN 全量）跑完了，丢的只是簿记与 push——「验完先 push 再簿记」也许该成为规矩：`push <验过的 SHA>:main` 只要一秒，簿记笔可以后补；今天 0943f7da 从 15:40 验绿到 16:2x 才推出，中间 45 分钟 main 没动，只因为没人活到写簿记那一步。
- **本节五数**（15:2x–16:2x，含补记段）：CI 未起跑（计费）· 集成时延——31 完工报 15:22 → 进 main 16:2x **约 60 分**（其中 15:40→16:1x 约 35 分是两任推送方消失的空档）；35 完工 15:33 → 16:2x **约 50 分**；中位数约 55 分（2 票）· 重放笔数 **7**（31 六笔 + 35 一笔，均由前两任 pick，本会话零新 pick）· 重复开发 0 · **会话重置 5**（通道 1 两次：15:3x 上一任 + 15:4x–15:5x 接手会话；通道 2 一次 16:04；通道 5 一次 15:3x push 后；通道 6 一次 15:57 后）· 非作者评审缺席 0（35 零行为自审如实计）

### 16:2x–16:3x 同一会话：已推 `95863889`；作者树拆完；用户「自决」→ 34 封存 + 接管单派通道 6

- **已推**：16:28 `push 95863889:main` 成，**远端 main = `95863889`**，共享树 ff 同 SHA（76 处未提交与本批 19 件零重叠）；`mcp3-sacc31` / `mcp5-sacc35` 内容逐文件对 main 零差（31 只差票面簿记 hunk）后 → `merged/`、远端删；`idp-replay-wave8` 拆（junction 先删、退 0、目录无）。通道 5 16:3x、通道 3 16:3x 各报作者树比内容后拆（两份取证都核 blob / 三样各查、未加 `--force`），`worktree list` 只剩主树 + `mcp6-sacc34`。
- **用户「你自决」→ 34 走 (a)+(b) 合一**：先封存再接着做。推送方在通道 6 树上把三文件原样封存 **`dd49cc8c`**（`chore(salvage)`，一字不改；量到 `go build ./...` 0、两包 vet 0、`application` 包 ok、http 包 1 FAIL——`TestARejectedHandoffEnvelopeIsAFourHundredNotARetry` 500 want 422，即裁决 4 端点映射未写；提交信写「mtime 15:57:23、该会话 15:57–16:0x 间结束」不写「已死」），推 origin；第一版提交信把时刻估成 16:5x，随即 `--amend` 改为 git 时间 16:34 并 `--force-with-lease`（`6d52187c→dd49cc8c`，无人基于旧 SHA）。**接管单 task-62ea8999 → 通道 6**（16:38；心跳 16:36 在等、16:0x 自报空闲且是取证人，未另点名——2 / 3 / 5 刚各报空闲留评审）：第 1 步读封存 diff 判用 / 不用回 ≤10 行；剩余裁决 4 端点 4xx + 判据 (5) 库面回滚格（`cmd/parcel-api` 带 DSN）、裁决 5 / 判据 (4) VE 幂等真库两格、判据 (3) 自核、(6) 验证带 DSN 占号、(7) 清点干净检出、spec 残差句、完成记录 + resolved；封存笔不进 main、完工前重切；不碰 `endpoints.go` / `main.go`；基线零重叠不必 rebase。旧单 task-78fa0373 留 pending 作台账痕迹。
- **前沿**：在途 sa-cc/34（6，估 60 分）；ready 1——sa-cc/32（Blocked by 解，**按 34 裁决 2 等 34 进 main 后派**）；2 / 3 / 5 空闲留评审；draft 0；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**（累计不变）+ `idp-parcel-mcp6-sacc34-evidence` 空壳可删。

## 2026-09-15 16:5x 通道 1 新会话（今天通道 1 第六次重置；上一任 16:39 推完 `8928cb93` 后消失。用户先用自带 `ask_question` 验通道，再经队列「`idp-parcel-mcp6-sacc34-evidence` / `idp-parcel-mcp6-sacc34` 这两个可以 replay 了」）

### 16:5x–17:1x：量状态 → evidence 空壳删 → 34 整支不可重放 → 试裁决 1–3 两笔：带 DSN 1 FAIL（`cmd/parcel-dispatch` 两用例旧串接形重算 ID）→ 派评审、通知作者、等用户

- **接手取证**（只按 git 与台账，不读自报）：`ls-remote` main = `8928cb93` = 本地 = origin/main；共享树 76 处未提交（CRLF 幻影 + 六件 untracked 归用户，未动）；`worktree list` = 主树 + `idp-owner-queue`（detached）+ `mcp6-sacc34`；`%TEMP%\idp-parcel-mcp6-sacc34-evidence` 空目录 0 项（内容 15:3x 已在 main `760332c7`、指针 `merged/mcp6-sacc34-evidence@aec424a6` 留）→ **已删**；`mcp6-sacc34@dd49cc8c` = origin、树 `status` 0 行、`main..` 三笔（`ed1da549` 裁决 2 / `f07d65dd` 裁决 1 / 3 / `dd49cc8c` 封存），远端自 16:36 无新 ref；分支改动文件与 main 自 merge-base `0b3f0831` 起的改动**零重叠**。tip 上复现：`go build ./...` 0、`application` ok、**http 包 1 FAIL** `TestARejectedHandoffEnvelopeIsAFourHundredNotARetry` 500 want 422。台账 task-62ea8999 仍 **pending**、通道 6 `lastActivityAt` 16:38:52（= 派单时刻，之后零动作）、我队列无其第 1 步报告 → 17:0x `send_to_session(6)` 问一行状态（在做 / 未开工 / 新会话），未等到。**结论一：整支不能重放**（封存笔 + 裁决 4 / 5 未做 + 无评审）。
- **试裁决 1–3 两笔单独进 main**（不依赖用户答复的那半先做）：`%TEMP%\idp-replay-sacc34` detached `8928cb93`，`cherry-pick ed1da549 f07d65dd` 零冲突 → `67bfa0c2` / `0d724483`；`gofmt -l` 空、build / vet 0；不带 DSN CC 九包 + outboxintent + architecture + `cmd/parcel-api` + `cmd/parcel-dispatch` + VE `adapters/customscompliance` 全 ok。17:0x 占号广播 → 带 DSN `go test -p 1 -count=1 ./...` **114 ok / 1 FAIL / 16 无测试 / 0 cached**（17:10:18→17:12:31，133 s）→ 17:1x 释号广播（带结果）。FAIL = `cmd/parcel-dispatch` 两用例 `TestAnEstablishedCustomsCaseDerivesAProjectionPerParcelAndPublishes` / `TestAFormedDeclarationSubmissionDerivesAProjectionPerMemberAndPublishes`：`recordEstablishedCustomsCase` / `recordFormedDeclarationSubmission` 尾句仍按旧串接形拼信封 ID 交 `countInbox`，换指纹形后行数 0 want 1（投影已生成、published==1，断言形没跟上）——**票 34 判据 (2) 漏网两处**，票面地盘只写了 CC `adapters/postgres`，这两处不带 DSN 时 skip、作者到判据 (6) 才会撞见。归作者（改调 `outboxintent.FingerprintEventID` 重算或从 outbox 表读回 `event_id`），推送方不代改作者代码。**结论二：裁决 1–3 两笔修好这两处之前也进不了 main。** 全文代落票 34 Comments。
- **派评审 task-19dcdf04 → 通道 3**（17:0x；按 16:2x 节「2 / 3 / 5 空闲留评审」与台账 idle 16:47，未另点名，单尾留「已在做别的回一句我改派」）：钉 `f07d65dd`、基线 `0b3f0831`、只评两笔、两轴、20 分、不占 55432；17:1x 追一句「两处 `cmd/parcel-dispatch` 漏网推送方已量到，Spec 轴可直接引为阻断；顺手 grep `cmd/` 下还有没有其它带 DSN 才跑的旧串接形重算」。作者通道 6 队列已直送两处漏网 + 「修好前两笔也进不了 main」。
- **等用户裁**：(a) 等 / 改派 34 接管（通道 6 自 16:38 零动作、任务估 60 分到 17:38；若到时仍无应答按「截至 HH:MM 未应答」改派 2 或 5，接管单原文可复用 + 加这两处）；(b) 要不要把这两处 `cmd/parcel-dispatch` 断言形改法写进票面「做法」(5)。替身树 `idp-replay-sacc34` 留着等修笔叠上重验；`idp-review-sacc34` 归通道 3 评完自拆。**前沿**：在途 sa-cc/34（6，无应答）+ 其评审（3）；ready 1——sa-cc/32（等 34 裁决 2 进 main）；2 / 4 / 5 空闲；draft 0；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**（累计）：CI 计费与 admin-web `tsc` 步；六件 untracked；34 接管改派与否。
- **本节五数**（16:5x–17:1x）：CI 未起跑（计费）· 集成时延 —（本节零票进 main）· 重放笔数 **0**（替身树两笔 pick 未进 main 不计）· 重复开发 0 · **会话重置 1**（通道 1 本会话新起；通道 6 疑似——只记「16:38 起零动作、17:0x 问未答」，不记已死）· 非作者评审缺席 0（34 评审已派）

### 17:1x–17:2x 同一会话：通道 6 应答（新会话、未开工）→ 评审到（两轴 0 阻断）→ 开工令 + 范围加三样

- **通道 6 应答**（17:1x，两条）：新会话无记忆、未开工、按 17:0x「不要现在开工」持有；已从 tasks.md 16:2x–16:3x 节补回接管单摘要，问 dispatch 两处断言修正归谁。**会话重置 +1**（通道 6，确认为新会话）。
- **评审到 · 34 裁决 1–3 ← 通道 3**（17:1x，task-19dcdf04 done，钉 `f07d65dd`）：**两轴 0 阻断**；Standards 3 非阻断（`externalResultPartitionKey` 头注半句领先于代码、`manifestEventIDPort` 头注「与分区主体登记口名段同词」不成立、跨包计数 / `beat` 三常量口味级）；Spec 2 非阻断（判据 7(1) 超长入队格只落四只缺 declaration-submission / case-closure / follow-up；restriction 维度不含租户为既有口径）；「两笔单独进 main 无代码级残差」。全文代落票 34 Comments。评审树已自拆。
- **开工令 → 通道 6**（17:2x，task-62ea8999 生效）：范围 = 接管单 + 三样归本票——(1) `cmd/parcel-dispatch` 两处断言改 `FingerprintEventID` 重算 / 读 outbox，票面地盘补两件；(2) 评审 Standards ①②；(3) 评审 Spec ① 推送方裁**补齐三只超长入队格**、Spec ② 归 spec 残差句、Standards ③ 作者自定。顺序：第 1 步封存判用 / 不用 ≤10 行 → **第一笔先做 (1)+(2) 推完 report working 带 SHA**（推送方据此决定要不要先放裁决 1–3 + 该笔进 main 解 32 的阻；6 若裁决 4 / 5 走得快就不拆）→ 裁决 4 / 5 + 判据 (3)–(7)。估 60–75 分、18:3x 报进度。基线不必 rebase（main `91df650c` 与 `8928cb93` 只差 .md）。
- **前沿**：在途 sa-cc/34（6，开工 17:2x）；ready 1——sa-cc/32（等 34 裁决 2 进 main）；2 / 3 / 4 / 5 空闲；draft 0；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。替身树 `%TEMP%\idp-replay-sacc34@0d724483` 留着等 6 第一笔叠上重验。**归用户**（累计）：CI 计费与 admin-web `tsc` 步；六件 untracked；上一节「改派与否」已不必答（6 活着）。
- **17:2x–17:3x 追记**：通道 3 评审追记到（纯 grep：两处 dispatch 代入为 Spec 阻断、`cmd/` 其余重算七只旧串接形的用例零，五条依据）→ 代落票 34。通道 6 `report_task working` + 第 1 步六条（dd49cc8c 判用；量到裁决 4 首句 `external_result_handoff.go` 分格在分支上也没做、作者补；http 押 422 不采 → 400 + `HANDOFF_ENVELOPE_REJECTED`，推送方核 ADR-0022 否决选项两条 + `adapters/http` 零处 422 / 409 后接受；完工前 `reset --soft f07d65dd` 重分笔）→ 代落票 34。**推送方撤回「拆两次进 main」**：一次进 main、顺序作者定——早放裁决 1–3 只换 32 提前约半小时开工，却多一轮重放 + 评审 + 全量，今天推送方是瓶颈，不值。替身树 `idp-replay-sacc34` 因此改为完工后重建、现可拆（推前拆）。

## 2026-09-15 18:1x 通道 1 新会话（今天通道 1 第七次重置；上一任 17:3x 推完 `2c3832ce` 后消失。用户先用自带 `ask_question` 验通道，再经队列问「`idp-parcel-mcp6-sacc34` 全部完成了吗」附截图 → 答「没有」→ 用户「继续彻底完成吧，你派工」）

### 18:2x–18:3x：量 34 现场 → 诊断两格红的成因 → 点名 → 派作者单 + 预派评审单

- **量**（只按 git 与真库，不读自报）：`ls-remote` main = `2c3832ce` = 本地 = origin/main；`worktree list` = 主树 + `idp-owner-queue`（detached）+ `mcp6-sacc34`；`branch-state.ps1 -Path internal/customscompliance`：在途只 `mcp6-sacc34@5646c6ad`（= origin 同 SHA，领先 main 4 笔、落后 13 笔——13 笔全 .md / SA 侧，与分支自 merge-base `0b3f0831` 起的改动**代码零重叠**，只票面 34 的 .md 两边都动过）；树内**未提交 2 件**——`cmd/parcel-dispatch/customs_case_projection_test.go`(+71) / `declaration_submission_projection_test.go`(+32) = 判据 (4) 两格 `Test…UnderTwoEnvelopeIDsDerivesOnce`，mtime 17:45，与用户截图末帧一致。四笔已提交 = 裁决 2（`ed1da549`）/ 裁决 1·3（`f07d65dd`）/ 裁决 4（`74c44ac9`）/ 判据 (1)(2) 补齐 + 评审 Standards ② Spec ①（`5646c6ad`）——作者按 17:3x 所报 `reset --soft f07d65dd` 重分了笔，封存笔 `dd49cc8c` 已不在支上。
- **两格红的成因**（推送方在同树带 DSN 复现 `published = 3, want 1`，再用 `go test -overlay` 挂一只临时诊断用例 dump outbox——未写入仓、跑完即删）：第 1 拍定稿指纹形那封 → VE 派生两条成员投影、各入队一封 `visibility-exception.tracking-projection.derived`（分区 `SYN-TENANT-01/SYN-PARCEL-01` / `-02`）；第 2 拍定稿 3 封 = 重发的串接形那封 + 那两封派生信封；第 3 拍 0；全程 outbox 4 行、第二封**未再派生**——裁决 5 要证的 `FactExistingResult` 成立，错在断言数拍内 `published`。修法交作者：断两封 CC 信封各 `PUBLISHED` + `tracking-projection.derived` 仍两封 + inbox / 投影照旧。另量到该文件包头注「不登记 tracking-projection.derived」与运行时相悖，交作者核。`gofmt -l` 空、三包 vet 0。
- **点名**（18:28 广播 2/3/4/5/6、截止 18:31）：4 / 3 / 5 / 2 / 6 五应答全在截止前、全报「空闲 · 地盘无」；6 补报「握着 `mcp6-sacc34` 树 @ `5646c6ad`、本会话新开」——即作者通道 6 又重置一次（会话重置 +1），17:2x 开工令那个会话在 17:45 之后结束，两件未提交是它留下的活现场。
- **派单**：**task-e119cc09 → 通道 6**（作者，握树者；单里写全诊断与修法、判据 (3)(5)(6)(7) 自核清单、票面单独成笔只含 .md、spec / tasks.md 不改、每笔推分支、完工 `report_task done` + 直送通道 3「tip = …，可评」；估 30–45 分）；**task-9e4bb550 → 通道 3**（合入前独立评审，全支两轴，基线 `0b3f0831`，**开工条件 = 收到通道 6 那一行**，之前回 check_messages 等；它 17:1x 评过裁决 1–3 故续派；只读不占 55432；20 分）。2 / 4 / 5 空闲留用。
- **推送方接下来**（作者 done + 评审无阻断后）：`%TEMP%\idp-replay-sacc34` 重建于 main tip → cherry-pick 代码笔（票面 .md 笔预期与 main 上 `a79145c5` / `2c3832ce` 的评审代落冲突，手工并）→ 清点在 tip 重生成（预报 `internal/platform` +1/+1、CC 零变）→ 占号带 DSN 全量一次 → **先 `push <验过 SHA>:main` 再簿记**（今天三例「验完簿记前断」的教训）→ 票 34 Status「已进 main」+ 评审代落 + 进 main 记录 + spec 34 行（含裁决 4 残差句）+ 本节 → 分支 `merged/`、远端删 → 6 比内容拆树。之后 sa-cc/32 解阻（等用户一句或下一轮自决再派）。
- **前沿**：在途 sa-cc/34（6 收口中）+ 其评审（3 预派待触发）；ready 1——sa-cc/32（Blocked by 34 裁决 2 进 main）；2 / 4 / 5 空闲；draft 0；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**（累计）：CI 计费与 admin-web `tsc` 步；六件 untracked（`d_tasks.txt` / `docs/api/` / `handoff.md` / 三份 logo）。

### 18:4x–19:5x：作者 done → 房务两笔叠上 → 票面改基 main 版 → 通道 1 两任 crash → 用户令通道 6 代推送方 → 评审 ← 通道 2 → `6683f989` 进 main

（本节由通道 6 写——它是 34 的作者，也是本批的代推送方；两个角色的取数都在下面各自标明。）

- **作者侧**（通道 6，task-e119cc09，18:3x–18:5x）：判据 (4) 两格改断 `event_id` 查 `status = PUBLISHED` × 2 + `countOutboxOfType(tracking-projection.derived) == 2` + inbox / 投影照旧、拍内 `published` 弱断 `>= 1`（`e2941cde`，只两份 `_test.go`，生产零改）；两文件头注「不登记 tracking-projection.derived」经 `git log -S` 核——写于 `b98568b2`（08-19 21:26）早于 `49a2ab03`（22:04）登记，`b4b6a642` 照抄——改准；判据 (3) 三条零命中 / 十处 / 零改、(5) 四格用例名、(6) 占号带 DSN 包组 `-count=1 -p 1` 全 ok（`cc/adapters/postgres` 8.9s、`cmd/parcel-dispatch` 5.3s、`cmd/parcel-api` 2.5s、`architecture` 3.6s）两只真库格 `-v` PASS、(7) 预报 platform +1/+1 CC 零变；票面先基分支版成笔 `f79add49`，按推送方 18:4x 补充改基 main `0f433022` 版重写 `2c0aff8d`（四条 Comments 保留，对 main 54 增 / 1 删）。
- **推送方侧 · 通道 1 两任**（按 git 与队列台账取数）：18:44 `%TEMP%\idp-replay-sacc34` 重建于 `0f433022`，五笔代码 cherry-pick 零冲突；18:46 清点重生成 `d5b74eb7`（platform 17→18 / 16→17，合计 991→992 / 943→944）；18:46:44→18:48:42 带 DSN 全量 **115 ok / 0 FAIL**（那一跑验的是 `d5b74eb7`）；18:50 房务 ← 通道 4 `mcp4-housekeeping@da112a2f` 两笔 .md 叠上（`37f8050a→363e3bc3` pp-seams/04 立票、`da112a2f→89082de0` owner-review-queue 重生成）；18:52 票面 `f79add49` 与 main 四条 Comments 手工并成 `72a0dfa4`，18:56 收到 `2c0aff8d` 后换成 `6683f989`（内容与 `2c0aff8d` 逐字节同）；**随即消失**（心跳止于 18:5x）。19:05 通道 1 新会话（原通道 4 窗口接手）读到「可以 replay 了」，做到 patch-id 对照那一步**再断**（心跳止于 19:11:59）。**今天第四例「重放做好、簿记之前断」，且又是两任连断**——15:2x 节写的「验完先 push 再簿记」这次仍没来得及：`d5b74eb7` 18:48 验绿，到 19:5x 才由第三个人推出去，中间一小时 main 没动。
- **推送方侧 · 通道 6 接手**（用户 19:1x「mcp-1 crash 了，请你继续他的工作」附截图）：**只按 git 与真库，不读自报**——`worktree list` 主树 `0f433022` + `mcp4-housekeeping@da112a2f` + `mcp6-sacc34@2c0aff8d` + `idp-replay-sacc34`（detached `6683f989`）；`fetch` 核 origin/main = `0f433022` 未动；patch-id：七笔对应来源（`ed1da549` / `f07d65dd` / `74c44ac9` / `5646c6ad` / `e2941cde` / `37f8050a` / `da112a2f`），`d5b74eb7` 与 `6683f989` 为推送方手工笔——后者按内容核：票 34 与 `2c0aff8d` 零 diff、分支自 `0b3f0831` 改过的每一文件与 tip 零 diff；`0f433022→6683f989` 恰 31 件 = 34 的 28 件 + 清点 + 房务 2；tip 上清点**重生成零 diff**、`go build` / `go vet` 0、`gofmt -l`（七个含 Go 的顶层目录）空。**验证（推送方全量一次，`6683f989`，验的就是要推的那个 SHA）**：19:17 占号 → 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**，退出码 0（19:17:00→19:18:55，115 s）→ 19:19 释号。
- **评审门**：通道 3 预派单 task-9e4bb550 未回——心跳 18:49:13 起停、我 18:50 那行「可评」它没读到；**点名 2 / 5**（19:14，截止 19:18）：2 答「空闲 · 地盘无 · 余量充足」，5 未答（心跳在等、不算认领）→ **task-342e8acd → 通道 2**（只读隔离树 `idp-review-sacc34-2`，重评裁决 4 / 5 三笔 + 票面 + 清点、复核 17:1x 三条非阻断处置、过眼房务两笔；20 分）。19:5x 回：**两轴 0 阻断**；Standards 2 非阻断（`ports.ExternalResultHandoff` 口约头注缺 29 那句「确定性拒收交回 `ErrHandoffEnvelopeRejected`」、`Handle` 头注尾句「投递失败不翻结果」未随裁决 4 分格）+ 1 口味（`outboxStatus` 落案件用例文件被申报用例共用）；Spec 1 非阻断（判据 (4) 格落同包投影用例文件而非裁决 6 (6) 字面的 `assemble_test.go`，合精神只记）；无发现逐条含裁决 4 六处、ADR-0022 400、两用例串接形与 `760332c7` 逐字同序、头注核 `veinbox.TrackingProjectionDerivedEventType: veCustomerViewRouted` 属实、判据 (1)(2)(3)(7)、17:1x 三条处置属实、房务只 .md。**处置**：两句注释记票 34「残差」归尾巴票或随 32 顺手（不为两句注释再走一轮重放）；口味与 Spec ① 只记。评审树已自拆。
- **push**：19:5x `fetch` 再核 `0f433022` 未动 → `push 6683f989:main` 成，**远端 main = `6683f989`**；共享树 `merge --ff-only` 同 SHA（76 处未提交与批内 31 件零重叠）；广播。**簿记本笔**（纯 .md 自审，在 `6683f989` 之上）：票 34 Status「进 main」+ 评审代落 + 进 main 记录 + 残差两句；sa-cc spec 34 行（draft→resolved + 进 main + 残差）与 32 行（34 进 main、只引）；本节 → `ls-remote` 核 `6683f989` 未动 → `push <本笔>:main` → 共享树 ff。
- **SHA 对照**：34 `ed1da549→6f4e4f85` / `f07d65dd→cd8df62c` / `74c44ac9→bc09d668` / `5646c6ad→033b0832` / `e2941cde→650b6a66` / 票面 `2c0aff8d→6683f989`；清点 `d5b74eb7`；房务 `37f8050a→363e3bc3` / `da112a2f→89082de0`。分支 `mcp6-sacc34` / `mcp4-housekeeping` → `merged/`、远端删；树 `idp-parcel-mcp6-sacc34`（作者即本人，比内容后拆）/ `idp-parcel-mcp4-housekeeping`（通道 4 会话已成通道 1 新会话再断，`status` 空、两文件与 main 逐字节同，代拆）/ `idp-replay-sacc34`（推完拆）。
- **前沿**：在途 0；ready 1——**sa-cc/32**（Blocked by 全解：31 与 34 都在 main；`FingerprintEventID` 只引；等用户一句或下一轮自决再派，SA owner 口径）；draft 1——pp-seams/04（NO owner，待裁四条）；2 / 5 空闲（5 点名未答但心跳在等）；1 / 3 / 4 三通道 crash 待用户重开；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**（累计）：CI 计费与 admin-web `tsc` 步；六件 untracked；**通道 1 今天第八次重置**——推送方角色今天四次在「验绿之后、push 之前」断掉，建议把「验绿即 push、簿记后补」写成硬规矩并让作者在评审无阻断时可直接 `push <验过 SHA>:main`（评审人已核过同一 SHA），不必等推送方活着。
- **本节五数**（18:4x–19:5x）：CI 未起跑（计费）· 集成时延——34 代码推齐 18:44（`e2941cde`）→ 进 main 19:5x **约 70 分**（其中 18:56→19:1x 约 20 分是通道 1 两任消失的空档、19:1x→19:5x 约 40 分是接手取数 + 全量重跑 + 点名 + 评审）· 重放笔数 **8**（34 六笔 + 房务两笔，不含清点与本笔）· 重复开发 0 · **会话重置 3**（通道 1 两次：18:5x 旧任 + 19:1x 新任；通道 3 一次：18:49 起心跳停）· 非作者评审缺席 0（3 未回 → 改派 2 补上）
## 2026-09-16 10:0x 通道 1 新会话（用户先用自带 `ask_question` 验通道，再经队列问「这个项目的 scratch，开发工作结束了吗」→ 答「没结束但接近开发方可达的终点」→ 用户「sa-cc/32 收掉」→ 11:1x 用户问「sa-cc/32 收了吗」）

### 10:1x–10:4x：盘 .scratch 答「结束了吗」

- 量法：`Status:` 首 token 统计（UTF-8 读、`\s（(—·,，;；` 切）——346 张实施票，终态 335（resolved 325 / 已进 main 8 / done 2），非终态 11：ready 1（sa-cc/32）、draft 3（pp-seams/04、lc/20、lc/22）、needs-info 4、blocked 2、superseded 1；24 份父 spec：resolved 18 / superseded 1 / in-progress 5（first-tenant-runway、label-channel、pp-seams、sa-cc、tenant-implementation-01）。`completion-assessment-2026-09-04` 时是 224 张关 199——宣告就绪后又开又关了 120 多张。开发计划 `Status: resolved`（M4 08-28 到账）。答：机制半边已宣告产品就绪且天天在收尾；剩的一半按红线等租户，不是欠账；干净的停手点是 sa-cc/32 收掉 + CI 变绿。

### 10:5x–11:5x：sa-cc/32 认领 → 七笔 → 自审 → 进 main（作者 = 推送方 = 本会话）

- **开工第一眼**：`branch-state.ps1`——main = origin = `21bb1310`，在途分支零，共享树 76 处未提交（与本票零重叠，不动）。隔离树 `%TEMP%\idp-parcel-mcp1-sacc32` 分支 `mcp1-sacc32`；占号广播 10:5x。Docker Desktop 未起 → 起之 → `idp-parcel-postgres-gate` 复用（compose 撞名，`docker start` 即可；顺手删了误建的 `idp-parcel-mcp1-sacc32_default` 网络）。
- **/implement → /tdd 四片 red→green**（每片一笔、每笔推分支）：`58edc520` 适配器（指纹 ID + `ports.ErrFundsFactHandoffRejected`；red 是编译期 undefined）；`71ff0d3e` 编排（`handOffFact` 改 `(string, error)`，三路返错；red 三格 `err = <nil>`）；`57a0d7dd` CLI（`execute` 归用法格 1；red 是真库上退 3「未决」）；`90da4da2` 端点 400 `HANDOFF_ENVELOPE_REJECTED` + admin-web 词条（red 两口答 500）。加 `e91824b3` 真库两 ID 幂等（首跑即绿，钉既有行为）+ CC 夹具头注；`5a8080db` `cmd/parcel-api` 一处按 ID 读信封漏网（取证条 (6) 之后 31 才落，跑反向依赖带 DSN 红出来才见）。
- **验证**：`gofmt -l` 空 / `go build` / `go vet` 0；反向依赖用 `go list -f '{{.ImportPath}} {{.Deps}}'` 反查、带 DSN `-p 1 -count=1` 22 包 ok；两只真库格 `-v` PASS；清点在 `5a8080db` 干净检出重生成零 diff（票面预报 +1/+1 已随 34 落）；迁移零 diff；(5) grep：CC `fingerprintEventID` 零命中，`FingerprintEventID(` 生产命中 = 定义 1 + SA 1 + CC 10。
- **评审门**：点名 11:13（截止 11:17）**零应答**——队列里只有自己两条广播的回声；`/code-review` 两轴子代理各一 + 重试一 → 三次「Authentication error」（与 09-14 sa-cc/11 / 12 / 13 同一失效）。按 parallel-sessions「没有可用通道时推送方自己审」退到**自审**：Standards 0 阻断 / 2 非阻断（测试口名字面三处、`adoptedFact` 与 `existingFact` 同形），Spec 0 阻断 / 1 判断项（判据 (3) 间接观测）。票面如实写「推送方自审，不算非作者评审」。
- **push**：票面 `3fe6bedf` 成笔后 `ls-remote` 核 `21bb1310` 未动 → tip 带 DSN 全量 **115 ok / 0 FAIL / 16 无测试 / 0 cached**（127 s）→ 再 `ls-remote` 仍未动 → `push 3fe6bedf:main` 成 → 共享树 `merge --ff-only` 同 SHA → 广播 11:5x。**基底 = main tip，直接 ff、八笔 SHA 一个没换、零重放**——上一节建议的「验绿即 push、簿记后补」这次照做，push 到簿记之间没有空档可断。
- **簿记本笔**（纯 .md 自审）：票 32 Status 加「已进 main」+ 进 main 记录 + 残差两条；spec 32 行 draft→resolved · 已进 main；本节。
- **前沿**：在途 0；ready 0；draft 1——pp-seams/04（NO owner，待裁四条）；lc 20 / 22 等真源；needs-info / blocked 六张阻断在外。**归用户**（累计）：CI 计费与 admin-web `tsc` 步；根目录六件 untracked（`handoff.md` 空文件、`d_tasks.txt`、三个 logo、`docs/api/...`）；共享树 76 处未提交多为 CRLF 幻影未清；子代理鉴权错三天未修，评审门在无空闲通道时只剩自审一条路。34 残差那句 CC `ports.ExternalResultHandoff` 口约头注仍开着（本票只动 SA `ports`）。
- **本节五数**（10:5x–11:5x）：CI 未起跑（计费）· 集成时延——代码推齐 11:0x（`5a8080db`）→ 进 main 11:5x **约 50 分**（其中点名等 4 分 + 子代理三次失败 + 自审约 15 分、票面 + 全量 127 s + push 约 10 分）· 重放笔数 **0**（ff）· 重复开发 0 · 会话重置 0（本会话自 10:0x 起一任到底）· 非作者评审缺席 **1**（点名零应答、子代理鉴权错 → 自审）
### 11:2x–11:3x 同一会话：通道 5 完工报（main CI 红修法）→ 重放 → `aca52b5d` 进 main

- **来报**（通道 5，11:2x，两条）：main 自 09-14 计费恢复起每 run 红在 `go vet`，成因是 `internal/transportfulfillment/adapters/networkrouting/delivery_windows.go` 文件名的 `_windows` 后缀被 Go 当 GOOS 约束——linux 上整包无文件、`cmd/parcel-api` 导入即断，本机 Windows 看不见；09-08–09-14 那段红是 GitHub 计费「job not started」，非代码。修法只改名两件 → `delivery_window_source{,_test}.go`（加一段中文头注说理由）+ `production_wiring_baseline.txt` 注释里那一处文件名。分支 `mcp5-ci-goos@df9ce8ac`，父 `21bb1310`；PR #3 的 run 35051378254 对 `df9ce8ac` **全绿**（Static checks + 四片 Test 含 `-race` 与 DSN）——仓里 09-07 `96558acd` 之后第一个绿 run。
- **推送方**：完工报到时 main 已是 `8f68f41f`（32 刚落），`df9ce8ac` 不能直推 → 隔离树 `%TEMP%\idp-replay-ci-goos` 于 `8f68f41f` cherry-pick，零冲突：`df9ce8ac → aca52b5d`（与 32 的 17 件零重叠，不必回作者重验）。通道 5 随后自己 rebase 出 `18f4a218`（父 `8f68f41f`），与 `aca52b5d` patch-id 相等、树 diff 空。**自审**（「只改名不改义」归「不评什么」）：头注中文、说的是为什么不能叫 `_windows`、与同级 `*_source.go` 看齐；基线注释一处文件名对上；无发现。**验证（tip `aca52b5d`）**：`gofmt -l` 空；**`GOOS=linux go build ./...` / `go vet ./...` 零信号**（修的正是这一格，本机 Windows vet 本来就绿，不算证据）；清点重生成零 diff；带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**（131 s）。`ls-remote` 核 `8f68f41f` 未动 → `push aca52b5d:main` 成 → 共享树 ff（与 76 处未提交零重叠）→ 广播 + 单发通道 5（SHA 对照、拆树先比内容、PR #3 归它处置、main 上这一笔的 CI run 结果报我一句）。
- **未动**：`mcp5-ci-goos` 指针待通道 5 拆树后改 `merged/`（它的树还检出着这支）；`idp-parcel-mcp2-ci-diag`（detached `21bb1310`）是通道 2 的，没碰。
- **前沿**：在途 0；ready 0；draft 1（pp-seams/04）。**归用户**：CI 计费——main 上 `aca52b5d` 那一 run 若起跑且绿，「远端 CI 自 08-12 无一次 success」这句就该在基线文档里改掉（通道 5 报结果后再动）；admin-web `tsc` 步、根目录六件 untracked、子代理鉴权错照旧。
- **11:4x 追记**：通道 5 回报三件——树按「先比内容」拆掉（`remove` 退 255 是另一 shell cwd 在树里，逐格查后清壳）、PR #3 已关并留评论指向 `aca52b5d`、**main 上 `aca52b5d` 的 run 35052004796 全绿**（Static checks + 四片 Test，rest 4m10s）——**main 自 09-07 16:10Z `96558acd` 以来第一个绿 run**。推送方随手：`mcp5-ci-goos` → `merged/`、远端删；基线文档「运行事实加注」那条从 08-28 版换成 09-16 版（空窗三段 + 首个绿 run 的 run 号，不删只改，证据层级如实）。通道 5 释号、空闲。
- **本节五数**（11:2x–11:3x）：CI **main 绿/总 1/1**（ca52b5d run 35052004796，11:4x 追记；PR #3 另绿 1/1）· 集成时延——完工报 11:2x → 进 main 11:3x **约 10 分**（重放 + GOOS=linux 核 + 全量 131 s）· 重放笔数 **1** · 重复开发 0（通道 5 自 rebase 的 `18f4a218` 与重放同 patch-id，不算）· 会话重置 0 · 非作者评审缺席 0（改名笔按「不评什么」推送方自审）

### 11:5x–12:2x 同一会话：用户「三项待办全部收掉，close 项目」→ 通道 1 / 2 各收一半 → `f961a30a` 进 main，三项清零

- **来令**：11:5x 用户问「三项待办现在能继续吗」，只读取证答三条——admin-web `tsc` 步能做但卡依赖来源（lockfile overrides 指别的宿主上的 `file:/home/tops/idp-ui-tgz/*.tgz`、`package.json` 无 overrides、`.npmrc` 指 GitHub Packages，三者互不认，CI 装不了）；六件 untracked 纯归裁；子代理鉴权错刚烟测仍复现。用户回「你自决，全部都收掉吧，我要 close 这个项目了」。
- **并行现场**：通道 2 同刻在收另一半，其 11:5x 占号广播本会话读到时已推完——`754dde04` docs/api 方案稿原样入库 + README 历史材料一条；`01f11736` 根目录三 logo 原样入库；两个 0 字节空文件删；76 处 CRLF 幻影逐文件 hash-object 核与 HEAD 全等后 `checkout --` 复原不成笔；`idp-parcel-mcp2-ci-diag` 树拆（暂存内容 ⊆ `aca52b5d`）。本会话曾把方案稿搬 `docs/reference`、logo 搬 `docs/brand`，见其两笔已落地即撤回自己的搬动、照落地形状。**教训一句**：同一裁决两个通道各自「自决」时，先落地的算，后到的改自己不改别人——两边都写了 README 入口，我那条没提就撤，避免同一稿两条入口。
- **本会话那一半（`f961a30a`，作者 = 推送方，基 `01f11736` 直接 ff）**：
  - admin-web `tsc` 步：裁**七份 tgz 随仓 vendor**（`apps/admin-web/vendor/idpxyz-ui/`，含只作传递依赖的 `ui-icons`；WSL 源目录 sha256 逐一同，合计 318 KB），`pnpm.overrides` 七行钉文件 + `packageManager=pnpm@10.32.1`，lockfile 重写只动 45 行来源路径、其余零差。CI 新增 `admin-web` job（与 static 并行，不装 Go 不起库）：setup-node v6.5.0 钉 SHA → `npm i -g pnpm@10.32.1` → `pnpm install --frozen-lockfile` → `node node_modules/typescript/bin/tsc -b --noEmit` → `node scripts/run-tests.mjs` → `pnpm exec vite build`。本机删 `node_modules` 全新 frozen install 3 s、tsc 0、run-tests **220/220**、vite build 0（2765 modules，3.35 s）——此前 `pnpm build` 在 PostCSS 装载阶段断（`@alloc/quick-lru` / `picocolors` 缺）是半残安装，非源码，workflow.md 那条据此改写。`apps/admin-web/README.md` 删 PAT 段与「CI 需读包凭据」跟进，安装与验证口径改 vendor 来源。
  - 子代理鉴权错：11:5x 用一个不读不写的最小 explore 子代理烟测，原样 `Authentication error. If you are logged in, try logging out and back in.`（与 09-14 起 sa-cc/11 / 12 / 13 / 32 同）。仓内不可修，处置 = 用户登出重登 Cursor；记入 `docs/agents/workflow.md` 本机环境一条（识别标志 + 评审门退自审的落法），不再挂「归用户」。
  - 验证：清点重生成零 diff；`test-shards.sh check` 131 包通过；ci.yml PyYAML 解析 4 job / admin-web 7 步；暂存五个文本 blob 无 BOM 无 CR。`ls-remote` 核 `01f11736` 未动 → `push f961a30a:main` 成（12:13）→ **CI run 35054768775：`admin-web` job 首跑 success，29 s**（04:13:34→04:14:03Z）；整 run **全绿**——Static checks 1m12s、四片 Test（customs-transport 1m44s / network-visibility 2m00s / main-chain 2m57s / rest 4m01s）全 success，run 总时 4m12s（04:13:31→04:17:43Z）。main 上连续第三个绿 run（`aca52b5d` → `01f11736` → `f961a30a`）。
- **前沿**：三项待办清零；在途 0；ready 0；draft 1（pp-seams/04，NO owner）。**归用户**只剩：Cursor 登出重登（子代理鉴权，仓外）；远端旧名分支未删（不动）。项目按用户令 close。
- **本节五数**（11:5x–12:2x）：CI **main 绿/总 1/1**（run 35054768775，含首个 admin-web job）· 集成时延——来令 11:5x → 进 main 12:13 **约 20 分**（取证 + vendor + 全新安装三道门 + ci.yml + 两处 .md，无评审等待）· 重放笔数 **0**（ff）· 重复开发 **1 次撤回**（方案稿 / logo 搬动与通道 2 撞，见其落地即撤，未成笔）· 会话重置 0 · 非作者评审缺席 **1**（构建配置与 .md 一笔，推送方自审；子代理仍鉴权错）
## 2026-09-16 15:3x 通道 1 新会话（用户先用自带 `ask_question` 验通道，再经队列问「这个现在到什么程度了：`idp-parcel-mcp6-adminweb` / `idp-parcel-mcp1-api-adrs`」→ 答两棵树状态 → 用户「你能够派发给相关的通道来完成吗」）

### 15:3x–15:4x：盘两棵树 → 点名 → 派单

- **取证**（读 git，不读自报）：两棵 worktree 都在 `a608536d`（= main = origin）零提交，全部工作躺在工作区未提交；文件 mtime 全在 13:27–13:36，之后两小时没动；`list_sessions` 通道 2–6 `running` 但 lastActivity 同在 13:3x；`query_tasks` 的 5 pending / 3 working 全是 09-14~15 旧记录（对应分支已 `merged/`）。
  - `idp-parcel-mcp1-api-adrs`（`mcp1-api-adrs-0140-0142`）：ADR-0140 / 0141 / 0142 三草案（Proposed，Context / Decision / Consequences / Alternatives / 风险点 / Links 齐全）+ README +3 行 + `.scratch/first-party-api` spec（draft）+ 三票（draft，阻塞边指 ADR-0139 接受）。文稿成形、只差 commit。
  - `idp-parcel-mcp6-adminweb`（`mcp6-admin-web-legal-entities`）：spec in-progress + 6 票（01/02 in-progress，03/06 ready-for-agent，04/05 needs-info）；逻辑层已落（`moment.ts`、`legal-entity-list.ts`、`legal-entity-form.ts` 各带 test，`status.tsx` 补 `已登记` / `已停用`，`RegistrationAnswerNote` 抬成导出件）；实测 `tsc -b --noEmit` 绿、`run-tests` 235/235；`GroupLegalEntitiesPage.tsx` 一字未动、`LegalEntityRegistrationForm.tsx` 不存在——页面半为零。
- **点名** 15:40（截止 15:43）→ 15:41 前 **5 个应答**（通道 2 / 3 / 4 / 5 / 6 皆「空闲 · 地盘无 · 余量充足」——通道 6 自报地盘无，即它不记得自己那棵树，派单里因此写明从哪接）。
- **派单**：
  - `task-a7bd108e` → 通道 6：admin-web 01+02 页面接线，写明「从 `mcp6-admin-web-legal-entities`@a608536d 工作区（未提交）接着做，不另起」，第一笔先原样 commit 逻辑层；完工报 SHA、不推 main。
  - mcp1 那棵是通道 1 自己的地盘 → 本会话 `git add -A` 原样收入成 **`21b20938`**（八份文件 CR=0 无 BOM，正文一字未改；提交消息 `-F` 单引号 here-string，标题反引号保住）。
  - `task-b3ade6d7` → 通道 2：ADR 三草案两轴非作者评审，钉 `21b20938` 基线 `a608536d`，时限 20 分（至 16:05），report_task + 单发通道 1；作者分支不动。
- **前沿**：在途 2（mcp6 01+02 · 通道 6；ADR-0140/0141/0142 评审 · 通道 2）；评审无阻断则 ff `21b20938` 进 main 推远端。ADR-0139 → Accepted 与 first-party-api 三票激活归用户。
### 15:4x–15:5x 同一会话：通道 2 / 3 / 6 crash → 收通道 6 半成品 → 重新点名 → 改派 4 / 5

- **来报**（用户 15:4x，附两张截图）：「mcp-2/3/6 crash，mcp-6 做到如图 crash」。读 git 不读截图：通道 6 已按派单第一笔把逻辑层 commit 成 `bad2cea8`，接着 /tdd 写票 01 裁决 1（`legalEntityCountSummary` / `legalEntityNoMatchNote` + 测试、`ListPageTemplate` 加 `emptyRowsNote` 可选槽），15:48:54 最后落盘后死在「Thought for 57s」；三文件未提交。通道 3 空等时也 crash——crash 是 Cursor 侧的，不由任务触发。
- **收半成品**：三文件实测 `tsc` 0、`run-tests` 236/236，是完整一片 green → 派发方原样收入 **`8314b7d8`**（消息写明作者通道 6、通道 1 只收不改）。通道 2 的评审单无产出。
- **重新点名** 15:5x（截止 15:56）→ 15:54 前 **2 个应答**（通道 4 / 5）；2 / 3 / 6 未应答（记「截至 15:56 未应答」）。原单 `task-a7bd108e`（→6）、`task-b3ade6d7`（→2）作废，工具无取消口、留作记录。
- **改派**：`task-86714afe` → 通道 4：01+02 页面接线从 `8314b7d8` 接，**每片 green 即 commit**；`task-96162690` → 通道 5：ADR 三草案两轴评审钉 `21b20938`，**评完一轴先报一轴**，时限至 16:15。
- **前沿**：在途 2（同上，换了人）；会话重置本节 **3**（通道 2 / 3 / 6）。

### 15:5x–18:2x 补记（16:0x–18:2x 之间的推送方会话未及写入 tasks.md；按 git 提交时间、任务台账 context / result 与队列补，时刻取 git / 台账，不取任何人的自报）

- **01+02 进 main**：通道 4 按 `task-86714afe` 从 `8314b7d8` 接，`e8d31b46`（16:03，票 01 页面接线）、`71c7d5a3`（16:06，票 02 逐字段表单）两笔推分支；台账始终 pending（未 report）。17:32 点名（截止 17:36）5 应答 → **`task-825d2d6b` → 通道 3** 两轴评审（17:39 派、17:48 交）：0 阻断 / Standards 4 非阻断 / Spec 3 非阻断（最重：`sortLegalEntities` 字符串比在 RFC3339Nano 剪尾零的同秒内错序）。推送方代作者（4 / 6 会话已换）在同分支修 Standards 1 / 2 / 4 / 附 + Spec 2 → `4306a34c`（17:57）→ **`task-eb56aa94` → 通道 3** 只重跑 Standards 轴（18:00 派、18:03 交：已消、新发现 0）→ 簿记 `3c299854`（17:59，票 01 / 02 完成记录 + 评审三格 + Status resolved）→ 分支基 `a608536d` = 当时 main，直接 ff，零重放。
- **ADR-0140 / 0141 / 0142 进 main**：`task-96162690`（→5，15:56）无产出；17:37 改派 **`task-54cee2f9` → 通道 2**（17:48 交）：阻断 1（0141 引 0140 Consequences 里不存在的「契约集成测试」）+ 非阻断若干 → 作者（推送方自己）同分支修 `6425deb9`（17:53）→ **`task-db69ad20` → 通道 2** 复评（17:54 派、17:57 交：阻断 1 已消、新发现 0）→ 重放到 `3c299854` 之上：`21b20938→8a7ec41b`、`6425deb9→24ee60dd`（18:01 进 main，CI run 35082937398 success）。
- **派 03 / 06**：17:42 **`task-b16e54a9` → 通道 4** 票 06（ADR-0091 逐口放行身份族五口；分支 `mcp4-pc-identity-isolated-write`）；17:44 **`task-7f3eeaff` → 通道 5** 票 03（读口 + 抽屉历史区；分支 `mcp5-legal-entity-revisions` 基 `71c7d5a3`）。
- **03 作者半途断、推送方重放后亦断**：通道 5 `028c0d8a`（18:04，Go 半边）推分支；前端半边已 stage 未提交时会话断 → 推送方按「未提交现场」原样封存 `284c49d3`（18:11，`chore(salvage)`）。18:11 点名（截止 18:14）应答 2 / 6 → **`task-2ad8c9f0` → 通道 2** 两轴评审（18:15 派；派单里写「通道 1 现由通道 3 会话接管」，即这一任推送方是开在通道 3 窗口里的会话）。同时在 `%TEMP%\idp-land`（detached `24ee60dd`）重放 `028c0d8a→53238188`、`284c49d3→9a297b2c`，清点重生成 `ef7086f0`（18:17；partycommercial 生产 123→126 / 测试 138→140、端口声明 411→412、端点 124→125），树内 `pnpm install`（真目录，18:18）跑前端三道门，带 DSN 全量起跑——**之后无任何动作**：数未留、未 push、未簿记（评审派单里「带 DSN 的一轮正在跑，结果由通道 1 自记」那句没有下文）。今天第五例「重放做好、簿记之前断」。
- **06 在途**：通道 4 `7900ab12`（17:57）/ `ab6e7a48`（18:00）/ `d0814692`（18:02）/ `52e1cd93` / `ee4fb886`（18:08）五口五笔全推 origin，树干净；台账 pending 未 report，队列里无完工报。

## 2026-09-16 18:3x 通道 1 新会话（用户先用自带 `ask_question` 验通道，再令「调 idp-mcp-1 的 check_messages 开始监听」；首条消息即通道 2 的 03 评审汇总）

### 18:3x–18:5x：03 评审到（两轴 0 阻断）→ 重验 idp-land → `ef7086f0` 进 main → 注释修正 + 簿记

- **评审 ← 通道 2**（18:27 / 18:29 分轴、18:31 汇总，`task-2ad8c9f0` done）：Standards 0 阻断 / 4 非阻断（三处 CONTEXT 旧节名「身份生命周期」；两处计数引用；两条 Fowler 判断题可接受）；Spec 0 阻断 / 2 非阻断（清点未随笔——推送方已在 tip 重生成；票面「ADR-0078 决定四只此一维」已被 ADR-0091 决定一停用——票面改一句）。全文代落票 03 Comments。
- **量**（只按 git，不读自报）：`ls-remote` main = `24ee60dd` = 本地 = origin/main；`idp-land@ef7086f0` 干净、merge-base = main、`main..` 三笔 = 03 两笔重放 + 清点，03 的 17 件对作者 tip `284c49d3` 零 diff；作者树 `mcp5-le-revisions@284c49d3` 干净、= origin；台账 `query_tasks` 补齐上节。
- **验证（本会话重跑，不沿用前任的数；钉 `ef7086f0`）**：`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；清点在 tip 重生成 porcelain 空、生成器 vet / test ok；18:36 占号广播 → 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**，退出码 0（18:38:45→18:40:44，119 s）；`-v` 探针 `TestLegalEntityRevisionHistoryListsAllRevisionsByRevisionNumber` / `…AnswersUnknownAndForeignAsEmpty` PASS 非 SKIP；admin-web `tsc -b --noEmit` 0、`run-tests` 240/240、`vite build` 0。
- **push**：`ls-remote` 再核 `24ee60dd` 未动 → 18:42 `push ef7086f0:main` 成，**远端 main = `ef7086f0`**，CI run 35086324656 **success**；共享树 `merge --ff-only` 同 SHA（tasks.md 未提交 20 行与本批 18 件零重叠）→ 释号广播。**先 push 再簿记**照 15:2x 节的规矩，这次 push 到簿记之间没有空档可断。
- **簿记两笔**（在 `ef7086f0` 之上，推送方自审）：(1) 纯注释——评审 Standards ①② 五处：`ports/legal_entity_revision_history.go`、`http/query_legal_entity_revisions.go`、`legal-entity-revisions.ts` 三处 CONTEXT 节名改为 Lifecycles「参与方身份（业务参与方、责任法人、货主客户账户）」，`query_party_identities.go` / `legal_entity_revision_history.go` 两处去计数；`git diff -U0` 滤 `//` / `*` 行后空、diff 无 CR，gofmt / build / vet 0、http 包 ok、tsc 0。(2) 纯 .md——票 03 Status resolved + 完成记录（推送方按 `git diff 71c7d5a3 284c49d3` 代写，作者会话已无；浏览器验收如实记未验）+ 评审代落 + 进 main 记录 + 第 3 条改句；spec 03 行；本节与上节补记。→ `ls-remote` 核 `ef7086f0` 未动 → push → 共享树 ff。
- **收尾**：分支 `mcp5-legal-entity-revisions`（17 件对 main 零 diff）/ `mcp1-api-adrs-0140-0142`（`6425deb9` 与 `24ee60dd` patch-id 同、分支改过的每件对 main 零 diff）/ `mcp6-admin-web-legal-entities`（`3c299854` 是 main 祖先）→ `merged/`、远端删；树 `idp-land`（先删 `node_modules` 真目录再 `remove`，不加 `--force`）/ `idp-parcel-mcp5-le-revisions`（作者会话已无、`status` 空，代拆）/ `idp-parcel-mcp1-api-adrs`（自己的）/ `idp-parcel-mcp6-adminweb`（4 / 6 会话已换、`status` 空，代拆）四棵拆；`idp-parcel-mcp4-pc-identity-write` 在途不动。
- **前沿**：在途 admin-web-group-legal-entities/06（通道 4，`mcp4-pc-identity-isolated-write@ee4fb886` 五口五笔已推、无完工报，`task-b16e54a9` pending——等完工报或下一轮点名问一句）；ready 0；needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票 draft 等 ADR-0139 接受。共享树 18:38 新出现 untracked `docs/ux/commercial-party.md`（29 KB，18:4x 仍在改），不是通道 1 的，已广播问认领、未动。**归用户**：ADR-0139 → Accepted 与否；`docs/ux/` 归属。
- **本节五数**（15:5x–18:5x，含补记段）：CI **main 绿/总 2/2**（`24ee60dd` run 35082937398、`ef7086f0` run 35086324656；01+02 四笔随 `24ee60dd` 同批推出，无单独 run）· 集成时延——01+02 代码推齐 16:06 → 进 main 18:01 **约 115 分**（其中约 85 分是 16:0x–17:3x 推送方空档、评审 + 修 + 复评约 30 分）；ADR 三草案 `21b20938` 15:43 → 18:01 约 135 分；03 代码推齐 18:04 / 封存 18:11 → 进 main 18:42 **约 35 分**（其中 18:2x–18:3x 约 15 分是推送方会话断的空档）；中位数约 115 分（3 票）· 重放笔数 **4**（ADR 两笔 + 03 两笔，不含清点与簿记）· 重复开发 0 · **会话重置（可证的）4**（推送方两次：16:0x–17:3x 空档那一次 + 18:2x 那一次；通道 5 一次 18:0x——03 前端半边 stage 后断；通道 4 / 6 至少一次——17:5x 派单写「4 / 6 已换会话」）· 非作者评审缺席 0（三张代码票各有非作者评审；两笔簿记按「不评什么」自审如实计）

### 18:5x–19:1x 同一会话：06 接手单 → 作者 rebase 收尾 ‖ 通道 2 评审 → `f6569f51` 进 main → 立 07

- **通道 4 应答**（18:5x）：新会话无记忆、未接手 06。推送方按 git 量现场（五笔全推、树干净、票面 .md 不在分支上——分支基 `a608536d` 早于票面入库；隔离树干跑 cherry-pick 到 `a7bdd666` 零冲突、build / vet 0、两包 ok、清点预报 3 行）→ **`task-c4255f39` → 通道 4**（按其自报空闲即派，未另点名）：rebase 到 origin/main → 带 DSN 包组 → 按 diff 代写完成记录 → report；不改代码、不碰 spec / tasks / 清点。干跑树拆（`worktree remove` 退 255 Permission denied、登记已摘，按 workflow.md 清壳）。
- **作者侧**（通道 4 新会话，18:5x–19:07）：rebase 零冲突 `7900ab12→b6f99b6e` / `ab6e7a48→0a95ba6f` / `d0814692→89771bc1` / `52e1cd93→515b7c14` / `ee4fb886→835690b8`，`--force-with-lease` 推；读五笔 diff 对票面 1–4 无阻断语义问题；19:0x 占号 → 带 DSN 三包（`partycommercial/adapters/http` / `cmd/parcel-api` / `architecture`）ok 0 FAIL、探针 `TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase` PASS、`cmd/parcel-api` 全 `-v` SKIP=0 → 19:02 释号；完成记录 `da0b72cd`（只票面 .md，+33/−1，判断项 11 条）；`report_task done` 19:07。
- **评审 ← 通道 2**（**`task-ec977434`**，代码 SHA 定下即 18:5x 派、与作者收尾并行；19:06 两轴、19:07 全文）：Standards 0 阻断 / 3 非阻断（N1 `decodeClosedDocument` 与同包 `decodeStrict` 重复且少尾随内容拒——行为分叉；N2 Fowler 判断题——非五份同形，可收约二十处错误包装成助手；N3 两处注释跨包计数）；Spec 0 阻断 / 1 非阻断（S1 票面 201 `PARTY_IDENTITY_REGISTERED` 与线上名 `REGISTERED` 不符——改票面）；`da0b72cd` 判断项 11 条核过无失真。全文代落票 06 Comments。
- **进 main**（零重放——作者已 rebase，六笔原 SHA ff）：`%TEMP%\idp-land06` detached `da0b72cd`，清点重生成 **`f6569f51`**（partycommercial 生产 126→127 / 测试 140→141 / http 适配器 28→29，`cmd/` 测试 89→90；合计 996 / 947）；`gofmt -l` 空、build / vet 0；19:07 占号 → 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**，退出码 0（19:08:33→19:10:43，130 s）；探针 `TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase` / `TestEveryAssembledEndpointAnswersUnconfigured` PASS → `ls-remote` 核 `a7bdd666` 未动 → 19:11 `push f6569f51:main` 成，**远端 main = `f6569f51`**（push 后一次 `ls-remote` 撞 schannel TLS 握手失败，push 本身已报 `a7bdd666..f6569f51`）→ 共享树 ff 同 SHA → 释号广播。
- **簿记本笔**（纯 .md 自审）：票 06 完成判据 S1 改句 + 进 main 记录 + 评审代落三格；**立 07**（bug，ready-for-agent，A 类尾巴：`decodeClosedDocument` 改调 `decodeStrict` 补尾随内容拒 + 三处注释去计数，可选 N2 助手）；spec 06 行 → resolved、新 07 行；本节。→ `ls-remote` 核 `f6569f51` 未动 → push → 共享树 ff。
- **收尾**：作者树 `idp-parcel-mcp4-pc-identity-write` 归通道 4 比内容后拆（已在释号广播里写明比法），拆完推送方把 `mcp4-pc-identity-isolated-write` → `merged/`、远端删。`idp-land06` 推完拆。
- **前沿**：ready 1——admin-web-group-legal-entities/07（即派通道 4，作者收自己的尾巴；评审派通道 2）；在途 0；needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票等 ADR-0139。`docs/ux/commercial-party.md` 仍 untracked、无人认领、未动。**归用户**：ADR-0139 → Accepted 与否；`docs/ux/` 归属。
- **本节五数**（18:5x–19:1x）：CI main 绿/总 未量（`f6569f51` run 刚起）· 集成时延——06 代码推齐 18:08（`ee4fb886`）→ 进 main 19:11 **约 63 分**（其中 18:08→18:5x 约 45 分是作者会话断、无人接手的空档；接手单到 push 约 20 分）· 重放笔数 **0**（ff）· 重复开发 0 · 会话重置 **1**（通道 4，18:5x 自报新会话）· 非作者评审缺席 0

### 19:1x–19:2x 同一会话（本节由下一任按台账与队列回显补记，无自报可读）

- **派 07**：`task-17ba8eed` → 通道 4（19:16，作者收自己的尾巴；隔离树 `D:/tops/idp-parcel-mcp4-adminweb07`，分支 `mcp4-adminweb07` 基 `2e8183d6`）。作者 19:24 `report_task done`：`23f3582e`（要做的 1 红→绿）/ `5dff14d7`（要做的 2 纯注释四处）/ `19047d51`（票面 Status resolved），三笔全推 origin；本包 182 PASS / 0 SKIP，`cmd/parcel-api` 带 DSN 探针 PASS，清点预报零差；可选 N2 未做已写理由。
- **派评审**：`task-8d992f9a` → 通道 2（19:25，钉 `5dff14d7`，15 分限）。通道 2 当时那个会话 19:25:31 检出 `%TEMP%\idp-review-07@5dff14d7` 后无下文（台账停在 pending、无分轴消息）。
- **占号广播**（19:2x）：「07 作者完工、评审已派通道 2、推送方并行在 `%TEMP%\idp-land07` 带 DSN 全量一次，勿占 55432」——`idp-land07` 确已 detached 到 `19047d51`，**但那一跑没起**（下一任 19:3x 查：无 go 进程、`idp-land07` 无日志文件、terminals 无对应命令）。广播之后会话断。教训归「开窗通知与完工报成对，做不成也要报」那一格：占号说了要跑，跑没起也没人知道，55432 在别人眼里空占了十几分钟。

## 2026-09-16 19:3x 通道 1 新会话（用户先用自带 `ask_question` 验通道，再令「调 idp-mcp-1 的 check_messages 开始监听」；队列里三条：通道 4 的 07 完工报、前任的占号广播回显、一页项目概览的摘要）

### 19:3x–19:5x：接 07 完工报 → 补跑全量 → 催评审 → `19047d51` ff 进 main → 簿记

- **量现场**（只按 git 与台账，不读自报）：`ls-remote` main = `2e8183d6` = 本地 = origin/main；`mcp4-adminweb07` 本地 = 远端 = `19047d51`，基 `2e8183d6`，可 ff，对 main 四件 +77/−11；worktree 四棵——共享树、`idp-land07@19047d51`（干净）、`idp-review-07@5dff14d7`（干净）、作者树；`query_tasks`：07 作者单 done、评审单 pending。前任广播的「全量在跑」不成立（无进程、无日志），按过期断言处置、不沿用。
- **补跑（钉 `19047d51`）**：`idp-land07` 干净检出上清点重生成 porcelain 空（零差，无单独清点笔）、`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；19:37 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**，退出码 0（19:37:45→19:39:38，113 s）；`-v` 探针 `TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase` PASS 非 SKIP；新用例 `…RefusesTrailingContentAfterThePayload` 登记口 / 停用口两子测试 PASS。19:40 释号广播（写明前任那一跑没起、此为补跑）。admin-web 未跑：本票只 Go + .md。
- **评审**：19:40 台账仍 pending、15 分限到 → 催通道 2（19:47 硬限，到时自跑隔离 /code-review）。19:41 通道 2 **新会话**认领（自报「队列里只到两条广播、从 `query_tasks` 才看到此单」——派单消息本身没送达那个新会话，靠台账兜住），硬限改 19:56；提醒它复用已存在的 `idp-review-07`。19:49 两轴到：**Standards 0 阻断 / 1 非阻断（N1 `PartyIdentityDeactivationIntake` 头注「名称镜像」放在命令上不精确）· Spec 0 阻断 / 2 非阻断（S1 判断项 2 对 `encoding/json` 的断言不成立——本机探针 `{"a":1} garbage` 第一次 Decode err=nil；S2 完成记录补「500 为替身零值答复，生产上 201 + 第二文档无声消失」半句）**；红已本机复现（换回 `2e8183d6` 版两口皆 FAIL 500）。全文代落票 07 Comments。
- **进 main**（零重放）：`ls-remote` 核 `2e8183d6` 未动 → 19:49:10 `push 19047d51:main` 成，**远端 main = `19047d51`**，CI run 35092371232 起；共享树 `merge --ff-only` 同 SHA（树上只有两块 untracked：`.scratch/project-overview-2026-09-16/`、`docs/ux/`，零重叠）→ 广播「远端 main = 19047d51」+ 通道 4 拆树比法。
- **簿记两笔**（在 `19047d51` 之上，推送方自审）：(1) 纯注释 **`1a2ceef9`**——N1 一词（`register_party_identity.go` 头注「封闭集的名称镜像」→「一个封闭集」）；`git diff -U0` 滤注释行后空、无 CR，gofmt 空、build / vet 0、http 包 ok。(2) 纯 .md——票 07 进 main 记录 + 评审代落三格 + S1 理由改写 + S2 补句 + 标题对齐（零量级项）；spec 07 行 → resolved；本节与上节补记。→ `ls-remote` 核未动 → push → 共享树 ff。
- **上节未量的 CI 补记**：`f6569f51` run 35088959569 **success**、`2e8183d6` run 35089328754 **success**。
- **收尾**：作者树 `idp-parcel-mcp4-adminweb07` 归通道 4 按广播里的比法拆，拆完推送方把 `mcp4-adminweb07` → `merged/`、远端删；`idp-land07` 簿记推完拆（不加 `--force`）；`idp-review-07` 通道 2 已自拆。
- **前沿**：ready 0；在途 0；needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票等 ADR-0139。共享树两块 untracked 都不是通道 1 的活：`.scratch/project-overview-2026-09-16/`（一页项目概览，摘要经队列到达，作者未署通道号；是否入库归用户）、`docs/ux/commercial-party.md`（18:38 起 untracked、无人认领、未动）。**归用户**：ADR-0139 → Accepted 与否；`docs/ux/` 归属；概览页是否入库。
- **本节五数**（19:1x–19:5x，含补记段）：CI main 绿/总 **2/2 已完 + 1 在跑**（`f6569f51`、`2e8183d6` success；`19047d51` run 35092371232 未完）· 集成时延——07 完工报 19:24 → 进 main 19:49 **约 25 分**（其中 19:2x→19:3x 约 10 分是推送方会话断的空档、19:25→19:41 约 16 分是评审侧会话断的空档，两段有重叠；1 票）· 重放笔数 **0**（ff）· 重复开发 0 · 会话重置 **2**（推送方 19:2x→19:3x 一次；通道 2 19:41 自报新会话一次）· 非作者评审缺席 0

### 19:5x–20:5x 同一会话：用户经队列连问四句 → 重建 parcel-api → 业务参与方页评估 → 立票 08–12 → 点名派 08 / 09 / 11

- **用户问「集团与法人不能手动添加吗」**（19:5x）：答页面有「登记法人」签、业务参与方页有「登记」签，「集团」= 运营集团租户本身（ADR-0003 第一级）不是可添加的行；实探本机 parcel-api（`%TEMP%\idp-parcel-run\parcel-api.exe` 13:04 构建、早于 06）：`GET /commercial-group-legal-entities` 200、`POST /commercial-legal-entity-registrations` **403**——墙没降是因为二进制旧。用户令「重建并同环境变量重启即放行」→ 20:06 从 main `dbe989cc` 重建、停旧进程（pid 43888）、同四个环境变量起新进程（pid 45720，8090），启动日志 `admittedCommandLines` 含身份族五口；实探五个 POST 空载荷 **400**（不再 403）。admin-web 5174 走代理不必重启。
- **「如何登参与方身份」「能修改的吗」**：给了 `businessParties` 单项 JSON 形状（不带 tenantId——`refuseSelfReportedTenant` 键在场即拒）、修订号规则、停用走 `deactivations`；「修改」= 同标识登 revision + 1 翻旧插新。答的时候发现登记签提示句仍写「外加整批的 tenantId」——06 前反正 403 看不见，现在照填即 400，且 400 到页面只剩 code、`problemNote('MALFORMED_REQUEST')` 是读口 `?kind=` 的措辞。
- **「请你评估这个页面」→「按你的建议开始吧」**：评估「业务参与方」页（`BusinessPartiesPage.tsx`，钉 `dbe989cc`；代码 + 实探，浏览器未验）落 spec「第二轮」节，四档缺口；**立票 08**（bug：提示句去 tenantId + `MALFORMED_REQUEST` 措辞）**09**（读面打磨同 01 形）**10**（三册逐字段表单，Blocked by 09）**11**（写口 400 带 detail，Go + TS）**12**（参与方修订历史读口同 03 形，Blocked by 09）；spec 子票表补五行、范围补「第二轮」一条。→ `513fc4b5`。
- **push 失手一次（记下，不辩）**：20:42:19 提 `513fc4b5`，20:42:20 通道 6 在共享树 main 上提了 `cadd2d46`（project-overview.html 第 2 版，1808 行纯 HTML）；我的 push 脚本在 `ls-remote` 核过之后取的是 `git rev-parse HEAD` 而不是钉死的 `513fc4b5`，于是 `push cadd2d46:main` 把通道 6 那一笔一并发了。parallel-sessions「推经验证的确切 SHA」那节写的正是这一格，写着照样踩——**脚本里的 `HEAD` 就是会动的分支名**。结果无害（通道 6 随后广播「用户令重放：即将 push cadd2d46」，它本就要发；纯 .scratch，CI 不看），已在点名广播里认错、私信通道 6 说明其 ls-remote 读到的 `dbe989cc` 是旧值。此后 push 一律 `push <字面 SHA>:main`，`$sha` 在 `ls-remote` 之前取、之后不再读 HEAD。
- **点名**（20:43 广播，截止 20:47）：`list_sessions` 2–6 running/online、7–11 offline；应答：通道 2 20:44、通道 4 20:44、通道 5 20:45（均空闲、无地盘、余量充足）；通道 6 20:45 以广播形式释地盘 `.scratch/project-overview-2026-09-16/` 并报空闲（非固定格式，截止前，按空闲计）；**通道 3 截至 20:47 未应答**。派单第 0 步：`branch-state.ps1 -Path` 对 `apps/admin-web/src/pages/party` / `internal/partycommercial/adapters/http` / `apps/admin-web/src/components/registration` 三块地盘均「无 main 之外的提交、无未提交现场」，worktree 只剩共享树。
- **派单**（20:46–20:48）：**09 → 通道 6**（`task-f77eaef0`，`mcp6-adminweb09`，只动两张读签 + 抽屉，登记签一行不动；`Instant` / `useCopyToClipboard` 抬共享）；**11 → 通道 4**（`task-94298e51`，`mcp4-adminweb11`，Go `writeProblem` + `registration_transport.go` + TS `catalogue-api.ts` / `RegistrationPanel.tsx`）；**08 → 通道 5**（`task-934ef20e`，`mcp5-adminweb08`，只 `presentation.ts`）。三票地盘不交。**通道 2 留作评审**不派活。10 / 12 等 09 进 main 后下一轮点名。
- **前沿**：在途 08 / 09 / 11；ready 10 / 12（Blocked by 09）；needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票等 ADR-0139。共享树 untracked 只剩 `docs/ux/`（无人认领、未动）；project-overview 已由通道 6 入库（`cadd2d46`）。**归用户**：ADR-0139 → Accepted 与否；`docs/ux/` 归属。
- **本节五数**（19:5x–20:5x）：CI main 绿/总 未量（`19047d51` / `dbe989cc` / `cadd2d46` 三 run 未收）· 集成时延 无票进 main（本节只立票派单）· 重放笔数 0 · 重复开发 0 · 会话重置 0（本节内无新会话自报）· 非作者评审缺席 0

### 20:5x–22:0x 补记（前任推送方会话未及写入 tasks.md；按任务台账、队列回显与 git 补，时刻取台账 / git，不取自报）

- **完工报三份**：09 ← 通道 6 21:03（`task-f77eaef0` done，`mcp6-adminweb09@bee86b4d` 三笔全推）；11 ← 通道 4 21:23（`task-94298e51` done，`mcp4-adminweb11@a78d465c` 四笔全推；作者会话中途换过一次，票面完成记录自述「前一会话所作…同通道新会话接手收尾」）；08 ← 通道 5 21:23（`task-934ef20e` done，`mcp5-adminweb08@62a1aea5` 两笔全推；代码笔 `f6f1597c` 为前任会话 20:58 所作，新会话接续复跑三道门写完成记录）。
- **派评审三份**：08 → 通道 6（`task-4cda8c0d`，21:06；派单写「作者通道 5 会话已断…你余量将满，只给你这张最小的」）21:36 交：两轴 0 阻断（Standards 0/2、Spec 0/2）；09 → 通道 2（`task-a9ea070f`，21:30）21:38 交：两轴 0 阻断（Standards 0/4、Spec 0/4）；11 → 通道 5（`task-8f469857`，21:31）**至 22:0x 仍 pending、未被认领**——通道 5 的 lastActivity 停在派单那一刻，派单消息没有被任何会话消费。
- **前任 21:32 在 `%TEMP%\idp-land0911` 把 08 + 11 + 09 九笔按此序叠到 `e5ff0f20` 之上（tip `cb8cb066`）、同刻建 `idp-review-11@564f813d` 给评审用，随后无任何动作**——未验、未推、未簿记、未广播。今天第六例「重放做好、簿记之前断」；这一次还多一格：三票里 11 的评审门没过，九笔叠一棵树等于让已过门的 08 / 09 陪 11 一起等——**重放的批要按评审门切，不按完工报到达的顺序切**。

## 2026-09-16 22:0x 通道 1 新会话（用户先用自带 `ask_question` 验通道，再令「调 idp-mcp-1 的 check_messages 开始监听」；首条消息即通道 2 的 09 评审汇总）

### 22:0x–22:3x：09 评审到 → 拆批（08 + 09 先、11 另批）→ 点名改派 11 评审 → `1f569998` / `958e6c57` 两批进 main → 簿记

- **量现场**（只按 git 与台账，不读自报）：`ls-remote` main = `e5ff0f20` = 本地 = origin/main；`branch-state.ps1`：三支 `mcp6-adminweb09@bee86b4d` / `mcp4-adminweb11@a78d465c` / `mcp5-adminweb08@62a1aea5` 全推 origin 同 SHA、三棵作者树干净；`idp-land0911@cb8cb066` 干净未推；共享树只 `docs/ux/` untracked（「未提交 1 行」是 CRLF 幻影）；台账：08 / 09 评审 done 无阻断，11 评审 pending。
- **拆批**：不沿用 `idp-land0911`（序 08→11→09，11 未过评审门）。新建 `%TEMP%\idp-land0809` detached `e5ff0f20`，cherry-pick 08 两笔 + 09 三笔零冲突（三票地盘不交，08 与 09 只在 `pages/party/` 同目录不同文件），patch-id 逐笔同、两票文件对作者 tip 零 diff。
- **点名**（22:08 广播 2–6，截止 22:12）：通道 6 / 5 / 2 三应答（皆「空闲 · 地盘无 · 余量充足」），3 / 4 截至 22:12 未应答。→ **`task-19267f7f` → 通道 2**（11 两轴评审，复用 `idp-review-11`，至 22:35；通道 2 刚评完 09、熟本批地盘）；单发通道 5：`task-8f469857` 作废、不动 `idp-review-11`、08 树拆法。
- **验证 + push（`1f569998`）**：`gofmt -l` 空、`go build` / `go vet` 0；清点重生成 porcelain 空（**生成器不数 `apps/admin-web` 文件**——09 新增六件 .ts/.tsx 清点零变，票面「清点预报」那句落空，已在票面记明）；admin-web 全新 `pnpm install --frozen-lockfile` 后 tsc 0 / run-tests 252 / vite 0；22:12 占号 → 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**（22:12:36→22:14:51，135 s；两票不动 Go，这一跑验的是 tip 整体）→ `ls-remote` 核 `e5ff0f20` 未动 → 22:15 `push 1f569998:main` 成，**远端 main = `1f569998`**（CI run 35107280672 **success**）→ 共享树 ff → 释号广播（SHA 对照、作者拆树比法、知会 TF 页「已撤销」neutral→warning 一色之变）。**先 push 再簿记**，push 到簿记之间没有空档可断。
- **评审 ← 通道 2（11）** 22:25：Standards 0 阻断 / 1 非阻断（注释「六种」跨文件计数，三处）· Spec 0 / 0；真红核过（`registration_transport.go` 换回 `cadd2d46` 版四子测皆 FAIL）；ADR-0140 Decision 三原文核实与票面引述一致、归 owner。派后 12 分交。通道 5 22:2x 报 08 树已拆（`remove` 退 255 Directory not empty → 按 parallel-sessions 配方三样逐查后 `\\?\` 清壳，指针留着）。
- **推送方代落两笔（在 `1f569998` 之上）**：`b185c0f2`——08 N1（`presentation.ts` `tenantGridFilledByChannel` 头注「两口」→「那几口」）+ 09 N1（`BusinessPartiesPage.tsx` `statusBadge` 头注去「三词 / 五词」）+ 09 N2（`GroupLegalEntitiesPage.tsx` 删「自票 09 起住在 detail-primitives.tsx」变更说明行），`git diff -U0` 滤注释行后空、无 CR、tsc 0，自审；`4ec765f2`——08 / 09 票面进 main 记录 + 评审代落三格 + 处置（09 的 N3 / N4 / S3 记为 `pages/party` 收口项，不挡 10 / 12），spec 08 / 09 → resolved、10 / 12 解阻、11 → in-progress。
- **验证 + push（`958e6c57`）**：`%TEMP%\idp-land11` detached `4ec765f2`，11 四笔 cherry-pick 零冲突（patch-id 逐笔同、8 件对作者 tip `a78d465c` 零 diff），其上推送方笔 `958e6c57`（评审 N1：「六种」→「多种 / 每一种」，`catalogue_intake.go` 头注 1 行 + `isolated_write_intake_test.go` 头注 2 行与 `t.Fatalf` 文案 1 行，零行为、自审——含在同一 tip 里让全量一并覆盖，不另起一轮）；gofmt / build / vet 0、清点零差、admin-web tsc 0 / run-tests 254 / vite 0；22:24 占号 → 带 DSN 全量 **115 ok / 0 FAIL / 16 无测试 / 0 cached**（22:24:35→22:26:51，136 s）；`-v` 探针 `TestMalformedRegistrationRefusalCarriesTheReasonAsDetail` 四子测 PASS、`cmd/parcel-api` `TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase` PASS 非 SKIP → `ls-remote` 核 `1f569998` 未动 → 22:27 `push 958e6c57:main` 成，**远端 main = `958e6c57`**（CI run 35108644386 起跑）→ 共享树 ff → 释号广播（SHA 对照、通道 4 拆树比法——其 8 件对 main 只差推送方那 4 行改字）。
- **SHA 对照**：08 `f6f1597c→db8a3ee4` / `62a1aea5→86edd4da`；09 `55d2d81d→fab71faf` / `d5396299→2c390907` / `bee86b4d→1f569998`；11 `a1d8022c→c407d05a` / `7fec23ce→dc426d3b` / `564f813d→d79bb2a1` / `a78d465c→5100fd56`；推送方 `b185c0f2` / `4ec765f2` / `958e6c57`；无清点笔。
- **簿记本笔**（纯 .md 自审，在 `958e6c57` 之上）：票 11 进 main 记录 + 评审代落三格 + 处置；spec 11 行 → resolved（ADR-0140 冲突归 owner）；本节与上节补记。→ `ls-remote` 核 `958e6c57` 未动 → push → 共享树 ff。
- **收尾**：分支 `mcp5-adminweb08` / `mcp6-adminweb09` / `mcp4-adminweb11` → `merged/`、远端删（各票文件对 main 零 diff，11 只差推送方改字笔）；树 `idp-land0809` / `idp-land11`（先删 `node_modules` 真目录再 `remove`，不加 `--force`）与 `idp-land0911`（前任弃树：干净、九笔与进 main 的九笔 patch-id 逐一同，内容已全部覆盖）由推送方拆；作者树 `idp-parcel-mcp6-adminweb09`（通道 6）/ `idp-parcel-mcp4-adminweb11`（通道 4）归作者按广播比法拆，拆完推送方改前缀。
- **上节未量的 CI 补记**：`19047d51` run 35092371232 / `dbe989cc` run 35092859307 / `cadd2d46` run 35097391560 / `e5ff0f20` run 35098103314 皆 **success**。
- **前沿**：ready 2——**10**（参与方身份 / 关系 / 身份停用三册逐字段表单）与 **12**（参与方修订历史读口 + 抽屉历史区），Blocked by 09 已解；两票前端半边都落 `BusinessPartiesPage.tsx`，先后做——12 的 Go 半边（读口 + 端点）可与 10 并行，前端半边等 10 进 main；下一轮点名派。needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票等 ADR-0139。**归用户**：ADR-0139 → Accepted 与否；**票 11 的 `detail` 格与 ADR-0140 草案 Decision 三「不带 detail」相反**，0140 接受前必裁一个（收窄 Decision 三 / 收进 `httpapi` 时删格），票面判断项 1 两条出路写清；`docs/ux/commercial-party.md` 仍 untracked、无人认领、未动；`pages/party` 三处收口项（`statusBadge` 两份、「参与方册查无此身份」同词两处、`legal-entity-list.ts` 比较器未切 `list-order.ts`、两列表模块筛选码类型两种形状）记在票 09 处置，等 10 / 12 之后一张收口票。
- **本节五数**（20:5x–22:3x，含补记段）：CI **main 绿/总 1/1 已完 + 1 在跑**（`1f569998` success；`958e6c57` run 35108644386 未完）· 集成时延——09 完工报 21:03 → 进 main 22:15 **约 72 分**（其中 21:03→21:30 约 27 分是前任派评审前的等待、21:3x→22:0x 约 30 分是推送方会话断的空档）；08 完工报 21:23 → 22:15 约 52 分；11 完工报 21:23 → 22:27 约 64 分（其中评审首派无人认领约 40 分）；中位数约 64 分（3 票）· 重放笔数 **9**（08 两 + 09 三 + 11 四，不含推送方三笔）· 重复开发 0 · 会话重置 **3**（推送方 21:3x→22:0x 一次；通道 5 一次——08 作者 20:58 后断；通道 4 一次——11 作者两笔推完后断，票面自述）· 非作者评审缺席 0（三票各有非作者评审；11 首派未被认领、改派后 12 分交）

### 22:3x–22:4x 同一会话：簿记 `f6010739` 进 main → 收尾 → 点名派 10 / 12

- **push**：簿记笔 `f6010739`（票 11 进 main 记录 + 评审代落、spec 11 行、tasks.md 两节）`ls-remote` 核 `958e6c57` 未动 → 22:32 `push f6010739:main` 成，**远端 main = `f6010739`**；共享树 ff。
- **收尾**（按 git 核，不读自报）：通道 5 22:2x、通道 4 22:4x、通道 6 22:4x 各按「先比内容」拆了自己的树（三棵都撞 `node_modules` 超长路径 `Directory not empty`，都按 parallel-sessions 配方三样逐查后清壳；通道 6 顺手杀了一只 20:58 起的孤儿 vite dev server pid 47328 / esbuild 44816、释 5209）；通道 6 自己把 `mcp6-adminweb09` 改 `merged/`；推送方改 `mcp5-adminweb08` / `mcp4-adminweb11` → `merged/`，三支远端删；三棵推送方树 `idp-land0809` / `idp-land11`（先删 `node_modules` 再 `remove`）/ `idp-land0911`（前任弃树，九笔 patch-id 与进 main 九笔逐一同后 `remove`，撞 `node_modules` 同案、清壳）拆净，`worktree list` 只剩共享树。
- **点名**（22:36 广播 2–6，截止 22:47）：**5 应答**（2 / 3 / 4 / 5 / 6 皆「空闲 · 地盘无 · 余量充足」；通道 3 今天首次应答）。派单第 0 步：`branch-state.ps1 -Path apps/admin-web/src/pages/party` 与 `-Path internal/partycommercial` 均「无 main 之外的提交、无未提交现场」，在途分支零。
- **派单**（22:38–22:39）：**10 → 通道 6**（`task-23ba5469`，`mcp6-adminweb10` 基 `f6010739`；09 作者，`BusinessPartiesPage.tsx` 唯一写入方，只动登记签段 + 列表状态上抬；参照 02 表单与 `isolated_write_intake.go` 三份 `*Document`；演示口 8090 那只进程不含 11 的 detail 格、要看就另起端口）；**12 → 通道 4**（`task-826da937`，`mcp4-adminweb12` 基 `f6010739`；三段——(a) Go 半边照 03 的 `53238188` 搬一册、(b) `api.ts` + 时间线判读能泛化 `legal-entity-revisions.ts` 就泛化、(c) 抽屉接线**等 10 进 main 广播后**再碰 `BusinessPartiesPage.tsx`；(a)(b) 先完可先报）。两票地盘不交；2 / 3 / 5 留作评审。
- **前沿**：在途 10（6）/ 12（4）；ready 0；needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票等 ADR-0139。**归用户**同上节（ADR-0139；票 11 vs ADR-0140 Decision 三；`docs/ux/`）。推送方接下来：等两份完工报 → 点名派评审 → 10 先进 main 并广播 SHA 让 12 接 (c) → 12 进 main → `pages/party` 收口票（09 处置里三条）视余量立。

### 22:5x–23:1x 同一会话：12 (a)(b) 完工 → 评审 ← 通道 2 → `44c4ebe2` 进 main；10 完工 → 评审派通道 5、八笔预叠

- **12 作者侧**（通道 4，`task-826da937`，22:39→23:0x）：七笔全推——`ee8481b0` 认领 / `d0cb97d0` ports + pcpostgres / `fa459894` 端点 / `90a8ee57` 装配 / `67e24804` TS（判读**泛化**成 `revision-timeline.ts`，`legal-entity-revisions.ts` 改薄适配导出名不动）/ `f435e8d9` 清点 / `0f4ff6c8` 完成记录；占号两次（pg 用例、12 反向依赖包 + architecture 带 DSN，PASS 1608 / SKIP 0）皆按「占号—释号成对」广播；**未碰 `BusinessPartiesPage.tsx`**（派单裁 (c) 等 10）。完工报 23:0x。
- **12 评审**：`task-244a5040` → 通道 2（22:5x 派，23:09 交，14 分）：**Standards 0 / 0 · Spec 0 / 1**（票面把 `isolatedReadAdmittedPatterns` 注释的三条归给 ADR-0091 决定一，实为 ADR-0078 Decision 二 / 表内既有口径；代码不需改，(c) 那笔顺手改票面）；真红核过、端点五用例与 `cmd/parcel-api` 六条无 DSN 探针 PASS。代落票 12 Comments。
- **12 (a)(b) 进 main**（推送方与评审并行重放）：`%TEMP%\idp-land12` detached `44b6f41b`，七笔 cherry-pick 零冲突、patch-id 逐笔同、19 件对作者 tip 零 diff；gofmt / build / vet 0、清点在 tip 重生成零 diff、admin-web tsc 0 / run-tests 257 / vite 0；23:05 占号 → 带 DSN 全量 **115 ok / 0 FAIL / 16 无测试 / 0 cached**（23:05:05→23:07:23）、探针 pg 两条 + `cmd/parcel-api` 真库 PASS 非 SKIP → 23:08 释号 → 评审到 → `ls-remote` 核 `44b6f41b` 未动 → 23:13 `push 44c4ebe2:main` 成，**远端 main = `44c4ebe2`** → 共享树 ff → 广播（SHA 对照；通道 4 分支留着做 (c)，等「10 进 main」广播后 rebase）。**部分票先进 main**：(a)(b) 自足且绿（端点可用、抽屉仍如实写「读口尚未建立」），不让已过门的七笔陪 (c) 等 10——与上节「按评审门切批」同一条。
- **10 作者侧**（通道 6，`task-23ba5469`，22:39→23:1x）：八笔全推 `ee15cc65` → `a66df9b6` / `073f577c` / `cda11680`（三份 `*-form.ts` + test + `identityKindLabels`）→ `ad59ae97`（三份表单组件 + `party-registration-fields.tsx` 共用格）→ `5938948c`（页面接线、列表状态上抬）→ `14bb61ae`（作者自审去跨文件计数）→ `916030c7`（完成记录）；tsc 0 / run-tests 276 / vite 0；演示 8090 直投三步 201；判断项 6 不做（抽屉签名为 12 冻结）；跟进三条归收口票（02 表单同形副本切共用格、`chipClass` 抬共享、`ReferencePicker` 交出行）。完工报 23:1x。
- **10 评审**：`task-9e976583` → 通道 5（23:1x 派，至 23:40；通道 2 当时在评 12）。推送方并行：`%TEMP%\idp-land10` detached `44c4ebe2`，八笔 cherry-pick 零冲突、patch-id 逐笔同、13 件对作者 tip 零 diff、与 12 (a)(b) 文件零重叠；gofmt / build / vet 0、清点零差、admin-web tsc 0 / **run-tests 279**（254 + 12 的 3 + 10 的 22）/ vite 0；tip `c5717960` 等评审无阻断后带 DSN 全量一次再推。
- **簿记本笔**（纯 .md 自审，在 `44c4ebe2` 之上）：票 12 Comments 追评审代落 + (a)(b) 进 main 记录（Status 仍 in-progress）；spec 12 行 → in-progress（(a)(b) 进 main）、10 行 → in-progress（评审中）；本节。
- **前沿**：在途 10（评审中，tip 预叠 `c5717960`）/ 12 (c)（等 10 进 main）；ready 0。**归用户**同上。

### 23:1x–23:2x 同一会话：10 评审 ← 通道 5 → `b1efbfdf` 进 main → 解 12 (c)

- **簿记 `7b699398`** 先进 main（23:16；12 (a)(b) 票面 + spec + 上节）→ 10 的预叠树 `rebase --onto 7b699398 44c4ebe2`，零冲突、patch-id 逐笔仍同、tsc 0 / 279 / vite 0。
- **10 评审**：`task-9e976583` → 通道 5（23:1x 派，报 23:36 为其时钟；本机 23:20 收到）：**Standards 0 / 5 · Spec 0 / 2，七条皆判断项**（最重：三表单壳 Duplicated Code；停用非参与方种类后表单内册面不重读）；`run-tests` 276 复现、真红核（`effectiveEndsAt` 写 null → 275 / 1 fail）。代落票 10 Comments；七条 + 09 的三条合成 **`pages/party` 收口票清单**（写在票 10 处置），12 (c) 进 main 后立票。
- **10 进 main**：tip `b1efbfdf`（= `7b699398` + 八笔）23:21 占号 → 带 DSN 全量 **115 ok / 0 FAIL / 16 无测试 / 0 cached**（23:21:09→23:23:29）、`cmd/parcel-api` 真库探针 PASS → `ls-remote` 核 `7b699398` 未动 → 23:23 `push b1efbfdf:main` 成，**远端 main = `b1efbfdf`** → 共享树 ff → 23:24 广播（释号、SHA 对照、**「10 进 main = b1efbfdf」解 12 (c)**、通道 6 拆树比法、票 12 面末两节位置）。
- **簿记本笔**（纯 .md 自审，在 `b1efbfdf` 之上）：票 10 进 main 记录 + 评审代落 + 处置（收口清单八条）；spec 10 行 → resolved；本节。
- **前沿**：在途 12 (c)（通道 4，接抽屉）；ready 0；收口票待立（12 (c) 进 main 后）。**归用户**同上。

### 23:2x–23:4x 同一会话：12 (c) 完工 → 评审 ← 通道 2 → `29e5117b` 进 main → 第二轮五票收齐 → 立收口票 13

- **12 (c) 作者侧**（通道 4）：收到 23:24「10 进 main = b1efbfdf」广播后 `rebase origin/main`（七笔被识为上游已有而跳过）→ `5ee87a88`（`BusinessPartiesPage.tsx` 新 `BusinessPartyRevisionHistory`，照 03 的 `LegalEntityRevisionHistory`；只动这一文件）+ `c280ea0f`（票面 (c) 完成记录 + S1 归因改口）→ `--force-with-lease` 推；tsc 0 / run-tests 279 / vite 0；演示形态浏览器未验如实记。完工报 23:2x。
- **12 (c) 评审**：`task-2510f5fc` → 通道 2（23:2x 派，23:34 交）：**Standards 0 / 1（判断项：`key` 带修订号在今天的 `useRegisterList` 重取做法下冗余无害）· Spec 0 / 0**；实跑三道门；组件与 03 逐态同形；S1 改口两处到位。代落票 12 Comments。
- **12 (c) 进 main**（与评审并行重放）：`%TEMP%\idp-land12c` detached `e86ecfea`，两笔 cherry-pick 零冲突 `5ee87a88→c7afefda` / `c280ea0f→29e5117b`（patch-id 同、两件对作者 tip 零 diff）；gofmt / build / vet 0、清点零差、admin-web 0 / 279 / 0；23:32 占号 → 带 DSN 全量 **115 ok / 0 FAIL / 16 无测试 / 0 cached**（23:32:10→23:34:30）、探针 PASS → 23:35 释号 → 评审到 → `ls-remote` 核 `e86ecfea` 未动 → 23:35 `push 29e5117b:main` 成，**远端 main = `29e5117b`** → 共享树 ff → 广播（SHA 对照、通道 4 拆树比法、「第二轮五票全部进 main，今晚不再派新活」）。
- **簿记本笔**（纯 .md 自审）：票 12 Status → resolved + (c) 评审代落 + (c) 进 main 记录（N1 与作者「法人页是否同病」并入收口第 ⑨ 条）；**立票 13**（chore，ready-for-agent：`pages/party` 收口九条——10 的 N1–N5 / S1 / S2、09 的 N3 / N4 / S3、12 (c) 的 N1，每条独立成笔可分人，完成判据「既有用例零改动仍绿 + 四个共用件各只剩一份定义」）；spec 12 行 → resolved、新 13 行、范围「第二轮」补「五票已全部进 main」；本节与五数。
- **收尾**：`idp-land12c` 拆（先删 node_modules）；`mcp4-adminweb12` 待通道 4 拆树后改 `merged/`、远端删。
- **前沿**：ready 1——**13**（收口，九条可分人；明天点名派，不赶今晚）；在途 0；needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票等 ADR-0139。**归用户**：ADR-0139 → Accepted 与否；**票 11 的 `detail` 格 vs ADR-0140 草案 Decision 三**（接受 0140 前必裁一个）；`docs/ux/commercial-party.md` 仍 untracked、无人认领、未动。
- **23:4x 追记**：用户经队列「继续」→ 23:43 点名（截止 23:46）5 应答——**通道 3 余量充足；2 / 4 / 5 / 6 皆自报「将满」不接 90–120 分长票**（各自今天做完 08+10 评审 / 11+12 全程 / 三轮评审 / 09+10 全程），如实计。派单第 0 步 `branch-state.ps1 -Path apps/admin-web/src` 无 main 之外提交、无未提交现场。**13 → 通道 3**（`task-41352358`，`mcp3-adminweb13` 基 `8608aa33`；一人一支九条逐笔推、半程报一次；顺序建议 6 → 3 → 8 → 1/2/4/5/7 → 9；2 / 5 留短评审、6 留答问）。12 (c) 评审全文到后补进票 12 与票 13 第 9 条（`8608aa33`）。前沿：在途 13（3）；ready 0。
- **本节五数**（22:0x–23:4x 全会话）：CI **main 绿/总 8/8 已完 + 1 在跑**（`1f569998` / `958e6c57` / `f6010739` / `44b6f41b` / `44c4ebe2` / `7b699398` / `b1efbfdf` / `e86ecfea` 皆 success；`29e5117b` run 35116364710 未完）· 集成时延——08 / 09 / 11 见上节（中位约 64 分，含前任断档）；本节三段：12 (a)(b) 完工 23:0x → 进 main 23:13 **约 10 分**；10 完工 23:1x → 进 main 23:23 **约 10 分**；12 (c) 完工 23:2x → 进 main 23:35 **约 8 分**（评审与重放并行，评审到即推）；全会话六段中位数约 30 分（6 段）· 重放笔数 **26**（08 两 + 09 三 + 11 四 + 12 七 + 10 八 + 12 (c) 两；不含推送方五笔簿记 / 注释）· 重复开发 0 · 会话重置 **3**（上节所记；本节内零——通道 1 自 22:0x 起一任到底，2 / 4 / 5 / 6 全程在线）· 非作者评审缺席 0（六段各有非作者评审：通道 2 四次、通道 6 一次、通道 5 一次）

## 2026-09-20 10:5x 通道 1 新会话（用户先用自带 `ask_question` 验通道，再令「调 idp-mcp-1 的 check_messages 开始监听」；首条消息问「`D:\tops\idp-parcel-mcp3-adminweb13` 现在全部完成可以 replay 了吗」→ 答不能 → 用户「全面完成，并 replay」→ 两次「继续」）

### 10:5x–11:1x：量 mcp3-adminweb13 → 答「不能 replay」→ 接手收尾第 4 / 9 条 + 票面

- **量现场**（只读 git，不读自报）：分支 `mcp3-adminweb13@ef007b22` = origin，基 `8608aa33`；九条里 6 / 3 / 8 / 1 / 5+2 / 7 六笔已成（09-17 00:01–00:30，`92ca5332` 提交信自述通道 3 会话中断于第 5 条半途、通道 4 接手补齐）；**第 4 条 `chipClass` 做了未提交**（4 M + 1 ??，`components/registration/chip.ts` 中文正常非 `Set-Content` 乱码）；**第 9 条未开始**（`BusinessPartiesPage` 历史区仍 `key` 带修订号、`GroupLegalEntitiesPage` 无 key，两页未取齐，抽不抽未判）；票面 in-progress、无完成记录 / 钉 SHA / 评审。工作树实跑 tsc 0 / run-tests 289 / vite 0。`list_sessions`：2–6 心跳全停在 09-16 23:4x 之后，作者树无人在写。
- **接手**（用户令「全面完成」）：`95b5d967` 第 4 条——逐字核对后原样收入（只加提交）；`d1926646` 第 9 条——判「抽」：两页历史区抽成 `pages/party/RevisionHistorySection.tsx`，两册各交模块级 `RevisionHistoryRegister`（subject / endpoint / load / echoedSubjectId / timelineOf / noteOf）；判「要 revision 依赖」：第 6 条后 `useRegisterList` 重取保留旧答案、抽屉不再卸载，同对象登下一笔后行 revision 变而 subjectId 不变，只按 subjectId 记依赖会显旧链——12 (c) 评审说的「key 冗余」在第 6 条落地那刻失效；共用件 effect 依赖 `[load, subjectId, revision, reloadKey]` 一处守住，两页 `key` 撤下，文案逐字保住，两页 −167 行；三道门 0 / 289 / 0、既有用例零改动。`5421fc7e` 票面完成记录 + Status resolved（九条落点、完成判据三条钉 `d1926646`、判断项五条：第 3 条「不裁空白」那半不做的理由、`suggested*Revision` 四份同形只记不动、其它模块十一处 `chipClass` 归后续）。每笔推分支。
- **点名** 11:17（截止 11:21）→ 通道 6 / 3 应答（皆「空闲 · 地盘无 · 余量充足」）；3 是作者通道不能评 → **`task-3faeb437` → 通道 6** 两轴评审钉 `5421fc7e`，至 11:50；单发通道 3 说明不派它、树由推送方拆。

### 11:1x–11:3x 同一会话：重放 → 全量 → 用户两次「继续」→ `090e250e` 进 main → 簿记

- **重放**：`%TEMP%\idp-land13` detached `041adc37`（= 远端 main，比分支基只多一笔 tasks.md），十笔 cherry-pick 零冲突 `9848d60a→4a75c5f5` / `b3579c45→3b8e777f` / `b6e708b8→75684633` / `6e4d09dc→a3b06e53` / `71c6d932→af057c94` / `92ca5332→29b56bd7` / `ef007b22→fe6d774c` / `95b5d967→fc8c602e` / `d1926646→fb31773e` / `5421fc7e→090e250e`，`patch-id --stable` 逐笔同、30 件对作者 tip 零 diff；清点重生成 porcelain 空、`gofmt -l` 空、build / vet 0；admin-web 全新 frozen install 5 s → tsc 0 / run-tests 289 / vite 0。11:21 占号 → 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**，退出码 0（11:21:43→11:24:03，140 s）→ 11:24 释号。
- **评审门**：等通道 6 期间用户 11:25 / 11:26 两次经队列「继续」（台账 `task-3faeb437` 仍 pending，通道 2–6 心跳在 11:24 释号广播上刷新——活着、在评，未 report）。按用户令先推：推前自审（逐笔提交信、三处共用件正文、四共用件 grep 各一、零 diff、三门 + 全量绿），票面如实写「不算非作者评审」；通道 6 结论到后补记，有阻断另立票于 main 上修。
- **push**：`ls-remote` 核 `041adc37` 未动 → 11:27 `push 090e250e:main` 成（`$sha` 字面写死，不读 HEAD），**远端 main = `090e250e`**，CI run 35486623746 起；共享树 `merge --ff-only` 同 SHA（树上只两块 untracked `.scratch/admin-web-ux-alignment/`、`docs/ux/`，零重叠）→ 广播（SHA 对照、评审在途、分支待改 `merged/`）。**先 push 再簿记**。
- **簿记本笔**（纯 .md 自审，在 `090e250e` 之上）：票 13 进 main 记录 + 评审门如实记；spec 13 行 → resolved（后续候选三条）+ 范围「第二轮」补「13 亦已进 main」；本节。
- **上节未量的 CI 补记**：`29e5117b` run 35116364710 **success**；`8608aa33` run 35117072635、`041adc37` run 35117907776 皆 success。
- **前沿**：在途 0（13 评审 ← 通道 6 结论待补记）；ready 0；needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票等 ADR-0139。后续候选（未立票）：`suggested*Revision` 四份同形折一份、其它模块十一处 `chipClass` 改导入、02 载荷「不裁空白」判据（先改 02 判据再改码）。共享树 untracked：`.scratch/admin-web-ux-alignment/`（spec + 三票，无人认领）、`docs/ux/commercial-party.md`（自 09-16 起）。**归用户**：ADR-0139 → Accepted 与否；票 11 `detail` 格 vs ADR-0140 Decision 三；`docs/ux/` 与 `admin-web-ux-alignment` 归属。
- **本节五数**（10:5x–11:3x）：CI main 绿/总 **3/3 已完 + 1 在跑**（`29e5117b` / `8608aa33` / `041adc37` success；`090e250e` run 35486623746 未完）· 集成时延——13 代码齐 11:13（`d1926646`）→ 进 main 11:27 **约 14 分**（含点名 4 分 + 全量 140 s；评审未等，1 票）· 重放笔数 **10**（不含本簿记笔）· 重复开发 0 · 会话重置 0（本会话 10:5x 起一任到底）· 非作者评审缺席 **1**（用户令先推；通道 6 在途，到后补记则改 0）

### 11:3x–11:5x 同一会话：通道 6 评审到（两轴 0 阻断）→ 用户「那怎么办，请你自决」→ N1 注释笔 + 立票 14 + 簿记进 main

- **评审 ← 通道 6**（`task-3faeb437`，11:4x 分两条到，隔离树 `idp-review-13`）：**Standards 0 / 3**（N1 五处注释跨文件活计数 + 「主语两字」误；N2 `deactivationTargetsOf` 三件生产侧无调用方；N3 `kindRegister<Body>` 擦型）· **Spec 0 / 2**（S1 第 2 条分派表仍两张、完成记录未提；S2 第 9 条 JSX 换行空白之变）；逐条 1–9 ✓、完成判据 ✓、判断项五条核过成立。全文代落票 13 Comments。到达时 `090e250e` 已在 main 五分钟——「用户令先推」那一格如实记，评审结论没改变要推的东西。
- **自决**（用户 11:4x「那怎么办，请你自决」）：N1 是纯注释、归「不评什么」→ 推送方在 `%TEMP%\idp-land13n`（detached `56f5408f`）改成一笔 **`a0b16cd7`**（五处 + `register-list.ts` 头同类一处；`git diff -U0` 滤注释行后空、无 CR；junction 借共享树 `node_modules` 跑 tsc 0 / run-tests 289，跑完摘除）；S2 只记；**N2 / N3 / S1 + 13 判断项里的 `suggested*Revision` 四份同形、其它模块十一处 `chipClass` → 立票 14**（chore，ready-for-agent，四条可分人；02 载荷「不裁空白」判据列入「不做」并注明先问 owner）。
- **簿记本笔**（纯 .md 自审，叠在 `a0b16cd7` 上）：票 13 评审代落 + 处置；新票 14；spec 13 行改口（评审到、N1 笔）+ 新 14 行；本节。→ `ls-remote` 核 `56f5408f` 未动 → `push <本笔>:main` → 共享树 ff → 广播 + 单发通道 6（谢、拆树、report_task 由它自己收）。
- **收尾**：`idp-land13n` 推完拆；`idp-review-13` 归通道 6 拆。
- **前沿**：ready 1——**14**（下一轮点名派；不赶）；在途 0；needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票等 ADR-0139。**归用户**同上节。
- **本节五数**（11:3x–11:5x）：CI main 绿/总 未量（`090e250e` / `56f5408f` run 在跑或刚起）· 集成时延——本节无新代码票（N1 注释笔按「不评什么」自审直推）· 重放笔数 0 · 重复开发 0 · 会话重置 0 · 非作者评审缺席 **0**（13 的非作者评审已到、两轴 0 阻断；上节那格的「1」是时序——先推后到——不是缺席，两节合看为 0）

### 11:4x–11:5x 同一会话：用户「按你的建议开始」→ 点名 → 派票 14 两支

- **CI 补记**：`090e250e` run 35486623746 **success**、`56f5408f` run 35486800454 **success**；`e16497de` run 35487229654 排队。通道 6 的 `task-3faeb437` 11:4x `report_task done` 全文到，与已代落两轴一致（并建议「三条同源合一票」——即票 14）。
- **派单第 0 步**：`branch-state.ps1 -Path apps/admin-web/src`——main = origin = `e16497de`，在途分支零，地盘上无 main 之外提交、无未提交现场（共享树 5 行是两块 untracked）。
- **点名** 11:49（截止 11:53）→ **5 应答**（3 / 6 / 5 / 4 / 2，皆「空闲 · 地盘无 · 余量充足」）。
- **派单**（11:5x）：**14 第 1 / 2 / 4 条 → 通道 5**（`task-78363aa5`，`mcp5-adminweb14a` 基 `e16497de`，地盘 `pages/party` 的停用 / 登记表单纯逻辑 + 组件，票面只它一人写，第一笔认领；约 60 分、过半报）；**14 第 3 条 → 通道 4**（`task-8beff385`，`mcp4-adminweb14b` 基 `e16497de`，地盘其它模块页面十一处 `chipClass` 改导入，不碰 `pages/party` / `components/registration` / 票面；约 30 分）。两支地盘不交。**通道 6 留作评审**（熟 13 的地面），2 / 3 本轮无派单。
- **前沿**：在途 14（5 + 4）；ready 0；needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票等 ADR-0139。推送方接下来：等两份完工报 → 派通道 6 评审（两支一并）→ 重放到 tip（先 5 的三笔再 4 的，票面由推送方补第 3 条落点与 Status resolved）→ 带 DSN 全量一次 → push → 簿记。

### 11:5x–12:3x 同一会话：14 两支完工 → 评审 ← 通道 6 → `3164b7d1` 进 main → 簿记

- **完工报**：14b ← 通道 4 12:0x（`task-8beff385` done + 单发；`mcp4-adminweb14b@3b26cdfe` 一笔 11 件 +11/−82，留存无，grep 只剩 chip.ts）——推送方按 git 核：diff 除导入行只有十一段同字模板串删除；预叠 `%TEMP%\idp-land14`（detached `c0b29402`）`3b26cdfe→f1d6da55` patch-id 同。14a ← 通道 5 12:1x（进度报一次 + `task-78363aa5` done + 单发；`mcp5-adminweb14a` 五笔 `fd1a47ee` 认领 / `d29b984b` 第 1 / `3877f60f` 第 2 / `baabfb1b` 第 4 / `8ad23acf` 票面）。**第 4 条作者判「改」**：擦型藏着换种类那一帧 `loaded` 错配会抛的真缺陷（`LEGAL_ENTITY` ↔ `CUSTOMER_ACCOUNT` 互切），改成 `RegisterAnswer<K>` 由册自己结对、`selectedRegisterFor` 种类不对当没取到；负向验证 tsc 拒错配；本机无浏览器未复现。
- **评审 ← 通道 6**（`task-13121320`，12:1x 派，两轴 12:2x 分两条到）：**Standards 0 / 4**（N1 两处变更说明式注释；N2 第 4 条设计量可接受；N3 `targetsOf<K>(paired)` 与方法同名异型；N4 `chip.ts` 住 `components/registration/` 却被非登记页导入，归后续）· **Spec 0 / 2**（S1 第 4 条那次抛无自动化钉；S2 第 1 条删掉的旧用例「不拿别的册冒充」正是第 4 条出事的规则，完成记录未点出关联）；**证实真缺陷成立且是 13 `92ca5332` 引入的回归**（13 前三册各有槽位不会错配）。全文代落票 14 Comments。
- **重放**：14a 五笔叠在 14b 之上零冲突，patch-id 逐笔同、两支文件对各自作者 tip 零 diff；推送方 N1 + N3 一笔 **`3164b7d1`**（两处注释收成现状句、改名 `pairedTargetsOf`，`git diff -U0` 非注释行只改名两行；tsc 0 / 290 / vite 0，自审）；清点零差、gofmt / build / vet 0、admin-web 全新 frozen install 0 / 290 / 0、`const chipClass` 一处、`revision + 1` 实现一处。12:22 占号 → 带 DSN 全量 **115 ok / 0 FAIL / 16 无测试 / 0 cached**（12:22:17→12:24:39）→ `ls-remote` 核 `c0b29402` 未动 → 12:24 `push 3164b7d1:main` 成，**远端 main = `3164b7d1`** → 共享树 ff → 12:25 释号 + 广播（SHA 对照、两作者树比法）。**先 push 再簿记**。
- **簿记本笔**（纯 .md 自审，在 `3164b7d1` 之上）：票 14 第 3 条落点代落（通道 4 不写票面）+ 评审代落 + 处置 + 进 main 记录 + Status resolved；spec 14 行 → resolved；本节。候选未立票：配对比较抬进纯模块以 node:test 钉（S1 / S2）、`chip.ts` 上提 `components/`（N4）——等再被撞到。
- **收尾**：`idp-land14` 簿记推完拆（先删 node_modules）；作者树 `idp-parcel-mcp5-adminweb14a` / `idp-parcel-mcp4-adminweb14b` 归通道 5 / 4 按广播比法拆，拆完推送方改 `merged/`、远端删。
- **前沿**：在途 0；ready 0；needs-info 04 / 05（归 owner）；draft pp-seams/04；first-party-api 三票等 ADR-0139。admin-web-group-legal-entities 十四票里 12 张 resolved、04 / 05 needs-info。共享树 untracked 两块未动。**归用户**：ADR-0139 → Accepted 与否；票 11 `detail` 格 vs ADR-0140 Decision 三；`docs/ux/` 与 `.scratch/admin-web-ux-alignment/` 归属。
- **本节五数**（11:5x–12:3x）：CI main 绿/总 **2/2 已完 + 1 在跑**（`e16497de` run 35487229654、`c0b29402` run 35487718075 success；`3164b7d1` run 35489081750 未完）· 集成时延——14b 完工 12:0x → 进 main 12:24 **约 20 分**（等 14a 与评审）；14a 完工 12:1x → 进 main 12:24 **约 12 分**（评审与重放并行，评审到 → 全量 140 s → 推）；中位约 16 分（2 支）· 重放笔数 **6**（14b 一 + 14a 五；不含推送方 N1/N3 笔与本簿记）· 重复开发 0 · 会话重置 0（本会话 10:5x 起一任到底；2–6 全程在线）· 非作者评审缺席 0（通道 6 两轴到后才推）
