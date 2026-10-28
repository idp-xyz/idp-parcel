# 04 管理台的操作者引用长串与显示时区

Category: enhancement
Status: in-progress——2026-10-28 tops-20（idp-parcel-91）立并认领（用户「都要修」），分支 claude-demo-ui
Blocked by: 无

## 现象

彩排（`talk-track.md` 所据彩排，钉 `d086bc3c8`）：
1. 「来源」「提交人」「授权依据」「处理人」等列原样显示 `http://127.0.0.1:5558/dex#Ch…` 这类「发行方#主体」长串，把表格挤窄；委托查阅与提交版本签最明显。
2. 页面时间按浏览器所在机器的时区显示；演示机若不在上海时区，给国内客户看的时间是错位的。

## 做什么（只改 `apps/admin-web`）

1. 一个共享的引用显示：形如「发行方#主体」的操作者 / 客户端引用，单元格显示短名（主体部分；主体是不透明长串时截短并加省略），`title` 悬停给全文；可复制全文。找出所有显示这类引用的列统一用它（先 `grep` 现有页面里怎么渲染这些字段）。不改后端、不改数据。
2. 显示时区可配置：所有时间格式化走一个共享函数，时区取构建 / 运行配置（例如 `VITE_DISPLAY_TIME_ZONE`，IANA 名），未配置时按浏览器时区（今天的行为）；显示时带时区标注（例如「UTC+8」）避免误读。产品不写死任何租户的时区。`scripts/demo-seeds/demo-run/README.md` 与 `talk-track.md` 开机清单改成「起管理台时设 `VITE_DISPLAY_TIME_ZONE=Asia/Shanghai`」。

## 完成判据

- 单测覆盖引用短名（几种形状含无 `#` 的原样）与时区格式化（配置 / 未配置）；`tsc -b`、全部单测、`vite build` 通过。
- 碰共享面（`components/*`、`shell/*` 等）按 `docs/agents/workflow.md`「前端切片」要一份非作者评审。
