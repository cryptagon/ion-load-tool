package ion

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/producer"
	"github.com/pion/producer/ivf"
	"github.com/pion/producer/webm"
)

var (
	clientsWaitGroup sync.WaitGroup
	clientNameTmpl   = "client_%v"
)

type testRun struct {
	client      RoomClient
	consume     bool
	produce     bool
	mediaSource producer.IFileProducer
	doneCh      chan interface{}
	index       int
}

type RoomReport struct {
	RoomName string        `json:"room"`
	Reports  []*PeerReport `json:"reports"`
}

type RoomRun struct {
	RoomReport
	doneCh chan interface{}
}

type RoomData struct {
	ContainerPath, ContainerType, AccessToken string
	IonPath, RoomName, ReportPath             string
	NumClients, RunSeconds, NumRooms          int
	Consume, Produce                          bool
	StaggerDuration                           time.Duration
	Audio                                     bool
}

func (t *testRun) runClient() {
	defer clientsWaitGroup.Done()

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
	t.client = NewClient(name, room, path, accessToken)
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

func (t *RoomRun) Stop() {
	close(t.doneCh)
}

func (t *RoomRun) Run(r *RoomData, roomWaitGroup *sync.WaitGroup) {
	defer roomWaitGroup.Done()

	t.RoomReport = RoomReport{Reports: make([]*PeerReport, r.NumClients), RoomName: r.RoomName}
	t.doneCh = make(chan interface{})

	clients := make([]*testRun, r.NumClients)

	clientsWaitGroup.Add(r.NumClients)
	log.Println("Adding", r.NumClients, "clients to room:", r.RoomName)
	for i := 0; i < r.NumClients; i++ {
		cfg := &testRun{consume: r.Consume, produce: r.Produce, index: i}
		cfg.setupClient(r.RoomName, r.IonPath, r.ContainerPath, r.ContainerType, r.Audio, r.AccessToken)
		clients[i] = cfg
		time.Sleep(r.StaggerDuration)
	}

	select {
	case <-t.doneCh:
	}

	for i, a := range clients {
		// Signal shutdown
		close(a.doneCh)
		// Staggered shutdown.
		if len(clients) > 1 && i < len(clients)-1 {
			time.Sleep(r.StaggerDuration)
		}
		// Add report
		t.RoomReport.Reports[i] = a.client.PeerReport
	}

	log.Println("Wait for client shutdown")
	clientsWaitGroup.Wait()
	log.Println("Room:", r.RoomName, "all clients shut down")
}
