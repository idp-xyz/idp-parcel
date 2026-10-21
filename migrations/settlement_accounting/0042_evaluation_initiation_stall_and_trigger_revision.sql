-- 评价发起停办册（ADR-0229 决定四），连同 BUY 触发册的版次（决定七，见文末）。
--
-- 发起停办：一拍计费时点或一份已形成的发生项走到发起那一步就答`未配置`（或包裹没有商业解析），一份请求都没发出去。
-- 此前这一拍入账即散，租户隔天补登也没有东西再推它；这两本册把那一拍记下来，续办时按记下的输入原样重跑发起编排。
--
-- 它们不是请求册：SELL 请求要的结算账户、结算币种与计价范围此刻都答不出，BUY 请求要的费用项目与主要范围同样还没有，
-- 往请求册里塞一行缺格的请求会让按标识回读的计价落空。按自然键一行，同一自然键再停一次换上这一次的停因、时刻与依据——
-- 它是等待表，不是历史；这件包裹在这一时点（这一版发生项）有了请求就不再等着，那一行留作来路。停因只收发起之前、
-- 本路的那几格；`采用了别的时点`与修订版不发起不是停下，不记。不登任何租户的行。

CREATE TABLE settlement_accounting.sell_evaluation_initiation_stall (
    tenant_id        text        NOT NULL,
    parcel_id        text        NOT NULL,
    billing_moment   text        NOT NULL,
    source_fact_ref  text        NOT NULL,
    pricing_basis_at timestamptz NOT NULL,
    stop_cause       text        NOT NULL,
    stopped_at       timestamptz NOT NULL,
    resume_basis     text,
    inserted_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT sell_evaluation_initiation_stall_pkey
        PRIMARY KEY (tenant_id, parcel_id, billing_moment),

    CONSTRAINT sell_evaluation_initiation_stall_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(source_fact_ref) <> ''
            AND (resume_basis IS NULL OR btrim(resume_basis) <> '')
        ),

    CONSTRAINT sell_evaluation_initiation_stall_moment_closed
        CHECK (billing_moment IN ('NETWORK_INTAKE_ADOPTED', 'EFFECTIVE_DELIVERY_FINALIZED')),

    CONSTRAINT sell_evaluation_initiation_stall_cause_closed
        CHECK (stop_cause IN (
            'SETTLEMENT_POLICY_NOT_ADOPTED', 'SETTLEMENT_ACCOUNT_NOT_REGISTERED', 'SELL_BILLING_MOMENT_NOT_ADOPTED',
            'PARCEL_HAS_NO_COMMERCIAL_RESOLUTION'
        ))
);

CREATE INDEX sell_evaluation_initiation_stall_stopped_idx
    ON settlement_accounting.sell_evaluation_initiation_stall (tenant_id, stopped_at);

CREATE TABLE settlement_accounting.buy_evaluation_initiation_stall (
    tenant_id          text        NOT NULL,
    occurrence_id      text        NOT NULL,
    occurrence_version text        NOT NULL,
    occurrence_reason  text        NOT NULL,
    occurred_at        timestamptz NOT NULL,
    agreement_ref      text        NOT NULL,
    stop_cause         text        NOT NULL,
    stopped_at         timestamptz NOT NULL,
    resume_basis       text,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT buy_evaluation_initiation_stall_pkey
        PRIMARY KEY (tenant_id, occurrence_id, occurrence_version),

    CONSTRAINT buy_evaluation_initiation_stall_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(occurrence_id) <> ''
            AND btrim(occurrence_version) <> ''
            AND btrim(occurrence_reason) <> ''
            AND btrim(agreement_ref) <> ''
            AND (resume_basis IS NULL OR btrim(resume_basis) <> '')
        ),

    CONSTRAINT buy_evaluation_initiation_stall_cause_closed
        CHECK (stop_cause IN ('OCCURRENCE_REASON_NOT_REGISTERED', 'FEE_ITEM_NOT_REGISTERED'))
);

CREATE INDEX buy_evaluation_initiation_stall_stopped_idx
    ON settlement_accounting.buy_evaluation_initiation_stall (tenant_id, stopped_at);

-- BUY 发起停办「还在等」按这一版发生项有没有请求判，请求册上原本没有按发生项的索引。
CREATE INDEX evaluation_request_occurrence_idx
    ON settlement_accounting.evaluation_request (tenant_id, occurrence_id, occurrence_version);

-- BUY 触发册加版次（ADR-0229 决定七）。此前本册不设修订：先登了一行没有费用项目的，再来补登答冲突，停在「费用项目
-- 没登」上的发生项就永远等不来续办。现在按（租户、原因）排版次：最新一版没登费用项目时，可以追加一版带费用项目的，
-- 旧版留在册上作历史；读口取最新一版。已登的费用项目换成另一个仍答冲突。存量行都是第一版。
ALTER TABLE settlement_accounting.buy_evaluation_trigger
    ADD COLUMN revision integer NOT NULL DEFAULT 1;

ALTER TABLE settlement_accounting.buy_evaluation_trigger
    ALTER COLUMN revision DROP DEFAULT;

ALTER TABLE settlement_accounting.buy_evaluation_trigger
    DROP CONSTRAINT buy_evaluation_trigger_pkey;

ALTER TABLE settlement_accounting.buy_evaluation_trigger
    ADD CONSTRAINT buy_evaluation_trigger_pkey
        PRIMARY KEY (tenant_id, occurrence_reason, revision),
    ADD CONSTRAINT buy_evaluation_trigger_revision_positive
        CHECK (revision >= 1);
