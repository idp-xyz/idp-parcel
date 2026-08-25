# 06 cmd/parcel-api 装配:七端点登记

Category: feature
Status: draft
Owner: MCP-2
Blocked by: 02, 03, 04, 05

四张后端票落库后,把七个查询端点(计价二、网络一、关务一、商业二…以各票实际构造器
为准)登进 `cmd/parcel-api`:main 构造真库读适配器交入、endpoints.go 端点表登记、
endpoints_test.go 装配测试跟上、unwired_orchestration.go 若需桩位按 267cb44 先例处理。

共享文件纪律:改前在频道广播占号(migrations.go 同款三条:占号/同笔/`add` 前逐块核),
四件同一笔提交。

## 完成标准

全仓 build/vet 零告警,`go test -count=1 ./...` 全绿(写明含不含 PG),自己提交,
票面 resolved + SHA,回频道 2 并释号。

## Comments
