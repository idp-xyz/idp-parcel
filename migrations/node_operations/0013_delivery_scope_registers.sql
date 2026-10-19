-- 可验证交付范围的三本册（ADR-0212 决定二、三；票 ps-external-mark-relations/08）：送站预报册（预报与撤回）、送站交接单册（交接单、
-- 成员、关闭、绑定结论）、预报有效时窗册。所有权取 NO CONTEXT「Boundaries and ownership」：送站预报、送站交接单及其绑定确认归本上下文；
-- 客户账户与授权交付关系归 party-commercial，这里只按标识引用，不设外键。
--
-- 每本册都只增：没有改写路径，撞键不覆盖。撤回、关闭、结论各是一份事实，各占一张表，状态由「有没有那一行」读出，不在主行上另设状态列——
-- 状态列要改写，改写就会丢掉是谁、何时、凭什么。范围成立是派生，不落列。
--
-- 本迁移不含任何实例默认值。时窗的提前量与宽限量是租户取值（PAR-NET-18），一行都没登时推导答未配置；演示走 SYN- 合成种子。

-- 送站预报：一行即客户账户经客户渠道交来的一份预报。标识由本上下文签发（ADR-0212 决定二），来源（客户端）+ 来源请求键是重放身份。
--
-- 号码存成数组、不拆子表：判断只问「这个号在不在这份预报里」，按预报取一行就答得了，没有按号码跨预报查的读口。号码的形状门与 PS
-- 入册门同一界，在写入方过（ports.PreAdvisedNumberGate），库面不另立一份界值；非空在库面再守一道，不重复由领域守。
CREATE TABLE node_operations.delivery_pre_advice (
    tenant_id           text        NOT NULL,
    pre_advice_id       text        NOT NULL,

    customer_account_id text        NOT NULL,
    node_ref            text        NOT NULL,
    expected_arrival_at timestamptz NOT NULL,
    parcel_numbers      text[]      NOT NULL,
    -- NULL 即没给交付方线索。
    delivering_party    text,

    source_channel      text        NOT NULL,
    source_request_key  text        NOT NULL,
    content_digest      text        NOT NULL,
    formed_at           timestamptz NOT NULL,

    CONSTRAINT delivery_pre_advice_pkey
        PRIMARY KEY (tenant_id, pre_advice_id),

    CONSTRAINT delivery_pre_advice_source_unique
        UNIQUE (tenant_id, source_channel, source_request_key),

    CONSTRAINT delivery_pre_advice_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(pre_advice_id) <> '' AND btrim(customer_account_id) <> ''
            AND btrim(node_ref) <> '' AND btrim(source_channel) <> '' AND btrim(source_request_key) <> ''
            AND btrim(content_digest) <> ''),

    CONSTRAINT delivery_pre_advice_numbers_present
        CHECK (cardinality(parcel_numbers) >= 1)
);

-- 预报撤回：一份预报至多一份撤回（主键即预报），撤回是终态。customer_account_id 是撤回方；写口只在它等于预报的客户账户时落行。
CREATE TABLE node_operations.delivery_pre_advice_withdrawal (
    tenant_id           text        NOT NULL,
    pre_advice_id       text        NOT NULL,

    customer_account_id text        NOT NULL,
    source_channel      text        NOT NULL,
    source_request_key  text        NOT NULL,
    content_digest      text        NOT NULL,
    withdrawn_at        timestamptz NOT NULL,

    CONSTRAINT delivery_pre_advice_withdrawal_pkey
        PRIMARY KEY (tenant_id, pre_advice_id),

    CONSTRAINT delivery_pre_advice_withdrawal_source_unique
        UNIQUE (tenant_id, source_channel, source_request_key),

    CONSTRAINT delivery_pre_advice_withdrawal_of_pre_advice
        FOREIGN KEY (tenant_id, pre_advice_id)
        REFERENCES node_operations.delivery_pre_advice (tenant_id, pre_advice_id),

    CONSTRAINT delivery_pre_advice_withdrawal_not_blank
        CHECK (btrim(customer_account_id) <> '' AND btrim(source_channel) <> '' AND btrim(source_request_key) <> ''
            AND btrim(content_digest) <> '')
);

