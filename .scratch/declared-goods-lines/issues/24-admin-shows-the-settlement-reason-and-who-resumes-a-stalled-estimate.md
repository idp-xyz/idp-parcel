# 24 管理台：估价停因带结算原因与服务产品，写明谁来续办

Category: enhancement
Status: in-progress
Blocked by: 23（已提交，未进 main；同在分支 `feat/goods-details-admin`）

依据：[ADR-0380](../../../docs/adr/0380-an-estimate-the-price-card-side-cannot-form-waits-for-operator-registration-and-the-attempt-records-why.md) 决定五；票 21 的停因译法（`shipment-request/stall-reasons.ts`）。

## 要做的

- 运单详情「受理进度」与「受理前财务控制」两处：`lastAttemptSettlementReason` 在场时，用它译出细原因。服务产品在场时一并写，例如「估价没形成：没有在用价卡（服务产品 SYN-PROD-…）」。
- 按 `lastAttemptResumePath`（续办方，票 23 交）写谁来动手，不从 `lastAttemptContinuation`（那是续办引用）或原因推：
  - 价卡一侧的等运营登记写「登记一张生效期覆盖提交时点的价卡后自动续办」，链到价卡目录；
  - 内部重试写「系统会自动重试」；
  - 等客户补充照旧。
- 细原因的码复用票 21 已有的 `SA-` 译表：存的码不带前缀，补上 `SA-` 查同一张表，不另立一份；对照 Go 源的契约测试照旧。

## 完成判据

三道门通过；纯函数单测覆盖带与不带细原因、带与不带产品、三种续办方。

## 不做的

后端（票 23）。

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/admin-stall-reason-detail` 上的 `096ec1bb3` 与评审修订 `e6acc3f58`（基线 `cf9fdb50a`），全部改动在 `apps/admin-web/`，未推送、未进 main。后端（票 23）不在这棵树上，照钉死的字段名写，没联调。
- 拣入：在 `feat/goods-details-admin` 上是 `b536b71e3` 与评审修订 `0f49f0556`，叠在票 23 之后。
- 与票面不同的几处（含开工后协调方按评审改钉的）：
  - **续办方读 `lastAttemptResumePath`。** 开工时票面写「按 `lastAttemptContinuation`（续办方）」；实测于 `cf9fdb50a`，那一格是续办引用（由未决原因与范围派生的 `CONT-…` 串），说不出续办方，委托详情也没有交续办方的格。先按 Go 的 `JudgmentPendingReason.resumePath()` 从停因推过一版，评审改钉为后端另交 `lastAttemptResumePath`（取处理尝试落库的续办方），不从停因或续办引用推；现行做法只读这一格，缺席或认不得的词不说。
  - **价卡一侧那句的写法改钉**为「登记一张生效期覆盖提交时点的价卡后自动续办」（估价按提交的接收时点取卡，只从现在起生效的卡解不开），链到价格表 `#/price-card-catalog`，链接字是「去价格表」。
  - **币种对不上不归价卡一侧**（评审改钉）：价卡一侧是没有在用价卡、价卡适用冲突、试算没有合计、合计不为正、某件为负；币种对不上照旧内部重试，不链价格表。代码与 ADR-0380 决定一现行的名单一致。
  - **前缀**：存的码不带 `SA-`，补上前缀查票 21 的同一张表。
  - 等运营登记而细原因不在价卡一侧（授权规则、财务控制策略、估价采用等），写「运营登记后自动续办」，不指到价格表——那些登记不在那里。
  - 票面只列了等运营登记、内部重试、等客户补充；领域 `ResumePath` 另有等人工复核与等授权处置，一并认得：分别写「复核角色记下复核后续办」「授权处置角色选定去向后续办」。等客户补充照旧，不另起一句。
  - 任务已完成或已停止时不说谁来续办：最近一次尝试还留着，那份等待已经过去。
  - 细原因前的说法按结算估价那一口分：估价各格写「估价没形成：…」，估价以外（如财务控制策略未配置）写「财务控制没形成：…」。
  - 服务产品只接在财务控制那一步的停因上，带不带细原因都接；等客户补充、等人工复核等照旧不带（ADR-0380 决定五「等客户补充、内部重试的写法照旧」；评审改，首版各种停因都接）。
  - 续办另起一句说等运营登记时，停因去掉句尾的「，等运营登记」，免得同一屏说两遍（评审改）。
  - 复核页的「上次没推进」同用这一句（复核案卷与运单详情同一个编码函数，会带上同样的格），但复核页不另起续办一行，取整句（`resume: false`）；空串停因照旧当缺席。复核页技术详情补结算未形成原因码与续办方两行原码（评审改）。
