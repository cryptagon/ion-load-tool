package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cloudwebrtc/go-protoo/logger"
	"github.com/pion/ion-load-tool/ion"
	"github.com/pion/producer"
	"github.com/pion/producer/ivf"
	"github.com/pion/producer/webm"
)

var (
	waitGroup      sync.WaitGroup
	clientNameTmpl = "client_%v"
)

func init() {
	logger.SetLevel(logger.InfoLevel)
}

type testRun struct {
	client      ion.RoomClient
	consume     bool
	produce     bool
	mediaSource producer.IFileProducer
	doneCh      chan interface{}
	index       int
}

func (t *testRun) runClient() {
	defer waitGroup.Done()

	t.client.Init()
	t.client.Join()

	// Start producer
	if t.produce {
		log.Println("Video codes is", t.mediaSource.VideoCodec())
		t.client.Publish(t.mediaSource.VideoCodec())
	}

	// Wire consumers
	// Wait for the end of the test then shutdown
	done := false
	for !done {
		select {
		case msg := <-t.client.OnStreamAdd:
			if t.consume {
				t.client.Subscribe(msg)
			}
		case msg := <-t.client.OnStreamRemove:
			if t.consume {
				t.client.UnSubscribe(msg.MediaInfo)
			}
		case <-t.client.OnBroadcast:
		case <-t.doneCh:
			done = true
			break
		}
	}
	log.Printf("Begin client %v shutdown", t.index)

	// Close producer and sender
	if t.produce {
		t.mediaSource.Stop()
		t.client.UnPublish()
	}

	// Close client
	t.client.Leave()
	t.client.Close()
}

func (t *testRun) setupClient(room, path, vidFile, fileType string, Audio bool, accessToken string) {
	name := fmt.Sprintf(clientNameTmpl, t.index)
	t.client = ion.NewClient(name, room, path, accessToken)
	t.doneCh = make(chan interface{})

	if t.produce {
		// Configure sender tracks
		offset := t.index * 5
		if fileType == "webm" {
			t.mediaSource = webm.NewMFileProducer(vidFile, offset, producer.TrackSelect{
				Audio: Audio,
				Video: true,
			})
		} else if fileType == "ivf" {
			Audio = false
			t.mediaSource = ivf.NewIVFProducer(vidFile, offset)
		}
		t.client.VideoTrack = t.mediaSource.VideoTrack()
		if Audio {
			t.client.AudioTrack = t.mediaSource.AudioTrack()
		}
		t.mediaSource.Start()
	}

	go t.runClient()
}

func printReport(reportPath string, testRuns []*testRun) {
	var b bytes.Buffer
	fmt.Fprintln(&b, "***** REPORT *****")
	fmt.Fprintf(&b, "Clients: %v\n", len(testRuns))
	for _, test := range testRuns {
		pr := test.client.PeerReport
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "Peer: %v\n", pr.Name)
		fmt.Fprintf(&b, "  received streams: %v, tracks: %v\n", pr.StreamsRecvNum, pr.TracksRecvNum)
		fmt.Fprintf(&b, "  published audio: %t, video: %t, errors: %v, unpublished: %v\n", pr.Audio, pr.Video, pr.PublishError, pr.UnpublishCalls)
		fmt.Fprintf(&b, "  ICE Failures: %v, Disconnects: %v\n", pr.IceFailure, pr.IceDisconnect)
	}
	fmt.Fprintln(&b, "**** END ****")
	if reportPath != "" {
		err := ioutil.WriteFile(reportPath, b.Bytes(), 0644)
		if err != nil {
			log.Println("Failed to write report to " + reportPath + "!!!!!")
			log.Println(b.String())
		}
	} else {
		log.Println(b.String())
	}
}

func main() {
	var containerPath, containerType, accessToken string
	var ionPath, roomName, reportPath string
	var numClients, runSeconds, numRooms int
	var consume, produce bool
	var staggerSeconds float64
	var Audio bool

	flag.StringVar(&containerPath, "produce", "", "path to the media file you want to playback")
	flag.StringVar(&ionPath, "ion-url", "ws://localhost:8443/ws", "websocket url for ion biz system")
	flag.StringVar(&roomName, "room-name", "Video-demo", "Room name for Ion")
	flag.IntVar(&numClients, "clients", 1, "Number of clients to start")
	flag.Float64Var(&staggerSeconds, "stagger", 1.0, "Number of seconds to stagger client start and stop")
	flag.IntVar(&runSeconds, "seconds", 60, "Number of seconds to run test for")
	flag.BoolVar(&consume, "consume", false, "Run subscribe to all streams and consume data")
	flag.BoolVar(&Audio, "audio", false, "Publish Audio stream from webm file")
	flag.StringVar(&accessToken, "token", "", "Access token")
	flag.StringVar(&reportPath, "report", "", "test run report file path. if not provided report will be printed in stdout")
	flag.IntVar(&numRooms, "rooms", 1, "number of rooms (each room will have --clients number)")

	flag.Parse()

	produce = containerPath != ""

	// Validate type
	if produce {
		ext, ok := producer.ValidateVPFile(containerPath)
		log.Println(ext)
		if !ok {
			panic("Only IVF and WEBM containers are supported.")
		}
		containerType = ext
	}

	clients := make([]*testRun, numClients)
	staggerDur := time.Duration(staggerSeconds*1000) * time.Millisecond
	waitGroup.Add(numClients)

	for i := 0; i < numClients; i++ {
		cfg := &testRun{consume: consume, produce: produce, index: i}
		cfg.setupClient(roomName, ionPath, containerPath, containerType, Audio, accessToken)
		clients[i] = cfg
		time.Sleep(staggerDur)
	}

	// Setup shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	timer := time.NewTimer(time.Duration(runSeconds) * time.Second)

	select {
	case <-sigs:
	case <-timer.C:
	}

	for i, a := range clients {
		// Signal shutdown
		close(a.doneCh)
		// Staggered shutdown.
		if len(clients) > 1 && i < len(clients)-1 {
			time.Sleep(staggerDur)
		}
	}

	log.Println("Wait for client shutdown")
	waitGroup.Wait()
	log.Println("All clients shut down")
	printReport(reportPath, clients)
}
