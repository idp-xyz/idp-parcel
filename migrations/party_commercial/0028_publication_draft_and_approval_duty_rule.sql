-- 待批准发布载体与审批职责规则（ADR-0126 Decision 三；票 admin-write-faces/08）。
--
-- publication_draft：运营操作者面录入的一份商业版本在发布之前的载体，一版一行、键 = 版本身份四元。
-- 它不是商业版本——任何解析读不到它，与 commercial_version 分表且不设外键：发布之前那一册里还没有
-- 这一版；发布之后载体只是历史痕迹，权威记录是 commercial_version 那一行。
--
-- 这是本上下文唯一一张就地更新的表：`待批准`期间换内容替换行（正文、摘要、录入者与时刻随之更新），
-- 批准与发布推进 status 并补各自的痕迹。理由写在 ADR-0126 Decision 三——载体不是权威记录，权威记录
-- 那一册照旧只插不改。
--
-- 正文快照（content_document）就是服务端规范化文档（PCC-<n>）的字节：它恰是 content_digest 盖住的那些
-- 字节，快照与摘要是一样东西的两面；重建时折回正文再算一遍与列上摘要比。content_canonicalization 说
-- 文档按哪一版形状写，摘要串以它为前缀（`PCC-1:<hex>`，ADR-0014 摘要自带版本）。
--
-- 状态三格封闭（1 待批准 / 2 已批准 / 3 已发布），各格带且只带该带的痕迹：批准者与批准时刻自 2 起在场，
-- 发布时刻只在 3 在场。CHECK 是领域 RehydratePublicationDraft 那几条不变量在库上的镜像。
--
-- 行属实例半边：真实录入由租户的操作者发起，今天没有租户因而本表为空。

CREATE TABLE party_commercial.publication_draft (
    tenant_id                text        NOT NULL,
    object_kind              smallint    NOT NULL,
    object_id                text        NOT NULL,
    version_label            text        NOT NULL,

    scope_ref                text        NOT NULL,
    effective_starts_at      timestamptz NOT NULL,
    effective_ends_at        timestamptz,
    -- 壳上的指名引用：[{"kind": <smallint>, "objectId": <text>}]；空清单存 '[]'。
    declared_references      jsonb       NOT NULL DEFAULT '[]'::jsonb,

    content_canonicalization text        NOT NULL,
    content_digest           text        NOT NULL,
    content_document         jsonb       NOT NULL,

    submitter_ref            text        NOT NULL,
    submitted_at             timestamptz NOT NULL,
    status                   smallint    NOT NULL,
    approver_ref             text,
    approved_at              timestamptz,
    published_at             timestamptz,

    CONSTRAINT publication_draft_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT publication_draft_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(content_canonicalization) <> ''
            AND btrim(content_digest) <> ''
            AND btrim(submitter_ref) <> ''
        ),

    CONSTRAINT publication_draft_status_closed
        CHECK (status IN (1, 2, 3)),

    -- 摘要串以规范化版本为前缀（ADR-0014 在本上下文的落法）。
    CONSTRAINT publication_draft_digest_carries_canonicalization
        CHECK (left(content_digest, length(content_canonicalization) + 1) = content_canonicalization || ':'),

    CONSTRAINT publication_draft_references_is_array
        CHECK (jsonb_typeof(declared_references) = 'array'),

    CONSTRAINT publication_draft_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at),

    -- 批准痕迹自`已批准`起在场且不早于录入；`待批准`不得带。
    CONSTRAINT publication_draft_approval_traces
        CHECK (
            (status = 1 AND approver_ref IS NULL AND approved_at IS NULL)
            OR (status >= 2 AND approver_ref IS NOT NULL AND btrim(approver_ref) <> ''
                AND approved_at IS NOT NULL AND approved_at >= submitted_at)
        ),

    -- 发布时刻只在`已发布`在场且不早于批准。
    CONSTRAINT publication_draft_publication_trace
        CHECK (
            (status < 3 AND published_at IS NULL)
            OR (status = 3 AND published_at IS NOT NULL AND published_at >= approved_at)
        )
);

-- 审批职责规则（PAR-COM-18）：一租户一条——录入者与批准者须否为不同主体、批准者须持哪一格授予（可缺 = 不
-- 要求）。两格都不要求也是一条合法的租户声明；没有行就是未登记，批准门据此答`未配置`、不放行。
--
-- 规则是租户治理参数（实例半边）：今天没有治理写面调它，写口只给装配与测试用；登记面归治理写面那一族按
-- ADR-0085 决定四另裁（ADR-0126 Decision 五）。撞键不覆盖：改规则今天没有入口，等那一族的裁决再定按修订
-- 还是按替换。
CREATE TABLE party_commercial.publication_approval_duty_rule (
    tenant_id                  text        NOT NULL,
    requires_distinct_subjects boolean     NOT NULL,
    required_approver_level    text,
    registered_at              timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT publication_approval_duty_rule_pkey
        PRIMARY KEY (tenant_id),

    CONSTRAINT publication_approval_duty_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND (required_approver_level IS NULL OR btrim(required_approver_level) <> '')
        )
);
