-- 价卡草稿册（ADR-0101 决定三；票 price-card-import/03）：一次导入在本上下文立下的一份草稿，键 =（租户 + 方案 +
-- 方案版本），一版一行。它不是价卡版本——不进版本清单，任何评价读不到它；与 price_card_version 分表且不设外键：
-- 发布之前那一册里还没有这一版，发布之后草稿只是历史痕迹，权威记录是 price_card_version 那一行。
--
-- 这是本上下文唯一一张就地更新的表：`已批准`之前换内容替换那一行（内容、录入者与录入时刻随之更新），批准与发布
-- 推进 status 并补各自的痕迹。理由同 ADR-0126 决定三——草稿不是权威记录，权威记录那一册照旧只插不改。
--
-- 状态四格对齐 CONTEXT 生命周期（spec 自决第 1 格）：DRAFT 只在 content_document 里记逐格问题，不带方案；VALIDATED
-- 起带方案快照与方向授权引用，规范化版本、内容摘要与授权引用另上列作比对；APPROVED 起带批准者与批准时刻，PUBLISHED
-- 再带发布时刻。CHECK 是领域 RehydratePriceCardDraft 那几条不变量在库上的镜像；权威内容在 content_document，读回经
-- 领域整图重验（含按规范化版本重算内容摘要自校），再与比对列交叉核。
--
-- 源文件只登名称与 SHA-256：外置证据库的连接器尚无（spec 自决第 3 格）。行属实例半边：真实录入由租户的操作者发起，
-- 本迁移不种任何行。

CREATE TABLE parcel_pricing.price_card_draft (
    tenant_id             text        NOT NULL,
    plan_id               text        NOT NULL,
    plan_version          text        NOT NULL,
    status                text        NOT NULL,

    source_file_name      text        NOT NULL,
    source_file_sha256    text        NOT NULL,

    -- 已校验起才有。摘要只在同一规范化版本内可比（ADR-0014）。
    canonicalization      text,
    content_digest        text,
    authorization_id      text,
    authorization_version text,

    content_document      jsonb       NOT NULL,

    submitter             text        NOT NULL,
    submitted_at          timestamptz NOT NULL,
    approver              text,
    approved_at           timestamptz,
    published_at          timestamptz,

    CONSTRAINT price_card_draft_pkey
        PRIMARY KEY (tenant_id, plan_id, plan_version),

    CONSTRAINT price_card_draft_status_closed
        CHECK (status IN ('DRAFT', 'VALIDATED', 'APPROVED', 'PUBLISHED')),

    CONSTRAINT price_card_draft_sha256_shape
        CHECK (source_file_sha256 ~ '^[0-9a-f]{64}$'),

    CONSTRAINT price_card_draft_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(plan_id) <> ''
            AND btrim(plan_version) <> ''
            AND btrim(source_file_name) <> ''
            AND btrim(submitter) <> ''
        ),

    CONSTRAINT price_card_draft_document_is_object
        CHECK (jsonb_typeof(content_document) = 'object'),

    -- 内容比对列只在已校验起在场，四列同进同出。NULL 让 CHECK 放行，所以在场那一支逐列写 IS NOT NULL。
    CONSTRAINT price_card_draft_content_columns
        CHECK (
            (status = 'DRAFT'
                AND canonicalization IS NULL AND content_digest IS NULL
                AND authorization_id IS NULL AND authorization_version IS NULL)
            OR (status <> 'DRAFT'
                AND canonicalization IS NOT NULL AND btrim(canonicalization) <> ''
                AND content_digest IS NOT NULL AND btrim(content_digest) <> ''
                AND authorization_id IS NOT NULL AND btrim(authorization_id) <> ''
                AND authorization_version IS NOT NULL AND btrim(authorization_version) <> '')
        ),

    -- 批准痕迹自已批准起在场且不早于录入；之前不得带。
    CONSTRAINT price_card_draft_approval_traces
        CHECK (
            (status IN ('DRAFT', 'VALIDATED') AND approver IS NULL AND approved_at IS NULL)
            OR (status IN ('APPROVED', 'PUBLISHED')
                AND approver IS NOT NULL AND btrim(approver) <> ''
                AND approved_at IS NOT NULL AND approved_at >= submitted_at)
        ),

    -- 发布时刻只在已发布在场且不早于批准。
    CONSTRAINT price_card_draft_publication_trace
        CHECK (
            (status <> 'PUBLISHED' AND published_at IS NULL)
            OR (status = 'PUBLISHED' AND published_at IS NOT NULL AND published_at >= approved_at)
        )
);
