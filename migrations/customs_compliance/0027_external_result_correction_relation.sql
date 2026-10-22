-- 外部结果的层内更正关系（UC-CC-006「同一层次和业务事项的后续明确更正形成新结果及更正关系；原结果保持历史」）。
--
-- corrects_source_id 是来源明确声明本结果所更正的那一份的来源标识：来源内容，与接收记录同一行、同一次落下，
-- 不由本上下文从业务时间或语义差别推出；没有更正语义时为 NULL。correction_in_force 是接收那一刻的判断——
-- 更正关系在同一范围上生效，所更正那份从此退出当前判断。两列都记在更正一侧的新行上，所更正那一行一列不动：
-- 接收记录只建立不改写，「已被取代」是从后继行派生出来的问答，不是被取代行上的可变标记。
--
-- 存量行都不带更正语义：此前接收口收不进更正指向，NULL 与 false 如实。默认 false 留着，是给不写更正指向的
-- 行：没有指向就没有更正关系，false 如实；带指向的行由登记册适配器连同生效与否一并显式写下。

ALTER TABLE customs_compliance.external_result
    ADD COLUMN corrects_source_id text,
    ADD COLUMN correction_in_force boolean NOT NULL DEFAULT false;

ALTER TABLE customs_compliance.external_result
    -- 更正只在在册的可归属事实之间成立；外键钉住「所指那份在册」，跨租户的指向因为键含租户而指不过去。
    ADD CONSTRAINT external_result_corrects_a_booked_result
        FOREIGN KEY (tenant_id, corrects_source_id)
        REFERENCES customs_compliance.external_result (tenant_id, source_id),
    -- 归属不上的留存没有监管事实，带不了更正关系；一份结果不更正它自己；生效必有所指；记了冲突的更正不生效
    -- ——冲突的一方不得改变当前判断（CONTEXT「不得……按最后到达直接改变当前判断」）。
    ADD CONSTRAINT external_result_correction_shape
        CHECK (
            (corrects_source_id IS NULL AND NOT correction_in_force)
            OR (
                corrects_source_id IS NOT NULL
                AND NOT unattributable
                AND btrim(corrects_source_id) <> ''
                AND corrects_source_id <> source_id
                AND NOT (correction_in_force AND layer_conflict)
            )
        );

-- 只许更正链尾：一份事实至多被一份生效的更正取代。同一提交版本上的比对与落册已按提交版本串行（ADR-0226
-- 决定二），这里在库里再守一遍；它同时是放行读口按「有没有生效的后继」排掉被取代那份时走的索引。
CREATE UNIQUE INDEX external_result_one_correction_in_force
    ON customs_compliance.external_result (tenant_id, corrects_source_id)
    WHERE correction_in_force;
