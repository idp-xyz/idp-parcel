-- 派送待形成册（ADR-0230 决定一、二；票 demo-chain-gaps/70）。
--
-- 派送发起执行器答`要求缺失`时，哪个对象在哪个段、凭哪条入场依据、缺哪几件、什么时候答的，记在这里；任务形成时了结，对象
-- 已不在段里时作罢。写它的只有执行器那一处，与执行器那一拍同笔：随交接信封发起时就是 inbox 那一笔，补拍与按册重问各是自己
-- 那一笔。
--
-- **一个（租户，段，对象）一行，不按入场依据分行。** 同一段同一对象的参与只有一条替代链（再次进入是新的段），证据更正长出的
-- 替代版本不是进段，派送也就仍是那一项（票 demo-chain-gaps/129）。入场依据只记最近一次答复时链尾那一版。
--
-- **本表是待办台账，不是事实来源。** 一行随执行器最近一次对它的答复改写状态：要求缺失立起或刷新、任务形成了结、对象不在段里
-- 作罢。事实——参与、任务、所有者的答复——各在各的册与所有者那里，本表不替它们留历史。
CREATE TABLE transport_fulfillment.delivery_dispatch_pending (
    tenant_id         text        NOT NULL,
    segment_ref       text        NOT NULL,
    object_ref        text        NOT NULL,

    -- 最近一次答复时链尾参与的入场依据。
    entry_basis       text        NOT NULL,
    -- 最近一次答`要求缺失`时所有者答「没有」的那几件，按地点、时间窗、条件的固定顺序。了结与作罢之后照留：那是这一件为什么
    -- 等过。
    missing           text[]      NOT NULL,
    first_answered_at timestamptz NOT NULL,
    last_answered_at  timestamptz NOT NULL,
    -- 最近一次问这一件的时刻，不论那一拍答了什么（未决、引用被占、不受理也算）。按册重问按它排队、到期重问按它算：只按最近一次
    -- 答复排，答不出缺失的那几件就永远排在队头，后面的问不到。
    last_asked_at     timestamptz NOT NULL,

    state             text        NOT NULL,
    -- 了结时形成或认出的那一项任务。
    task_ref          text,
    -- 作罢时执行器答的那一格（对象不在段里、已离场）。
    lapsed_by         text,
    settled_at        timestamptz,

    CONSTRAINT delivery_dispatch_pending_pkey
        PRIMARY KEY (tenant_id, segment_ref, object_ref),

    CONSTRAINT delivery_dispatch_pending_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(segment_ref) <> ''
            AND btrim(object_ref) <> ''
            AND btrim(entry_basis) <> ''
        ),

    -- 缺件封闭三值，至少一件：没有缺件就不是`要求缺失`。
    CONSTRAINT delivery_dispatch_pending_missing_closed
        CHECK (
            cardinality(missing) >= 1
            AND missing <@ ARRAY['DELIVERY_PLACE', 'DELIVERY_WINDOW', 'DELIVERY_CONDITION']::text[]
        ),

    CONSTRAINT delivery_dispatch_pending_state_closed
        CHECK (state IN ('PENDING', 'FORMED', 'LAPSED')),

    -- 状态与了结三件成组：待形成没有了结；已形成必有任务、没有作罢格；已作罢必有作罢格、没有任务。
    CONSTRAINT delivery_dispatch_pending_settlement_coupled
        CHECK (
            (state = 'PENDING' AND task_ref IS NULL AND lapsed_by IS NULL AND settled_at IS NULL)
            OR (state = 'FORMED' AND task_ref IS NOT NULL AND btrim(task_ref) <> '' AND lapsed_by IS NULL AND settled_at IS NOT NULL)
            OR (state = 'LAPSED' AND lapsed_by IN ('OBJECT_NOT_IN_SEGMENT', 'PARTICIPATION_NOT_ACTIVE')
                AND task_ref IS NULL AND settled_at IS NOT NULL)
        )
);

-- 按租户列待形成的那几件：查阅页与按册重问都只问 PENDING，按最近一次问的时刻排。
CREATE INDEX delivery_dispatch_pending_open_by_tenant
    ON transport_fulfillment.delivery_dispatch_pending (tenant_id, last_asked_at, segment_ref, object_ref)
    WHERE state = 'PENDING';
