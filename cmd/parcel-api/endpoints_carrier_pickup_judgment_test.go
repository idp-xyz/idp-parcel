package main

func init() {
	registerUnwired(carrierPickupJudgmentWiring{
		carrierPickupChain:    unwiredCarrierPickupChain{},
		carrierPickupJudgment: unwiredCarrierPickupJudgment{},
	})
}
