-- 采认、取消与终局：UC-PS-003 / UC-PS-006 / UC-PS-004 的三张判断库。
--
-- intake_adoption 主键取幂等三维加租户（租户+包裹+来源类型+来源版本）；采用与不采用
-- 二居其一由 result_shape 钉住。「同一包裹不能形成两个责任起点」（AT-PS-049）落成
-- 部分唯一索引：每（租户+包裹）至多一行 adopted——并发第二个采用撞索引译`已有记录`。
--
-- parcel_cancellation 主键取（租户+请求键+包裹）：同一请求身份返回原逐包裹结果。
-- 三走向（取消成立/待处置/拒绝）的在场件矩阵逐走向 CHECK。
--
-- final_outcome 主键取幂等三维加租户（租户+包裹+结果种类+结果版本）。重派生翻旧插新：
-- 每（租户+包裹）至多一行 is_current（部分唯一索引），新版本行以 prior_version 指回
-- 前版，原终局历史保留在原版本行上（AT-PS-063）。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL，布尔列 NOT NULL——没有一条比较式能单独以 NULL 决定约束。

CREATE TABLE parcel_shipment.intake_adoption (
    tenant_id           text        NOT NULL,
    parcel_id           text        NOT NULL,
    source_kind         text        NOT NULL,
    source_version      text        NOT NULL,

    customer_account_id text        NOT NULL,
    shipment_request_id text        NOT NULL,
    content_digest      text        NOT NULL,
    adopted             boolean     NOT NULL,

    source_object       text,
    source_place        text,
    source_control      text,
    occurred_at         timestamptz,
    baseline_version    text,
    commitment_version  text,
    expected_commitment text,
    refusal_basis       text,
    adopted_at          timestamptz NOT NULL,

    CONSTRAINT intake_adoption_pkey
        PRIMARY KEY (tenant_id, parcel_id, source_kind, source_version),

    CONSTRAINT intake_adoption_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(source_version) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(content_digest) <> ''
        ),

    -- 合格来源封闭二值（AT-PS-043）：普通扫描、卸载、车辆到场在库里也没有格。
    CONSTRAINT intake_adoption_kind_closed
        CHECK (source_kind IN ('NODE_INTAKE', 'OFFSITE_PICKUP')),

    -- 采用带收寄与承诺全件、不采用带原因——两面互斥，缺件与混面都是坏写入。
    CONSTRAINT intake_adoption_result_shape
        CHECK (
            (adopted
                AND source_object IS NOT NULL AND btrim(source_object) <> ''
                AND source_place IS NOT NULL AND btrim(source_place) <> ''
                AND source_control IS NOT NULL AND btrim(source_control) <> ''
                AND occurred_at IS NOT NULL
                AND baseline_version IS NOT NULL AND btrim(baseline_version) <> ''
                AND commitment_version IS NOT NULL AND btrim(commitment_version) <> ''
                AND expected_commitment IS NOT NULL AND btrim(expected_commitment) <> ''
                AND refusal_basis IS NULL)
            OR (NOT adopted
                AND source_object IS NULL
                AND source_place IS NULL
                AND source_control IS NULL
                AND occurred_at IS NULL
                AND baseline_version IS NULL
                AND commitment_version IS NULL
                AND expected_commitment IS NULL
                AND refusal_basis IS NOT NULL AND btrim(refusal_basis) <> '')
        )
);

-- 责任起点唯一（AT-PS-049）：先合法形成者保留，并发第二个采用在这里撞墙。
CREATE UNIQUE INDEX intake_adoption_responsibility_start
    ON parcel_shipment.intake_adoption (tenant_id, parcel_id)
    WHERE adopted;

