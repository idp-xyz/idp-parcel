# 复核端点（未配置格）；管理台复核动作、逐字段登记表单、更正动作

Category: enhancement
Status: resolved——04a 后端落 `9035df7`（MCP-1）；04b 前端两件（复核行动作、JSON 登记口退为「高级」）落 `121097d`（MCP-4），余三件（复核状态列、逐字段登记表单与预览、更正此版本预填）按 Comments 末条并入票 [08](./08-series-write-face-needs-a-draft-and-a-version-read-face.md)，08 已 resolved（`0a67406`，随远端 `f148759` 发布）。本票要建的五件因此各有落点，只差状态行。状态由通道 2 于 2026-09-04 代簿记
Blocked by: 无（03 的代码已是主线祖先 `f62d619`；票 03 卡的只是 `seed.sh` 真库冒烟那一格，
不拦本票）

## 要建什么

按 ADR-0085 与 ADR-0099：

1. **`POST /pricing-reference-series-reviews`**：`adapters/http` 增 Intake 接口 + 处理器接口 + 封闭响应形状（ADR-0022 状态码语义），装配以 `UnconfiguredIntake{}` 起步；装配行进 `cmd/parcel-api/endpoints.go`（共享接线文件，先在频道占号）。
2. **管理台「计价参考序列」页**（`apps/admin-web/src/pages/pricing/ReferenceSeriesPage.tsx`）：
   - 目录列增「状态」列：已登记 / 在用 / 已退回 / 已替代（由读口给，见 05 或本票内最小扩展 `ReferenceSeriesListResponseBody`）。
   - 行动作「复核」：结论 + 依据；复核责任方取当前操作员标识（未配置时端点如实 403，文案说「接入渠道未配置」不说「尚未实现」）。
   - 「登记序列」签改为**逐字段表单**：序列（选已有 / 新建）、种类、来源标识、口径（汇率必填；从商业价格政策目录读口选带口径的版本——若该读口今天没有，先做成手填两格并在文案里说明）、期次表格（起 / 止 / 值 / 凭证引用）、登记责任方取当前操作员。提交前预览：证据等级、内容摘要、与上一版逐期差异。表单在前端组快照后仍走 `POST /pricing-reference-series-registrations`——摘要由后端领域构造函数算，前端只预览。
   - 行动作「更正此版本」：预填该版本全部期次，强制填更正依据，自动带 `PriorVersion`；**页面上不出现「编辑」**。
   - JSON 粘贴口保留为「高级」折叠，供 API 集成方。

## 红线

- 写准入不另立形（ADR-0085 决定二）；隔离 demo 里这些动作如实答未配置。
- 前端不算摘要、不裁证据等级，只呈现后端答复。
- 前端门禁只有 `tsc --noEmit`（本机 `pnpm build` 坏在环境，见 workflow.md），别去跑 `pnpm install`。

## 验证

http 单测：只收 POST、未配置 403、三态响应。`node node_modules/typescript/bin/tsc --noEmit` 绿。

## 切片

**04a 后端（已落）／04b 前端（未做）**，分开是因为**前端半必须晚于后端半落地**：`2a9a76a`
那道门禁断言前端路径 ⊆ parcel-api 端点表，先推一个发向 `/pricing-reference-series-reviews`
的前端动作而端点表还没那一行，会把全仓染红在 `internal/architecture/`——而那种红看起来
与自己的改动无关，别人得先停下来查归属。

## Comments

