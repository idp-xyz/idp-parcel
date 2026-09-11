-- 结算输入版本里「付款核对」那一格的采用登记（票 sa-cc/09；UC-SA-001 步 2「采用明确版本的税费、
-- 付款核对、资金事实和合同责任 → 形成结算输入版本」）。
--
-- 只存核对版本引用与采用时刻，别的一列都没有。覆盖 / 差额 / 有效性三态、关联依据、核对形成时刻都是
-- customs-compliance 的事实：本上下文按引用回读那一版，存一份就是第二处口径，而实际代垫成立判断也不得
-- 由任一单项输入直接推导（SA CONTEXT「实际代垫成立判断」词条）。
--
-- 键取提供方核对幂等键的全部维度——申报范围、税费义务（那边的监管核定税费版本引用）、资金事实、
-- 内容指纹。四维合起来才指得到一版：迟到事实在提供方换指纹换版，同一范围会有多版，各版各一行。
-- 同键重插由适配器以 ON CONFLICT DO NOTHING 答`已采用`，没有更新路径：核对版本在提供方不可变，
-- 内容变了是新指纹、新行，不是改这一行。
--
-- 这张表只登「输入已接收」，不登任何判断；advance_assessment 那张才是判断（0003）。两张分开是
-- CONTEXT 生命周期「结算输入已接收 → 实际代垫成立 / 不成立 / 待判断 / 冲突」两格在库面上的形。

CREATE TABLE settlement_accounting.duty_payment_verification_adoption (
    tenant_id       text        NOT NULL,
    scope_ref       text        NOT NULL,
    duty_ref        text        NOT NULL,
    funds_ref       text        NOT NULL,
    version_digest  text        NOT NULL,
    adopted_at      timestamptz NOT NULL,
    inserted_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT duty_payment_verification_adoption_pkey
        PRIMARY KEY (tenant_id, scope_ref, duty_ref, funds_ref, version_digest),

    CONSTRAINT duty_payment_verification_adoption_refs_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(duty_ref) <> ''
            AND btrim(funds_ref) <> ''
            AND btrim(version_digest) <> ''
        )
);
