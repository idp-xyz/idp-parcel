-- 履约参与关系上的替代链（ADR-0112 决定一；票 tf-segment-lifecycle-closure/10）。
--
-- 0006 把主键取成（租户+段+对象），头注写「已结束的参与也不重开——再次进入是新的段，所以这里不需要
-- 版本维」。那句话的前半仍成立——承运责任变了才是另一段；后半被 CONTEXT 生命周期句的后半推翻：
-- 「来源证据被更正……保留原段、原参与关系和原判断，形成失效或替代关系并重新派生当前有效控制」。
-- 参与关系以来源版本为入场依据，来源一更正，段里那条参与指着的就是一个已被回指的版本、一个已被更正的
-- 起点——同段内要长出替代版本，主键因此纳入 entry_basis（入场依据本就是来源版本引用的写法）。
--
-- **登记册仍只插不改**：原参与那一行一字不动，「被替代」不落列，读回时按回指派生——没有任何行回指它的
-- 那一版是当前参与（形照 0015 offsite_pickup 与 parcel_shipment 0017 intake_adoption 的链）。
ALTER TABLE transport_fulfillment.fulfillment_participation
    DROP CONSTRAINT fulfillment_participation_pkey;

ALTER TABLE transport_fulfillment.fulfillment_participation
    ADD CONSTRAINT fulfillment_participation_pkey
        PRIMARY KEY (tenant_id, segment_ref, object_ref, entry_basis);

ALTER TABLE transport_fulfillment.fulfillment_participation
    ADD COLUMN supersedes_entry_basis text;

-- 回指不自指、不空白。回指与原参与的其它关系（继承离场三件、起点不晚于终点）是跨行事实，CHECK 表达不了，
-- 由领域重派生门与重建门守。
ALTER TABLE transport_fulfillment.fulfillment_participation
    ADD CONSTRAINT fulfillment_participation_supersedes_coherent
        CHECK (
            supersedes_entry_basis IS NULL
            OR (
                btrim(supersedes_entry_basis) <> ''
                AND supersedes_entry_basis <> entry_basis
            )
        );

-- 自引用外键：被替代的必须是**同租户同段同对象**下登过的一版——链不悬空、不跨对象、不跨段。
-- MATCH SIMPLE 下 supersedes_entry_basis 为 NULL 的首登行不受检。
ALTER TABLE transport_fulfillment.fulfillment_participation
    ADD CONSTRAINT fulfillment_participation_supersedes_fkey
        FOREIGN KEY (tenant_id, segment_ref, object_ref, supersedes_entry_basis)
        REFERENCES transport_fulfillment.fulfillment_participation (tenant_id, segment_ref, object_ref, entry_basis);

-- 两道部分唯一索引让链严格线性，ParticipationFor 才答得出唯一的链尾：一对象在一段里只有一个首登
-- （0006 主键原来担的那一半，`Join` 撞它译`已在段内`）；一版最多被替代一次（并发第二次替代同一前版
-- 撞它译`已替代`，读回先到的那一次）。ON CONFLICT DO NOTHING 对主键与两道索引一视同仁。
CREATE UNIQUE INDEX fulfillment_participation_one_root_per_object
    ON transport_fulfillment.fulfillment_participation (tenant_id, segment_ref, object_ref)
    WHERE supersedes_entry_basis IS NULL;

CREATE UNIQUE INDEX fulfillment_participation_supersedes_once
    ON transport_fulfillment.fulfillment_participation (tenant_id, segment_ref, object_ref, supersedes_entry_basis)
    WHERE supersedes_entry_basis IS NOT NULL;

-- 两条**故意没有下沉**的不变量，与 0006 尾注同一个理由（跨行条件、不立第二口径）：
--   - 「在场」= ended_at 为空**且无人回指**——被替代的版本不再表达当前控制，不算在场也不会被结束；
--     EndParticipation 与 FindActiveSegments 两个窄口用同一段 NOT EXISTS 表达它。
--   - 替代版本继承原参与的离场三件、起点不晚于继承的终点——领域重派生门守。
