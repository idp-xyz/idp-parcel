-- 外部资金事实采用加版本维：一版本一行（票 sa-cc/20 裁决 1）。
--
-- UC-SA-001「更正必须形成新来源版本」与 SA CONTEXT「它不修改外部资金事实」要的是更正版本进得来、原版本
-- 留得住；0004 的 external_funds_fact 主键 (tenant_id, fact_id) 一事实一行，更正版本（同事实、新版本、回指
-- 前版）写不进去——领域 CorrectAmount 形成的新版本在本上下文没有落点，sa-cc/02 裁决 2「更正再发一封」也就
-- 没有能产生第二封的写路径。
--
-- 形取**版本子表**而不是主键加版本，与 customs_compliance/0021 同形但理由不同：本迁移之前全部 SA 迁移没有
-- 一道 REFERENCES，CC 那条「保外键」的理由在这里不成立；成立的是两条——
-- (i) funds_mapping.fact_id 与 settlement_application.fact_id 引用的是**事实身份**：映射与核销是对一条事实做的，
--     新版本到达后差额与核销的重算另行进行（AT-SA-114）；身份行让这两张表的引用继续指到一个确定的东西，而
--     主键加版本后 fact_id 单独指不到行、按 fact_id 关联的聚合会逐版本重复。
-- (ii) 同一条事实两侧同形（身份一行、版本多行），信封 <租户>/funds-fact/<事实>/<版本> 两头读法一致。
-- 0004 / 0018 不改（已施加，校验和按文件内容算）：这里改它们建的表，不回写它们。

-- 一、版本子表：键（租户、事实、版本），外键到身份行。内容列从身份表原样搬来，CHECK 与 0004 / 0018 逐条同：
-- 金额为正、种类封闭、更正两半同在或同缺且更正版本不得等于本版本、付款人 NULL 即「来源未提供」而空白仍拒。
--
-- 与 CC 那张子表相反，这里**校验版本链的形**。本上下文是铸造方（ADR-0137 决定四）：更正版本由 CorrectAmount
-- 从当前链头形成、必须回指当前链头（裁决 2），链因此是线性的——这句话在库上写成三道约束：回指必须指向同一
-- 事实已有的版本（外键到本表）；同一前版只能被更正一次（回指唯一）；一条事实只有一个首版（无回指的行唯一，
-- 见下方部分唯一索引）。三道合起来让「无后继的那一版」恰好一行，FindByKey 交回链头不需要任何「谁是当前」的
-- 标记列。CC 作为接收方容忍乱序、不校验链是否连续（customs_compliance/0021 头注），是另一侧的纪律。
CREATE TABLE settlement_accounting.external_funds_fact_version (
    tenant_id      text        NOT NULL,
    fact_id        text        NOT NULL,
    version        text        NOT NULL,

    source_ref     text        NOT NULL,
    payer_ref      text,
    kind           text        NOT NULL,
    currency       text        NOT NULL,
    amount_minor   bigint      NOT NULL,
    occurred_at    timestamptz NOT NULL,
    corrects       text,
    corrected_at   timestamptz,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT external_funds_fact_version_pkey
        PRIMARY KEY (tenant_id, fact_id, version),

    CONSTRAINT external_funds_fact_version_belongs_to_fact
        FOREIGN KEY (tenant_id, fact_id)
        REFERENCES settlement_accounting.external_funds_fact (tenant_id, fact_id),

    CONSTRAINT external_funds_fact_version_corrects_a_known_version
        FOREIGN KEY (tenant_id, fact_id, corrects)
        REFERENCES settlement_accounting.external_funds_fact_version (tenant_id, fact_id, version),

    CONSTRAINT external_funds_fact_version_corrects_once
        UNIQUE (tenant_id, fact_id, corrects),

    CONSTRAINT external_funds_fact_version_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(version) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(kind) <> ''
            AND btrim(currency) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT external_funds_fact_version_payer_not_blank
        CHECK (payer_ref IS NULL OR btrim(payer_ref) <> ''),

    CONSTRAINT external_funds_fact_version_amount_positive
        CHECK (amount_minor > 0),

    CONSTRAINT external_funds_fact_version_kind_closed
        CHECK (kind IN ('RECEIPT_CONFIRMED', 'PAYMENT_FAILED', 'FUNDS_RETURNED')),

    CONSTRAINT external_funds_fact_version_correction_coupled
        CHECK (
            (corrects IS NULL AND corrected_at IS NULL)
            OR (corrects IS NOT NULL AND btrim(corrects) <> ''
                AND corrects <> version
                AND corrected_at IS NOT NULL
                AND corrected_at >= occurred_at)
        )
);

-- UNIQUE 对 NULL 不去重（SQL 三值逻辑），「一条事实只有一个首版」要另用部分唯一索引写。
CREATE UNIQUE INDEX external_funds_fact_version_single_first_version
    ON settlement_accounting.external_funds_fact_version (tenant_id, fact_id)
    WHERE corrects IS NULL;

-- 二、身份表退成身份：内容各列搬到版本子表，身份行只留（租户、事实）与首次采用时刻——0004 的 recorded_at
-- 留在身份行上，此后只在首版落下时写一次，后续版本的 recorded_at 各在自己那一行。存量若不为零，搬迁要替
-- 每一行补一个版本链起点，那是编造；当前无租户、本上下文的采用没有生产入口、存量为零（0018 头注），若哪个
-- 库上不为零，宁可让迁移停下交人搬。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM settlement_accounting.external_funds_fact) THEN
        RAISE EXCEPTION 'settlement_accounting.external_funds_fact 有存量行：须人工搬入 external_funds_fact_version 后再施加 0021';
    END IF;
END
$$;

ALTER TABLE settlement_accounting.external_funds_fact
    DROP CONSTRAINT external_funds_fact_scope_not_blank,
    DROP CONSTRAINT external_funds_fact_amount_positive,
    DROP CONSTRAINT external_funds_fact_kind_closed,
    DROP CONSTRAINT external_funds_fact_correction_coupled,
    DROP CONSTRAINT external_funds_fact_payer_not_blank,
    DROP COLUMN source_ref,
    DROP COLUMN payer_ref,
    DROP COLUMN kind,
    DROP COLUMN currency,
    DROP COLUMN amount_minor,
    DROP COLUMN version,
    DROP COLUMN occurred_at,
    DROP COLUMN corrects,
    DROP COLUMN corrected_at,
    DROP COLUMN content_digest;

ALTER TABLE settlement_accounting.external_funds_fact
    ADD CONSTRAINT external_funds_fact_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_id) <> ''
        );
