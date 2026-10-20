# 15 送站预报与送站交接单的两条列表查询没有索引

Category: enhancement
Status: in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 立并代决、认领，分支 `claude-psxm15`，NO 迁移号 0021
Blocked by: 无
父票：[13](./13-admin-web-reads-pre-advices-handover-notes-and-scope-state.md)
出处：票 13 完成记录「没做」与评审。
地盘：
- migrations/node_operations/（0021）
- internal/nodeoperations/adapters/postgres/ 两条列表查询所在文件的测试（只在要钉住查询计划时加）
- docs/product/MECHANISM-INVENTORY.md（重生成）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；2026-10-19 用户答「yes」。

## 现象（读码）

票 13 的预报列表（按节点与预计送达区间、新者在前）与交接单列表（按节点与开立区间）在本租户全表上筛与排序；票 08 的迁移 0013 只为写口与按标识点读建了键，没有为这两条读建索引。演示量下无感，真实租户的预报与交接单逐日累积后，复核页每开一次就是一次全表扫描。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决；机制）

照两条查询的实际谓词与排序各建一条复合索引：（租户，节点，排序时刻）——预报取预计送达时刻、交接单取开立时刻，排序方向与查询一致；节点筛选缺席时的全租户列也能用同一条索引的前缀加排序。不改查询的答复形状与语义。迁移照本仓执行器的约束放在事务里（不用 `CONCURRENTLY`，表今天还小；`runner.go` 头注写明了这一条显式未决）。

## 完成判据

- [x] 迁移 0021 两条索引，头注写依据（对应哪条查询的谓词与排序）。
- [x] 用例：在一次性库上灌够数量的合成预报与交接单，`EXPLAIN` 两条列表查询走到新索引（不走 Seq Scan）；既有列表用例照过。——带节点的读法成立；不带节点的全租户读不成立，见完成记录「发现」。
- [x] Go 门禁；全量用 `scripts/test-full.sh`；机制清点重生成。
- [x] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）。——作者半边：独立工作树与分支、pathspec 提交、全量一跑；推送与进 main 归推送方。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-psxm15`）

起点 main `a657bef1`，完工后 rebase 到 `origin/main` `cb31172f`。证据层级 **S**：夹具全为 `SYN-` 合成值，跑在隔离测试库上。

**提交**

| 提交 | 内容 |
|---|---|
| `b390f53e` | 迁移 `node_operations/0021` 两条索引；计划用例 |
| `e7e16894` | 在 `b390f53e` 上重生成机制清点（NO 测试文件 114→115、迁移 18→19） |
| 本笔 | 本完成记录 |

**落点**

- 迁移 `migrations/node_operations/0021_delivery_scope_catalogue_indexes.sql`。
- 用例在新文件 `internal/nodeoperations/adapters/postgres/delivery_scope_catalogue_plan_test.go`。没有追加进 `delivery_scope_catalogue_test.go`：要另开一只带查询追踪的连接池，夹具与既有两条列表用例不共用。
- 读口 `DeliveryScopeCatalogue` 一字未改。

**两条索引对应哪条查询**

| 索引 | 查询 | 键 |
|---|---|---|
| `delivery_pre_advice_by_node_expected_arrival` | `DeliveryScopeCatalogue.ListDeliveryPreAdvices` | `(tenant_id, node_ref, expected_arrival_at DESC, pre_advice_id)` |
| `delivery_handover_note_by_node_opening` | `DeliveryScopeCatalogue.ListDeliveryHandoverNotes` | `(tenant_id, node_ref, opened_at DESC, note_id)` |

- 比裁定的「（租户，节点，排序时刻）」多一列标识。查询是「时刻倒序、同刻按标识」，标识不进键，同刻那几行还要另排一次。
- 列向逐列与查询同向。在另起的一次性实例上（同样每租户 2000 行）实测，两种别的建法都会在计划里多出一个 `Incremental Sort`（presorted key 是时刻）：
  - `(tenant_id, node_ref, expected_arrival_at, pre_advice_id)`：全正序，反着读出来是「时刻倒、标识倒」。
  - `(tenant_id, node_ref, expected_arrival_at DESC)`：标识不进键。
- 在事务内建，不用 `CONCURRENTLY`，照裁定。

**EXPLAIN 结论**（两租户各 2000 行、8 个节点，业务时刻每行错 17 分钟；本租户每张交接单两件成员；灌完 `ANALYZE`）

| 读法 | 计划（预报；交接单同形，另带成员计数子计划 `Index Only Scan using delivery_handover_member_by_note`） | 断言 |
|---|---|---|
| 节点与一日（约 11 行） | `Limit → Sort → Bitmap Heap Scan → Bitmap Index Scan using <新索引>` | 经新索引、无 Seq Scan |
| 只按节点 | `Limit → Index Scan using <新索引>` | 经新索引、无 Seq Scan、无排序 |
| 全租户一日 | `Limit → Sort → Seq Scan`；关 seqscan 后，用例那次是 `Bitmap Index Scan using <表>_pkey`，另起实例那次是新索引 | 不断言 |
| 全租户（不筛） | `Limit → Sort → Seq Scan`；关 seqscan 后 `Bitmap Index Scan using <表>_pkey` | 不断言 |

- 带节点的两种读法：没改任何计划器设置，规划器自己选了新索引。节点与一日那一读只有十来行，规划器取位图扫描再排序，估价比按键序逐行回表低，所以不断言无排序。
- 先红后绿：把 0021 移出迁移目录，四格都红，计划是 `Limit → Sort → Seq Scan`，只按节点那两格还连带报排序。放回即绿。
- 用例 EXPLAIN 的是读口实际发出的语句与参数，经 `pgx` 查询追踪原样记下，测试里不抄 SQL。参数随语句绑定，「参数为空即不设界」那几支在规划时就折掉，得到的是定制计划。
- 定制计划还是通用计划：读口经 `pgx` 的预备语句执行，`plan_cache_mode = auto` 时规划器会在第五次之后比较通用计划。在另起的一次性实例上用 `PREPARE` 同形语句实测：
  - 只按节点连跑 7 次后，`pg_prepared_statements` 记 8 次定制、0 次通用。
  - 强制通用计划时，索引条件只剩 `tenant_id = $1`，其余条件是 Filter，上面还压一个 Sort。估价高于定制计划，所以 `auto` 不会选它。
  - 本格只在那次实验里看过，用例不断言。

**用例**

- 新：`TestPreAdviceAndHandoverNoteListsReadThroughTheirNodeIndexes`，四个子用例：预报、交接单各两种带节点的读法。
- 灌量之后照旧证答复本身：节点逐字、区间左闭右开、时刻倒序同刻按标识、不带区间读满一页、交接单每行成员数 2。
- 既有列表用例照过：`TestPreAdvicesAreListedByNodeAndExpectedArrivalWithinTheTenant`、`TestHandoverNotesAreListedByNodeAndOpeningWithClaimsAndMembers`。
- 灌量走一条 `INSERT … SELECT generate_series`，不逐行过写口。读回仍过还原门，列表照常调用。
- `ANALYZE` 不是计划器设置，代的是生产上 autovacuum 维护的统计。用例里没有任何 `SET`。

**门**（在 `e7e16894` 上）

- `gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0。
- `scripts/test-full.sh`：153 ok / 0 FAIL（另 16 个包无测试文件）。
- 机制清点重生成有差，单独一笔 `e7e16894`。

