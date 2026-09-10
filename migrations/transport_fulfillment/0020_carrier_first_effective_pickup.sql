-- 实际承运商首次有效收寄（label-channel/31，ADR-0135）。
--
-- CONTEXT「实际承运商首次有效收寄」：本上下文就一个载运对象首次进入某实际承运商运输控制所形成的独立控制事实，在
-- 合格来源之上经一次显式判断形成，带版本；已形成即参与起点；待确认是带原因的版本但不构成收寄、不进段、不提供；
-- 依据的来源事实被更正时沿更正关系形成替代或失效版本，原版本不改写。
--
-- 两张表：版本表一行一版本，依据表一行一条依据（一版至少一条，多条是来源冲突把全部依据并进来）。**只插不改**：没有
-- current 列，链尾按「未被任何版本回指」派生（与 external_carrier_tracking_fact 同一条纪律）。一对象至多一条链由
-- 首登（不回指）上的部分唯一索引表达；链线性（一版只被回指一次）由回指列上的部分唯一索引表达。
--
-- 逐格完备性在库内再守一遍：领域构造门与重建门是第一道，CHECK 是第二道，两道互补不互替（0001 立的纪律）。
-- 全部 CHECK 过 SQL 三值逻辑那一眼：可空列使用前先 IS NULL / IS NOT NULL。
CREATE TABLE transport_fulfillment.carrier_first_effective_pickup (
    tenant_id          text        NOT NULL,
    fact_ref           text        NOT NULL,
    version            text        NOT NULL,
    object_ref         text        NOT NULL,

    -- 结果封闭三格；已形成恰带承运主体与业务发生时间，待确认恰带原因与名称素材，失效三者都不带且必回指前版。
    result             text        NOT NULL,
    carrier_kind       text,
    carrier_ref        text,
    occurred_at        timestamptz,
    pending_reason     text,
    claimed_material   text,

    -- 判断形成时间由本上下文记，与业务发生时间分列（ADR-0135 决定三）；登记时刻是登记册的事实，第三个时间。
    judged_at          timestamptz NOT NULL,
    supersedes_version text,
    recorded_at        timestamptz NOT NULL,

    CONSTRAINT carrier_first_effective_pickup_pkey
        PRIMARY KEY (tenant_id, fact_ref, version),

    CONSTRAINT carrier_first_effective_pickup_scope_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(fact_ref) <> '' AND btrim(version) <> '' AND btrim(object_ref) <> ''),

    CONSTRAINT carrier_first_effective_pickup_result_closed
        CHECK (result IN ('FORMED', 'PENDING', 'VOIDED')),

    -- 已形成 ⇔ 承运主体两列与业务发生时间**各自**在场——三列分写，否则「待确认却带承运主体、缺业务时间」
    -- 的行会从合取的反面溜过去。
    CONSTRAINT carrier_first_effective_pickup_formed_has_carrier_kind
        CHECK ((result = 'FORMED') = (carrier_kind IS NOT NULL)),
    CONSTRAINT carrier_first_effective_pickup_formed_has_carrier_ref
        CHECK ((result = 'FORMED') = (carrier_ref IS NOT NULL)),
    CONSTRAINT carrier_first_effective_pickup_formed_has_occurred_at
        CHECK ((result = 'FORMED') = (occurred_at IS NOT NULL)),

    -- 待确认 ⇔ 原因与名称素材各自在场（不据名称铸身份，名称作为素材随依据保留）。
    CONSTRAINT carrier_first_effective_pickup_pending_has_reason
        CHECK ((result = 'PENDING') = (pending_reason IS NOT NULL)),
    CONSTRAINT carrier_first_effective_pickup_pending_has_material
        CHECK ((result = 'PENDING') = (claimed_material IS NOT NULL)),

    -- 失效必回指前版：首登不能失效，从未形成就没有东西可失效。
    CONSTRAINT carrier_first_effective_pickup_voided_chains
        CHECK (result <> 'VOIDED' OR supersedes_version IS NOT NULL),

    CONSTRAINT carrier_first_effective_pickup_carrier_kind_closed
        CHECK (carrier_kind IS NULL OR carrier_kind IN ('EXTERNAL_PARTY', 'OPERATING_LEGAL_ENTITY')),

    CONSTRAINT carrier_first_effective_pickup_carrier_ref_not_blank
        CHECK (carrier_ref IS NULL OR btrim(carrier_ref) <> ''),

    -- 「无合格证据」不是收寄的待确认原因（ADR-0135 决定四）。
    CONSTRAINT carrier_first_effective_pickup_pending_reason_closed
        CHECK (pending_reason IS NULL OR pending_reason IN ('SOURCE_CONFLICT', 'IDENTITY_NOT_REGISTERED')),

    CONSTRAINT carrier_first_effective_pickup_material_not_blank
        CHECK (claimed_material IS NULL OR btrim(claimed_material) <> ''),

    CONSTRAINT carrier_first_effective_pickup_supersedes_not_blank
        CHECK (supersedes_version IS NULL OR btrim(supersedes_version) <> ''),

    -- 回指不自指。
    CONSTRAINT carrier_first_effective_pickup_supersedes_not_self
        CHECK (supersedes_version IS NULL OR supersedes_version <> version)
);

