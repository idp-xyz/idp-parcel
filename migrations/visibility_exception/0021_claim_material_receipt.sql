-- 索赔材料归集面：收讫登记行与撤销行（.scratch/ve-claims-read-seams/02）。在此之前
-- ClaimEvidenceView 无表可读，资格审核的最低材料维只能停在「归集无从查起」的未决。
-- 本迁移只建门：谁收到了什么材料属实例半边的事实，不造默认行。
--
-- 收讫行按（租户、索赔批次、索赔项、材料要求引用、收讫时间）五件成行。材料要求引用
-- 是资格目录签发的词——差集两边同词，缺口才核得出来；行上只登收讫事实与经手声明，
-- 不登材料内容实体：敏感材料实体外置，仓库只登脱敏引用（AGENTS 红线「敏感实例外置」）。
-- 同五件重登幂等（写入口 ON CONFLICT DO NOTHING）；同一材料再次收讫是新的收讫时间、
-- 另起一行。
--
-- 两张表都只追加，没有 UPDATE 与 DELETE 路径：撤销不改收讫行，另立撤销行。与
-- customs_compliance 0006「撤销不是删除」是同一条纪律的另一种形状——那边撤销列与
-- 形成行同表，这边收讫行连撤销列都不长，因为收讫是提交事实，事实列不进更新集
-- （0018 对 claim_item 的同款处置）。读侧口径是「收讫减撤销」：有收讫行且无对应
-- 撤销行才算在手。
CREATE TABLE visibility_exception.claim_material_receipt (
    tenant_id       text        NOT NULL,
    claim_batch_ref text        NOT NULL,
    claim_item_id   text        NOT NULL,
    material_ref    text        NOT NULL,
    received_at     timestamptz NOT NULL,

    received_by     text        NOT NULL,
    inserted_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT claim_material_receipt_pkey
        PRIMARY KEY (tenant_id, claim_batch_ref, claim_item_id, material_ref, received_at),

    CONSTRAINT claim_material_receipt_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(claim_batch_ref) <> ''
            AND btrim(claim_item_id) <> ''
            AND btrim(material_ref) <> ''
            AND btrim(received_by) <> ''
        )
);

-- 撤销行以收讫行的五件全键指名撤销对象：同一（批次+项+材料）可能收讫多次，缺收讫
-- 时间就指不清撤的是哪一次。一次收讫至多一行撤销（主键同五件）；撤销之后同一材料要
-- 再次采信，走新的收讫行，不设「取消撤销」路径——那会让减数时有时无，历史读不回。
CREATE TABLE visibility_exception.claim_material_receipt_revocation (
    tenant_id       text        NOT NULL,
    claim_batch_ref text        NOT NULL,
    claim_item_id   text        NOT NULL,
    material_ref    text        NOT NULL,
    received_at     timestamptz NOT NULL,

    revoked_by      text        NOT NULL,
    revoked_at      timestamptz NOT NULL,
    inserted_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT claim_material_receipt_revocation_pkey
        PRIMARY KEY (tenant_id, claim_batch_ref, claim_item_id, material_ref, received_at),

    -- 撤销必须指向在场的收讫行：孤立撤销行会让「收讫减撤销」的减数凭空多出来。收讫
    -- 行没有 DELETE 路径，本约束不会拦下任何正当次序。
    CONSTRAINT claim_material_receipt_revocation_receipt_fkey
        FOREIGN KEY (tenant_id, claim_batch_ref, claim_item_id, material_ref, received_at)
        REFERENCES visibility_exception.claim_material_receipt
            (tenant_id, claim_batch_ref, claim_item_id, material_ref, received_at)
        ON DELETE RESTRICT,

    CONSTRAINT claim_material_receipt_revocation_not_blank
        CHECK (btrim(revoked_by) <> ''),

    -- 撤销不能早于收讫：早于收讫时间的撤销讲不出「收下之后发现不能采信」这件事
    -- （形状循 customs_compliance 0006 的 revoked_after_judged）。
    CONSTRAINT claim_material_receipt_revocation_after_receipt
        CHECK (revoked_at >= received_at)
);
