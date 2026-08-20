# 03 号票路线取证与推荐：账户迟到绑定后的客户视图重派生

Status: resolved（用户 2026-08-20 已拍板，见文末「五、裁断与落票」）

取证基线：`origin/main` = `0f05304`（`fix(ve): claim_item 整行 UPSERT 加修订守卫`）。
取证树：`C:\Users\topsx\AppData\Local\Temp\idp-parcel-inspect-03`，detached 于该 SHA，只读无改动。

本文只回答[03 号票](./03-customer-view-has-no-rederive-after-late-account-binding.md)的「未定问题」：
触发器从哪来。不推翻 `.scratch/ve-parcel-to-party-lookup/report.md` 的三格代数，只补事后空档。

## 一、票面复核：成立，但比票面写的更窄一处、更宽一处

`DeriveCustomerViewOnProjectionAdapter.HandleDerivedTrackingProjection` 在基线上的形状与票面一致：
账户反查 `!accountFound` 交回 `nil`，不派生视图、不发明账户，消费门把该封入账。VE-008 接线
（`veinbox.TrackingProjectionDerivedEventType` → `veCustomerViewRouted`）确已在装配路由表里，
是真接线不是骨架。**票面描述不需要改写。**

两处票面没写但影响选路的事实：

**更窄——重试机会比票面说的还少一格。** 该处理方在读回当前投影后先判版本：
「信封宣告的版本落后于当前版时业务终局跳过」。所以「下一个投影版本信封」并非都能当重试用，
只有**恰好宣告当前版**的那一封会走到账户反查。空档因此不止「末次源事实之后」，还包括
「账户在两封在途信封之间到位」这类窄缝。结论方向不变，空档只更实。

**更宽——零行有两种成因，今天在数据上分不开。** 处理方注释已明写：零行的两种成因是
「包裹引用装着集运单元号」与「尚无已接受委托」，因为 TF 事实不带身份种类。这一条直接决定选路
——任何**盲扫**式补派生都分不开这两者，而事件驱动天然只碰第二种。

## 二、三条路线的证据

### a) 事件驱动补派生

**触发事实已存在，且已在路由表里。** PS 侧已发布的事实只有三类
（`parcel-shipment.acceptance-decision.formed` / `.network-intake.recorded` / `.final-outcome.formed`），
其中 `acceptance-decision.formed` 就是票面说的「委托接受」——`FindCurrentAcceptedByParcel` 的过滤条件
是 `state = ACCEPTED`，而使该行进入 `ACCEPTED` 的正是这份决定。

**ADR-0049「接得住才登记」这一关的实际形状与预想不同：不需要新登记。** 该类型此刻已由
`nrinbox.AcceptanceConsumer` 订阅并在装配路由表内。加 VE 是给同一 `EventType` 挂第二个消费者走
`dispatch.FanOut`，与现有收寄/揽收/交付三类同形。ADR-0049 真正要挡的
「没有订阅者即显式失败卡分区」在这条路上根本不会触发。

**但 ADR-0049 的另半边要付代价：载荷接不住。** 该信封只带来源身份四维 + 委托号 + 提交版本 +
决定标识 + 状态字，**不带包裹清单**——`nrinbox.AcceptedDecision` 的注释写明这是刻意的
（「路由创建要读的基线与包裹清单由编排按引用重新取」，跨上下文只传引用）。所以 a 路线必须
补一个 PS 只读口：按（租户 + 委托号）交回声明包裹清单。

**这个新读口的代价很低，因为列已经在了。** ADR-0060 决策一已在 `shipment_request` 同行落了
`declared_parcel_ids`（text[] 查询投影列），并按决策四建了谓词 `state = 2` 的部分 GIN。反查口
`FindCurrentAcceptedByParcel` 正在用它。补的是同一列的反方向单行读，**不要迁移、不要新表**。

**成员循环的形状已有先例。** ADR-0066 已裁「多对象信封在消费侧按成员拆分」，`CONS-PROJ-DECL-A`
（`7d3d35c`）就是按这个形状落的。a 路线的「一份委托 → N 件包裹 → 各自重派生」与它同形。

