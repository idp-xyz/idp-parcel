-- 来源保全：客户提交的原始事实一经保全即不可改写。
--
-- 分两张表而不是一张，是因为 SourceSubmissionRepository 的 AppendObservation
-- 明写「追加在已保全事实旁边，绝不替换它」。做成一行一身份再 UPSERT，第二次
-- 观察就会覆盖第一次的 occurredAt/receivedAt——那正是用例禁止的「不同的发生
-- 时间改写历史」。

CREATE TABLE parcel_shipment.source_submission (
    tenant_id           text        NOT NULL,
    customer_account_id text        NOT NULL,
    source              text        NOT NULL,
    source_request_key  text        NOT NULL,

    payload_digest      text        NOT NULL,
    occurred_at         timestamptz NOT NULL,
    received_at         timestamptz NOT NULL,
    preserved_at        timestamptz NOT NULL DEFAULT now(),

    -- 主键即完整来源身份。唯一性至少覆盖租户、客户账户、来源与来源请求键，
    -- 因此不同租户或不同客户账户下的相同外部键天然隔离。
    CONSTRAINT source_submission_pkey
        PRIMARY KEY (tenant_id, customer_account_id, source, source_request_key),

    CONSTRAINT source_submission_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(source) <> ''
            AND btrim(source_request_key) <> ''
        ),
    CONSTRAINT source_submission_digest_not_blank
        CHECK (btrim(payload_digest) <> '')
);

-- 重复观察只追加，不改写上表任何一列。同键同摘要是重放，同键不同摘要是冲突，
-- 两者都要留下痕迹，而冲突那一支尤其不能覆盖原内容。
CREATE TABLE parcel_shipment.source_submission_observation (
    observation_id      bigint      GENERATED ALWAYS AS IDENTITY,

    tenant_id           text        NOT NULL,
    customer_account_id text        NOT NULL,
    source              text        NOT NULL,
    source_request_key  text        NOT NULL,

    payload_digest      text        NOT NULL,
    occurred_at         timestamptz NOT NULL,
    received_at         timestamptz NOT NULL,
    appended_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT source_submission_observation_pkey
        PRIMARY KEY (observation_id),

    -- 观察只能挂在已保全的事实上：没有先保全就先追加观察，说明调用顺序错了，
    -- 让数据库拦住而不是留下一条无主记录。
    CONSTRAINT source_submission_observation_preserved_fkey
        FOREIGN KEY (tenant_id, customer_account_id, source, source_request_key)
        REFERENCES parcel_shipment.source_submission
            (tenant_id, customer_account_id, source, source_request_key),

    CONSTRAINT source_submission_observation_digest_not_blank
        CHECK (btrim(payload_digest) <> '')
);

-- 按来源身份取回一份事实的全部观察，顺序稳定。
CREATE INDEX source_submission_observation_by_identity
    ON parcel_shipment.source_submission_observation
        (tenant_id, customer_account_id, source, source_request_key, observation_id);
