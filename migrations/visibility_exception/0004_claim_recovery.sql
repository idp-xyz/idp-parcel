-- 客户索赔与追偿。
--
-- claim_item：键=租户+批次+项（项标识由客户提交侧建立，批次内唯一是它的口径）。
-- 收到、通过资格审核和确认赔偿责任是三个不同判断（CONTEXT 硬句 173）——三判各占
-- 状态列与依据列，压成一列就分不出客户此刻等在哪一步。复核换版走前版列：原结论
-- 保留在 prior_conclusion（CONTEXT 253），不翻旧插新——领域对象本身就是这个形状。
--
-- recovery_matter：键=事项标识；（租户+案件+相对方+范围）唯一承担事项幂等。事项
-- 要件成立即固定（硬句 184），行内无可回写列。
--
-- recovery_action：只增记录（重试是新记录不是改写，硬句 185 过程节点分别记录）；
-- 外键指回事项——动作不可能先于事项存在；attempt 序列按（事项+种类）各自递增，
-- 由应用层 CountActions 供号、同（种类+attempt+节点）重复行不设唯一约束——同一
-- 尝试的多个过程节点各占一行正是要保留的东西。

CREATE TABLE visibility_exception.claim_item (
    tenant_id        text        NOT NULL,
    batch_ref        text        NOT NULL,
    item_id          text        NOT NULL,

    customer_ref     text        NOT NULL,
    contract_ref     text        NOT NULL,
    target_ref       text        NOT NULL,
    kind_ref         text        NOT NULL,
    submitted_at     timestamptz NOT NULL,

    screen           text,
    screen_basis     text,
    conclusion       text,
    concluded_at     timestamptz,
    review_by        timestamptz,
    prior_conclusion text,
    withdrawn        boolean     NOT NULL DEFAULT false,
    withdrawn_at     timestamptz,

    CONSTRAINT claim_item_pkey PRIMARY KEY (tenant_id, batch_ref, item_id),

    CONSTRAINT claim_item_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(batch_ref) <> ''
            AND btrim(item_id) <> ''
            AND btrim(customer_ref) <> ''
            AND btrim(contract_ref) <> ''
            AND btrim(target_ref) <> ''
            AND btrim(kind_ref) <> ''
        ),
    -- 审过必有依据，没审就没有依据——半截审核是坏写入。
    CONSTRAINT claim_item_screen_shape
        CHECK (
            ((screen IS NULL) = (screen_basis IS NULL))
            AND (screen IS NULL OR screen IN ('ELIGIBLE', 'INELIGIBLE'))
            AND (screen_basis IS NULL OR btrim(screen_basis) <> '')
        ),
    -- 结论必经过审（资格通过）、带结论时间与复核截止；未有结论不得有这三样。
    CONSTRAINT claim_item_conclusion_shape
        CHECK (
            ((conclusion IS NULL) = (concluded_at IS NULL))
            AND ((conclusion IS NULL) = (review_by IS NULL))
            AND (conclusion IS NULL
                 OR conclusion IN ('FULLY_ESTABLISHED', 'PARTIALLY_ESTABLISHED', 'NOT_ESTABLISHED', 'UNDETERMINABLE'))
            -- IS NOT DISTINCT FROM：screen 为 NULL 时普通等号让整个 CHECK 按
            -- NULL 放行——没过审的结论会溜进来。
            AND (conclusion IS NULL OR screen IS NOT DISTINCT FROM 'ELIGIBLE')
            AND (concluded_at IS NULL OR review_by IS NULL OR concluded_at <= review_by)
        ),
    -- 前版只随复核出现且不等于现结论（CONTEXT 253 原结论保留）。
    CONSTRAINT claim_item_prior_shape
        CHECK (
            prior_conclusion IS NULL
            OR (conclusion IS NOT NULL
                AND prior_conclusion <> conclusion
                AND prior_conclusion IN ('FULLY_ESTABLISHED', 'PARTIALLY_ESTABLISHED', 'NOT_ESTABLISHED', 'UNDETERMINABLE'))
        ),
    -- 撤回与结论互斥（最终结论前才撤得回），撤回必有时间。
    CONSTRAINT claim_item_withdrawn_shape
        CHECK (
            (withdrawn = (withdrawn_at IS NOT NULL))
            AND (NOT withdrawn OR conclusion IS NULL)
        )
);

CREATE TABLE visibility_exception.recovery_matter (
    tenant_id        text        NOT NULL,
    matter_id        text        NOT NULL,

    case_id          text        NOT NULL,
    counterparty_ref text        NOT NULL,
    scope_ref        text        NOT NULL,
    basis_ref        text        NOT NULL,
    legal_entity_ref text        NOT NULL,
    evidence_ref     text        NOT NULL,
    deadline         timestamptz NOT NULL,
    opened_at        timestamptz NOT NULL,

    CONSTRAINT recovery_matter_pkey PRIMARY KEY (tenant_id, matter_id),
    -- （案件+相对方+范围）的事项幂等：同键当前至多一份。
    CONSTRAINT recovery_matter_current_unique
        UNIQUE (tenant_id, case_id, counterparty_ref, scope_ref),

    CONSTRAINT recovery_matter_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(matter_id) <> ''
            AND btrim(case_id) <> ''
            AND btrim(counterparty_ref) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(legal_entity_ref) <> ''
            AND btrim(evidence_ref) <> ''
        ),
    CONSTRAINT recovery_matter_deadline_after_open
        CHECK (deadline > opened_at)
);

CREATE TABLE visibility_exception.recovery_action (
    tenant_id   text        NOT NULL,
    matter_id   text        NOT NULL,
    kind        text        NOT NULL,
    attempt     integer     NOT NULL,
    milestone   text        NOT NULL,
    content_ref text        NOT NULL,
    obligation  text        NOT NULL,
    occurred_at timestamptz NOT NULL,
    -- 同一尝试的多个过程节点各占一行，主键只能落在到达序上。
    seq         bigint      GENERATED ALWAYS AS IDENTITY,

    CONSTRAINT recovery_action_pkey PRIMARY KEY (seq),
    CONSTRAINT recovery_action_matter_exists
        FOREIGN KEY (tenant_id, matter_id)
        REFERENCES visibility_exception.recovery_matter (tenant_id, matter_id),

    CONSTRAINT recovery_action_not_blank
        CHECK (btrim(content_ref) <> '' AND btrim(obligation) <> ''),
    CONSTRAINT recovery_action_attempt_positive
        CHECK (attempt >= 1),
    -- 预先通知与正式主张不能合并为一个模糊的「已追偿」（硬句 184 末句）。
    CONSTRAINT recovery_action_kind_closed
        CHECK (kind IN ('PRELIMINARY_NOTICE', 'FORMAL_ASSERTION')),
    -- 过程节点封闭集合（硬句 185）：准备、提交、渠道接受、送达、确认与两种失败。
    CONSTRAINT recovery_action_milestone_closed
        CHECK (milestone IN ('PREPARED', 'SUBMITTED', 'CHANNEL_ACCEPTED', 'DELIVERED',
                             'ACKNOWLEDGED', 'SUBMISSION_FAILED', 'DELIVERY_FAILED'))
);

CREATE INDEX recovery_action_by_matter_kind
    ON visibility_exception.recovery_action (tenant_id, matter_id, kind, attempt);
