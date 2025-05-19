package nat

type Empty struct{}

var _ INAT = (*Empty)(nil)

func (Empty) NAT(buf []byte) {}
