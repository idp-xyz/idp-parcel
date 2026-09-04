# CC 四条：步骤在 UC 里、执行器不在编排里——接线

Category: enhancement
Status: resolved——MCP-4，2026-09-04；三组三笔 `c3b4311`（CC-a）、`45c3eeb`（CC-b）、`616646d`（CC-c），四条全部出名单，完成记录见文末 `## 完成记录`
Blocked by: 无（第三组先定 SA→CC 外部资金事实入向缝的形状，见下——已定，见完成记录）

由[票 03](./03-fourteen-that-only-tests-ever-call.md) `## Answer` 立出，按 [spec「处置裁决」](../spec.md) 第 1 条。举证在票 03，此处只列改动对象与完成判据。

## 三组，每组一个编排文件，可各成一笔

**CC-a 凭证登记面一口** — `RegisterCredential` 没有编号 UC，它是登记面：`RegulatoryCredential` 是「监管凭证的不可变版本」，与 `internal/customscompliance/application/register_ports_paths.go`、`register_case_requirement_rule.go`、`register_case_configuration.go` 同形再立一口 `register_credential.go`（签发机构、持有人、适用程序、有效期、次数额度；额度未提供必须明确记录为未提供，不猜测补齐）。第一个消费者是 `UC-CC-003` 步 7「核验监管凭证身份、适用性、有效期和截至当前的可用依据」——接完登记面后核一下 `JudgeReady` 今天读不读凭证（票 03「没能确认的」第二条），不读则消费侧也要补。

**CC-b `UC-CC-006` 步 5 放行层** — `receive_external_result.go` 调 `InterpretExternalResult` + `CheckLayerConsistency`，放行层被解释了但没落成 `CustomsReleaseOutcome`。补 `ReceiveReleaseOutcome`：部分与附条件放行必须带范围/条件说明，未被放行的范围继续受门禁约束。**动手前先读 `InterpretExternalResult` 正文**（票 03「没能确认的」第一条）：若它已把放行语义解释进别的形状，这条是「平行第二写法」，处置是二选一而不是接线。

**CC-c `UC-CC-009` 步 4–7** — `application/` 里没有步 2–7 的任何文件（`verify_release_gate.go` 只做步 10）。新用例文件接两条：
- `FormDutyCollaboration`（步 4–5）：核定税费格 / 明确无需付款格；「缺少税费结果」走不进任何一格，`ErrCollaborationNotFundable` 时编排保持未决，不形成支付指令也不解释为无需付款。不创建支付交易。
- `VerifyDutyPayment`（步 7）：按税费版本、外部引用、范围、付款人、金额、币种、业务时间关联，形成覆盖/差额/有效性三维；无法权威关联时保持外部资金事实待关联。
- 入向缝：`ExternalFundsFactReference` 在 CC 非测试代码里只出现在 `duty_release.go`；外部资金事实在 SA 那头由 `UC-SA-005`（`map_external_funds.go`）形成，到 CC 的缝不存在。先定形状再接。

## 完成判据

