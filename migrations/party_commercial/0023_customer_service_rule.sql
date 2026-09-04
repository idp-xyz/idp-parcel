-- 客户服务规则册（票 party-commercial-context-gaps/05，ADR-0104）：一个客户服务规则版本的正文——
-- 它挂在哪个商业对象上、责任方、范围，以及首发的两项规则内容：索赔期限与最低材料。
--
-- 与版本册分表，判据同信用政策册（0020）：版本回答「有没有这份规则对象」，正文回答「这一版对
-- 哪些索赔说了什么」。父子表照 0014：父行一条，子行按项类成行。
--
-- **子表分两张强类型表，而不是一张带项类列的表。** 两项内容的形状不同——期限是「种类 × 起算事件
-- × 时长 × 日历」四列，材料是「索赔类型 × 材料条目」逐条成行——并成一张表就得靠一列项类标记去
-- 挑另几列怎么读，那正是本上下文反复否决的「一个数加一列标记」形态；材料条目的去重在数组列上
-- 也写不进 CHECK，逐条成行则由主键守住。ADR-0104 Decision 三「项类首发封闭两值、CHECK 钉死」
-- 在这里的落法是：**项类即表**——另四项（追踪披露、异常响应、客户更新、通知义务）没有表，结构上
-- 写不进来；届时「放宽 CHECK 走新迁移」就是「建新表走新迁移」，同样不必新 ADR。
--
-- 至少一项（Decision 三）：两张子表合起来至少一行。SQL 表达不了跨表的「至少一行」，因此有父行而
-- 两张子表都空是坏数据——装载走 error，不得折成未配置；无父行才是 found=false。
--
-- 适用声明是**并存两列 + 恰一非空**（镜像 domain.CustomerServiceRuleApplicability 的两格封闭）：
-- 同一个标识串作产品与作合同是两件事，合成一列加类别标记之后它们长得一模一样。父行上没有有效
-- 区间——规则版本的区间在版本壳上，这里不抄第二份。
--
-- object_kind CHECK = 10（CustomerServiceRuleObject，0019 放进封闭集）。
--
-- 本表族不进整册装载（LoadForScope）、不进 ViewRevision：解析按范围与锚点选版本壳，正文由消费方
-- 按已选中的版本点读（ports.CustomerServiceRuleContentView）。
--
-- 行属实例半边：期限天数、日历、材料清单由租户登记，今天没有租户因而本表族为空。表上没有任何
-- 默认期限或默认材料——缺规则就是未登记，visibility-exception 据以停在指名到维的未决。

CREATE TABLE party_commercial.customer_service_rule (
    tenant_id             text        NOT NULL,
    object_kind           smallint    NOT NULL,
    object_id             text        NOT NULL,
    version_label         text        NOT NULL,

    service_product_id    text,
    customer_contract_id  text,
    responsible_party_id  text        NOT NULL,
    scope_ref             text        NOT NULL,
    registered_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT customer_service_rule_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT customer_service_rule_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT customer_service_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(responsible_party_id) <> ''
            AND btrim(scope_ref) <> ''
        ),

    CONSTRAINT customer_service_rule_kind_only
        CHECK (object_kind = 10),

    -- 镜像 domain.CustomerServiceRuleApplicability 的两格封闭：恰一列在场，且在场的那一列非空白。
    CONSTRAINT customer_service_rule_applicability_exactly_one
        CHECK ((service_product_id IS NULL) <> (customer_contract_id IS NULL)),

    CONSTRAINT customer_service_rule_applicability_not_blank
        CHECK (
            (service_product_id IS NULL OR btrim(service_product_id) <> '')
            AND (customer_contract_id IS NULL OR btrim(customer_contract_id) <> '')
        )
);

-- 索赔期限：一行一种期限。主键含种类，挡住同一种期限登记两行（每种至多一行，ADR-0104 Decision 二）。
--
-- deadline_kind CHECK 镜像 domain.ClaimDeadlineKind 三值封闭集，逐字对应 visibility-exception CONTEXT
-- 「三个独立期限」。start_event_ref 与 calendar_ref 是引用不是封闭集：起算事件的解释权与日历内容
-- 都在别处（前者归 visibility-exception，后者属实例半边），本表只携带引用；VE 定下起算事件封闭集
-- 那天走新迁移收紧 CHECK。duration_days 以整数天计且为正——非正时长一形成就已届满，不是一条规则。
CREATE TABLE party_commercial.customer_service_rule_claim_deadline (
    tenant_id        text     NOT NULL,
    object_kind      smallint NOT NULL,
    object_id        text     NOT NULL,
    version_label    text     NOT NULL,
    deadline_kind    text     NOT NULL,
    start_event_ref  text     NOT NULL,
    duration_days    integer  NOT NULL,
    calendar_ref     text     NOT NULL,

    CONSTRAINT customer_service_rule_claim_deadline_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, deadline_kind),

    CONSTRAINT customer_service_rule_claim_deadline_rule_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.customer_service_rule
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT customer_service_rule_claim_deadline_kind_closed
        CHECK (deadline_kind IN ('FIRST_CLAIM', 'MATERIAL_SUPPLEMENT', 'CONCLUSION_REVIEW')),

    CONSTRAINT customer_service_rule_claim_deadline_not_blank
        CHECK (btrim(start_event_ref) <> '' AND btrim(calendar_ref) <> ''),

    CONSTRAINT customer_service_rule_claim_deadline_positive
        CHECK (duration_days > 0)
);

-- 最低材料：一行一件材料条目，同一索赔类型的清单就是它的全部行。主键含类型与条目，挡住同一
-- 清单里同一条目登记两次；「清单至少一项」由行的存在表达——一种索赔类型只能经它的条目行进册，
-- 没有条目行就是这一版对该类型没有说话（domain.NewMinimumMaterialsRule 拒空清单）。
--
-- claim_kind_ref 与 material_ref 都是引用：索赔类型目录与材料目录归 visibility-exception，本表不
-- 预造那两本目录，也不对它们设外键。
CREATE TABLE party_commercial.customer_service_rule_minimum_material (
    tenant_id       text     NOT NULL,
    object_kind     smallint NOT NULL,
    object_id       text     NOT NULL,
    version_label   text     NOT NULL,
    claim_kind_ref  text     NOT NULL,
    material_ref    text     NOT NULL,

    CONSTRAINT customer_service_rule_minimum_material_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, claim_kind_ref, material_ref),

    CONSTRAINT customer_service_rule_minimum_material_rule_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.customer_service_rule
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT customer_service_rule_minimum_material_not_blank
        CHECK (btrim(claim_kind_ref) <> '' AND btrim(material_ref) <> '')
);
