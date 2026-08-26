# 治理查阅面：三张登记表没有租户维，读面形状先要裁一道键形

Category: enhancement
Status: draft

自[接线前沿盘点](../report.md)批 B。盘点初稿把它与票 02 归为同一批（都有 CLI、都零行、都缺查阅面），实读库结构后拆开：**键形不同，形状装不上**。键形未裁前不派。

## 事实（实读代码与演示库取证，锚 `7ce41e4`）

1. `parcel-governance-register` 的三个命令（`authority-interval`、`suspend`、`resume`）已在，第四第五类（阶段评审、接管）CLI 自己写着属第二批、未开。
2. 三张表演示库里全零行——`seed.sh` 没调用这个 CLI。
3. **三张表都没有租户列**（实测 `\d`：`authority_interval` 是 `object_scope`/`capability`/`fact_kind`/`authority` 四件加区间；`suspension_decision` 是 `suspension_id` 主键加 `scope`/`executed_by`）。
4. `pilotgovernance` **整个 `adapters/http` 包不存在**；`adapters/postgres` 五个文件全是写口，唯一的列读 `AuthorityIntervals.ListCurrent(ctx)` 不收租户参数——它是为受控登记口而设的单租户装配形状。

## 要答的那一问

`ADR-0077` 把租户放在读口方法签名上；`ADR-0078` 的隔离读准入注入的也是一个**租户**范围（`SYN-` 前缀门禁）。治理登记册没有这一维。

试点治理治的是**试点本身**、不是租户的数据，无租户维多半是对的设计——但那样一来两件事都要重答：**隔离读准入按什么放行**（今天的机制只认租户），**页面按什么隔离**。

不答就动手会得到一个「按租户过滤」的读口去查一张没有租户的表。那是本仓反复记的那个形状：**接错看着像接对**——填一个空租户或忽略该参数，答出来的恰好是当前唯一走得到的那一格正确答案，而真有第二个租户时它就错，且不会有任何东西变红。

同族先例可参：`.scratch/ve-claims-read-seams/issues/01` 记的是反过来的一种——真适配器把租户钉在装配期，多租户入口接不上，那票的裁决是**另立一个带租户维的读法、两个形状各答各的调用面、不合并**。本票要判的是治理这一格该不该有那一维，不能照抄结论。

## 裁完之后要做什么（范围预估，开工时对新 tip 重核）

- 治理种子（`seeds/governance/` 新建，`seed.sh` 加行属占号文件）；
- `ports` 伴生列表读端口 + `postgres` 读适配器（键形按裁决）；
- **整个 `adapters/http` 包**：未配置 Intake 与隔离读 Intake 一对、查询处理器；
- `cmd/parcel-api` 装配（占号）；`apps/admin-web` 的 `StageAdmissionPage` 接真。

## 一格要在票面先说清

`stage-admission` 页面概念上有五格，今天只有三格灌得进去——阶段评审与接管属 CLI 自记的第二批。这一页接出来会是**三格有内容、两格如实说明未开**，符合「不填假值」，但要在页面上如实交代，别让人以为漏了。
