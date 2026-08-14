# `.scratch` 六张票通读：没有重复，但有两族

只读通读，**未合并、未修改任何一张票**。产出只有一件：指出哪些票是同一种病的不同实例，
以及那种病怎么在下一次动手前认出来。

盘于 `76c3d13`，六张票全数读过。

## 结论一：没有两张票是同一件事

六张各有各的所有者、各有各的修法，一张都不该合并。**但把它们并排读，会看见两族。**

## 族一：键取自消费方的问题，而不是提供方的发布单位（四张）

| 票 | 键取了什么 | 提供方实际按什么发布 |
|---|---|---|
| [`ve-milestone-mapping-key/01`](./ve-milestone-mapping-key/issues/01-mapping-catalog-keyed-on-fact-reference-is-unfillable.md) | 源事实**引用**（一条具体事实） | 事实**类型码**到里程碑的映射，一行覆盖此后同类型全部事实 |
| [`nr-route-evidence-views/01`](./nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md)（已否掉的快照表那条路） | 判断键里的 `asOf`——**请求期才存在的值** | 带有效区间的目录版本，查询按时点选版 |
| [`ps-external-mark-relations/01`](./ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md) | `mark → parcel` 唯一键 | 外部标识关系：标识类型、分配方、**被标识对象（三类）**、适用范围、替代关系 |
| [`ve-claim-eligibility-dimensions/01`](./ve-claim-eligibility-dimensions/issues/01-eligibility-query-cannot-carry-four-of-seven-dimensions.md) 第一、二节 | 六维查询 | CONTEXT 要七维，其中两维（重复关系、最低材料）**根本不是目录行**，是仓储与证据聚合的数据 |

前两张已经互相引用（NR 那张明写「与 ve-milestone-mapping-key 记的是同一类错误」），
后两张此前没被归进来。

**共同形状**：读取面的键是照着消费方那句问话取的，而不是照着拥有数据那一方的发布单位取的。
后果一律相同——**表在结构上填不满**，不是暂时空着。

**这一族与「实例半边暂时为空」长得一模一样，而两者的处置完全相反。** 分辨只要一问：

> 这张表由谁来填？填一行，能不能覆盖此后很多次提问？

能，就是正常的空目录，等租户登记即可（本仓大多数目录属此，例如 SA 的确认条件目录与分摊
规则适用登记）。只能覆盖一次已经发生的提问——那就是这一族，**再等也不会满**。

`ve-milestone-mapping-key` 那张把这句判据写得最清楚，值得当作族里的样板：「这不是『目录
暂时是空的』……而是这张目录**在结构上填不满**。空目录会随租户登记而变满；这一张不会。」

## 族二：可用的答复格数少于业务要求的格数（两张）

| 票 | 现有代数 | 业务要求的那一格 | 落进错误格的后果 |
|---|---|---|---|
| [`ve-claim-eligibility-dimensions/01`](./ve-claim-eligibility-dimensions/issues/01-eligibility-query-cannot-carry-four-of-seven-dimensions.md) 第三节 | `EligibilityScreen` 封闭二值 | CONTEXT 生命周期要的`等待补充` | 资料不足答`不予受理`，而 `ScreenEligibility` 一次性——**索赔被永久拒掉且再也审不了** |
| [`route-handoff-delivery-granularity/01`](./route-handoff-delivery-granularity/issues/01-per-parcel-independence-cannot-be-expressed-in-one-delivery-one-transaction.md) | 消费门三出口（提交／回滚重投／拒收） | `AT-NR-012` 要的「这个包裹提交、那个包裹留待重试」 | 只能二选一：延迟（整体回滚）或丢失（入账收工） |

**共同形状**：不是判断写错了，是**没有地方写下正确的判断**。两张票各自都已经指出，按当前
形状「两条都想要的性质只能得其一」。

修法也同形：要么给代数加一格（加`等待补充`／加一个逐成员出口），要么改粒度让那一格不再
需要（拆信封到包裹级）。两张票各自都列了这两条路。

**这一族最危险的地方是它编译得过也测得过。** VE 那张记下了一个具体的陷阱：`parcel-shipment`
的 `IntakeEligibilityView` 在证据取不到时如实答「未成立」并点名缺口，那是对的，因为它有
第三格且可续办；**把同一份写法搬到 VE 的二值 `Screen` 上就变成不可逆的默认拒赔**，而两边
看起来同形。

## 剩下那张不属任何一族

[`supplier-expected-cost-correction/01`](./supplier-expected-cost-correction/issues/01-same-currency-correction-contradicts-the-forming-door.md)
是六张里唯一不涉及跨边界契约的：同一个领域包里两扇门（`FormSupplierExpectedCost` 与
`AppendCorrection`）对同币种两额能否不等给出相反答案。它也是唯一由**库内 CHECK** 逼出来的
——领域用例各测各的门，因而全绿。

## 一条横贯五张的观察

六张里有五张（除上面那张外）都是同一个顺序问题的实例：**端口形状先定，提供方那一侧后建模，
于是端口问了一个提供方答不出的问题。**

- 族一四张：键的形状先定，拥有数据的一侧按别的单位发布。
- `route-handoff-delivery-granularity`：信封粒度先定（委托级），消费方按包裹级推进。

PC 读取面那件事（三个通道各自撞上「PC 领域件在位但无端口可装载」）是同一个顺序问题的
第六个实例，只是它撞在提供方**完全没有读取面**这一极端上。

这不构成一条禁令——端口先行往往是对的，消费方确实最清楚自己要问什么。它构成一个检查点：
**定完端口形状，去问一次拥有那份数据的上下文按什么单位发布它。** 六张票里没有一张是在那
一步被发现的，全部是在写适配器、真库门禁打红或逐口清点时才暴露。

## 本文件的性质

只读分析，不是票，没有 `Status`。它不替任何一张票拍板，也不建议合并。六张票的所有权与
`needs-triage` 状态原样不动。
