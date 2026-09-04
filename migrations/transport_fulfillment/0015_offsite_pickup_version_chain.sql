-- 揽收登记的更正走新版本（票 tf-segment-lifecycle-closure/08 裁决 A；CONTEXT「来源证据被更正时，
-- 保留原事实和原判断，形成失效、替代及重新派生结果」）。
--
-- 0005 把主键取成（租户+对象+尝试）——那时一次尝试对一个对象只有一行，版本只是行上的一列。更正形成
-- 同键下的新行新版本、回指前版、原行一字不动（`OffsitePickup.Correct`），主键因此纳入 pickup_version，
-- 版本链在行上回指（corrects_version / corrected_at），形照 transport_handover。**登记册仍只插不改**：
-- 表上没有 current 列，「当前版」按回指派生——没有任何行回指它的那一版（同 0014 effective_time_rule）。
--
-- 两道部分唯一索引让链严格线性，FindByKey 才答得出唯一的当前版：一键只有一个首登行（并发第二次
-- 首登撞它译`已登记`，与 0005 主键原来担的那一半同义）；一版最多被更正一次（并发第二次更正同一前版
-- 撞它译`已登记`，读回先到的那一次）。ON CONFLICT DO NOTHING 对三道唯一约束一视同仁。
ALTER TABLE transport_fulfillment.offsite_pickup
    DROP CONSTRAINT offsite_pickup_pkey;

ALTER TABLE transport_fulfillment.offsite_pickup
    ADD CONSTRAINT offsite_pickup_pkey
        PRIMARY KEY (tenant_id, object_ref, attempt_ref, pickup_version);

ALTER TABLE transport_fulfillment.offsite_pickup
    ADD COLUMN corrects_version text,
    ADD COLUMN corrected_at     timestamptz;

-- 版本链两形态互斥：首登无前版无更正时间；更正版两者齐、不自指。更正时间与被更正版本登记时间的
-- 先后是跨行事实，CHECK 表达不了，由编排在读回前版时守。
ALTER TABLE transport_fulfillment.offsite_pickup
    ADD CONSTRAINT offsite_pickup_chain_coherent
        CHECK (
            (corrects_version IS NULL AND corrected_at IS NULL)
            OR (
                corrects_version IS NOT NULL
                AND btrim(corrects_version) <> ''
                AND corrects_version <> pickup_version
                AND corrected_at IS NOT NULL
            )
        );

CREATE UNIQUE INDEX offsite_pickup_one_first_registration
    ON transport_fulfillment.offsite_pickup (tenant_id, object_ref, attempt_ref)
    WHERE corrects_version IS NULL;

CREATE UNIQUE INDEX offsite_pickup_corrects_once
    ON transport_fulfillment.offsite_pickup (tenant_id, object_ref, attempt_ref, corrects_version)
    WHERE corrects_version IS NOT NULL;
