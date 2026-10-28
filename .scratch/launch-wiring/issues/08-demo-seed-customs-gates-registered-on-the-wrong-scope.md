# 08 演示种子的门禁目录与「税费付款」规则挂错范围、未覆盖各受守护动作

Category: task
Status: in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-lw-seed（最后一笔见下方提交）。此前：in-progress——2026-10-28 会话 tops-20（idp-parcel-91）立并认领：psb/05 收口重走停点一；分支 claude-lw-seed
Blocked by: 无

## 现象

psb/05「收口重走（钉 `d78fd1fbf`，2026-10-28）」停点一：种子的门禁目录、前置满足与「税费付款」规则行挂在 `SYN-SCOPE-01`，而动作准入读门禁用申报单元的判断范围（`SYN-UNIT-CN-EXPORT-DEMO-01`）；每个受守护动作各要一行目录与一行规则，种子只覆盖 `CROSS_CUSTOMS_MOVEMENT` 的规则与 `FINAL_DELIVERY` 的目录。按种子原样立案后交出、出发、交付门禁读数都是`未决`；按单元范围补登后放行 `FULL` 使交出读数 `ADMITTED`。

## 做什么

只改演示种子（`scripts/demo-seeds/data/customs/*` 与对应步骤脚本）：按申报单元范围登门禁目录、前置满足与税费付款规则，覆盖动线上会走到的每个受守护动作；取值是演示租户的 `S` 级采用，注释写明。不改代码。灌一次性库验证，并按票 05 收口重走记下的关务顺序实打一遍：立案、提交、放行 `FULL`、门禁核对 `MET`、交给干线承运商读数 `ADMITTED`。动线文档若有对应说法同改。

## 完成判据

- 按种子原样（不补登）走关务专项包裹，放行后交出读数不再是`未决`。


## 完成记录

- 起点：origin/main `489db2936`。
- 改了什么：`scripts/demo-seeds/data/customs/34`–`42` 九份新种子，`steps/250-customs.sh` 紧随 SYN-SCOPE-01 那几行登记，注释写明理由与「演示租户 S 级采用，非产品默认」。范围是演示申报单元 `SYN-UNIT-CN-EXPORT-DEMO-01`、边界 `SYN-PROC-CN-EXPORT`：四个受守护动作（`OUTBOUND_RELEASE`、`LOADING_DEPARTURE`、`CROSS_CUSTOMS_MOVEMENT`、`FINAL_DELIVERY`）各一行门禁目录（`34`–`37`），`CROSS_CUSTOMS_MOVEMENT` 一行前置满足 `SYN-PRE-EXPORT-DECL-RELEASED`/`MET`（`38`），四道各一行「税费付款」规则行取「不构成前置条件」（`39`–`42`）。`SYN-SCOPE-01` 上原有的 `08`、`09`、`15`、`32` 保留不动（放行门禁核对演示按商业范围问）。种子 README 与动线文档「收口重走」那条说法同改。未改 Go 代码。号：`33` 已被分支 `claude-psb10` 占用，故从 `34` 起。
- 代决：(1) 只有跨关务区域移动登前置满足，其余三道只登目录，折为`不适用`（与票 05 重走里第二件包裹的读数一致）；(2) 第二个演示单元 `…-DEMO-02` 不预登（动线脚本一库一件，停点四另记）。
- 自验（含 PG，证据只记 `S`）：一次性库 `seed08`，`seed.sh` 退 0。起 `parcel-api`、`parcel-dispatch`（前置照动线），不补登任何行：提交 `201 SUBMITTED`；dispatch 接受；预报后收寄 `201 INTAKE_FORMED`；`declare-one-parcel.sh` 立案 `ESTABLISHED`、授权确认、提交 `DECLARATION_SUBMITTED`；报关服务商令牌 `RELEASE_RESULT`/`FULL`，范围取单元，答 `201 RESULT_RECORDED`；放行门禁核对（单元范围、`CROSS_CUSTOMS_MOVEMENT`）`201 RECORDED`、`MET`、前置 `SYN-PRE-EXPORT-DECL-RELEASED`；同范围另三个动作核对各 `201`、`NOT_APPLICABLE`（不再是目录未登）；干线委托 `v3` `201 COMMISSION_SUBMITTED`；深圳口岸交给干线承运商 `201 HANDOVER_REGISTERED`，`gateReading.answer` 为 **`ADMITTED`**；判断册里 `OUTBOUND_RELEASE` 为 `NOT_GUARDED`/`GATE_NOT_APPLICABLE`、`CROSS_CUSTOMS_MOVEMENT` 为 `ADMITTED`/`RELEASED_IN_FULL`、来源是那条放行结果。走完已停进程、删库、清令牌与二进制。`go test -count=1 -p 1 ./cmd/parcel-customs-register/... ./cmd/parcel-dispatch/... ./cmd/parcel-api/...` 通过。没有测试按路径读 `data/customs`（票面那条 grep 无命中），故读种子的用例按 `demo-seeds` 的几处一并跑。
- 没实打：出发、有效交付两个动作的读数没在进程上走（那两道目录与规则行在位，核对口答`不适用`为证）；订舱与承运答复、SHA→SZX 自营段没走（与本票判据无关，装配不依赖）。
- 留给协调方：无新增停点。
