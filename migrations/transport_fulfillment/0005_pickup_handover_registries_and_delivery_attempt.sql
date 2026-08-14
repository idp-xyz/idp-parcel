-- 对象级揽收登记、交接判断登记、派送尝试，以及两个结果版本签发器。
--
-- 三张表都只登「已经越过提交边界的东西」，写入代数同 ADR-0031：撞键是业务答案不是
-- 错误，第二个写入方按键读回赢家。逐格完备性在库内再守一遍——领域构造门是第一道，
-- CHECK 是第二道，两道互补不互替（0001 立的纪律）。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼：可空列使用前先 IS NULL / IS NOT NULL。

-- 对象级场外揽收登记。主键取（租户+对象+尝试）：同一次到场对同一对象只登一次揽收，
-- 来源更正走新版本另行登记，不在首登处顶替。
--
-- 控制依据 NOT NULL 且非空是这张表的要害：它正是「有效收寄」与「一次失败到场」的
-- 分界（CONTEXT：只有运输方取得控制才建立履约参与关系）。少了它，一行失败到访就能
-- 冒充揽收，而下游 parcel-shipment 会据此采认收寄。
CREATE TABLE transport_fulfillment.offsite_pickup (
    tenant_id       text        NOT NULL,
    object_ref      text        NOT NULL,
    attempt_ref     text        NOT NULL,

    task_ref        text        NOT NULL,
    place_ref       text        NOT NULL,
    control_ref     text        NOT NULL,
    executed_by     text        NOT NULL,
    pickup_version  text        NOT NULL,
    occurred_at     timestamptz NOT NULL,
    content_digest  text        NOT NULL,
    recorded_at     timestamptz NOT NULL,

    CONSTRAINT offsite_pickup_pkey
        PRIMARY KEY (tenant_id, object_ref, attempt_ref),

    CONSTRAINT offsite_pickup_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_ref) <> ''
            AND btrim(attempt_ref) <> ''
            AND btrim(pickup_version) <> ''
        ),

    -- 控制依据与执行事实七件全备：领域 FormOffsitePickup 的构造门在库面的复刻。
    CONSTRAINT offsite_pickup_control_required
        CHECK (
            btrim(control_ref) <> ''
            AND btrim(task_ref) <> ''
            AND btrim(place_ref) <> ''
            AND btrim(executed_by) <> ''
        ),

    CONSTRAINT offsite_pickup_digest_not_blank
        CHECK (btrim(content_digest) <> '')
);

-- 权威运输交接判断登记。主键取（租户+对象+范围+版本）：判断版本由裁决过程指名，
-- 更正是新版本新行，原行不删——版本链在本体上回指（corrects_version）。
CREATE TABLE transport_fulfillment.transport_handover (
    tenant_id           text        NOT NULL,
    object_ref          text        NOT NULL,
    scope_ref           text        NOT NULL,
    handover_version    text        NOT NULL,

    released_by         text        NOT NULL,
    received_by         text        NOT NULL,
    verdict             text        NOT NULL,
    releasing_evidence  text,
    receiving_evidence  text,
    rule_ref            text,
    basis_ref           text,
    corrects_version    text,
    corrected_at        timestamptz,
    judged_at           timestamptz NOT NULL,
    content_digest      text        NOT NULL,
    recorded_at         timestamptz NOT NULL,

    CONSTRAINT transport_handover_pkey
        PRIMARY KEY (tenant_id, object_ref, scope_ref, handover_version),

    CONSTRAINT transport_handover_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_ref) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(handover_version) <> ''
            AND btrim(released_by) <> ''
            AND btrim(received_by) <> ''
            AND btrim(content_digest) <> ''
        ),

    -- 三值裁决在库里也封闭（domain.HandoverVerdict）。
    CONSTRAINT transport_handover_verdict_closed
        CHECK (verdict IN ('HANDED_OVER', 'REFUSED', 'PENDING_CONFIRMATION')),

    -- 逐格完备性（FormTransportHandover 的库面）：`已交接`要双方证据加适用规则、
    -- 不得带拒收/待确认依据；拒收与待确认必须带依据——没有原因的拒收与数据丢失无从
    -- 分辨，而证据允许只有一侧（缺的那侧往往正是争点）。
    CONSTRAINT transport_handover_verdict_shaped
        CHECK (
            (verdict = 'HANDED_OVER'
                AND releasing_evidence IS NOT NULL AND btrim(releasing_evidence) <> ''
                AND receiving_evidence IS NOT NULL AND btrim(receiving_evidence) <> ''
                AND rule_ref IS NOT NULL AND btrim(rule_ref) <> ''
                AND basis_ref IS NULL)
            OR (verdict IN ('REFUSED', 'PENDING_CONFIRMATION')
                AND basis_ref IS NOT NULL AND btrim(basis_ref) <> '')
        ),

    -- 版本链两形态互斥：首登无前版无更正时间；更正版两者齐、不自指、不早于裁决。
    CONSTRAINT transport_handover_chain_coherent
        CHECK (
            (corrects_version IS NULL AND corrected_at IS NULL)
            OR (
                corrects_version IS NOT NULL
                AND btrim(corrects_version) <> ''
                AND corrects_version <> handover_version
                AND corrected_at IS NOT NULL
                AND corrected_at >= judged_at
            )
        )
);

