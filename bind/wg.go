package bind

type WireGuardMessageType = uint8

const (
	WireGuardMessageInitiator WireGuardMessageType = iota + 1
	WireGuardMessageResponder
	_
	WireGuardMessageData
)