**a 路线天然避开零行歧义。** 它由一份真实的已接受委托驱动，手里拿的是真实声明包裹清单，
从头到尾不需要区分「集运单元号」与「尚无委托」。第一节那条「更宽」的事实在这条路上不构成负担。

**常态下它是空转。** 正常序（先接受委托、后包裹流转）时该消费者被触发，按包裹读当前投影读不到，
什么也不做。只有在票面那个倒序里才真正干活。

**待解的接线细节（不阻塞选路，但要记）：** 该信封同时覆盖接受与拒绝两种走向（`state` 字段），
VE 侧只应对接受态动作。另外这条 FanOut 的 NR 腿本来就撞 `ROUTE_EVIDENCE_NOT_CONFIGURED`
实例墙，01 号票的分格修复（`a097d7f`，仅全路未决才记 `consumer_undecided`）意味着：VE 腿成功不改变
NR 腿的未决记账，但 VE 腿硬失败会把整格升成硬失败。这是正确行为，接线票要如实更新既有断言。

**a 路线的真正代价在领域语言，不在代码。** 见第三节。

### b) 运维重放口

**全仓没有运维口先例。** 基线上 `internal/*/adapters/http/` 共 24 个文件，全部是业务受理口
（提交/撤回/受理/登记/查询）或 `unconfigured_intake.go` 那道诚实墙，没有任何 admin/ops 面。
b 路线会是本仓第一个运维面，属新开一类架构面。

**它会立刻撞上与客户查询口同一堵墙。** `visibilityhttp.QueryIntake` 的注释已表过态：认证方式属
`PAR-INT-01` 待提供，「未决期间本包不带任何实现，包括『开发用』的采信头部版本」。一个能触发
重派生的运维口权限只会更敏感，同样取不到授权依据；要么一起阻在 `PAR-INT-01`，要么发明默认——
后者撞 AGENTS.md 红线。

**它满足不了「必须」。** UC-VE-008 与 VE CONTEXT 两处都写的是**必须**重新派生（见第三节）。
手工触发意味着不变量只在有人记得时成立。

**但它作为兜底仍有价值**，与选哪条主路无关。

### c) 接受为已知边界

这条与两份权威文档的现行措辞直接冲突：

- UC-VE-008「规则、权限与隔离」：「来源迟到、更正、撤销、替代、POD 失效、身份谱系或终局变化
  **必须**触发当前视图重新派生。」
- VE `CONTEXT.md`「客户全程追踪视图」规则段：「来源更正、事实有效性变化、身份谱系变化、
  ETA 新版本或终局更正**必须**重新派生客户视图并形成追加更正或明确替代关系。」

把空档写进「已知边界」不是接受一条边界，而是削弱一条已写死的**必须**。

**对客影响是具体的，且不可分辨。** `NewQueryCustomerTrackingViewEndpoint` 的业务结果只有两格，
`VIEW_NOT_FOUND` 按 ADR-0029 探针纪律同时承担「包裹不存在」「不属于本账户」「视图尚未形成」。
所以账户迟到的包裹，其授权客户拿到的答复与「这不是你的包裹」逐字节相同。

**c 唯一站得住的形态**见第三节的 Q1：若裁定账户绑定不属于上述触发清单里的任何一项，
c 就不再是「削弱必须」，而是「这条必须本来就没管到这里」。那是一次领域语言裁断，不是工程取舍。

### 顺带关掉一条：读时派生不是选项

UC-VE-008 的「触发」栏里列了「授权客户查询」，看上去像第四条路（查询时现派生）。这条是封的：
`visibilityhttp.TrackingViewReader` 的注释直接引了 VE CONTEXT 的
「客户读取或门户展示 → 形成查询/展示结果，不自动形成异常披露决定、主动通知、送达或客户确认」，
并据此「接存储读面，不接派生编排」。不再展开。

### 旁证：未合入的平行实现 `dca024a` 独立印证了同一个空档

`mcp3-ve008-wire` 分支的 `dca024a`（VE-008-WIRE-A）与已入 main 的 `49a2ab0` 同根、互不为祖先，
是同一道缝的另一种做法，**已搂浅、不是现行实现**，此处只读作旁证。

**它的零行格与 main 逐字同义：`!found` 交回 `nil`。** 所以空档不是赢家那版的疏漏，而是这道缝本身的
形状——两位作者独立落到同一格。