**发现：全租户读没有被这两条索引服务——管理台两区默认打开的正是这一读**

- 裁定写「节点筛选缺席时的全租户列也能用同一条索引的前缀加排序」。实测不成立：
  - 对全租户读，新索引给得出的只有租户前缀，主键 `(tenant_id, <标识>)` 也给得出，而且更窄。
  - 本租户占册表大头时，规划器选顺序扫描，带一日与不带一日都一样。
  - 关掉 seqscan 后，不带一日选主键位图扫描；带一日时主键与新索引估价贴得很近，两次实测各选了一个。
  - 无论选哪个，都是租户内全量筛，再排序取一页。
- 管理台两区默认打开时，两格筛选都是空的：`DeliveryPreAdvicesSection` 与 `DeliveryHandoverNotesSection` 的初值都是 `{ node: '', day: '' }`，`deliveryScopeListPathOf` 这时不带查询串。所以「现象」里那句「复核页每开一次就是一次全表扫描」，对默认打开那一下仍然成立。本票只解了按节点筛之后的读。
- 迁移头注照实写了这一条。用例不断言全租户读：顺序扫描下断言「经新索引」是假的，关掉 seqscan 后又选哪条不稳，证不出什么。
- 留给评审定，作者不越裁定（以下各条均未实测）：
  - 甲：每本册再加一条 `(tenant_id, <业务时刻> DESC, <标识>)`。四种读法各有一条索引能按键序读满一页即停；代价是写口每行多维护一本索引。
  - 乙：每本册改用 `(tenant_id, <业务时刻> DESC, <标识>)` 一条，代替节点那条。带节点的读要在租户的时刻序里逐行滤节点，节点多、某节点稀疏时会读很远。
  - 丙：页面默认带当日，让默认那一读落在区间上。区间在全租户读上也只是索引内筛，仍要再排一次序，单靠这一条不够。

**没做**

- 全租户读的索引：见上，裁定外，未加。
- `CONCURRENTLY`：仍是 `migrate.applyStep` 头注那条显式未决，照裁定。
