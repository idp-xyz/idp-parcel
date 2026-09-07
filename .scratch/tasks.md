
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
