-- 提交答出的答案（票 docs-consistency/code/39，ADR-0266）。
--
-- UC-PS-001「必须持久保留的业务记录」要留下接受流程前的生产归属结果；重发同一请求时，提交编排凭这些答案分辨这份提交上一次
-- 走到了哪一步：委托已在（按委托号找建它所据的那一次）、已答出拒绝或已形成挡下的结果（答原结果），生产归属未决（重判），
-- 还是放行了却没建成（接着建单）。
--
-- 只追加：生产归属未决之后的重发会重判，每一次判断都留一行，后一次不覆盖前一次。决定整份存一列 jsonb，读回经领域构造函数
-- 重新校验，与委托快照同一种做法。
CREATE TABLE parcel_shipment.submission_answer (
    answer_id           bigint      GENERATED ALWAYS AS IDENTITY,

    tenant_id           text        NOT NULL,
    customer_account_id text        NOT NULL,
    source              text        NOT NULL,
    source_request_key  text        NOT NULL,

    -- 取得放行之前就答完的拒绝（输入未受理、原委托不可见、关联不适用）；空即这一行是一次归属判断。
    refusal             smallint,
    -- 门禁挡下的原因；空数组即放行（拒绝行恒为空）。
    block_reasons       smallint[]  NOT NULL,
    -- 放行时将要建的那份委托。答案在建单之前记下，所以这里有号不等于委托已在。
    shipment_request_id text,
    decision            jsonb,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT submission_answer_pkey
        PRIMARY KEY (answer_id),

    -- 答案只挂在已保全的来源上：保全之后才作答（UC-PS-001 步骤 1 先于 3A、3B）。
    CONSTRAINT submission_answer_preserved_fkey
        FOREIGN KEY (tenant_id, customer_account_id, source, source_request_key)
        REFERENCES parcel_shipment.source_submission
            (tenant_id, customer_account_id, source, source_request_key),

    -- 拒绝与归属判断二选一：拒绝没有决定，判断必有决定。
    CONSTRAINT submission_answer_refusal_xor_decision
        CHECK ((refusal IS NULL) = (decision IS NOT NULL)),
    CONSTRAINT submission_answer_refusal_has_no_gate
        CHECK (refusal IS NULL OR cardinality(block_reasons) = 0),
    -- 放行才有将要建的委托，挡下与拒绝都没有：错配时，重发会把一份没放行的答案当成建单所据。
    CONSTRAINT submission_answer_request_iff_allowed
        CHECK ((refusal IS NULL AND cardinality(block_reasons) = 0) = (shipment_request_id IS NOT NULL)),
    CONSTRAINT submission_answer_request_not_blank
        CHECK (shipment_request_id IS NULL OR btrim(shipment_request_id) <> '')
);

-- 按来源身份取最近一次、或取某委托号所据的那一次，顺序稳定。
CREATE INDEX submission_answer_by_identity
    ON parcel_shipment.submission_answer
        (tenant_id, customer_account_id, source, source_request_key, answer_id);