- 2026-09-03 · MCP-1：**切片 04a 落地：复核端点进端点表，生产编排接真。**

  `POST /pricing-reference-series-reviews`。`adapters/http` 增 `ReferenceSeriesReviewIntake`
  与 `ReferenceSeriesReviewer` 两个接口 + 端点，`UnconfiguredIntake` 补第三个方法；装配行
  进 `endpoints.go` 挂字面量 `UnconfiguredIntake{}`（写准入不另立形，ADR-0085 决定二），
  `main.go` 接真编排，`unwired_orchestration.go` 加占位，探针表补一格。

  **七格答案逐字透出，其中三格是治理答案不是调用方错误**：`需换人复核`（换个人来）、
  `版本不在册`（先去登记）、`冲突`（改主意另追加一条）。折成 4xx 会让调用方以为自己请求
  写错了，而那三格的恢复动作各不相同。`已追加`取 201，其余取 200。

  **这一口比两个登记口更不能松，写在了三处代码注释里**：复核责任方是四眼门的一半（领域
  拒绝复核责任方等于登记责任方），任何采信自报身份的 Intake 都等于把那道门拆了。因此
  `TestIsolatedReadIntakeCannotServeRegistration` 补了第三条断言——隔离读那个注入的合成身份
  装不进复核口，编译期成立。

  **事务包装不与两个登记包装合并**（`transactionalReferenceSeriesReview` 独立成型）：登记与
  复核是两种命令两套代数，合成之后装配点可以把登记编排接到复核端点上而编译仍绿。生产侧
  读口与写口是两只适配器落在两张表上（`ReferenceSeriesVersions` 读回登记供四眼门，
  `ReferenceSeriesReviews` 追加复核），分设不是为了对称。

  **未做（04b，前端那半）**：状态列、复核动作、逐字段登记表单、更正动作、JSON 粘贴口折成
  「高级」。端点已在表上，那道门禁不再拦着它，谁接都行。**接的人注意票面里那条**：口径要从
  商业价格政策目录读口选带口径的版本，而那个读口今天可能没有——票面自己给了退路（先做成
  手填两格并在文案里说明），**但请先取证它到底有没有，别照票面那句直接实现**。

  **验证**：`gofmt -l` 为空、`go build ./...` 退 0、`go vet ./cmd/parcel-api/
  ./internal/parcelpricing/...` 退 0、`go test -count=1 ./cmd/parcel-api/
  ./internal/parcelpricing/... ./internal/architecture/` 全 ok（含真库；本切片对
  `adapters/postgres` 零改动，那一层的绿是既有用例给的）。`-race` 未跑。

- 2026-09-03 · MCP-4：**认领 04b。开工前先取证，结果改了本票三处形状；两处待裁已由 owner
  裁完。本条只记结论与依据，实现随后。**

  **一、`Blocked by` 行里那半句已删。** 原写「那一格真正缺的是 WSL 里的 Go 工具链」——
  MCP-5 已自行撤回（它那次探测用的是不读 `~/.profile` 的非登录 shell，`go` 实际在
  `/usr/local/go/bin/go`，登录 shell 下是 go1.26.5），MCP-3 已在票 03 就地更正
  （`426f3fb`）。**那句是在更正之前被抄到本票的**，而本票本就 `Blocked by: 无`，那个子句
  对它自己不起作用，只是把一个错因多存了一份。MCP-5 的自评值得连着记：**「我撤回得够快，
  但没有任何机制把撤回追到已经抄走它的地方。」**

  **二、票面担心的「口径读口今天可能没有」不成立，那条退路不要走。**
  `GET /commercial-policies?kind=PRICE_POLICY` 的 `pricePolicyBody` 已带 `caliberDeclared`
  与 `caliber.fx{quoteType, asOfSemantics, asOfPolicyVersion}`，而序列登记的 `QuoteBasis`
  正是 `ArtifactCommercialPolicy` 的版本引用，键对得上。**做成手填两格不只是多余，是错的**
  ——它让操作员手敲一个服务端已经知道的值，而手敲值与目录里那个版本对不对得上没有任何
  东西在校。真正要做的在前端：`apps/admin-web/src/pages/party/api.ts` 的
  `PricePolicyRecord` 止于 `registeredAt`，**没有那几个字段**；接上即可（MCP-5 取证）。
  注意 `caliberDeclared` 可为假（0010 早于 0022，只有正文没口径的行合法），选单要把这两类
  分开——那个布尔存在就是为了分开它俩。

  **三、「状态列」与「更正预填」纯前端做不了，owner 已授权本票扩到后端读口。**
  `ports.ReferenceSeriesCatalogueRow` 逐字段核过：既无复核状态也无期次。更正动作要预填
  全部期次，而列面不返期次、今天也没有任何详情读口——没它就得让人重敲一遍全部期次，
  **而那正好制造本票要防的那类错误**。

  **四、裁决：状态列不含「在用」，只透纯转写的复核事实（owner 2026-09-03 裁）。**

  这一格是取证时才看清的：`domain.SelectInForceSeriesVersion(candidates, at)` **要一个
  时刻**——在用是相对**评价形成时刻**派生的结论，而目录页没有那个时刻。拿「浏览此刻」
  代入会让页面显示一个只对此刻成立的结论，而昨天形成的评价可能用的是另一版，**那种页面
  看起来是权威的**；读面这么做还会形成判断，违反 ADR-0077 读面通例。

  **而 `0004_reference_series_review.sql` 的文件头早就把话说死了**：「『在用』不是它上面的
  状态列，而是从本表按评价形成时刻派生的结论……做成状态列会让『谁在何时凭什么通过』在
  结构上无处落。」票面原句「状态列：已登记／在用／已退回／已替代」与它直接冲突——**写票面
  的人没读到那段**。

  因此本票的列改为**复核状态**，只透后端能纯转写、不需要发明任何排序的事实；「在用」
  留给票 05a 的覆盖读口（它本就要给在用版本引用，且那里有正当的时刻来源）。

  **五、一条约束，与谁做无关，接 05a 的人同守**：在用判定只在
  `domain.SelectInForceSeriesVersion` 一处。`ports/reference_series_review.go` 的注释原话是
  「SQL 若也排一遍就是两处口径」。本票读口只取事实、不在 SQL 里裁。

  **六、这是今晚第六件同族的事**（前五件：撤销的合成种子、票 03 照抄的网络理由、票 02 五处
  错引 ADR、票 08 那个永远不可达的答案格、MCP-5 的非登录 shell 伪象）。**六件里有五件是
  写票面的人当时没去量那一句，而它在票面上与量过的那些长得一模一样。**

