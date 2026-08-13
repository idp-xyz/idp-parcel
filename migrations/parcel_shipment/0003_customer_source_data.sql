-- 客户原始资料版本登记册：版本一经形成不可覆盖，只追加。
--
-- 键=委托的完整来源身份+版本标识；版本链（基准/前版引用）与内容指纹平铺成列供查，
-- 其余留痕清单（范围、意图、原因、请求方、决定方、授权快照、适用/形成时间）进 jsonb
-- ——查询按链走，读回逐字段过领域构造函数（与委托聚合快照同一条纪律）。

CREATE TABLE parcel_shipment.customer_source_data_version (
    tenant_id           text        NOT NULL,
    customer_account_id text        NOT NULL,
    source              text        NOT NULL,
    source_request_key  text        NOT NULL,
    version_id          text        NOT NULL,

    shipment_request_id text        NOT NULL,
    on_baseline         boolean     NOT NULL,
    prior_version_id    text,
    payload_digest      text        NOT NULL,
    snapshot            jsonb       NOT NULL,
    formed_at           timestamptz NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT customer_source_data_version_pkey
        PRIMARY KEY (tenant_id, customer_account_id, source, source_request_key, version_id),

    CONSTRAINT customer_source_data_version_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(source) <> ''
            AND btrim(source_request_key) <> ''
            AND btrim(version_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(payload_digest) <> ''
        ),
    -- 基准两形态互斥：以接受基线为基准即无前版，指名前版即不在基线上。半截的一行
    -- （都缺或都有）在领域是立不起来的基准，让数据库同样拦住。
    CONSTRAINT customer_source_data_version_basis_coherent
        CHECK (
            (on_baseline AND prior_version_id IS NULL)
            OR (NOT on_baseline AND prior_version_id IS NOT NULL AND btrim(prior_version_id) <> '')
        ),
    -- 前版不指自己：自指的版本链在第一环就绕回。
    CONSTRAINT customer_source_data_version_no_self_reference
        CHECK (prior_version_id IS NULL OR prior_version_id <> version_id)
);

-- 按委托取回版本链，顺序稳定。
CREATE INDEX customer_source_data_version_by_request
    ON parcel_shipment.customer_source_data_version
        (tenant_id, customer_account_id, shipment_request_id, formed_at, version_id);
