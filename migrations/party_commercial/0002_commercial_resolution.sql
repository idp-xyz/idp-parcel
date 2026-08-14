-- 已固定的商业解析：按（租户+解析标识）回指一次不可覆盖的闭包（ADR-0027）。
--
-- 本票只收唯一已解析。其它结局没有解析标识可回指，不入本表。正文进 jsonb，
-- 内容摘要平铺供撞键后判重放还是冲突；读回逐字段过领域重建门。

CREATE TABLE party_commercial.commercial_resolution (
    tenant_id            text        NOT NULL,
    resolution_id        text        NOT NULL,
    customer_account_id  text        NOT NULL,
    outcome              smallint    NOT NULL,
    content_digest       text        NOT NULL,
    snapshot             jsonb       NOT NULL,
    recorded_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commercial_resolution_pkey
        PRIMARY KEY (tenant_id, resolution_id),

    CONSTRAINT commercial_resolution_identity_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(resolution_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(content_digest) <> ''
        ),
    -- 1 = UNIQUELY_RESOLVED。其它结局没有可回指的标识。
    CONSTRAINT commercial_resolution_unique_only
        CHECK (outcome = 1),
    -- jsonb 三值缝：`jsonb_typeof(NULL)` 是 NULL，CHECK 把 NULL 当通过。
    CONSTRAINT commercial_resolution_snapshot_object
        CHECK (snapshot IS NOT NULL AND jsonb_typeof(snapshot) = 'object')
);
