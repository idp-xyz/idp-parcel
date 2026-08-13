-- 客户当前追踪视图：键=租户+货主客户账户+包裹，库只管当前版本，原版本由替代关系
-- （prior_version 指回）承担历史。
--
-- 账户在键上不是过滤器而是身份的一部分（「客户视图只包含当前货主客户账户及其授权
-- 对象范围」）；跨账户/跨租户的否定结果与「不存在」由 SQL 条件天然长得一样。
--
-- 版本推进用乐观锁：首发只在无当前行时成立（ON CONFLICT DO NOTHING），替代只在
-- 当前版本恰为新版本所指前版时成立（UPDATE ... WHERE version = prior）——两个并发
-- 派生只有一个赢，输家零行命中后重读再走幂等/替代路。
--
-- 四维三态扁平成列并逐维 CHECK：「展示必带内容来处，待确认与不展示必不带」这条领域
-- 构造期规则在库里再守一遍，坏行进不来。

CREATE TABLE visibility_exception.customer_view (
    tenant_id            text        NOT NULL,
    customer_account_id  text        NOT NULL,
    parcel_id            text        NOT NULL,

    version              text        NOT NULL,
    based_on_projection  text        NOT NULL,
    prior_version        text,
    published_at         timestamptz NOT NULL,

    milestones_state     text        NOT NULL,
    milestones_content   text,
    eta_state            text        NOT NULL,
    eta_content          text,
    final_state          text        NOT NULL,
    final_content        text,
    note_state           text        NOT NULL,
    note_content         text,

    recorded_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT customer_view_pkey
        PRIMARY KEY (tenant_id, customer_account_id, parcel_id),

    CONSTRAINT customer_view_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(version) <> ''
            AND btrim(based_on_projection) <> ''
        ),

    CONSTRAINT customer_view_states_closed
        CHECK (
            milestones_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
            AND eta_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
            AND final_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
            AND note_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
        ),

    -- 逐维：展示必带内容，非展示必不带——两个方向的虚构都进不了库。
    CONSTRAINT customer_view_milestones_content_presence
        CHECK ((milestones_state = 'SHOWN') = (milestones_content IS NOT NULL)),
    CONSTRAINT customer_view_eta_content_presence
        CHECK ((eta_state = 'SHOWN') = (eta_content IS NOT NULL)),
    CONSTRAINT customer_view_final_content_presence
        CHECK ((final_state = 'SHOWN') = (final_content IS NOT NULL)),
    CONSTRAINT customer_view_note_content_presence
        CHECK ((note_state = 'SHOWN') = (note_content IS NOT NULL))
);
