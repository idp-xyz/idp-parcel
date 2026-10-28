-- 费用确认后接受前占用转正式扣费（ADR-0343）：冻结与信用暴露各加「已转出金额」与「最近转出时刻」两列，另立按（占用、包裹）
-- 记账的转出册，让同一包裹的重放认得出自己。
--
-- 转出金额是冻结与暴露行上的事实，不是第三本账：占着的金额 = amount_minor - converted_minor（仅 status 为占着时），
-- 求和仍由账本当场做（HeldMinor / ExposedMinor），库上不存第二份。原金额与原发生时间不因转出改写。
--
-- 状态加第四格 4=CONVERTED：转满。转满的行不再占用，也不再可释放（没有可放的东西）；部分转出的行仍是 1，占着的是余额。
-- 释放一笔部分转出的占用只放余额，已转出的部分已是正式扣费，不随释放回流。

ALTER TABLE settlement_accounting.funds_freeze
    ADD COLUMN converted_minor bigint      NOT NULL DEFAULT 0,
    ADD COLUMN converted_at    timestamptz,
    DROP CONSTRAINT funds_freeze_status_coherent,
    ADD CONSTRAINT funds_freeze_conversion_coherent
        CHECK (
            converted_minor >= 0
            AND converted_minor <= amount_minor
            AND ((converted_minor = 0) = (converted_at IS NULL))
            AND (converted_at IS NULL OR converted_at >= frozen_at)
        ),
    ADD CONSTRAINT funds_freeze_status_coherent
        CHECK (
            (status = 1 AND released_at IS NULL AND converted_minor < amount_minor)
            OR (status = 2 AND released_at IS NOT NULL AND released_at >= frozen_at AND converted_minor < amount_minor)
            OR (status = 4 AND released_at IS NULL AND converted_minor = amount_minor)
        );

ALTER TABLE settlement_accounting.credit_exposure
    ADD COLUMN converted_minor bigint      NOT NULL DEFAULT 0,
    ADD COLUMN converted_at    timestamptz,
    DROP CONSTRAINT credit_exposure_status_coherent,
    ADD CONSTRAINT credit_exposure_conversion_coherent
        CHECK (
            converted_minor >= 0
            AND converted_minor <= amount_minor
            AND ((converted_minor = 0) = (converted_at IS NULL))
            AND (converted_at IS NULL OR converted_at >= exposed_at)
        ),
    ADD CONSTRAINT credit_exposure_status_coherent
        CHECK (
            (status = 1 AND released_at IS NULL AND converted_minor < amount_minor)
            OR (status = 2 AND released_at IS NOT NULL AND released_at >= exposed_at AND converted_minor < amount_minor)
            OR (status = 4 AND released_at IS NULL AND converted_minor = amount_minor)
        );

-- 转出册：一行是「某笔占用里某件包裹的份额已转出」，键里有占用种类，组合策略下冻结与暴露各转各的。
-- 只增不改——同键再来是重放，冲突即不写；份额金额取自受理前估价留痕里那件包裹的试算合计（pre_acceptance_estimate_trace.trials），
-- 触发它的已确认费用记在 charge_id 上，供追溯「哪一笔确认把这一份转出的」。
CREATE TABLE settlement_accounting.pre_acceptance_control_conversion (
    tenant_id          text        NOT NULL,
    legal_entity       text        NOT NULL,
    account_id         text        NOT NULL,
    currency           text        NOT NULL,
    control_request_id text        NOT NULL,
    parcel_id          text        NOT NULL,
    occupation_kind    text        NOT NULL,

    converted_minor    bigint      NOT NULL,
    charge_id          text        NOT NULL,
    converted_at       timestamptz NOT NULL,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pre_acceptance_control_conversion_pkey
        PRIMARY KEY (tenant_id, legal_entity, account_id, currency, control_request_id, parcel_id, occupation_kind),

    CONSTRAINT pre_acceptance_control_conversion_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(legal_entity) <> ''
            AND btrim(account_id) <> ''
            AND btrim(currency) <> ''
            AND btrim(control_request_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(charge_id) <> ''
        ),

    CONSTRAINT pre_acceptance_control_conversion_kind_closed
        CHECK (occupation_kind IN ('FREEZE', 'EXPOSURE')),

    CONSTRAINT pre_acceptance_control_conversion_amount_positive
        CHECK (converted_minor > 0)
);
