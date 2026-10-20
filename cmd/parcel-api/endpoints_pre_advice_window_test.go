package main

func init() {
	registerUnwired(preAdviceWindowWiring{
		preAdviceWindowRegistration: unwiredPreAdviceWindowRegistration{},
		preAdviceWindowClosure:      unwiredPreAdviceWindowClosure{},
	})
}
