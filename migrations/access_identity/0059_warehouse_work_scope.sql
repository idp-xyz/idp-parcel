-- 作业范围种类加仓（ADR-0384 决定三）。与 accessidentity.WorkScopeKind 同一份封闭集，两处同笔改。
-- NODE 与 OFFSITE 留着：一台设备仍只登记一个范围，节点设备拿来收货不是把节点从封闭集里拿掉。
-- 只重建作业范围那两道 CHECK。本迁移不种任何行：哪个操作者持有哪个仓、哪台设备登在哪个仓，是租户取值。
--
-- 只列本份基线上的全集。之后谁再重建这两道 CHECK，都取此前各份的并集，再加自己的词：迁移不按编号先后施加，
-- 漏掉已有的词会让那一格的授予或设备登不进。

ALTER TABLE access_identity.operator_grant
    DROP CONSTRAINT operator_grant_work_scope_decided;

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_work_scope_decided
        CHECK (work_scope_kind IS NULL OR (work_scope_kind IN ('NODE', 'OFFSITE', 'WAREHOUSE') AND btrim(work_scope_ref) <> ''));

ALTER TABLE access_identity.device
    DROP CONSTRAINT device_work_scope_decided;

ALTER TABLE access_identity.device
    ADD CONSTRAINT device_work_scope_decided
        CHECK (work_scope_kind IN ('NODE', 'OFFSITE', 'WAREHOUSE'));
