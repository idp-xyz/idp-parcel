-- 票 catalogue-read-pagination-rollout/13：操作者册迁到 ADR-0144 之后按租户、以「登记时刻 + 行标识」取页，补一条与缺省序同形的
-- 索引（ADR-0144 决定七）。行标识是（发行方、主体），即 operator_pkey 那两列；那条主键不以租户打头，按租户翻页用不上它。
-- 作业设备册按设备标识取页，缺省序与行标识都是设备标识，与 device_pkey（租户、设备标识）同形，不另建。

CREATE INDEX operator_by_tenant_recorded
    ON access_identity.operator (tenant_id, recorded_at, issuer, subject);
