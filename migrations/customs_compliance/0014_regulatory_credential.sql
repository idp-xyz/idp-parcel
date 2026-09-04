-- 监管凭证登记册（票 mechanism-executor-triage/07 CC-a）。所有权取 CONTEXT「监管凭证」
-- 语言：凭证有独立身份，附件文件只是证据，不能代替凭证身份和适用性判断；本上下文拥有
-- 凭证的登记、适用性判断与使用关系。此前关务迁移无凭证表，领域工厂 RegisterCredential
-- 因此只被测试调过——本迁移补的是它的落点。
--
-- 一行即一版不可变凭证：签发机构、持有人、适用程序、有效期、次数额度一次进入
-- （列与 domain.RegulatoryCredential 逐格对应）。键是（租户，凭证身份），一身份一版——换期限或换
-- 额度是另一张凭证（另一个身份），不是覆盖；写口不 UPSERT、无 UPDATE 路径。
--
-- 额度：uses 为零在领域约定为「来源未提供次数额度」，与「零次可用」不是一格——后者在
-- 本册没有表达方式，因为余额不是登记内容：占用/释放/核销是凭证使用的生命周期
-- （UC-CC-005 步 7/9、UC-CC-006 步 7），时点由真实程序定（PAR-CUS-04），另立册子。
-- 这里存的是凭证本身。
--
-- 真实签发机构、持有人、程序、期限与额度全部属实例半边（PAR-CUS-04 待提供），本迁移
-- 不含任何实例默认值；隔离演示走 SYN- 前缀合成种子（ADR-0078）。
CREATE TABLE customs_compliance.regulatory_credential (
    tenant_id      text        NOT NULL,
    credential_id  text        NOT NULL,

    issuer_ref     text        NOT NULL,
    holder_ref     text        NOT NULL,
    procedure_ref  text        NOT NULL,
    valid_from     timestamptz NOT NULL,
    valid_to       timestamptz NOT NULL,
    -- 零即「来源未提供」；负数是矛盾输入，领域构造期已拒，库内再守一遍。
    uses           integer     NOT NULL,
    registered_at  timestamptz NOT NULL,

    CONSTRAINT regulatory_credential_pkey
        PRIMARY KEY (tenant_id, credential_id),

    CONSTRAINT regulatory_credential_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(credential_id) <> ''
            AND btrim(issuer_ref) <> ''
            AND btrim(holder_ref) <> ''
            AND btrim(procedure_ref) <> ''
        ),

    -- 有效期严格有序，与领域 validTo.After(validFrom) 同判据。
    CONSTRAINT regulatory_credential_validity_ordered
        CHECK (valid_to > valid_from),

    CONSTRAINT regulatory_credential_uses_not_negative
        CHECK (uses >= 0)
);
