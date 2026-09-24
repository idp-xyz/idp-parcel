# 18 关务立案与提交申报：生产入口与触发面

Category: enhancement
Status: needs-triage——2026-09-24 通道 2 立票（用户令通道 2 独立承接）；分诊时先定「要裁的」再转 ready-for-agent
Blocked by: 无；触发面与 [routing-first-cut/12](../../routing-first-cut/issues/12-cc-customs-applicability-judgment-for-route-candidates.md)（候选的关务适用性）、[票 10](./10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) 第 1 项（申报发送连接器）相邻，分诊时划清
地盘：`internal/customscompliance`（入口与触发）、`cmd/parcel-api` 或 `cmd/parcel-dispatch` 的装配。
出处：[票 05](./05-demo-journey-criterion-evidence.md) 格 9——开发主线重定级表 PN-05 第一项记「未见缺口」，动线取证补出这一格。

## 现象

`customscompliance/application` 的 `NewEstablishCaseHandler`、`NewSubmitDeclarationHandler` 在 `cmd/` 零引用；接受决定的扇出只到 VE 与初始路由。生产进程里没有任何端点或消费门会建案或提交，外部结果口因此答「归属不上」（票 05 格 11「08 重走」）。

## 做什么

- **机制半边**：两份编排进生产装配，各有入口。
- **产品策略半边**：触发面的内置策略——谁、在什么业务时点、为哪些包裹建案；提交在什么条件下发起。

## 要裁的

1. 立案：只给运营命令面，还是另有内置触发（例如消费接受决定并按关务适用性判断派生）？若是后者，与 routing-first-cut/12 的判断口怎么分工。
2. 提交：[首发范围](../../../docs/product/PILOT-SCOPE.md)要求授权确认后提交、不启用生产自动申报——提交是否只给命令面，授权确认从哪一本册读。
3. 入口形态：隔离形态下照 operator-channel/08 放行，生产形态等操作者渠道（ADR-0149 前线操作者族）。

## 完成判据

- 生产进程至少有一条路能建案并形成提交版本（隔离形态实测只记 `S`），票 05 格 9 越过；触发面有内置策略或显式的命令面，没有默认的自动申报。