**更有分量的是它的注释把本票的假设写死了：**
`DeriveOnTrackingProjectionAdapter.HandleDerivedTrackingProjection` 的零行段写
「若委托日后才落到已接受，下一版投影的信封会再触发本链，不靠重投这一封等」。这正是 03 号票指出
有洞的那条假设——**作者当时就想到了迟到接受，并明确把它托付给「下一版投影的信封」**。票面说的
「没有下一封时怎么办」因此不是事后诸葛，而是一条被自觉搁置的短路，两版实现都搁在同一处。

**两版在一处分歧上相反，且都引了权威**，与本票选路相关，故记下：客户视图该基于哪一版投影派生。
`dca024a` 用 `FindByVersion` 读信封指名的那版，理由是「同一包裹连发两版投影时，各版各长自己的
视图代（ADR-0065 / AT-VE-161），读 `FindCurrent` 会让先到的信封吃掉后到那版的更正」；main 用
`FindCurrent` 加版本相等判断跳过旧版，理由是「客户视图只基于当前投影形成（UC-VE-008）」。

**这处分歧对本票是顺风。** a 路线的补派生由账户绑定触发，手里没有「信封宣告的版本」，只能基于
当前投影派生——这在 main 的 `FindCurrent` 形状里是自然延伸，在 `FindByVersion` 形状里则无处安放。
赢家那版恰好让 a 更好落。分歧本身是否要重开不属本票，仅记录。

## 三、推荐

**推荐 a，附 b 作后续兜底票；c 不推荐。**

选 a 的三条理由按分量排序：

1. **只有 a 能满足两份文档写的「必须」**，且它是唯一不需要人记得的形态。
2. **代价比预想低。** 不新登记事件类型（已在路由表）、不迁移（`declared_parcel_ids` 已在）、
   成员循环有 ADR-0066 先例。真正的新东西只有一个 PS 只读口和一个 VE 消费者。
3. **它天然绕开零行两成因的歧义**，而 b 的盲扫绕不开。

**但 a 有一处必须先补的领域语言缺口，这是我请你注意的重点：**
两份文档的重派生触发清单枚举的都是**源事实**的变化（更正、有效性、身份谱系、ETA、终局）。
「账户关系迟到建立」不是源事实变化，而是**归属维**从取不回变成取得回。现行清单里没有一项名正言顺地
覆盖它——最接近的「身份谱系变化」在本仓语境里指的是拆分/合并
（VE CONTEXT：「真实拆分或合并后，各当前有效包裹分别形成后续投影」），不是账户绑定。

所以 a 不是一张纯接线票：它要么先给 VE `CONTEXT.md` 的客户视图触发清单补一项命名，
要么就得论证账户绑定已被「身份谱系变化」涵盖。按 AGENTS.md「改领域语言 → 先改 `CONTEXT.md`
再改引用它的 `UC-*`」，这一步在编码之前。

## 四、待拍板（frontier）

❓ **Q1 — 账户绑定算不算现行重派生触发清单里的一项？**
这一问同时决定 a 的工作量与 c 是否成立。三种裁法：把它并入「身份谱系变化」（不改文档，但语义被撑宽）；
在 VE `CONTEXT.md` 与 UC-VE-008 各补一项新触发命名（诚实，但要动两份权威文档）；
裁定它不在清单内（则 c 成立，且不算削弱必须）。

➡️ 推荐第二种：补一项新命名。「身份谱系」在本仓已经明确指拆分/合并，撑宽它会让两个概念挤在一个词里，
正是 `/domain-modeling` 要防的那种重载。

❓ **Q2 — 若走 a，触发事实取 `acceptance-decision.formed` 是否就够？**
它覆盖「委托被接受」这一刻。但账户维取得回还有第二种成因：委托早已接受、而包裹是后来经新提交版本
才成为其成员的。那条链走的是 ADR-0066 的提交版本形成，不是接受决定。

➡️ 推荐首发只接 `acceptance-decision.formed`，把「新提交版本改变成员」列为已知未覆盖并写进票面。
理由是后者需要 PS 侧给出成员差集语义，牵动面大出一截，不该塞进这张票。

