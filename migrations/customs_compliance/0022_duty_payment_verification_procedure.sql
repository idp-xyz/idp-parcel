-- 税费付款核对记录带监管程序（票 sa-cc/22 裁决 1 / 2）。
-- 付款人那一维「要不要求」按哪个真实程序的规则判，今天只在核对命令上、不进记录：形成之后看不出付款人维是按
-- 哪个程序判的（ADR-0137 决定一「记录带依据引用」对付款人维缺一维；CC CONTEXT「税费付款核对」词条：付款人是
-- 「来源提供或真实程序要求的」维度）。这里只加**记录列**，主键 (tenant_id, duty_ref, funds_ref, scope_ref,
-- version_digest) 一字不动：同三轴同依据但按不同程序判是两份不同的判断，这一层区分由应用层把程序折进
-- version_digest 来表达——存着这份键形的几处（0019 gate_verification 三列、settlement_accounting 0020 采用表、
-- 信封）因此都不必跟着改。0016 不改（已施加，校验和按文件内容算）：这里改它建的表，不回写它。
--
-- 存量：程序字面无来源（0016 起的一行不知道自己是按哪个程序判的），且指纹算法随本票加维，旧行的指纹也对不上
-- 自己；当前无租户、存量为零。若哪个库上不为零，宁可让迁移停下交人，不替旧行编一个程序、也不替它重算指纹
-- （照 0021 之形）。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM customs_compliance.duty_payment_verification) THEN
        RAISE EXCEPTION 'customs_compliance.duty_payment_verification 有存量行：程序字面无来源、指纹已随 0022 加维，须人工处置存量后再施加 0022';
    END IF;
END
$$;

ALTER TABLE customs_compliance.duty_payment_verification
    ADD COLUMN procedure_ref text NOT NULL;

-- 与 not_blank 那道同判据：说不出按哪个程序判的核对进不来。
ALTER TABLE customs_compliance.duty_payment_verification
    ADD CONSTRAINT duty_payment_verification_procedure_not_blank
        CHECK (btrim(procedure_ref) <> '');
