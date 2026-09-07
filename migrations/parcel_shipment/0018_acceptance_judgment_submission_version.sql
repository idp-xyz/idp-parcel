-- 判断账长出提交版本维（ADR-0045 Consequences 预告的后续项；票 first-tenant-runway/10 裁甲）。
--
-- 0005 把可达性判断与财务控制的主键取成（租户+委托+[成员]+时点），头注自陈「没有 submission_version
-- 列……多版本之后旧版本的判断会被读进新版本的决定」。ADR-0106 之后这一格第一次有了生产路径：受控
-- 补充形成新版本 → 「新提交版本已形成」信封 → 同一条链拿新版本再驱一拍。若规则包声明的时点策略对
-- 两版给出同一时点（「以首次提交时刻为时点」是合法声明），新版本的判断撞旧键被 ON CONFLICT DO NOTHING
-- 吞掉，形成决定读回的仍是旧版的`证据不足`，链再次停等补充——客户补了件却推不动它。
--
-- 版本进主键、读口按当前版本取：CONTEXT「每份已提交委托必须针对**当前提交版本**形成或续办独立的接受
-- 判断任务」「旧版本及其判断历史继续保留」——旧版的判断是对旧内容作出的，留在行里作历史，版本列钉住
-- 它属于哪一版，不参与当前版本的决定。ON CONFLICT DO NOTHING 的语义不变：同版本同成员同时点仍是重放。
--
-- ADD COLUMN NOT NULL 无默认（照 visibility_exception 0003 / 0014）：测试库与 CI 走到这里表是空的
-- （尚无租户，判断行只可能是隔离合成 `S`）；有行却填不出版本就让迁移失败，禁止代填——一个猜出来的
-- 版本号会让一份旧判断被读进它从未判过的那一版。
--
-- 处理尝试表与采用解析表不动：续办引用的派生已含提交版本，尝试不参与决定；采用解析每轮按版本重解、
-- 后写覆盖，读回的恒是本轮那次。

ALTER TABLE parcel_shipment.acceptance_reachability_judgment
    ADD COLUMN submission_version text NOT NULL,
    ADD CONSTRAINT acceptance_reachability_judgment_version_not_blank
        CHECK (btrim(submission_version) <> ''),
    DROP CONSTRAINT acceptance_reachability_judgment_pkey,
    ADD CONSTRAINT acceptance_reachability_judgment_pkey
        PRIMARY KEY (tenant_id, shipment_request_id, submission_version, parcel_id, as_of_at);

ALTER TABLE parcel_shipment.acceptance_financial_control
    ADD COLUMN submission_version text NOT NULL,
    ADD CONSTRAINT acceptance_financial_control_version_not_blank
        CHECK (btrim(submission_version) <> ''),
    DROP CONSTRAINT acceptance_financial_control_pkey,
    ADD CONSTRAINT acceptance_financial_control_pkey
        PRIMARY KEY (tenant_id, shipment_request_id, submission_version, as_of_at);