-- 按（租户+对象+范围）取回版本链，顺序稳定。
CREATE INDEX transport_handover_by_scope
    ON transport_fulfillment.transport_handover
        (tenant_id, object_ref, scope_ref, judged_at, handover_version);

-- 派送尝试与逐对象派送结果。父子两张的形状对齐揽收侧的 pickup_attempt /
-- pickup_attempt_result（0002）：同一个 FulfillmentAttempt 领域对象，两处行模型分家
-- 会让同一格事实长出两个样子。
--
-- 主键取（租户+尝试）而不是来源身份：揽收侧那张登的是「一次提交」因而按来源幂等，
-- 这张登的是「一次到场」本身，而 AttemptReference 就是它的身份——改约与重派形成新
-- 尝试新行，rescheduled_from 指回被接续的旧尝试。
CREATE TABLE transport_fulfillment.delivery_attempt (
    tenant_id        text        NOT NULL,
    attempt_ref      text        NOT NULL,

    task_ref         text        NOT NULL,
    executed_by      text        NOT NULL,
    place_ref        text        NOT NULL,
    planned_from     timestamptz NOT NULL,
    planned_to       timestamptz NOT NULL,
    arrived_at       timestamptz NOT NULL,
    evidence_ref     text        NOT NULL,
    rescheduled_from text,
    objects          jsonb       NOT NULL,
    recorded_at      timestamptz NOT NULL,

    CONSTRAINT delivery_attempt_pkey
        PRIMARY KEY (tenant_id, attempt_ref),

    CONSTRAINT delivery_attempt_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(attempt_ref) <> ''
            AND btrim(task_ref) <> ''
            AND btrim(executed_by) <> ''
            AND btrim(place_ref) <> ''
            AND btrim(evidence_ref) <> ''
        ),

    CONSTRAINT delivery_attempt_window_ordered
        CHECK (planned_to > planned_from),

    -- 对象范围非空：一次没有对象的到场没有可记的执行结果。
    CONSTRAINT delivery_attempt_objects_present
        CHECK (jsonb_typeof(objects) = 'array' AND jsonb_array_length(objects) > 0),

    -- 改约/重派沿用旧身份就是用新到场重写旧尝试——CONTEXT 禁止的正是这件事。
    CONSTRAINT delivery_attempt_reschedule_not_self
        CHECK (rescheduled_from IS NULL OR rescheduled_from <> attempt_ref)
);

CREATE TABLE transport_fulfillment.delivery_attempt_result (
    tenant_id    text        NOT NULL,
    attempt_ref  text        NOT NULL,
    object_ref   text        NOT NULL,

    outcome      text        NOT NULL,
    basis        text,
    occurred_at  timestamptz NOT NULL,

    CONSTRAINT delivery_attempt_result_pkey
        PRIMARY KEY (tenant_id, attempt_ref, object_ref),

    CONSTRAINT delivery_attempt_result_of_attempt
        FOREIGN KEY (tenant_id, attempt_ref)
        REFERENCES transport_fulfillment.delivery_attempt (tenant_id, attempt_ref),

    CONSTRAINT delivery_attempt_result_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(attempt_ref) <> ''
            AND btrim(object_ref) <> ''
        ),

    -- 四值结果在库里也封闭（domain.DeliveryObjectOutcome）。
    CONSTRAINT delivery_attempt_result_outcome_closed
        CHECK (outcome IN ('DELIVERED', 'REFUSED', 'NO_ONE_TO_RECEIVE', 'WRONG_ADDRESS')),

    -- 失败必带原因、妥投不得带依据：妥投的证据走 POD 进有效交付，往成功结果里塞一份
    -- 「依据」会让两处口径打架（FormDeliveryAttemptResult 的库面）。
    CONSTRAINT delivery_attempt_result_basis_coupled
        CHECK (
            (outcome = 'DELIVERED' AND basis IS NULL)
            OR (outcome <> 'DELIVERED' AND basis IS NOT NULL AND btrim(basis) <> '')
        )
);

-- 揽收与交付的结果版本签发器。两条序列分开：两种结果的版本各自是一条独立的号，
-- 合用一条会让「揽收第 7 版」与「交付第 7 版」抢同一个号而其中一个必须跳号——号
-- 本身没有业务含义，但两个域共用一个计数器读起来像有。
--
-- 与计划版本序列同一条纪律：nextval 不随事务回滚，未提交的判断烧掉一个号，但绝不把
-- 这个号让给下一次——两份结果共用一个版本号会被登记表的主键压成一行。
CREATE SEQUENCE transport_fulfillment.pickup_result_version_seq
    AS bigint
    START WITH 1
    INCREMENT BY 1
    NO CYCLE;

CREATE SEQUENCE transport_fulfillment.delivery_result_version_seq
    AS bigint
    START WITH 1
    INCREMENT BY 1
    NO CYCLE;
