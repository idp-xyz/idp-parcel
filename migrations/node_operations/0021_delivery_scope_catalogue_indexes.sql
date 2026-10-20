-- 送站预报与送站交接单两条查阅列表的索引（票 ps-external-mark-relations/15 裁定）。迁移 0013 只为写口与按标识读建了键，管理台的两条
-- 列表（DeliveryScopeCatalogue.ListDeliveryPreAdvices、DeliveryScopeCatalogue.ListDeliveryHandoverNotes）在本租户全表上筛与排序；
-- 两本册只增、逐日累积，复核页每开一次就是一次全表扫描。只加索引，不动表形状、约束与两条查询；不含任何实例默认值。
--
-- 两条查询同形：租户等值；节点等值（可缺）；业务时刻（预报取预计送达、交接单取开立）左闭右开区间（两端各可缺）；业务时刻倒序、同刻
-- 按标识正序，取一页。键序照这个形状排：
--
-- - 租户、节点两个等值列在前，业务时刻在后：带节点时，区间是同一节点下的一段连续键，键序就是答复的次序，可以读满一页即停、不排序。
-- - 标识进键：「同刻按标识」是排序的一部分，不进键的话同一时刻的那几行还要另排一次。
-- - 列向逐列与查询同向（时刻倒、标识正）。btree 反着读给得出的只是建时列向的整体反向：两列都按默认正序建，反读得到「时刻倒、标识
--   倒」，同一时刻的那几行照样要另排一次，标识就白进了键。
--
-- 不带节点的全租户列表，这两条索引帮不上：对那一读它们给得出的只有租户前缀，与主键一样，全租户的时刻序不在键里。那一读仍是租户内
-- 全量筛、排序再取一页——在本租户占册表大头的数据上，规划器选顺序扫描；关掉顺序扫描，它在这条索引与更窄的主键之间选，选哪条都
-- 一样要全量筛再排序（票 15 完成记录）。票 15 裁定每本册只取这一条，本迁移不为全租户读另立（租户，业务时刻）一条。交接单列表每行
-- 的成员计数是另一张表上的相关子查询，走 0013 的 delivery_handover_member_by_note，本迁移不动它。
--
-- 不用 CONCURRENTLY：本仓迁移执行器只收能在事务块内运行的迁移（migrate.applyStep 头注写明了这一条显式未决）。建索引期间两张表的
-- 写口要等，表今天还小。

-- 对 ListDeliveryPreAdvices：WHERE tenant_id、node_ref、expected_arrival_at 区间；ORDER BY expected_arrival_at DESC, pre_advice_id。
CREATE INDEX delivery_pre_advice_by_node_expected_arrival
    ON node_operations.delivery_pre_advice (tenant_id, node_ref, expected_arrival_at DESC, pre_advice_id);

-- 对 ListDeliveryHandoverNotes：WHERE tenant_id、node_ref、opened_at 区间；ORDER BY opened_at DESC, note_id。
CREATE INDEX delivery_handover_note_by_node_opening
    ON node_operations.delivery_handover_note (tenant_id, node_ref, opened_at DESC, note_id);
