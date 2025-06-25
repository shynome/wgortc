package pubkey

import (
	"bytes"
	"encoding/binary"

	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"github.com/shynome/wgortc/device/vtun"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
)

func Initiator(sk device.NoisePrivateKey, pk device.NoisePublicKey) (_ []byte, err error) {
	defer err0.Then(&err, nil, nil)

	tdev := try.To1(vtun.CreateTUN("vtun", device.DefaultMTU))
	b := conn.NewStdNetBind()
	lg := device.NewLogger(device.LogLevelSilent, "")
	dev := device.NewDevice(tdev, b, lg)
	try.To(dev.SetPrivateKey(sk))
	p := try.To1(dev.NewPeer(pk))
	// p.SendHandshakeInitiation(false)
	msg := try.To1(dev.CreateMessageInitiation(p))
	buf := [device.MessageInitiationSize]byte{}
	w := bytes.NewBuffer(buf[:0])
	binary.Write(w, binary.LittleEndian, msg)
	packet := w.Bytes()

	return packet, nil
}
