-- 装载分配与它的对象范围（tf-unwired-seven/05）。
--
-- **本表存的是版本链，不是一行会变的记录。** CONTEXT「装载分配形成、变化或撤回时保存版本和
-- 对象范围」——变化与撤回各自形成新版本、原版本原样保留，所以主键里带版本，写入只插不改。
-- 把版本挪出主键就等于允许 UPDATE 覆盖分配历史，而那正是这条硬句要防的。
--
-- **没有任何已装载或控制列。** 分配是执行意图，它不证明物理装载完成也不证明控制转移；实际
-- 装载、短装、多装或错装是与分配版本比较的独立事实。少这几列是有意的。
--
-- **也没有成员计数列。** CONTEXT 要求申请量、接受量、预占量、分配量、释放量与实际装载量分别
-- 保存，那六个量在容量池那一侧；本表若再存一个「分配了几个」，它会与成员行漂移，而漂移时
-- 没有任何东西会报。要数就数成员行。
CREATE TABLE transport_fulfillment.load_assignment (
    tenant_id        text        NOT NULL,
    assignment_ref   text        NOT NULL,
    version          text        NOT NULL,

    schedule_ref     text        NOT NULL,
    -- assigned_at 在整条链上不变：变化与撤回沿用原分配时刻，各自另有自己的时刻列。
    assigned_at      timestamptz NOT NULL,

    corrects_version text,
    revised_at       timestamptz,

    withdrawn        boolean     NOT NULL DEFAULT false,
    withdrawn_at     timestamptz,

    recorded_at      timestamptz NOT NULL,

    CONSTRAINT load_assignment_pkey
        PRIMARY KEY (tenant_id, assignment_ref, version),

    CONSTRAINT load_assignment_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(assignment_ref) <> ''
            AND btrim(version) <> ''
            AND btrim(schedule_ref) <> ''
        ),

    CONSTRAINT load_assignment_corrects_not_blank
        CHECK (corrects_version IS NULL OR btrim(corrects_version) <> ''),

    -- 前版引用指向自己就是一条读不动的链。
    CONSTRAINT load_assignment_corrects_not_self
        CHECK (corrects_version IS NULL OR corrects_version <> version),

    -- 撤回两件成对：Withdrawn() 按 withdrawn 与时刻一起给出，半截会装回一个
    -- 「撤了但不知何时撤的」版本，而那是领域任何路径都产不出的状态。
    CONSTRAINT load_assignment_withdrawal_coupled
        CHECK (withdrawn = (withdrawn_at IS NOT NULL)),

    -- **一个版本要么是变化要么是撤回，不能两者都是。** 领域 ReviseMembers 只填 revised_at、
    -- Withdraw 只填撤回两件，两条路各自产出一个新版本，没有哪条会同时填。
    CONSTRAINT load_assignment_revision_xor_withdrawal
        CHECK (NOT (revised_at IS NOT NULL AND withdrawn)),

    -- **前版引用与「这是链上的后继版本」互为充要。** 首版没有前身也不带变化/撤回时刻；
    -- 后继版本必定回指前身。缺一半就读不出这一版是怎么来的。
    CONSTRAINT load_assignment_successor_coupled
        CHECK ((corrects_version IS NOT NULL) = (revised_at IS NOT NULL OR withdrawn)),

    CONSTRAINT load_assignment_revised_after_assigned
        CHECK (revised_at IS NULL OR revised_at >= assigned_at),

    CONSTRAINT load_assignment_withdrawn_after_assigned
        CHECK (withdrawn_at IS NULL OR withdrawn_at >= assigned_at)
);

-- 分配某一版本的对象范围。
--
-- 主键带版本：**每个版本各存一份自己的成员集**，而不是让后继版本共享前身那一份。共享就等于
-- 改前身的成员集——CONTEXT 要求每个版本各自「保存版本和对象范围」，两版之间差了哪些对象是
-- 要能读出来的，共享一份就读不出来了。代价是同一对象在链上重复几行，那是对的代价。
CREATE TABLE transport_fulfillment.load_assignment_member (
    tenant_id      text        NOT NULL,
    assignment_ref text        NOT NULL,
    version        text        NOT NULL,
    object_ref     text        NOT NULL,

    recorded_at    timestamptz NOT NULL,

    CONSTRAINT load_assignment_member_pkey
        PRIMARY KEY (tenant_id, assignment_ref, version, object_ref),

    CONSTRAINT load_assignment_member_version_fkey
        FOREIGN KEY (tenant_id, assignment_ref, version)
        REFERENCES transport_fulfillment.load_assignment (tenant_id, assignment_ref, version),

    CONSTRAINT load_assignment_member_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(assignment_ref) <> ''
            AND btrim(version) <> ''
            AND btrim(object_ref) <> ''
        )
);

-- 两条故意没有下沉的不变量，写在这里是因为读到上面那几条 CHECK 的人会问起：
--
-- 一、每个版本至少一个对象。跨行条件，CHECK 表达不动；留在领域（FormLoadAssignment 与重建门
--     都对空成员集拒）。
-- 二、已撤回的分配不再变化（ErrLoadAssignmentWithdrawn）。它要看**前一版**的状态，是跨行条件，
--     同样表达不动；留在领域的 ReviseMembers 与 Withdraw。
--
-- 硬把它们写成触发器就是为同一条不变量立第二个口径，领域改一次判据、那一份会悄悄漂移。
-- 与 0006 里 ErrSegmentStillActive 那条注是同一个形状。
