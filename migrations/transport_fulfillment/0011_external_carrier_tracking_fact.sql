-- 外部承运轨迹事实与未收编素材留痕（label-channel/16，落文 ADR-0102）。
--
-- **主键带版本，写入只插不改。** 源声明的更正与本上下文的有效时间判断都形成新版本回指前版，
-- 原版本与原判断继续保留（CONTEXT「外部承运轨迹事实」生命周期节）。
--
-- **三个时间三列，归属不同，库面不许互相顶替。** occurred_at 由源给，NOT NULL——源未给发生时间
-- 的素材根本不进本表，它进下面的留痕表；received_at 由本上下文在接到素材时铸；effective_at 是
-- 本上下文的一次判断，待判断时为 NULL，且 NULL 与「判断过」由 effective_basis 分得开——不允许
-- 一行既说待判断又带着时间，也不允许一行说判过了却给不出时间。
--
-- **没有任何自营事实的列。** 它不是有效收寄、权威运输交接、实际移动或有效交付；外部 `DELIVERED`
-- 一类状态词原样存在 status_ref 里，不解释、不映射（ADR-0102 决定五）。
CREATE TABLE transport_fulfillment.external_carrier_tracking_fact (
    tenant_id              text        NOT NULL,
    fact_ref               text        NOT NULL,
    version                text        NOT NULL,

    source_ref             text        NOT NULL,
    credential_ref         text        NOT NULL,
    object_ref             text        NOT NULL,

    -- 源事件标识由源给，源未给即 NULL——不按内容摘要补一个（ADR-0102 决定四）。
    source_event           text,
    status_ref             text        NOT NULL,

    occurred_at            timestamptz NOT NULL,
    received_at            timestamptz NOT NULL,

    effective_basis        text        NOT NULL,
    effective_at           timestamptz,
    effective_rule         text,
    effective_rule_version text,

    -- 源显式声明「本条更正了哪条源事件」，原样带；本上下文解析到自己某一版后才有 supersedes_version。
    correction_of          text,
    supersedes_version     text,

    -- 这一版是素材到达形成的（首次认领或源声明的更正），还是本上下文的一次判断形成的。判断版本
    -- 原样携带源事件却不是一次新的到达，幂等锚因此只看 MATERIAL 那些行。
    version_origin         text        NOT NULL,

    recorded_at            timestamptz NOT NULL,

    CONSTRAINT external_carrier_tracking_fact_pkey
        PRIMARY KEY (tenant_id, fact_ref, version),

    CONSTRAINT external_carrier_tracking_fact_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_ref) <> ''
            AND btrim(version) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(credential_ref) <> ''
            AND btrim(object_ref) <> ''
            AND btrim(status_ref) <> ''
        ),

    CONSTRAINT external_carrier_tracking_fact_source_event_not_blank
        CHECK (source_event IS NULL OR btrim(source_event) <> ''),

    CONSTRAINT external_carrier_tracking_fact_effective_basis_closed
        CHECK (effective_basis IN ('PENDING', 'JUDGED_EXPLICITLY', 'JUDGED_BY_RULE')),

    -- 待判断 ⇔ 没有有效时间。两边都守：默认「等于发生时间」在库面就写不进去。
    CONSTRAINT external_carrier_tracking_fact_effective_time_matches_basis
        CHECK ((effective_basis = 'PENDING') = (effective_at IS NULL)),

    -- 按规则判断 ⇔ 记下了规则与版本。
    CONSTRAINT external_carrier_tracking_fact_rule_matches_basis
        CHECK (
            (effective_basis = 'JUDGED_BY_RULE')
            = (effective_rule IS NOT NULL AND effective_rule_version IS NOT NULL)
        ),

    CONSTRAINT external_carrier_tracking_fact_rule_not_blank
        CHECK (
            (effective_rule IS NULL OR btrim(effective_rule) <> '')
            AND (effective_rule_version IS NULL OR btrim(effective_rule_version) <> '')
        ),

    CONSTRAINT external_carrier_tracking_fact_correction_of_not_blank
        CHECK (correction_of IS NULL OR btrim(correction_of) <> ''),

    CONSTRAINT external_carrier_tracking_fact_supersedes_not_blank
        CHECK (supersedes_version IS NULL OR btrim(supersedes_version) <> ''),

    -- 前版引用指向自己就是一条读不动的链。
    CONSTRAINT external_carrier_tracking_fact_supersedes_not_self
        CHECK (supersedes_version IS NULL OR supersedes_version <> version),

    CONSTRAINT external_carrier_tracking_fact_version_origin_closed
        CHECK (version_origin IN ('MATERIAL', 'JUDGMENT')),

    -- 素材版本回指前版只有一条来路：解析了源声明的更正。没有声明的回指等于本上下文自己推断了
    -- 取代关系——CONTEXT 明禁。
    CONSTRAINT external_carrier_tracking_fact_material_supersedes_needs_a_declaration
        CHECK (
            version_origin <> 'MATERIAL'
            OR supersedes_version IS NULL
            OR correction_of IS NOT NULL
        ),

    -- 判断版本必定回指被判断的那一版，且必定判断过——「判断为待判断」不是判断。
    CONSTRAINT external_carrier_tracking_fact_judgment_supersedes_and_is_judged
        CHECK (
            version_origin <> 'JUDGMENT'
            OR (supersedes_version IS NOT NULL AND effective_basis <> 'PENDING')
        )
);

