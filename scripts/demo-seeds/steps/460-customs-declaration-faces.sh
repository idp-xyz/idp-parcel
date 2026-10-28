echo "== 关务立案与申报提交三口的前提（pilot-governance 权威区间 + customs-compliance 演示单元就绪；ADR-0227） =="
# 配置员持建案、提交授权确认、申报提交执行三种运营决定（data/access/02 的授予、03 的对照随 450-access 一起登）；三口铸信封时判
# 准入范围，这里登对应的三份运营决定权威区间——少了哪一份，那一口授予齐备也答 OUTSIDE_ADMISSION_SCOPE。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/42-authority-interval-operation-decision-customs-case-establishment.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/43-authority-interval-operation-decision-customs-submission-authorization.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/44-authority-interval-operation-decision-customs-declaration-submission.json"
# 演示申报单元 SYN-UNIT-CN-EXPORT-DEMO-01 只登就绪、不登授权：授权要由持授权确认授予的人经 /customs-submission-authorizations
# 逐次确认（首发范围：业务角色明确确认后才能提交），不走 authority-grant 那条运维登记。250-customs 里单元 01、02 的授权是案件页
# 展示用的既有种子，不是这一条动线。就绪判断的依据是合成串，证据只记 S。
"$BIN/parcel-customs-register" readiness-register -input "$SEEDS/customs/31-readiness-cn-export-declaration-demo.json"
# 演示单元的发送目标 SYN-CHANNEL-CN-EXPORT 登到本机的合成报关服务商（declaration-broker/，ADR-0342 决定三）：dispatch 收到提交版本
# 已形成就经它发出去。没起它，dispatch 记「确知没发出去」、照常续办；不登这一行，发送答通道未配置、尝试停在已登记待发送。通道
# 不带凭据：合成服务商不认凭据，凭据那一格演示的是「不填」。
"$BIN/parcel-customs-register" declaration-channel -input "$SEEDS/customs/33-declaration-channel-cn-export-synthetic-broker.json"
