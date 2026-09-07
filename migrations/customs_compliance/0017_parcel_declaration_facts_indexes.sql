-- 按正式包裹反查申报链的读面（ADR-0118 决定四拆出的票 ps-port-remainder/05）：parcel-shipment
-- 判「资料修订阶段」要问关务三件事——该包裹是不是某个尚无提交版本的申报单元的成员、有没有
-- 已固定的提交版本把它写进组成快照、所在案件有没有当前已关闭。三件都按包裹引用反查 jsonb
-- 列。0002 自注「成员只在版本固定那一刻有意义，无按成员检索的读面」到此不再成立：读面立了，
-- 索引随它立；0002 那句留在原文里作历史，不改写已施加的迁移。
--
-- GIN + jsonb_path_ops 只服务 @> 包含查询——读面只问「这个引用在不在数组里」，不问键存在，
-- 小一半的索引换掉 ? 操作符是划算的。租户维不进 GIN（不引 btree_gin 扩展），由 WHERE 的
-- tenant_id 等值条件在回表时再收；成员列上的命中本就按包裹引用收得很窄。
--
-- 只加索引，不动任何表的形状与约束：读面读的是既有事实，没有新事实要落。

CREATE INDEX declaration_unit_members_gin
    ON customs_compliance.declaration_unit USING GIN (members jsonb_path_ops);

CREATE INDEX declaration_submission_members_gin
    ON customs_compliance.declaration_submission USING GIN (members jsonb_path_ops);

-- 案件建立时的直接包裹关联是「所在案件」的另一路（CaseParcelAssociation 落在 parcels
-- 数组里，元素形如 {"parcel": …, "customer": …, "sourceRef": …}）；@> 对对象数组同样按
-- 路径匹配，jsonb_build_array(jsonb_build_object('parcel', $1)) 即命中含该包裹的元素。
CREATE INDEX customs_case_parcels_gin
    ON customs_compliance.customs_case USING GIN (parcels jsonb_path_ops);

-- 「这个单元有没有被替代」是读面排除死单元的那一问；替代关系今天还没有写入方（0010
-- 自注：替代编排随后续票），索引先立在列上，写入方来了读面不必改。
CREATE INDEX declaration_unit_replaces_idx
    ON customs_compliance.declaration_unit (tenant_id, replaces_unit_id)
    WHERE replaces_unit_id IS NOT NULL;
