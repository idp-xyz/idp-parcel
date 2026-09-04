-- 采用账长版本链（ADR-0117 决定二，票 label-channel-service-first-release/24）。
--
-- 同来源更正版本在采用口不再被当作第二责任起点（AT-PS-049 那一格），而是形成新的采用判断
-- 版本（AT-PS-050）：新行回指它取代的那一版同种类来源，原行一字不动——只插不改。
-- 「当前责任起点」是链尾（没有任何行回指它的那一行 adopted），按回指派生，不存 current
-- 列：存一列就得 UPDATE 旧行，与「原责任判断与承诺历史不删」相违；transport_fulfillment
-- 的 offsite_pickup（迁移 0015）已用同一形状走通一次。
--
-- 三列同在或同缺：更正形成的采用带被取代版本、承诺前版与调整原因（领域
-- RestateOnCorrectedIntake 的产物），根采用与不采用行都不带。被取代版本以自引用外键指回
-- 同（租户+包裹+来源种类）下的那一行：链不跨来源种类、不指向不存在的版本，这两句在库面
-- 由外键说而不靠编排记得。
--
-- 原部分唯一索引「每（租户+包裹）至多一行 adopted」换成两条：根唯一守责任起点唯一
-- （AT-PS-049 仍在库面，第二个根撞墙）；同一前版至多被取代一次守链线性（并发第二个更正
-- 撞墙）。两处撞墙都照 ON CONFLICT DO NOTHING 译成`已有记录`（ADR-0031），由重试方经
-- FindResponsibilityStart 读到链尾收敛。
--
-- CHECK 过 SQL 三值逻辑那一眼（0004 头注的纪律）：可空列先 IS NULL / IS NOT NULL，adopted
-- 是 NOT NULL 布尔。

ALTER TABLE parcel_shipment.intake_adoption
    ADD COLUMN supersedes_source_version    text,
    ADD COLUMN commitment_prior_version     text,
    ADD COLUMN commitment_adjustment_reason text;

ALTER TABLE parcel_shipment.intake_adoption
    ADD CONSTRAINT intake_adoption_supersession_coherent
        CHECK (
            (supersedes_source_version IS NULL
                AND commitment_prior_version IS NULL
                AND commitment_adjustment_reason IS NULL)
            OR (adopted
                AND supersedes_source_version IS NOT NULL AND btrim(supersedes_source_version) <> ''
                AND supersedes_source_version <> source_version
                AND commitment_prior_version IS NOT NULL AND btrim(commitment_prior_version) <> ''
                AND commitment_version IS NOT NULL
                AND commitment_prior_version <> commitment_version
                AND commitment_adjustment_reason IS NOT NULL AND btrim(commitment_adjustment_reason) <> '')
        );

-- 被取代版本必须是同（租户+包裹+来源种类）下确实登过的一版：链不跨种类，也不悬空。
ALTER TABLE parcel_shipment.intake_adoption
    ADD CONSTRAINT intake_adoption_supersedes_a_registered_version
        FOREIGN KEY (tenant_id, parcel_id, source_kind, supersedes_source_version)
        REFERENCES parcel_shipment.intake_adoption (tenant_id, parcel_id, source_kind, source_version);

DROP INDEX parcel_shipment.intake_adoption_responsibility_start;

-- 根唯一：先合法形成的责任起点每包裹只有一个。
CREATE UNIQUE INDEX intake_adoption_responsibility_start
    ON parcel_shipment.intake_adoption (tenant_id, parcel_id)
    WHERE adopted AND supersedes_source_version IS NULL;

-- 链线性：同一版本至多被取代一次。
CREATE UNIQUE INDEX intake_adoption_supersedes_once
    ON parcel_shipment.intake_adoption (tenant_id, parcel_id, source_kind, supersedes_source_version)
    WHERE adopted AND supersedes_source_version IS NOT NULL;
