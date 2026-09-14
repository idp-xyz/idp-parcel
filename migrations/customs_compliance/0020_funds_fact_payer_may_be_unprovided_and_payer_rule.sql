-- 外部资金事实的付款人可「来源未提供」，与「真实程序要不要求付款人」的规则册（票 sa-cc/12）。
-- CC CONTEXT「税费付款核对」把付款人定为「来源提供或真实程序要求的」维度，Rules 一句「未提供或不适用必须
-- 明确记录，规则要求但缺失时保持未决」——所以「未提供」是一格要记下来的值，「要不要求」是登记进来的规则，
-- 两样都不是常量也不是默认。0016 不改（已施加、校验和按文件内容算）：这里放宽它，不回写它。

-- 一、payer_ref 放宽为可空：NULL 在这一列的唯一含义是「来源明确未提供付款人」（ports.ExternalFundsFactRegistration.Payer
-- 的 FundsPayerNotProvided 一格），空白串仍不许——空白既不是引用也不是「未提供」的显式说法。原 not_blank CHECK
-- 连着 payer_ref 一起写，重加时把它从里面拿掉、另立一条只管 payer_ref 的；其余四列的判据一字不变。
ALTER TABLE customs_compliance.external_funds_fact
    ALTER COLUMN payer_ref DROP NOT NULL;

ALTER TABLE customs_compliance.external_funds_fact
    DROP CONSTRAINT external_funds_fact_not_blank;

ALTER TABLE customs_compliance.external_funds_fact
    ADD CONSTRAINT external_funds_fact_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_ref) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(currency) <> ''
        ),
    ADD CONSTRAINT external_funds_fact_payer_provided_or_null
        CHECK (payer_ref IS NULL OR btrim(payer_ref) <> '');

-- 二、规则册：按（租户、监管程序）一行至多一条「要不要求付款人」（形照 0019 的规则型目录行，ADR-0137 决定三）。
-- 它不挂门禁目录那一族（键是范围 / 动作 / 边界、每份申报一行），因为「某程序核对时要不要付款人」是程序的属性，
-- 与哪份申报无关。表上没有任何默认行：哪个程序要、哪个不要属实例半边，登记方一条条登进来；未登记的程序在读口
-- 上是「没有这一行」，核对编排答「规则未配置」而不取任何默认。词形封闭二值，与 domain.PayerRequirement 的 String
-- 同词。改规则不 UPDATE：走复核另登。
CREATE TABLE customs_compliance.duty_payment_payer_rule (
    tenant_id         text        NOT NULL,
    procedure_ref     text        NOT NULL,

    payer_requirement text        NOT NULL,
    registered_at     timestamptz NOT NULL,

    CONSTRAINT duty_payment_payer_rule_pkey
        PRIMARY KEY (tenant_id, procedure_ref),

    CONSTRAINT duty_payment_payer_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(procedure_ref) <> ''
        ),

    CONSTRAINT duty_payment_payer_rule_requirement_closed
        CHECK (payer_requirement IN ('REQUIRED', 'NOT_REQUIRED'))
);
