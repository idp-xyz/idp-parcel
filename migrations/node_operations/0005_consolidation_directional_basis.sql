-- 方向依据记录：加成员事实落册时，同一笔记下该作业实物当时的方向依据（票 no-directional-basis/01 偏离标记半边）。
--
-- 三格——身份、服务结果、路由依据——每格取 ESTABLISHED / MISSING / NOT_CONFIGURED / UNREADABLE / NOT_JUDGED
-- 之一，MISSING 附原因，另带判断时刻。词表与各格的取值规则在领域，领域的重建门（RehydrateDirectionalBasis）读回时
-- 逐条再验；库面这一层守形状：只有加成员行带；带了就是三格齐全、取值在封闭五值内、MISSING 必附原因且只有
-- MISSING 带原因、原因是各格自己的词、判断时刻在场的对象。绕过适配器直接写库的一行——迁移脚本、运维改数、日后
-- 另写的适配器——领域构造器一次都不会经过，这几条是它们唯一过的门。
--
-- **不回填，也不给 DEFAULT。** 这一列加上之前落的加成员事实当时没有判过方向依据，NULL 读出为`未记录`。任何回填
-- 值都是事后补判冒充当时所知，而记录说的正是「判断时刻所知」；回填一份「依据齐全」更会把可能已取消、已终局的件
-- 读成没事。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律，同 0003）：jsonb 取键可能给 NULL，比较前一律先
-- COALESCE，否则整条按 NULL 放行。原因数组不调 jsonb_array_length——它对非数组报错，而 CHECK 里 AND 两侧的求值
-- 次序不作保证；改与空数组字面量比。

ALTER TABLE node_operations.consolidation_fact
    ADD COLUMN directional_basis jsonb;

ALTER TABLE node_operations.consolidation_fact
    ADD CONSTRAINT consolidation_fact_directional_basis_add_member_only
        CHECK (directional_basis IS NULL OR action = 'ADD_MEMBER');

ALTER TABLE node_operations.consolidation_fact
    ADD CONSTRAINT consolidation_fact_directional_basis_shaped
        CHECK (
            directional_basis IS NULL
            OR (
                jsonb_typeof(directional_basis) = 'object'
                AND btrim(COALESCE(directional_basis ->> 'judgedAt', '')) <> ''

                AND COALESCE(directional_basis #>> '{identity,answer}', '')
                    IN ('ESTABLISHED', 'MISSING', 'NOT_CONFIGURED', 'UNREADABLE', 'NOT_JUDGED')
                AND COALESCE(directional_basis #>> '{serviceResult,answer}', '')
                    IN ('ESTABLISHED', 'MISSING', 'NOT_CONFIGURED', 'UNREADABLE', 'NOT_JUDGED')
                AND COALESCE(directional_basis #>> '{routing,answer}', '')
                    IN ('ESTABLISHED', 'MISSING', 'NOT_CONFIGURED', 'UNREADABLE', 'NOT_JUDGED')

                -- 原因在场就是数组；MISSING 与「带非空原因数组」同真同假。
                AND COALESCE(jsonb_typeof(directional_basis #> '{identity,reasons}'), 'array') = 'array'
                AND COALESCE(jsonb_typeof(directional_basis #> '{serviceResult,reasons}'), 'array') = 'array'
                AND COALESCE(jsonb_typeof(directional_basis #> '{routing,reasons}'), 'array') = 'array'
                AND (COALESCE(directional_basis #>> '{identity,answer}', '') = 'MISSING')
                    = COALESCE(jsonb_typeof(directional_basis #> '{identity,reasons}') = 'array'
                               AND directional_basis #> '{identity,reasons}' <> '[]'::jsonb, false)
                AND (COALESCE(directional_basis #>> '{serviceResult,answer}', '') = 'MISSING')
                    = COALESCE(jsonb_typeof(directional_basis #> '{serviceResult,reasons}') = 'array'
                               AND directional_basis #> '{serviceResult,reasons}' <> '[]'::jsonb, false)
                AND (COALESCE(directional_basis #>> '{routing,answer}', '') = 'MISSING')
                    = COALESCE(jsonb_typeof(directional_basis #> '{routing,reasons}') = 'array'
                               AND directional_basis #> '{routing,reasons}' <> '[]'::jsonb, false)

                -- 原因是各格自己的词。路由依据今天没有原因词（成立与缺少要等 NR 的路由指令与兼容判断），
                -- 它的原因数组里出现任何东西都拒。
                AND NOT jsonb_path_exists(directional_basis,
                    '$.identity.reasons[*] ? (@.type() != "string" || (@ != "NO_INTAKE_RECORD" && @ != "PENDING_IDENTIFICATION" && @ != "IDENTITY_CONFLICT"))')
                AND NOT jsonb_path_exists(directional_basis,
                    '$.serviceResult.reasons[*] ? (@.type() != "string" || (@ != "CANCELLED" && @ != "FINAL"))')
                AND NOT jsonb_path_exists(directional_basis, '$.routing.reasons[*]')
            )
        );

-- 身份那一格按作业实物查收寄册上带收寄的行（Receptions.FindIntakeIdentities）。此前收寄册只按来源身份（主键）与
-- 正式包裹关联（0004）可查；按作业实物查要这一条表达式索引。部分索引的谓词与查询里的 intake IS NOT NULL 同一句，
-- 规划器才认得它。
CREATE INDEX reception_intake_unit_idx
    ON node_operations.reception (tenant_id, (intake ->> 'unit'))
    WHERE intake IS NOT NULL;