-- 接收形态的幂等锚：同一（轨迹源，源事件标识）只成一条**素材版本**。部分索引——源未给事件标识
-- 的素材不判重（ADR-0090 决定四：锚在已有身份上，不新造传输层的键）；判断版本原样携带源事件，
-- 但它不是一次新的到达，不进锚。
CREATE UNIQUE INDEX external_carrier_tracking_fact_source_event_key
    ON transport_fulfillment.external_carrier_tracking_fact (tenant_id, source_ref, source_event)
    WHERE source_event IS NOT NULL AND version_origin = 'MATERIAL';

-- 没有成为事实的素材的留痕。只追加。本体不存，只留摘要——「我们确实收到过这条」的全部证据。
-- 它不是拒收：所有者随时能看见「这家源给了多少条没有时间的状态」（ADR-0102 Consequences）。
-- 主键由库自己编号：留痕是本上下文自己的证据流水，不是任何人的事实身份。
CREATE TABLE transport_fulfillment.unadopted_tracking_material (
    entry          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id      text        NOT NULL,
    source_ref     text        NOT NULL,
    source_event   text,
    credential_ref text        NOT NULL,
    status_ref     text        NOT NULL,
    received_at    timestamptz NOT NULL,
    payload_digest text        NOT NULL,
    reason         text        NOT NULL,
    recorded_at    timestamptz NOT NULL,

    CONSTRAINT unadopted_tracking_material_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(credential_ref) <> ''
            AND btrim(status_ref) <> ''
            AND btrim(payload_digest) <> ''
        ),

    CONSTRAINT unadopted_tracking_material_source_event_not_blank
        CHECK (source_event IS NULL OR btrim(source_event) <> ''),

    CONSTRAINT unadopted_tracking_material_reason_closed
        CHECK (reason IN ('OCCURRED_AT_NOT_GIVEN_BY_SOURCE', 'CREDENTIAL_UNKNOWN'))
);

-- 本上下文自己的事实身份与版本各一条序列。号只为可读，不承载语义：不得可解析出租户、源或
-- 源事件——那些维度已经在行上，写进号里就成了第二处定义。
CREATE SEQUENCE transport_fulfillment.external_tracking_fact_ref_seq;
CREATE SEQUENCE transport_fulfillment.external_tracking_fact_version_seq;
