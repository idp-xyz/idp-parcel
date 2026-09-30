-- 结算账户登记册。一行是一个账户：责任法人、结算相对方、收付方向、结算币种、结算政策对象
-- 固定在这一行上，同一租户里这五格再出现一次就是把两个账户并成一个，唯一约束拒掉。
-- 政策这一列存对象标识，不存某一版。账户标识另是主键。同一标识再登不同内容由写入方读回后答冲突，本表不提供更新。
--
-- 不设修订、不设有效区间。改五格中的任何一格是另一个账户，不是这一行的下一版。
-- 合同或责任依据是这一行必须带上的引用，空串过不了 CHECK，库也不替它填值。
-- 对账周期、业务时区、截单时刻、付款条件不在这一行上。
-- 实际付款责任方只在与结算相对方不同时另存一列。

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
    ),
    CONSTRAINT settlement_account_payer_distinct CHECK (
        payer_id IS NULL OR (btrim(payer_id) <> '' AND payer_id <> counterparty_id)
    )
);