-- 送站交接单：一行即节点当场开立的一张交接单，标识由设备签发（ADR-0023）。声称的客户账户至少一个，按标识定序存。
CREATE TABLE node_operations.delivery_handover_note (
    tenant_id            text        NOT NULL,
    note_id              text        NOT NULL,

    node_ref             text        NOT NULL,
    delivering_party     text        NOT NULL,
    claimed_account_ids  text[]      NOT NULL,
    opened_at            timestamptz NOT NULL,

    CONSTRAINT delivery_handover_note_pkey
        PRIMARY KEY (tenant_id, note_id),

    CONSTRAINT delivery_handover_note_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(note_id) <> '' AND btrim(node_ref) <> '' AND btrim(delivering_party) <> ''),

    CONSTRAINT delivery_handover_note_claims_present
        CHECK (cardinality(claimed_account_ids) >= 1)
);

-- 交接单成员：一行即一件作业实物经收寄或归入落在交接单名下。重放身份是（形成方式，事实身份）：收寄带来的成员取那次收寄的来源身份，
-- 归入取归入事实自己的身份，两种身份由不同的设备事实签出，分开认。member_sequence 只为「同一业务时刻的两次归入谁在后」给出落册先后，
-- 不对外。
--
-- 关闭之后不再落成员，由写口在同一张交接单的行锁下判（加成员取共享锁、关闭取排他锁），库面的检查约束读不到另一张表。
CREATE TABLE node_operations.delivery_handover_member (
    tenant_id        text        NOT NULL,
    membership_kind  text        NOT NULL,
    fact_id          text        NOT NULL,

    note_id          text        NOT NULL,
    unit_id          text        NOT NULL,
    formed_at        timestamptz NOT NULL,
    member_sequence  bigint      GENERATED ALWAYS AS IDENTITY,

    CONSTRAINT delivery_handover_member_pkey
        PRIMARY KEY (tenant_id, membership_kind, fact_id),

    CONSTRAINT delivery_handover_member_of_note
        FOREIGN KEY (tenant_id, note_id)
        REFERENCES node_operations.delivery_handover_note (tenant_id, note_id),

    CONSTRAINT delivery_handover_member_kind_closed
        CHECK (membership_kind IN ('RECEPTION', 'ASSIGNMENT')),

    CONSTRAINT delivery_handover_member_not_blank
        CHECK (btrim(fact_id) <> '' AND btrim(unit_id) <> '')
);

-- 按交接单列成员。
CREATE INDEX delivery_handover_member_by_note
    ON node_operations.delivery_handover_member (tenant_id, note_id, member_sequence);

-- 「一件实物最后归入的交接单」那一问（ADR-0212 决定三）：只看归入，按业务时间取最后一次。部分条件与写口的查询谓词逐字同形，规划器才
-- 认得出这本索引可用。
CREATE INDEX delivery_handover_member_last_assignment
    ON node_operations.delivery_handover_member (tenant_id, unit_id, formed_at DESC, member_sequence DESC)
    WHERE membership_kind = 'ASSIGNMENT';

-- 交接单关闭：一张交接单至多关闭一次（主键即交接单）。
CREATE TABLE node_operations.delivery_handover_closure (
    tenant_id  text        NOT NULL,
    note_id    text        NOT NULL,

    fact_id    text        NOT NULL,
    closed_at  timestamptz NOT NULL,

    CONSTRAINT delivery_handover_closure_pkey
        PRIMARY KEY (tenant_id, note_id),

    CONSTRAINT delivery_handover_closure_of_note
        FOREIGN KEY (tenant_id, note_id)
        REFERENCES node_operations.delivery_handover_note (tenant_id, note_id),

    CONSTRAINT delivery_handover_closure_not_blank
        CHECK (btrim(fact_id) <> '')
);

