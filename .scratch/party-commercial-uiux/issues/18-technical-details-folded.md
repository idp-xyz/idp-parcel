# 18 — 接入态、拒绝态与登记签里的接口路径、错误码、文档出处收进「技术详情」

**Parent:** [spec](../spec.md)

**What to build:** 评估时发现：目录读不出来或写口被拒时，页面把状态码、问题码、接口路径、主责上下文、场景出处一股脑摆在明面——「调用方式问题（HTTP 400）」「服务端未形成答案（HTTP 502）」「接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）」「提交打到 POST /commercial-…」「按 ADR-0055 如实拒绝……PAR-INT-01……」。配置员读不懂也用不上，真要报障或对日志的人才需要。

改法：新立共享件「技术详情」折叠（默认收起，字仍在标记里，按码检索与测试按字断言不受影响），把这些字收进去；明面只留业务上怎么回事和下一步做什么。

- 共享件 `components/states/technical-details.tsx`：`TechnicalDetails`、`httpRows`（状态码与问题码两行）。
- 接入态卡（未配置、需要登录、没有授予）：明面留标题、说明、放行条件；主责上下文、场景出处、接口路径、错误码进技术详情。行内的「需要登录 / 没有授予」一句不再带状态码与错误码，它们进句下的技术详情。
- 页级错误态与区块级错误：标题不再带「（HTTP n）」，状态码与问题码走技术详情；目录页的状态映射（`catalogueViewState`）与模板的错误态各多收一个可选的 `technical`。
- 登记签：「提交打到 POST …」与「在线登记口与受控登记 CLI 消费同一登记用例」进技术详情；答复行「登记册答复：REGISTERED —— 已登记」改成先说中文，答复码、拒绝理由码、在等的原因码进技术详情；接入渠道未配置那一大段（ADR-0055、PAR-INT-01、装配侧换 Intake……）改成一句业务话，原委进技术详情。
- 参与方模块里照同一格式自拼「调用方式问题（HTTP n）」的几处（法人资料、参与方全景、修订历史、发布流程、关系结束动作）一并收。

不做的：别的模块自己拼文字时用的 `accessRefusalHeadline`、`operatorRefusalSentence`、`action-feedback` 的码句不动（几十处调用、各自的测试钉着字，业务上也是它们各自的句子）；别的模块页里自写的状态码文字不动；服务端随 4xx 交回的理由散文与未受理的原因原样留在明面（它们是登记方要去改的那半句，不是排查用的字）。

**Blocked by:** None — can start immediately

**Category:** enhancement

**Status:** resolved

- [x] 目录页的未配置、服务端无答案、调用方式问题三态，明面不再有状态码、问题码、接口路径、主责上下文、场景出处，技术详情里都在。
- [x] 登记签与关系结束动作不再有「提交打到 POST …」；答复行先说中文。
- [x] 折叠件与各处改动有 node:test 用例，原有被钉的字照旧或随改动更新；三道门退 0。
- [x] 共享面（`components/*`、`templates/*`）有评审。

## 完成记录（2026-10-26，tops-54，前端切片，worktree `idp-parcel-pcux8`）

- **落点：** 本笔提交。
- **改了什么：**
  - 新件 `technical-details.tsx`；`access-state.tsx` 的卡面与 `OperatorRefusalNote`（改成 `div` 里一句加一格折叠，`UnconfiguredFacts` 多一个可选的 `endpoint`）；`section-error.tsx` 与 `ErrorState` 多收可选的 `technical`；`templates/state-slot.tsx` 的错误态多收并转交 `technical`。
  - `auth/access-refusal.ts` 多一个 `operatorRefusalMessage`（不带状态码与错误码的整句），`operatorRefusalSentence` 不动。
  - `pages/catalogue-view.ts`：未配置说明不再带接口路径与主责上下文名（进卡面的技术详情），两个错误态标题去掉「HTTP n」、带 `technical`。
  - `components/registration/RegistrationPanel.tsx`：提交说明、答复行、三种非业务答复的呈现。
  - 参与方模块五处：法人资料、参与方全景、修订历史、发布流程的答复句，关系结束动作的提交说明；选择器暂读不到可选项的提示去掉状态码。
