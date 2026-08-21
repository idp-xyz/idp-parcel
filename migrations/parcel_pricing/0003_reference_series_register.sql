-- 计价参考序列登记册：燃油费率与汇率等外部数值序列的版本登记（ADR-0013：计价拥有
-- 登记、版本化、发布治理与按计价基准时点的解析，但不生产数值、不选定商业口径）。
--
-- 来源：SYN-WALL-DOOR-AUDIT 票 08——REFERENCE_SERIES_UNRESOLVED / EXCHANGE_RATE_UNRESOLVED
-- 的墙是对的，缺的是门。本迁移只建登记结构，不种任何期次取值：序列实例全属实例半边
-- （PAR-SET-11 待提供），机制先行，隔离取值不得冒充生产序列（PN-07 准入门槛）。
--
-- 键 =（租户 + 序列 + 序列版本）。行只增不改：取值发现错误时形成新序列版本并声明与
-- 原版本的更正关系（prior_version + correction_basis 成对，先例：supplier_expected_cost
-- 的纠错成对约束），既有评价保留原取值不被追溯改写。
--
-- 登记一期取值是一次来源事实断言（CONTEXT）：来源标识、登记责任方必备；每期凭证在
-- 快照内逐期携带，evidence_grade 列汇总整版等级——任何一期缺可复核凭证，整版只有
-- ASSERTED（断言强度，只准隔离验证，不得支撑生产金额）。
--
-- 列面只是比对与检索；权威内容在 snapshot——领域折装的登记快照（含逐期取值、凭证
-- 引用与自校内容摘要），读回经领域整版重验。摘要只在同一规范化形状（PRS 族）内可比，
-- 与价卡的 PPC 族同一条 ADR-0014 纪律、各自演进。

CREATE TABLE parcel_pricing.reference_series_version (
    tenant_id           text        NOT NULL,
    series_id           text        NOT NULL,
    series_version      text        NOT NULL,

    kind                text        NOT NULL,
    source_identifier   text        NOT NULL,
    registrant          text        NOT NULL,

    -- 汇率口径由 party-commercial 的商业价格政策版本化声明；不接受未声明口径的
    -- 裸汇率。燃油的折扣系数写在卡上，无需口径。
    quote_basis_id      text,
    quote_basis_version text,

    -- 期次包络 [effective_from, effective_to)，effective_to 为 NULL 表示末期无上界；
    -- 逐期取值在快照内。
    effective_from      timestamptz NOT NULL,
    effective_to        timestamptz,

    evidence_grade      text        NOT NULL,

    prior_version       text,
    correction_basis    text,

    canonicalization    text        NOT NULL,
    content_digest      text        NOT NULL,
    snapshot            jsonb       NOT NULL,
    registered_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT reference_series_version_pkey
        PRIMARY KEY (tenant_id, series_id, series_version),

    CONSTRAINT reference_series_version_kind_closed
        CHECK (kind IN ('FUEL_RATE', 'EXCHANGE_RATE')),

    CONSTRAINT reference_series_version_grade_closed
        CHECK (evidence_grade IN ('VERIFIABLE', 'ASSERTED')),

    -- 口径引用两列成对：只有 ID 或只有版本的引用指不到任何政策。
    CONSTRAINT reference_series_version_quote_basis_paired
        CHECK (
            (quote_basis_id IS NULL AND quote_basis_version IS NULL)
            OR (quote_basis_id IS NOT NULL AND btrim(quote_basis_id) <> ''
                AND quote_basis_version IS NOT NULL AND btrim(quote_basis_version) <> '')
        ),

    CONSTRAINT reference_series_version_fx_demands_quote_basis
        CHECK (kind <> 'EXCHANGE_RATE' OR quote_basis_id IS NOT NULL),

    CONSTRAINT reference_series_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from),

    -- 更正两件成对且不自指：只带回指或只带依据的行说不清它更正的是哪一版。
    CONSTRAINT reference_series_version_correction_paired
        CHECK (
            (prior_version IS NULL AND correction_basis IS NULL)
            OR (prior_version IS NOT NULL AND correction_basis IS NOT NULL
                AND btrim(prior_version) <> ''
                AND btrim(correction_basis) <> ''
                AND prior_version <> series_version)
        ),

    CONSTRAINT reference_series_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(series_id) <> ''
            AND btrim(series_version) <> ''
            AND btrim(source_identifier) <> ''
            AND btrim(registrant) <> ''
            AND btrim(canonicalization) <> ''
            AND btrim(content_digest) <> ''
        )
);
