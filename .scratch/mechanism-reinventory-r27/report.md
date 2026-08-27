# 第二十七轮机制半边重盘（r26 建议路径收讫后的复点 + 八切片三判据 + 全绿证据）

Category: chore
Status: resolved

盘于 `f6f0029`（= 收盘时 HEAD，工作树除本报告族外干净）。承
[开发计划](../development-plan-2026-08-20.md) T4/M4 与
[r26 第六节建议路径](../mechanism-reinventory-r26/report.md)：三张可派票全部收讫后作本轮
盘点戳。工具、两口径判据与其对 r25 发布树的复现校验全部沿用 r26
（[tool/main.go](../mechanism-reinventory-r26/tool/main.go) 同一程序，方法论未变，本轮不复述）。

**结论一句话**：机制半边「可做未做」清零——r26 点名的三张可派票全部收讫（SA 控制策略视图
`0fb4040`、VE 租户维读法 `caf1cc5`、VE 材料归集面 `0adb65d`），同期另收管理台接线前沿八子票
（父票 `61c6b81`）与商业闭包结算键三票（`ae41e7e`）；判据 B 缺 **7→6**，剩余六口全部是
有记录的显式留待；全量真库 `-race` 套件 **80 包全 ok、零 FAIL、零竞态**。八切片达到 r26
预告的「达标或有裁定的显式留待」状态，就绪宣布材料齐备——按 T4 第 3 条，**宣布（含认可
各处留待）是用户的决定**，交用户的差量清单见第五节。

## 一、锚点与取数过程（本轮特有的一段账）

本轮 [raw/](./raw/) 里有两组产物，来源不同、数字一致，先把账说清：

- **先行作业（另一会话，2026-08-27 22:24–22:38，未在 messenger 任何通道心跳）**：
  首点 [raw/r27-head-1728134.txt](./raw/r27-head-1728134.txt)（240 口，A 缺全局 7 /
  同上下文 9，B 缺 6，扫描适配器/平台生产文件 280；跑在 Temp 工作树副本，UTF-16 输出）、
  补点 [raw/r27-recheck-b69bdaa.txt](./raw/r27-recheck-b69bdaa.txt)（238 口——CC 两张目录
  读口彼时未落，各缺口与首点相同，扫描 275，与 r26 追补点 `b69bdaa` 衔接）、一份 `-race`
  全绿日志 [raw/race-run-2026-08-27.log](./raw/race-run-2026-08-27.log)
  （22:30:38→22:38:48 EXIT 0，**SHA 头为空**——没锚提交）。
- **本轮终点（接手后重点复核）**：[raw/r27-head-f6f0029.txt](./raw/r27-head-f6f0029.txt)
  于收盘 HEAD `f6f0029` 重跑，表格与首点 1728134 **逐格一致**（两点之间的三个提交
  `ae41e7e`→`515359c`→`f6f0029` 族是收口记账、注释更正与 pgtest，不动端口面）；`-race`
  于同一 SHA 重跑并带锚（第四节）。先行数字经复核采信，本轮引用一律以 f6f0029 两份为准。

## 二、端口两口径复点（事实）

盘于 `f6f0029`，扫描适配器/平台生产文件 280（r26：269）。

| 上下文 | 接口总数 | 判据 A 缺 | 判据 B 缺 |
|---|---:|---:|---:|
| customscompliance | 40 | 0 | 0 |
| networkrouting | 17 | 0 | 0 |
| nodeoperations | 12 | 1 | 0 |
| parcelpricing | 7 | 0 | 0 |
| parcelshipment | 38 | 2 | 2 |
| partycommercial | 18 | 0 | 0 |
| pilotgovernance | 9 | 0 | 0 |
| settlementaccounting | 36 | 3 | 3 |
| transportfulfillment | 24 | 0 | 0 |
| visibilityexception | 39 | 1 | 1 |
| **合计** | **240** | **7** | **6** |

**差式 vs r26（`e5301f8`）**：

- **总数 235→240**，+5 全为新声明读口，逐口有来票：CC `CaseRegisterCatalogueRead`
  （接线前沿 05，`0cfa6b9`）与 `GateConditionCatalogueRead`（06，`cd23768`）、
  VE `CatalogueListRead`（02，`8f1c8e7`）与 `MaterialReceiptRegistry`
  （ve-claims-read-seams/02，`a85fbbf`）、PC `CommercialRelationCatalogueRead`
  （01，`d69bbca`）。五口判据 A/B 都记已实现——即新开口没有欠新账。
