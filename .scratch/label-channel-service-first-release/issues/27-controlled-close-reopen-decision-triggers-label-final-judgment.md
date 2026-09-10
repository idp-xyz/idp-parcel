# 27 受控关闭 / 重开决定生效 → `JudgeLabelServiceFinalHandler`：决定口今天连写入方都没有，先要那张写面票

Category: enhancement
Status: draft——通道 3 于 2026-09-10 按 MCP-1 派单 task-b5dba034 立票，取证锚远端 main `c7e3522c`；**只写票面，未动代码。** 触发本身很小，但它挂的那一拍（决定被追加进登记册）今天没有生产路径能到，见 Blocked by
Blocked by: **形成受控关闭 / 重开决定的写面**（命令编排 + PC 授权校验 + 入口）——**今天无票**，见「要裁的」1；[`26`](./26-label-transaction-settlement-beat-triggers-label-final-judgment.md) 的「要裁的」1（同一次调用内 / 后置一拍）裁定后，本票触发形状随之同形，不另裁

## 缺口

票 [`11`](./11-parcel-final-across-transactions.md) Answer「不在本票」节的第三路：**受控关闭 / 重开决定生效**——票 [`10`](./10-continued-attempt-decision-registry.md) 的决定口。lc/11 当时就写了「那口自身还没有生产写入方」；本票取证坐实它，并把「没有」量到符号名。

**决定口今天有什么。** `ports.ContinuedAttemptRegisterRepository{FindByParcel, Insert, Save}`，postgres 实现 `adapters/postgres/continued_attempt_register.go`（`NewContinuedAttemptRegisters`，PS 迁移 `0012`）；领域 `ContinuedAttemptRegister.Append(spec, currentFinalPresent)` 收两种决定 `ControlledClosureDecision` / `ReopeningDecision`，各自必备项由 `decisionFrom` 逐条把守（关闭独有截断边界与关闭责任来源；重开须关联此前关闭且生效时间严格晚于它）。`Append` 头注：「本方法不判断授权够不够格。授权规则属 party-commercial……它也不会自动形成任何决定——CONTEXT 明写『系统不得自动形成』」。

**决定口今天没有什么。** `internal/parcelshipment/application/` 里 `ContinuedAttempt` 只出现在 `judge_label_service_final.go` 及其测试——那是**读**侧（`ContinuedAttemptRegisterView.FindByParcel`）。没有任何编排调 `Append` / `Insert` / `Save`；`cmd/` 下 `ContinuedAttemptRegister` 零命中；无 HTTP 端点、无 CLI、无管理台写面。端口头注自己写着：「**本口今天没有生产写入方，这是设计而不是欠账**：形成关闭或重开决定的命令口要先过 party-commercial 的授权规则校验……那是另一张票」；lc/10 完成记录「刻意没做」同一句。**那「另一张票」至今没立**——`.scratch/` 搜「关闭决定 / 重开决定 / 受控关闭」只命中 lc/10、lc/11、本目录 spec、余工表与几处读面票，没有一张是写面。

**余工表的一句已过期，顺手更正。** `unresolved-review-20260904/remaining-work-a3a4814.md`「面单渠道链」第 2 条写「`RehydrateContinuedAttemptRegister` 仍在 `production_wiring_baseline.txt`」。在 `c7e3522c` 上它**不在名单**：基线文件头部流水账记 2026-09-03 随 lc/10 仓储适配器落地剪掉（「加它时写的『读它的仓储适配器落地那天这一条出名单』兑现」）；名单里的 PS 条目今天没有它。那句话在 09-04 取证时对，此后变旧；本票不改余工表（通道 4 在重核），只记在这里。

**「生效」是哪一拍——代码已答，不必裁。** `ContinuedAttemptRegister.standingClosure()` 顺序扫决定：关闭置位、重开清位，**不看时钟、不比生效时间与当下**；`Judge` 与 `StandingClosure` 都据它。所以在判断眼里，一份关闭决定**被追加进登记册那一刻**即为「生效且未被重开」；`EffectiveAt` 是决定的业务生效时间，终局形成时它成为 `verdict.EffectiveAt()` → 责任结果的 `OccurredAt`，不是一个要等到才翻转的开关。因此本票的触发点就是写面 `Save` 成功那一拍，不需要定时器或节拍去等某个未来时刻。

**重开那一拍也触发但预期不形成。** 重开清掉生效关闭后，`JudgeLabelServiceFinal` 走到关闭路径会答 `NOT_FINAL`（原因 `ContinuedAttemptStillOpen`）；判断不写、不形成时无副作用，多判一次是安全的。两种决定都触发，判断自己分格——不在触发处按决定种类挑，挑了就有第二套关闭路径口径。

## 做法（写面票落地后才可开工）

写面票（「要裁的」1）落地后，它的编排在 `Save` 成功之后多一拍：逐决定所在包裹（登记册键就是租户 + 包裹，一册一件，不必反查覆盖）→ `CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel` 取委托来源身份与委托标识 → 折 `JudgeLabelServiceFinalCommand{Identity, ShipmentRequestID, Parcel}`（不带 `FirstEffectivePickup`，走关闭路径）→ `Handle` → 结果按 `LabelServiceFinalOutcome` 五值译成调用方 / 消费者结论（与票 `25` 同一张表）。

触发形状随 `26` 的裁决：**甲**（同一次调用内）则写面编排的 deps 长判断口与反查口，`Save` 后直接调；**乙**（后置一拍）则写面编排落库同事务入队一封 `parcel-shipment.continued-attempt-decision.*` 指针式信封（租户 + 包裹 + 决定标识；决定标识进事件 ID 让每条决定各自入队），PS inbox 消费者收它 → 读回登记册 → 折命令 → `Handle`；处理方适配器与 `25` / `26` 共用。

