# 单依据形态的提交前重解已被闭包形态取代：`ValidateBeforeDecision` 是留下的第一版

Category: chore
Status: resolved——2026-09-08，MCP-6（task 5f716c71，用户 02:0x 自 MCP-3 改派；分支 `mcp6-wbr03-05` 基 `4524cfd4`）：`ValidateBeforeDecision` 及只为它写的用例删去，`ValidateClosureBeforeDecision` 注释改历史注，基线行剪掉、头注记成因第一种并钉 SHA 两法同得 7→6，PC 组族界那句改成只指 `ManualReviewRequirementFor`；完成记录见 Comments。此前 draft：只读取证（MCP-6，锚 `2efef58e`），PC 地盘归 MCP-3；交 MCP-1 派
Blocked by: 无（纯删，PC 地盘内）

## 条目

`internal/partycommercial/domain ValidateBeforeDecision`（`commercial_resolution.go`）。基线理由行：「返回同型 Resolution 是变换不是构造。留着不排除是有意的」；triage spec 另注「开发主线 PN-02 行把提交前重解写成已落地」并视之为张力——**那不是张力，落地的是闭包形态**，见下。

## 它是什么

`ValidateBeforeDecision(registry, prior Resolution, standingOf) Resolution`：UC-PC-002 步 8 的提交前重解，**单依据**形态——按原查询重跑第一阶段（`ResolveCommercialBasis`），权威读不到保持`解析未决`，结果变了答`已失效`。

## 今天的生产路径

- `application/validate_commercial_basis.go` 调 `domain.ValidateClosureBeforeDecision`（`reference_closure.go`）——**闭包**形态，注释自称「与单依据侧的 ValidateBeforeDecision 是同一条规则的闭包形态」。
- 闭包形态**不调**单依据形态：两者各自内部调 `ResolveCommercialBasis` / `ResolveCommercialClosure`。所以单依据**解析**（`ResolveCommercialBasis`）活着——闭包逐项解析时调它；单依据**重解**（`ValidateBeforeDecision`）没有任何调用方，连测试外的同包引用也没有。
- `application/resolve_commercial_basis.go` 只走闭包；PS 消费端口 `ports.CommercialBasisResolver.ResolveCommercialBasis` 名字虽同，适配器（`parcelshipment/adapters/partycommercial/commercial_basis.go`）走的是 PC 的闭包用例。

## 三分

**死码**（被取代的第一版，与 PP 那对方案层快照门同一类）。基线那句「变换不是构造、留着不排除」讲的是它该不该在网内，与它还活不活是两个正交的问题——族界成立不妨碍它是死码。

## 建议的处置（PC 地盘做）

1. 删 `ValidateBeforeDecision` 及只为它写的测试；
2. `ValidateClosureBeforeDecision` 注释里「与单依据侧的 ValidateBeforeDecision 是同一条规则的闭包形态」改成历史注（曾有单依据形态，已删，理由：闭包形态是唯一生产路径）——否则那句在删掉之后指向一个不存在的名字；
3. 剪基线行，按头注纪律记数（三分成因写第一种）；
4. 顺带看一眼单依据 `Resolution` 族里还有没有同样只剩闭包内部在用的导出函数——`ResolveCommercialBasis` 有闭包这个同包调用方所以不在名单上，但若它日后也被闭包内联，会是同一格。

## 能不能归到已认可的留待

不适用：它不缺调用方，它被替代了。

## 完成判据（落地那笔连基线一起改；MCP-1 2026-09-07 裁）

1. `ValidateBeforeDecision` 及只为它写的测试删去；`ValidateClosureBeforeDecision` 的注释改成历史注（曾有单依据形态，已删；闭包形态是唯一生产路径），不再指向一个不存在的名字。
2. 剪基线行，头注记一句成因第一种（已删，被闭包形态取代）；在自己那笔的干净检出上两法同得记数、钉 SHA。
3. PC 组那句「下面两条落在网里但不是工厂：ManualReviewRequirementFor 返回 bool 是个谓词，ValidateBeforeDecision 返回同型 Resolution 是变换不是构造」改成只指 `ManualReviewRequirementFor` 一条（票 04 落地时再由它改写成理由行）——否则它在删掉之后指着一个不在名单上的名字，而 `TestWiringBaselineHasNoStaleEntry` 不读注释，不会为此红。
4. 顺带核一遍单依据 `Resolution` 族里有没有同样只剩闭包内部在用的导出函数，有就在本票 Comments 记名，不在本票删。

## 边界

本票不改代码、不改基线。（立票时的边界；落地笔见 Comments。）

## Comments

