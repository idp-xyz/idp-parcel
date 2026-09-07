-- 「资料修订」进授权动作封闭集 + 合同委派册（票 party-commercial-context-gaps/08，ADR-0116）。
--
-- 两件同一份迁移，因为它们兑现的是同一句 CONTEXT：「接受后客户原始资料的修订是与人工复核、
-- 主动拒绝并列的授权动作……运营角色代客户请求时，实际决定方由该合同版本的合同委派解出」。
-- 动作格答「许不许」，委派答「谁替谁决定」——只加前者，运营角色代录的请求永远解不出决定方；
-- 只加后者，委派的那个动作在授权册上进不去。
--
-- 一、authorization_grant 的动作 CHECK 重建为三值。0003 头注预告「撤回与资料修订不在集内，本表
-- 不预开空格」，本迁移兑现其中资料修订那一半；撤回照旧不在集内（一票一格，归 PAR-COM-14 那条线）。
-- 不改已施加的 0003：CHECK 走 DROP + ADD，既有行一字不动——它们的取值仍在新集合内。
-- 镜像 domain.AuthorizedAction；新增动作先改领域封闭集，再改这一条。

ALTER TABLE party_commercial.authorization_grant
    DROP CONSTRAINT authorization_grant_action_closed;

ALTER TABLE party_commercial.authorization_grant
    ADD CONSTRAINT authorization_grant_action_closed
        CHECK (action IN ('MANUAL_REVIEW', 'ACTIVE_REJECTION', 'SOURCE_DATA_AMENDMENT'));

-- 二、合同委派册：父子两表，形照 0007 / 0012——拥有对象是**客户合同版本**（object_kind = 2），
-- 委派是这份合同说的话（ADR-0042 的归属纪律），随合同版本发布登记，更正走新合同版本，不开行级
-- UPDATE / DELETE。外键回 commercial_version：委派挂在一个入了册的版本上，不许先于版本存在。
--
-- 父行是声明壳；子行一条一委派，主键含（动作 × 范围 × 等级）守 ADR-0116 Decision 二「同一合同
-- 版本内（动作 × 范围 × 等级）唯一」。领域要求「至少一条」子行，SQL 表达不了：无父行 = 未声明
-- （found=false）；父行在场而零子行 = 装载 error，不得折成未声明（判据同 0013 三族）。
--
-- 委派方用「种类 + 引用」两列而不是两个可空列恰一：领域侧两个构造器分立已经把「法人委派」与
-- 「忘了填」分开，库上再摆两个可空列只是多一种造出「两空」坏行的方法。种类 CHECK 镜像
-- domain.DelegatorKind。
--
-- 受托方是权限等级（authority_level）而不是操作者主体：本上下文不存操作者（ADR-0100 Decision 二）。
-- 行上没有资料组：委派答「谁能替谁提」，允许矩阵（票 pc-gaps/10）答「这一格能不能改」，两问分开。
--
-- action 的 CHECK 首发只有 SOURCE_DATA_AMENDMENT：客户只能委派自己拥有的决定，人工复核与主动拒绝
-- 是运营侧凭授权规则自己作的决定。镜像 domain.AuthorizedAction 里 decidedByCustomer 为真的那一格。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼：唯一可空列 effective_ends_at 先 IS NULL 再比较。
--
-- 行属实例半边：谁委派给哪一等级、哪个范围、什么期间由租户在合同里登记（PAR-COM-13），今天没有
-- 租户因而本表族为空；表上没有任何默认委派——缺行就是没委派，Authorize 据以答 ErrDelegationAbsent。

CREATE TABLE party_commercial.contract_delegation_content (
    tenant_id      text        NOT NULL,
    object_kind    smallint    NOT NULL,
    object_id      text        NOT NULL,
    version_label  text        NOT NULL,
    declared_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT contract_delegation_content_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT contract_delegation_content_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT contract_delegation_content_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
        ),

    -- 2 = CustomerContractObject。
    CONSTRAINT contract_delegation_content_contract_only
        CHECK (object_kind = 2)
);

CREATE TABLE party_commercial.contract_delegation (
    tenant_id            text        NOT NULL,
    object_kind          smallint    NOT NULL,
    object_id            text        NOT NULL,
    version_label        text        NOT NULL,
    action               text        NOT NULL,
    scope_ref            text        NOT NULL,
    authority_level      text        NOT NULL,

    delegator_kind       text        NOT NULL,
    delegator_ref        text        NOT NULL,
    effective_starts_at  timestamptz NOT NULL,
    effective_ends_at    timestamptz,

    CONSTRAINT contract_delegation_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, action, scope_ref, authority_level),

    CONSTRAINT contract_delegation_content_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.contract_delegation_content
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT contract_delegation_not_blank
        CHECK (
            btrim(scope_ref) <> ''
            AND btrim(authority_level) <> ''
            AND btrim(delegator_ref) <> ''
        ),

    -- 镜像 domain.AuthorizedAction 里决定权归客户的那一格；放宽走新迁移。
    CONSTRAINT contract_delegation_action_delegable
        CHECK (action IN ('SOURCE_DATA_AMENDMENT')),

    -- 镜像 domain.DelegatorKind。
    CONSTRAINT contract_delegation_delegator_kind_closed
        CHECK (delegator_kind IN ('CUSTOMER_ACCOUNT', 'LEGAL_ENTITY')),

    CONSTRAINT contract_delegation_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);

-- 裁定按（租户+范围+时点）装载当时有效的委派，与 authorization_grant_by_scope 同形。
CREATE INDEX contract_delegation_by_scope
    ON party_commercial.contract_delegation
        (tenant_id, scope_ref, effective_starts_at);