-- 一个载运对象至多一条链：首登（不回指）在（租户，对象）上唯一。
CREATE UNIQUE INDEX carrier_first_effective_pickup_one_chain_per_object
    ON transport_fulfillment.carrier_first_effective_pickup (tenant_id, object_ref)
    WHERE supersedes_version IS NULL;

-- 链线性：同一事实的一版最多被回指一次。
CREATE UNIQUE INDEX carrier_first_effective_pickup_superseded_once
    ON transport_fulfillment.carrier_first_effective_pickup (tenant_id, fact_ref, supersedes_version)
    WHERE supersedes_version IS NOT NULL;

-- 按对象取链（链尾与整链）。
CREATE INDEX carrier_first_effective_pickup_by_object
    ON transport_fulfillment.carrier_first_effective_pickup (tenant_id, object_ref, judged_at);

-- 依据：来源种类（封闭四格，同实际承运商判断）、来源事实引用与**来源事实的版本**——依据是「F 的第 v1 代」而不是
-- 「F」，更正才有办法回指到被更正的那一代（ADR-0135 决定二）。
CREATE TABLE transport_fulfillment.carrier_first_effective_pickup_basis (
    tenant_id       text    NOT NULL,
    fact_ref        text    NOT NULL,
    version         text    NOT NULL,
    ordinal         integer NOT NULL,
    source          text    NOT NULL,
    evidence_ref    text    NOT NULL,
    source_version  text    NOT NULL,

    CONSTRAINT carrier_first_effective_pickup_basis_pkey
        PRIMARY KEY (tenant_id, fact_ref, version, ordinal),

    CONSTRAINT carrier_first_effective_pickup_basis_version_fkey
        FOREIGN KEY (tenant_id, fact_ref, version)
        REFERENCES transport_fulfillment.carrier_first_effective_pickup (tenant_id, fact_ref, version),

    CONSTRAINT carrier_first_effective_pickup_basis_ordinal_positive
        CHECK (ordinal >= 1),

    CONSTRAINT carrier_first_effective_pickup_basis_source_closed
        CHECK (source IN ('CARRIER_PICKUP_SCAN', 'CARRIER_RECEIPT_VOUCHER', 'TRANSPORT_HANDOVER_TO_CARRIER', 'TRUSTED_CHANNEL_CALLBACK')),

    CONSTRAINT carrier_first_effective_pickup_basis_not_blank
        CHECK (btrim(evidence_ref) <> '' AND btrim(source_version) <> ''),

    -- 同一版里同一（引用，来源版本）不出现两次。
    CONSTRAINT carrier_first_effective_pickup_basis_unique_per_version
        UNIQUE (tenant_id, fact_ref, version, evidence_ref, source_version)
);

-- 事实身份与版本各一条序列；版本号不可解析出租户、对象或事实（那些维度已在键上）。
CREATE SEQUENCE transport_fulfillment.carrier_first_effective_pickup_ref_seq;
CREATE SEQUENCE transport_fulfillment.carrier_first_effective_pickup_version_seq;

-- 参与起点封闭集长第三格（ADR-0135 决定五）：已形成的首次有效收寄是本上下文判过「取得运输控制」的控制事实，不是扫描。
ALTER TABLE transport_fulfillment.fulfillment_participation
    DROP CONSTRAINT fulfillment_participation_entry_kind_closed;
ALTER TABLE transport_fulfillment.fulfillment_participation
    ADD CONSTRAINT fulfillment_participation_entry_kind_closed
        CHECK (entry_kind IN ('OFFSITE_PICKUP', 'TRANSPORT_HANDOVER', 'CARRIER_FIRST_EFFECTIVE_PICKUP'));

-- 失效版本也从收寄链的失效版本长出来（ADR-0135 决定六），与交接更正并列；揽收更正仍走不到这一格。
ALTER TABLE transport_fulfillment.fulfillment_participation
    DROP CONSTRAINT fulfillment_participation_voided_coherent;
ALTER TABLE transport_fulfillment.fulfillment_participation
    ADD CONSTRAINT fulfillment_participation_voided_coherent
        CHECK (
            NOT voided
            OR (
                supersedes_entry_basis IS NOT NULL
                AND entry_kind IN ('TRANSPORT_HANDOVER', 'CARRIER_FIRST_EFFECTIVE_PICKUP')
            )
        );
