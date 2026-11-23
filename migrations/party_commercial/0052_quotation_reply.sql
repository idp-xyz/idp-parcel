-- 报价答复。只增不改，没有状态列。一条发出至多一条。
--
-- 记下接受或拒绝、客户答复时刻、证据索引、代录人与登记时刻。文件字节不进这张表。
-- 过期与撤下不改已有行：那些是读的时候派生的。
--
-- 本迁移不回填，也不插默认行。

CREATE TABLE party_commercial.quotation_reply (
    tenant_id          text        NOT NULL,
    quotation_id       text        NOT NULL,
    decision           text        NOT NULL,
    replied_at         timestamptz NOT NULL,
    evidence_index     text        NOT NULL,
    rejection_reason   text        NOT NULL,
    recorded_by        text        NOT NULL,
    recorded_at        timestamptz NOT NULL,

    CONSTRAINT quotation_reply_pkey
        PRIMARY KEY (tenant_id, quotation_id),

    CONSTRAINT quotation_reply_send_fkey
        FOREIGN KEY (tenant_id, quotation_id)
        REFERENCES party_commercial.quotation_send (tenant_id, quotation_id),

    CONSTRAINT quotation_reply_decision_closed
        CHECK (decision IN ('ACCEPTED', 'REJECTED')),

    CONSTRAINT quotation_reply_evidence_present
        CHECK (
            btrim(evidence_index) <> ''
            AND evidence_index = btrim(evidence_index)
            AND evidence_index !~ '[[:cntrl:]]'
        ),

    CONSTRAINT quotation_reply_reason_shape
        CHECK (
            (decision = 'ACCEPTED' AND rejection_reason = '')
            OR (
                decision = 'REJECTED'
                AND rejection_reason = btrim(rejection_reason)
                AND rejection_reason !~ '[[:cntrl:]]'
            )
        ),

    CONSTRAINT quotation_reply_recorder_present
        CHECK (
            btrim(recorded_by) <> ''
            AND recorded_by = btrim(recorded_by)
            AND btrim(tenant_id) <> ''
            AND btrim(quotation_id) <> ''
        )
);
