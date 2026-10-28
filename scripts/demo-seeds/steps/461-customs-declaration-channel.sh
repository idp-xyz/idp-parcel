echo "== 演示申报单元的发送通道（customs-compliance 申报通道册；ADR-0342） =="
# 演示单元的发送目标 SYN-CHANNEL-CN-EXPORT 登到本机的合成报关服务商（declaration-broker/，ADR-0342 决定三）：dispatch 收到提交版本
# 已形成就经它发出去。没起它，dispatch 记「确知没发出去」、照常续办，重投预算用完就放弃这一封；不登这一行，发送答通道未配置、尝试停在已登记待发送。通道
# 不带凭据：合成服务商不认凭据，凭据那一格演示的是「不填」。
"$BIN/parcel-customs-register" declaration-channel -input "$SEEDS/customs/33-declaration-channel-cn-export-synthetic-broker.json"
