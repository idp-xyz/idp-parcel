-- 参与方资料登记册（票 business-party-profile/02；ADR-0369 决定一；PC CONTEXT「参与方资料」词条与诸句）。
--
-- 一行一笔修订，键 = 租户 + 往来单位 + 修订号，修订不可覆盖。正文各项都可缺，全部放在快照里：与法人资料修订表不同，这里
-- 没有一项是必填的、也没有一项要在库上单独比对，镜像列只会多一份要与快照保持一致的东西。往来单位在不在册、是不是我方公司、
-- 号合不合目录由写入用例判，库只钉单看一行就判得出的形状。

CREATE TABLE party_commercial.business_party_profile_revision (
    tenant_id      text        NOT NULL,
    party_id       text        NOT NULL,
    revision       integer     NOT NULL,

    basis_ref      text        NOT NULL,
    effective_from timestamptz NOT NULL,
    content_digest text        NOT NULL,
    snapshot       jsonb       NOT NULL,
    recorded_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT business_party_profile_revision_pkey
        PRIMARY KEY (tenant_id, party_id, revision),

    CONSTRAINT business_party_profile_revision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(party_id) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT business_party_profile_revision_revision_positive
        CHECK (revision >= 1),

    CONSTRAINT business_party_profile_revision_snapshot_object
        CHECK (jsonb_typeof(snapshot) = 'object')
);