- **判据：**
  - ✅ 演示环境里在页面内改写 `fetch` 造出三种答复再看服务产品页：403 未配置，卡面是「访问通道尚未配置 / 目录的读取入口当前不可用；这不是“目录为空”。/ 放行条件 … / ▸ 技术详情」；502，「服务端没有形成答案 / 目录读取依赖未能形成答案，可稍后重试。/ ▸ 技术详情 / 重试」；400，「调用方式问题 / 查询参数不在服务端接受的封闭集合内… / ▸ 技术详情」。技术详情默认收起。
  - ✅ 商业规则与策略页「JSON 发布（高级）」签：明面没有「提交打到」，技术详情默认收起。
  - ✅ 用例：新增 8 条——折叠件 3 条（默认收起、字在标记里、空行不显、HTTP 两行）；接入态卡面明处与折叠处的分界；区块级错误带技术详情；`operatorRefusalMessage` 与 `operatorRefusalSentence` 的差别；目录页三态的 `facts` 与 `technical`（`catalogueViewState`）；登记答复的码三格（`outcomeTechnicalRows`，从 RegistrationPanel 抽成纯函数）；另一条被改变的旧断言（行内一句不再带状态码，并按 `<details` 切开断言状态码只在折叠里）随改动更新。三道门：`tsc -b --noEmit` 退 0，`run-tests` 757/757，`vite build` 退 0；`jsx-cjk-space` 零命中。另跑 `go test ./internal/architecture` 退 0（见评审第一条：它扫 `src` 下全部 .ts/.tsx 里的请求路径，含测试夹具）。
  - ◑ 需要登录、没有授予两态的真页面没在浏览器里走（演示租户的配置员有授予）；markup 由 `renderToStaticMarkup` 的用例钉。
- **已知：** 改共享件的呈现，所有模块里用到它们的页面一并得到折叠（`OperatorRefusalNote` 二十余处、接入态卡、`SectionError`、登记签）；别的模块自拼文字的几十处照旧带码。
- **评审：** 隔离子代理一份（Spec 轴，只读，2026-10-26），总评「需先修」，已修后再跑全部门：
  - **阻断，已修：** `access-state.test.ts` 夹具里的 `endpoint: 'GET /x'` 被 `internal/architecture` 的 `TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable` 认成管理台请求路径、报端点表无此行，CI 会红；三道门不含这一道，作者没跑到。夹具改用端点表上的真实路径（同 `catalogue-view.test.ts` 的写法）。以后改 `apps/admin-web` 凡是写进路径形字符串（含测试夹具），多跑这一道。
  - **已修（文案）：** 登记签「接入渠道未配置」那句业务话把「管理员」改成仓内用语「配部署的人」；「受控登记 CLI」与「受控登记命令行」统一；折叠里补回「登记端点已建立并装配」那一半事实；状态码与错误码改取 `accessRefusalStatus`、`accessRefusalCode`，不手写；页级错误态缺省说明的「请携错误码求助」补上「请展开「技术详情」」的指路。
  - **已修（注释与测试）：** 含变更叙述与位置词的几处注释（「不再带」「仍用上面那句」「今天」「原来」「分两张表」）改写；区块级错误测试夹具改用新标题；补上目录页三态、`OperatorRefusalNote` 按 `<details` 切开、登记答复码三格的用例。
  - **核实无问题：** 回归扫描（用 TypeScript AST 扫全部 `.tsx`）没有任何调用点把这几个组件放在 `p/span/button/label/a/h*/li/td` 里，`div` 嵌套合法；没有样式选择器或测试依赖原来的 `<p>`；其他调用方只构造 `facts`，不自己把 owner、source、unlock 当明面字；`error.technical` 可选，向后兼容；折叠件只引 react，node:test 钉得到。
  - **未采纳：** 无。