- 三组各自：对应 UC 步骤在编排里有一条真实路径走到该领域函数；`git grep -w <符号> -- 'internal/*.go' ':(exclude)*_test.go'` 在 application 层至少命中一处调用。
- 每组带真库用例与应用层测试；先写 red 再读既有编排。
- `production_wiring_baseline.txt` 名单按接线结果剪，在干净检出上重数并带 SHA，不加宽棘轮。
- 不写任何真实程序、税率、口岸取值；程序依据、凭证实例走登记面。
- `gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿（含真库，`-v` 下看 `PASS`）。

## 地盘

`internal/customscompliance/{application,ports,adapters/postgres}`；CC-a 若要新迁移走 `migrations/customs_compliance/` 新号并按「同笔提交」纪律带 `migrations.go`（先占号）。CC-c 的缝若要 SA 侧发东西，先在频道问 settlementaccounting 地盘持有者。`cmd/parcel-api` 端点表若加行另报。

## 完成记录

MCP-4，2026-09-04，基线 `92875ed`，三组各成一笔，按 `/implement` 走、每组先 red 再读既有编排。`migrations.go` 一行未改——`customs_compliance` 目录早已在嵌入清单里，「同笔带 `migrations.go`」那条只对新模块成立。`cmd/parcel-api` 端点表未加行：三组都以应用层编排为生产调用方（棘轮量的正是「domain 包外的非测试引用」），在线登记面/端点归下一批 admin 写面票。

### CC-a `c3b4311`（父 `024cb5f`）——凭证登记面一口

- `application/register_credential.go`：与 `register_ports_paths.go` 同形（写口 DO NOTHING、读回逐字段比、重放/冲突/未决分格）；额度为零按领域约定「来源未提供」原样落册。
- **票 03「没能确认的」第二条答完**：`JudgeReady(unit, basis, judgedAt)` 收的是依据引用，**不读凭证**——所以消费侧同笔补了 `application/judge_credential_applicability.go`（UC-CC-003 步 7 的判断半边，四格：适用 / 不适用 / 凭证未登记 / 未决）。只判不记：步 7 的「记录凭证门禁」随就绪判断的逐门禁编排一起落，今天就绪是带依据引用登记进来的事实、步 3–10 没有逐门禁计算的编排——**这是留给下一张票的**。
- `domain/compliance_judgment.go` 补 `Procedure / ValidFrom / ValidTo / Uses` 四个读口；`ports` 加 `CredentialRegistry` / `CredentialView`；`adapters/postgres/credential_registry.go`；迁移 `0014_regulatory_credential.sql`（一身份一版，无 UPDATE 路径）。
- 基线：函数名剪 `RegisterCredential`（`024cb5f` 上 24→23）；类型剪 `CredentialID / CredentialHolderReference / RegulatoryCredential`（63→60）。

### CC-b `45c3eeb`（父 `a3adb75`）——UC-CC-006 步 5 放行层

- **票 03「没能确认的」第一条答完**：读了 `InterpretExternalResult` 正文——它只记原始语义与规则引用，**不把放行语义解释进任何形状**，所以 `ReceiveReleaseOutcome` 不是平行第二写法，处置是接线。
- `receive_external_result.go`：命令加 `Release *ReleaseContent`（种类/机构/条件，范围沿用结果范围），由接入侧按来源权威语义拆出交进来（与 `VerifyDispositionCommand` 交入已成型 `RegulatoryDecision` 同形——代码到语义的映射是 PAR-CUS-01/02 的实例半边，编排今天只拿得到规则引用）。放行层缺它 → 新未决原因 `RELEASE_SEMANTICS_UNINTERPRETED`（UC-CC-006「外部结果解释未决」在放行层的样子，不落记录、不按文字猜种类）；非放行层带它 → 未受理（硬句 185）；三件立不起领域对象 → 未受理；三件参与来源身份的内容指纹。
- `ports.ExternalResultRecord.Release`；`adapters/postgres/external_result.go` 同事务写 `release_outcome` 并在 `FindByKey` 一并读回；迁移 `0015_release_outcome.sql`（与 `external_result` 同键、外键钉「没有分层事实就没有放行事实」、CHECK 守「条件随种类」）。
- 基线：函数名剪 `ReceiveReleaseOutcome`（`a3adb75` 上 22→21）；类型剪 `CustomsReleaseOutcome / ReleaseKind`（56→54）。

### CC-c `616646d`（父 `f148759`）——UC-CC-009 步 4–7

- **入向缝的形状**：问过 SA 地盘持有者 MCP-3，取证于 `b3d3343`——SA 的 `AdoptFundsFact` 不发信封，SA 适配器无 `external-funds-fact.*` 事件类型。所以缝的 CC 半边立在 CC 内：`ReceiveFundsFact` 入向命令 + `external_funds_fact` 登记册（按事实引用幂等；「待关联」派生——没有核对引用它的事实就是待关联，无状态推进写口）。**SA 事实采用发信封 + CC inbox 消费者接进本口，另立票归 SA**（MCP-3 建议：那封的分区主体取租户/资金事实引用，因为事实的更正/撤销在 SA 是新事实回指原事实）。
- `application/reconcile_duty_payment.go` 三方法：`FormCollaboration`（步 4–5；「缺少税费结果」走不进任何一格 → 业务未决 `DUTY_OBLIGATION_BASIS_ABSENT`，不形成支付指令也不解释为无需付款）、`ReceiveFundsFact`（步 6 的 CC 半边）、`VerifyPayment`（步 7；两道前置各有自己的格——资金事实未接收 / 协作事项未形成；无关联依据保持待关联；三轴集外由领域拒；同三维同内容重放`已存在`、换内容按新版本追加不覆盖）。三轴与关联依据由调用方交进来——真实程序的付款条件与关联规则属实例半边，与 `RegisterGateFinding` 收登记方判断同形。
- `ports` 加 `DutyCollaborationStore` / `ExternalFundsFactRegister` / `DutyVerificationStore`；`domain/duty_release.go` 补 `DutyPaymentVerification.Scope`；`adapters/postgres/duty_payment_reconciliation.go` 三口一型；迁移 `0016_duty_payment_reconciliation.sql` 三表分三层（UC-CC-009「七层对象必须分离」）。
- 基线：函数名剪 `FormDutyCollaboration / VerifyDutyPayment`（`3f4a428` 内容上 19→17，`3f4a428..f148759` 无人碰这两份），**CC 组至此清空**并留「名单清空只判名单清空」的告诫；类型剪 CC 一类余下十二型（47→35）。本笔的基线 blob 按 parallel-sessions「HEAD＋只有自己的块」构造（共享工作副本上同时躺着 MCP-3 SA-c 的未提交 hunk）。

### 验证

- 本机 DSN 指 `idp-parcel-postgres-gate`：`gofmt -l internal/customscompliance/` 空；`go vet ./internal/customscompliance/...` 退 0；`go test -count=1 ./internal/customscompliance/...` 全 ok，`adapters/postgres` **真库 PASS**（新增 `credential_registry_test.go` 六例、`release_outcome_test.go` 四例、`duty_payment_reconciliation_test.go` 六例）。
- 三笔各自在 `$TEMP` 干净 detached worktree 检出后：`go build ./...` / `go vet ./...` 退 0、`gofmt -l .` 空、`./internal/architecture/` ok、`./cmd/parcel-api/` ok；`616646d` 上另加 `./migrations/` ok。共享树上两道棘轮期间反复对 SA/VE 报红，全是 MCP-3/MCP-6 的在途件，逐次核过不是本票的。
- 未跑 `-race`（本机 CGO 状态见 workflow.md），全仓 `go test ./...` 归推送方（MCP-1）。

### 没做、留给后继票的

1. UC-CC-003 步 7「记录凭证门禁」的持久化——等就绪判断的逐门禁编排。
2. 放行层的代码映射（PAR-CUS-01/02）——实例半边；到位后接入侧译装出 `ReleaseContent`，编排不改。
3. UC-CC-009 步 8 核对向 `settlement-accounting` 的交接（outbox 意图）与步 10 门禁核对读付款核对——各一张。
4. SA 事实采用发信封 + CC inbox 消费者（归 SA 立）。
5. 三组的在线登记面/端点与 admin 写面。
