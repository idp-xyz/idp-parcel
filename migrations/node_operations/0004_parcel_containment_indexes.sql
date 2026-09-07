-- 按正式包裹的版本化关联反查作业实物的读面（ADR-0118 决定四拆出的票 ps-port-remainder/05）：
-- parcel-shipment 判「资料修订阶段」要问节点作业「该包裹此刻在不在某个未关闭集运单元里」。
-- 正式包裹与作业实物之间只有两条路，都落在 0001 的收寄判断行上：识别成功后建立的版本化
-- 关联（intake ->> 'association'），以及待识别实物的候选关联（candidates 数组）。此前没有按
-- 它们反查的读面；读面立了，索引随它立。
--
-- 关联那一路是等值匹配，带租户维的表达式 btree 即可；候选那一路问「数组里有没有这个引用」，
-- 走 GIN + jsonb_path_ops 的 @>。只加索引，不动表形状与约束：读面读的是既有事实。

CREATE INDEX reception_intake_association_idx
    ON node_operations.reception (tenant_id, (intake ->> 'association'))
    WHERE intake IS NOT NULL;

CREATE INDEX reception_candidates_gin
    ON node_operations.reception USING GIN (candidates jsonb_path_ops);
