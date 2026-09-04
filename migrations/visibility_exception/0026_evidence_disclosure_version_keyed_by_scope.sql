-- 证据披露版本的行身份补上披露范围：一个披露版本是「明确披露范围 + 脱敏版本」这一对
-- （CONTEXT「对外披露必须形成明确披露范围或脱敏版本」），同一份脱敏内容对两个相对方是
-- 两次披露、两个版本——0024 只按（证据项 + 脱敏指纹）成行，会把第二个范围的准备静默读成
-- 「已有」并交回另一范围的版本，准备方要的范围就这样丢了。
--
-- 改主键而不另加唯一约束：行身份就是这四件，没有第二个候选键。表在首发是空的（证据内容
-- 属实例半边），ALTER 不会撞上任何既有行；读口与写入口同笔按四件键取与存。
ALTER TABLE visibility_exception.evidence_disclosure_version
    DROP CONSTRAINT evidence_disclosure_version_pkey;

ALTER TABLE visibility_exception.evidence_disclosure_version
    ADD CONSTRAINT evidence_disclosure_version_pkey
        PRIMARY KEY (tenant_id, evidence_id, redacted_digest, scope);
