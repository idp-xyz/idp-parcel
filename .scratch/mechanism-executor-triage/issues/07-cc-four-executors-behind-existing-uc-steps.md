# CC 四条：步骤在 UC 里、执行器不在编排里——接线

Category: enhancement
Status: ready-for-agent
Blocked by: 无（第三组先定 SA→CC 外部资金事实入向缝的形状，见下）

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
