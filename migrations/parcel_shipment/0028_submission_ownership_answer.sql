-- 提交判得的生产归属答案（票 docs-consistency/code/39）。
--
-- UC-PS-001「必须持久保留的业务记录」要留下接受流程前的生产归属结果；重发同一请求时，提交编排凭它分辨这份提交上一次走到了
-- 哪一步：委托已在（按委托号找建它所据的那一次）、已形成挡下的结果（准入暂停、已安全交给他方，答原归属结果），还是什么都没
-- 形成或归属未决（接着判归属与建单）。
--
-- 只追加：归属未决之后的重发会重判，每一次判断都留一行，后一次不覆盖前一次。决定整份存一列 jsonb，读回经领域构造函数重新
-- 校验，与委托快照同一种做法。
CREATE TABLE parcel_shipment.submission_ownership_answer (
    answer_id           bigint      GENERATED ALWAYS AS IDENTITY,

    tenant_id           text        NOT NULL,
    customer_account_id text        NOT NULL,
    source              text        NOT NULL,
    source_request_key  text        NOT NULL,

    -- 门禁挡下的原因；空数组即放行。
    block_reasons       smallint[]  NOT NULL,
    -- 放行时将要建的那份委托。答案在建单之前记下，所以这里有号不等于委托已在。
    shipment_request_id text,
    decision            jsonb       NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT submission_ownership_answer_pkey
        PRIMARY KEY (answer_id),

    -- 答案只挂在已保全的来源上：保全之后才判归属（UC-PS-001 步骤 1 先于 3B）。
    CONSTRAINT submission_ownership_answer_preserved_fkey
        FOREIGN KEY (tenant_id, customer_account_id, source, source_request_key)
        REFERENCES parcel_shipment.source_submission
            (tenant_id, customer_account_id, source, source_request_key),

    -- 放行才有将要建的委托，挡下就没有：两格错配时，重发会把一份挡下的答案当成建单所据。
    CONSTRAINT submission_ownership_answer_request_iff_allowed
        CHECK ((cardinality(block_reasons) = 0) = (shipment_request_id IS NOT NULL)),
    CONSTRAINT submission_ownership_answer_request_not_blank
        CHECK (shipment_request_id IS NULL OR btrim(shipment_request_id) <> '')
);

-- 按来源身份取最近一次、或取某委托号所据的那一次，顺序稳定。
CREATE INDEX submission_ownership_answer_by_identity
    ON parcel_shipment.submission_ownership_answer
        (tenant_id, customer_account_id, source, source_request_key, answer_id);
