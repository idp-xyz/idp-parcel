# ADR-0316：发布载荷的逐格问题带稳定的问题码与参数；管理台按码译，认不出的码退回原话

Status: Accepted（2026-10-27，Claude Code 会话代 owner 决，用户授权代决；票 party-commercial-uiux-backend/05。裁决能力边界：读过运营操作者面发布载荷的解码与逐格问题收集（`publication_draft_payload*.go`）、预览与录入的答复形状、管理台 `field-problem-text.ts` 的原话译表与各表单对问题行的消费；越权风险点见文末，待 PC owner 复核。）<!-- 索引注记 -->｜**入口**：逐格问题的 `problem` 是领域错误或解析错误的 `err.Error()` 原话，管理台只能按原话的形状猜中文，服务端一改措辞就译不出。**取法**：每一格问题除原话外另带 `code`（封闭词表，只增不改）与 `params`（字符串键值，键随码固定）；管理台认得的码直接查话术，码缺席、是兜底的 `INVALID` 或不认得时退回按原话译。
Date: 2026-10-27

## Context

- 预览与录入的 `NOT_ACCEPTED` 答复里，逐格问题是 `{field, problem}`，`problem` 是构造门的原话：领域错误（`party commercial: blank value: …`）、解析错误（`parsing time …`）与适配器里手写的中文句子混在一起。配置员读不懂、改不对；管理台因此维护了一张按原话形状译中文的表（`field-problem-text.ts`），那张表的文件头注释自己写着「前端先行的过渡，根治是服务端交回问题码加参数」。
- 按原话译有两个无法消除的毛病：服务端改措辞（哪怕只改一个标点）时前端静默退回通用的一句；原话里的主语与取值要靠正则抠，抠错了显错话。
- 逐格问题已经是一个封闭的答复形状（`payloadProblemAnswer`，预览、录入、词表读口共用），加两个字段不改任何结果代数，也不改状态码纪律。

## 候选与反方

- **只在前端加更多正则。** 否决：那正是现状，毛病原样保留。
- **服务端直接交回中文。** 否决：话术归呈现层（管理台、将来的别的客户端各有各的措辞与语种），服务端交中文等于把呈现写进契约；且原话已经是中文白话与英文错误混杂，再并一套只是第三份。
- **给每个领域错误都立一个码。** 否决：领域错误几十个，大半对配置员没有可行动的话术；一次铺满会立下一份没人维护的大词表，而码一旦发出就不能改。
- **码只覆盖有话术的几类，其余兜底，前端退回原话（采纳）。** 词表小而稳，随着更多格被贴上码逐步扩，不需要一次定完。

## Decision

**一、每条逐格问题带 `code` 与 `params`。**（机制）

- 答复里每一条问题是 `{field, problem, code, params?}`。`problem` 照旧是原话，供排查与兜底；`code` 恒在场；`params` 是字符串键值，没有参数时整个键不出现。
- 码与参数键（词表在 `publication_problem_codes.go`，那里的常量注释是契约的一部分）：`REQUIRED`（`subject` 可缺）、`NOT_IN_SET`（`what`、`value`）、`TIME_FORMAT`、`EXACTLY_ONE`（`subject`）、`RATIO_BASE_NOT_ALLOWED`、`RATIO_BASE_REQUIRED`、`DAYS_NOT_POSITIVE`（`value`）、`ORDER_NOT_POSITIVE`（`value`）、`CHARGED_UNDECLARED`、`LIST_EMPTY_OR_DUPLICATE`，以及兜底的 `INVALID`（没有参数）。
- 贴码在问题产生的地方做，不在出口按原话猜：适配器里的构造点用带码的原因（原话不变、`errors.Is` / `errors.As` 照常穿过）；领域的空值错误 `ErrBlankValue` 在收集处认成 `REQUIRED`；其余没有更具体码的答 `INVALID`。时刻格留空答 `REQUIRED`（对配置员那是「没填」），格式错答 `TIME_FORMAT`。

**二、稳定性承诺。**（机制）

- 码只增不改：一个码一旦发出就不换含义、不回收；要改含义就立新码。每个码的参数键同样只增不改。新增码与新增参数键对旧客户端是兼容的，因为客户端对不认得的码退回原话。
- `problem` 原话不在承诺内：它的措辞可以随实现改，客户端不得据它分支（兜底的形状译表除外，那是已知的权宜）。

**三、管理台按码译，认不出的退回原话。**（管理台形状，不是产品规则）

- 管理台对认得的码查自己的话术（参数里的主语与格名直接用，缺参数时用不带主语的说法）；码缺席、是 `INVALID` 或不认得的新码时，退回原先按原话形状译的那张表，再认不出给通用的一句。原话照旧收进可展开的一格。
- 问题在页面里以字符串行的形式流过各表单组件：带认得的码的问题在归组处就译好，并把原话用一个私有分隔符接在后面带过去，显示那一刻拆开。这是为了不改各个只认字符串的表单组件的权宜；等表单组件改成直接收问题对象时去掉。

**四、不在本记录。** 受控批文口（`/commercial-publications`）与各登记 JSON 镜像口的问题答复（它们是另一套答案代数）；领域错误的全面编码；话术的多语种。

## Consequences

- `adapters/http`：新文件 `publication_problem_codes.go`（词表、带码的原因、收集时的取码）；`PayloadProblem` 与 `payloadProblemAnswer` 各多 `Code`、`Params`；各册载荷里对应这几类的构造点改用带码的原因（原话字节不变，既有用例不动）。
- 管理台：`PayloadProblemRecord` 多 `code?`、`params?`；`field-problem-text.ts` 多按码的话术表与 `codedProblemLine`；`problemsByField` 经它出行。
- 无迁移，无新授予，无结果代数变化。

## 越权风险点

1. **码一旦发出就是公开契约**（PC owner）。词表虽小，管理台与将来的客户端都会依赖它；本记录承诺只增不改，若日后发现某个码的含义划错了，只能立新码并让旧码停止发出（旧码对旧客户端仍要保留一个可解释的含义）。
2. **没有更具体码的问题答 `INVALID`**（PC owner、管理台）。多数领域错误今天仍落在兜底里，管理台对它们仍按原话的形状译；这是有意的，但意味着「按码译」只覆盖了词表里的几类，其余的措辞稳定性问题没有消除，只是没有恶化。
3. **`NOT_IN_SET` 的 `what` 是服务端写死的中文格名**（PC owner）。它是给人读的名词（如「规则分类」），不是机器键；客户端要多语种时需要一张 `what` 到各语种的表，或日后把它换成稳定的格键——那时要立新参数键，不改这个。
4. **行内编码的分隔符是管理台内部的权宜**（管理台）。它不出现在任何契约里；若服务端的原话里将来出现该分隔符（概率可忽略），拆分会错位，届时应按第三点所说改成直接收问题对象。

## Links

- [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)：运营操作者面发布载荷的形状与逐格问题的收集
- [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)：预览与录入共用同一段解码，逐格问题出自同一处
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：`NOT_ACCEPTED` 答 200，状态码只说答案有没有形成
