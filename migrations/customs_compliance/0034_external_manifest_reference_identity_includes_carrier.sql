-- 外部监管舱单引用的键加上承运商责任（UC-CC-012）。
--
-- 外部舱单身份是承运商自己编号空间里的号。原来的键是（租户，舱单身份），两家承运商用到同一个号时，先交的一家占住这一
-- 行、后交的一家永远登不进；首登口对「号已被别家占用」与「号还空着」答得不一样，也就成了探测别家舱单在不在册的口。
-- 键改成（租户，承运商责任，舱单身份）之后，各家只读写自己编号空间里的那一格。
--
-- 这张表此前没有任何生产写入口（接收编排在本迁移同一笔才接进进程），所以不做数据搬移：既有行每行自带 carrier_ref，
-- 新键在它们上面照样唯一。

ALTER TABLE customs_compliance.external_manifest_reference
    DROP CONSTRAINT external_manifest_reference_pkey;

ALTER TABLE customs_compliance.external_manifest_reference
    ADD CONSTRAINT external_manifest_reference_pkey
        PRIMARY KEY (tenant_id, carrier_ref, manifest_id);
