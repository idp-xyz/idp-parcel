-- 票 catalogue-read-pagination-rollout/05：运输履约查阅页 registry 分派的各册与外部承运轨迹事实当前版迁到 ADR-0144 之后，
-- 各册按「租户 + 排序维 + 行标识」取页，每一册要一条与缺省序同形的索引（ADR-0144 决定七）。行标识与排序同向作决胜键，
-- 缺省序倒序的册标识段也倒序。已有同形前缀而缺标识段的，换成补齐标识段的同名索引，不让两条前缀相同的索引在写路径上
-- 各付一次。
--
-- 已有索引恰与缺省序同形、本迁移不再新建的几册，复用的索引在此点名：
--   周期安排册      主键 (tenant_id, template_code, template_version)
--   容量池册        主键 (tenant_id, pool_id)
--   派送待形成册    delivery_dispatch_pending_open_by_tenant（0045，谓词 state = 'PENDING'）
--   重问间隔册      delivery_dispatch_reask_cadence_current（0046）
--   实际履约未形成册 actual_fulfillment_unformed_open_by_tenant（0048，谓词 state = 'UNFORMED'）
--
-- 待确认复核清单不在上列，本迁移也不为它新建：它是揽收、派送两侧待确认的行并起来的一册，两侧各有一条部分索引（0055，谓词
-- outcome = 'PENDING_CONFIRMATION'），都只部分同形。delivery_attempt_result_pending 是（租户，发生时刻，尝试，对象），派送一支的
-- 「一侧」恒为 DELIVERY、「来源身份」恒为空串，按它取出的次序就是本册在这一支的次序；pickup_attempt_result_pending 是（租户，
-- 发生时刻，来源身份，对象），只有（租户，发生时刻）这段前缀与缺省序同形——本册在揽收一支接着按尝试排，尝试取自联上的
-- pickup_attempt，不在 pickup_attempt_result 上。两支并起来之后整册还要再排一遍，哪一条单表索引都给不出整册的序；待确认是要人
-- 逐件清空的复核清单，只含还没认定的那几件，再排的只是这几件。

-- 班次册：只列每个班次的当前版；缺省序出发时刻倒序，标识（班次，版本）。
CREATE INDEX transport_schedule_by_tenant_departure
    ON transport_fulfillment.transport_schedule (tenant_id, departs_at DESC, schedule_id DESC, schedule_version DESC);

-- 权威交接判断册：一行一版本，按包裹计量；缺省序裁决时刻倒序，标识（对象，范围，版本）。
CREATE INDEX transport_handover_by_tenant_judged
    ON transport_fulfillment.transport_handover
        (tenant_id, judged_at DESC, object_ref DESC, scope_ref DESC, handover_version DESC);

-- 有效交付结果册：只列当前版，按包裹计量；缺省序交付发生时刻倒序，标识（对象，尝试，版本），谓词与读面同为 is_current。
CREATE INDEX effective_delivery_current_by_tenant_occurred
    ON transport_fulfillment.effective_delivery
        (tenant_id, occurred_at DESC, object_ref DESC, attempt_ref DESC, delivery_version DESC)
    WHERE is_current;

-- 总单册：一行一版本；缺省序登记时刻倒序，标识（总单，版本）。
CREATE INDEX carrier_master_document_by_tenant_recorded
    ON transport_fulfillment.carrier_master_document (tenant_id, recorded_at DESC, master_document_ref DESC, version DESC);

-- 运输准备结果册：按包裹计量；缺省序判断时刻倒序，标识（段，对象，规则版本）。原索引只到判断时刻，换成补齐标识段的同名索引。
DROP INDEX transport_fulfillment.transport_preparation_judged;
CREATE INDEX transport_preparation_judged
    ON transport_fulfillment.transport_preparation
        (tenant_id, judged_at DESC, planned_leg_ref DESC, object_ref DESC, rule_version DESC);

-- 订舱册：缺省序申请时刻倒序，标识订舱。
CREATE INDEX booking_request_by_tenant_requested
    ON transport_fulfillment.booking_request (tenant_id, requested_at DESC, booking_id DESC);

-- 承运答复挡回册：缺省序最近一次挡回时刻倒序，标识（答复方，订舱，答复，依据）。
CREATE INDEX booking_answer_bounce_by_tenant_last_bounced
    ON transport_fulfillment.booking_answer_bounce
        (tenant_id, last_bounced_at DESC, source_ref DESC, booking_id DESC, acceptance_id DESC, basis DESC);

-- 外部承运轨迹事实当前版：按事件计量；源是必给的选择器，进键在排序维之前；缺省序发生时刻升序，标识（事实，版本）。
CREATE INDEX external_carrier_tracking_fact_by_source_occurred
    ON transport_fulfillment.external_carrier_tracking_fact (tenant_id, source_ref, occurred_at, fact_ref, version);
