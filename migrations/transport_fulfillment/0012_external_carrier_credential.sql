-- 外部承运凭证登记册（label-channel/18）。
--
-- CONTEXT「外部承运凭证」：外部运输服务提供方为运输委托、订舱、班次、载运对象或实际履约段分配的
-- 业务凭证，必须保存分配方、真实标识对象、适用范围和版本，不能全部解释为包裹的当前运单号。
-- 规则节：作废、失效或替代只改变其适用关系，不得删除历史凭证。
--
-- **主键带版本，写入只插不改。** 作废、失效、替代各形成一个新版本回指前版并落定适用终点；原版本
-- 一字不动。「当前版」按回指派生（没有任何行回指它），表上没有 current 列。
--
-- **真实标识对象是登记出来的两列（类别 + 引用），不从 credential_ref 的格式推断。** 类别封闭为
-- CONTEXT 词条那五类；只有 CARRIED_OBJECT 那一类才会被凭证解析口交回给收编执行器。
--
-- **本表不存任何承运商的凭证格式或校验规则**——那是实例半边，留空。
CREATE TABLE transport_fulfillment.external_carrier_credential (
    tenant_id              text        NOT NULL,
    credential_ref         text        NOT NULL,
    version                text        NOT NULL,

    assigner_ref           text        NOT NULL,
    identified_kind        text        NOT NULL,
    identified_ref         text        NOT NULL,

    -- 适用范围：左闭右开的业务时间区间，终点开放时为 NULL。
    effective_from         timestamptz NOT NULL,
    effective_until        timestamptz,

    standing               text        NOT NULL,
    -- 改变适用关系的业务时间；首版没有。它与 recorded_at（登记落库时刻）是两个时间。
    changed_at             timestamptz,
    supersedes_version     text,
    replaced_by_credential text,

    recorded_at            timestamptz NOT NULL,

    CONSTRAINT external_carrier_credential_pkey
        PRIMARY KEY (tenant_id, credential_ref, version),

    CONSTRAINT external_carrier_credential_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(credential_ref) <> ''
            AND btrim(version) <> ''
            AND btrim(assigner_ref) <> ''
            AND btrim(identified_ref) <> ''
        ),

    CONSTRAINT external_carrier_credential_identified_kind_closed
        CHECK (identified_kind IN (
            'TRANSPORT_COMMISSION', 'BOOKING', 'TRANSPORT_SCHEDULE', 'CARRIED_OBJECT', 'FULFILLMENT_SEGMENT'
        )),

    CONSTRAINT external_carrier_credential_standing_closed
        CHECK (standing IN ('APPLICABLE', 'REVOKED', 'EXPIRED', 'SUPERSEDED')),

    CONSTRAINT external_carrier_credential_range_ordered
        CHECK (effective_until IS NULL OR effective_until > effective_from),

    -- 适用中的版本没有「改变」可记：不回指、无改变时间、无替代者。
    CONSTRAINT external_carrier_credential_applicable_has_no_change
        CHECK (
            standing <> 'APPLICABLE'
            OR (changed_at IS NULL AND supersedes_version IS NULL AND replaced_by_credential IS NULL)
        ),

    -- 作废、失效、替代都是一次改变：必有业务时间、必回指前版、必有落定的终点。
    CONSTRAINT external_carrier_credential_change_is_complete
        CHECK (
            standing = 'APPLICABLE'
            OR (changed_at IS NOT NULL AND supersedes_version IS NOT NULL AND effective_until IS NOT NULL)
        ),

    -- 替代者只在已替代时出现，且已替代必有替代者。
    CONSTRAINT external_carrier_credential_replacement_matches_standing
        CHECK ((standing = 'SUPERSEDED') = (replaced_by_credential IS NOT NULL)),

    CONSTRAINT external_carrier_credential_replacement_not_self
        CHECK (replaced_by_credential IS NULL OR replaced_by_credential <> credential_ref),

    CONSTRAINT external_carrier_credential_supersedes_not_blank
        CHECK (supersedes_version IS NULL OR btrim(supersedes_version) <> ''),

    CONSTRAINT external_carrier_credential_replacement_not_blank
        CHECK (replaced_by_credential IS NULL OR btrim(replaced_by_credential) <> ''),

    -- 前版引用指向自己就是一条读不动的链。
    CONSTRAINT external_carrier_credential_supersedes_not_self
        CHECK (supersedes_version IS NULL OR supersedes_version <> version)
);

-- 凭证解析口按（租户，凭证）取当前版；版本表按登记先后列全部版本。
CREATE INDEX external_carrier_credential_by_credential
    ON transport_fulfillment.external_carrier_credential (tenant_id, credential_ref, recorded_at);