采用幂等：关闭路径形成终局时来源版本 = 生效关闭决定的标识（`responsibilityOutcomeOf`：`CONTINUED-ATTEMPT-CLOSURE/<决定标识>`），同一份关闭重放返原；关过—重开—再关是新决定标识，走重派生。

## 要裁的

1. **写面票立不立、立在哪、拆几半。** 它至少两半：**PS 半边**——形成关闭 / 重开决定的命令编排（读当前有效终局作 `currentFinalPresent`、开册或读回、`Append`、`Insert` / `Save`）+ 入口（端点或管理台，形照 `cmd/parcel-api` 既有受控编排壳）+ 请求方 / 实际决定方 / 授权角色 / 授权依据快照四件从授权答复取，PS 不自判；**PC 半边**——授权动作封闭集今天三格（人工复核 / 主动拒绝 / 资料修订，ADR-0116 决定一「一票一格」），关闭与重开**没有格**，要先改 PC CONTEXT 授权那句再加格，形照 ADR-0116 与 pc-gaps/08 那一路；PS 适配器照 `WithdrawalAuthorizationAdapter` 形。归哪个目录（本目录 / ps-port-remainder / party-commercial-context-gaps）、PC 半边要不要 ADR（ADR-0116 的先例是「加格不要，改裁定形状要」）归派单方与 owner。**本票不揉进这两半**——lc/10 与端口头注都把写面判为另一张票，本票只是它落地后的一拍。

## 红线

- 不自动形成任何关闭或重开决定（CONTEXT「系统不得自动形成」；`Append` 头注）；本票只在人形成的决定落库之后判终局。
- 不动 `standingClosure` / `Judge` 的派生规则；「生效」以追加为准，不加时钟、不加定时重判。
- 不在触发处按决定种类挑：两种决定都触发，分格归 `JudgeLabelServiceFinal`。
- 不动 `internal/partycommercial/**`（PC 半边归写面票）。

## 完成判据（非作者评审逐项对）

1. 写面编排 `Save` 成功后判断被调用（甲：替身记录一次调用且命令不带 `FirstEffectivePickup`；乙：outbox 里一封、事件 ID 含决定标识），`Save` 失败 / 版本冲突时不调用 / 不入队——各有用例。
2. 关闭决定后：全部交易明确失败的夹具 → 采用路径收到 `LABEL_SERVICE_FAILURE`；成功结果全部作废的夹具 → 收到 `LABEL_SERVICE_OUTCOME`；仍有有效成功结果 → `NOT_FINAL`。重开决定后 → `NOT_FINAL`（`ContinuedAttemptStillOpen`）。四格各有用例。
3. 同一份关闭重放 → 采用返原（来源版本 = 决定标识）。
4. `LabelServiceFinalOutcome` 五值逐格译成结论，与 `25` / `26` 同一张表（共用适配器则用例证共用）。
5. 乙路另加：消费者三维缺一毒丸、事件类型不符拒收；`cmd/parcel-dispatch/assemble.go` 路由表有该类型；`assemble_test.go` 补一条。
6. `ContinuedAttemptRegisterRepository` 头注「本口今天没有生产写入方」随写面票落地改口（归写面票），本票核对它已改、不留旧话。
7. `gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 地盘

写面票落地后的它那只编排文件（今天不存在，届时占号）；乙路另加 `internal/parcelshipment/adapters/postgres/`（新 handoff）、`internal/parcelshipment/adapters/inbox/`（新消费者）、与 `25` / `26` 共用的处理方适配器所在包、`cmd/parcel-dispatch/`；本票面。**不动** `domain/**`、`internal/partycommercial/**`。

## 参照

票 `11` Answer「不在本票」节；票 `10` 完成记录「刻意没做」；`internal/parcelshipment/ports/ports.go` 的 `ContinuedAttemptRegisterRepository` / `ContinuedAttemptRegisterView` 头注；`internal/parcelshipment/domain/continued_attempt_register.go`（`Append` 头注、`decisionFrom`、`standingClosure`、`Judge` 头注）；`internal/parcelshipment/domain/continued_attempt.go`（`ContinuedAttemptDecisionSpec`、两种决定）；`internal/parcelshipment/adapters/postgres/continued_attempt_register.go`；`internal/parcelshipment/application/judge_label_service_final.go`（`responsibilityOutcomeOf` 关闭那一支）；`internal/architecture/production_wiring_baseline.txt` 头部 2026-09-03 那条流水；PS CONTEXT「面单继续尝试」生命周期与「形成关闭或重开决定时仍须重新校验当前角色与客户授权」；ADR-0084 决定六；ADR-0116 决定一；`adapters/partycommercial/withdrawal_authorization.go`（写面 PS 半边的授权适配器先例）。

## Comments

- 2026-09-10 · 通道 3（task-b5dba034，取证锚 `c7e3522c`）：立票。**只写票面，未动代码。** 能力边界：读过 lc/10 全文、`ContinuedAttemptRegisterRepository` / `View` 头注、`continued_attempt_register.go` 的 `Append` / `decisionFrom` / `standingClosure` / `Judge`、`continued_attempt.go` 的 spec 与两种决定、基线文件头部流水与名单、`application/` 全目录 grep；**没读** `adapters/postgres/continued_attempt_register.go` 全文与 `label_transaction_views.go` 的读面派生。「写面无票」按 `.scratch/` 全目录 grep 判——若有人在别的目录以别的词立过，以那张为先，本票 Blocked by 改指过去。
