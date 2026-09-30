-- 四本登记册，给既有读口一个可登的地方。册里的行是租户取值，本迁移不插入任何行。
-- 同一键再登不同内容由写入方读回后答冲突，本表不提供更新，也不设修订。
--
-- 供应商应付账户与确认事实里的结算账户标识必须指向已有的结算账户行。
-- 本表不另存账户五格，也不从供应商或费用身份推导账户。

CREATE TABLE settlement_accounting.supplier_audit_authority (
    tenant_id       text        NOT NULL,
    supplier_id     text        NOT NULL,
    legal_entity_id text        NOT NULL,
    auditor_id      text        NOT NULL,
    registered_at   timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, supplier_id, legal_entity_id),
    CONSTRAINT supplier_audit_authority_text_present CHECK (
        btrim(tenant_id) <> ''
        AND btrim(supplier_id) <> ''
        AND btrim(legal_entity_id) <> ''
        AND btrim(auditor_id) <> ''
    )
);

CREATE TABLE settlement_accounting.supplier_payable_account (
    tenant_id       text        NOT NULL,
    supplier_id     text        NOT NULL,
    legal_entity_id text        NOT NULL,
    currency        text        NOT NULL,
    account_id      text        NOT NULL,
    registered_at   timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, supplier_id, legal_entity_id, currency),
    CONSTRAINT supplier_payable_account_text_present CHECK (
        btrim(tenant_id) <> ''
        AND btrim(supplier_id) <> ''
        AND btrim(legal_entity_id) <> ''
        AND btrim(currency) <> ''
        AND btrim(account_id) <> ''
    ),
    CONSTRAINT supplier_payable_account_account_fkey
        FOREIGN KEY (tenant_id, account_id)
        REFERENCES settlement_accounting.settlement_account (tenant_id, account_id)
);

CREATE TABLE settlement_accounting.claim_amount_rule (
    tenant_id                    text        NOT NULL,
    responsibility_conclusion_id text        NOT NULL,
    rule_version                 text        NOT NULL,
    registered_at                timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, responsibility_conclusion_id),
    CONSTRAINT claim_amount_rule_text_present CHECK (
        btrim(tenant_id) <> ''
        AND btrim(responsibility_conclusion_id) <> ''
        AND btrim(rule_version) <> ''
    )
);

-- 确认前供 ConfirmedChargeFactsView 读取的七项。customer_charge 上的同名列是确认
-- 之后钉住的结果（0014），不是这本册：确认当时那些列还是空的。
CREATE TABLE settlement_accounting.charge_confirmation_fact (
    tenant_id              text        NOT NULL,
    charge_id              text        NOT NULL,
    responsible_entity     text        NOT NULL,
    counterparty_ref       text        NOT NULL,
    charge_direction       text        NOT NULL,
    settlement_account_id  text        NOT NULL,
    contract_basis         text        NOT NULL,
    primary_charging_scope text        NOT NULL,
    source_fact_ref        text        NOT NULL,
    registered_at          timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, charge_id),
    CONSTRAINT charge_confirmation_fact_direction_check
        CHECK (charge_direction IN ('RECEIVABLE', 'PAYABLE')),
    CONSTRAINT charge_confirmation_fact_text_present CHECK (
        btrim(tenant_id) <> ''
        AND btrim(charge_id) <> ''
        AND btrim(responsible_entity) <> ''
        AND btrim(counterparty_ref) <> ''
        AND btrim(settlement_account_id) <> ''
        AND btrim(contract_basis) <> ''
        AND btrim(primary_charging_scope) <> ''
        AND btrim(source_fact_ref) <> ''
    ),
    CONSTRAINT charge_confirmation_fact_account_fkey
        FOREIGN KEY (tenant_id, settlement_account_id)
        REFERENCES settlement_accounting.settlement_account (tenant_id, account_id)
);
