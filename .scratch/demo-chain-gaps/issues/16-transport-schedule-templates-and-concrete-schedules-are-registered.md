# 16 周期安排与具体班次登记（01·一）

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无
地盘：TF：`domain/transport_schedule_capacity.go`、新 `schedule_template.go`、`payload_canonicalization.go`；`adapters/postgres/transport_schedule.go`、新 `schedule_template.go`、`review_catalogue.go`；`ports/catalogue_read.go`、新 `ports/transport_opportunity.go`；tfhttp 新 `register_transport_schedule*.go`、`query_transport_fulfillment_records.go`；迁移 TF 0024；新 `cmd/parcel-transport-register`；新 `cmd/parcel-api/assemble_transport_schedule.go` + `endpoints.go`/`main.go` 行；种子 `data/transport/`、`seed.sh`、README；文档 ADR-0201、TF CONTEXT、UC-TF-003、PAR-NET-08 拆分。
出处：[01](./01-transport-opportunity-has-no-production-entry.md)；方案「01 运输机会与班次」D01-2、D01-3、D01-4、D01-5、D01-9、D01-11，「跨票通则」X1、X2，「需要的 ADR」ADR-0201。

## 做什么

- TF 自有「周期安排」登记册；周期安排与具体班次走登记册配置写，CLI 与端点同一用例（方案 D01-2、D01-3、X2）。
- 班次方向身份改为 NR 连接代码加起讫节点，按 NR 拼写不透明保存；自由串方向退役（方案 D01-4）。
- 周期文法只内置一种；具体班次标识由（周期安排代码、当地运行日期）确定性派生；物化只在准备时评估候选与显式登记两处，另可登临时班次（方案 D01-9）。
- 查阅口加 `transport-schedule-template`；生产空册答 `未配置`，不建占位班次（方案 D01-5）。
- 演示种子登 SYN 周期安排，各段履约方式照方案 X1。
- 文档：起草 ADR-0201（要点见方案「需要的 ADR」）；TF CONTEXT、UC-TF-003 与 PAR-NET-08 拆分照方案「随票同步的文档」。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 周期安排、具体班次两口和 CLI 同用例。同键同内容答已存在，异内容成新版本。
- （模板、日期）物化确定且幂等（真库）。
- 自由串方向退役（表无生产行）。
- 查阅口新增 `transport-schedule-template`。
- 生产空册答 `未配置`。
- 演示：seed 后查阅口列出三条 SYN 周期安排；显式登一个班次答 201。
- 演示（接 [19](./19-load-assignment-checks-the-schedule-is-registered.md) 移交的那一项，见 19 的「评审记录」）：对上面显式登的那个物化班次提交装载分配答 201，对乱填的班次答 `SCHEDULE_NOT_FOUND`。若本票先于 19 进 main，这一项不归本票，回 19 自己走。

## 规模

L
