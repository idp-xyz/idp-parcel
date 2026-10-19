# 22 初始路由信封 ID 越过上限：真实标识下交接入不了队，初始路由信封一封也发不出去

Category: bug
Status: 完工，待评审与进 main——2026-10-18 Claude Code 会话立并实施（用户「你作为业务和系统专家自决……解决所有的问题」授权），分支 `claude-rfc22`。出自演示动线重走（[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)）。
Blocked by: 无
地盘：`internal/networkrouting/adapters/postgres/initial_route_handoff.go` 与其用例；`cmd/parcel-dispatch/initial_route_projection_test.go` 里照抄信封 ID 的那个助手。
出处：`outboxintent.FingerprintEventID` 头注（票 sa-cc/34 裁决 2）；ADR-0043；先例 `customsCaseEventID` 与 `customsCasePartitionKey`。

## 现状（实测，证据 `S`）

- 演示委托初始路由 `ROUTE_FORMED`，`network_routing.initial_route` 有行，`bento.outbox` 里却没有 `network-routing.initial-route.formed`。dispatch 日志一行没报。
- 一次性探针在演示库上对那一份判断重放交接，答 `hand off initial route: outbox intent: enqueue: eventing: invalid envelope: id exceeds 128 bytes`。
- 成因：`initialRouteEventID` 把判断键原样串接。演示的真实标识里，委托、包裹、提交版本各是前缀加二十六位编码，拼出来一百三十多字节。用例里的短合成值没越过上限。
- 交接失败不翻结果，只留一条续办引用（ADR-0043 的纪律）。可这一格的失败是确定性的，重发同一份照样被拒，可视异常那条按初始路由派生投影的消费链就永远等不到信封。

## 裁定（代决）

照 `FingerprintEventID` 头注已定的规则：信封 ID 取口名前缀加判断键六维的指纹，长度与任何一维多长无关。原先漏掉的客户维一并进指纹。分区键保留原先那一串可读串接、一字不变：分区键是顺序语义（先例 `customsCasePartitionKey`）。

其余仍原样串接实例引用的口另立 [envelope-id-bounds/01](../../envelope-id-bounds/issues/01-remaining-envelope-ids-still-concatenate-instance-references.md)。

## 完成记录（2026-10-18，Claude Code 会话，分支 `claude-rfc22` 基 `45b9a088`）

- `initialRouteEventID` 改走 `outboxintent.FingerprintEventID("initial-route", …六维)`；新增 `initialRoutePartitionKey`，取原串接形。
- 用例：
  - `TestAnInitialRouteWithFullLengthIdentifiersStillEnqueues` 用演示真实长度的标识，先断言原串接形确实越过上限，再证指纹形入得了队、分区键仍是可读串接。旧实现在这一条上入队被拒。
  - 既有用例与 dispatch 的 `synInitialRouteEventID` 改成同一公式。