-- 绑定结论：（租户，交接单，客户账户）至多一个结论，确认、否认都是终态（ADR-0212 决定三）；异结论撞主键，不覆盖。运营决定只能确认，
-- 且必带交付方参与方（party-commercial 的参与方标识）与依据引用；客户渠道的结论不带这两格。客户账户在不在声称之列由写口在插入时判。
CREATE TABLE node_operations.delivery_handover_binding (
    tenant_id               text        NOT NULL,
    note_id                 text        NOT NULL,
    customer_account_id     text        NOT NULL,

    conclusion              text        NOT NULL,
    route                   text        NOT NULL,
    concluder               text        NOT NULL,
    delivering_participant  text,
    basis_ref               text,
    concluded_at            timestamptz NOT NULL,

    CONSTRAINT delivery_handover_binding_pkey
        PRIMARY KEY (tenant_id, note_id, customer_account_id),

    CONSTRAINT delivery_handover_binding_of_note
        FOREIGN KEY (tenant_id, note_id)
        REFERENCES node_operations.delivery_handover_note (tenant_id, note_id),

    CONSTRAINT delivery_handover_binding_conclusion_closed
        CHECK (conclusion IN ('CONFIRMED', 'DENIED')),

    CONSTRAINT delivery_handover_binding_route_closed
        CHECK (route IN ('CUSTOMER_CHANNEL', 'OPERATION_DECISION')),

    CONSTRAINT delivery_handover_binding_operation_decision_only_confirms
        CHECK (route <> 'OPERATION_DECISION' OR conclusion = 'CONFIRMED'),

    CONSTRAINT delivery_handover_binding_evidence_with_operation_decision
        CHECK ((route = 'OPERATION_DECISION') = (delivering_participant IS NOT NULL AND basis_ref IS NOT NULL)
           AND (route = 'OPERATION_DECISION' OR (delivering_participant IS NULL AND basis_ref IS NULL))),

    CONSTRAINT delivery_handover_binding_not_blank
        CHECK (btrim(concluder) <> '')
);

-- 预报有效时窗（PAR-NET-18）：一行即（租户，提前量，宽限量，生效区间，登记依据）。时长以分钟存，粒度是机制；生效区间半开，终点 NULL 即
-- 没有终点。只增：同租户区间不重叠由排他约束交付——首条登记没有可锁的既有行，约束是唯一不漏首插并发的那条路（判据同关务解释规则
-- 那本册）。没有「后继给开放前版落终点」那条改写：那会让「重叠在登记时拒」不再成立（ADR-0212 决定二）。
--
-- gist 上的文本等值列要 btree_gist。关务迁移也装它，但本上下文的迁移先施加，干净库上走到这里时它还没装；IF NOT EXISTS 让两处谁先到
-- 都成立。
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE node_operations.pre_advice_window (
    tenant_id        text        NOT NULL,
    effective_from   timestamptz NOT NULL,
    effective_until  timestamptz,

    lead_minutes     integer     NOT NULL,
    grace_minutes    integer     NOT NULL,
    basis_ref        text        NOT NULL,
    registered_at    timestamptz NOT NULL,

    CONSTRAINT pre_advice_window_pkey
        PRIMARY KEY (tenant_id, effective_from),

    CONSTRAINT pre_advice_window_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(basis_ref) <> ''),

    CONSTRAINT pre_advice_window_durations_not_negative
        CHECK (lead_minutes >= 0 AND grace_minutes >= 0),

    CONSTRAINT pre_advice_window_interval_ordered
        CHECK (effective_until IS NULL OR effective_until > effective_from),

    CONSTRAINT pre_advice_window_no_overlap
        EXCLUDE USING gist (
            tenant_id WITH =,
            tstzrange(effective_from, effective_until) WITH &&
        )
);
