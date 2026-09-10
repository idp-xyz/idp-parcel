-- 交付条件声明族（票 party-commercial-context-gaps/11，ADR-0133 决定四）：服务产品版本声明产品级条件、客户合同
-- 版本只能在其内收紧的商业条件——允许的交付方式集合、收件范围规则引用、交付证明规则引用。它是 PC CONTEXT
-- 「产品声明、合同收紧」那个形（保价条件同形）的第一份落地；此前仓内只有 TF 的端口与替身，语言在、格没有，
-- 租户即便定好了交付方式也无处登，每一份都只能答「没有」。
--
-- 形照 0007 / 0012 / 0027：父行是一层声明的壳（规则引用两格直接在父行——它们各恰一，不是集合），子行是允许的
-- 方式，一行一种；随版本发布登记、更正走新版本，不开行级改写。两层同住一张父表、以 object_kind 分层：
-- 1 = ServiceProductObject（产品层），2 = CustomerContractObject（合同层），别的类别一律拒——交付条件不归接单
-- 规则包那一族（ADR-0133 否决那一支，越权风险点 2 供 owner 复核）。
--
-- 合同层多两列：所收紧的服务产品版本（对象标识 + 版本号）。收紧是对着一份具体的产品层说的话，所以合同层必须
-- 指名、产品层必须为空，由 CHECK 按层钉住。「方式集合在那一版产品层之内」这一条 SQL 表达不了（跨行子集），
-- 由持久化写口在同一事务里读回产品层经领域 TightensWithin 核；表上不留「已核」标记——核不过的行根本不会写进来。
--
-- 方式与规则引用都是开放引用（PAR-NET-09 / PAR-COM-05 / PAR-COM-06 实例半边）：只查非空，不登词表、不校验存在，
-- 也不内置任何一种方式。领域要求「至少一种方式」；SQL 表达不了「子表至少一行」，与 0027 同一句：无父行 = 这一层
-- 没有声明；父行在场而正文立不住 = 装载 error，不得折成「没有」。
--
-- 本表不进 ViewRevision（open-decisions D-4）：声明改动与选择无关。行属实例半边：今天没有租户，两表皆空；表上
-- 没有任何一格默认允许，也没有「本人签收」这类默认方式。

CREATE TABLE party_commercial.delivery_condition (
    tenant_id                  text        NOT NULL,
    object_kind                smallint    NOT NULL,
    object_id                  text        NOT NULL,
    version_label              text        NOT NULL,
    tightens_object_id         text,
    tightens_version_label     text,
    recipient_scope_rule_ref   text        NOT NULL,
    proof_of_delivery_rule_ref text        NOT NULL,
    declared_at                timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT delivery_condition_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT delivery_condition_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(recipient_scope_rule_ref) <> ''
            AND btrim(proof_of_delivery_rule_ref) <> ''
        ),

    -- 1 = ServiceProductObject（产品层），2 = CustomerContractObject（合同层）。
    CONSTRAINT delivery_condition_two_layers
        CHECK (object_kind IN (1, 2)),

    -- 产品层不收紧任何东西；合同层必须指名所收紧的服务产品版本。
    CONSTRAINT delivery_condition_tightens_by_layer
        CHECK (
            (object_kind = 1 AND tightens_object_id IS NULL AND tightens_version_label IS NULL)
            OR (object_kind = 2
                AND btrim(tightens_object_id) <> ''
                AND btrim(tightens_version_label) <> '')
        )
);

-- 一行是一种允许的交付方式。主键含方式引用，同一方式不两行。
CREATE TABLE party_commercial.delivery_condition_method (
    tenant_id     text     NOT NULL,
    object_kind   smallint NOT NULL,
    object_id     text     NOT NULL,
    version_label text     NOT NULL,
    method_ref    text     NOT NULL,

    CONSTRAINT delivery_condition_method_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, method_ref),

    CONSTRAINT delivery_condition_method_parent_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.delivery_condition
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT delivery_condition_method_not_blank
        CHECK (btrim(method_ref) <> '')
);
