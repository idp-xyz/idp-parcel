-- 计价参考目录登记册：承运商分区表与偏远档位表等外部分类事实的版本登记（ADR-0109：计价拥有
-- 登记、版本化与按计价基准时点的解析，不生产任何一行映射）。
--
-- 来源：票 price-card-shape-gaps/01——评价输入里的分区与偏远档位此前只有消费侧、没有提供侧。
-- 本迁移只建登记结构，不种任何邮编行：目录实例全属实例半边，机制先行，SYN 夹具只记 S。
--
-- 键 =（租户 + 目录 + 目录版本）。行只增不改：映射发现错误时形成新目录版本并声明与原版本的更正
-- 关系（prior_version + correction_basis 成对，先例：reference_series_version），既有评价保留原读数
-- 不被追溯改写。
--
-- 列面只是比对与检索；权威内容在 snapshot——领域折装的登记快照（含始发维度、逐条前缀映射与自校
-- 内容摘要），读回经领域整版重验。摘要只在同一规范化形状（PRC 族）内可比，与序列的 PRS 族、价卡
-- 的 PPC 族同一条 ADR-0014 纪律、各自演进。
--
-- 前缀粒度（prefix_length）是这一版自己声明的实例值（CONTEXT「随首份真实分区表声明，不预拟」）；
-- 库层只守它为正，不钉任何具体位数。

CREATE TABLE parcel_pricing.reference_catalogue_version (
    tenant_id           text        NOT NULL,
    catalogue_id        text        NOT NULL,
    catalogue_version   text        NOT NULL,

    kind                text        NOT NULL,
    source_identifier   text        NOT NULL,
    registrant          text        NOT NULL,

    origin_scope        text        NOT NULL,
    prefix_length       integer     NOT NULL,
    entry_count         integer     NOT NULL,

    -- 生效区间 [effective_from, effective_to)，effective_to 为 NULL 表示无上界。
    effective_from      timestamptz NOT NULL,
    effective_to        timestamptz,

    prior_version       text,
    correction_basis    text,

    canonicalization    text        NOT NULL,
    content_digest      text        NOT NULL,
    snapshot            jsonb       NOT NULL,
    registered_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT reference_catalogue_version_pkey
        PRIMARY KEY (tenant_id, catalogue_id, catalogue_version),

    CONSTRAINT reference_catalogue_version_kind_closed
        CHECK (kind IN ('ZONE', 'REMOTE_TIER')),

    CONSTRAINT reference_catalogue_version_origin_scope_closed
        CHECK (origin_scope IN ('INDEPENDENT', 'POSTAL_PREFIXES')),

    CONSTRAINT reference_catalogue_version_prefix_length_positive
        CHECK (prefix_length > 0),

    CONSTRAINT reference_catalogue_version_has_entries
        CHECK (entry_count > 0),

    CONSTRAINT reference_catalogue_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from),

    -- 更正两件成对且不自指：只带回指或只带依据的行说不清它更正的是哪一版。
    CONSTRAINT reference_catalogue_version_correction_paired
        CHECK (
            (prior_version IS NULL AND correction_basis IS NULL)
            OR (prior_version IS NOT NULL AND correction_basis IS NOT NULL
                AND btrim(prior_version) <> ''
                AND btrim(correction_basis) <> ''
                AND prior_version <> catalogue_version)
        ),

    CONSTRAINT reference_catalogue_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(catalogue_id) <> ''
            AND btrim(catalogue_version) <> ''
            AND btrim(source_identifier) <> ''
            AND btrim(registrant) <> ''
            AND btrim(canonicalization) <> ''
            AND btrim(content_digest) <> ''
        )
);
