# 19 供应商协议目录读面镜像后端已透的正文：`contentRegistered` 显式布尔 + 0021 各键上列，「没有正文表可读」那几句退场

Category: enhancement
Status: in-progress——2026-09-08 16:5x MCP-6 完工，待通道 1 重放进 main 与非作者评审后转 resolved（分支 `mcp6-awf19` 代码 tip `a21ef0c3`，逐笔、完成判据逐项与验证强度见文末「完成记录」；tsc 退 0 / run-tests 116/116，过期话 `git grep` 零命中且 main 上阳性对照命中）。此前 in-progress——16:4x MCP-6 认领（用户经通道 6 队列派发；分支 `mcp6-awf19`，先并入 main `dc1f0c07` 成 `d572e554` 再动手，票 10 那几笔因此在场）。此前 ready-for-agent——形状已裁清（后端读口已透的键逐一镜像、显式布尔照合同页同款、行转写抽纯函数；本票无待裁问题）。由票 [11](./11-supplier-agreement-form.md) 的非作者评审 Spec 非阻断 (1) 拆出（MCP-6 2026-09-08 15:1x，通道 1 指派；锚 main `83970dcf`）。不是伞票 07 的子票——07 管的是各册的发布主路径，本票是读面
Blocked by: 无（11 已进 main `83970dcf`，写签在场）

## 缺什么

后端 `GET /commercial-supplier-agreements` 自票 party-commercial-context-gaps/03 起逐字段透出版本壳与 0021 正文：
`internal/partycommercial/adapters/http/query_commercial_relations.go` 的 `supplierAgreementBody` 带 `contentRegistered`
显式布尔，为真时在场 `supplier`、`legalEntity`、`purchasePlan`、`agreementScope`、`agreementEffectiveStartsAt`、
`agreementEffectiveEndsAt`（可缺）、`registeredAt`；方向**不透出**——那一处注释的原话是「领域恒为 BUY，库上不成列，
转写一个常量等于为同一件事立第二个口径」。

前端没跟上：`apps/admin-web/src/pages/party/api.ts` 的 `SupplierAgreementRecord` 止于 `publishedAt`，头注写着
「服务端没有正文表可读(见后端 supplierAgreementBody 注释),因此这里也没有对应字段」；`SupplierAgreementsPage.tsx`
的列注（「今天没有对应的正文表可读」）与页面描述（「尚无正文册可读，不上列」）说的是同一句——这几句在票 pc-gaps/03
落地那天就过期了，只是一直没人碰这页。票 11 落了写签之后它变得可见：操作者从「发布协议版本」发出去的正文，在同页
「协议目录」里**版本行可见、正文列不可见**（票 11 评审 Spec 非阻断 (1) 原话）。

## 要做的

- `party/api.ts`：`SupplierAgreementRecord` 镜像后端各键——`contentRegistered: boolean` 必在，正文各键可缺（与 Go 侧
  `omitempty` 同形）；过期头注改掉，判据照本文件 `CustomerContractRecord` 上那段 contentRegistered 注释写：**没登记正文**与
  **登记了正文但某键为空**都可能表现为键缺席，恢复动作相反，页面必须先看布尔。
- `SupplierAgreementsPage.tsx`：目录加正文列——正文（已登记 / 未登记）、供应商、责任法人、采购方案引用、协议范围、协议
  区间、登记时间。壳在正文缺是合法状态（壳可先入册、正文随发布登记），照合同页「未登记」如实显示；布尔为真而键缺是
  响应不合契约，点名而不显示成空（判据同 `policy-rows.ts` 里接受前财务控制策略册那一支）。列注与页面描述那两句过期话
  退场，换成读面此刻真实的说明。
- 行转写抽成纯函数 + node:test（写法照 `party/policy-rows.ts`，钉：只有壳的行、带正文的行、布尔为真键缺的坏行三例）。

## 边界

不动后端读口、不动 0021、不加端点、不加查询参数（过滤只在已取回数据上做，README「列表页上列通则」）；方向不上列——
后端刻意不透，前端转写常量等于第二个口径；写签（票 11 的 `SupplierAgreementPublicationForm`）不动；读面 403 那堵墙
照旧由 `catalogueViewState` 答，本票不碰。

## 完成判据

