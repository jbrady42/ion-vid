package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwebrtc/go-protoo/client"
	"github.com/cloudwebrtc/go-protoo/logger"
	"github.com/cloudwebrtc/go-protoo/peer"
	"github.com/cloudwebrtc/go-protoo/transport"
	"github.com/frankenbeanies/uuid4"
	"github.com/pion/rtwatch/gst"
	"github.com/pion/webrtc/v2"
	"github.com/pion/webrtc/v2/pkg/media"
)

type watchSrv struct {
	peerCon    *webrtc.PeerConnection
	room       RoomInfo
	name       string
	audioTrack *webrtc.Track
	videoTrack *webrtc.Track
	pipeline   *gst.Pipeline
	paused     bool
}

func (t *watchSrv) handleWebSocketOpen(transport *transport.WebSocketTransport) {
	logger.Infof("handleWebSocketOpen")

	pr := peer.NewPeer(t.room.Uid, transport)
	pr.On("close", func(code int, err string) {
		logger.Infof("peer close [%d] %s", code, err)
	})

	handleRequest := func(request peer.Request, accept peer.AcceptFunc, reject peer.RejectFunc) {
		method := request.Method
		logger.Infof("handleRequest =>  (%s) ", method)
		if method == "kick" {
			reject(486, "Busy Here")
		} else {
			accept(nil)
		}
	}

	handleClose := func(code int, err string) {
		logger.Infof("handleClose => peer (%s) [%d] %s", pr.ID(), code, err)
	}

	pr.On("request", handleRequest)
	pr.On("notification", t.handleMessage)
	pr.On("close", handleClose)

	joinMsg := JoinMsg{RoomInfo: t.room, Info: UserInfo{Name: t.name}}

	pr.Request("join", joinMsg,
		func(result json.RawMessage) {
			logger.Infof("login success: =>  %s", result)
			// Add media stream
			t.publish(pr)
		},
		func(code int, err string) {
			logger.Infof("login reject: %d => %s", code, err)
		})
}

func (t *watchSrv) handleMessage(notification peer.Notification) {
	logger.Infof("handleNotification => %s", notification.Method)
	method := notification.Method
	if method != "broadcast" {
		return
	}
	var msg ChatMsg
	err := json.Unmarshal(notification.Data, &msg)
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
		if strings.Contains(search, a) {
			return true
		}
	}
	return false
}

func (t *watchSrv) setPaused(paused bool) {
	if t.paused == paused {
		return
	}
	t.paused = paused
	if t.paused {
		go t.trackPausedLoop(t.audioTrack)
	}
}

func (t *watchSrv) trackPausedLoop(track *webrtc.Track) {
	log.Println("Send pause frame")
	for t.paused {
		// Produce empty sample
		track.WriteSample(media.Sample{Data: make([]byte, 8), Samples: 1})
		time.Sleep(500 * time.Millisecond)
	}
	log.Println("Exit pause frame")
}

func (t *watchSrv) handleCommand(cmd string) {
	log.Println("Got command", cmd)
	if contains([]string{"play", "start"}, cmd) {
		t.pipeline.Play()
		t.setPaused(false)
	} else if contains([]string{"pause", "stop"}, cmd) {
		t.pipeline.Pause()
		t.setPaused(true)
	} else if contains([]string{"seek"}, cmd) {
		list := strings.Split(cmd, " ")
		log.Println(list)
		if len(list) < 2 {
			return
		}
		time, err := strconv.ParseInt(list[1], 10, 64)
		if err != nil {
			log.Println("Error parsing seek string")
			return
		}
		t.pipeline.SeekToTime(time)
	}
}

func (t *watchSrv) publish(peer *peer.Peer) {
	// Get code from rtwatch and gstreamer
	if _, err := t.peerCon.AddTrack(t.audioTrack); err != nil {
		log.Print(err)
		panic(err)
	}
	if _, err := t.peerCon.AddTrack(t.videoTrack); err != nil {
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

	pubMsg := PublishMsg{RoomInfo: t.room, Jsep: offer, Options: newPublishOptions()}

	peer.Request("publish", pubMsg, t.finalizeConnect,
		func(code int, err string) {
			logger.Infof("publish reject: %d => %s", code, err)
		})
}

type connectMsg struct {
	Ans webrtc.SessionDescription `json:"jsep"`
}

func (t *watchSrv) finalizeConnect(result json.RawMessage) {
	logger.Infof("publish success: =>  %s", result)

	var msg connectMsg
	err := json.Unmarshal(result, &msg)
	if err != nil {
		log.Println(err)
		return
	}

	// Set the remote SessionDescription
	err = t.peerCon.SetRemoteDescription(msg.Ans)
	if err != nil {
		panic(err)
	}
}

func main() {
	var containerPath string
	var ionPath string
	var roomName string

	flag.StringVar(&containerPath, "container-path", "", "path to the media file you want to playback")
	flag.StringVar(&ionPath, "ion-url", "ws://localhost:8443/ws", "websocket url for ion biz system")
	flag.StringVar(&roomName, "room", "video-demo", "Room name for Ion")
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

	videoTrack, err := pc.NewTrack(webrtc.DefaultPayloadTypeH264, rand.Uint32(), "synced-video", "synced-video")
	if err != nil {
		log.Fatal(err)
	}

	audioTrack, err := pc.NewTrack(webrtc.DefaultPayloadTypeOpus, rand.Uint32(), "synced-audio", "synced-video")
	if err != nil {
		log.Fatal(err)
	}

	pipeline := gst.CreatePipeline(containerPath, audioTrack, videoTrack)
	pipeline.Start()

	uuidStr := uuid4.New().String()
	peerId := "video-client-" + uuidStr

	watchS := watchSrv{
		peerCon:    pc,
		room:       RoomInfo{Rid: roomName, Uid: peerId},
		name:       "Video User",
		videoTrack: videoTrack,
		audioTrack: audioTrack,
		pipeline:   pipeline,
	}

	var wsClient = client.NewClient(ionPath+"?peer="+peerId, watchS.handleWebSocketOpen)
	wsClient.ReadMessage()
}
