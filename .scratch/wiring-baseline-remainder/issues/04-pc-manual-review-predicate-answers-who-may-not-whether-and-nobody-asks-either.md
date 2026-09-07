# 人工复核谓词：「要不要」已由接单规则正文答，「谁有权」这一问的消费方（复核完成授权）尚无端口

Category: enhancement
Status: draft——只读取证（MCP-6，锚 `2efef58e`），PC 地盘归 MCP-3；交 MCP-1 派
Blocked by: UC-PC-003「四、人工复核那一格的措辞与分工对不齐」那条裁决（改名为「谁有权」形态，或拆成两问）

## 条目

`internal/partycommercial/domain ManualReviewRequirementFor`（`authority_grant.go`）。基线理由行：「返回 bool 是个谓词……留着不排除是有意的」——讲的是族界，不是活性。

## 它是什么

`ManualReviewRequirementFor(grants, scope, at) bool`：授权授予册里该范围此刻有生效的 `ManualReviewAction` 授予即答真；注释「回答某个范围是否要求人工复核。沉默意味着不要求」。`ManualReviewAction` 是 `AuthorizedAction` 封闭二值之一（另一格主动拒绝），登记口已接（`adapters/postgres/authorization_grant.go` 能读回这一格）。

## 「要不要」今天走的是另一条路，且已接通

接单规则正文 `AcceptanceRuleContent.ManualReview()`（`ManualReviewDirective`，UC-PC-001 发布时随规则包声明）→ PS `CommercialBasisSnapshot.ManualReviewPolicy()` → `parcelshipment/application/form_acceptance_decision.go`（未声明即 `ManualReviewPolicyNotDeclared` 未决，不默认）。CONTEXT 硬句「接单规则包必须明确……哪些条件确实需要人工业务复核」落的正是这条。所以按函数今天的注释读，它是那条路的**平行第二写法**——两处各答一遍「要不要」，而 PS 只读其中一处。

## UC-PC-003 已经点破这件事

「四、人工复核那一格的措辞与分工对不齐。ADR-0042 明确把既有的 `ManualReviewRequirementFor(grants…)` 归为授权治理（**谁有权**复核），而该函数今天的名字与注释写的是『回答某个范围是否要求人工复核』（**要不要**）。两种读法在同一个函数上并存，正是 ADR-0042 要拦的并格。……在人工复核经本端口对外暴露之前必须先定。」首切只覆盖主动拒绝，人工复核那格不在首切内。

## 该有的调用方（按 ADR-0042 的读法）

UC-PC-003 的第二切——复核**完成**的授权裁定。PS `application/complete_manual_review.go` 的 `ManualReviewCompletion` 带三项引用（复核授权、复核人、证据），今天由 Intake 从认证结果整组注入（`adapters/http/unconfigured_intake.go` 自注「复核人与授权引用整组来自认证结果，采信自报的……不算」），**没有人向 PC 问「这个角色在这个范围此刻有没有复核授权」**。对应 PS 侧该有与 `ActiveRejectionAuthorizer` 同形的复核授权端口（三值答复），PC 侧经 UC-PC-003 的裁定用例答它——而那个用例读授权授予册时调的就是这个谓词（改名后）。

## 三分

**支路未接**，前置一格命名 / 分工裁决。不是死码：「谁有权复核」这一问在 UC-PC-003 有步骤、在授予册有数据、在 PS 有消费点（复核完成的授权引用），只是三处没连起来。删掉它等于把「谁有权」这一问也删了。

## 能不能归到已认可的留待

不能：缺的是端口与用例，不是实例值。

## 边界

本票不改代码、不改基线。裁决落地（改名 / 拆分）时理由行同步改写；剪基线行的时刻是 UC-PC-003 第二切的裁定用例真调它那一笔。