供应商协议页「协议目录」签对带正文的行显出正文各键、对只有壳的行显「未登记」；`apps/admin-web/src` 下「没有正文表可读」
「尚无正文册可读」一类的话对供应商协议零命中（用 ASCII 之外的针要显式 UTF-8 读，见 workflow.md 本机环境）；tsc /
run-tests 绿。

## 完成记录（2026-09-08，MCP-6；分支 `mcp6-awf19`，基 main `dc1f0c07`——立票时锚的 `83970dcf` 已被 main 前进 19 笔，开工前先 `git merge main` 成 `d572e554`）

**逐笔（分支 SHA；main 上的 SHA 由进 main 记录补）**：

| 分支 SHA | 内容 |
|---|---|
| `8a9856f9` | 立票（基 `83970dcf`，只此一件 .md）——**不在 main 上**，重放时要带 |
| `d572e554` | merge main `dc1f0c07`——只为对着当前 tip 编译与测试，重放时跳过 |
| `b470bfe6` | 票面认领转 in-progress |
| `d6fdeacc` | `party/api.ts`：`SupplierAgreementRecord` 逐键镜像后端 `supplierAgreementBody`（`contentRegistered` 必在，正文各键可缺，与 Go 侧 omitempty 同形），过期头注换成合同页同款判据；新文件 `supplier-agreement-rows.ts`（列集 + `supplierAgreementRowsOf`，正文三态）与 `supplier-agreement-rows.test.ts`（票面三例 + 列集 + 协议区间无上界）。本笔单独可编译——页面此时仍用旧列，stash 页面改动后 tsc 退 0 实测 |
| `a21ef0c3` | `SupplierAgreementsPage.tsx` 改吃纯函数的列集与行（消费方式同 `CommercialPoliciesPage`），正文各列上列，列注与页面描述两句过期话退场 |
| （本笔） | 票面完成记录 |

**完成判据逐项**：

1. 带正文的行显出正文各键、只有壳的行显「未登记」——`supplier-agreement-rows.test.ts` 钉住：带正文行逐键（供应商、责任法人、采购方案引用、协议适用范围、协议有效区间带结束、登记时间）；只有壳行正文格「未登记」且其余正文格不给值（模板显「—」）；布尔为真键缺→正文格「正文缺失(响应不合契约)」、缺哪键点名哪格（判据同 `policy-rows.ts` 的 `contentRegisteredCell`）；`agreementEffectiveEndsAt` 缺按「持续有效」不点名。输入照后端 `TestSupplierAgreementsEndpointTranscribesContentOnlyWhenRegistered` 的两行。**未在浏览器里对着真后端看过**：本机 `pnpm build` 跑不了（workflow.md 本机环境），页面接线只由 tsc 与「消费方式同策略页」作证。
2. 过期话零命中——`git grep -e '正文表可读' -e '正文册可读' -- apps/admin-web/src`：**阳性对照** `main` 上命中 3 行（`SupplierAgreementsPage.tsx` 两处、`api.ts` 一处），工作树与 `a21ef0c3` 隔离检出上都是零；针是中文，所以先做了阳性对照才敢报零（workflow.md「零命中恰恰就是自查想要的结果」那条）。
3. tsc 退 0、run-tests 116/116（含本票 5 条）——工作树与隔离检出各跑一遍。

**验证强度（钉 `a21ef0c3`，隔离 detached 检出 `%TEMP%\idp-verify-awf19`，验后已拆：不加 `--force`，`Test-Path` 假、`worktree list` 无）**：`tsc --noEmit` 退 0；`run-tests` 116 / 116；过期话 grep 零。`.go` / `.sql` 无变动，未跑 Go 门禁。证据层级 **S**（隔离合成；无真后端）。

**边界在场**：后端读口、0021、端点、查询参数一样没动；方向不上列；`SupplierAgreementPublicationForm` 与 `catalogueViewState` 未动。

**自审（`/code-review` 两轴，基线 `d572e554`；子代理不可用——auth error——改串行自跑，两轴分开记）**：

