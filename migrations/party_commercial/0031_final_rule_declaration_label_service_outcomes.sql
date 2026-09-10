-- 终局规则声明的责任结果封闭集加面单渠道服务两格（票 party-commercial-context-gaps/12；PC CONTEXT「面单服务终局规则」
-- 词条）：LABEL_SERVICE_COMPLETED（非取消终局结果）与 LABEL_SERVICE_FAILED（终局失败结果）。0013 的
-- final_rule_declaration_closed_set 把当时的字面量钉死在 CHECK 里，租户因此无处登记首个面单渠道产品的这两行——
-- 不改已施加的 0013，这里 DROP 再 ADD 同名约束，镜像 domain.DeclaredResponsibilityOutcome 此刻的 valid()。
--
-- 只放宽可选值：主键不动（outcome 仍在主键里，同一结果两行仍拒）、final_kind 不动、不预填任何行——哪种面单结果形成
-- 哪种终局类型属实例半边（PAR-COM-17），今天没有租户因而这两行全空。取消结果不进这里：取消不是终局规则声明的对象，
-- 取消终局由 parcel-shipment 直接形成。
--
-- 既有行全在旧字面量之内，新 CHECK 对它们恒真；ADD CONSTRAINT 默认校验存量行，这一点由它自己证。

ALTER TABLE party_commercial.final_rule_declaration
    DROP CONSTRAINT final_rule_declaration_closed_set;

ALTER TABLE party_commercial.final_rule_declaration
    ADD CONSTRAINT final_rule_declaration_closed_set
        CHECK (outcome IN (
            'EFFECTIVE_DELIVERY',
            'RETURN_COMPLETED',
            'SERVICE_TERMINATED',
            'REGULATORY_DISPOSITION',
            'LABEL_SERVICE_COMPLETED',
            'LABEL_SERVICE_FAILED'
        ));