- **判据 A 缺 9→7**：关 2 开 0——SA `PreAcceptanceControlPolicyView` 随
  [ADR-0079](../../docs/adr/0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)
  的生产适配器关闭（`be389d3`，SA→PC 消费缝落在 ADR-0054 预留的落点）；
  VE `ClaimEvidenceView` 随材料归集面从 `cmd` 显式未配置桩换成真库读（`a85fbbf`），
  从 r26 点名的「虚低」名单退场。
- **判据 B 缺 7→6**：关 1——同 SA `PreAcceptanceControlPolicyView`（r26 第三节表格里
  唯一的「可做未做」，至此该分类**清零**）。
- **同上下文子树口径 A 缺 9**（交叉核对档）：比全局口径多出的两口都是 A 的口径噪声、
  判据 B 均记已实现——PC `Clock`（实现在五个 `cmd` 组合根的 `systemClock`，r26 已记）与
  PC `CommercialResolutionView`（实现就在本上下文 `adapters/postgres/commercial_resolution.go`，
  只是接口名未在该子树整词出现，无 `var _` 断言的名扫盲区）。虚低机制维持 r26 结论：
  B 口径继续作精确对照。

## 三、判据 B 剩余 6 口——全部是有记录的显式留待

| 端口 | 分类（沿 r26 不翻案） | 留待依据 |
|---|---|---|
| SA `ContractResponsibilityView` | 登记面形状留待实例证据 | 端口注释：回收需要合同依据（实例半边），未配置停在未决 |
| SA `ClaimAmountRuleView` | 登记面形状留待实例证据 | 端口注释：没有规则版本不形成金额（AT-SA-147） |
| SA `SupplierAuditAuthorityView` | 登记面形状留待实例证据 | 端口注释：授权未配置时审核停在未决，不默认放行 |
| PS `SourceDataRuleDeclaration` | 登记面形状留待实例证据 | 端口注释：由 `PAR-COM-13` 与真实合同/产品/线路/关务规则登记 |
| PS `SourceDataAmendmentAuthorizer` | 建模未决 | 端口注释：真实请求方与授权入口是 `BD-PS-009` 待确认的实例参数 |
| VE `NotificationChannelGateway` | 实例半边（真实外部渠道） | 端口注释：真实渠道与凭证属实例参数，唯一实现是测试替身 |

两条本轮新增的旁证，让这张表比 r26 时更硬：

1. **注释与名单对上了**：`515359c` 实测发现 `internal/*/ports` 有十处注释声称「今天没有
   实现，唯一实现是测试替身」，其中九处早已有真库 Outbox 适配器——全部按符号名改指落地
   的实现，只留 VE `NotificationChannelGateway` 一处，它恰好也是 B 名单上唯一「真无生产
   实现」的实例半边口。注释宣称与工具名单现在一一对得上。
2. **四口登记面的口径张力**维持 r26 第三节的记载与处置（预造登记面就是在没有事实的地方
   立形状，收口路径两条：随首个租户登记面成形时实现，或裁断轮翻案预造），本轮不重述。

## 四、全绿证据（受控案例可复算）

- 本轮锚定跑：[raw/race-run-2026-08-27-f6f0029.log](./raw/race-run-2026-08-27-f6f0029.log)
  ——`go test -race -p 1 -count=1 ./...`，WSL go1.26.5 + cgo，DSN 指 `postgres:16.14`
  门禁容器（127.0.0.1:55432），SHA 头 `f6f0029`，2026-08-27 23:42:57→23:51:19，
  **EXIT 0，80 包全 ok、零 FAIL、零 DATA RACE**。比 r26 多出的第 80 包正是本轮新缝
  `internal/settlementaccounting/adapters/partycommercial`（ADR-0079）。
- 本跑压在同 SHA 的 pgtest 改造上（`f6f0029`，PGTEST-CONN：管理面收敛为进程级常驻连接、
  池封顶 2、删库失败出声），即测试基建改造与全绿证据互为验证。
- 测试面 552 份测试文件对 543 份生产文件（internal+cmd，r26：538/529）；`internal/`
  逐上下文：PS 90、VE 78、CC 61、PC 57、NR 44、PP 42、TF 41、SA 41、NO 20、PG 14，
  另 platform 11、architecture 9，`cmd/` 合计 44。

## 五、八切片逐三判据（判断）与交用户的差量清单

三判据的全局底座：**可复算**见第四节；**骨架完整**——生产代码（internal+cmd 非测试）
`TODO|FIXME|not implemented|panic(` 命中 **0 行**（本轮重扫），应用编排生产文件 69→**71**
（CC 12、PS 13、SA 9、TF 8、VE 9、NR 6、PC 5、NO 3、PG 3、PP 3——PS 增的是接受判断链
编排、VE 增的是材料归集编排），真库适配器 160→**165**（CC 29、SA 29、VE 25、TF 21、
PS 16、PC 16、NR 12、NO 7、PP 5、PG 5）；**参数显式未配置**——维持，新读面全走
ADR-0078 隔离读准入（`SYN-` 前缀门禁），无业务阈值常量入生产代码，直接依赖仍为
chi/pgx/bento 三个。

