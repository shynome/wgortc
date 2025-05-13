package pubkey

import (
	"bytes"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/shynome/err0"
	"github.com/shynome/err0/try"
	"golang.org/x/crypto/blake2s"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tai64n"
)

func Unpack(privkey device.NoisePrivateKey, initiator []byte) (_ device.NoisePublicKey, err error) {
	defer err0.Then(&err, nil, nil)

	var msg device.MessageInitiation
	reader := bytes.NewReader(initiator)
	err = binary.Read(reader, binary.LittleEndian, &msg)
	try.To(err)

	var (
		hash     [blake2s.Size]byte
		chainKey [blake2s.Size]byte
	)

	pubkey := publicKey(&privkey)

	mixHash(&hash, &device.InitialHash, pubkey[:])
	mixHash(&hash, &hash, msg.Ephemeral[:])
	mixKey(&chainKey, &device.InitialChainKey, msg.Ephemeral[:])

	// decrypt static key
	var peerPK device.NoisePublicKey
	var key [chacha20poly1305.KeySize]byte
	ss, err := sharedSecret(&privkey, msg.Ephemeral)
	try.To(err)

	device.KDF2(&chainKey, &key, chainKey[:], ss[:])

	aead, _ := chacha20poly1305.New(key[:])
	_, err = aead.Open(peerPK[:0], device.ZeroNonce[:], msg.Static[:], hash[:])

	mixHash(&hash, &hash, msg.Static[:])

	// verify identity
	var timestamp tai64n.Timestamp
	precomputedStaticStatic := try.To1(sharedSecret(&privkey, peerPK))
	device.KDF2(
		&chainKey,
		&key,
		chainKey[:],
		precomputedStaticStatic[:],
	)
	aead, _ = chacha20poly1305.New(key[:])
	_, err = aead.Open(timestamp[:0], device.ZeroNonce[:], msg.Timestamp[:], hash[:])
	try.To(err)
	mixHash(&hash, &hash, msg.Timestamp[:])

	// protect against replay
	t := toTime(timestamp)
	replay := time.Since(t) >= WebRTCConnectTimeout
	if replay {
		err0.Throw(fmt.Errorf("%v - ConsumeMessageInitiation: handshake replay @ %v", peerPK, timestamp))
		return
	}

	return peerPK, nil
}

const WebRTCConnectTimeout = 30 * time.Second

var timestampBase = uint64(0x400000000000000a)

func toTime(t tai64n.Timestamp) time.Time {
	return time.Unix(int64(binary.BigEndian.Uint64(t[:8])-timestampBase), int64(binary.BigEndian.Uint32(t[8:12])))
}

func mixHash(dst, h *[blake2s.Size]byte, data []byte) {
	hash, _ := blake2s.New256(nil)
	hash.Write(h[:])
	hash.Write(data)
	hash.Sum(dst[:0])
	hash.Reset()
}

func mixKey(dst, c *[blake2s.Size]byte, data []byte) {
	device.KDF1(dst, c[:], data)
}

func sharedSecret(sk *device.NoisePrivateKey, pk device.NoisePublicKey) (ss [device.NoisePublicKeySize]byte, err error) {
	apk := (*[device.NoisePublicKeySize]byte)(&pk)
	ask := (*[device.NoisePrivateKeySize]byte)(sk)
	curve25519.ScalarMult(&ss, ask, apk)
	if isZero(ss[:]) {
		return ss, fmt.Errorf("errInvalidPublicKey")
	}
	return ss, nil
}

func isZero(val []byte) bool {
	acc := 1
	for _, b := range val {
		acc &= subtle.ConstantTimeByteEq(b, 0)
	}
	return acc == 1
}

func publicKey(sk *device.NoisePrivateKey) (pk device.NoisePublicKey) {
	if sk == nil {
		return
	}
	apk := (*[device.NoisePublicKeySize]byte)(&pk)
	ask := (*[device.NoisePrivateKeySize]byte)(sk)
	curve25519.ScalarBaseMult(apk, ask)
	return
}
