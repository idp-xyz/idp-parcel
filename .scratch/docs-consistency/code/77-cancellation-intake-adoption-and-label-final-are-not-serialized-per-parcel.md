# 77 PS 取消提交、收寄采用与面单终局形成三处之间不按包裹串行

Category: bug
Status: ready-for-agent——R2（上线后尽快）：两份互斥的终局可能同时落下，且之后没有任何信封会重新比对。read committed 下，取消的提交边界与收寄采用、面单终局形成各在自己的事务里读对方的表；两笔并发时都看不见对方尚未提交的写入，取消与责任起点（或非取消终局）会同时成立。今天取消授权停在未配置，取消形成不了，这一格还到不了；租户一登记取消授权即生效。2026-10-21 协调方据 code/21、35 隔离评审 N4 立，分级 R2。
Blocked by: 无
出处：code/21、code/35 隔离评审 N4（协调方转达）；[ADR-0254](../../../docs/adr/0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md) 决定二「谁先越过提交边界只决定走哪一格」——前提是「先越过」有一个确定的先后。

## 现象

ADR-0254 的三格建立在「谁先越过提交边界」可以判定之上：取消到提交边界时边界事实已在册 → 不形成取消；取消先提交 → 后到的边界事实按业务时间比。两边各在自己的事务里读对方：

- 取消编排（`CancelParcelHandler.Handle`）在提交边界上读当前有效终局、采用链与首次有效收寄，随后写取消记录；
- 收寄采用（`AdoptNetworkIntakeHandler.Handle`）读已成立的取消，随后写采用记录；
- 面单终局判断（`JudgeLabelServiceFinalHandler.Handle`）读已成立的取消，随后经采用路径写终局。

三处之间没有按包裹的锁，也没有把对方的写入挡在同一索引上的唯一约束（取消表与采用表、终局表是三张表）。两笔并发、都在对方提交之前读，各自看到「对方不在册」，就会同时提交取消与责任起点，或取消与非取消终局。此后两边都已越过提交边界，消费门都已入账，不会再有信封重新比对——ADR-0254 的冲突那一格从未被触发，冲突册里也没有这一行。

## 证据

- 代码：`internal/parcelshipment/application/cancel_parcel.go`、`adopt_network_intake.go`、`judge_label_service_final.go` 三处读对方、写自己；`cmd/parcel-api/assemble_cancellation.go` 与 `cmd/parcel-dispatch/assemble.go` 的事务壳都不取包裹级锁。
- 先例：code/70 的作用域级咨询锁（`ports.PreAcceptanceControlScopeLock`）、code/39 的来源身份锁、code/14 的分户账锁。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：一个按（租户，包裹）取 `pg_advisory_xact_lock` 的端口与适配器（锁空间如 `parcel_shipment.parcel.cancellation_boundary`，走 `RequireExecutor`）；三处编排在各自的提交边界读之前取锁，锁与读写在同一笔环境事务里；真库并发用例三对各一遍（屏障停在先手读完之后，后手要么已提交、要么经 `pg_locks` 看到等在锁上）。无迁移。

## 最小的诚实修法

1. 立端口（例如 `ports.ParcelBoundaryLock`）与 PostgreSQL 适配器，照 code/70 的形。
2. 取消编排在读终局之前取锁；收寄采用在读取消之前取锁；面单终局判断在读取消之前取锁（形成终局走的采用路径在同一笔事务里，锁一直持到提交）。
3. 取锁失败按依赖未决，各编排各用自己的原因格。
4. 补并发用例：取消对收寄采用、取消对面单终局，各证后手看得见先手的写入，落到 ADR-0254 的某一格，而不是两份终局同时成立。

## 要裁的

- 网络服务的交付终局（`AdoptOnEffectiveDelivery` 那一路）是否也要进同一把锁：交付以责任起点为前提，责任起点已由采用那一路串行，今天看不出需要；落地时核一遍。
