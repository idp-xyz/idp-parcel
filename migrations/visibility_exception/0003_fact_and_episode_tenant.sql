-- 给 0002 的三张表补租户维（ADR-0003：租户是最高数据隔离边界）。0002 落地时键无
-- 租户——VE 消费多个源上下文的事实，事实引用与发作期对象只在各自租户内唯一，缺这
-- 一维两个租户的同名引用会共用一行。
--
-- 已施加的迁移不可改写（校验和把关），故以新文件重建键。三张表此刻不可能有真实
-- 数据（库只在测试与 CI 的全新数据库上走到这里），ADD COLUMN NOT NULL 无需默认值
-- ——给它填默认租户反而是把隔离边界写成可猜的常量。

ALTER TABLE visibility_exception.accepted_fact
    ADD COLUMN tenant_id text NOT NULL,
    ADD CONSTRAINT accepted_fact_tenant_not_blank CHECK (btrim(tenant_id) <> ''),
    DROP CONSTRAINT accepted_fact_pkey,
    ADD CONSTRAINT accepted_fact_pkey
        PRIMARY KEY (tenant_id, source_context, fact_ref, fact_version);

DROP INDEX visibility_exception.accepted_fact_by_parcel;
CREATE INDEX accepted_fact_by_parcel
    ON visibility_exception.accepted_fact (tenant_id, parcel_ref, received_at);

-- 发作期标识由身份工厂签发、全局唯一，主键保持 episode_id；租户作 NOT NULL 列进
-- 每条语句的条件（含 SaveHit 的 UPDATE——另一个租户拿同名标识也改不动）。
ALTER TABLE visibility_exception.signal_episode
    ADD COLUMN tenant_id text NOT NULL,
    ADD CONSTRAINT signal_episode_tenant_not_blank CHECK (btrim(tenant_id) <> '');

DROP INDEX visibility_exception.signal_episode_latest;
CREATE INDEX signal_episode_latest
    ON visibility_exception.signal_episode (tenant_id, parcel_ref, kind_ref, started_at DESC, seq DESC);

ALTER TABLE visibility_exception.triage_conclusion
    ADD COLUMN tenant_id text NOT NULL,
    ADD CONSTRAINT triage_conclusion_tenant_not_blank CHECK (btrim(tenant_id) <> '');
