-- 交接登记册与计划版本签发器：UC-NR-001 步骤 2 的重放/冲突分界，以及本上下文自有的
-- 计划版本标识来源。
--
-- route_handoff_log 主键取（租户+交接关联）：同一关联只登一份指纹，再来的读回已登的那
-- 份比对——同指纹是重放、异指纹是冲突。行只增不改，换写会把一次冲突悄悄变成重放，所以
-- 适配器一律 ON CONFLICT DO NOTHING，不给 DO UPDATE 留路。
--
-- digest 只校验非空，不校验长度或字符集。指纹算法属应用层（create_initial_route.go 的
-- handoffDigest），在库里钉死它的形状等于把算法复制成第二处定义——换算法那天迁移先炸，
-- 而库其实并不关心指纹长什么样，只关心两次交接的指纹相不相等。

CREATE TABLE network_routing.route_handoff_log (
    tenant_id       text        NOT NULL,
    correlation_id  text        NOT NULL,
    digest          text        NOT NULL,
    appended_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT route_handoff_log_pkey
        PRIMARY KEY (tenant_id, correlation_id),

    CONSTRAINT route_handoff_log_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(correlation_id) <> ''
            AND btrim(digest) <> ''
        )
);

-- 计划版本签发器。序列而不是表：nextval 不随事务回滚，因此两个并发判断即便其中一个
-- 最终未提交，也拿不到同一个版本号——而 plan_applicability 以 plan_id 单列作主键，
-- 正是把「版本标识全局唯一」整个压在签发方身上。
--
-- 序列不带租户维：NextRoutePlanVersionID 的签名里没有租户，全局单调比按租户分段更强，
-- 也不需要为新租户预先开号段。
CREATE SEQUENCE network_routing.route_plan_version_seq
    AS bigint
    START WITH 1
    INCREMENT BY 1
    NO CYCLE;
