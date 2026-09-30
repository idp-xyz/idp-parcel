-- 结算账户登记册。一行是一个账户：责任法人、结算相对方、收付方向、结算币种、结算政策
-- 固定在这一行上，同一租户里这五格再出现一次就是把两个账户并成一个，唯一约束拒掉。
-- 账户标识另是主键。同一标识再登不同内容由写入方读回后答冲突，本表不提供更新。
--
-- 不设修订、不设有效区间。改五格中的任何一格是另一个账户，不是这一行的下一版。
-- 对账周期、业务时区、截单时刻、付款条件、合同或责任依据是这一行必须带上的租户原文，
-- 空串过不了 CHECK，库也不替它们填值。实际付款责任方只在与结算相对方不同时另存一列。

CREATE TABLE settlement_accounting.settlement_account (
    tenant_id            text        NOT NULL,
    account_id           text        NOT NULL,
    legal_entity_id      text        NOT NULL,
    counterparty_id      text        NOT NULL,
    direction            text        NOT NULL,
    currency             text        NOT NULL,
    settlement_policy_id text        NOT NULL,
    payer_id             text,
    responsibility_basis text        NOT NULL,
    reconciliation_cycle text        NOT NULL,
    business_time_zone   text        NOT NULL,
    cutoff               text        NOT NULL,
    payment_terms        text        NOT NULL,
    registered_at        timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, account_id),
    CONSTRAINT settlement_account_shape_key UNIQUE (
        tenant_id, legal_entity_id, counterparty_id, direction, currency, settlement_policy_id
    ),
    CONSTRAINT settlement_account_direction_check CHECK (direction IN ('RECEIVABLE', 'PAYABLE')),
    CONSTRAINT settlement_account_text_present CHECK (
        btrim(tenant_id) <> ''
        AND btrim(account_id) <> ''
        AND btrim(legal_entity_id) <> ''
        AND btrim(counterparty_id) <> ''
        AND btrim(currency) <> ''
        AND btrim(settlement_policy_id) <> ''
        AND btrim(responsibility_basis) <> ''
        AND btrim(reconciliation_cycle) <> ''
        AND btrim(business_time_zone) <> ''
        AND btrim(cutoff) <> ''
        AND btrim(payment_terms) <> ''
    ),
    CONSTRAINT settlement_account_payer_distinct CHECK (
        payer_id IS NULL OR (btrim(payer_id) <> '' AND payer_id <> counterparty_id)
    )
);
