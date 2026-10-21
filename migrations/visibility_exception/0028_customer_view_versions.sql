-- 客户全程追踪视图版本只增不改写：替代追加新版本行并回指前一版，已向客户发布过的版本连同四维与发布时间一并
-- 留存、按版本读得回（VE CONTEXT「已经向客户发布的里程碑、ETA、异常说明或通知出现更正时，必须以追加更新或明确
-- 替代关系表达，不得静默删除或改写历史披露」）。当前版由 customer_view_current 的显式标记指名，读当前版走标记
-- 直取，不扫描历史——形状照 0016 的投影版本表（ADR-0065 决定一、二）。
--
-- 0001 头注称「库只管当前版本，原版本由替代关系（prior_version 指回）承担历史」——那句与它自己造成的效果不符：
-- 替代时同一行被 UPDATE 改写，指针在，靶子不在。已施加迁移是不可变资产（校验和把关），同义句不改写历史文件，
-- 由本迁移在此更正。
--
-- 版本行一经写入不再改动。投影换了版本而四维没变时视图不换代（CONTEXT「投影重新派生本身不要求形成新的客户视图
-- 版本」），这时改记的「当前所依据的投影版本」落在当前标记上，不落在版本行上：版本行的 based_on_projection
-- 是那一版形成时所依据的投影，标记上的那一格是当前版最近一次按之确认四维未变的投影。两格分开，已发布的版本行
-- 就一列都不必改。

CREATE TABLE visibility_exception.customer_view_version (
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

    -- 账户与包裹在键上：版本只在它所属的那份视图里有意义，按版本读回也带着账户，跨账户探针与不存在同答。
    CONSTRAINT customer_view_version_pkey
        PRIMARY KEY (tenant_id, customer_account_id, parcel_id, version),

    CONSTRAINT customer_view_version_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(version) <> ''
            AND btrim(based_on_projection) <> ''
        ),
    CONSTRAINT customer_view_version_prior_not_self
        CHECK (
            prior_version IS NULL
            OR (btrim(prior_version) <> '' AND prior_version <> version)
        ),

    CONSTRAINT customer_view_version_states_closed
        CHECK (
            milestones_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
            AND eta_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
            AND final_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
            AND note_state IN ('SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED')
        ),

    -- 逐维：展示必带内容，非展示必不带——两个方向的虚构都进不了库。
    CONSTRAINT customer_view_version_milestones_content_presence
        CHECK ((milestones_state = 'SHOWN') = (milestones_content IS NOT NULL)),
    CONSTRAINT customer_view_version_eta_content_presence
        CHECK ((eta_state = 'SHOWN') = (eta_content IS NOT NULL)),
    CONSTRAINT customer_view_version_final_content_presence
        CHECK ((final_state = 'SHOWN') = (final_content IS NOT NULL)),
    CONSTRAINT customer_view_version_note_content_presence
        CHECK ((note_state = 'SHOWN') = (note_content IS NOT NULL))
);

-- prior_version 留裸文本不设外键：存量行的前身已被旧 UPDATE 改写、无从追回，指针如实留存。自本迁移起，替代
-- 只在当前标记恰指前一版时成立，而标记经外键只能指存下来的版本行，新链条的前身因此都读得回。

CREATE TABLE visibility_exception.customer_view_current (
    tenant_id            text NOT NULL,
    customer_account_id  text NOT NULL,
    parcel_id            text NOT NULL,
    version              text NOT NULL,
    based_on_projection  text NOT NULL,

    CONSTRAINT customer_view_current_pkey
        PRIMARY KEY (tenant_id, customer_account_id, parcel_id),

    -- 版本列的非空形状由外键传递保证：四列必须整体命中一条版本行，而版本行已拦空白。
    CONSTRAINT customer_view_current_names_stored_version
        FOREIGN KEY (tenant_id, customer_account_id, parcel_id, version)
        REFERENCES visibility_exception.customer_view_version (tenant_id, customer_account_id, parcel_id, version),

    CONSTRAINT customer_view_current_basis_not_blank
        CHECK (btrim(based_on_projection) <> '')
);

-- 存量行是各视图现存的当前版：迁入版本表并立当前标记，先版本后标记以满足外键。前版指回原样带过来——那一版
-- 确曾替代过别的版本，抹成首版等于改写已发布的替代关系；它指的那一行读不回，是旧形状已经造成的事实。

INSERT INTO visibility_exception.customer_view_version
    (tenant_id, customer_account_id, parcel_id,
     version, based_on_projection, prior_version, published_at,
     milestones_state, milestones_content,
     eta_state, eta_content,
     final_state, final_content,
     note_state, note_content,
     recorded_at)
SELECT tenant_id, customer_account_id, parcel_id,
       version, based_on_projection, prior_version, published_at,
       milestones_state, milestones_content,
       eta_state, eta_content,
       final_state, final_content,
       note_state, note_content,
       recorded_at
  FROM visibility_exception.customer_view;

INSERT INTO visibility_exception.customer_view_current
    (tenant_id, customer_account_id, parcel_id, version, based_on_projection)
SELECT tenant_id, customer_account_id, parcel_id, version, based_on_projection
  FROM visibility_exception.customer_view;

DROP TABLE visibility_exception.customer_view;