- 落点：
  - 新文件 `pages/shipment-request/last-attempt.ts`：`lastAttemptOf`（停因一句、估价一块用的那截、谁来续办；`resume: false` 给不另起续办一行的页）、`knownResumePaths`。价格表的地址取 `pages/party/ref-links.ts` 新出的 `refPageHash('price-card')`，不另存一份。
  - `pages/shipment-request/stall-reasons.ts`：`SA-` 表各条加估价那一口与价卡一侧两个标记；新 `settlementReasonOf`。
  - `pages/shipment-request/pre-acceptance-estimate.ts`：`estimateBlockStateOf` 改收 `lastAttemptOf` 的结果，没有留痕、当前版本缺估价而说出停因时带上续办那一句。
  - 新文件 `pages/shipment-request/resume-note.tsx`：续办那一句与链接的摆法，「受理进度」与「受理前财务控制」共用。
  - `ShipmentRequestDetailPage.tsx`：「受理进度」的最近处理结果改读 `lastAttemptOf`，加「谁来续办」一行；技术详情加结算未形成原因码与续办方原词。`PreAcceptanceFinancialControl.tsx` 同读；`review-case.tsx` 以 `resume: false` 读，技术详情补两行原码。
  - `pages/shipment-request/api.ts`：`AcceptanceTaskRecord` 加 `lastAttemptResumePath?`、`lastAttemptSettlementReason?`、`lastAttemptServiceProduct?`。
- 判据：
  - 带与不带细原因、带与不带服务产品（只接在财务控制那一步，等客户补充与等人工复核不接）；价卡一侧各种各一与币种对不上；等运营登记分价卡一侧与别处；续办另起一句时停因去掉句尾的等运营登记、续办方缺席或任务不在运行时整句；`resume: false` 取整句；内部重试；等客户补充照旧；复核与授权处置；续办方缺席、认不得、只有续办引用；任务已完成与已停止；空串当缺席；认不得的细原因与停因——单测 ✅。
  - 价卡一侧的结算原因整份钉住（按标记从 `SA-` 表筛出，与 ADR-0380 决定一的名单逐个相等）——单测 ✅。
  - 估价一块：带细原因与产品时停因与续办、续办另起一句时停因不重说等运营登记、当前版本缺估价时同样带、当前版本已有估价或结论不适用时不带——单测 ✅。
  - 续办方的词与 Go 的 `ResumePath.String()` 两个方向对得上——契约测试 ✅（在测试产物上改一个词实测会红并点名）；原有的契约测试照旧绿。
  - 三道门通过 ✅（见验证）。
- 未做：
  - 浏览器里没走过：这台机器上起不了带后端的页面，没看真实版面，也没做仓外渲染探针。
  - 联调等票 23。价卡一侧那份名单与后端适配器的分法对不对得上，今天没有契约测试核：票 23 落地后，那份名单在适配器里才有，可补一条对着它核。
- 验证（`e6acc3f58`）：`apps/admin-web` 下 `node node_modules/typescript/bin/tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 1406 过 0 败（基线 1391；`096ec1bb3` 时 1405）；`node_modules/.bin/vite build` 退 0。
- 评审（非作者，2026-11-11）：无阻断。应改与小意见都在 `e6acc3f58` 改了——服务产品只接在财务控制那一步的停因上，等客户补充等照旧；复核页技术详情补结算未形成原因码与续办方两行原码；单测整份钉住价卡一侧的结算原因；记录里对 ADR-0380 决定一的说法改成与现行名单一致、不计数；注释去掉「两处」；价格表的地址只留 `ref-links.ts` 一处；复核页照旧把空串停因当缺席；续办另起一句时停因不重说「等运营登记」。
