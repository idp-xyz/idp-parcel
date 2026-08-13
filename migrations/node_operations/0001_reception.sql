-- 收寄判断记录：同一来源身份只有一份处理结果越过提交边界（幂等键=租户+来源标识）。
--
-- 同键第二份写入由主键拦住，适配器以 ON CONFLICT DO NOTHING 译成`已有记录`——重复
-- 回传按已有结果作答，同键异内容的冲突分界靠内容指纹列，两者都不覆盖先到者。
--
-- 收寄与控制只在`收寄已形成`/`待识别`两格在场（结果语义契约），这条形状规则在库里
-- 也成立：其余两格写进非空 intake/control 即写入方缺陷，读回前就拦下。

CREATE TABLE node_operations.reception (
    tenant_id         text        NOT NULL,
    source_id         text        NOT NULL,

    content_digest    text        NOT NULL,
    kind              text        NOT NULL,
    intake            jsonb,
    control           jsonb,
    candidates        jsonb       NOT NULL,
    identity_conflict boolean     NOT NULL,
    refusal_reason    text        NOT NULL DEFAULT '',
    service_markers   jsonb       NOT NULL,
    recorded_at       timestamptz NOT NULL,
    inserted_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT reception_pkey
        PRIMARY KEY (tenant_id, source_id),

    CONSTRAINT reception_scope_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(source_id) <> ''),
    CONSTRAINT reception_digest_not_blank
        CHECK (btrim(content_digest) <> ''),

    CONSTRAINT reception_kind_closed
        CHECK (kind IN ('INTAKE_FORMED', 'PENDING_IDENTIFICATION', 'INTAKE_NOT_FORMED', 'RECEPTION_UNDECIDED')),

    -- 两格在场规则：形成/待识别必带收寄与控制，另两格必不带。
    CONSTRAINT reception_intake_presence
        CHECK (
            (kind IN ('INTAKE_FORMED', 'PENDING_IDENTIFICATION'))
                = (intake IS NOT NULL AND control IS NOT NULL)
        ),

    CONSTRAINT reception_lists_shaped
        CHECK (jsonb_typeof(candidates) = 'array' AND jsonb_typeof(service_markers) = 'array')
);
