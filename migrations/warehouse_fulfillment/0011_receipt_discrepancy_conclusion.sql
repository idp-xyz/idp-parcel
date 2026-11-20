-- 收货差异的结论。一份差异至多一个现行结论，挂在差异标识上，不挂在行上。
-- 收货形成的那一份（多收、错货、破损可以多行）共用一个标识；完结形成的少收是另一个标识。两份各自作结论。
-- 结论不改库存账。
--
-- 货主本人的结论不再改。代录不取代任何已有结论。货主本人的结论取代代录的那一个时，被取代的行留下，current 改为 false。
-- 货主口的来源请求键随成立的结论落册，在租户内唯一。代录没有来源请求键，依据引用只记在代录行上。
--
-- 少收不进 receipt_discrepancy。此前已经落下的少收行没有差异标识，用预报标识补上：预报标识与以后签发的差异标识不是一个签发器。
-- 新的完结在有少收时另签一个标识，同一份少收的各行共用。
--
-- 本迁移只建结构、不种任何行。仓配开关关着的部署也施加本迁移。

ALTER TABLE warehouse_fulfillment.inbound_pre_advice_shortage
    ADD COLUMN discrepancy_id text;

UPDATE warehouse_fulfillment.inbound_pre_advice_shortage
   SET discrepancy_id = pre_advice_id
 WHERE discrepancy_id IS NULL;

ALTER TABLE warehouse_fulfillment.inbound_pre_advice_shortage
    ALTER COLUMN discrepancy_id SET NOT NULL;

ALTER TABLE warehouse_fulfillment.inbound_pre_advice_shortage
    ADD CONSTRAINT inbound_pre_advice_shortage_discrepancy_not_blank
        CHECK (btrim(discrepancy_id) <> '');

CREATE INDEX inbound_pre_advice_shortage_discrepancy_idx
    ON warehouse_fulfillment.inbound_pre_advice_shortage (tenant_id, discrepancy_id);

CREATE TABLE warehouse_fulfillment.receipt_discrepancy_conclusion (
    tenant_id           text        NOT NULL,
    discrepancy_id      text        NOT NULL,
    recorded_seq        bigint      GENERATED ALWAYS AS IDENTITY,
    verdict             text        NOT NULL,
    note                text,
    submission_path     text        NOT NULL,
    source_request_key  text,
    operator_id         text,
    evidence_reference  text,
    recorded_at         timestamptz NOT NULL,
    current             boolean     NOT NULL,

    CONSTRAINT receipt_discrepancy_conclusion_pkey
        PRIMARY KEY (tenant_id, discrepancy_id, recorded_seq),

    CONSTRAINT receipt_discrepancy_conclusion_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(discrepancy_id) <> ''),

    CONSTRAINT receipt_discrepancy_conclusion_verdict
        CHECK (verdict IN ('CONFIRM', 'OBJECT')),

    CONSTRAINT receipt_discrepancy_conclusion_path
        CHECK (submission_path IN ('OWNER', 'ON_BEHALF')),

    CONSTRAINT receipt_discrepancy_conclusion_note
        CHECK (note IS NULL OR btrim(note) <> ''),

    CONSTRAINT receipt_discrepancy_conclusion_shape
        CHECK (
            (
                submission_path = 'OWNER'
                AND source_request_key IS NOT NULL
                AND btrim(source_request_key) <> ''
                AND operator_id IS NULL
                AND evidence_reference IS NULL
            )
            OR (
                submission_path = 'ON_BEHALF'
                AND source_request_key IS NULL
                AND operator_id IS NOT NULL
                AND btrim(operator_id) <> ''
                AND evidence_reference IS NOT NULL
                AND btrim(evidence_reference) <> ''
            )
        )
);

CREATE UNIQUE INDEX receipt_discrepancy_conclusion_current_key
    ON warehouse_fulfillment.receipt_discrepancy_conclusion (tenant_id, discrepancy_id)
    WHERE current;

CREATE UNIQUE INDEX receipt_discrepancy_conclusion_request_key
    ON warehouse_fulfillment.receipt_discrepancy_conclusion (tenant_id, source_request_key)
    WHERE source_request_key IS NOT NULL;