- Standards · 阻断：无。自审中改掉三处：`present` → `orMissing`（Mysterious Name）；注释里「六格」计数改「其余各格」（计数会随列增减无声变错）；页面注释原写「采购价格条件与结算条件不在 0021 正文里」有误——采购价格条件在正文里就是采购定价方案的引用串，改准为「结算条件不在、价格条件按引用上列」。判断题三条留给非作者评审：(1) `supplier-agreement-rows.ts` 的 `col` 与 `policy-rows.ts` 的 `col` 同形（Duplicated Code）——两处 Row 类型不同，抽共享层要多一个模块，等第三个消费者出现再抬（判法同票 11 评审 Standards 那条「第三册接进时再改 switch」）；(2) 「协议 / 版本」从两行 JSX 变成 `id@version` 单串——README「展示层合成允许」点名了 `id@version`，与 `CommercialPoliciesPage` 同款，但与同页系的合同页不同款；(3) 列头「适用范围」「有效区间」加「版本」前缀——正文的「协议适用范围」「协议有效区间」并列上来后不加前缀两对分不开，词取发布签版本壳一节的格名。
- Spec · 阻断：无。判断题三条：(a) 只有壳的行正文各格显「—」而不是每格「未登记」——正文那一格已说「未登记」，采 `policy-rows.ts` 里 `jointPassCondition` 的写法；合同页每格写「正文未登记」是另一种，票面「照合同页『未登记』如实显示」两种读法都通得过；(b) 搜索扩到转写后的全部格（正文列也搜得到）——票面没要，但是「过滤只在已取回数据上做」之内的事、策略页同款；(c) 测试比票面点名的三例多两条（列集、协议区间无上界）——同一模块的钉，不是别的行为。

**未做（各归其处）**：浏览器实看（无真后端、`pnpm build` 本机不可用）；非作者评审由通道 1 派；进 main 后 Status 转 resolved。

**拆树提醒**：worktree `%TEMP%\idp-parcel-mcp6-awf19\apps\admin-web\node_modules` 是指向主树 `node_modules` 的目录联接（为跑 tsc 建的，被 `.gitignore` 忽略、不入库）；拆树前先 `cmd /c rmdir` 掉那个联接再 `git worktree remove`，别让递归删除顺着它走。

## Comments

### 非作者合入前评审（2026-09-08 17:0x，MCP-5；`/code-review` 两轴，钉 `mcp6-awf19@3d31f910`，评审基线 main `dc1f0c07`）

隔离检出 `%TEMP%\idp-review-awf19`（detached `3d31f910`；`apps/admin-web/node_modules` 是指向主树的目录联接，验后先 `rmdir` 联接再拆树）。两轴串行自跑、分开记、不合并排序。自跑核数：`node node_modules/typescript/bin/tsc --noEmit -p .` 退 0；`node scripts/run-tests.mjs` 116 / 116；`git grep -e '正文表可读' -e '正文册可读' -- apps/admin-web/src` 在 `3d31f910` 零命中、在基线 `dc1f0c07` 命中 3 行作阳性对照（同一 shell 同一针，先阳性再报零）；`git diff --name-only dc1f0c07 3d31f910 -- '*.go' '*.sql'` 为空。有效变更只在 `apps/admin-web/src/pages/party/`（`api.ts` 的 `SupplierAgreementRecord` 一段、`SupplierAgreementsPage.tsx`、新文件 `supplier-agreement-rows.ts` / `.test.ts`）与本票面。

#### Standards

阻断：无。

非阻断：

1. **Duplicated Code（判断题，作者 Standards (1)）** — `supplier-agreement-rows.ts` 的 `col` 与 `SupplierAgreementRow`，同 `policy-rows.ts` 的 `col` 与 `PolicyRow` 逐字同形（连 `Readonly<Record<string, string>>` 都一样）：两处 Row 不是「类型不同」，是同一形状起了两个名。作者判「等第三个消费者出现再抬」可接受——本仓无成文规则要求现在合，抬到共享层只是搬家；但下一张字符串行表的页面接进时应把 `col` 与行类型一并抬走，不要再抄第三份。
2. **测试内重复钉列名（nit）** — `supplier-agreement-rows.test.ts` 「只有壳的行照列为未登记」一例手写正文格 id 列表循环断言不在场，与 `supplierAgreementColumns` 的正文段各自维护；新增正文列时这一例不会替新列作证。可从 `supplierAgreementColumns` 取 `'contentRegistered'` 之后的 id 派生，非必须。

