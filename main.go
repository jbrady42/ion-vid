package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"

	"github.com/cloudwebrtc/go-protoo/client"
	"github.com/cloudwebrtc/go-protoo/logger"
	"github.com/cloudwebrtc/go-protoo/peer"
	"github.com/cloudwebrtc/go-protoo/transport"
	"github.com/jbrady42/ion-vid/gst"
	"github.com/mitchellh/mapstructure"
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
	room    RoomInfo
	name    string
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
	RoomInfo `mapstructure:",squash"`
	Info     ChatInfo `json:"info"`
}

type PublishMsg struct {
	RoomInfo
	Jsep    webrtc.SessionDescription `json:"jsep"`
	Options PublishOptions            `json:"options"`
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

	// handleNotification := func(notification map[string]interface{}) {
	// 	logger.Infof("handleNotification => %s", notification["method"])
	// }

	handleClose := func(code int, err string) {
		logger.Infof("handleClose => peer (%s) [%d] %s", peer.ID(), code, err)
	}

	peer.On("request", handleRequest)
	peer.On("notification", t.handleMessage)
	peer.On("close", handleClose)

	joinMsg := JoinMsg{RoomInfo: t.room, Info: UserInfo{Name: t.name}}
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
}

func (t watchSrv) handleMessage(notification map[string]interface{}) {
	logger.Infof("handleNotification => %s", notification["method"])
	method := notification["method"].(string)
	if method != "broadcast" {
		return
	}
	var msg ChatMsg
	err := mapstructure.Decode(notification["data"], &msg)
	if err != nil {
		panic(err)
	}
	cmdStr := msg.Info.Msg
	if !strings.HasPrefix(cmdStr, "@") {
		return
	}
	t.handleCommand(strings.Trim(cmdStr, " @\n"))
}

func contains(p []string, search string) bool {
	for _, a := range p {
		if a == search {
			return true
		}
	}
	return false
}

func (t watchSrv) handleCommand(cmd string) {
	if contains([]string{"play", "start"}, cmd) {
		log.Println("Got COMMAND", cmd)
		pipeline.Play()
	} else if contains([]string{"pause", "stop"}, cmd) {
		pipeline.Pause()
	}
}

func (t watchSrv) publish(peer *peer.Peer) {
	// Get code from rtwatch and gstreamer
	if _, err := t.peerCon.AddTrack(audioTrack); err != nil {
		log.Print(err)
		panic(err)
	}
	if _, err := t.peerCon.AddTrack(videoTrack); err != nil {
		log.Print(err)
		panic(err)
	}

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

	peer.Request("publish", JsonHack(pubMsg), t.finalizeConnect,
		func(code int, err string) {
			logger.Infof("publish reject: %d => %s", code, err)
		})
}

func (t watchSrv) finalizeConnect(result map[string]interface{}) {
	logger.Infof("publish success: =>  %s", result)

	ans := webrtc.SessionDescription{}

	// Hack hack
	jsep, _ := json.Marshal(result["jsep"])
	json.Unmarshal(jsep, &ans)

	// Set the remote SessionDescription
	err := t.peerCon.SetRemoteDescription(ans)
	if err != nil {
		panic(err)
	}
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

	videoTrack, err = pc.NewTrack(webrtc.DefaultPayloadTypeH264, rand.Uint32(), "synced-video", "synced-video")
	if err != nil {
		log.Fatal(err)
	}

	audioTrack, err = pc.NewTrack(webrtc.DefaultPayloadTypeOpus, rand.Uint32(), "synced-audio", "synced-video")
	if err != nil {
		log.Fatal(err)
	}

	watchS := watchSrv{
		peerCon: pc,
		room:    RoomInfo{Rid: "alice", Uid: peerId},
		name:    "Video User",
	}

	pipeline = gst.CreatePipeline(containerPath, audioTrack, videoTrack)
	pipeline.Start()

	var wsClient = client.NewClient("wss://sup.chat.overcloud.org/ws?peer="+peerId, watchS.handleWebSocketOpen)
	wsClient.ReadMessage()
}
