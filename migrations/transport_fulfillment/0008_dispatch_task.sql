-- 揽派任务与它的对象范围（tf-unwired-seven/05）。
--
-- 分两张表而不是把对象塞进一列数组：CONTEXT「一个任务和一次尝试可以覆盖多个载运对象，但每个
-- 对象必须分别保存成功、失败、拒收、待确认或其他适用结果，任务汇总只能由对象结果派生」。对象
-- 成员是可被分别指名的东西，压进一列就再也指不动它们中的某一个。
--
-- 逐格完备性在库内再守一遍：领域构造门是第一道，CHECK 是第二道，两道互补不互替（0001 立的
-- 纪律）。全部 CHECK 过 SQL 三值逻辑那一眼：可空列使用前先 IS NULL / IS NOT NULL。

-- 任务本身：工作范围加一个关闭状态。
--
-- **没有版本维，这是本表最要紧的一条。** 领域 Reschedule 的原话是「改约或重派形成的是新**尝试**，
-- 不是新任务」——改约只换窗口、任务身份不变，所以主键就是（租户+任务）。要再揽派一次，那是新
-- 任务、新键。给它加版本列就等于为「改约算不算新任务」立第二个口径。
--
-- **也没有任何到场、控制或交付列。** 任务表达需要完成什么，不表达已经发生什么；那些事实在
-- 履约尝试、场外揽收与有效交付各自的表上，经 task_ref 回指本表。少这几列是有意的。
CREATE TABLE transport_fulfillment.dispatch_task (
    tenant_id      text        NOT NULL,
    task_ref       text        NOT NULL,

    kind           text        NOT NULL,
    place_ref      text        NOT NULL,
    window_from    timestamptz NOT NULL,
    window_to      timestamptz NOT NULL,
    conditions_ref text        NOT NULL,
    opened_at      timestamptz NOT NULL,

    reschedules    integer     NOT NULL DEFAULT 0,

    state          text        NOT NULL,
    closure_basis  text,
    closed_at      timestamptz,

    recorded_at    timestamptz NOT NULL,

    CONSTRAINT dispatch_task_pkey
        PRIMARY KEY (tenant_id, task_ref),

    CONSTRAINT dispatch_task_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(task_ref) <> ''
            AND btrim(place_ref) <> ''
            AND btrim(conditions_ref) <> ''
        ),

    -- 任务种类封闭二值。领域 DispatchTaskKind 只有场外揽收与末端派送两格，库面镜像它——
    -- 绕过构造门的写入路径同样不该落进一个集外取值。
    CONSTRAINT dispatch_task_kind_closed
        CHECK (kind IN ('PICKUP', 'DELIVERY')),

    -- 状态封闭三值。没有「其它」那一格是有意的。
    CONSTRAINT dispatch_task_state_closed
        CHECK (state IN ('OPEN', 'TERMINATED', 'COMPLETED')),

    -- **关闭三件与状态成组**：领域 close 要求依据必备且一次为限，Closure() 按 closed_at 是否
    -- 零值给出。半截会重建出一个「关了但说不出依据」的任务，而**没有依据的关闭与「一次失败尝试
    -- 自动结束任务」在库里分不开**——后者正是 CONTEXT 明禁的那条。
    CONSTRAINT dispatch_task_closure_coupled
        CHECK (
            (state = 'OPEN' AND closure_basis IS NULL AND closed_at IS NULL)
            OR (state <> 'OPEN' AND closure_basis IS NOT NULL AND closed_at IS NOT NULL)
        ),

    CONSTRAINT dispatch_task_closure_basis_not_blank
        CHECK (closure_basis IS NULL OR btrim(closure_basis) <> ''),

    -- 窗口首尾有序：领域构造门与 Reschedule 都要求 windowTo 严格晚于 windowFrom。
    CONSTRAINT dispatch_task_window_ordered
        CHECK (window_to > window_from),

    CONSTRAINT dispatch_task_closed_after_opened
        CHECK (closed_at IS NULL OR closed_at >= opened_at),

    -- 改约次数只增不减。它是计数不是状态，负数只可能来自写坏。
    CONSTRAINT dispatch_task_reschedules_not_negative
        CHECK (reschedules >= 0)
);

-- 任务的对象范围：一个任务覆盖哪些载运对象。
--
-- 主键取（租户+任务+对象）：同一对象在同一任务里只出现一次，领域构造门已按重复对象拒绝整个
-- 任务，库面把同一条守成主键。**本表没有逐对象结果列**——对象级的成功、失败、拒收、待确认在
-- 履约尝试那一侧，本表只说「这个任务的工作范围包含它」。把结果列加进来就是把任务与尝试压成
-- 一张表，而 CONTEXT 第一句就是「揽派任务与履约尝试分离」。
CREATE TABLE transport_fulfillment.dispatch_task_object (
    tenant_id   text        NOT NULL,
    task_ref    text        NOT NULL,
    object_ref  text        NOT NULL,

    recorded_at timestamptz NOT NULL,

    CONSTRAINT dispatch_task_object_pkey
        PRIMARY KEY (tenant_id, task_ref, object_ref),

    CONSTRAINT dispatch_task_object_task_fkey
        FOREIGN KEY (tenant_id, task_ref)
        REFERENCES transport_fulfillment.dispatch_task (tenant_id, task_ref),

    CONSTRAINT dispatch_task_object_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(task_ref) <> ''
            AND btrim(object_ref) <> ''
        )
);

-- 一条**故意没有下沉**的不变量：任务至少覆盖一个对象。
--
-- 它是跨行条件（本表有没有行），CHECK 表达不动；硬写成触发器就等于为同一条不变量立第二个
-- 口径，领域改一次判据、那一份会悄悄漂移。它留在领域（OpenDispatchTask 对空对象集拒绝）与
-- 重建门。写在这里是因为下一个人读到上面那条外键时会问「那空任务呢」——答案是有人守，只是
-- 不在这张表上。与 0006 里 ErrSegmentStillActive 那条注是同一个形状。
