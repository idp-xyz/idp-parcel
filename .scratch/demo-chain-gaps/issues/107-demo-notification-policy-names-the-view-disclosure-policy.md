# 107 演示通知策略登在客户视图披露策略版本下，异常披露决定查不到通知指令

Category: bug
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自票 [97](./97-demo-disclosure-policy-keeps-the-final-dimension-pending.md) 的隔离评审：作者完成记录里顺带记了这条观察，评审按代码核过属实，要求另立票以免丢。号 107 是自取的（协调方没给号段，取号前查过本地各分支本目录最大到 106），进 main 前请协调方对号。
Blocked by: 无
地盘：种子 `scripts/demo-seeds/data/visibility/03-notification-policy-disclose-v1.json`；种子 README 故事第 5 条若要补一句通知，也在这里。
出处：VE `NotificationPolicies.DirectNotification` 的头注「按披露决定所依据的那条披露策略查……披露策略引用指名的就是版本化信息披露规则」。

## 现象（读代码，钉 `63093e55`，没有实走）

- 通知策略种子 `data/visibility/03` 登的 `policy` 是 `SYN-VE-DISCLOSE-V1`，这是客户视图四维的披露策略版本（`data/visibility/07`）。
- 异常披露决定带的策略引用取自异常披露规则版本：`ExceptionDisclosureRules` 把读到的规则版本当 `rule.Policy`，演示里是 `SYN-VE-EXC-DISCLOSE-V1`（`data/visibility/08`）。`DecideDisclosureHandler` 照它形成披露决定。
- `NotifyCustomerHandler` 经 `DirectNotification` 按披露决定的策略引用查 `notification_policy.disclosure_policy_ref`。两边的值对不上，演示租户上 `SYN-ACCOUNT-01` 的 `CUSTOMS_HOLD` 即便获准披露、走到通知，也只会答未配置。

## 影响

- 演示动线走到客户可见异常与主动通知时，通知停在未配置，看上去像租户没登通知规则，其实登了，只是登错了键。
- 只是种子登错了键，查找键的口径没错：头注写明按信息披露规则查，客户视图的披露策略本来就不管通知。

## 三类

- 租户取值：演示租户的通知策略挂在哪一版信息披露规则下。种子改为登在 `SYN-VE-EXC-DISCLOSE-V1` 下，证据记 `S`。
- 机制：无（查找键已按头注定）。若分诊认为两套「披露策略」同名易混、该在目录或登记面上分开叫，另起票。
- 产品策略：无。

## 最小的诚实修法

改 `data/visibility/03` 的 `policy` 为 `SYN-VE-EXC-DISCLOSE-V1`；在一次性库上 `seed.sh` 灌一遍退 0；能走到异常披露与通知的那段动线时，实走确认通知指令形成。文件名里的 `disclose-v1` 可一并改成按异常披露规则命名，`seed.sh` 那一行跟着改。

## 要裁的

- 改种子值还是另登一行：已在册的 `SYN-VE-DISCLOSE-V1` 那一行要不要留。它不会被任何披露决定命中，留着只会误导，建议直接改。
- 名字易混：客户视图四维那一套的登记子命令叫 `disclosure-policy`，而异常披露决定带的引用类型叫 `DisclosurePolicyReference`、通知策略的列叫 `disclosure_policy_ref`，指的却是异常信息披露规则。种子照名字挂到客户视图那一套上，多半由此而来。要不要分开叫，归 VE owner。

## 既有票

none
