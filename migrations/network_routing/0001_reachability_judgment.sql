-- 可达性判断记录：一次请求关联只有一个结果版本越过提交边界（AT-NR-028）。
--
-- 主键取（租户+请求关联）而不带任何范围维：范围一致与否是领域判断（重放/冲突
-- 分界在 SameJudgmentScope），库只负责「同一关联恰一条」；第二个写入方撞主键即
-- `已有记录`，由适配器译成写入代数，不覆盖先到者。
--
-- 候选与证据缺口是判断的组成部分而非独立对象，随判断整行保存（jsonb）；三值结论
-- 单列冗余存放，读回时经领域矩阵重算比对——一次坏写入在读回处暴露，而不是悄悄
-- 变成一个看起来合法的判断。

CREATE TABLE network_routing.reachability_judgment (
    tenant_id              text        NOT NULL,
    correlation_id         text        NOT NULL,

    customer_account_id    text        NOT NULL,
    shipment_request_id    text        NOT NULL,
    submission_version_id  text        NOT NULL,
    declared_parcel_id     text        NOT NULL,
    service_purpose        text        NOT NULL,
    as_of_semantic         text        NOT NULL,
    as_of_at               timestamptz NOT NULL,
    as_of_strategy_version text        NOT NULL,

    conclusion             text        NOT NULL,
    candidates             jsonb       NOT NULL,
    evidence_gaps          jsonb       NOT NULL,
    view_revision          text        NOT NULL,
    judged_at              timestamptz NOT NULL,
    recorded_at            timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT reachability_judgment_pkey
        PRIMARY KEY (tenant_id, correlation_id),

    CONSTRAINT reachability_judgment_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(correlation_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(submission_version_id) <> ''
            AND btrim(declared_parcel_id) <> ''
            AND btrim(service_purpose) <> ''
            AND btrim(as_of_semantic) <> ''
            AND btrim(as_of_strategy_version) <> ''
            AND btrim(view_revision) <> ''
        ),

    -- 三值封闭集在库里也封闭：`未形成判断`是应用结果，写进来就是写入方的缺陷。
    CONSTRAINT reachability_judgment_conclusion_closed
        CHECK (conclusion IN ('REACHABLE', 'UNREACHABLE', 'INSUFFICIENT_EVIDENCE')),

    -- 候选空间不成立则判断不存在（领域 ErrCandidateSpaceNotEstablished 的库面）。
    CONSTRAINT reachability_judgment_candidates_present
        CHECK (jsonb_typeof(candidates) = 'array' AND jsonb_array_length(candidates) > 0),
    CONSTRAINT reachability_judgment_gaps_shaped
        CHECK (jsonb_typeof(evidence_gaps) = 'array')
);
