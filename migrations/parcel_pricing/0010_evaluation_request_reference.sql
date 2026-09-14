-- 评价回指评价请求（票 sa-cc/11 裁决 2）：一份评价可以是为 settlement-accounting 的哪一份评价请求而形成的。
-- 可空——PP 内部路径、重放与测试形成的评价没有它。
--
-- 它是**检索列面**不是权威内容：权威在 evaluation.snapshot 的 requestReference 一格，读回评价对象只经快照装载与
-- 整图重验，本列供「按（租户、评价请求）取那一份评价」那一问，读回后与快照交叉核——列与快照分岔说明行被改过。
-- 回指不进语义摘要（同一评价带不带回指语义摘要逐字相同），所以它不能折进 semantic_digest 那一列，只能另立。
--
-- 同租户同请求只形成一份评价（票面做法 3「同一请求不形成第二份评价」）：部分唯一索引在库上守。它与评价标识主键
-- 一道构成 INSERT … ON CONFLICT DO NOTHING 的第二个撞点——适配器照旧译成 `已有记录`，编排按回指读回先到者作答，
-- 不覆盖、不顶替。
--
-- 不回填：本仓尚无租户，生产库里没有评价；迁移之前落册的隔离评价没有回指，读回回指为空。

ALTER TABLE parcel_pricing.evaluation
    ADD COLUMN request_reference text;

ALTER TABLE parcel_pricing.evaluation
    ADD CONSTRAINT evaluation_request_reference_not_blank
        CHECK (request_reference IS NULL OR btrim(request_reference) <> '');

CREATE UNIQUE INDEX evaluation_one_per_request
    ON parcel_pricing.evaluation (tenant_id, request_reference)
    WHERE request_reference IS NOT NULL;
