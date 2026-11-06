-- 票 catalogue-read-pagination-rollout/09：customs-compliance 的列表读迁到 ADR-0144 之后，各册按「租户 + 排序维 + 行标识」
-- 取页，每迁一册补一条与缺省序同形的索引（ADR-0144 决定七）。只加索引，不动任何表的形状与约束；各册排序维与行标识的列
-- 都是 NOT NULL，键序比较碰不到 NULL。
--
-- 主键或唯一约束已与缺省序同形的册不重建，服务它的就是那一条：
--   关务案件                    customs_case_id_unique（缺省序就是行标识：案件标识）
--   建案要求规则                case_requirement_rule_pkey
--   解释规则                    interpretation_rule_pkey（生效起点改成与首列同向的升序之后，主键正好同形）
--   就绪判断、提交授权          readiness_judgment_pkey、submission_authority_pkey
--   关闭义务目录                closure_obligation_catalog_pkey
--   门禁条件册                  gate_condition_catalog_pkey
--   口岸目录、申报路径目录      candidate_port_pkey、declaration_path_pkey（生效起点同样改为升序）
--   监管凭证册                  regulatory_credential_pkey
--   税费付款协作事项册          duty_payment_collaboration_pkey
-- 卡关对象是从准入判断派生的集合（每个对象取最近一次判断），页序排在派生出来的行上，没有索引能直接服务它；派生那一步
-- 走 action_admission_judgement_by_object。

-- 提交版本：缺省序申报单元，标识（单元、程序、固定时刻、版本号）。主键（单元、程序、版本号）是逐版一行的键，不能换成插了
-- 固定时刻的这一条，只能另立。
CREATE INDEX declaration_submission_by_tenant_unit
    ON customs_compliance.declaration_submission (tenant_id, unit_id, procedure_ref, fixed_at, version_id);

-- 外部监管结果：缺省序记下时刻，标识来源标识。按提交版本收窄时仍走 external_result_by_submission。
CREATE INDEX external_result_by_tenant_recorded
    ON customs_compliance.external_result (tenant_id, recorded_at, source_id);

-- 动作准入判断册：缺省序判断时刻倒序，标识判断标识，同向倒序。按对象收窄时仍走 action_admission_judgement_by_object。
CREATE INDEX action_admission_judgement_by_tenant_judged
    ON customs_compliance.action_admission_judgement (tenant_id, judged_at DESC, judgement_id DESC);

-- 税费付款核对册：缺省序税费引用，标识（税费、资金事实、范围、核对时刻、版本指纹）。主键是「同三维同内容重放不出第二版」
-- 的幂等键，不带核对时刻，同样只能另立。
CREATE INDEX duty_payment_verification_by_tenant_duty
    ON customs_compliance.duty_payment_verification
        (tenant_id, duty_ref, funds_ref, scope_ref, verified_at, version_digest);
