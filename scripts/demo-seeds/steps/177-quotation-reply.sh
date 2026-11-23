echo "== 报价答复：给已经发出的那条 SYN- 报价登一条接受（证据 S，只存索引） =="
# 发出记录、相对方与价卡都是前面步骤已经灌进去的。这里不新造价格金额，也不把证据文件放进产品库。
"$BIN/parcel-commercial" register-quotation-reply -input "$SEEDS/commercial/register-quotation-reply.json"
