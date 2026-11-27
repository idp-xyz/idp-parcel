-- 服务产品的给人看的名称，和适用范围的给人看的叫法。
--
-- 两张表都不进商业版本的内容摘要。摘要盖的是正文（今天是可缺的交付条件）；把名称塞进摘要会让
-- 已经发布、正文为空的每一版都变成另一个摘要。名称随这一版落一行，页面不就地改名：要换叫法就发新版，
-- 新版自己再落一行。范围叫法是范围这一串字的名册，不是版本正文：同一范围被产品、合同、价格引用，
-- 叫法只登记一次。没登记叫法就是未配置，本迁移不种任何行。

CREATE TABLE party_commercial.service_product_display_name (
    tenant_id     text NOT NULL,
    object_kind   smallint NOT NULL,
    object_id     text NOT NULL,
    version_label text NOT NULL,
    name          text NOT NULL,

    CONSTRAINT service_product_display_name_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT service_product_display_name_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT service_product_display_name_not_blank
        CHECK (btrim(name) <> '' AND btrim(object_id) <> '' AND btrim(version_label) <> '')
);

CREATE TABLE party_commercial.commercial_scope_name (
    tenant_id text NOT NULL,
    scope_ref text NOT NULL,
    name      text NOT NULL,

    CONSTRAINT commercial_scope_name_pkey
        PRIMARY KEY (tenant_id, scope_ref),

    CONSTRAINT commercial_scope_name_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(scope_ref) <> '' AND btrim(name) <> '')
);
