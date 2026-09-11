
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
- **归用户（新增）**：三个通道半小时内各 crash 一次（4 建树后、6 写了 20 分钟未提交、2 提交两笔后），crash 点都在读写大文件 / 跑测试之后——成因归本机环境；「每笔提交后推分支」在 2 有效、在 6 无效、在 4 未到那一步。建议把「建树后立刻提一笔认领并推」抬成开工第一步写进 `docs/agents/parallel-sessions.md`，归你拍。
