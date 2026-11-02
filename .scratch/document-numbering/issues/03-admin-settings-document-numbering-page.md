# 03 管理台「设置 → 单据编号」

Category: enhancement
Status: ready-for-agent
父票：[spec](../spec.md)
Blocked by: [02](./02-platform-numbering-adoption-and-rule-register.md)
出处：spec 子票表；[ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) 决定五、六

## 做什么

前端切片（只动 `apps/admin-web/src/**`，照 `docs/agents/workflow.md`「前端切片」）：设置区新增「单据编号」页，工作台形态。

- 列表：十一类单据，每行单据名（业务叫法）、此刻生效的格式与样例号、来源胶囊（产品默认 / 本公司规则）、重置周期、日期所用时区。
- 右栏：这一类的历次规则版本与生效时刻；类型代码、模板原文进技术详情。
- 登记：任务式表单「改用新的编号格式」——填模板与生效时刻，边填边调预览口显示样例号与「可能与已用号段重叠」的提示；按登记册配置写授予收起（ADR-0315）。模板令牌给一份中文说明（年、月、日、流水位数、加扰），不要求用户懂语法。
- 单据编号时区：页头显示租户此刻的单据编号时区，没登就醒目地写「单据编号时区未配置——开立方没有业务时区的单据（如运单、异常案件）暂不带单据号」，并给登记入口（同样按授予收起）。
- 诚实态：读口未配置 / 无权 / 读不到分开显示；没登记过的类型如实显示「产品默认」。

## 完成判据

`tsc -b`、`pnpm test`、`vite build` 过；浏览器核过列表、右栏、预览与收起（真后端或合成答复，写明哪种）。
