# 40 PC、PP 三处 Go 注释用计数引别处的文档，其中一处已经数错

Category: chore
Status: resolved——2026-10-27 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-cmtB
Blocked by: 无
出处：[audit/pc-pp.md](../audit/pc-pp.md) L6（审计钉 `6b0074d0`；2026-10-21 按 main `73a2c4a79` 复核，三处注释原样在）。

## 现象

三处注释用「几种」「几项之一」「只留几格」指称另一个文件里的清单。

## 证据

- `internal/parcelpricing/domain/value_objects.go` 的 `ChargeMethod` 注释：「CONTEXT 把取值集合闭合为四种」。PP CONTEXT「计算方法」词条列的是定额、查表、按基数百分比、取当期序列定额，以及其中两者取较大值；代码常量含 `ChargeMethodSeriesAmount`。已经数错。
- `internal/partycommercial/domain/credit_limit.go`：「「入账余额」是 SA CONTEXT 运营结算余额五项之一的原词」。今天数得对，被数的是 SA CONTEXT。
- `internal/partycommercial/adapters/postgres/commercial_resolution.go` 的 `settlementPolicyDocument.policy` 注释：「ADR-0044 只留两格」。今天数得对，被数的是 ADR-0044 的方式集合。

## 三类

机制：注释写法。产品策略、租户取值：无。

## 规模

S：三处注释。

## 最小的诚实修法

三处都改为引词条名或原词，不写数：`ChargeMethod` 注释改为「取值集合由 CONTEXT「计算方法」词条闭合」；`credit_limit.go` 改为「「入账余额」是 SA CONTEXT「运营结算余额」的原词」；`commercial_resolution.go` 改为「ADR-0044 的方式只有预付与账期」——数本身不是论点，不必锚 SHA。

## 要裁的

无。

## 完成记录（2026-10-27，作者会话，分支 claude-cmtB，基 origin/main `f6a4a5298`）

只改注释，去掉注释行后 `git diff -U0` 无变动。三处位置都仍在原处，没有漂移。

### 逐处结果

| 位置 | 结果 |
|---|---|
| `parcelpricing/domain/value_objects.go` `ChargeMethod` | 已改：「取值集合由 CONTEXT「计算方法」词条闭合」 |
| `partycommercial/domain/credit_limit.go` | 已改：「入账余额」是 SA CONTEXT 规则句「运营结算余额必须区分入账余额……」里的原词。票里建议写「SA CONTEXT「运营结算余额」的原词」，核对后词条「运营结算余额」本身的释义里没有「入账余额」，这个词出现在 Rules 那一句里，所以点名规则句更准 |
| `partycommercial/adapters/postgres/commercial_resolution.go` `policy` | 已改：「ADR-0044 的方式只有预付与账期」 |

### 同类残留（PC、PP 包内）

- 同一错数在别处：`parcelpricing/domain/plan_structures.go` 的 `NewSeriesRateSurcharge` 也写「CONTEXT 把计算方法闭合为四种」，改为词条名；`domain/evaluation.go` `EvaluationUnratable` 的「CONTEXT 不许四种结果互相顶替」改为「这几类结果」。
- 非原文引文，改成原文或不带引号的转述：`commercial_resolution.go` 的「含则必填、不含则必缺」（ADR-0044 决定二实际写的是「请求结算依据必填，其余请求必缺」；该短语是 ADR-0129 对它的转述）；`publish_commercial_authority.go` 的 UC-PC-001「原子保存单一对象版本与发布意图」（原文还有「关系、审计」）；`publication_vocabulary.go` 的 ADR-0078 判据「消费本上下文自己的存储读面」（原文「消费所属上下文的存储读面」）；`query_party_identities.go` 的 CONTEXT「哪一方对哪一方持有该角色」（CONTEXT 没有这句，改引 Rules 里「参与方关系必须保存双方、角色、方向……」）；`parcelpricing/domain/published_record.go` 的 ADR-0092「摘要在收到本体那一刻算」（原文多一个「要」）；`plan_structures.go` 的 ADR-0107「未声明即不取整并记问题项」（原文有「如实」，改为不带引号）。
- 错指的决定号：`ports/operator_registration_handoff.go` 说「ADR-0094 决定四原话『不在消费门认原因名字』」——决定四讲的是续办触发同笔落地，消费门不认原因取值的是决定一，且原文没有这句；改为转述并指决定一。同一句里的「第四格」是 `ResumePath` 的序数，改为「一格」。
- 变更叙述与票号：`domain/reference_closure.go` `ValidateClosureBeforeDecision` 里的「今天唯一的生产形态」「曾另有一个单依据形态……随票……删去」去掉，UC-PC-002 的「步 8『提交前重解』」（不是原文）改引应用流程里「业务决定提交前校验解析和关键结果仍相容」一步。

### 代决

1. 没有逐一改写 PC、PP 包里所有 `Covers:` 与头注里带引号的概念名：机器核对出的不匹配里大半是写注释的人用引号标一个概念（如「每主体一次」「结构上算不出来」），或用省略号缩写原句，不是在引文，不改；只改了确实声称在引某个文件原句而原句对不上的。测试文件里同类的不匹配（`adjudicate_commercial_authorization_test.go`、`validate_commercial_basis_test.go`、`customer_service_rule_product_base_fixing_test.go` 等十来处）本票没动，列在越权风险点。
2. 「不写数」只针对数别的文件的清单；注释对本函数、本类型自己列出的成员数数（如「第二阶段四种未成形」之后逐一列出）保留。

### 越权风险点

- 测试文件里仍有一批引文与原文不逐字相同（未逐个核）；不影响行为，下一轮文档自洽可顺手清。
- `partycommercial/adapters/http/publication_vocabulary.go` 里「那是另一张票（票 20 完成记录列为判断点）」是指向票面的流水指针，没动。

## 进 main 记录（2026-10-27，协调方）

- 分支 `claude-cmtB` 与 code/38、40、45、55 同批，由协调方快进，tip `c6f983ea2`；隔离评审（Sonnet）可合入，无阻断：每个改动的 `.go` 文件在去掉注释后语法树逐个比对（程序放评审私有目录，对有代码变动的老提交试过会报差异），38、40、45 三个提交 0 差异；新增的 38 条带引号或「」的引文机械核了 36 条逐字相同，另 2 条是带省略号的前缀引文与一句假想读者的话；新增的 ADR、UC、AT 指针逐条核过。协调方单独提交重算的 AT 计数、重生成的 ADR 索引与机制清点（本批无变动）；全量 `scripts/test-full.sh` 退 0、159 个包 ok；`go build ./...`、`go vet ./...` 干净；管理台 `tsc -b --noEmit`、`node scripts/run-tests.mjs`（803 过）、`vite build` 与 `./internal/architecture/...` 全过。无迁移、无新 ADR、无 AT 号变化。
- 本票：三处计数引文都改了，同一个错数（「四种」）在 `parcelpricing/domain/plan_structures.go` 与 `domain/evaluation.go` 还有，一并改；`ports/operator_registration_handoff.go` 里错指的决定号（决定四应为决定一）订正；对不上原文的 ADR、UC、CONTEXT 引文几处订正。`internal/partycommercial` 一处注释改动已向另一会话（tops-54）确认没有在飞改动。
- 去向：无新票。测试文件里仍有十来处引文与原文不完全相同，只做了机器核对，已记在票面越权风险点。
