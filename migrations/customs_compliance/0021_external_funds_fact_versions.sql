-- 外部资金事实入向登记加版本维：一版本一行（票 sa-cc/13 裁决 1）。
-- CC CONTEXT 集成规则「资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实」与 UC-CC-009「保留全部
-- 版本和原事实」要的是更正版本进得来、原版本留得住；0016 的 external_funds_fact 主键 (tenant_id, fact_ref) 一事实
-- 一行，更正版本到了只能落成「内容冲突」丢在 inbox。形取**版本子表**而不是主键加版本：0016 的 duty_payment_verification
-- 外键钉在 external_funds_fact (tenant_id, fact_ref) 上——「没有接收的资金事实就没有核对」是事实身份层面的话，
-- 版本是事实的历史；主键加版本会拆掉那道外键，子表让身份行不动、版本各占一行，外键原样成立。
-- 0016 / 0020 不改（已施加，校验和按文件内容算）：这里改它们建的表，不回写它们。

-- 一、版本子表：键（租户、事实、版本）。corrects_version 是提供方给的「本版更正前一版」回指，NULL 即首版；不设
-- 到本表自身的外键——版本链的权威在 settlement-accounting，迟到的前版按自己的版本进，本上下文照登不校验链是否
-- 连续（票面红线「不把 SA 的版本链复制成 CC 的第二份」）。内容各列与 0016 / 0020 在身份表上的判据逐条同：
-- 付款人 NULL 即「来源未提供」、空白仍拒；金额允许为零、为负拒。received_at 是本上下文接收这一版的时间。
CREATE TABLE customs_compliance.external_funds_fact_version (
    tenant_id        text        NOT NULL,
    fact_ref         text        NOT NULL,
    version          text        NOT NULL,

    corrects_version text        NULL,
    source_ref       text        NOT NULL,
    payer_ref        text        NULL,
    currency         text        NOT NULL,
    amount_minor     bigint      NOT NULL,
    occurred_at      timestamptz NOT NULL,
    received_at      timestamptz NOT NULL,

    CONSTRAINT external_funds_fact_version_pkey
        PRIMARY KEY (tenant_id, fact_ref, version),

    CONSTRAINT external_funds_fact_version_belongs_to_fact
        FOREIGN KEY (tenant_id, fact_ref)
        REFERENCES customs_compliance.external_funds_fact (tenant_id, fact_ref),

    CONSTRAINT external_funds_fact_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_ref) <> ''
            AND btrim(version) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(currency) <> ''
        ),

    CONSTRAINT external_funds_fact_version_payer_provided_or_null
        CHECK (payer_ref IS NULL OR btrim(payer_ref) <> ''),

    CONSTRAINT external_funds_fact_version_amount_not_negative
        CHECK (amount_minor >= 0),

    -- 回指若在，必须是另一版：回指自己是形状矛盾。
    CONSTRAINT external_funds_fact_version_corrects_another_version
        CHECK (corrects_version IS NULL OR (btrim(corrects_version) <> '' AND corrects_version <> version))
);

-- 二、身份表退成身份：内容各列搬到版本子表，身份行只留（租户、事实、首次接收时间）。存量搬迁没有版本字面可用
-- （0016 起的一行不知道自己是哪一版），当前无租户、存量为零；若哪个库上不为零，宁可让迁移停下交人搬，不替它编
-- 一个版本。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM customs_compliance.external_funds_fact) THEN
        RAISE EXCEPTION 'customs_compliance.external_funds_fact 有存量行：版本字面无来源，须人工搬入 external_funds_fact_version 后再施加 0021';
    END IF;
END
$$;

ALTER TABLE customs_compliance.external_funds_fact
    DROP CONSTRAINT external_funds_fact_not_blank,
    DROP CONSTRAINT external_funds_fact_payer_provided_or_null,
    DROP CONSTRAINT external_funds_fact_amount_not_negative,
    DROP COLUMN source_ref,
    DROP COLUMN payer_ref,
    DROP COLUMN currency,
    DROP COLUMN amount_minor,
    DROP COLUMN occurred_at;

ALTER TABLE customs_compliance.external_funds_fact
    ADD CONSTRAINT external_funds_fact_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_ref) <> ''
        );
