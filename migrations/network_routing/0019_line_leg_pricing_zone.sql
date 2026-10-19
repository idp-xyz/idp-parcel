-- 段计价区域（ADR-0189 决定二；票 routing-first-cut/18）：租户为一条线路版本的某一段登记的价卡区域。这一段所引的
-- 价卡没绑分区目录时，候选成本按这个区计；绑了目录的卡由包裹邮编路线经目录读出区域，不读这一行（ADR-0189 决定一）。
--
-- 区域词归 parcel-pricing，本表原样持有、不解释；区不在卡上时由计价答待判断，这里不校。外键指向成本依据行：区域
-- 只对挂了依据的段有意义——它说的是「按那张卡计价时用哪个区」。与成本依据一样不进目录修订，也不随线路版本内容走：
-- 已采用而未闭区间的线路版本改不了内容，区域另行登记才登得上。登记只增不改，同段再登撞主键。

CREATE TABLE network_routing.line_leg_pricing_zone (
    tenant_id     text        NOT NULL,
    line_code     text        NOT NULL,
    version       integer     NOT NULL,
    segment_index integer     NOT NULL,

    zone          text        NOT NULL,
    basis_ref     text        NOT NULL,
    registered_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT line_leg_pricing_zone_pkey
        PRIMARY KEY (tenant_id, line_code, version, segment_index),
    CONSTRAINT line_leg_pricing_zone_not_blank
        CHECK (btrim(zone) <> '' AND btrim(basis_ref) <> ''),
    CONSTRAINT line_leg_pricing_zone_on_a_cost_basis
        FOREIGN KEY (tenant_id, line_code, version, segment_index)
        REFERENCES network_routing.line_cost_basis (tenant_id, line_code, version, segment_index)
);
