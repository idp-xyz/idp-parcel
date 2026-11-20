-- 待认领到货全生命周期与处置规则（UC-WF-005；仓配履约 CONTEXT「入库与加工」「待认领到货」）。
--
-- 本迁移补齐四件事，不种任何租户行：
-- 1. receipt_confirmation 增加可选列 unclaimed_arrival_id，供认领后的收货确认引用所认领的到货（AT-WF-059）；
-- 2. unclaimed_arrival 扩展支持一线作业直接登记（WORK_FACT，入库到货或退件）与收货转来（DIVERTED_RECEIPT），
--    原有已转来的行就地回填仓、设备、操作者、发生与接收时刻，并保留对 receipt_confirmation 的外键；
-- 3. 新增认领（只增不改）、认领撤销（只增不改，每笔认领至多撤一次）与处置（每件到货至多处置一次）三张表；
-- 4. 新增各仓待认领处置规则修订表及处置方式子表（只追加修订，绝不 UPDATE）。

ALTER TABLE warehouse_fulfillment.receipt_confirmation
    ADD COLUMN unclaimed_arrival_id text;

ALTER TABLE warehouse_fulfillment.receipt_confirmation
    ADD CONSTRAINT receipt_confirmation_unclaimed_arrival_not_blank
        CHECK (unclaimed_arrival_id IS NULL OR btrim(unclaimed_arrival_id) <> '');

ALTER TABLE warehouse_fulfillment.receipt_confirmation
    ADD CONSTRAINT receipt_confirmation_diverted_no_arrival_ref
        CHECK (outcome <> 'DIVERTED_TO_UNCLAIMED' OR unclaimed_arrival_id IS NULL);

CREATE INDEX receipt_confirmation_unclaimed_arrival_idx
    ON warehouse_fulfillment.receipt_confirmation (tenant_id, unclaimed_arrival_id)
    WHERE unclaimed_arrival_id IS NOT NULL;

-- 扩展 unclaimed_arrival 支持作业事实直接登记与收货转来两种来源。
ALTER TABLE warehouse_fulfillment.unclaimed_arrival
    DROP CONSTRAINT unclaimed_arrival_fact_fkey,
    DROP CONSTRAINT unclaimed_arrival_not_blank;

ALTER TABLE warehouse_fulfillment.unclaimed_arrival
    ALTER COLUMN pre_advice_id DROP NOT NULL,
    ALTER COLUMN owner_account_id DROP NOT NULL;

ALTER TABLE warehouse_fulfillment.unclaimed_arrival
    ADD COLUMN origin                   text        NOT NULL DEFAULT 'DIVERTED_RECEIPT',
    ADD COLUMN kind                     text        NOT NULL DEFAULT 'INBOUND',
    ADD COLUMN warehouse_code           text,
    ADD COLUMN device_id                text,
    ADD COLUMN operator_id              text,
    ADD COLUMN occurred_at              timestamptz,
    ADD COLUMN device_sequence          bigint,
    ADD COLUMN received_at              timestamptz,
    ADD COLUMN observation              text,
    ADD COLUMN storage_location         text,
    ADD COLUMN diverted_receipt_fact_id text,
    ADD COLUMN status                   text        NOT NULL DEFAULT 'UNCLAIMED';

UPDATE warehouse_fulfillment.unclaimed_arrival arrival
   SET warehouse_code           = receipt.warehouse_code,
       device_id                = receipt.device_id,
       operator_id              = receipt.operator_id,
       occurred_at              = receipt.occurred_at,
       received_at              = receipt.received_at,
       diverted_receipt_fact_id = receipt.fact_id
  FROM warehouse_fulfillment.receipt_confirmation receipt
 WHERE arrival.tenant_id = receipt.tenant_id
   AND arrival.fact_id = receipt.fact_id;

ALTER TABLE warehouse_fulfillment.unclaimed_arrival
    ALTER COLUMN warehouse_code SET NOT NULL,
    ALTER COLUMN device_id SET NOT NULL,
    ALTER COLUMN operator_id SET NOT NULL,
    ALTER COLUMN occurred_at SET NOT NULL,
    ALTER COLUMN received_at SET NOT NULL;

