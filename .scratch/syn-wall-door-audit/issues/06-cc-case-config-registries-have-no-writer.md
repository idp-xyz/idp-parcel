# 关务案件配置面五类登记册只读,无写入方无登记口

Category: enhancement
Status: needs-triage

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W13。

## 墙

`DECLARATION_UNDECIDED`(`customscompliance/application/submit_declaration.go`)、`RESULT_UNDECIDED`(`receive_external_result.go`)——申报就绪、提交授权、解释规则、关闭义务、门禁条件五类配置空册时,案件链各判断停在未决。

## 现状:有装载无写入

- 表:迁移 0006(readiness 与 submission authority)、0007(interpretation 与 case requirement rules)、0008(closure obligation 与 gate conditions)。
- 装载口:五个只读视图齐(`readiness_view.go`、`submission_authority_view.go`、`interpretation_rule_view.go`、`obligation_inventory_view.go`、`gate_condition_view.go`)。
- 写入方与登记口:零(非测试代码无 INSERT;无登记用例;`/customs/external-results` 端点还在接入渠道墙后)。

## 缺的最小机制件

关务配置登记口:版本化登记用例 + 写入方,覆盖五类配置(按法定生效区间与适用时点版本化,PAR-CUS-04 的机制半边);进程级入口。

## 红线

- PAR-CUS-01..07 实例值(真实程序/服务方/渠道/规则源)待提供是常态;本票只建门,验证用脱敏合成配置,S 级只记 S。
- 规则版本不可覆盖;结果代码与层次映射按版本登记,不得写死。

## 参照

PAR-CUS-01..07(尤其 PAR-CUS-04);`docs/design/customs-slice-0-business-development-handoff.md`。
