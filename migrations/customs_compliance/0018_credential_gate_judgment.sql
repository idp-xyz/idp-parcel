-- 凭证门禁判断登记册（票 sa-cc/04，ADR-0137 决定一 / 二）。所有权取 CONTEXT 那句：「逐门禁判断
-- （凭证门禁为第一册）是本上下文登记的不可覆盖事实，就绪判断按不可变引用绑定它们、不内嵌其
-- 内部结构」。此前 JudgeCredentialApplicability 算得出四格却无处落——UC-CC-003 步 7「记录凭证
-- 门禁」那半没有表；本迁移补的是它的落点，readiness_judgment 的形不动。
--
-- 一行即一版判断：键是判断身份三维（租户、申报单元、凭证身份）加内容指纹——同键同内容重放不出
-- 第二版；程序、持有人、截至时点、结论、依据引用、责任角色任一变了即换指纹追加新版，不覆盖，
-- 写口不 UPSERT、无 UPDATE 路径。来源变化（凭证登记 / 程序变更）不在这里翻任何一行：它让既有
-- 就绪判断`不再就绪`（readiness_judgment 的 revoked_* 两列），门禁记录只在评估请求到达时新增。
--
-- 「凭证身份与版本」在本仓折成 credential_id 一列：监管凭证是「一身份一版」（0014 自注），换内容
-- 是另一张凭证。不外键到 regulatory_credential——「凭证未登记」是四格之一，那一格的行指向的正是
-- 册上还没有的身份，外键会让这一格落不进来。
--
-- as_of 是判断覆盖到的截至时点（拟使用的业务时间，凭证有效期按它算），judged_at 是判断发生的
-- 时刻，两列两轴；只记门禁判断，不占用、不释放、不核销——表上没有额度或使用列。
--
-- 真实凭证、程序、持有人、依据与角色全部属实例半边（PAR-CUS-04 待提供），本迁移不含任何实例
-- 默认值；隔离演示走 SYN- 前缀合成种子（ADR-0078）。
CREATE TABLE customs_compliance.credential_gate_judgment (
    tenant_id      text        NOT NULL,
    unit_id        text        NOT NULL,
    credential_id  text        NOT NULL,
    version_digest text        NOT NULL,

    procedure_ref  text        NOT NULL,
    holder_ref     text        NOT NULL,
    as_of          timestamptz NOT NULL,
    conclusion     text        NOT NULL,
    basis_ref      text        NOT NULL,
    role_ref       text        NOT NULL,
    judged_at      timestamptz NOT NULL,

    CONSTRAINT credential_gate_judgment_pkey
        PRIMARY KEY (tenant_id, unit_id, credential_id, version_digest),

    CONSTRAINT credential_gate_judgment_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(unit_id) <> ''
            AND btrim(credential_id) <> ''
            AND btrim(version_digest) <> ''
            AND btrim(procedure_ref) <> ''
            AND btrim(holder_ref) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(role_ref) <> ''
        ),

    -- 结论封闭四格，与 domain.CredentialGateConclusion 的 String 同词。「凭证未登记」与「不适用」
    -- 各占一词——压成一格，租户上线前每一次判断都会读成「凭证不适用」（票 sa-cc/04 红线）。
    CONSTRAINT credential_gate_judgment_conclusion_closed
        CHECK (conclusion IN ('APPLICABLE', 'NOT_APPLICABLE', 'CREDENTIAL_NOT_REGISTERED', 'UNDECIDED'))
);
