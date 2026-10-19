# ADR-0140：首方 API 的机器可读契约从端点描述符生成、生成物入库并由 CI 校验一致——描述符随处理器住在各上下文 `adapters/http`，OpenAPI 文档是发布形态不是事实源；首方与集成 API 带 URL 主版本 `/v1`，操作者 API 不带；4xx/5xx 一律 RFC 9457 问题详情承载稳定错误码；兼容性以已发布的不可变契约为基线由破坏性变更检查把门；弃用走 `Deprecation` / `Sunset` 头，窗口长度是产品策略

Status: Accepted（2026-10-19 **修订后接受**：用户在 Claude Code 会话里答「接受」，按会话复核建议修订。修订要点：契约按端点表上静态声明的族分拣成首方、集成、操作者三份（Decision 一）；`/v1` 延到集成族（Decision 二）；`detail` 只随「改请求才会好」的 4xx 在场，新增 `429` 一格并据此部分停用 [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 三类表 4xx 行（限首方与集成两族的限流答复，Decision 三）；破坏判据以锁定版本的工具规则为准（Decision 五）；弃用窗口归产品策略（Decision 六）；文档匿名可读、另有集成契约入口（Decision 七）；不另开监听端口（Decision 八）；越权风险点逐条按复核建议处置。此前 Proposed——**草案**，2026-09-16。用户经 IDP 队列通道 1 先问「这个方式是我们内外 API 的科学、专业、先进的解决方案吗」，通道 1 复核后答「身份与准入层成立；作为完整方案缺六件，第一件是契约生产与版本策略」，用户令「按建议继续」；本记录据此起草，**接受与否归用户**，接受前不是依据。它填的是 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 五显式留白的那一格——「契约怎么生产、URL 主版本、public 与 internal 契约要不要拆、文档门户放哪个入口，本记录不定」。裁决能力边界：读过 [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、ADR-0139 全文，[`docs/api` 的 Go + chi + Scalar 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)全文，`internal/platform/httpapi/router.go`，`internal/parcelshipment/adapters/http/submit_shipment_request.go` 与 `cmd/parcel-api` 的 `assembleBusinessEndpoints`，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)对机制半边清点生成物的 CI 纪律那一句，[docs/README](../README.md) 对该方案稿的登记；未逐一重读其余上下文的 `adapters/http`、未核对 OpenAPI 3.1 在本仓将锁定的工具版本上的实际支持——本记录因此只裁**契约的事实源、生成与校验纪律、版本与弃用的机制、错误响应的媒体类型**，不裁任何端点的字段、不裁文档的阅读策略、不裁弃用窗口的长度）
Date: 2026-09-16

## Context

**首方 API 已经有了契约的语义，没有契约的文档。** ADR-0022 定了客户端据以行动的契约：状态码只答「有没有形成答案」、业务判别进封闭的 `outcome` 词表、不新造 `Idempotency-Key`、`!found` 统一 `404`。ADR-0139 把首方客户渠道立为产品自有的接入渠道族，并在 Decision 五写下「首方 API 有一份产品拥有的机器可读契约，是客户面端点的单一权威」，却把契约怎么生产留白。今天这份契约以什么形态存在？答案是**只以 Go 代码存在**：各上下文 `adapters/http` 里的处理器各自拥有请求解码、封闭的 `outcome` 枚举、响应结构体与传输层错误码常量；`internal/platform/httpapi` 只挂路由与无业务判断的传输件（如 `BearerToken`），刻意不带 `Method` 字段，让 405 由处理器自守。没有任何一份能交给集成方读、能让工具比对的文档。

**契约文档只能有一个事实源，而本仓已经有了一个。** `docs/api` 方案稿的主线是「以 OpenAPI 契约为接口事实源，`oapi-codegen` 生成 chi 路由胶水，处理器实现生成的接口」。那条路对一个从零开始的仓是对的；对本仓不是：装配表上的处理器全是手写的，每个都按 ADR-0022 自守 405、自写问题体、自持 `outcome` 词表，且这些形状各有传输层测试钉住。改成生成胶水要重写它们全部，迁移期间同一个 operation 同时有手写实现与 YAML 两个事实源——那正是方案稿自己在「迁移限制」里禁的事，也正是本仓红线「单一权威」要消除的重复。方案稿另一处与本仓既有裁决相冲：`chi-server` 生成的路由会把 405 从处理器自守改成由路由层回答，[docs/README](../README.md) 登记该稿时已点名这一条。

**本仓已经有一套「生成物入库、CI 校验一致」的纪律可以直接借。** 机制半边清点 `docs/product/MECHANISM-INVENTORY.md` 由 `tools/mechanism-inventory` 从代码生成，CI 每次重新生成并与提交进来的那份比对，不一致即失败。契约文档与它同构：都是从代码派生的、谁都不拥有的中间产物，都靠「生成 → 入库 → 重生成比对」守一致。方案稿的 CI 节也要求「重生成及 Git 状态检查」，两边说的是同一件事。

**错误响应的形状今天是自造的，而标准是免费的。** 处理器的问题体是 `{"error":{"code":"…"}}`；party-commercial 与 network-routing 的写口另在「改载荷才会好」的 `MALFORMED_REQUEST` 上带一格散文 `detail`，5xx 一律不带（票 admin-web-group-legal-entities/11，接受修订时的现状）。`UC-PS-001` 要求错误信息不得泄露其他客户的存在、业务量或内容。RFC 9457 的 `application/problem+json` 表达同样的信息——`type`、`title`、`status` 与扩展成员 `code`——且集成方的通用客户端认得它。ADR-0022 只禁 4xx/5xx 携带 `outcome`，不禁问题详情；`UC-PS-001` 只禁泄露性的措辞，不禁结构化的稳定码。换媒体类型不动 ADR-0022 一字。

**版本与弃用今天没有任何机制。** 路径不带主版本（`/shipment-requests`），没有兼容性基线，没有弃用信号。产品内两个客户端（管理台、一线作业）与后端按 ADR-0021 同版本发布，它们不需要这些；首方 API 的调用方是租户的客户系统，不与本产品同版本发布，**它们需要**。方案稿在版本一节把三种版本（`openapi` 规范版本、`info.version` 契约版本、URL 主版本）分开，这一条本记录照收。

**首方客户、集成客户端与操作者三族在契约上也要分开，但分法不是标签。** 方案稿建议 public / internal 两份根契约、两个 Router 两个端口。本仓各族的分界已经是结构性的：ADR-0139 Decision 三让一枚令牌只过得了一族、一种信封只铸得出一族的命令，端点属于哪一族决定了它属于哪份契约。契约的拆分因此可以**从端点表导出**，不必靠 `x-audience` 这类要构建规则去解释的元数据——方案稿自己也写明「标签本身不提供访问控制」。

## Decision

**一、契约的事实源是代码里的端点描述符，OpenAPI 文档是从它生成的发布形态。** 每个业务端点在它所在上下文的 `adapters/http` 包里、与处理器构造函数并列，导出一份**端点描述符**：路径模板、方法、请求体形状、`outcome` 封闭词表（取应用结果枚举的原名）、成功响应形状、该端点会答的传输层错误码、所属用例编号。描述符是 Go 值，由处理器包自己构造——`outcome` 词表就是那个包已有的枚举，响应形状就是那个包已有的结构体，描述符不复制第二份，只引用。`tools/api-contract` 读端点表上每个端点的描述符，按**端点表行上静态声明的族**分拣（`httpapi.BusinessEndpoint` 增一格族），生成三份 OpenAPI 3.1 文档：**首方 API 契约**（首方客户族的端点）、**集成 API 契约**（集成客户端族的端点：外部结果、监管凭证、外部资金事实及其更正）与**操作者 API 契约**（操作者族各能力面的端点）。分拣不取运行时实挂的 Intake：那随部署参数变，发行方参数未设时一律是 `UnconfiguredIntake{}`；装配测试钉住每行实挂的 Intake 属于所声明之族或为未配置。生成物落 `docs/api/generated/`，随笔提交；CI 在 tip 上重生成并与提交的那份比对，不一致即失败——纪律与机制半边清点同一条，原因也同一条：从代码派生的文档若可以手改，改过的那份对任何一笔提交都不成立。

描述符与处理器同包、同笔提交是有意的：把描述符集中到一个目录，加一个端点就要改两处，而改的人不会路过另一处。处理器包的传输层测试同时钉描述符——响应体里出现描述符没声明的字段、`outcome` 出现描述符词表没有的值，测试红。

**二、首方 API 与集成 API 带 URL 主版本 `/v1`，操作者 API 不带。** 首方渠道 Intake 在装配点接上时，客户面端点的路径模板加前缀 `/v1`；集成客户端族各口同样加——它们的调用方是报关服务商、承运商、银行的系统，与首方 API 的调用方一样不与本产品同版本发布（接受修订时它们没有外部调用方，加前缀的代价只在演示脚本）；描述符里的路径模板不含前缀，前缀由装配点按族加——同一个处理器若将来也要在 `/v2` 下服务，改的是装配表不是处理器。操作者 API 的调用方是本产品自己的客户端，与后端同版本发布（ADR-0021），主版本在那里只会制造一个永远是 `v1` 的前缀，不加。`info.version` 记契约版本、随每次契约变更递增，由生成器从描述符集合的内容摘要派生而不是人填；`openapi` 字段记规范版本。三者各自独立，不混用。

**三、4xx 与 5xx 一律 `application/problem+json`（RFC 9457）。** 成员：`type`（本产品拥有的稳定 URI，按错误码一码一 URI）、`title`（错误码的固定短语，不随请求变）、`status`、`code`（今天各处理器已有的传输层稳定码原样保留：`ACCESS_CHANNEL_NOT_CONFIGURED`、`MALFORMED_REQUEST`、`INTAKE_FAILED`、`NO_ANSWER_FORMED`、`METHOD_NOT_ALLOWED` 等，以及 ADR-0139 Decision 四命名的各族码）、`requestId`（取 `middleware.RequestID` 已经生成的那个值，是集成方与运维对账的唯一句柄）。`detail` 只随「改请求才会好」的 4xx 在场（接受修订时是 `MALFORMED_REQUEST`），内容只说调用方自己这份载荷错在哪，是散文，客户端不据此分支——要可判别的理由得在用例侧立封闭枚举；`403`、`404` 与 5xx 一律不带：前两者受探针同答约束（ADR-0055 Decision 四、ADR-0022 `!found` 一条），后者是依赖故障的内部原文。三族同用这条边界，它沿用票 admin-web-group-legal-entities/11 已落的那一格；只说调用方自己的载荷，不泄露任何别的客户，不违反 `UC-PS-001` 那条约束。ADR-0022 对 4xx/5xx 不携带 `outcome` 的规定不变；问题详情不是业务答案，它只说明为什么没有 `outcome`。写问题体的辅助函数从各处理器包收进 `internal/platform/httpapi`：它是纯传输件，不含业务判断，符合 ADR-0072 Alternatives 对 `platform` 的界定；错误码常量仍归各处理器包，辅助函数只收码不定码。

**`429` 是唯一按 `Retry-After` 重发即正当的 4xx**（接受修订新增）。限流答 `429` + `Retry-After`（秒）+ 问题详情，码 `RATE_LIMITED`（等一会）与 `QUOTA_EXCEEDED`（等下个周期或找租户运营提额）两格恢复动作不同、不合并。ADR-0022 三类表 4xx 行「调用方的错，重发不会改变结果｜出队，交给人，不自动重发」对 `429` 据此**部分停用**，**适用场景**：首方与集成两族的限流答复；其余 4xx 照旧。限流放在哪一层、键与取值归票 `first-party-api/02`。

**四、请求校验留在 Intake 与处理器里，不加契约校验中间件。** 方案稿推荐 `nethttp-middleware` 按契约验证传入请求。本记录不采：路由层不知道渠道与业务（ADR-0055 Decision 一「路由层不知道渠道这个概念，另设闸就是同一问题的第二处权威」），一份按描述符校验请求的中间件会成为每个处理器已有校验之外的第二处权威，两处对同一个请求的判断不一致时没有规则说谁算数。处理器的传输层测试钉描述符（Decision 一末句）是同一目标的另一条路：不在运行时多一道闸，而在测试期证明处理器与它自己声明的形状一致。

**五、兼容性以已发布的不可变契约为基线，由破坏性变更检查把门。** 首方 API 契约每次对外发布，生成物连同其 SHA-256 归档为不可变基线（发布标签指向的那一份，不是开发分支的最新文件——方案稿这一条原样收）。CI 对每笔改动跑破坏性变更检查（`oasdiff breaking` 或等价工具，锁定版本），与**最近一次已发布基线**比对；判为破坏的变更只能进下一个主版本，或附一份显式迁移计划并由 owner 放行。什么算破坏，以锁定版本的破坏性变更检查工具的规则为准（方案稿版本演进原则那张表只作参考，它不是权威），本记录加一条**本仓特有的**：给某端点的 `outcome` 词表**新增取值**在本记录下**不是**破坏性变更，条件是契约文档写明「客户端遇到未识别的 `outcome` 必须视为已形成答案但不可自动处置，交人」——这是 ADR-0022 「4xx 出队交人」纪律对未知答案的自然延伸；删除或改名 `outcome` 取值是破坏。首版发布前没有基线，首次发布显式标记为首个基线。

**六、弃用走标准头，窗口长度是产品策略。** 端点或字段进入弃用期时响应带 `Deprecation` 头（RFC 9745）与 `Sunset` 头（RFC 8594），契约文档在该 operation 上标 `deprecated: true` 并链接迁移说明；`Sunset` 之后该端点在下一个主版本里不存在，在当前主版本里继续答到 `Sunset` 为止。**窗口长度是开发方对 API 调用方的产品承诺，属产品策略**（[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 分界检验：不看任何一个租户就能回答），不是租户取值，不进试点参数登记册；随契约文档发布，取值是主版本进入弃用后至少 12 个月。这个值写进契约文档之前，不弃用任何已发布端点。

**七、文档门户是发布产物的一部分，由 `parcel-api` 提供。** 首方 API 契约的 JSON、集成 API 契约的 JSON 与一页自托管的 Scalar API Reference（锁定版本、随镜像打包、不引用浮动 CDN）由 `parcel-api` 在 `/docs`、`/openapi/v1.json` 与 `/openapi/integration-v1.json` 提供；操作者 API 契约随 `apps/admin-web` 发布、不由 `parcel-api` 对外提供。文档页面**匿名可读**：契约不含任何租户数据，「谁能读文档」与「谁能调 API」是两道各自独立的门，租户要收紧就在部署层加前置代理；文档登录态不成为任何 API 的调用凭证（方案稿文档会话一节的原句）。生产页面只读；沙箱页面允许发起测试请求，沙箱的 `servers` 由 [ADR-0141](./0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md) 定。

**八、不做的，逐条写明。**

- 不采 `oapi-codegen` 的 `chi-server` / `strict-server` 生成路由胶水，不采 Huma、不采 Swaggo 注释——三者都会让契约文档与手写处理器各成一个事实源，或让 405 从处理器自守迁到路由层。
- 不为契约新建 `internal/transport/*` 目录——端点按 ADR-0018 落位结论住在各上下文 `adapters/http`，描述符跟着住。
- 不在本记录定任何端点的字段、任何 `outcome` 词表的内容——那些归各上下文 owner 与其用例。
- 不定 SDK 是否生成、生成哪门语言——从契约生成客户端是本记录的自然下游，但它是一项交付物决定，另立记录或票。
- 不为文档另开监听端口（接受修订）：文档与契约各入口与业务端点同一监听，不入端点表、不带 Intake；要拆由部署层做。

## Consequences

- 每个入表上下文的 `adapters/http` 增一份端点描述符（与处理器同包、同笔），传输层测试增「响应与描述符一致」一格；新增端点从此必须同时给描述符，否则生成物比对红——这是把「加端点要补文档」从纪律变成结构。
- 新增 `tools/api-contract`（生成器）与 `docs/api/generated/`（首方、集成与操作者三份契约 JSON）；CI 增「契约生成物一致」一步，与机制半边清点那一步并列。`docs/api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md` 在 [docs/README](../README.md) 的身份仍是参考——本记录收了它的版本三分法、不可变基线、破坏性变更门禁、文档会话与凭证分离、自托管不引 CDN 五处；否决了它的生成路线、双 Router 双端口作为结构前提、契约校验中间件三处。
- 各处理器的 `writeProblem` 收进 `internal/platform/httpapi`，问题体媒体类型改为 `application/problem+json`、成员按 Decision 三；既有传输层测试里断言问题体形状的那些随之改写。这是对**尚未对外发布**的形状的改动，没有集成方要迁移；发布之后再改它就是破坏性变更。
- 首方渠道 Intake 接上时（ADR-0139 Decision 四），客户面端点在装配点加 `/v1` 前缀；`cmd/parcel-api` 的装配测试对首方族端点断言前缀、对操作者族端点断言无前缀。
- `parcel-api` 增 `/docs`（自托管 Scalar 页，版本锁定、随镜像打包、不引浮动 CDN）、`/openapi/v1.json`（首方契约 JSON）与 `/openapi/integration-v1.json`（集成契约 JSON）三个只读入口（Decision 七）；它们不是业务端点，不入 `assembleBusinessEndpoints` 的端点表、不带 Intake，装配测试断言两点在、且文档登录态换不到任何 API 的调用凭证。操作者契约 JSON 随 `apps/admin-web` 构建产物发布，`parcel-api` 不供。阅读策略按 Decision 七匿名可读。
- 首次对外发布前要过三道：锁定工具链版本并验证 OpenAPI 3.1 生成物在 Scalar、`oasdiff` 的锁定版本上可读（风险点 1）；显式标记首个基线；弃用窗口的取值写进契约文档（Decision 六；写进之前不得弃用任何已发布端点）。
- ADR-0022 正文不改，其三类表 4xx 行对 `429` 的适用由 Decision 三部分停用，ADR-0022 的 Status 行与 Links 各加一条前向指针；ADR-0139 Decision 五留白的那一格由本记录填。
- 首方族就位后，客户面提交口的路径变为 `/v1/shipment-requests`，`scripts/demo-seeds/submit-one-shipment.sh` 与演示动线脚本随之改。

## Alternatives considered

- **契约先行：手写 OpenAPI，`oapi-codegen` 生成 chi 路由胶水与严格接口，处理器实现接口。** 否决：装配表上的处理器已按 ADR-0022 各自成型并有测试钉住，迁移期间同一 operation 有两个事实源；`chi-server` 让 405 由路由层回答，与 `httpapi.BusinessEndpoint` 刻意不带 `Method` 的裁决相反；且它把请求解码从 Intake 缝里挪走——Intake 缝是 ADR-0055 立的替换点，缝的两侧不能换人。
- **Huma（代码优先自动生成 OpenAPI）。** 否决：同样要把处理器改写成 Huma 的注册形式，收益与上一条同、代价与上一条同；它默认导出 3.1 这一点本记录已经拿到了，不需要为此换框架。
- **Swaggo 注释。** 否决：注释是没有类型检查的第二份声明，与处理器漂移时任何东西都不会红——正是 AGENTS 对「跨文件引用」那条判语的同一形状。
- **手写 OpenAPI，不生成任何东西，靠纪律保持一致。** 否决：本仓对「从代码派生的文档可以手改」的病已经付过代价，机制半边清点改成生成物就是为此；契约文档没有理由走回去。
- **OpenAPI 3.0.3 作基线（方案稿的选择）。** 否决：3.0.3 没有顶层 `webhooks`，ADR-0142 的出向事件要靠它进同一份契约；3.1 与 JSON Schema 2020-12 对齐，`outcome` 封闭枚举与可空字段的表达少一层翻译。代价是工具支持要逐个核实，列为风险点 1，核不过再退。
- **保留 `{"error":{"code"}}` 自造形状。** 否决：它与问题详情表达的是同一组信息，而后者是通用客户端认得的标准，切换成本只在尚未发布的今天为零。
- **用请求头或媒体类型参数做版本（`Accept: …;v=2`）。** 否决：首方 API 的调用方是租户客户的集成工程师，URL 主版本是文档、网关与 SDK 都看得见的兼容边界，头里的版本三处都看不见。
- **操作者 API 也带主版本。** 否决：它与客户端同版本发布，主版本只会是一个永远不变的前缀。
- **用契约校验中间件在运行时拒不合规请求。** 否决：见 Decision 四——第二处权威。

## 越权风险点（归 owner 复核）

本记录能力边界之外、实施时必须先答的几件，逐条列出以免被当作已裁：

**复核处置（2026-10-19，Claude Code 会话的复核建议，随用户接受一并认可）**：逐条标在各条之下；标「留待」的仍归 owner 复核，其余 owner 不认可的走 supersede。

1. **OpenAPI 3.1 工具链核实**——Scalar API Reference、`oasdiff` 在本仓将锁定的版本上对 3.1 的支持（`webhooks` 顶层对象、JSON Schema 2020-12 的 `const` / `oneOf` 表达）；核不过的项按方案稿「引入 3.1 前一起验证」那句退到 3.0.3 并记下代价（`webhooks` 改用补充文档）。归实施票，锁版本那一笔。
   - **认可**：实施票锁版本时核，核不过照原句退 3.0.3。
2. **描述符的 Go 形状**——它引用处理器包已有的枚举与结构体，但 OpenAPI 需要 JSON Schema，从 Go 类型到 Schema 的映射（反射还是显式声明）决定描述符写多少字；归 `tools/api-contract` 的实施票。
   - **认可**：归 `tools/api-contract` 的实施票。
3. **文档阅读策略**——匿名可读、合作伙伴登录、还是随沙箱凭据；产品决定，归 owner。
   - **复核建议**：匿名可读（Decision 七）。
4. **弃用窗口长度**——对租户的服务承诺，商业参数，参数登记册增行；未登记前不弃用。
   - **改**：归产品策略，主版本进入弃用后至少 12 个月，随契约文档发布，不进试点参数登记册（Decision 六）。
5. **SDK**——是否从契约生成客户端、哪门语言先出、由谁维护；交付物决定，另立。
   - **留待，仍归 owner 复核**：首发不出 SDK；公开的 3.1 契约已能让集成方自行生成。
6. **`parcel-api` 是否为文档与契约另开监听端口**——部署形态，与 ADR-0139 Decision 三「两族在装配点分」不冲突，但方案稿「两 Router 两端口」是否值得，归部署 owner。
   - **复核建议**：不另开端口（Decision 八）。
7. **每个上下文 owner 对本上下文端点描述符的确认**——描述符是对外承诺的第一份书面形态，`outcome` 词表进了契约就进了兼容性基线；各 owner 在首次发布前逐端点确认。
   - **认可，确认本身仍归各上下文 owner**：作为首次对外发布前的一道门。

## Links

- [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)：首方族与 Decision 五的留白；本记录填其「契约怎么生产、URL 主版本、public / internal 拆分、文档门户」四问，正文不改
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：契约语义的权威；正文不改，三类表 4xx 行对 `429` 的适用由 Decision 三部分停用；问题详情只承载「为什么没有 `outcome`」
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：Decision 一「路由层不知道渠道这个概念，另设闸就是第二处权威」——本记录 Decision 四不加契约校验中间件的依据；Decision 二错误码纪律
- [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：操作者族——操作者 API 契约的覆盖面由挂操作者 Intake 的端点导出
- [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md)：产品内客户端与后端同版本发布——操作者 API 不带主版本的依据
- [ADR-0018](./0018-product-clients-share-release-boundary-under-apps.md)（已被取代，落位结论沿用）：业务端点住各上下文 `adapters/http`——描述符跟着住的依据
- [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)：Alternatives 对 `platform` 的界定「承载无业务判断的技术件」——问题体辅助函数收进 `httpapi` 的依据
- [ADR-0141](./0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md)：沙箱 `servers` 与可发起测试请求的文档页
- [ADR-0142](./0142-first-party-webhooks-are-a-product-owned-outbound-channel.md)：出向事件进同一份契约的 `webhooks` 节——选 3.1 的理由之一
- [`docs/api` Go + chi + Scalar 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)：参考输入；收与否各条见 Consequences 第二点
- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：机制半边清点「CI 每次重新生成并与提交进来的那份比对，不一致即失败」——本记录 Decision 一借用的纪律
- `internal/platform/httpapi/router.go`：`BusinessEndpoint` 刻意不带 `Method`、405 由处理器自守、`middleware.RequestID` 的出处
- `internal/parcelshipment/adapters/http/submit_shipment_request.go`：`outcome` 封闭词表、`problemResponse` 不带自由文本、传输层错误码常量的现状形状——描述符引用的就是这些
- [ADR-0144](./0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md)：列表端点的分页形状随描述符进契约
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：弃用窗口归产品策略的分界检验
- [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：集成客户端族——集成 API 契约的覆盖面
- [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html)：问题详情；[RFC 9745](https://www.rfc-editor.org/rfc/rfc9745.html)：`Deprecation` 头；[RFC 8594](https://www.rfc-editor.org/rfc/rfc8594.html)：`Sunset` 头
- 来源：IDP 队列通道 1 的复核与用户「按建议继续」指令（2026-09-16）；Claude Code 会话复核（2026-10-19），用户在会话里答「接受」，修订后接受
