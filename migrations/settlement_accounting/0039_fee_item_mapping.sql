-- 费用项目映射册（ADR-0190 决定五）：键是（租户、计价费用代码），值是（费用项目、登记依据），只增不改。
-- 没登记的代码不映射，SELL 消费门据此答待判断（AT-SA-174），不拿价格代码当费用项目。不登任何租户的行。

CREATE TABLE settlement_accounting.fee_item_mapping (
    tenant_id           text        NOT NULL,
    pricing_charge_code text        NOT NULL,
    fee_item            text        NOT NULL,
    basis_ref           text        NOT NULL,
    registered_at       timestamptz NOT NULL,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT fee_item_mapping_pkey
        PRIMARY KEY (tenant_id, pricing_charge_code),

    CONSTRAINT fee_item_mapping_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(pricing_charge_code) <> ''
            AND btrim(fee_item) <> ''
            AND btrim(basis_ref) <> ''
        )
);
