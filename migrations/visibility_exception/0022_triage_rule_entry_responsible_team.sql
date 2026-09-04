-- 分诊条目补责任团队一维（`PAR-VIS-05`）：走向为自动建案的条目在同一行说清案件归谁。
--
-- 「建立案件 → 固定根对象、初始影响范围、责任团队、初始优先级和响应策略」（CONTEXT
-- 生命周期），而「每个开放案件始终必须有一个内部案件责任团队」——分诊规则说「自动
-- 建案」却不说归谁，案件就建不起来。团队因此不是案件建立时由编排挑的，是命中的那条
-- 规则登记时就带着的：规则版本换了，归属随版本一起换，历史案件仍能追溯到当时那一版。
--
-- 团队是谁属实例半边（`PAR-VIS-04` 的「责任团队和处置路径」待提供），本迁移只加列与
-- 成对约束，不种任何取值。
--
-- 成对约束两向都守：自动建案必带、其余走向必不带。给一条人工复核条目挂团队，是把
-- 「谁来复核」误写进了「建案归谁」那一格——读口若把它读成自动建案的归属，就会替一条
-- 没说自动建案的规则建出案件。表在首发是空的（目录内容待租户登记），ALTER 不会撞上
-- 任何既有行。
ALTER TABLE visibility_exception.triage_rule_entry
    ADD COLUMN responsible_team text;

ALTER TABLE visibility_exception.triage_rule_entry
    ADD CONSTRAINT triage_rule_entry_auto_establish_names_team
        CHECK (
            (outcome = 'AUTO_ESTABLISH') = (responsible_team IS NOT NULL)
            AND (responsible_team IS NULL OR btrim(responsible_team) <> '')
        );
