package attestor

// LocalAttestor provides attestation data from the local process.
type LocalAttestor struct{}

func NewLocal() *LocalAttestor {
	return &LocalAttestor{}
}

// todo
