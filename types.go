package main

import "github.com/pion/ion/pkg/node/biz"

type ChatInfo struct {
	Msg        string `json:"msg"`
	SenderName string `json:"senderName"`
}

type ChatMsg struct {
	biz.RoomInfo
	Info ChatInfo `json:"info"`
}
