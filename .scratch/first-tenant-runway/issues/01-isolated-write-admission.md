# 隔离形态写面放行——把 ADR-0078 的形状从读面扩到写面

Category: enhancement
Status: ready-for-human

演示动线三堵墙的**墙一**。取证基线 `c9835bf`。

## 墙

[演示动线脚本](../../../docs/design/synthetic-demo-journey-script.md)第 5 步：`POST /shipment-requests` 与 `POST /shipment-requests/parcel-cancellations` 答 `403` + `ACCESS_CHANNEL_NOT_CONFIGURED`。成因是各上下文的写端点一律装 `UnconfiguredIntake{}`（`cmd/parcel-api` 的 `assembleBusinessEndpoints`），而 [ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 的隔离放行面只枚举运营查阅那几行，**把写端点显式排除在外**。

脚本给的重启条件是「`PAR-INT-01` 最低证据到位」——那是**生产**放行的条件，属实例半边。本票问的是另一件事：**隔离演示环境要不要一条同样苛刻的写面放行。**

## 做什么

一份新 ADR，外加按它落地的装配改动。

ADR 要裁的是一句话：**隔离形态下按装配注入放行写端点，是否与 ADR-0055/0072 的裁定相容。**我的判断是相容，理由写在这里供裁决时驳：那两份 ADR 否决的是**运行时渠道登记表**（登记册形状等真实渠道证据、不预先替租户拟），而隔离放行根本不是登记表——它是装配期注入，生产装配里那条路径压根不存在。两者管的不是同一件事。

**开工第一步必须先把 ADR-0078 的 Decision 逐条读完。**如果它当初把「写面排除」写成了带理由的正面裁定（而不只是划定了本次范围），本票的论证要重做，不能靠「它没说不行」推进。

形状直接照抄 ADR-0078，一处不改：

- 装配期注入，不读任何运行时登记表；
- 启动必出声（同 ADR-0078 那行 `INFO`，放行的是写面这件事要在事后可查）；
- `SYN-` 前缀守卫，非 `SYN-` 前缀**进程启动即拒**、带原因退出、不静默回落；
- 生产装配里不存在通往它的代码路径。

## 必须守住的一格

**放行的是「有没有渠道」这道门，不是它后面的任何一道。** 过了 Intake 之后，提交编排仍要走完真实的来源保全、授权、生产归属、接受判断——墙二、墙三照旧拦着。本票若做完发现委托能一路建成，那说明放行放过头了，要回头查。

## 完成判据

ADR 落文并被接受；`cmd/parcel-api` 装配点按它改；三态对照（不设变量 / `SYN-TENANT-01` / 非 `SYN-` 前缀）在写面逐条复现并记进演示动线脚本；`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

演示动线脚本第 5 步与「对照组」两节同笔更新——本票改了那两节描述的行为，不留「行为已变、脚本仍说 403」的中间态。

## 参照

ADR-0078（隔离环境运营读按装配注入放行）、[ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0072](../../../docs/adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)；`cmd/parcel-api` 的 `assembleBusinessEndpoints` 与 `unwired_orchestration.go`；`.scratch/syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md`。
