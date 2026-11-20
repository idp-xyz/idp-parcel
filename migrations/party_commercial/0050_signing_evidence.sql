-- 签署证据与发布前提（客户合同、供应商协议）。
--
-- 证据挂在版本旁，不进版本正文，也不改内容摘要。目标必须已经在 commercial_version 或
-- publication_draft 至少一处出现，发布前后都能登；两表都可能还没有对方那一行，所以这里不设外键，
-- 存在性由写入时检查。只插不改：登错了再插一条，corrects_evidence_id 指名被更正的那一条。
-- 同一条旧证据至多被更正一次。
--
-- 签署件不进产品库，只留文件名、SHA-256 与受限证据库索引。来源本票只有线下签署。
--
-- 要求规则一户一行，两个布尔分开。没有行就是没登记，发布不设这道门。本迁移不插任何租户行。

CREATE TABLE party_commercial.signing_evidence (
    tenant_id               text        NOT NULL,
    evidence_id             text        NOT NULL,
    -- 2 = 客户合同，3 = 供应商协议，与领域 CommercialObjectKind 的取值同一套。
    object_kind             smallint    NOT NULL,
    object_id               text        NOT NULL,
    version_label           text        NOT NULL,
    signed_on               date        NOT NULL,
    counterparty_signatory  text,
    our_signatory           text,
    file_name               text        NOT NULL,
    file_sha256             text        NOT NULL,
    library_index           text        NOT NULL,
    source                  text        NOT NULL,
    corrects_evidence_id    text,
    recorded_by             text        NOT NULL,
    recorded_at             timestamptz NOT NULL,

    CONSTRAINT signing_evidence_pkey
        PRIMARY KEY (tenant_id, evidence_id),

    CONSTRAINT signing_evidence_kind_closed
        CHECK (object_kind IN (2, 3)),

    CONSTRAINT signing_evidence_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(evidence_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(file_name) <> ''
            AND btrim(library_index) <> ''
            AND btrim(recorded_by) <> ''
            AND (counterparty_signatory IS NULL OR btrim(counterparty_signatory) <> '')
            AND (our_signatory IS NULL OR btrim(our_signatory) <> '')
            AND (corrects_evidence_id IS NULL OR btrim(corrects_evidence_id) <> '')
        ),

    CONSTRAINT signing_evidence_file_name_has_no_separator
        CHECK (strpos(file_name, '/') = 0 AND strpos(file_name, E'\\') = 0),

    CONSTRAINT signing_evidence_sha256_lowercase
        CHECK (file_sha256 ~ '^[0-9a-f]{64}$'),

    CONSTRAINT signing_evidence_source_offline
        CHECK (source = 'OFFLINE_SIGNED'),

    CONSTRAINT signing_evidence_does_not_correct_itself
        CHECK (corrects_evidence_id IS NULL OR corrects_evidence_id <> evidence_id)
);

-- 同一条旧证据至多被更正一次。首登的 corrects 为空，不进这道唯一性。
CREATE UNIQUE INDEX signing_evidence_corrects_once
    ON party_commercial.signing_evidence (tenant_id, corrects_evidence_id)
    WHERE corrects_evidence_id IS NOT NULL;

-- 目录行用 EXISTS 按版本问「有没有证据」，发布门也按这一组键问。
CREATE INDEX signing_evidence_by_version
    ON party_commercial.signing_evidence (tenant_id, object_kind, object_id, version_label);

CREATE TABLE party_commercial.signing_evidence_requirement (
    tenant_id                    text        NOT NULL,
    requires_customer_contract   boolean     NOT NULL,
    requires_supplier_agreement  boolean     NOT NULL,
    registered_at                timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT signing_evidence_requirement_pkey
        PRIMARY KEY (tenant_id),

    CONSTRAINT signing_evidence_requirement_not_blank
        CHECK (btrim(tenant_id) <> '')
);
