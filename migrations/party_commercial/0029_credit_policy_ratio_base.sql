-- 信用政策正文的比例基数（ADR-0129）：比例额度相对于什么，由政策正文自己声明。
--
-- 一列 text，取值是领域封闭集 domain.CreditRatioBase 的原词。**可空只为存量行**：0029 之前登进去的
-- 比例行没有这一格，重建门按 ADR-0028 只校验不重算、如实读回为「未声明」，由 settlement-accounting
-- 停在它自己的 CREDIT_RATIO_BASE_UNDECIDED；新登记的比例行由领域构造门（domain.NewCreditRatioLimit）
-- 保证非空，本 CHECK 不追溯——追溯会让一次迁移对着一批已固定的正文替租户拍板。
--
-- 「含则必填、不含则必缺」（ADR-0044 同形）的库上镜像：基数只跟比例走，金额行必空；非空时必在集合内
-- 且非空白。集合成员与领域枚举同一份原词，这里不另定语义。
--
-- 无租户故本表为空，本迁移不改任何一行。

ALTER TABLE party_commercial.credit_policy
    ADD COLUMN ratio_base text;

ALTER TABLE party_commercial.credit_policy
    ADD CONSTRAINT credit_policy_ratio_base_follows_ratio
        CHECK (
            ratio_base IS NULL
            OR (
                limit_ratio_bps IS NOT NULL
                AND ratio_base IN ('POSTED_BALANCE', 'PRIOR_PERIOD_CONFIRMED_CHARGES')
            )
        );
