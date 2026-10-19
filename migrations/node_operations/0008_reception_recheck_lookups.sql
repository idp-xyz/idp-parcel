-- 重核按号与按判断版本找收寄判断（ADR-0199 决定三，票 no-reception-source/03）。只加索引，不动表形状与约束。
--
-- reception_recheckable_by_mark：PS 外部标识关联已形成的门铃只带（租户，号码），重核按来源文档里的外部标识观察逐字找待识别。
-- 只覆盖重核对象所在的行：待识别、不记身份冲突、来源已保全（ADR-0199 Consequences「只覆盖待识别行」）；已形成、冲突版与来源
-- 保全之前的行不进这本索引。键取号码的 md5 而不取号码本身：号码由设备原样上报、没有长度上限，btree 的一项有上限，按号码本身
-- 建索引，一件贴着超长号的待识别就落不了册；查的一方同时比对 md5 与号码原文，摘要撞上只多比一次，不错配。
--
-- reception_by_judgment_version：人工重核与按判断版本自投的门铃都指名一版判断。判断版本由本上下文签发，长度有界。

CREATE INDEX reception_recheckable_by_mark
    ON node_operations.reception (tenant_id, md5(source_document ->> 'mark'))
    WHERE kind = 'PENDING_IDENTIFICATION'
      AND NOT identity_conflict
      AND source_document IS NOT NULL;

CREATE INDEX reception_by_judgment_version
    ON node_operations.reception (tenant_id, judgment_version)
    WHERE judgment_version IS NOT NULL;
