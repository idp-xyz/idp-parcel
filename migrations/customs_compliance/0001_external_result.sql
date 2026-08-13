-- 外部监管结果登记册：同一来源响应身份（租户+来源标识）只有一份接收记录越过提交
-- 边界；同键第二份由主键拦住，适配器以 ON CONFLICT DO NOTHING 译成`已有记录`，
-- 同键异内容的冲突分界靠内容指纹列——两者都不覆盖先到者。
--
-- 归属不上的响应以 unattributable 留存原始语义与声称版本（留存不猜，CONTEXT 187）：
-- 此时没有可供判断消费的监管事实，result 为 NULL；这条形状规则由 CHECK 在库里再守
-- 一遍。submission_version 单列冗余存放（可归属时必非空），供同层一致性比对按
-- （租户+提交版本）读回各层事实。

CREATE TABLE customs_compliance.external_result (
    tenant_id          text        NOT NULL,
    source_id          text        NOT NULL,

    content_digest     text        NOT NULL,
    unattributable     boolean     NOT NULL,
    raw_semantics      text        NOT NULL DEFAULT '',
    claimed_version    text        NOT NULL DEFAULT '',
    layer_conflict     boolean     NOT NULL,
    result             jsonb,
    submission_version text,
    recorded_at        timestamptz NOT NULL,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT external_result_pkey
        PRIMARY KEY (tenant_id, source_id),

    CONSTRAINT external_result_scope_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(source_id) <> ''),
    CONSTRAINT external_result_digest_not_blank
        CHECK (btrim(content_digest) <> ''),

    -- 留存不猜的形状：归属不上即无监管事实，可归属即事实与提交版本都在场。
    CONSTRAINT external_result_attribution_shape
        CHECK (
            (unattributable AND result IS NULL AND submission_version IS NULL)
            OR (NOT unattributable AND result IS NOT NULL AND btrim(coalesce(submission_version, '')) <> '')
        )
);

-- 同层一致性比对按（租户+提交版本）读回全部已保存事实，顺序稳定。
CREATE INDEX external_result_by_submission
    ON customs_compliance.external_result (tenant_id, submission_version, inserted_at)
    WHERE submission_version IS NOT NULL;
