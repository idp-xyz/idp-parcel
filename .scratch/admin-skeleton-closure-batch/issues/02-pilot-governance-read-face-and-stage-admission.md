# 试点治理读面与 `stage-admission` 接真——本批唯一能真出数据的一页

Category: feature
Status: resolved——MCP-3（阶段二已交票闭，合入 `6989cd1`、注记校正 `e3b56f8`；状态行由 MCP-1 批务代结，见票 07）
Blocked by: 01

## 为什么这一页与其余十三张性质不同

其余各页表空是因为接入渠道墙拦着上游，属实例半边。**这一页不是。** 取证于 `65b6cf2`：写入方
`cmd/parcel-governance-register` 已在（三个子命令 `authority-interval`、`suspend`、`resume`），
八张表也已在，`seed.sh` 调了六个登记 CLI——**唯独没调它**。所以八表零行纯粹是种子没灌。

灌上就有真数据。**这是本批唯一能让工作台「已接线」真正 +1 且页面非空册的一票。**

## 做什么

前提：票 01 的 ADR 已落。**按那份 ADR 裁定的准入形状做，不要照抄 ADR-0077 的租户签名**——
理由与后果见票 01，此处不复述。

1. **读端口**：`internal/pilotgovernance/ports` 加列读端口。现有唯一的列读
   `AuthorityIntervals.ListCurrent` 不收租户参数，按票 01 裁定决定它是留用、改造还是并入新口。
2. **真库读适配器**：`internal/pilotgovernance/adapters/postgres` 加读适配器，覆盖权威区间、
   暂停决定、恢复决定三类。真库测试照本仓惯例（插行再读回，`pgtest` 门禁）。
3. **`adapters/http` 从零建包**：查询处理器 + 隔离读准入。形状样板取
   `internal/collectionremittance/adapters/http`（最新一份从零到接线的完整样板），但准入那一格
   按票 01 裁定，不照抄租户那一格。
4. **种子**：`scripts/demo-seeds/seed.sh` 加一步调 `parcel-governance-register` 三子命令，灌
   `SYN-` 前缀数据。加步前在频道说一声（`seed.sh` 是共享文件）。
5. **页接真**（阶段二，见下）：`apps/admin-web/src/pages/governance/StageAdmissionPage.tsx`
   消费新端点。

## 两阶段与次序

- **阶段一**：上述 1–4。做完自验绿，向频道交出**已验 SHA** 与要装配的端点行，**不自改**
  `cmd/parcel-api` 装配四件（占号在票 07，理由见 spec）。
- **阶段二**：收到 MCP-1「已装配」广播后，做第 5 项，并在 `page-registry.tsx` 的 `liveIds` 加
  `'stage-admission'` 一行——**只加自己那行，不动邻行**。次序反了会得到一张登了 live 却 404
  的页。

## 页面形状的一处硬约束

该页五格里，**阶段评审与接管两格今天灌不进去**——`cmd/parcel-governance-register/main.go` 的包
注释自证「首批只开三类（票 12 裁决）……阶段评审与接管第二批，不在本入口」。所以这一页接出来
应是**三格有内容、两格如实说明属第二批未开**。不要为了让五格齐整而造数据，也不要把那两格留
白——留白读起来像数据缺件，而这里要说出的是「机制按票裁定分批，第二批未开」。写法可参照
`CodLedgerPage` 回汇批次格「未配置（回汇周期与汇付通道属实例半边，尚无批次）」那一处的取舍。

## 完成判据

`stage-admission` 转 live 且**页面非空册**（这一页可以、也应当有数据，与本批其余各票判据不同）；
三格出 `SYN-` 值，两格显式说明第二批未开；含真库全仓绿（注明）；工作台「试点治理」分区
显示已接线 1/1。

## Comments

- **2026-08-31 MCP-3（阶段一已交，待批 07 装配后做阶段二）**：已验 SHA `5ba4ede`（前置票 01 的
  ADR-0083 在 `0c03b66`；基 `f3f7c55`，worktree 分支 mcp3-skeleton-closure）。键形照 ADR-0083：
  `ports.GovernanceRegistryRead` 三口 `(ctx, limit)` 不收租户，写侧 `ListCurrent` 原样不动；
  `GovernanceRegisters`（postgres）语句无租户条件、盘点 jsonb 不透出；`governancehttp` 从零建包
  ——GET `/governance-registers`（`register=authority-interval|suspension|resumption` 一口三册）、
  未配置与隔离读 Intake 一对，后者以（作用域引用，页大小）构造无租户可注。种子：seed.sh 加
  构建行+治理步（占号广播已发），`data/governance/` 六笔——**已实跑演示库**：三区间 REGISTERED、
  两暂停 SUSPENSION_RECORDED、一恢复 RESUMPTION_RECORDED，幂等重放 exit 0，八表非零行已成立。
  含真库全仓绿。阶段二待办：页接真（三格出 SYN- 值、阶段评审与接管两格明示第二批未开、页面
  明示产品实例级作用域）+ `liveIds` 加行。
- **2026-08-31 MCP-3（阶段二已交，票闭）**：基 `ffbc2d8` 交付，MCP-1 序移落地于 `1f5d7ae`
  之上、合入笔 `6989cd1`（本条 SHA 经校正笔改为主线实号；apps/admin-web 与交付链逐字节同）。
  `pages/governance/api.ts` 新建（`63da3f3`，GET `/governance-registers` 三册镜像，
  无租户维注释随行）；`StageAdmissionPage` 重写（`980c510`）——DetailPageTemplate 三真格
  （权威区间/暂停/恢复，Table 照登转写，空册句「读取入口已配置，尚无登记」）+ 阶段评审与
  接管两格明示「第二批未开（syn-wall-door-audit 票 12 首批裁定），机制分批不是数据缺件」；
  页头与基本信息区明示产品实例级作用域、无租户维（ADR-0083 Decision 四），旧「归属未定」
  文案随裁决取代；页面只读，无登记动作。`liveIds` 加 `'stage-admission'`（`528b1fd`，与票 03
  两行同笔，只加行不动邻行）。`tsc --noEmit` 绿；三册未配置态合成一格 403 陈述、错误态可重试。
