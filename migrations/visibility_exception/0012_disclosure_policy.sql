-- 披露策略目录（`PAR-VIS-03` / `PAR-VIS-09`）：对这个货主客户账户，普通查询视图的
-- 四个展示维各自获不准展示、获准时内容从哪来。
--
-- 内容属实例半边（真实披露范围、地点粒度、各维内容引用待登记），表在首发是空的。
-- 空表时视图交回「未配置」，编排把四维全部落成待确认——那是如实的空白，**不是
-- 「不展示」**。把没人作过的披露决定写成不展示，就是替商业责任方签了字。
--
-- 拆成「版本」与「条目」两张表，理由与同目录下里程碑映射相同：必须分得开「目录
-- 整个没配」（等租户登记）与「目录配了但这个客户没有条目」（这个账户还没被写进
-- 这一版规则）。压成一张表，两者都表现为查不到行，而「租户已发布规则、只是还没
-- 轮到这个客户」和「租户完全没登记」的续办对象不同——前者问客户服务补条目，后
-- 者问轨迹运营先发布一版。本口对这两格都交回 found=false（没有默认披露值可发明），
-- 但版本号仍必须能被读口看见，重叠版本才能报冲突而不是静默挑一个。
--
-- 条目按（租户 + 客户账户）建键，**不按投影版本**。投影版本是运行时产物，目录
-- 填不进去；公开规则按客户合同与服务产品形成（CONTEXT），不是按每一版旅程视图
-- 形成。投影仍参与查找：适用版本按投影派生时点选择——规则换版只作用于生效后的
-- 新判断。
--
-- 四维各自独立，不共用一个对客开关（登记册：普通查询、异常披露、主动通知分别
-- 参数化）。通知渠道 / 时限 / 义务判据不在本表，那是 `notification_policy`。
--
-- 地点粒度不是第五维：它落在内容本身，本表不单开一列，以免用一个空开关冒充已经
-- 作过的粒度决定。

CREATE TABLE visibility_exception.disclosure_policy_version (
    tenant_id      text        NOT NULL,
    policy_version text        NOT NULL,

    -- 适用按投影的**派生时点**判定，不按查询时的当前时间：「新版本默认只作用于
    -- 生效后的判断」，一份在旧规则下派生的投影不应被事后换版改判可见性。
    effective_from timestamptz NOT NULL,
    effective_to   timestamptz,
    approved_by    text        NOT NULL,

    CONSTRAINT disclosure_policy_version_pkey PRIMARY KEY (tenant_id, policy_version),

    CONSTRAINT disclosure_policy_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(policy_version) <> ''
            AND btrim(approved_by) <> ''
        ),
    CONSTRAINT disclosure_policy_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

-- 同一租户至多一个仍然有效（未闭区间）的披露策略版本。两个并存版本会让「当前适用
-- 哪一版」有两个答案，而 CONTEXT 不许按最后到达挑一个——库先挡住，适配器再对已
-- 闭区间的重叠兜一道错。
CREATE UNIQUE INDEX disclosure_policy_version_one_open_per_tenant
    ON visibility_exception.disclosure_policy_version (tenant_id)
    WHERE effective_to IS NULL;

CREATE TABLE visibility_exception.disclosure_policy_entry (
    tenant_id            text NOT NULL,
    policy_version       text NOT NULL,
    customer_account_ref text NOT NULL,

    milestones_state     text NOT NULL,
    eta_state            text NOT NULL,
    final_state          text NOT NULL,
    note_state           text NOT NULL,

    milestones_content   text,
    eta_content          text,
    final_content        text,
    note_content         text,

    CONSTRAINT disclosure_policy_entry_pkey
        PRIMARY KEY (tenant_id, policy_version, customer_account_ref),

    CONSTRAINT disclosure_policy_entry_version_fkey
        FOREIGN KEY (tenant_id, policy_version)
        REFERENCES visibility_exception.disclosure_policy_version (tenant_id, policy_version)
        ON DELETE RESTRICT,

    CONSTRAINT disclosure_policy_entry_not_blank
        CHECK (btrim(customer_account_ref) <> ''),

    -- 三态封闭集。多一格就等于承认端口没有的第四种披露决定。
    CONSTRAINT disclosure_policy_entry_states_closed
        CHECK (
            milestones_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
            AND eta_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
            AND final_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
            AND note_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
        ),

    -- 与 ShowDimension / PendDimension / WithholdDimension 逐条对上：展示必带内容
    -- 来处，待确认与不展示必不带。空串不是「没有来处」——写成空串会让「这一维不
    -- 带引用」与「签发了一个空引用」在库里长得一模一样。可空列使用前先 IS NULL /
    -- IS NOT NULL，没有一条比较式能单独以 NULL 决定约束。
    CONSTRAINT disclosure_policy_entry_milestones_shape
        CHECK (
            (milestones_state = 'SHOWN'
                AND milestones_content IS NOT NULL AND btrim(milestones_content) <> '')
            OR (milestones_state IN ('PENDING_CONFIRMATION', 'NOT_DISCLOSED')
                AND milestones_content IS NULL)
        ),
    CONSTRAINT disclosure_policy_entry_eta_shape
        CHECK (
            (eta_state = 'SHOWN'
                AND eta_content IS NOT NULL AND btrim(eta_content) <> '')
            OR (eta_state IN ('PENDING_CONFIRMATION', 'NOT_DISCLOSED')
                AND eta_content IS NULL)
        ),
    CONSTRAINT disclosure_policy_entry_final_shape
        CHECK (
            (final_state = 'SHOWN'
                AND final_content IS NOT NULL AND btrim(final_content) <> '')
            OR (final_state IN ('PENDING_CONFIRMATION', 'NOT_DISCLOSED')
                AND final_content IS NULL)
        ),
    CONSTRAINT disclosure_policy_entry_note_shape
        CHECK (
            (note_state = 'SHOWN'
                AND note_content IS NOT NULL AND btrim(note_content) <> '')
            OR (note_state IN ('PENDING_CONFIRMATION', 'NOT_DISCLOSED')
                AND note_content IS NULL)
        )
);
