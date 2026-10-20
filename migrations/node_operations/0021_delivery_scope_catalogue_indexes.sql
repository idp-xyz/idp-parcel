-- 送站预报与送站交接单两条查阅列表的索引（票 ps-external-mark-relations/15 裁定与裁定补）。迁移 0013 只为写口与按标识读建了
-- 键，管理台的两条列表（DeliveryScopeCatalogue.ListDeliveryPreAdvices、DeliveryScopeCatalogue.ListDeliveryHandoverNotes）在本
-- 租户全表上筛与排序；两本册只增、逐日累积，复核页每开一次就是一次全表扫描。只加索引，不动表形状、约束与两条查询；不含任何实例
-- 默认值。
--
-- 两条查询同形：租户等值；节点等值（可缺）；业务时刻（预报取预计送达、交接单取开立）左闭右开区间（两端各可缺）；业务时刻倒序、同刻
-- 按标识正序，取一页。节点在不在决定了读的是哪一段键，所以每本册两条索引：带节点的读走（租户，节点，业务时刻，标识），不带节点的
-- 全租户读走（租户，业务时刻，标识）。
--
-- 带节点的那条，键序这样排：
--
-- - 租户、节点两个等值列在前，业务时刻在后：带节点时，区间是同一节点下的一段连续键，键序就是答复的次序，可以读满一页即停、不排序。
-- - 标识进键：「同刻按标识」是排序的一部分，不进键的话同一时刻的那几行还要另排一次。
-- - 列向逐列与查询同向（时刻倒、标识正）。btree 反着读给得出的只是建时列向的整体反向：两列都按默认正序建，反读得到「时刻倒、标识
--   倒」，同一时刻的那几行照样要另排一次，标识就白进了键。
--
-- 不带节点的全租户那条是管理台两区默认打开时发的读（两格筛选都空），最常走。带节点那条帮不上它：节点夹在租户与时刻之间，对全租户
-- 读给得出的只有租户前缀，与主键一样，那一读就成了租户内全量筛、排序再取一页——实测规划器在本租户占册表大头的数据上选顺序扫描
-- （票 15 完成记录）。去掉节点，全租户的时刻序就在键里：不带区间时按键序读满一页即停，带区间时区间是租户下的一段连续键。列向与
-- 标识进键的理由同上。两本册写得远比读得少，多一条索引的写入代价可以不计（票 15 裁定补）。
--
-- 不只留全租户那一条：带节点的读在租户的时刻序里逐行滤节点，节点多、某个节点稀疏时要读很远才凑满一页。
--
-- 交接单列表每行的成员计数是另一张表上的相关子查询，走 0013 的 delivery_handover_member_by_note，本迁移不动它。
--
-- 不用 CONCURRENTLY：本仓迁移执行器只收能在事务块内运行的迁移（migrate.applyStep 头注写明了这一条显式未决）。建索引期间两张表的
-- 写口要等，表今天还小。

-- 对 ListDeliveryPreAdvices 带节点的读：WHERE tenant_id、node_ref、expected_arrival_at 区间；ORDER BY expected_arrival_at DESC, pre_advice_id。
CREATE INDEX delivery_pre_advice_by_node_expected_arrival
    ON node_operations.delivery_pre_advice (tenant_id, node_ref, expected_arrival_at DESC, pre_advice_id);

-- 对 ListDeliveryHandoverNotes 带节点的读：WHERE tenant_id、node_ref、opened_at 区间；ORDER BY opened_at DESC, note_id。
CREATE INDEX delivery_handover_note_by_node_opening
    ON node_operations.delivery_handover_note (tenant_id, node_ref, opened_at DESC, note_id);

-- 对 ListDeliveryPreAdvices 不带节点的读：WHERE tenant_id、expected_arrival_at 区间；ORDER BY expected_arrival_at DESC, pre_advice_id。
CREATE INDEX delivery_pre_advice_by_expected_arrival
    ON node_operations.delivery_pre_advice (tenant_id, expected_arrival_at DESC, pre_advice_id);

-- 对 ListDeliveryHandoverNotes 不带节点的读：WHERE tenant_id、opened_at 区间；ORDER BY opened_at DESC, note_id。
CREATE INDEX delivery_handover_note_by_opening
    ON node_operations.delivery_handover_note (tenant_id, opened_at DESC, note_id);
