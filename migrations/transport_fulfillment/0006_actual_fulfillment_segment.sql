-- 实际履约段与履约参与关系（tf-unwired-seven/01）。
--
-- 分两张表而不是一张：CONTEXT 明说「共享实际履约段中的每个载运对象分别成立、结束和更正，
-- 不能由整段结果覆盖成员差异」。压成一张表就表达不出成员差异——那正是这条不变量要防的。
--
-- 逐格完备性在库内再守一遍：领域构造门与重建门是第一道，CHECK 是第二道，两道互补不互替
-- （0001 立的纪律）。全部 CHECK 过 SQL 三值逻辑那一眼：可空列使用前先 IS NULL / IS NOT NULL。

-- 段本身很薄：身份加一个关闭状态。重量全在参与关系上。
--
-- closed 与 closed_at 成对：领域 ClosedAt() 按 closed_at 是否零值给出，半截会重建出一个
-- 「关了但不知何时关的」段，而那是领域任何路径都产不出的状态。
CREATE TABLE transport_fulfillment.actual_fulfillment_segment (
    tenant_id   text        NOT NULL,
    segment_ref text        NOT NULL,

    closed      boolean     NOT NULL DEFAULT false,
    closed_at   timestamptz,
    recorded_at timestamptz NOT NULL,

    CONSTRAINT actual_fulfillment_segment_pkey
        PRIMARY KEY (tenant_id, segment_ref),

    CONSTRAINT actual_fulfillment_segment_scope_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(segment_ref) <> ''),

    CONSTRAINT actual_fulfillment_segment_closure_coupled
        CHECK (closed = (closed_at IS NOT NULL))
);

-- 履约参与关系：一个载运对象参与某段的对象级关系。
--
-- 主键取（租户+段+对象）：CONTEXT「同一实际控制范围不能因伙伴重投、任务重建或批量重试
-- 重复建立履约参与」，同一对象在同一段里只有一条。**已结束的参与也不重开**——再次进入是
-- 新的段，所以这里不需要版本维。
--
-- planned_ref 可空，而那个空是**有依据的缺席不是数据缺失**：CONTEXT 明确允许待路由的产品
-- 在没有可行候选时照样实际揽收，计划段此刻不存在。用 NULL 不用空串，正是为了让这两件事
-- 分得开——下一个人看到 NULL 该读作「这个对象当时没有计划段」，不是「这一行没填完」。
CREATE TABLE transport_fulfillment.fulfillment_participation (
    tenant_id   text        NOT NULL,
    segment_ref text        NOT NULL,
    object_ref  text        NOT NULL,

    planned_ref text,
    entry_kind  text        NOT NULL,
    entry_basis text        NOT NULL,
    entered_at  timestamptz NOT NULL,

    end_kind    text,
    end_basis   text,
    ended_at    timestamptz,

    recorded_at timestamptz NOT NULL,

    CONSTRAINT fulfillment_participation_pkey
        PRIMARY KEY (tenant_id, segment_ref, object_ref),

    CONSTRAINT fulfillment_participation_segment_fkey
        FOREIGN KEY (tenant_id, segment_ref)
        REFERENCES transport_fulfillment.actual_fulfillment_segment (tenant_id, segment_ref),

    CONSTRAINT fulfillment_participation_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(segment_ref) <> ''
            AND btrim(object_ref) <> ''
            AND btrim(entry_basis) <> ''
        ),

    -- 入场种类封闭二值。领域注释：「刻意没有第三格——扫描、装载、订舱确认都立不起参与」。
    -- 库面镜像它，因为绕过构造门的写入路径同样不该落进一个集外取值。
    CONSTRAINT fulfillment_participation_entry_kind_closed
        CHECK (entry_kind IN ('OFFSITE_PICKUP', 'TRANSPORT_HANDOVER')),

    -- 离场三件同在或同缺。Active() 按 ended_at 判，半截会重建出一个既非在场又非离场的参与。
    CONSTRAINT fulfillment_participation_end_coupled
        CHECK (
            (end_kind IS NULL AND end_basis IS NULL AND ended_at IS NULL)
            OR (end_kind IS NOT NULL AND end_basis IS NOT NULL AND ended_at IS NOT NULL)
        ),

    -- 离场种类封闭三值。领域注释：「中断、折返、异常案件不在其中——它们不结束控制」。
    -- 没有「其它」那一格是有意的：多一格就等于给「说不清为什么结束」开了一条路，而
    -- CONTEXT 要求达成计划、中断、折返、提前终止和异常转交各自保留不同结果。
    CONSTRAINT fulfillment_participation_end_kind_closed
        CHECK (
            end_kind IS NULL
            OR end_kind IN ('EFFECTIVE_DELIVERY', 'NEXT_HANDOVER', 'CONTROL_TERMINATED')
        ),

    CONSTRAINT fulfillment_participation_end_basis_not_blank
        CHECK (end_basis IS NULL OR btrim(end_basis) <> ''),

    CONSTRAINT fulfillment_participation_planned_not_blank
        CHECK (planned_ref IS NULL OR btrim(planned_ref) <> ''),

    CONSTRAINT fulfillment_participation_end_after_entry
        CHECK (ended_at IS NULL OR ended_at >= entered_at)
);

-- 一条**故意没有下沉**的不变量：`ErrSegmentStillActive`——仍有在场参与关系时段关不上。
--
-- 它是跨行条件，CHECK 表达不动；硬写成触发器就等于为同一条不变量立第二个口径，领域改一次
-- 判据、那一份会悄悄漂移。它留在领域（构造门 CloseSegment 与重建门各守一次），本表不管。
-- 写在这里是因为下一个人读到上面那条 closure_coupled 时会问「那『关了却还有人在场』呢」
-- ——答案是有人守，只是不在这张表上。