❓ **Q3 — b 是否作为独立后续票保留？**
即便 a 落地，仍会有 a 覆盖不到的成因（如 Q2 的第二种、或历史存量）。

➡️ 推荐保留为独立票但**不与 a 同期**，且明确阻塞于 `PAR-INT-01`：在授权依据到位之前，
本仓开不出诚实的运维面。

## 取证清单（复核用）

| 结论 | 出处 |
|---|---|
| 零行交回 nil、空档成立 | `DeriveCustomerViewOnProjectionAdapter.HandleDerivedTrackingProjection` |
| 只为当前版信封派生 | 同上，版本判新旧那一段 |
| 零行两种成因分不开 | 同上的三格注释 |
| VE-008 已真接线 | `assembleDispatcher` 路由表 `veinbox.TrackingProjectionDerivedEventType` |
| PS 已发布事实仅三类 | 全仓 `eventing.EventType` 常量清点 |
| 接受决定载荷不带包裹清单 | `nrinbox.AcceptedDecision` 与 `decodeAcceptedDecision` |
| `declared_parcel_ids` 已在同行 | ADR-0060 决策一、四；`ShipmentRequests.FindCurrentAcceptedByParcel` |
| 成员循环有先例 | ADR-0066；`7d3d35c` CONS-PROJ-DECL-A |
| 无订阅者不适用于本路 | ADR-0049 决策三 |
| 「必须重新派生」 | UC-VE-008 规则段；VE `CONTEXT.md` 客户视图规则段 |
| 读时不派生 | `visibilityhttp.TrackingViewReader` 注释所引 CONTEXT 原文 |
| `VIEW_NOT_FOUND` 三义合一 | `NewQueryCustomerTrackingViewEndpoint` 的 outcome 两格 |
| 全仓无运维口 | `internal/*/adapters/http/` 24 个文件清点 |
| 运维口撞 `PAR-INT-01` | `visibilityhttp.QueryIntake` 注释 |

## 五、裁断与落票（2026-08-20）

用户拍板，四问全定：

| 问 | 裁断 |
|---|---|
| 主路线 | **a**（事件驱动补派生） |
| Q1 | **补新触发命名**，不并入「身份谱系变化」 |
| Q2 | **首发只接 `acceptance-decision.formed`**，「新提交版本改变成员」列为已知未覆盖 |
| Q3 | **b 保留为独立后续票**，不与 a 同期，阻塞于 `PAR-INT-01` |

已落票 `.scratch/ve-008-late-account-rederive/issues/`：01 命名（前置）、02 PS 只读口、
03 VE 消费 + FanOut（draft，阻塞于 01+02）、04 运维口（阻塞于 `PAR-INT-01`）。

### 协调岗复核补记

本文两条承重结论由 MCP-1 独立复验于 `0f05304`，成立：`nrinbox.AcceptedDecisionEventType`
确在装配路由表内且当前只挂单一消费者 `routed`；`AcceptedDecision` 结构体确无包裹清单字段。

~~**一处可省的代价**：该结构体带 `CustomerAccountID`，账户维随触发信封一起到手，
所以 a 路线的 VE 消费者不需要再走 PS 账户反查。~~

**上条已撤回**（MCP-4 当场提异议，复验成立）。省掉反查不是省代价，是绕开一个安全格：
反查口还带 ADR-0060 第三格，两份当前已接受委托同时声明同一包裹时交 `ErrAmbiguousParcelTarget`
（`current_accepted_parcel_target.go` 确有此返回，`ports.go` 明写「不按时间或行序任选」，
VE 侧译名 `ErrAmbiguousCustomerAccount`）。信封的账户只回答「这份委托属于谁」，
而客户视图要问「这件包裹此刻唯一归谁」——绕开会造成跨账户泄露。

**定案**：反查口保留为账户维权威，信封的 `CustomerAccountID` 降为一致性校验值。
已改写进 03 号票，并附一条实现前要取证的余项（撤回/被取代导致的合法不一致该跳过还是硬失败）。
| 平行实现同格、同假设 | `dca024a`（未合入）`DeriveOnTrackingProjectionAdapter.HandleDerivedTrackingProjection` 零行段 |
