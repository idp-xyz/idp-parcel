-- 票 catalogue-read-pagination-rollout/08：settlement-accounting 的列表读口迁到 ADR-0144 之后，各册按「租户 + 排序维 + 行标识」取页，
-- 每册一条与缺省序同形的索引（ADR-0144 决定七）。缺省序与行标识恰是主键列序的几册，主键索引已经同形，不另建：
--   客户费用 customer_charge_pkey（费用标识）；
--   已发布对账单与挂着未了结争议的对账单 customer_statement_pkey（单号）；
--   外部资金事实 external_funds_fact_version_pkey（事实、版本；只列链头那一版的条件在这之上过滤）；
--   经营结果 operating_result_pkey（分析范围、账期、基准）；
--   成本分摊 cost_allocation_pkey（分摊标识）。
-- 下面补其余各册。

-- 供应商账单接收：缺省序收到时刻倒序，标识（主张、版本）同向倒序。收到时刻今天只在主张文档里，排序维须是非空可比的值
-- （该票 spec「可排维与行标识的列必须非空」），由取值函数导出；读面排序与表达式索引调的是同一个函数，函数体的类型转换不被内联展开，两边才对得上。
--
-- 函数标 IMMUTABLE 只对本表成立：文本转 timestamptz 一般随 DateStyle 与 TimeZone 变，下面的约束把本表能存的取值限定成
-- 带时区的 ISO 8601（写口按 RFC 3339 序列化 time.Time，存量行都是这一形），这一形的解析与两项会话设置无关。别处的 JSON
-- 不受这道约束，不要拿这个函数去取。
CREATE FUNCTION settlement_accounting.supplier_bill_received_at(claim jsonb)
RETURNS timestamptz
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
AS $$
    SELECT (claim ->> 'received_at')::timestamptz;
$$;

ALTER TABLE settlement_accounting.supplier_bill_reception
    ADD CONSTRAINT supplier_bill_reception_received_at_is_an_instant
        CHECK (
            (claim ->> 'received_at') IS NOT NULL
            AND (claim ->> 'received_at')
                ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$'
        );

CREATE INDEX supplier_bill_reception_by_received_at
    ON settlement_accounting.supplier_bill_reception
        (tenant_id, settlement_accounting.supplier_bill_received_at(claim) DESC, claim_id DESC, claim_version DESC);

-- 供应商预期成本版本册：缺省序发生项，标识（费用项目、版本）。首版幂等的唯一索引前缀相同而末段是计价规则版本、且只覆盖
-- 首版，不能顶替。
CREATE INDEX supplier_expected_cost_by_occurrence
    ON settlement_accounting.supplier_expected_cost (tenant_id, occurrence_id, fee_item, version);

-- 审核应付册：缺省序审核时刻倒序，标识应付同向倒序。
CREATE INDEX audited_payable_by_audited_at
    ON settlement_accounting.audited_payable (tenant_id, audited_at DESC, payable_id DESC);
