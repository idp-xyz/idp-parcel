-- 申请人维进库（切块 (c)）：索赔项长出申请人一格，申请人授权目录长出登记面
-- （`PAR-VIS-08` 的又一角，接 0011 的合同覆盖那一角）。
--
-- claim_item.applicant_ref 可空：本迁移之前受理的存量行没有申请人，重建门容缺、
-- 授权维对它如实答核不了，出路是按正确申请人重提。新受理一律带申请人——那道门在
-- 领域受理口，库面只守「带了就不许是空串」。判断列（screen 等）在整行更新时重写，
-- 本列是提交事实，与其余事实列一样不进更新集。

ALTER TABLE visibility_exception.claim_item
    ADD COLUMN applicant_ref text,
    ADD CONSTRAINT claim_item_applicant_not_blank
        CHECK (applicant_ref IS NULL OR btrim(applicant_ref) <> '');

-- 申请人授权目录：某客户账户授权哪些申请人提出索赔。`AT-VE-125` 把申请人授权与客户
-- 账户并列——账户是索赔归属的货主客户，申请人是操作提交的那一方，两者不是一回事。
--
-- 拆「目录」与「名单成员」两张表，照 0011 的 claim_contract_scope / claim_covered_kind
-- 先例：只有目录行在场，「不在名单里」才说得通。没有目录行时缺一个申请人是「还没
-- 登记」（实例半边，视图答未登记，编排停在未决），不是「不获授权」；目录在场而申请人
-- 不在名单里，核出的是「不匹配」，按 AT-VE-125 落「不受理」——不是 ADR-0051 的永久
-- 格，名单换版后照常再审。名单内容是租户登记的实例参数，不造默认行。

CREATE TABLE visibility_exception.claim_authorization_catalogue (
    tenant_id    text NOT NULL,
    customer_ref text NOT NULL,
    rule_version text NOT NULL,
    approved_by  text NOT NULL,

    CONSTRAINT claim_authorization_catalogue_pkey
        PRIMARY KEY (tenant_id, customer_ref),

    CONSTRAINT claim_authorization_catalogue_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_ref) <> ''
            AND btrim(rule_version) <> ''
            AND btrim(approved_by) <> ''
        )
);

CREATE TABLE visibility_exception.claim_authorized_applicant (
    tenant_id     text NOT NULL,
    customer_ref  text NOT NULL,
    applicant_ref text NOT NULL,

    CONSTRAINT claim_authorized_applicant_pkey
        PRIMARY KEY (tenant_id, customer_ref, applicant_ref),

    -- 名单行必须挂在已登记的目录下（理由同 0011）：孤立名单行会让「目录在场」这个
    -- 前提失真，而「不匹配 → 不受理」完全建立在它之上。
    CONSTRAINT claim_authorized_applicant_catalogue_fkey
        FOREIGN KEY (tenant_id, customer_ref)
        REFERENCES visibility_exception.claim_authorization_catalogue (tenant_id, customer_ref)
        ON DELETE RESTRICT,

    CONSTRAINT claim_authorized_applicant_not_blank
        CHECK (btrim(applicant_ref) <> '')
);
