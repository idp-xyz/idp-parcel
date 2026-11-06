-- 票 catalogue-read-pagination-rollout/03：visibility-exception 的列表册迁到 ADR-0144 之后，各册按「租户 + 排序维 + 行标识」
-- 取页，每迁一册补一条与缺省序同形的索引（ADR-0144 决定七）。本模块此前没有与这些缺省序同前缀的索引，下面全是新建；
-- 按键升序的几册由主键承担，见文末。

-- 追踪投影：缺省序派生时刻倒序，标识包裹。排序列在版本表上，键也建在版本表：按它走会跨过被替代的历史版，
-- 由读面按当前标记联回时滤掉。
CREATE INDEX tracking_projection_version_by_tenant_derived
    ON visibility_exception.tracking_projection_version (tenant_id, derived_at DESC, parcel_ref DESC);

-- 异常案件：缺省序建立时刻倒序，标识案件号。
CREATE INDEX exception_case_by_tenant_established
    ON visibility_exception.exception_case (tenant_id, established_at DESC, case_id DESC);

-- 信号发作期：缺省序首启时刻倒序，标识发作期号；到达序 seq 是 FindLatest 的并列裁决序，不进这条键。
CREATE INDEX signal_episode_by_tenant_started
    ON visibility_exception.signal_episode (tenant_id, started_at DESC, episode_id DESC);

-- 处置请求：缺省序发送时刻倒序，标识请求号。
CREATE INDEX disposition_request_by_tenant_sent
    ON visibility_exception.disposition_request (tenant_id, sent_at DESC, request_id DESC);

-- 客户通知：缺省序决定时刻倒序，标识通知号。
CREATE INDEX customer_notification_by_tenant_decided
    ON visibility_exception.customer_notification (tenant_id, decided_at DESC, notification_id DESC);

-- 索赔项：缺省序提交时刻倒序，标识（批次，项）。
CREATE INDEX claim_item_by_tenant_submitted
    ON visibility_exception.claim_item (tenant_id, submitted_at DESC, batch_ref DESC, item_id DESC);

-- 追偿事项：缺省序建立时刻倒序，标识事项号。
CREATE INDEX recovery_matter_by_tenant_opened
    ON visibility_exception.recovery_matter (tenant_id, opened_at DESC, matter_id DESC);

-- 区间型目录（里程碑映射、分诊规则、披露策略、异常披露规则）：缺省序生效起点倒序，标识版本号。
CREATE INDEX milestone_mapping_version_by_tenant_effective
    ON visibility_exception.milestone_mapping_version (tenant_id, effective_from DESC, mapping_version DESC);

CREATE INDEX triage_rule_version_by_tenant_effective
    ON visibility_exception.triage_rule_version (tenant_id, effective_from DESC, rule_version DESC);

CREATE INDEX disclosure_policy_version_by_tenant_effective
    ON visibility_exception.disclosure_policy_version (tenant_id, effective_from DESC, policy_version DESC);

CREATE INDEX exception_disclosure_rule_version_by_tenant_effective
    ON visibility_exception.exception_disclosure_rule_version (tenant_id, effective_from DESC, rule_version DESC);

-- 通知策略、索赔资格、索赔授权按键升序，键即标识：notification_policy_pkey、claim_contract_scope_pkey、
-- claim_authorization_catalogue_pkey 已是（租户，键）同形，不重建。冲突信号规则一租户至多一行，
-- conflict_signal_rule_pkey（租户）即够。
