-- 仓储这一路的 SELL 请求：自然键仍是（租户，发生项，版本），另铸请求标识，并记下问计价时的账户、币种、范围与基准时点。
ALTER TABLE settlement_accounting.warehouse_charge_request
    ADD COLUMN request_id text,
    ADD COLUMN account_id text,
    ADD COLUMN settlement_currency text,
    ADD COLUMN pricing_scope text,
    ADD COLUMN pricing_basis_at timestamptz;

ALTER TABLE settlement_accounting.warehouse_charge_request
    DROP CONSTRAINT warehouse_charge_request_status;

ALTER TABLE settlement_accounting.warehouse_charge_request
    ADD CONSTRAINT warehouse_charge_request_status CHECK (status IN (
        'FORMED', 'CONFIRMED', 'ZERO', 'PENDING', 'NOT_CONFIGURED', 'AWAITING_PERSON', 'SUPERSEDED', 'DIFFERENCE'
    ));
