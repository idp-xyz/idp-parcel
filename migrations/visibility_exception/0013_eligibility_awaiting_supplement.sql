-- 资格审核第三态（ADR-0051）：`等待补充` 进入 screen 封闭集；四件落点（缺少材料、
-- 补充范围、通知依据、当前截止时间）随索赔项；补充期限版本只追加不覆盖。
--
-- 不改 0004 的历史正文。旧 CHECK 把 screen 钉在二值上，与 CONTEXT 生命周期三岔
-- 冲突；本迁移替换那条约束，并补上等待补充必带四件、终局格必不带四件。
--
-- 期限历史另表：获批延期形成新版本，原期限必须还能被读到。写进索赔行的当前
-- 截止只是最新一版的冗余，历史以本表为准。

ALTER TABLE visibility_exception.claim_item
    ADD COLUMN missing_materials_ref text,
    ADD COLUMN supplement_scope_ref  text,
    ADD COLUMN supplement_notice_ref text,
    ADD COLUMN supplement_deadline   timestamptz;

ALTER TABLE visibility_exception.claim_item
    DROP CONSTRAINT claim_item_screen_shape;

ALTER TABLE visibility_exception.claim_item
    ADD CONSTRAINT claim_item_screen_shape
        CHECK (
            ((screen IS NULL) = (screen_basis IS NULL))
            AND (screen IS NULL OR screen IN ('ELIGIBLE', 'INELIGIBLE', 'AWAITING_SUPPLEMENT'))
            AND (screen_basis IS NULL OR btrim(screen_basis) <> '')
        );

-- 等待补充 ↔ 四件落点同在；终局或不审 ↔ 四件皆空。可空列使用前先 IS NULL /
-- IS NOT NULL。空串不是「没有缺少材料」——与「签发了空引用」会分不开。
ALTER TABLE visibility_exception.claim_item
    ADD CONSTRAINT claim_item_supplement_shape
        CHECK (
            (
                screen IS NOT DISTINCT FROM 'AWAITING_SUPPLEMENT'
                AND missing_materials_ref IS NOT NULL AND btrim(missing_materials_ref) <> ''
                AND supplement_scope_ref  IS NOT NULL AND btrim(supplement_scope_ref)  <> ''
                AND supplement_notice_ref IS NOT NULL AND btrim(supplement_notice_ref) <> ''
                AND supplement_deadline   IS NOT NULL
            )
            OR (
                screen IS DISTINCT FROM 'AWAITING_SUPPLEMENT'
                AND missing_materials_ref IS NULL
                AND supplement_scope_ref  IS NULL
                AND supplement_notice_ref IS NULL
                AND supplement_deadline   IS NULL
            )
        );

CREATE TABLE visibility_exception.claim_supplement_deadline (
    tenant_id      text        NOT NULL,
    batch_ref      text        NOT NULL,
    item_id        text        NOT NULL,
    version_seq    integer     NOT NULL,
    deadline       timestamptz NOT NULL,
    established_at timestamptz NOT NULL,

    CONSTRAINT claim_supplement_deadline_pkey
        PRIMARY KEY (tenant_id, batch_ref, item_id, version_seq),

    CONSTRAINT claim_supplement_deadline_claim_fkey
        FOREIGN KEY (tenant_id, batch_ref, item_id)
        REFERENCES visibility_exception.claim_item (tenant_id, batch_ref, item_id)
        ON DELETE RESTRICT,

    CONSTRAINT claim_supplement_deadline_seq_positive
        CHECK (version_seq >= 1),

    CONSTRAINT claim_supplement_deadline_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(batch_ref) <> ''
            AND btrim(item_id) <> ''
        )
);
