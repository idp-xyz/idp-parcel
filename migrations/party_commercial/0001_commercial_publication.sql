-- 商业发布登记册：已发布版本不可覆盖，只增不改。
--
-- 键=租户+对象类别+对象+版本号；生效区间平铺成列供范围查（选用区间的有效性更正
-- 另册另票，本表存版本自己声明的原区间）；内容摘要平铺供冲突判定；批准依据、指名
-- 引用与生命周期痕迹整份进 jsonb，读回逐字段过领域重建门。
--
-- 状态只收已发布之后的取值（2..6）：草稿不入册——它的正文还能改，而本表存在的
-- 理由正是「发布后正文不可覆盖」。

CREATE TABLE party_commercial.commercial_version (
    tenant_id           text        NOT NULL,
    object_kind         smallint    NOT NULL,
    object_id           text        NOT NULL,
    version_label       text        NOT NULL,

    scope_ref           text        NOT NULL,
    content_digest      text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,
    status              smallint    NOT NULL,
    snapshot            jsonb       NOT NULL,
    published_at        timestamptz NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commercial_version_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT commercial_version_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(content_digest) <> ''
        ),
    CONSTRAINT commercial_version_kind_known
        CHECK (object_kind BETWEEN 1 AND 7),
    -- 草稿不入册：2=PUBLISHED 3=EFFECTIVE 4=EXPIRED 5=RETIRED 6=SUPERSEDED。
    CONSTRAINT commercial_version_status_published
        CHECK (status BETWEEN 2 AND 6),
    -- 区间要么开放结束要么严格晚于开始——两边相等的区间什么时点都不含。
    CONSTRAINT commercial_version_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);

-- 解析按（租户+范围+时点）选候选：范围与区间起点的组合索引。
CREATE INDEX commercial_version_by_scope
    ON party_commercial.commercial_version
        (tenant_id, scope_ref, effective_starts_at);
