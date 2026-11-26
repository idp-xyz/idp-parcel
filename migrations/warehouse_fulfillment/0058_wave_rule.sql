-- 成波规则：租户按仓登记「哪些已预占的出库请求放进同一批波次、一个波次最多几张、怎么拣」。
-- 规则是租户的取值，产品不带默认的规则：没登记规则的仓，按规则成波答查无此规则，放波次仍可以由人点单放。
-- 修订只追加，一条规则此刻生效的是它最新的一版；停用也是登一版。
--
-- 圈单的条件都是「不带就是不限」：货主、发运方式留空是不限；件型 ANY 是不限，SINGLE_UNIT 只圈单品单件，
-- MULTI_UNIT 只圈不是单品单件的。max_requests 是一个波次最多放几张出库请求，圈到的单按受理先后切成几个波次。

CREATE TABLE warehouse_fulfillment.wave_rule (
    tenant_id        text        NOT NULL,
    warehouse_code   text        NOT NULL,
    rule_code        text        NOT NULL,
    revision         integer     NOT NULL,
    rule_name        text        NOT NULL,
    owner_account_id text,
    dispatch         text,
    profile          text        NOT NULL,
    max_requests     integer     NOT NULL,
    batch_pick       boolean     NOT NULL,
    deactivated      boolean     NOT NULL,
    operator_id      text        NOT NULL,
    registered_at    timestamptz NOT NULL,

    CONSTRAINT wave_rule_pkey PRIMARY KEY (tenant_id, warehouse_code, rule_code, revision),
    CONSTRAINT wave_rule_revision_positive CHECK (revision >= 1),
    CONSTRAINT wave_rule_max_requests_positive CHECK (max_requests >= 1),
    CONSTRAINT wave_rule_profile CHECK (profile IN ('ANY', 'SINGLE_UNIT', 'MULTI_UNIT')),
    CONSTRAINT wave_rule_dispatch CHECK (dispatch IS NULL OR dispatch IN ('PRODUCT_NETWORK', 'OWNER_LABEL', 'PICKUP')),
    CONSTRAINT wave_rule_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(warehouse_code) <> '' AND btrim(rule_code) <> '' AND btrim(rule_name) <> ''
        AND btrim(operator_id) <> '' AND (owner_account_id IS NULL OR btrim(owner_account_id) <> '')
    )
);

-- 按规则成波的一次执行。同一个执行号只成立一次：重放答已有结果，不再圈一遍。
-- released 是放进波次的请求数，skipped 是圈到了而分配不出、留在已预占的，remaining 是超过一次执行的上限、这次没轮到的。
CREATE TABLE warehouse_fulfillment.wave_rule_run (
    tenant_id      text        NOT NULL,
    run_id         text        NOT NULL,
    warehouse_code text        NOT NULL,
    rule_code      text        NOT NULL,
    rule_revision  integer     NOT NULL,
    operator_id    text        NOT NULL,
    ran_at         timestamptz NOT NULL,
    released       integer     NOT NULL,
    skipped        integer     NOT NULL,
    remaining      integer     NOT NULL,

    CONSTRAINT wave_rule_run_pkey PRIMARY KEY (tenant_id, run_id),
    CONSTRAINT wave_rule_run_rule_fkey
        FOREIGN KEY (tenant_id, warehouse_code, rule_code, rule_revision)
        REFERENCES warehouse_fulfillment.wave_rule (tenant_id, warehouse_code, rule_code, revision),
    CONSTRAINT wave_rule_run_counts CHECK (released >= 1 AND skipped >= 0 AND remaining >= 0)
);

-- 波次上记下它是哪条规则的哪一次执行放出来的；由人点单放的波次这两格是空的。
ALTER TABLE warehouse_fulfillment.wave
    ADD COLUMN rule_code text,
    ADD COLUMN rule_run  text,
    ADD CONSTRAINT wave_rule_pair CHECK ((rule_code IS NULL) = (rule_run IS NULL));

CREATE INDEX wave_rule_run_idx
    ON warehouse_fulfillment.wave (tenant_id, rule_run)
    WHERE rule_run IS NOT NULL;
