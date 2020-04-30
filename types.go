package main

import "github.com/pion/webrtc/v2"

type AcceptFunc func(data map[string]interface{})
type RejectFunc func(errorCode int, errorReason string)

var peerId = "go-client-id-xxxx"

type RoomInfo struct {
	Rid string `mapstructure:"rid"`
	Uid string `mapstructure:"uid"`
}

type ChatInfo struct {
	Msg        string `mapstructure:"msg"`
	SenderName string `mapstructure:"senderName"`
}

type UserInfo struct {
	Name string `mapstructure:"name"`
}

type PublishOptions struct {
	Codec      string `json:"codec"`
	Resolution string `json:"resolution"`
	Bandwidth  int    `json:"bandwidth"`
	Audio      bool   `json:"audio"`
	Video      bool   `json:"video"`
	Screen     bool   `json:"screen"`
}

type JoinMsg struct {
	RoomInfo `mapstructure:",squash"`
	Info     UserInfo `mapstructure:"info"`
}

type ChatMsg struct {
	RoomInfo `mapstructure:",squash"`
	Info     ChatInfo `mapstructure:"info"`
}

type PublishMsg struct {
	RoomInfo `json:",squash"`
	Jsep     webrtc.SessionDescription `json:"jsep"`
	Options  PublishOptions            `json:"options"`
}

func newPublishOptions() PublishOptions {
	return PublishOptions{
		Codec:      "h264",
		Resolution: "hd",
		Bandwidth:  1024,
		Audio:      true,
		Video:      true,
		Screen:     false,
	}
}
