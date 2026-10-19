# 98 照 `8197fa2f` 那次重走改演示动线与种子：交接载荷、自营段四步、重量行整数、私有集成客户端发行方，后两段运输准备落空

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 子代理立（演示动线重走，[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「重走（钉 `8197fa2f`，2026-10-19）」停点六，与「逐段」里「初始路由与运输准备」那一条），同一子代理接手。用户「你作为业务和系统专家自决……解决所有的问题」授权。
Blocked by: 无
地盘：`docs/design/synthetic-demo-journey-script.md`（只追加补记）；`scripts/demo-seeds/README.md`（组成表、数据故事第 8 条与「集成客户端渠道」一节只追加）；`scripts/demo-seeds/data/transport/` 新增周期安排；`scripts/demo-seeds/seed.sh` 追加一节。不改代码。
出处：psb/05 那一节停点六的建议句，与停点一里「动线文档该补一句」那一条；运输准备的停点在「逐段」的「初始路由与运输准备」。

## 现象（实测，钉 `8197fa2f`，只记 `S`）

**文档照写会撞上（停点一、六）**。这几处不挡路，但动线各条补记只写了自己新增的几格：

- 交接载荷：`已交接`必须带 `releasingEvidence`、`receivingEvidence` 与 `rule`，不得带 `basis`；带了答 `200 SOURCE_NOT_ACCEPTED`，答复不指名原因。动线各条交接补记只列段与节点几格。
- 交接的 `scope`：每次交接该各起一个，`version` 只给更正用。同一 `scope` 换版本号另登，节点作业当它是前一次的更正（机制缺口归 [95](./95-node-operations-reads-a-new-handover-under-the-same-scope-as-a-correction.md)），动线没写这条约定。
- 自营段：票 32 补记写「交接口带 `segment` 立段」再出发，票 62 补记第一步把上海交出与深圳口岸接收合成一次交接；拼起来，出发没有位置。
- 装载分配：重量池的分配量只收整数，`quantityFromMembers` 只许落在件数池；重量行写 `quantityFromMembers` 答 `200 INPUT_NOT_ACCEPTED`，不指名原因。演示包裹 1.5 KG。
- 集成客户端发行方：共享那只答 `invalid_client`。种子 README 只给了 `--force-recreate`，会让别的会话手里的令牌作废；各次实走都另起私有 Dex，README 没写这条路。

**后两段运输准备落空**。那次取证时段起于 20:33Z，即上海当地 04:33：

- `#2`（SZX→SIN）答 `NO_CURRENT_OPPORTUNITY`。种子物化的按日那一班到达 07:00Z，晚于窗口终点 06:00Z。
- `#3`（SIN-HUB→SIN-LM）同样答 `NO_CURRENT_OPPORTUNITY`。

后两段的窗口由前一段到达、节点处理时长与截单、连接缓冲推出，比首段窄。按日一班只接得住极少的提交时刻。

## 三类

- 租户取值：演示租户的班期。照票 18 先例只调演示租户的周期安排，不改匹配规则（ADR-0201 决定六），合成值，证据记 `S`。
- 机制：无。`scope` 与版本那一格的机制缺口归 95，本票只补动线写法。
- 产品策略：无。

## 完成判据

- 动线文档以新补记写「上面各条自此读作」，覆盖交接载荷、每次交接自起 `scope`、自营段四步、重量行整数与三段准备；带实走证据的旧补记不改写。
- 种子 README 写进另起私有集成客户端发行方的走法。
- 演示提交在一天里任何时刻，三段运输准备都至少有一班 `OPPORTUNITY_PREPARED`。
- 一次性库上 `seed.sh` 灌入退 0，提交一单核对三段准备结果。

## 规模

S
