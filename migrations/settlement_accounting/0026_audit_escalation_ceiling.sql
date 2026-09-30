-- 供应商账单审核的金额上限。超过上限必须升级、等于上限仍在权限内，这两句不在这张表里。
-- 审核授权册只存审核人，上限不写回去。不设修订。不登任何租户的行。

CREATE TABLE settlement_accounting.audit_escalation_ceiling (
    tenant_id        text        NOT NULL,
    supplier_id      text        NOT NULL,
    legal_entity_id  text        NOT NULL,
    currency         text        NOT NULL,
    ceiling_minor    bigint      NOT NULL,
    registered_at    timestamptz NOT NULL,
    inserted_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT audit_escalation_ceiling_pkey
        PRIMARY KEY (tenant_id, supplier_id, legal_entity_id, currency),

    CONSTRAINT audit_escalation_ceiling_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(supplier_id) <> ''
            AND btrim(legal_entity_id) <> ''
            AND btrim(currency) <> ''
        ),

    CONSTRAINT audit_escalation_ceiling_non_negative
        CHECK (ceiling_minor >= 0)
);
