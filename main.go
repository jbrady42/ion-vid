package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"

	"github.com/cloudwebrtc/go-protoo/client"
	"github.com/cloudwebrtc/go-protoo/logger"
	"github.com/cloudwebrtc/go-protoo/peer"
	"github.com/cloudwebrtc/go-protoo/transport"
	"github.com/jbrady42/ion-vid/gst"
	"github.com/pion/webrtc/v2"
)

var (
	peerConnectionConfig = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	containerPath = ""

	audioTrack = &webrtc.Track{}
	videoTrack = &webrtc.Track{}
	pipeline   = &gst.Pipeline{}
)

type watchSrv struct {
	peerCon *webrtc.PeerConnection
}

func JsonEncode(str string) map[string]interface{} {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(str), &data); err != nil {
		panic(err)
	}
	return data
}

func JsonHack(a interface{}) map[string]interface{} {
	b, _ := json.Marshal(a)
	return JsonEncode(string(b))
}

type AcceptFunc func(data map[string]interface{})
type RejectFunc func(errorCode int, errorReason string)

var peerId = "go-client-id-xxxx"

type RoomInfo struct {
	Rid string `json:"rid"`
	Uid string `json:"uid"`
}

type ChatInfo struct {
	Msg        string `json:"msg"`
	SenderName string `json:"senderName"`
}

type UserInfo struct {
	Name string `json:"name"`
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
	RoomInfo
	Info UserInfo `json:"info"`
}

type ChatMsg struct {
	RoomInfo
	Info ChatInfo `json:"info"`
}

type PublishMsg struct {
	RoomInfo
	Jsep    webrtc.SessionDescription `json:"jsep"`
	Options PublishOptions            `json:"options"`
}

func newPublishOptions() PublishOptions {
	return PublishOptions{
		Codec:      "vp8",
		Resolution: "hd",
		Bandwidth:  1024,
		Audio:      true,
		Video:      true,
		Screen:     false,
	}
}

func (t watchSrv) handleWebSocketOpen(transport *transport.WebSocketTransport) {
	logger.Infof("handleWebSocketOpen")

	peer := peer.NewPeer(peerId, transport)
	peer.On("close", func(code int, err string) {
		logger.Infof("peer close [%d] %s", code, err)
	})

	handleRequest := func(request map[string]interface{}, accept AcceptFunc, reject RejectFunc) {
		method := request["method"]
		logger.Infof("handleRequest =>  (%s) ", method)
		if method == "kick" {
			reject(486, "Busy Here")
		} else {
			accept(JsonEncode(`{}`))
		}
	}

	handleNotification := func(notification map[string]interface{}) {
		logger.Infof("handleNotification => %s", notification["method"])
	}

	handleClose := func(code int, err string) {
		logger.Infof("handleClose => peer (%s) [%d] %s", peer.ID(), code, err)
	}

	peer.On("request", handleRequest)
	peer.On("notification", handleNotification)
	peer.On("close", handleClose)

	info := RoomInfo{Rid: "alice", Uid: peerId}
	joinMsg := JoinMsg{RoomInfo: info, Info: UserInfo{Name: "MyName"}}
	// chatMsg := ChatMsg{RoomInfo: info, Info: ChatInfo{Msg: "hello chat", SenderName: "alice"}}

	peer.Request("join", JsonHack(joinMsg),
		func(result map[string]interface{}) {
			logger.Infof("login success: =>  %s", result)
			// Add media stream
		},
		func(code int, err string) {
			logger.Infof("login reject: %d => %s", code, err)
		})

	t.publish(peer)

	// peer.Request("broadcast", JsonHack(chatMsg),
	// 	func(result map[string]interface{}) {
	// 		logger.Infof("login success: =>  %s", result)
	// 	},
	// 	func(code int, err string) {
	// 		logger.Infof("login reject: %d => %s", code, err)
	// 	})
	// peer.Request("offer", JsonEncode(`{"sdp":"empty"}`),
	// 	func(result map[string]interface{}) {
	// 		logger.Infof("offer success: =>  %s", result)
	// 	},
	// 	func(code int, err string) {
	// 		logger.Infof("offer reject: %d => %s", code, err)
	// 	})
	/*
		peer.Request("join", JsonEncode(`{"client":"aaa", "type":"sender"}`),
			func(result map[string]interface{}) {
				logger.Infof("join success: =>  %s", result)
			},
			func(code int, err string) {
				logger.Infof("join reject: %d => %s", code, err)
			})
		peer.Request("publish", JsonEncode(`{"type":"sender", "jsep":{"type":"offer", "sdp":"111111111111111"}}`),
			func(result map[string]interface{}) {
				logger.Infof("publish success: =>  %s", result)
			},
			func(code int, err string) {
				logger.Infof("publish reject: %d => %s", code, err)
			})
	*/
}

func (t watchSrv) publish(peer *peer.Peer) {
	// Get code from rtwatch and gstreamer
	//
	if _, err := t.peerCon.AddTrack(audioTrack); err != nil {
		log.Print(err)
		panic(err)
		return
	}
	if _, err := t.peerCon.AddTrack(videoTrack); err != nil {
		log.Print(err)
		panic(err)
		return
	}
	// } else i

	t.peerCon.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	})

	// Create an offer to send to the browser
	offer, err := t.peerCon.CreateOffer(nil)
	if err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = t.peerCon.SetLocalDescription(offer)
	if err != nil {
		panic(err)
	}
	log.Println(offer)

	info := RoomInfo{Rid: "alice", Uid: peerId}
	pubMsg := PublishMsg{RoomInfo: info, Jsep: offer, Options: newPublishOptions()}

	peer.Request("publish", JsonHack(pubMsg),
		func(result map[string]interface{}) {
			logger.Infof("publish success: =>  %s", result)

			ans := webrtc.SessionDescription{}

			// Hack hack
			jsep, _ := json.Marshal(result["jsep"])
			json.Unmarshal(jsep, &ans)

			// Set the remote SessionDescription
			err = t.peerCon.SetRemoteDescription(ans)
			if err != nil {
				panic(err)
			}

			pipeline = gst.CreatePipeline(containerPath, audioTrack, videoTrack)
			pipeline.Start()
		},
		func(code int, err string) {
			logger.Infof("publish reject: %d => %s", code, err)
		})

	// Creat sdp
	// Publish sdp
	// Set response
}

func main() {

	flag.StringVar(&containerPath, "container-path", "", "path to the media file you want to playback")
	flag.Parse()

	if containerPath == "" {
		panic("-container-path must be specified")
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	videoTrack, err = pc.NewTrack(webrtc.DefaultPayloadTypeVP8, rand.Uint32(), "synced-video", "synced-video")
	if err != nil {
		log.Fatal(err)
	}

	audioTrack, err = pc.NewTrack(webrtc.DefaultPayloadTypeOpus, rand.Uint32(), "synced-audio", "synced-video")
	if err != nil {
		log.Fatal(err)
	}

	watchS := watchSrv{pc}

	var wsClient = client.NewClient("ws://localhost:8080/ws?peer="+peerId, watchS.handleWebSocketOpen)
	wsClient.ReadMessage()
}