| 切片 | 定级 | 本轮收掉的 | 剩余差量（全部有记录） |
|---|---|---|---|
| PN-01 治理记录 | **达标**（维持） | — | 无 |
| PN-02 商业解析与接受 | **达标—有裁定的显式留待** | SA 控制策略视图（ADR-0079）；商业闭包结算键三票：解析键登记面扩结算三维 `8dfe2e4`、结算政策正文接发布通道 `ae966e6`、接受判断信封驱动（ADR-0081，`46dbb90`/`e3fdcff`/`5fce8ab`）；PC 闭包写读缺陷同笔修复 `5911d3b` | PS 两口（`PAR-COM-13` 登记 / `BD-PS-009` 建模）；作用域账户目录与控制金额两缝（实例半边既有记录） |
| PN-03 网络收寄与节点作业 | **达标—有裁定的显式留待** | — | 外部标识关系子域等排期（[ps-external-mark-relations/01](../ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md) needs-info 有裁：不由消费侧端口倒逼开子域）；NO `ParcelIdentityView` 显式未配置桩在场（B 已实现） |
| PN-04 运输履约与终局 | **达标**（维持） | — | 无 |
| PN-05 关务 | **达标**（维持） | CC 38→40 两张目录读口同绿 | 无 |
| PN-06 可见性与异常 | **达标—有裁定的显式留待** | 资格规则视图带租户维读法 `bfabc0d`；材料归集面 `a85fbbf`（两票 [ve-claims-read-seams](../ve-claims-read-seams/) resolved） | `NotificationChannelGateway` 等真实渠道凭证（实例半边，注释经 `515359c` 复核为全仓唯一真话） |
| PN-07 计价与结算 | **达标—有裁定的显式留待** | — | SA 三口登记面留待实例证据（第三节；分类维持不翻案）；PP 7 口 0 缺 |
| PN-08 治理编排 | **达标**（维持） | — | B-06 Bento 双消费者绑定属跨仓闸门动作，在 T4 之外另行报批（08-25 已记） |

**交用户认可的留待清单**（认可即宣布就绪的前提，逐条有档）：

1. 四口登记面留待实例证据（SA 三目录 + PS `PAR-COM-13`，第三节）；
2. PS 修订授权建模未决（`BD-PS-009`）；
3. VE 真实渠道凭证（实例半边；相关显式停放票
   [syn-wall-door-audit/01](../syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md)
   与 [ve-008/04](../ve-008-late-account-rederive/issues/04-ops-replay-endpoint-blocked-on-par-int-01.md)
   均等 `PAR-INT-01` 真实租户证据）；
4. 外部标识关系子域排期（PN-03，needs-info 有裁）；
5. NR 路由证据视图的机制/实例切分等 `PAR-NET-14`
   （[nr-route-evidence-views/01](../nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md)
   needs-info——注意它不在判据 B 名单上，是端口计数看不见的既知余量，一并交认可）。

**就绪读数**：可复算与显式未配置两条判据全库满足且有本轮锚定证据；骨架完整在「可派机制
工作清零」意义上成立——上表四处「有裁定的显式留待」经用户认可即为八切片全数就绪。
按 T4 第 3 条，宣布本身是用户的决定；基线「机制半边现状」节本轮同步重戳（状态列在用户
认可前维持「部分」不擅改，差量格已按本表收缩）。

## 六、本轮没做的事

- 没实现任何端口、没动任何生产代码（盘点轮纪律；与本轮戳同 SHA 的 `f6f0029` pgtest 件是
  测试基建，先于开盘落库）。
- 没把四口「登记面留待实例证据」翻案成预造，没把 needs-info 四张停放票强转状态
  （M2 显式停放纪律）。
- 没更新棘轮普查 census（T2 量尺，口径不同属，维持 r26 处置留给下一次接线批）。
- 没有还原 r25 遗留的「r24 记 65 对重算 62 差 3」（维持差式可比口径）。
- 基线里两处**与本轮盘点无关的陈年计数**顺手更正并注明轮次（迁移 SQL 份数 62→92、
  跨上下文消费适配器 12 文件/8 缝→43 文件/16 缝、Outbox 手递 46→47）——它们触发了
  基线自设的重盘条件第三项「引用的计数与实际不符」，不改判据与定级。
