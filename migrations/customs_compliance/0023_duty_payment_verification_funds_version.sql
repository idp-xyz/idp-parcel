-- 税费付款核对记录带「比的是资金事实的哪一版」（票 sa-cc/19 裁决 3）。
-- 核对是「版本化比较判断」（CC CONTEXT「税费付款核对」）；0021 起一条事实多版本各占一行，而核对表只引用事实身份
-- （0016 外键钉在 external_funds_fact (tenant_id, fact_ref)）——新版本到达后看不出哪版核对比的是被取代的那一版，
-- 「形成新核对版本」（UC-CC-009）也就无从按版本起步。这里加**记录列** funds_version 并外键到版本子表：一版核对
-- 只能引用一版已接收的事实。主键一字不动（照 0022 之形）：资金版本由应用层折进 version_digest，同三轴同依据同程序、
-- 只换资金版本即换指纹另成一行；存着这份键形的地方（0019 gate_verification 的核对读数、settlement_accounting 0020
-- 采用表、信封）都不必跟着改。0016 / 0021 / 0022 不改（已施加，校验和按文件内容算）：这里改它们建的表，不回写它们。
--
-- 到身份表的既有外键 duty_payment_verification_funds_fact_received 原样保留：「没有接收的资金事实就没有核对」是身份
-- 层面的话，本条是它之上再钉一层「比的是已接收的哪一版」，两条各说各的。
--
-- 存量：版本字面无来源（0022 前后的一行都不知道自己比的是哪一版），且指纹随本票再加维；当前无租户、存量为零。
-- 若哪个库上不为零，宁可让迁移停下交人，不替旧行编一个版本、也不替它重算指纹（照 0021 / 0022 之形）。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM customs_compliance.duty_payment_verification) THEN
        RAISE EXCEPTION 'customs_compliance.duty_payment_verification 有存量行：资金版本字面无来源、指纹已随 0023 加维，须人工处置存量后再施加 0023';
    END IF;
END
$$;

ALTER TABLE customs_compliance.duty_payment_verification
    ADD COLUMN funds_version text NOT NULL;

ALTER TABLE customs_compliance.duty_payment_verification
    ADD CONSTRAINT duty_payment_verification_funds_version_not_blank
        CHECK (btrim(funds_version) <> ''),
    ADD CONSTRAINT duty_payment_verification_funds_version_received
        FOREIGN KEY (tenant_id, funds_ref, funds_version)
        REFERENCES customs_compliance.external_funds_fact_version (tenant_id, fact_ref, version);
