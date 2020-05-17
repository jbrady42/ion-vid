package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pion/ion-load-tool/ion"
	"github.com/pion/ion-load-tool/producer"
	"github.com/pion/webrtc/v2"
	"github.com/pion/webrtc/v2/pkg/media"
)

type watchSrv struct {
	doneCh chan interface{}
	client ion.RoomClient
	name   string
	paused bool
	player producer.IFilePlayer
}

func (t *watchSrv) handleMessage(data json.RawMessage) {
	var msg ChatMsg
	err := json.Unmarshal(data, &msg)
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
		go t.trackPausedLoop(t.player.AudioTrack())
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
		t.player.Pause(false)
		t.setPaused(false)

	} else if contains([]string{"pause", "stop"}, cmd) {
		t.player.Pause(true)
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
		t.player.SeekP(int(time))
	}
}

func (t *watchSrv) runClient() {
	t.client.Init()
	t.client.Join()

	// Start producer
	t.client.Publish(t.player.VideoCodec())

	done := false
	for !done {
		select {
		case <-t.client.OnStreamAdd:
		case <-t.client.OnStreamRemove:
		case msg := <-t.client.OnBroadcast:
			t.handleMessage(msg)
		case <-t.doneCh:
			done = true
			continue
		}
	}

	// Close producer and sender
	t.player.Stop()
	t.client.UnPublish()

	// Close client
	t.client.Leave()
	t.client.Close()
}

func (t *watchSrv) setupClient(room, path, clientName, vidFile, fileType string) {
	t.client = ion.NewClient(clientName, room, path)
	t.doneCh = make(chan interface{})

	// Configure sender tracks
	if fileType == "webm" {
		t.player = producer.NewMFileProducer(vidFile, 0, producer.TrackSelect{
			Audio: true,
			Video: true,
		})
	} else if fileType == "ivf" {
		t.player = producer.NewIVFProducer(vidFile, 0)
	} else if fileType == "gst" {
		t.player = producer.NewGSTProducer(vidFile)
	}
	t.client.VideoTrack = t.player.VideoTrack()
	t.client.AudioTrack = t.player.AudioTrack()

	t.player.Start()
}

func main() {
	var containerPath string
	var ionPath string
	var roomName, clientName string

	flag.StringVar(&containerPath, "file", "", "path to the media file you want to playback")
	flag.StringVar(&ionPath, "ion-url", "ws://localhost:8443/ws", "websocket url for ion biz system")
	flag.StringVar(&roomName, "room", "video-demo", "Room name for Ion")
	flag.StringVar(&clientName, "name", "video-user", "Client name for Ion")
	flag.Parse()

	if containerPath == "" {
		panic("-file must be specified")
	}

	watchS := watchSrv{name: "Video User"}

	containerType, ok := producer.ValidateVPFile(containerPath)
	if !ok {
		containerType = "gst"
		log.Println("Direct playback not support. Using gstreamer")
	}

	// Setup shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		close(watchS.doneCh)
	}()

	watchS.setupClient(roomName, ionPath, clientName, containerPath, containerType)
	watchS.runClient()

	log.Println("Clean shutdown")
}