ALTER TABLE warehouse_fulfillment.unclaimed_arrival
    ADD CONSTRAINT unclaimed_arrival_origin_closed
        CHECK (origin IN ('WORK_FACT', 'DIVERTED_RECEIPT')),
    ADD CONSTRAINT unclaimed_arrival_kind_closed
        CHECK (kind IN ('INBOUND', 'RETURN')),
    ADD CONSTRAINT unclaimed_arrival_status_closed
        CHECK (status IN ('UNCLAIMED', 'CLAIMED', 'DISPOSED')),
    ADD CONSTRAINT unclaimed_arrival_diverted_fact_fkey
        FOREIGN KEY (tenant_id, diverted_receipt_fact_id)
        REFERENCES warehouse_fulfillment.receipt_confirmation (tenant_id, fact_id),
    ADD CONSTRAINT unclaimed_arrival_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(arrival_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(device_id) <> ''
            AND btrim(operator_id) <> ''
            AND (pre_advice_id IS NULL OR btrim(pre_advice_id) <> '')
            AND (owner_account_id IS NULL OR btrim(owner_account_id) <> '')
            AND (observation IS NULL OR btrim(observation) <> '')
            AND (storage_location IS NULL OR btrim(storage_location) <> '')
        ),
    ADD CONSTRAINT unclaimed_arrival_origin_shape
        CHECK (
            (
                origin = 'DIVERTED_RECEIPT'
                AND kind = 'INBOUND'
                AND diverted_receipt_fact_id = fact_id
                AND pre_advice_id IS NOT NULL
                AND owner_account_id IS NOT NULL
                AND device_sequence IS NULL
            )
            OR (
                origin = 'WORK_FACT'
                AND diverted_receipt_fact_id IS NULL
                AND device_sequence IS NOT NULL
                AND device_sequence >= 1
                AND observation IS NOT NULL
            )
        );

CREATE INDEX unclaimed_arrival_list_idx
    ON warehouse_fulfillment.unclaimed_arrival (tenant_id, received_at DESC, arrival_id DESC);

-- 认领：只增不改。按 (tenant_id, decision_id) 判重；撤销另记在撤销表，原认领行照留。
CREATE TABLE warehouse_fulfillment.unclaimed_arrival_claim (
    tenant_id        text        NOT NULL,
    decision_id      text        NOT NULL,
    arrival_id       text        NOT NULL,
    claim_seq        bigint      GENERATED ALWAYS AS IDENTITY,
    owner_account_id text        NOT NULL,
    pre_advice_id    text,
    operator_id      text        NOT NULL,
    claimed_at       timestamptz NOT NULL,

    CONSTRAINT unclaimed_arrival_claim_pkey
        PRIMARY KEY (tenant_id, decision_id),

    CONSTRAINT unclaimed_arrival_claim_seq_key
        UNIQUE (claim_seq),

    CONSTRAINT unclaimed_arrival_claim_arrival_fkey
        FOREIGN KEY (tenant_id, arrival_id)
        REFERENCES warehouse_fulfillment.unclaimed_arrival (tenant_id, arrival_id),

    CONSTRAINT unclaimed_arrival_claim_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(decision_id) <> ''
            AND btrim(arrival_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND (pre_advice_id IS NULL OR btrim(pre_advice_id) <> '')
            AND btrim(operator_id) <> ''
        )
);

CREATE INDEX unclaimed_arrival_claim_arrival_idx
    ON warehouse_fulfillment.unclaimed_arrival_claim (tenant_id, arrival_id, claim_seq);