- 2026-09-08 02:2x · MCP-6（task 5f716c71，用户 02:0x 自崩溃的 MCP-3 改派；分支 `mcp6-wbr03-05` 基 origin/main `4524cfd4`）：
  **完成记录，转 resolved（剪，成因第一种）。** 完成判据逐项：① 删 `domain.ValidateBeforeDecision`（`commercial_resolution.go`）
  与只为它写的用例——`commercial_resolution_staleness_test.go` 六条（同范围新增候选即 STALE / 权威视图未变可用 / 重解换新标识 /
  别范围不影响 / 权威不可读停未决 / 只有唯一解析能重解），文件留下不涉它的 `TestResolutionCarriesTheAuthorityViewRevision` 一条
  并在头部记了删去的六条；`commercial_resolution_pending_test.go` 两条（`TestPendingFromPreDecisionValidationAlsoNamesItsReason` /
  `TestStaleNamesTheCauseThatOverturnedIt`），头注那句改成历史注。`ValidateClosureBeforeDecision` 注释改为「今天唯一的生产形态；
  曾有单依据第一版，已删；单依据解析 `ResolveCommercialBasis` 仍在、本函数逐项解析时调它」。`ResolutionStale` /
  `CurrentResolutionChanged` 两枚举值不动：闭包形态仍产出它们（PS / VE 消费适配器仍按它们分格）。② 剪基线行；头注加一段：成因
  第一种、剪前先核全仓此名只此一处声明；**本笔单独作用于父提交 `4524cfd4`，两法（PowerShell 与 git-bash grep 各数 `^internal/`
  行）同得剪前 7、剪后 6（party-commercial 5→4）**，只对该检出成立。剪前 `TestWiringBaselineHasNoStaleEntry` 按预期红并点名此条，
  剪后 `internal/architecture` 全绿。③ PC 组那句改成只指 `ManualReviewRequirementFor`（「下面一条……」），并以括注记下被删的那条
  与成因，票 04 落地时再由它改写成理由行。顺带：`production_type_reachability_baseline.txt` 里 `Resolution` 那条的注释原写
  「只经 ResolveCommercialBasis 与 ValidateBeforeDecision 取得」，同笔改成历史注（那门禁不读注释，不改也不红，但不能让它指着
  一个不存在的名字）。④ 顺带核单依据 `Resolution` 族（`commercial_resolution.go` 导出函数）：`ResolveCommercialBasis` 的生产调用
  方只剩闭包（`reference_closure.go` 同包），包外零非测试引用——与票面预判一致，闭包若日后内联它就是同一格，**本票不删**；另见
  `NewContinuationReference` 全仓零调用（连测试也没有），`New*` 前缀被函数名门禁跳过，记名不删。**不做的**：不动 `ResolveCommercialBasis`；
  不改 spec 状态行（三票都完后改一次）。**验证**：`gofmt -l` 空；`go build ./...` / `go vet ./internal/partycommercial/...` 退 0；
  `internal/partycommercial/domain` 与 `internal/architecture` 各 ok；全仓含 DSN 在三票收口的 tip 上一次跑（见 03 / 04 那笔记录）。
- 评审 ← 通道 2 · 钉 `c1dcbd90` · 10:54（基线 `4524cfd4`，隔离树 `%TEMP%\idp-review-wbr05`，只评 `db723eb1`；由 MCP-6 落票面）：
  **Standards** — 阻断：无。非阻断：(1) `internal/partycommercial/domain/commercial_resolution_staleness_test.go` 新头注「同一组规则的
  用例在 reference_closure_test.go」指错：该文件无一处调 `ValidateClosureBeforeDecision`；闭包形态重解的用例在
  `application/validate_commercial_basis_test.go`（`TestARevalidationSeesANewCandidateAndReportsTheResolutionStale` 等）。AGENTS.md
  「写代码注释」——引另一文件要指得准；改一个文件名即可，建议同分支顺手。(2) 删掉的八条里一条断言现无人钉：「只有唯一解析能被重解」——
  `ValidateClosureBeforeDecision` 首行守卫 `prior.outcome != UniquelyResolved → return prior`，domain 层无人调、application 层五条 prior
  全是 UniquelyResolved。删法合票面，可另立票补一条闭包形态小测试。其余七条均有等价：STALE / 续办 / 不带 adopted、不变同 ID（app）；
  换新 ID（`commercial_resolution_test.go` afterChange 断言）；别范围不影响（scope-other + app 按原键重解）；权威不可读→Pending +
  AuthorityUnreadable + 续办（app）；STALE 原因（PS 适配器 `commercial_basis_test.go` “PC-CURRENT_RESOLUTION_CHANGED”）。(3) 信息：
  `.scratch/at-coverage-inventory.md` AT-PC-024 / 026 两行仍点名被删测试；盘点钉在 08-13 的 SHA，同行其余测试仍钉住，无覆盖损失，
  记名不动。无发现：零调用方属实；闭包形态三分支与被删函数逐一对应；两枚枚举保留正确；PC CONTEXT / UC-PC-002 步 8 措辞不点形态，
  不需改。**Spec**（票 05 完成判据 1–4）— 阻断：无。非阻断：无。①–④ 逐项对上；基线计数以 `git grep -c '^internal/'` 在
  `4524cfd4` / `db723eb1` / `c1dcbd90` 复现 7 / 6 / 6、PC 5 / 4 / 4；`production_type_reachability_baseline.txt` 那句是删名的必要后果，
  不算 creep；spec 状态行未动。附：隔离树 `go test -count=1` architecture / PC domain / PC application 三包 ok。**结论：无阻断。**
- 2026-09-08 11:0x · MCP-6：落上条评审。非阻断 (1) 于 `c1dcbd90` 复核属实（`ValidateClosureBeforeDecision` 的引用文件里没有
  `reference_closure_test.go`），文件名另起一笔改（不混进纯 .md 笔）；(2) 记名，补不补闭包形态小测试随 wbr 批收口时一并裁；(3) 记名不动。
