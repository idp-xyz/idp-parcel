-- 申报就绪判断与提交授权：CONTEXT 244「提交授权与就绪判断分别形成和失效」的持久化
-- 面。两张表刻意不合一：合成一张就只剩一条轨，而失效必须能分别发生——就绪还在、授权
-- 已撤销是真实且必须表达得出的一格。
--
-- 三态由行的在场与 revoked_* 两列共同表达：无行=未配置（实例半边未登记，读口交回
-- found=false，编排停在未决）；有行且 revoked_at IS NULL=仍有效；有行且已撤销=`不再
-- 就绪`／`授权已失效`。撤销不是删除（CONTEXT 243/生命周期）——原依据与形成时间原样留
-- 在行内，因此没有 DELETE 路径，读口也不必把失效谎报成未配置。
--
-- 内容（哪个单元就绪、谁授的权）属实例半边；表与读口属机制半边，先于租户存在。

CREATE TABLE customs_compliance.readiness_judgment (
    tenant_id       text        NOT NULL,
    unit_id         text        NOT NULL,

    basis_ref       text        NOT NULL,
    judged_at       timestamptz NOT NULL,
    revoked_by      text,
    revoked_at      timestamptz,

    CONSTRAINT readiness_judgment_pkey
        PRIMARY KEY (tenant_id, unit_id),

    CONSTRAINT readiness_judgment_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(unit_id) <> ''
            AND btrim(basis_ref) <> ''
        ),
    -- 撤销的两列同生同灭：只有原因没有时间的「半个失效」在领域里构造不出来
    -- （Revoke 两者缺一即 ErrInvalidReadiness），库内再守一遍。
    CONSTRAINT readiness_judgment_revocation_paired
        CHECK (
            (revoked_by IS NULL) = (revoked_at IS NULL)
            AND (revoked_by IS NULL OR btrim(revoked_by) <> '')
        ),
    -- 失效不能早于形成：早于形成时间的撤销讲不出「提交前适用条件变化」这件事。
    CONSTRAINT readiness_judgment_revoked_after_judged
        CHECK (revoked_at IS NULL OR revoked_at >= judged_at)
);

CREATE TABLE customs_compliance.submission_authority (
    tenant_id       text        NOT NULL,
    unit_id         text        NOT NULL,

    authority_ref   text        NOT NULL,
    granted_at      timestamptz NOT NULL,
    revoked_by      text,
    revoked_at      timestamptz,

    CONSTRAINT submission_authority_pkey
        PRIMARY KEY (tenant_id, unit_id),

    CONSTRAINT submission_authority_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(unit_id) <> ''
            AND btrim(authority_ref) <> ''
        ),
    CONSTRAINT submission_authority_revocation_paired
        CHECK (
            (revoked_by IS NULL) = (revoked_at IS NULL)
            AND (revoked_by IS NULL OR btrim(revoked_by) <> '')
        ),
    CONSTRAINT submission_authority_revoked_after_granted
        CHECK (revoked_at IS NULL OR revoked_at >= granted_at)
);
