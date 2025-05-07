package whip

import "github.com/pion/webrtc/v4"

type SDP = webrtc.SessionDescription

type Payload[T any] struct {
	Type PaylodType
	Data T
}

type PaylodType string

const (
	PyalodTypeAnswer    PaylodType = "answer"
	PyalodTypeOffer     PaylodType = "offer"
	PyalodTypeCandidate PaylodType = "candidate"
)

type Candidate = Payload[webrtc.ICECandidateInit]

func PayloadCandidate(c webrtc.ICECandidateInit) Candidate {
	return Candidate{
		Type: PyalodTypeCandidate,
		Data: c,
	}
}

type PayloadSDP = Payload[SDP]

func PayloadOffer(sdp webrtc.SessionDescription) PayloadSDP {
	return PayloadSDP{
		Type: PyalodTypeOffer,
		Data: sdp,
	}
}

func PayloadAnswer(sdp webrtc.SessionDescription) PayloadSDP {
	return PayloadSDP{
		Type: PyalodTypeAnswer,
		Data: sdp,
	}
}
