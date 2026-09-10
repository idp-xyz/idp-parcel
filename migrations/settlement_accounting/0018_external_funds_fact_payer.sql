-- 外部资金事实加「来源提供的付款人」一维（票 sa-cc/03 裁决：付款人维取 A、在本上下文可缺席）。
--
-- customs-compliance 的税费付款核对把付款人列为「来源提供或真实程序要求的」维度（CC CONTEXT），
-- 而事实进产品只有本上下文采用这一口（ADR-0137 决定四）：来源给了这里不登，下游就永远拿不到。
-- 本上下文只保留不判断——它不修改外部资金事实。
--
-- 可空是语义不是权宜：来源未提供付款人时事实照样采用，NULL 就是「未提供」，不写空串顶替
-- （CHECK 拒空白：一枚空字符串的「付款人」比 NULL 更坏，它看起来像填过了）。不接结构化账户，
-- 不登任何真实银行字段。存量为零（本上下文的采用今天没有生产入口），不回填。
-- 更正版本同型带着它走，由领域 CorrectAmount 保证，这里不另设约束。
--
-- 不要与「付款方身份」混名：那是实际代垫成立判断的输入，是本上下文自己的概念。

ALTER TABLE settlement_accounting.external_funds_fact
    ADD COLUMN payer_ref text,
    ADD CONSTRAINT external_funds_fact_payer_not_blank
        CHECK (payer_ref IS NULL OR btrim(payer_ref) <> '');
