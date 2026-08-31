# ADR-0083: 试点治理读面按登记册实有维度成形——无租户维是设计；隔离读放行沿用同一开关，注入不带租户的产品级作用域；呈现面留在管理台并明示实例级作用域

Status: Accepted（2026-08-31，批 admin-skeleton-closure 票 01 指令产出本裁决；用户同日指示「全部解决」骨架页机制半边，授权来源句记于该批 spec）
Date: 2026-08-31

## Context

管理台 `stage-admission` 页要开读面，而 `pilot_governance` 是全仓唯一零 `tenant_id` 的上下文（取证 `65b6cf2`：`migrations/pilot_governance/` 八张表 0 处，其余十个上下文全部有）。照现成形状抄就是照 [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md) 抄——它把租户放在读口方法签名上；[ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 的隔离读准入注入的也是一个租户范围。抄的后果是一个按租户过滤的读口去查一张没有租户的表：编得过、测得过、页面也出得来，只是答的不对——两种状态可观察签名相同，没有任何东西会红。

这一问已有一次在先裁决：[admin-web-page-wiring-frontier 票 03](../../.scratch/admin-web-page-wiring-frontier/issues/03-governance-read-face-needs-a-tenant-key-ruling.md)（2026-08-26，经用户授权）裁定**不开第二种放行形态、页面维持骨架**，并立了三条重开判据（有行可看；有人要看且认了受众；不止一张同形读面）。本记录是对那道裁决的正式重开，不是绕开——逐条对表：

1. **有行可看**：同批票 02 给 `seed.sh` 加治理登记步（`parcel-governance-register` 三子命令灌 `SYN-` 种子），本裁决落地之日治理册非零行。
2. **有人要看，且认了受众**：受众在 08-26 之后已被另一道落地裁决换了前提。[syn-wall-door-audit 票 12](../../.scratch/syn-wall-door-audit/issues/12-governance-registration-has-no-process-entry.md)（裁于 2026-08-21，实现已入库）裁定**治理登记的主体是租户运营方自己，信任边界是运维边界而不是商业渠道**，这句就写在 `cmd/parcel-governance-register` 的包注释里。08-26 裁决的决定性理由「数据主是产品团队，不是租户操作员」与这道更早裁定、且已被实现钉住的归属相抵——登记册的写入方就是运营方的治理操作员，读回自己登的册子正是他的事。受众与承载面在 Decision 四作答。
3. **不止一张**：按原判据字面今天仍不成立——零租户上下文仍只有这一个。但该判据要防的是「用一条永久的通用先例换一张空册」；本记录不立通用先例（Decision 三钉死只覆盖本上下文），册也不再空（第 1 条）。「等一次裁清楚」就是这一次，对象是唯一长这个形状的上下文。

另一件在 Context 里说清：用户 2026-08-31 指示把 14 张骨架页的机制半边全部铺完（批 spec 的范围裁定），理由是「尚未接线」与「已接线但登记册为空」在工作台上长同一张脸而两者要人做的事相反。治理页不接，这道诚实缺口在治理分区就永远合不上。

## Decision

**一、治理登记册没有租户维是设计，不是漏了；「加 `tenant_id`」这条路否决。** 建表语句抬头已写明「治理是产品级机制，没有租户维——这里登记的是『本产品此刻拿什么去评审、谁在写生产』，全部是脱敏引用」（`migrations/pilot_governance/0001_governance_records.sql`、`0003_suspension_resumption_takeover.sql` 同句）。治理对象是（对象范围×能力×事实类型×权威方）四维的产品机制引用与试点范围版本，治的是**试点本身**：一条暂停决定暂停的是本产品实例某范围的新准入，不是某个租户名下的一份数据——给它挂租户维等于断言存在一种按租户分片的暂停，领域里没有这种东西。[ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) 的租户边界管的是业务数据隔离；治理册只存脱敏引用、敏感实例映射按红线外置，无租户维不触碰那条边界。为让读面「套上通例」而补列，是拿形状迁就反写数据所有权。

**二、治理列读口的签名按登记册实有维度成形：不收租户参数。** ADR-0077 Decision 五「租户维在方法签名上」立在租户级登记册上，前提是那一维存在；对没有这一维的册子，收下租户参数只有两种下场——被静默忽略（读口在签名上撒谎，真有第二租户那天答案错而无声）或拿去过滤一个不存在的列。治理列读口收 `(ctx, limit)`。查阅读端口**另立**，不并入也不改造写侧的 `AuthorityIntervalStore.ListCurrent`——那是冲突预检口，「重叠必须在准入前被抓住」是它的语义（08-26 裁决的纠读一并采纳：施工量按三件缺三件计）。空册如实答空列表走 2xx 成格，ADR-0077 Decision 四原文适用不另裁。

**三、隔离读放行沿用同一开关、同一装配点；治理查阅行注入的作用域只带登记册实有的维度（作用域引用），不带租户。这一格只覆盖 `pilot_governance`，不成通用档。** ADR-0078 Decision 四「按环境选择的只有装配点上查阅行的 Intake 一件事……只此一处、只此一维」维持原文：不添第二个环境变量、不添第二个开关，`IDP_PARCEL_ISOLATED_READ_TENANT` 未设时治理查阅行与其余各行同样未配置即拒，`SYN-` 前缀启动门禁一字不动。变化只有一处：开关生效时，治理行的注入式 Intake 以（作用域引用，页大小）构造，**没有租户可注**——开关值里的合成租户不进治理作用域。08-26 裁决点名的「判据空洞满足」陷阱由本条显式作答消解：治理行入格不是因为「没有租户所以不存在跨租户泄漏」，而是本记录明裁「作用域携带读面实有的隔离维度」——`pilot_governance` 的最高（也是唯一）隔离边界就是产品实例本身，其内容按建表纪律与登记 CLI 纪律只可能是脱敏引用，种子按 `SYN-` 纪律只记 S。前缀门禁在租户级读面上守住的性质（真实租户标识结构上进不来）在治理面没有对应物可守，不是被绕开。**钉窄**：本条只放行 `pilot_governance` 的列读行；日后任何新的无租户读面不得援引本条直接入格，须自证 08-26 那三条（有行、有受众、值一次裁决）并另出记录，本记录至多作形状参照。

**四、呈现面：`stage-admission` 留在管理台「试点治理」分区，页面明示内容是产品实例级、不按租户隔离。** 受众是运营方的治理操作员——与运行 `parcel-governance-register` 的是同一方（票 12 裁定），真实角色名属 `PAR-GOV-03..07` 实例半边照旧待提供。管理台就是本产品交给租户运营方的界面（[ADR-0020](./0020-tenant-admin-client-in-product-workers-are-processes.md)），这一页长在这里没有错位；此前错的不是位置，是页面没把作用域说出口。页面必须写明「本页登记的是产品实例级治理事实，不按租户隔离」；阶段评审与接管两格如实说明属第二批未开（票 12 首批三类裁定），不留白也不造数。备选各自否决：**另立产品运营台**——复活 ADR-0019 明文排除出产品的跨租户运维后台，且它要服务的受众与已持有本管理台的是同一方；**按试点范围（scope）隔离**——scope 是查询条件不是授权边界（ADR-0076/0077 同句）；**CLI 只读列举子命令**（08-26 预钉的最省路径）——只服务数据库网络内的操作员，合不上工作台两态分辨的批级目标。

## Consequences

- 票 02 解堵：`internal/pilotgovernance/ports` 另立列读端口（权威区间、暂停决定、恢复决定三册），`adapters/postgres` 读适配器 + 真库测试，`adapters/http` 从零建包（未配置 Intake 与隔离读 Intake 一对、查询处理器），种子经登记 CLI 灌 `SYN-` 数据；页接真与 `liveIds` 属阶段二。
- `cmd/parcel-api` 的 `buildIsolatedReadIntakes` 增治理一格，以（作用域引用，页大小）构造、不带租户；装配行归批 07 串行落地。
- `StageAdmissionPage` 现有「归属尚未裁定」的未配置文案由本记录取代——归属已裁定，页面改为如实呈现并标注实例级作用域。
- [admin-web-page-wiring-frontier 票 03](../../.scratch/admin-web-page-wiring-frontier/issues/03-governance-read-face-needs-a-tenant-key-ruling.md) 的「不接线」裁决自本记录起被取代；其推理中仍然成立的部分（写侧预检口不当读面复用、空洞满足陷阱、按租户过滤无租户表的警告）已逐条收进本记录。
- 治理读面照旧不触碰命令面：登记走受控 CLI（票 12），HTTP 命令入口维持不存在；本记录只裁查阅。

## Alternatives considered

- **给八张表补 `tenant_id`。** 否决，见 Decision 一：形状迁就反写数据所有权，且断言领域里不存在的「按租户分片的治理」。
- **读口照抄租户签名、装配时注入合成租户但读侧忽略它。** 否决：签名撒谎——单租户期间答案恰好对，真有第二租户那天静默变错，正是「接错看着像接对」。
- **为无租户读面另开第二个环境开关或第二种放行形态。** 否决：08-26 裁决理由一原样成立——那会把「真实标识结构上进不来」降级成「生产记得别设」；本记录以「同一开关、作用域按实有维度」达成同一目的而不付这个代价。
- **另立产品运营台 / 只加 CLI 列举子命令 / 维持不接线。** 均否决，见 Decision 四与 Context 的重开对表。

## Links

- [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)：目录读通例——Decision 五的签名规则本记录为无租户册裁出适用界；Decision 四空册语义原文适用
- [ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)：隔离读准入——Decision 四先例钉窄维持原文，本记录 Decision 三在其内为治理行裁作用域形状
- [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)：租户隔离边界的原始出处，Decision 一划出治理册与它的关系
- [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md)、[ADR-0020](./0020-tenant-admin-client-in-product-workers-are-processes.md)：管理台的归属与跨租户运维后台的排除，Decision 四的承载面依据
- [syn-wall-door-audit 票 12](../../.scratch/syn-wall-door-audit/issues/12-governance-registration-has-no-process-entry.md)：治理登记主体与首批三类的裁决，受众答案的出处
- [admin-web-page-wiring-frontier 票 03](../../.scratch/admin-web-page-wiring-frontier/issues/03-governance-read-face-needs-a-tenant-key-ruling.md)：被本记录取代的在先裁决及其三条重开判据
- [admin-skeleton-closure-batch spec](../../.scratch/admin-skeleton-closure-batch/spec.md)：本批范围裁定与用户授权来源句