-- 认领撤销：只增不改。按 (tenant_id, decision_id) 判重；每笔认领至多被撤销一次。
CREATE TABLE warehouse_fulfillment.unclaimed_arrival_claim_revocation (
    tenant_id         text        NOT NULL,
    decision_id       text        NOT NULL,
    arrival_id        text        NOT NULL,
    claim_decision_id text        NOT NULL,
    reason            text,
    operator_id       text        NOT NULL,
    revoked_at        timestamptz NOT NULL,

    CONSTRAINT unclaimed_arrival_claim_revocation_pkey
        PRIMARY KEY (tenant_id, decision_id),

    CONSTRAINT unclaimed_arrival_claim_revocation_claim_key
        UNIQUE (tenant_id, claim_decision_id),

    CONSTRAINT unclaimed_arrival_claim_revocation_arrival_fkey
        FOREIGN KEY (tenant_id, arrival_id)
        REFERENCES warehouse_fulfillment.unclaimed_arrival (tenant_id, arrival_id),

    CONSTRAINT unclaimed_arrival_claim_revocation_claim_fkey
        FOREIGN KEY (tenant_id, claim_decision_id)
        REFERENCES warehouse_fulfillment.unclaimed_arrival_claim (tenant_id, decision_id),

    CONSTRAINT unclaimed_arrival_claim_revocation_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(decision_id) <> ''
            AND btrim(arrival_id) <> ''
            AND btrim(claim_decision_id) <> ''
            AND (reason IS NULL OR btrim(reason) <> '')
            AND btrim(operator_id) <> ''
        )
);

-- 处置：每件到货至多处置一次，不改库存账；记下所据的处置规则修订。
CREATE TABLE warehouse_fulfillment.unclaimed_arrival_disposition (
    tenant_id           text        NOT NULL,
    arrival_id          text        NOT NULL,
    method              text        NOT NULL,
    rule_warehouse_code text        NOT NULL,
    rule_revision       integer     NOT NULL,
    note                text,
    operator_id         text        NOT NULL,
    disposed_at         timestamptz NOT NULL,

    CONSTRAINT unclaimed_arrival_disposition_pkey
        PRIMARY KEY (tenant_id, arrival_id),

    CONSTRAINT unclaimed_arrival_disposition_arrival_fkey
        FOREIGN KEY (tenant_id, arrival_id)
        REFERENCES warehouse_fulfillment.unclaimed_arrival (tenant_id, arrival_id),

    CONSTRAINT unclaimed_arrival_disposition_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(arrival_id) <> ''
            AND btrim(method) <> ''
            AND btrim(rule_warehouse_code) <> ''
            AND rule_revision >= 1
            AND (note IS NULL OR btrim(note) <> '')
            AND btrim(operator_id) <> ''
        )
);

-- 待认领处置规则修订册：按（租户，仓）只追加修订，绝不 UPDATE。产品不内置默认期限或处置方式。
CREATE TABLE warehouse_fulfillment.unclaimed_disposal_rule_revision (
    tenant_id            text        NOT NULL,
    warehouse_code       text        NOT NULL,
    revision             integer     NOT NULL,
    claim_window_seconds bigint      NOT NULL,
    registrant           text        NOT NULL,
    registered_at        timestamptz NOT NULL,

    CONSTRAINT unclaimed_disposal_rule_revision_pkey
        PRIMARY KEY (tenant_id, warehouse_code, revision),

    CONSTRAINT unclaimed_disposal_rule_revision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(registrant) <> ''
        ),

    CONSTRAINT unclaimed_disposal_rule_revision_positive
        CHECK (revision >= 1 AND claim_window_seconds > 0)
);

CREATE TABLE warehouse_fulfillment.unclaimed_disposal_rule_method (
    tenant_id      text    NOT NULL,
    warehouse_code text    NOT NULL,
    revision       integer NOT NULL,
    position       integer NOT NULL,
    method         text    NOT NULL,

    CONSTRAINT unclaimed_disposal_rule_method_pkey
        PRIMARY KEY (tenant_id, warehouse_code, revision, position),

    CONSTRAINT unclaimed_disposal_rule_method_unique
        UNIQUE (tenant_id, warehouse_code, revision, method),

    CONSTRAINT unclaimed_disposal_rule_method_fkey
        FOREIGN KEY (tenant_id, warehouse_code, revision)
        REFERENCES warehouse_fulfillment.unclaimed_disposal_rule_revision (tenant_id, warehouse_code, revision),

    CONSTRAINT unclaimed_disposal_rule_method_not_blank
        CHECK (position >= 0 AND btrim(method) <> '')
);
