-- 票 catalogue-read-pagination-rollout/06：node-operations 的各列表迁到 ADR-0144 之后，各册按「租户 + 排序维 + 行标识」取页，
-- 每迁一册补一条与缺省序同形的索引（ADR-0144 决定七）。决胜键与排序维同向（决定三）：已有同名索引前缀相同而决胜键反向或缺标识段的，
-- 换成同名的同向索引，不让两条前缀相同的索引在写路径上各付一次。
--
-- 不新建索引的两册：集运单元按单元号正序，主键（租户，单元号）就是它的键序；身份复核单元按导出列（每件实物最近一次待识别的登记时刻）
-- 排，导出列上建不了同形索引，分组读的是待识别那几行，走下面待识别实物那条部分索引的租户前缀。
--
-- 不用 CONCURRENTLY：本仓迁移执行器只收能在事务块内运行的迁移（migrate.applyStep 头注写明了这一条显式未决）。

-- 收寄册：缺省序登记时刻倒序，标识来源标识。只列带收寄的两种结果；部分条件与读面的种类条件逐字同形，规划器才认得出这一读可以走它。
CREATE INDEX reception_catalogue_by_recorded
    ON node_operations.reception (tenant_id, recorded_at DESC, source_id DESC)
    WHERE kind IN ('INTAKE_FORMED', 'PENDING_IDENTIFICATION');

-- 待识别实物册：缺省序与标识同收寄册，只列待识别一种结果。收寄册那条的条件也蕴含这一条，但走它要在租户的全部收寄里逐行滤掉已成
-- 收寄的那些。
CREATE INDEX reception_pending_catalogue_by_recorded
    ON node_operations.reception (tenant_id, recorded_at DESC, source_id DESC)
    WHERE kind = 'PENDING_IDENTIFICATION';

-- 集运成员关系册：缺省序记录时刻倒序，标识来源标识；只列移入与移出两种来源事实。
CREATE INDEX consolidation_fact_membership_by_recorded
    ON node_operations.consolidation_fact (tenant_id, recorded_at DESC, source_id DESC)
    WHERE action IN ('ADD_MEMBER', 'REMOVE_MEMBER');

-- 节点控制转移册：缺省序登记时刻倒序，标识（交接对象、交接范围、交接版本、方向）。0017 建的同名索引只有登记时刻，换成补齐标识段的。
DROP INDEX node_operations.physical_control_transition_by_recorded;
CREATE INDEX physical_control_transition_by_recorded
    ON node_operations.physical_control_transition
        (tenant_id, recorded_at DESC, handover_object DESC, handover_scope DESC, handover_version DESC, direction DESC);

-- 送站预报与送站交接单：缺省序业务时刻倒序（预报取预计送达、交接单取开立），标识预报号、交接单号。0021 为这两册建的索引按「同刻按
-- 标识正序」建，换成同名的同向索引；带节点与不带节点各一条的理由见 0021 头注，不变。
DROP INDEX node_operations.delivery_pre_advice_by_node_expected_arrival;
CREATE INDEX delivery_pre_advice_by_node_expected_arrival
    ON node_operations.delivery_pre_advice (tenant_id, node_ref, expected_arrival_at DESC, pre_advice_id DESC);

DROP INDEX node_operations.delivery_pre_advice_by_expected_arrival;
CREATE INDEX delivery_pre_advice_by_expected_arrival
    ON node_operations.delivery_pre_advice (tenant_id, expected_arrival_at DESC, pre_advice_id DESC);

DROP INDEX node_operations.delivery_handover_note_by_node_opening;
CREATE INDEX delivery_handover_note_by_node_opening
    ON node_operations.delivery_handover_note (tenant_id, node_ref, opened_at DESC, note_id DESC);

DROP INDEX node_operations.delivery_handover_note_by_opening;
CREATE INDEX delivery_handover_note_by_opening
    ON node_operations.delivery_handover_note (tenant_id, opened_at DESC, note_id DESC);
