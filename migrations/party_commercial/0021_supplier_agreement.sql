-- 供应商商业协议册（票 party-commercial-context-gaps/03）：一行记一个协议版本的正文。
--
-- 与版本册分表，判据同 0010/0020：版本回答「有没有这份协议对象」，正文回答「与哪个供应商、
-- 为哪个责任法人、在哪个范围内绑定了哪份采购定价方案」。CONTEXT：协议「不等于一次实际
-- 运输委托、订舱、履约事实或供应商账单」——所以表上没有任何委托、履约或应付列，那些归
-- transport-fulfillment 与 settlement-accounting。
--
-- 没有方向列。domain.SupplierAgreement.Direction 恒为 BUY，存一列常量等于为同一件事立第二个
-- 口径，读回来若不是 BUY 反倒要人判是坏数据还是新语义；类别 CHECK 已把「这是一份采购协议」
-- 钉住。
--
-- 没有终止列。终止（Terminate）是生效后的一次事件而不是发布时的正文，形状与有效性更正
-- （0009）同族——按版本追加、不改写原行；它要不要建册是另一裁，本表只登正文。
--
-- agreement_scope_ref 是协议自己的适用范围，与版本壳上的 scope_ref 分开存，判据同 0010 的
-- policy_scope_ref：范围是版本壳上的事实，正文的范围是正文的一部分。
--
-- object_kind CHECK = 3（SupplierAgreementObject）。形态只归供应商协议。
--
-- 本表不进整册装载（LoadForScope），消费方按已选中的版本点读（ports.SupplierAgreementContentView）。
-- 行属实例半边：真实协议由租户登记，今天没有租户因而本表为空。

CREATE TABLE party_commercial.supplier_agreement (
    tenant_id            text        NOT NULL,
    object_kind          smallint    NOT NULL,
    object_id            text        NOT NULL,
    version_label        text        NOT NULL,

    supplier_party_id    text        NOT NULL,
    legal_entity_ref     text        NOT NULL,
    agreement_scope_ref  text        NOT NULL,
    purchase_plan_ref    text        NOT NULL,
    effective_starts_at  timestamptz NOT NULL,
    effective_ends_at    timestamptz,
    registered_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT supplier_agreement_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT supplier_agreement_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT supplier_agreement_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(supplier_party_id) <> ''
            AND btrim(legal_entity_ref) <> ''
            AND btrim(agreement_scope_ref) <> ''
            AND btrim(purchase_plan_ref) <> ''
        ),

    CONSTRAINT supplier_agreement_supplier_only
        CHECK (object_kind = 3),

    CONSTRAINT supplier_agreement_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);
