-- 票 catalogue-read-pagination-rollout/02：parcel-shipment 九册迁到 ADR-0144 之后，各册按「租户 + 排序维 + 行标识」取页，
-- 每迁一册补一条与缺省序同形的索引（ADR-0144 决定七）。已有同形前缀而缺标识段的，换成补齐标识段的同名索引，
-- 不让两条前缀相同的索引在写路径上各付一次。

-- 面单交易：缺省序建立时刻倒序，标识交易号。
DROP INDEX parcel_shipment.label_transaction_by_tenant;
CREATE INDEX label_transaction_by_tenant
    ON parcel_shipment.label_transaction (tenant_id, established_at DESC, label_transaction_id DESC);

-- 委托查阅：缺省序提交时刻倒序，标识委托号；作用域里的客户账户集合在这之上过滤。
CREATE INDEX shipment_request_by_tenant_submitted
    ON parcel_shipment.shipment_request (tenant_id, submitted_at DESC, shipment_request_id DESC);

-- 复核队列与授权处置队列：缺省序提交时刻（最早的在前），部分索引补上标识段，谓词不变。
DROP INDEX parcel_shipment.shipment_request_manual_review_queue;
CREATE INDEX shipment_request_manual_review_queue
    ON parcel_shipment.shipment_request (tenant_id, submitted_at, shipment_request_id)
    WHERE task_waiting_on = 3 AND state = 1;

DROP INDEX parcel_shipment.shipment_request_authorized_disposition_queue;
CREATE INDEX shipment_request_authorized_disposition_queue
    ON parcel_shipment.shipment_request (tenant_id, submitted_at, shipment_request_id)
    WHERE task_waiting_on = 5 AND state = 1;

-- 渠道择优的并列冲突：缺省序决定时刻倒序，标识决定号。结论由读面以参数传入，所以结论进键而不做部分索引；
-- 按对象收窄时仍走 channel_selection_decision_by_subject。
CREATE INDEX channel_selection_decision_by_conclusion
    ON parcel_shipment.channel_selection_decision (tenant_id, conclusion, decided_at DESC, decision_id DESC);

-- 释放未受理册：缺省序登记时刻，标识（委托、提交版本、控制结果）。
DROP INDEX parcel_shipment.control_release_refusal_tenant_recorded;
CREATE INDEX control_release_refusal_tenant_recorded
    ON parcel_shipment.control_release_refusal
        (tenant_id, recorded_at, shipment_request_id, submission_version, control_result_id);

-- 取消先后冲突册：缺省序发现时刻，标识（包裹、边界种类、边界那一代、取消）。
CREATE INDEX cancellation_order_conflict_by_tenant_detected
    ON parcel_shipment.cancellation_order_conflict
        (tenant_id, detected_at, parcel_id, boundary_kind, boundary_ref, cancellation_id);

-- 等回应的处置请求：缺省序记下时刻，标识（请求键、包裹）；两路来源里量大的待处置那一路走这条。
DROP INDEX parcel_shipment.parcel_cancellation_pending_by_tenant;
CREATE INDEX parcel_cancellation_pending_by_tenant
    ON parcel_shipment.parcel_cancellation (tenant_id, decided_at, request_key, parcel_id)
    WHERE kind = 'DISPOSITION_PENDING';

-- 待人工处理项：缺省序决定时刻（落在 created_at，写入时取自所属决定），标识（包裹、决定版本、所有者、动作）。
DROP INDEX parcel_shipment.post_intake_disposition_manual_item_by_tenant;
CREATE INDEX post_intake_disposition_manual_item_by_tenant
    ON parcel_shipment.post_intake_disposition_manual_item
        (tenant_id, created_at, parcel_id, decision_version, owner, action);
