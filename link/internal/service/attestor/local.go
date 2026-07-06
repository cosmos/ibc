package attestor

type LocalAttestor struct{}

func NewLocal() *LocalAttestor {
	return &LocalAttestor{}
}