其余核过：注释全中文；跨文件引用皆用符号名（`policy-rows.ts` 的 `contentRegisteredCell`、`SupplierAgreementPublicationForm` 版本壳一节、后端 `TestSupplierAgreementsEndpointTranscribesContentOnlyWhenRegistered`），未见行号或跨文件计数，`SupplierAgreementsPage.tsx` 把「通则第六条」改成引用条目原题，合 AGENTS.md「引另一个文件……不用计数」。`orMissing` 以真值判空而不是 `??`：Go 侧 `omitempty` 下空串与缺席同形，坏行落成空串时也该点名，取舍正确。

#### Spec

阻断：无。

「要做的」「边界」「完成判据」逐项在场：

- `api.ts` `SupplierAgreementRecord`：`contentRegistered: boolean` 必在；正文各键可缺，与后端 `supplierAgreementBody` 带 `omitempty` 的字段集逐一对上（`supplier` / `legalEntity` / `purchasePlan` / `agreementScope` / `agreementEffectiveStartsAt` / `agreementEffectiveEndsAt` / `registeredAt`）；头注按本文件 `CustomerContractRecord` 那段的判据写。
- `supplier-agreement-rows.ts` `requiredBodyKeys` 与后端 `supplierAgreementBodyOf` 在 `HasContent` 分支必写的键一致；`agreementEffectiveEndsAt` 另受 `HasAgreementEffectiveEnd` 把守、不在其列——「协议区间不带结束是合法声明」那条测试钉住了这一点。
- 壳在正文缺 → 正文格「未登记」；布尔为真键缺 → 「正文缺失(响应不合契约)」且缺哪键点名哪格；三态与 `policy-rows.ts` `contentRegisteredCell` 同判据。
- 边界：`.go` / `.sql` 零改动；端点串与 `catalogueViewState` 调用未动（只换了计数实参）；过滤仍在已取回数据上做；无方向列；`SupplierAgreementPublicationForm.tsx` 不在 diff。

非阻断 / 判断题（作者六条逐一答复）：

1. （作者 Standards (2)）「协议 / 版本」由两行 JSX 变 `id@version` 单串——README「展示层合成允许」点名 `id@version`，与 `CommercialPoliciesPage` 同款；同页系 `PartyContractsPage.tsx` 仍是两行 JSX，页系内两种呈现并存是既成事实、不是本票造成。接受。
2. （作者 Standards (3)）列头加「版本」前缀——两对同名列并列后不加前缀分不开，词取 `SupplierAgreementPublicationForm` 的格名「版本适用范围」「协议适用范围」，与写签一致。接受。
3. （作者 Spec (a)）只有壳的行正文各格显「—」而非每格「未登记」——票面「照合同页『未登记』如实显示」两读皆通；`PartyContractsPage.tsx` 正文只有两格所以逐格写得起，这里逐格重复是噪音，且 `policy-rows.ts` `jointPassCondition` 已有先例。接受；一处提醒：「—」是模板对缺值的兜底，它读成「未登记」只靠同行「正文」格撑着，若日后正文列增多到需横向滚动、「正文」格滚出视野，这一读法要重估。
4. （作者 Spec (b)）搜索扩到转写后全部格——票面未要，但在「过滤只在已取回数据上做」之内、`CommercialPoliciesPage` 同款，占位文案同步改了。接受。附带一处**行为变化备案**：原实现搜的是 `status` 原码（如 `EFFECTIVE`），现搜的是词表译文（如「已生效」），搜英文原码不再命中；搜屏上所见属改对，记在此不算缺陷。
5. （作者 Spec (c)）测试比票面点名的三例多两条（列集、无上界）——同一模块的钉，不是别的行为。接受。
6. 作者 Standards (1) 见上方 Standards 非阻断 1。

未在浏览器对真后端实看（作者已如实记，本机 `pnpm build` 不可用），证据层级 S 与票面一致；评审不抬这一级。

**结论**：Standards 阻断 0 / 非阻断 2；Spec 阻断 0 / 非阻断 0（判断题六条皆接受，一处行为变化备案）。可进 main；进 main 后 Status 转 resolved 由通道 1 重放时办。本节写在分支 `mcp5-awf19-review`（基 origin/main `62bf6504`，只动本文件），与 `3d31f910` 那笔的「完成记录」同在文末追加，合并时完成记录在前、本节在后。
