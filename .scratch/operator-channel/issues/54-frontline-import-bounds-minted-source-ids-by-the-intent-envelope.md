# 54 前线导入铸出的来源标识越过意图信封上限时，报告说「重跑会重发」，重跑却永远交不出去

Category: bug
Status: ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立并代决（票 operator-channel/45 评审遗留）
Blocked by: operator-channel/45 进 main
父票：[45](./45-http-reception-does-not-swallow-a-failed-intent-enqueue.md)
出处：票 45 实施子代理回报的「没做」与会话评审。
地盘：
- cmd/parcel-frontline-import/intake_template.go 及其测试
- cmd/parcel-frontline-import/intake_import.go（只在报告措辞要跟着改时）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（读码，未实测）

收寄意图信封的 Subject 原样取来源标识（NO 的 `OutboxNodeIntakeHandoff`），越过框架的 `eventing.MaxSubjectLength` 或带控制字符就被确定性拒收。票 45 让 HTTP 设备路在接入时按这条上限拒成坏报文；前线导入路的来源标识由 `intakeSourceID` 从批次与事实两格铸出，没有同样的门。超长时收寄照常落册，意图入队失败被编排吞成续办引用，报告写「意图未交出（…），重跑同一文件会重发同一份」——而重跑每次都会撞同一道拒收。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决；机制，不涉租户取值）

在模板解码那一步按铸出的来源标识判：越过 `eventing.MaxSubjectLength` 或带控制字符，这一行按坏行进报告、不形成收寄，措辞让内勤知道是批次或事实编号太长。上限直接引 `eventing.MaxSubjectLength`，不另写数。不改编排吞错的写法（导入路仍要续办）。

## 完成判据

- [ ] 用例：批次与事实编号拼出的来源标识恰在上限内的一行照常形成收寄并交出意图（真库）；越过一个字节的那一行答坏行、册上不落、报告点名；带控制字符同。修前越界那一行先红。
- [ ] `go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `cmd/parcel-frontline-import` 与 `internal/architecture` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）。
