-- 服务产品形态册（ADR-0050）：一行记一个服务产品版本的服务形态。
--
-- 与版本册分表而不是往 commercial_version 加一列：形态只对九类里的一类有意义，加列会让
-- 另外八类各带一个永远为空的格。分工也是 ADR-0050 定的——版本回答「有没有这份产品对象」，
-- 产品回答「它是哪种服务形态」，两个答案分属两次登记。
--
-- 行**不自带版本快照**，与 0003 授权册相反。0003 当时内嵌整份版本文档，是因为 0001 的
-- object_kind CHECK 只到 1..7、第九类根本进不去版本册；0004 放宽到 1..9 之后那个理由已经
-- 消失。复制一份版本壳就能让两份不一致——版本册里已 RETIRED 而形态行内嵌的快照仍写着
-- EFFECTIVE——而那正是 ViewRevision 存在要防的东西，复制会让它在登记册自己内部被绕过。
-- 本表因此只存形态，版本壳一律回指 commercial_version。
--
-- 本表也不存 scope_ref：范围是版本壳上的事实，装载按版本行的 scope_ref 过滤。存第二份会让
-- 「形态行说 A 范围、版本行说 B 范围」这种行存得下来。
--
-- 不建二级索引：装载由 commercial_version 侧驱动（按 tenant_id + scope_ref 走既有
-- commercial_version_by_scope），再按四元键左连接到本表，命中的正是本表主键。
--
-- 行属实例半边（哪个产品是哪种形态待 PAR-COM-05 提供）；模式属机制半边。今天表空，装载
-- 交回的登记册因此没有形态。这一格**有意不设「未配置」标记列**：ADR-0050 决定四明写
-- 「产品缺席不使解析从`唯一解析`退化为`无适用依据`」——缺席就是查无此行，解析照常成立、
-- 只是形态不可观察，不需要第三种取值来表达它。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：本表无可空列，没有比较式能以
-- NULL 决定约束。

CREATE TABLE party_commercial.service_product_form (
    tenant_id     text        NOT NULL,
    object_kind   smallint    NOT NULL,
    object_id     text        NOT NULL,
    version_label text        NOT NULL,

    form          text        NOT NULL,
    registered_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT service_product_form_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    -- 形态挂在一份已入册的版本上。用外键而不是靠约定：一行无主的形态读回来无从判断它属于
    -- 哪个范围，而装载正是按版本行的范围过滤的——没有版本行，这一行既进不了任何范围的视图，
    -- 也不会有任何东西报它存在。
    CONSTRAINT service_product_form_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT service_product_form_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
        ),

    -- 1 = ServiceProductObject。形态只归服务产品，挂到别的类别上读回来无法构造。
    CONSTRAINT service_product_form_service_product_only
        CHECK (object_kind = 1),

    -- 镜像 domain.ServiceProductForm 今日封闭集。独立面单渠道服务是长期产品形态，
    -- PAR-COM-12 明确它对首发不适用，领域里有意没列，这里同样不列——预先列上第二个值
    -- 就是替租户拟一种它还没有的形态。扩展先改领域封闭集，再改这一条。
    CONSTRAINT service_product_form_closed
        CHECK (form IN ('NETWORK_SERVICE'))
);
