package environment

type chainCapabilities struct {
	miningControl bool
	nodeLifecycle bool
}

func deriveChainCapabilities(declaration ChainSpec) chainCapabilities {
	switch chain := declaration.(type) {
	case ManagedAnvil:
		return chainCapabilities{
			miningControl: chain.BlockInterval == 0,
			nodeLifecycle: true,
		}
	case ManagedBesu, AttachedEVM:
		return chainCapabilities{}
	default:
		return chainCapabilities{}
	}
}