- 2026-09-03 · MCP-4：**切片 04b 交两件，其余三件另立票 [08](./08-series-write-face-needs-a-draft-and-a-version-read-face.md)。**

  **已交：**

  1. **行动作「复核」**——目录页每行加复核按钮，展开一个两格表单（结论：通过 / 退回；
     依据：必填）。新 `SeriesReviewPanel.tsx`，打 `POST /pricing-reference-series-reviews`
     （04a 落的那一口），七格答案代数逐格中文。
  2. **JSON 登记口退为「高级」**——签名改为「高级：JSON 登记口」，文案讲明它是受控批量口
     的在线镜像、供 API 集成方与批量登记用，运营配置员的主路径另建。

  **按 ADR-0101 决定八，本条写明复核这一格选了哪一形与理由**（决定八要求每册在自己的
  实施票里写）：复核取**逐字段表单**，判据是它在决定一那三条上的直接读数——**低频、两格、
  治理性质**，不是价卡那种上千格的矩阵。这是本仓第一个逐字段登记面，先例记在这里。

  **表单上没有「复核责任方」，那不是漏做。** 它是四眼门的一半（领域拒绝复核责任方等于
  登记责任方），从浏览器收一个上去就是自报身份；传输层 `ReferenceSeriesReviewIntake` 的
  注释原话是「从请求内容里铸一个出来就等于把那道门拆了」。正当出处是 ADR-0100 的
  `OperatorEnvelope`。今天渠道未配置，提交必然 403——**页面因此说「接入渠道未配置」而不是
  「尚未实现」**：机制在、墙也在，两者不是一回事。复核时刻同理不在表单上。

  **未交三件与它们各自缺的东西，全部写进票 08**：复核状态列与「更正此版本」预填缺逐版本
  读面的两组字段；**逐字段登记表单的提交前预览缺一个「先校验、回摘要与证据等级、尚未
  登记」的后端步骤**——证据等级、内容摘要、逐期差异三样前端一件都算不了，而本票红线原句
  正是「前端不算摘要、不裁证据等级」。**没有预览的逐字段表单等于把一份可能被拒的载荷盲提
  上去**，所以没有硬做。

  **验证**：`node node_modules/typescript/bin/tsc --noEmit` 退 0；
  `node scripts/run-tests.mjs` **25/25 全绿**（本仓 admin-web 的测试是 node:test 不是
  vitest，命令在 `package.json` 的 `test` 脚本上——我先跑错成 `npx vitest`，它会从网上拉一个
  vitest 然后报 8 个「No test suite found」，**那不是红，是跑错了 runner**）。本片纯前端，
  Go 侧零改动。`pnpm build` 未跑（本机坏在环境，见 workflow.md）。

  **一件自己犯的、记下来免得下一个人照抄**：我顺手把改过的 `.tsx` / `.ts` 从 CRLF 洗成了
  LF，**而它们是 `text=auto` 不是 `eol=lf`——工作树 CRLF 是 `core.autocrlf` 的正常往返，
  不是违规**。MCP-5 今晚明确警告过这一格（「白洗一次、下次 checkout 又变回去，中间把别人
  的 status 搞脏一轮」）。没有实害（两侧都归一化，diff 仍只有内容改动），但那一洗是多余的。
  **判该不该洗要看 `git check-attr eol`，不是看有没有 CR。**
