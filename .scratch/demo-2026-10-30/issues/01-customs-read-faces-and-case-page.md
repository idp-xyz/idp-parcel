# 01 关务读口与 customs-cases 页

Category: enhancement
Status: in-progress——2026-10-28 tops-20（idp-parcel-91）立并认领，分支 claude-demo-cc
Blocked by: 无

## 现象

审计（`../ui-coverage-audit.md`）：`/customs-cases`、`/customs-declaration-submissions` 只有 POST；申报发送与技术回执、放行结果、关案记录、门禁判断册（交出读数 `ADMITTED` 只在交接登记响应体里）都没有 GET；`customs-cases` 页「关务案件、申报单元、资料快照、提交版本」四签是占位。演示「立案 → 申报 → 发送回执 → 放行 → 门禁放行交出」在管理台上看不到。

## 做什么

后端（`internal/customscompliance` 读侧 + `cmd/parcel-api`，照现有 CC 查阅口写法，操作者查阅面、租户取自认证信封、只读）：

1. 案件列表与单份读回（含申报单元、状态、关案记录若有）。
2. 申报提交版本与发送尝试 / 技术回执（PSB/10 第 1 项 ADR-0342 的发送事实与回执）。
3. 放行结果（外部结果登记与归属、`release_outcome`）。
4. 动作准入判断册读面（按包裹或申报单元列各受守护动作的读数与依据）。

前端（`apps/admin-web` 的 customs-cases 页）：四个占位签接上 1–3；加一签或一区显示 4（按包裹查）。照同页已接线签与 `party` 区页面写法，空态写清是什么。

不做：新的写口；改判断语义。

## 完成判据

- 真库用例：各读口在演示租户形状数据上答出内容、他租户不可见、未授予 403。
- 管理台：演示租户跑完 `declare-one-parcel.sh` + 发送 + 放行 + 交出后，四类结果在页面上可见（组件单测 + 类型检查；能起浏览器就截一次）。
