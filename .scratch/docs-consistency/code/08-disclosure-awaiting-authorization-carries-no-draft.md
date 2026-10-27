# 08 VE 披露决定「待授权」装不下内容草稿

Category: bug
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（线外，复核见 .scratch/launch-line-freeze-2026-10-27.md 第六节）：VE 披露决定「待授权」装不下内容草稿——`NewDecideDisclosureHandler` 在 `cmd/` 下没有调用方；解冻条件：接线这个处理器之前先修。此前 ready-for-agent——R3（不急）：`NewDecideDisclosureHandler` 在 `cmd/` 下没有调用方，今天无生产路径；接线之前必须先修，否则有权角色确认时无从看见要确认的内容。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-12（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

文档要「客户披露待授权」带内容草稿，交有权角色确认；领域构造规定只有披露格带内容、其余格必不带，待授权一带内容就报错。

## 证据

- 文档：
  - UC-VE-001 结果语义契约「客户披露待授权」：「客户、对象、内容草稿、依据、所需授权和时限」；`AT-VE-024`：「形成客户披露待授权和内容草稿」。
  - UC-VE-006「草稿/待授权」：「内容已生成但尚未取得发布授权」；`AT-VE-100`：「形成草稿和待授权，不提交渠道」。
  - 参数登记册 `PAR-VIS-07`：「默认草稿经授权确认」。
- 代码：`internal/visibilityexception/domain/customer_disclosure.go` 的 `DecideDisclosure`：`conclusion != DiscloseToCustomer && content.valid()` 即 `ErrInvalidDisclosure`；`DisclosureDecision.Content()` 只在披露格给出。

## 三类

机制。产品策略：无。租户取值：默认是否经授权确认（`PAR-VIS-07`）仍归登记册，本票不设默认。

## 规模

S：待授权格带「内容草稿」引用（与发布用的内容快照分开），存储加一列，构造与读回同一道门。

## 最小的诚实修法

`AwaitingAuthorization` 必带内容草稿引用；`DiscloseToCustomer` 照旧必带内容快照；暂不披露、不披露格照旧不带。授权通过后由草稿形成内容快照，草稿本身保留。

## 要裁的

无。
