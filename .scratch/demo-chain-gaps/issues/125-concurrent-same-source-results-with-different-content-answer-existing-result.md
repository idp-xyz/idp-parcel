# 125 同一来源标识、内容不同的两份关务结果并发到达：后到那份答「已有记录」，不答「来源冲突」

Category: bug
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 Claude Code 协调方立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [12](./12-successive-same-layer-customs-results-are-judged-conflicts.md) 的评审修订（作者「顺带核得、未改」那一条）。
Blocked by: 无
出处：[12](./12-successive-same-layer-customs-results-are-judged-conflicts.md) 票面「评审记录」；[ADR-0226](../../../docs/adr/0226-successive-same-layer-customs-results-are-successors-unless-they-contradict.md) 决定二（同层一致性比对按提交版本串行，不覆盖幂等读）。

## 现象

CC 收外部结果时先按幂等键读已有记录：读到了就比内容摘要，摘要不同答 `SOURCE_CONFLICT`。两份同一来源标识、内容不同的结果并发到达时，两笔事务在这一步都读不到对方，都往下走；后落册的那笔撞键，读回赢家后直接按已有记录作答，答 `EXISTING_RESULT` 并重发赢家的意图。来源方因此以为自己那份已被接受，实际上被静默丢弃，「同一来源前后说法不一」这件事没有留痕。

票 12 加的咨询锁只串行同层一致性比对那一段（在幂等读之后），挡不住这一格。这一缝在票 12 之前就有。

## 证据（读码）

`internal/customscompliance/application/receive_external_result.go`：`Handle` 开头 `FindByKey` 读到记录时经 `domain.CompareStoredDigest` 区分已有与来源冲突；落册那一步遇 `ports.ExternalResultAlreadyRecorded` 时读回赢家，交给 `existingResult` 作答，不再比摘要。

同形追记（2026-10-21，协调方；出自 [product-strategy-boundary/18](../../product-strategy-boundary/issues/18-customs-case-and-declaration-submission-entry.md) 的隔离评审）：`internal/customscompliance/application/submit_declaration.go` 的 `SubmitDeclarationHandler.commit` 遇 `ports.DeclarationSubmissionAlreadyRecorded` 时同样读回赢家、交给 `existingResult` 答 `EXISTING_VERSION`，不比内容指纹。票 18 给提交执行开了生产口（`POST /customs-declaration-submissions`），这一格因此生产可达：两份内容不同的提交并发到达，后到那份答已有版本而不是来源冲突。修法同下，一并改。

同形追记（2026-10-21，出自 [demo-chain-gaps/140](./140-operators-adopt-external-funds-facts-and-regulatory-credentials.md) 的隔离评审）：`internal/settlementaccounting/application/map_external_funds.go` 的 `AdoptFact`、`CorrectFact` 遇 `ports.FundsFactAlreadyAdopted` 时读回赢家（`FindByKey` 交回的链头）交给 `existingFact`，不比内容摘要；在线口（集成客户端族与票 140 开的手工口）的答复只有 `outcome`、不带记录，输掉竞态的一方无从分辨自己那份被丢了。三种情形：

- (a) 同一事实、同一版本字面、金额不同的两份并发采用：后到那份答 200 `EXISTING_FUNDS_FACT`，被静默丢弃（顺序到达答 `FUNDS_FACT_CONFLICT`）。
- (b) 同一事实、首版版本字面不同的两份并发采用（撞「一条事实只有一个首版」的部分唯一索引）：同上，后到答已存在（顺序到达答冲突）。
- (c) 两份更正并发回指同一链头、新版本字面不同（撞「一个前版只被更正一次」的唯一约束）：后到答已存在，交回的链头是对方那一版（顺序到达答 `SOURCE_NOT_ACCEPTED`，即`未受理`）。

最现实的是两族并发：财务照回单在管理台录入的同时，银行接口推来同一事实（同一 `factRef`）。修法同下：读回赢家之后照开头那一步比内容摘要，一致答已存在，不一致按顺序到达时的同一格作答（(a)(b) 答冲突，(c) 答未受理）；补真库并发用例。`internal/settlementaccounting/adapters/postgres/external_funds_fact.go` 的 `ExternalFundsFacts.Save` 头注与 `ports.FundsFactAlreadyAdopted` 注释原先把这一格写成「有意的」，票 140 已改成指向本票的缺陷说法。

CC 的监管凭证登记那一路**没有**这个问题，别误改：`RegisterCredentialHandler.Handle` 落册撞键（`CaseConfigurationAlreadyRegistered`）之后读回在册版本、按 CCC-1 比内容，异内容答 `CONTENT_CONFLICT`。

## 三类

机制。规模 S。

## 最小的诚实修法

落册撞键、读回赢家之后，照开头那一步同样比内容摘要：一致答 `EXISTING_RESULT`，不一致答 `SOURCE_CONFLICT`。补一条真库并发用例：同一来源标识、内容不同的两份并发到达，恰有一份落册，另一份答 `SOURCE_CONFLICT`。申报提交执行那一路（上面同形追记）同改；顺带核一下 CC 别的接收口（申报回执等）在撞键分支上是否同病。

## 要裁的

无（与开头那一步同一判据，不另立口径）。

## 既有票

none
