-- 证据项与证据披露版本（`UC-VE-007` 步 2）。在此之前 SubmitEvidence 与 PrepareDisclosure
-- 没有生产调用方：索赔编排有索赔项与追偿，证据项没有落点。
--
-- 证据项是被引的本体：「同一证据项可以被异常案件、客户索赔项和追偿事项分别引用」
-- （CONTEXT），引用在引用方身上，本表不带案件或索赔键——带了就把「谁引用它」写进本体，
-- 受控复用（`AT-VE-147`）就成了复制。它与 0021 的材料归集面是两回事：那边登的是某项索赔
-- 的某件要求材料收讫了，供最低材料维作差；这边登的是材料本体的证据评价与对外披露版本。
--
-- 行上只登来源、提供方、取得时间、内容指纹与评价——不登材料实体（敏感实例外置，仓库只
-- 登脱敏引用）。评价起点是 RECEIVED（「材料被提交不表示其内容已经被认定为事实」），经
-- 调查的采信/不采信必带依据，RECEIVED 必不带——与 domain.EvidenceItem 的构造门同一条线。
CREATE TABLE visibility_exception.evidence_item (
    tenant_id       text        NOT NULL,
    evidence_id     text        NOT NULL,

    provider_ref    text        NOT NULL,
    content_digest  text        NOT NULL,
    submitted_at    timestamptz NOT NULL,
    appraisal       text        NOT NULL,
    appraisal_basis text,

    CONSTRAINT evidence_item_pkey PRIMARY KEY (tenant_id, evidence_id),

    -- 同一提供方再次提交同一份材料是同一证据项；另一提供方提交同样的字节是另一项——
    -- 证据项保存来源与提供方，来源不同就不是同一份证据。写入口撞这条交回 AlreadyRecorded。
    CONSTRAINT evidence_item_one_per_provider_digest
        UNIQUE (tenant_id, provider_ref, content_digest),

    CONSTRAINT evidence_item_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(evidence_id) <> ''
            AND btrim(provider_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT evidence_item_appraisal_closed_set
        CHECK (appraisal IN ('RECEIVED', 'CREDITED', 'DISCREDITED')),

    CONSTRAINT evidence_item_basis_pairs_with_appraisal
        CHECK (
            (appraisal = 'RECEIVED' AND appraisal_basis IS NULL)
            OR (appraisal <> 'RECEIVED' AND appraisal_basis IS NOT NULL AND btrim(appraisal_basis) <> '')
        )
);

-- 披露版本：明确披露范围或脱敏版本，锚定原件指纹（CONTEXT「对外披露必须形成明确披露
-- 范围或脱敏版本，不能复制出来源不明、内容不一致的附件」）。按（证据项 + 脱敏指纹）成行、
-- 只增不改：同一原件的每个脱敏版本各占一行。脱敏指纹不得与原件相同——相同即原件外流，
-- 范围声明成了空话。行上只登准备完成：对外提交、送达与对方确认是追偿动作或通知那一侧
-- 分别记录的节点（`AT-VE-132`）。
CREATE TABLE visibility_exception.evidence_disclosure_version (
    tenant_id       text        NOT NULL,
    evidence_id     text        NOT NULL,
    redacted_digest text        NOT NULL,

    original_digest text        NOT NULL,
    scope           text        NOT NULL,
    prepared_at     timestamptz NOT NULL,

    CONSTRAINT evidence_disclosure_version_pkey
        PRIMARY KEY (tenant_id, evidence_id, redacted_digest),

    CONSTRAINT evidence_disclosure_version_item_fkey
        FOREIGN KEY (tenant_id, evidence_id)
        REFERENCES visibility_exception.evidence_item (tenant_id, evidence_id)
        ON DELETE RESTRICT,

    CONSTRAINT evidence_disclosure_version_not_blank
        CHECK (
            btrim(redacted_digest) <> ''
            AND btrim(original_digest) <> ''
            AND btrim(scope) <> ''
        ),

    CONSTRAINT evidence_disclosure_version_redacted_differs
        CHECK (redacted_digest <> original_digest)
);
