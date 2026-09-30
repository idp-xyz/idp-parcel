-- 可达性判断记下形成时的商业解析（ADR-0156 决定五）。
--
-- 0018 的主键是（租户+委托+提交版本+成员+时点）。「提交接收」对同一版本给出同一时点，
-- 换一次解析形成的新判断撞上旧键，ON CONFLICT DO NOTHING 把它吞掉，决定仍读旧判断，
-- 配的却是新解析。解析进主键：同一时点上不同解析各占一行；同一解析再来仍是重放。
--
-- ADD COLUMN NOT NULL 无默认：只有 ReachabilityAssessed 才记行。本系列之前闭包标识为 nil，
-- 可达性从未形成判断，任何环境走到这里都没有存量行。有行却填不出形成时的解析就让迁移失败，禁止代填。
-- 空串也不许：它会在同一时点上变成「没有解析」的那一行，决定再拿它去配新解析。

ALTER TABLE parcel_shipment.acceptance_reachability_judgment
    ADD COLUMN resolution_id text NOT NULL,
    ADD CONSTRAINT acceptance_reachability_judgment_resolution_not_blank
        CHECK (btrim(resolution_id) <> ''),
    DROP CONSTRAINT acceptance_reachability_judgment_pkey,
    ADD CONSTRAINT acceptance_reachability_judgment_pkey
        PRIMARY KEY (
            tenant_id,
            shipment_request_id,
            submission_version,
            parcel_id,
            as_of_at,
            resolution_id
        );
