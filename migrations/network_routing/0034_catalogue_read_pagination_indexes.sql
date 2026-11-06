-- 票 catalogue-read-pagination-rollout/07：network-routing 其余各册迁到 ADR-0144 之后，各册按「租户 + 排序维 + 行标识」取页，
-- 每册补一条与缺省序同形的索引（ADR-0144 决定七）。下列各表上都没有以「租户 + 排序维」打头的旧索引，没有要换掉的。

-- 初始路由判断册：缺省序落册时刻倒序，标识是判断键去掉租户后余下的各段。
CREATE INDEX initial_route_by_tenant_recorded
    ON network_routing.initial_route
        (tenant_id, recorded_at DESC, customer_account_id DESC, shipment_request_id DESC,
         acceptance_baseline DESC, declared_parcel_id DESC, service_purpose DESC);

-- 路由复核册：缺省序落册时刻倒序，标识触发关联。
CREATE INDEX route_reassessment_by_tenant_recorded
    ON network_routing.route_reassessment (tenant_id, recorded_at DESC, correlation_id DESC);

-- 改路建议册：建议住在复核行上，缺省序与标识同复核册，只取带建议的行；谓词与读面的条件是同一句。状态由读面导出，不进索引。
CREATE INDEX route_reassessment_suggestion_by_tenant_recorded
    ON network_routing.route_reassessment (tenant_id, recorded_at DESC, correlation_id DESC)
    WHERE suggestion IS NOT NULL;

-- 初始路由并列册：缺省序最近一次停住的时刻倒序，标识是判断键去掉租户后余下的各段、以包裹打头；只取停在并列的行。
CREATE INDEX initial_route_not_formed_tied_by_tenant_settled
    ON network_routing.initial_route_not_formed
        (tenant_id, settled_at DESC, declared_parcel_id DESC, customer_account_id DESC,
         shipment_request_id DESC, acceptance_baseline DESC, service_purpose DESC)
    WHERE reason = 'CANDIDATES_TIED';