CREATE TABLE parcel_shipment.parcel_cancellation (
    tenant_id              text        NOT NULL,
    request_key            text        NOT NULL,
    parcel_id              text        NOT NULL,

    content_digest         text        NOT NULL,
    kind                   text        NOT NULL,

    cancellation_id        text,
    requester_ref          text,
    authority_ref          text,
    reason_ref             text,
    requested_at           timestamptz,
    crossed_intake_version text,
    refusal_basis          text,
    decided_at             timestamptz NOT NULL,

    CONSTRAINT parcel_cancellation_pkey
        PRIMARY KEY (tenant_id, request_key, parcel_id),

    CONSTRAINT parcel_cancellation_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(request_key) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT parcel_cancellation_kind_closed
        CHECK (kind IN ('PARCEL_CANCELLED', 'DISPOSITION_PENDING', 'CANCELLATION_REFUSED')),

    -- 三走向在场件矩阵：取消成立带决定五件；待处置只带越过的收寄版本（AT-PS-082，
    -- 处置决定属后续独立判断）；拒绝只带规则依据。
    CONSTRAINT parcel_cancellation_kind_shape
        CHECK (
            (kind = 'PARCEL_CANCELLED'
                AND cancellation_id IS NOT NULL AND btrim(cancellation_id) <> ''
                AND requester_ref IS NOT NULL AND btrim(requester_ref) <> ''
                AND authority_ref IS NOT NULL AND btrim(authority_ref) <> ''
                AND reason_ref IS NOT NULL AND btrim(reason_ref) <> ''
                AND requested_at IS NOT NULL
                AND crossed_intake_version IS NULL
                AND refusal_basis IS NULL)
            OR (kind = 'DISPOSITION_PENDING'
                AND crossed_intake_version IS NOT NULL AND btrim(crossed_intake_version) <> ''
                AND cancellation_id IS NULL
                AND requester_ref IS NULL
                AND authority_ref IS NULL
                AND reason_ref IS NULL
                AND requested_at IS NULL
                AND refusal_basis IS NULL)
            OR (kind = 'CANCELLATION_REFUSED'
                AND refusal_basis IS NOT NULL AND btrim(refusal_basis) <> ''
                AND cancellation_id IS NULL
                AND requester_ref IS NULL
                AND authority_ref IS NULL
                AND reason_ref IS NULL
                AND requested_at IS NULL
                AND crossed_intake_version IS NULL)
        )
);

-- 采用编排按包裹核对取消边界（ParcelCancellationView 的读口）。
CREATE INDEX parcel_cancellation_by_parcel
    ON parcel_shipment.parcel_cancellation (tenant_id, parcel_id, kind);

CREATE TABLE parcel_shipment.final_outcome (
    tenant_id           text        NOT NULL,
    parcel_id           text        NOT NULL,
    outcome_kind        text        NOT NULL,
    outcome_version     text        NOT NULL,

    content_digest      text        NOT NULL,
    finalized           boolean     NOT NULL,
    is_current          boolean     NOT NULL,

    final_version       text,
    final_kind          text,
    rule_version        text,
    decision_ref        text,
    execution_ref       text,
    occurred_at         timestamptz,
    prior_version       text,
    rederivation_reason text,
    refusal_basis       text,
    adopted_at          timestamptz NOT NULL,

    CONSTRAINT final_outcome_pkey
        PRIMARY KEY (tenant_id, parcel_id, outcome_kind, outcome_version),

    CONSTRAINT final_outcome_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(outcome_version) <> ''
            AND btrim(content_digest) <> ''
        ),

    -- 责任结果封闭四行（UC-PS-004 终局来源责任矩阵）：班次完成、POD 上传、异常案件
    -- 在库里也没有格。
    CONSTRAINT final_outcome_kind_closed
        CHECK (outcome_kind IN
            ('EFFECTIVE_DELIVERY', 'RETURN_COMPLETED',
             'SERVICE_TERMINATED', 'REGULATORY_DISPOSITION_EXECUTED')),

    -- 终局带全件、不采用带依据——两面互斥。
    CONSTRAINT final_outcome_result_shape
        CHECK (
            (finalized
                AND final_version IS NOT NULL AND btrim(final_version) <> ''
                AND final_kind IS NOT NULL AND btrim(final_kind) <> ''
                AND rule_version IS NOT NULL AND btrim(rule_version) <> ''
                AND decision_ref IS NOT NULL AND btrim(decision_ref) <> ''
                AND execution_ref IS NOT NULL AND btrim(execution_ref) <> ''
                AND occurred_at IS NOT NULL
                AND refusal_basis IS NULL)
            OR (NOT finalized
                AND final_version IS NULL
                AND final_kind IS NULL
                AND rule_version IS NULL
                AND decision_ref IS NULL
                AND execution_ref IS NULL
                AND occurred_at IS NULL
                AND prior_version IS NULL
                AND rederivation_reason IS NULL
                AND refusal_basis IS NOT NULL AND btrim(refusal_basis) <> '')
        ),

    -- 重派生痕迹两半互证：前版与原因同在或同缺、不得自指（AT-PS-063 的库面）。
    CONSTRAINT final_outcome_rederivation_coherent
        CHECK (
            (prior_version IS NULL AND rederivation_reason IS NULL)
            OR (prior_version IS NOT NULL AND btrim(prior_version) <> ''
                AND rederivation_reason IS NOT NULL AND btrim(rederivation_reason) <> ''
                AND final_version IS NOT NULL
                AND prior_version <> final_version)
        ),

    -- 不采用记录从不是当前终局。
    CONSTRAINT final_outcome_current_only_finalized
        CHECK (finalized OR NOT is_current)
);

-- 每包裹至多一行当前终局：重派生翻旧插新在这里保证不出现双当前。
CREATE UNIQUE INDEX final_outcome_current_unique
    ON parcel_shipment.final_outcome (tenant_id, parcel_id)
    WHERE is_current;
