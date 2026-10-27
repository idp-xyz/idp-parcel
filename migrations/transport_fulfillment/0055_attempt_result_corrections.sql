-- 待确认的逐对象尝试结果凭同一次上门的后补来源更正成确定结果（票 docs-consistency/code/90；TF CONTEXT「揽收、派送与交付」：
-- 之后同一对象的确定结果，凭同一次上门的后补来源以更正形成新判断版本；待确认那一版留在册上不改写）。
--
-- 更正另立一张追加表而不在结果表上加版本列：首次提交的结果行（待确认那一版）一个字不改，结果表上所有读它的口（尝试登记的重放
-- 与冲突比对、失败尝试费、交付登记）只认首次提交；当前有效的结果由「首次提交，被更正则取更正」读出（适配器的读法，
-- 不在库里建视图——两侧读法不同，揽收要看来源身份、派送要看尝试身份）。
--
-- 回指待确认那一版靠外键：更正行带 corrects_outcome（恒为 PENDING_CONFIRMATION，CHECK 钉死）连同主键三列指向结果行的
-- （…，outcome）唯一键，于是库面就接不上一格已是确定结果的行——「更正只回指待确认」不靠纪律靠形状，与
-- pickup_attempt_pickup 只接 PICKED_UP 同一招。一格待确认至多一条更正（主键同结果行）；对已确定判断的再更正不在这条路上。
--
-- 揽收一侧按（租户，来源身份，对象）落，同 pickup_attempt_result 的主键：结果行按尝试提交的来源身份落，不按尝试身份。派送一侧
-- 按（租户，尝试身份，对象）。
--
-- control_ref 只在更正成揽收到手时在：更正时所报的控制依据，只为重放比对留——控制事实本身是 offsite_pickup 上的那一份，
-- 那里才是权威；这里没有外键指向它，因为对象级登记那一侧的键（租户，对象，尝试）带版本链，没有可被引用的唯一键，两边同笔
-- 事务写、由应用守。decided_by 是运营决定面提交时的操作者（「发行方#sub」），只进审计（ADR-0303 的待遇），可空不回填。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先 IS NULL / IS NOT NULL。

ALTER TABLE transport_fulfillment.delivery_attempt_result
    ADD CONSTRAINT delivery_attempt_result_outcome_unique
        UNIQUE (tenant_id, attempt_ref, object_ref, outcome);

CREATE TABLE transport_fulfillment.pickup_attempt_result_correction (
    tenant_id        text        NOT NULL,
    source_id        text        NOT NULL,
    object_ref       text        NOT NULL,
    corrects_outcome text        NOT NULL DEFAULT 'PENDING_CONFIRMATION',

    outcome          text        NOT NULL,
    basis            text,
    source_ref       text        NOT NULL,
    control_ref      text,
    occurred_at      timestamptz NOT NULL,
    corrected_at     timestamptz NOT NULL,
    decided_by       text,
    recorded_at      timestamptz NOT NULL,

    CONSTRAINT pickup_attempt_result_correction_pkey
        PRIMARY KEY (tenant_id, source_id, object_ref),

    CONSTRAINT pickup_attempt_result_correction_of_pending
        FOREIGN KEY (tenant_id, source_id, object_ref, corrects_outcome)
        REFERENCES transport_fulfillment.pickup_attempt_result (tenant_id, source_id, object_ref, outcome),

    CONSTRAINT pickup_attempt_result_correction_pending_only
        CHECK (corrects_outcome = 'PENDING_CONFIRMATION'),

    -- 更正出来的是确定结果：封闭集不含待确认。
    CONSTRAINT pickup_attempt_result_correction_outcome_closed
        CHECK (outcome IN (
            'PICKED_UP', 'CUSTOMER_ABSENT', 'GOODS_NOT_READY', 'PACKAGING_UNACCEPTABLE', 'EXECUTOR_REFUSED'
        )),

    -- 揽收到手不带依据、带控制依据；其余必带依据、不带控制依据。
    CONSTRAINT pickup_attempt_result_correction_outcome_shape
        CHECK (
            (outcome = 'PICKED_UP' AND basis IS NULL
                AND control_ref IS NOT NULL AND btrim(control_ref) <> '')
            OR (outcome <> 'PICKED_UP' AND control_ref IS NULL
                AND basis IS NOT NULL AND btrim(basis) <> '')
        ),

    CONSTRAINT pickup_attempt_result_correction_not_blank
        CHECK (
            btrim(object_ref) <> ''
            AND btrim(source_ref) <> ''
            AND (decided_by IS NULL OR btrim(decided_by) <> '')
        ),

    -- 更正这一判断不早于它说的那件事。
    CONSTRAINT pickup_attempt_result_correction_after_the_event
        CHECK (corrected_at >= occurred_at)
);

CREATE TABLE transport_fulfillment.delivery_attempt_result_correction (
    tenant_id        text        NOT NULL,
    attempt_ref      text        NOT NULL,
    object_ref       text        NOT NULL,
    corrects_outcome text        NOT NULL DEFAULT 'PENDING_CONFIRMATION',

    outcome          text        NOT NULL,
    basis            text,
    source_ref       text        NOT NULL,
    occurred_at      timestamptz NOT NULL,
    corrected_at     timestamptz NOT NULL,
    decided_by       text,
    recorded_at      timestamptz NOT NULL,

    CONSTRAINT delivery_attempt_result_correction_pkey
        PRIMARY KEY (tenant_id, attempt_ref, object_ref),

    CONSTRAINT delivery_attempt_result_correction_of_pending
        FOREIGN KEY (tenant_id, attempt_ref, object_ref, corrects_outcome)
        REFERENCES transport_fulfillment.delivery_attempt_result (tenant_id, attempt_ref, object_ref, outcome),

    CONSTRAINT delivery_attempt_result_correction_pending_only
        CHECK (corrects_outcome = 'PENDING_CONFIRMATION'),

    CONSTRAINT delivery_attempt_result_correction_outcome_closed
        CHECK (outcome IN ('DELIVERED', 'REFUSED', 'NO_ONE_TO_RECEIVE', 'WRONG_ADDRESS')),

    -- 妥投不带依据（证据走 POD 进有效交付），其余必带。
    CONSTRAINT delivery_attempt_result_correction_basis_coupled
        CHECK (
            (outcome = 'DELIVERED' AND basis IS NULL)
            OR (outcome <> 'DELIVERED' AND basis IS NOT NULL AND btrim(basis) <> '')
        ),

    CONSTRAINT delivery_attempt_result_correction_not_blank
        CHECK (
            btrim(object_ref) <> ''
            AND btrim(source_ref) <> ''
            AND (decided_by IS NULL OR btrim(decided_by) <> '')
        ),

    CONSTRAINT delivery_attempt_result_correction_after_the_event
        CHECK (corrected_at >= occurred_at)
);

-- 运营复核查阅「还在待确认的对象」：按租户取，最早的先看；只索引待确认那几行，确定结果不占索引。
CREATE INDEX pickup_attempt_result_pending
    ON transport_fulfillment.pickup_attempt_result (tenant_id, occurred_at, source_id, object_ref)
    WHERE outcome = 'PENDING_CONFIRMATION';

CREATE INDEX delivery_attempt_result_pending
    ON transport_fulfillment.delivery_attempt_result (tenant_id, occurred_at, attempt_ref, object_ref)
    WHERE outcome = 'PENDING_CONFIRMATION';
