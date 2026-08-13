-- 申报链：提交版本与发送尝试。
--
-- declaration_submission：键=逻辑申报目标（租户+申报单元+监管程序）——同一目标重复
-- 提交返回原版本不重复形成；版本是首次实际发送前固定的不可覆盖快照（CONTEXT 硬句
-- 168），行内无任何可回写列，适配器没有 UPDATE 语句。版本标识另设唯一约束供尝试表
-- 指回。组成快照存 jsonb 数组——成员只在版本固定那一刻有意义，无按成员检索的读面。
--
-- submission_attempt：针对明确提交版本实际发起的对外发送，键=（租户+版本+序号），
-- 序号递增由主键承担（同序第二次写被拦）。外键指回版本表正是硬句 168 可钉的那半：
-- 尝试不可能先于版本存在。首次尝试没有安全再次发送判断、受控重发必须携带（硬句
-- 170）——形状入 CHECK。

CREATE TABLE customs_compliance.declaration_submission (
    tenant_id       text        NOT NULL,
    unit_id         text        NOT NULL,
    procedure_ref   text        NOT NULL,

    version_id      text        NOT NULL,
    content_digest  text        NOT NULL,
    members         jsonb       NOT NULL,
    dossier_ref     text        NOT NULL,
    roles_ref       text        NOT NULL,
    readiness_basis text        NOT NULL,
    authority_ref   text        NOT NULL,
    fixed_at        timestamptz NOT NULL,
    recorded_at     timestamptz NOT NULL,

    CONSTRAINT declaration_submission_pkey
        PRIMARY KEY (tenant_id, unit_id, procedure_ref),
    CONSTRAINT declaration_submission_version_unique
        UNIQUE (tenant_id, version_id),

    CONSTRAINT declaration_submission_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(unit_id) <> ''
            AND btrim(procedure_ref) <> ''
            AND btrim(version_id) <> ''
            AND btrim(content_digest) <> ''
            AND btrim(dossier_ref) <> ''
            AND btrim(roles_ref) <> ''
            AND btrim(readiness_basis) <> ''
            AND btrim(authority_ref) <> ''
        ),
    CONSTRAINT declaration_submission_members_present
        CHECK (jsonb_typeof(members) = 'array' AND jsonb_array_length(members) >= 1)
);

CREATE TABLE customs_compliance.submission_attempt (
    tenant_id       text        NOT NULL,
    version_id      text        NOT NULL,
    sequence        integer     NOT NULL,

    target          text        NOT NULL,
    result          text        NOT NULL,
    safe_resend_ref text,
    sent_at         timestamptz NOT NULL,

    CONSTRAINT submission_attempt_pkey
        PRIMARY KEY (tenant_id, version_id, sequence),
    CONSTRAINT submission_attempt_version_exists
        FOREIGN KEY (tenant_id, version_id)
        REFERENCES customs_compliance.declaration_submission (tenant_id, version_id),

    CONSTRAINT submission_attempt_not_blank
        CHECK (btrim(target) <> '' AND btrim(result) <> ''),
    CONSTRAINT submission_attempt_sequence_positive
        CHECK (sequence >= 1),
    -- 结果封闭三值：结果未知保持待确认，超时不得直接解释为失败。
    CONSTRAINT submission_attempt_result_closed
        CHECK (result IN ('ACKNOWLEDGED', 'FAILED', 'PENDING_CONFIRMATION')),
    -- 首次尝试随版本形成、没有安全再次发送判断；同版本再次尝试必须携带（硬句 170）。
    CONSTRAINT submission_attempt_resend_shape
        CHECK (
            ((sequence = 1) = (safe_resend_ref IS NULL))
            AND (safe_resend_ref IS NULL OR btrim(safe_resend_ref) <> '')
        )
);
