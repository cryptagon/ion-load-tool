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
)

var (
	waitGroup      sync.WaitGroup
	clientNameTmpl = "client_%v"
)

func init() {
	logger.SetLevel(logger.InfoLevel)
}

func printReport(reportPath string, roomRun *ion.RoomRun) {
	var b bytes.Buffer
	fmt.Fprintln(&b, "***** REPORT *****")
	fmt.Fprintf(&b, "Clients: %v\n", len(roomRun.RoomReport))
	for _, pr := range roomRun.RoomReport {
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

func getSuffix(roomNum, index int) string {
	if roomNum == 1 {
		return ""
	}
	if index < 10 {
		return fmt.Sprintf("_0%d", index+1)
	}

	return fmt.Sprintf("_%d", index+1)
}

func main() {
	// var containerPath, containerType, accessToken string
	// var ionPath, roomName, reportPath string
	// var numClients, runSeconds, numRooms int
	// var consume, produce bool
	// var staggerSeconds float64
	// var Audio bool
	var numRooms int
	var staggerSeconds float64
	var r ion.RoomData

	flag.StringVar(&r.ContainerPath, "produce", "", "path to the media file you want to playback")
	flag.StringVar(&r.IonPath, "ion-url", "ws://localhost:8443/ws", "websocket url for ion biz system")
	flag.StringVar(&r.RoomName, "room-name", "Video-demo", "Room name for Ion")
	flag.IntVar(&r.NumClients, "clients", 1, "Number of clients to start")
	flag.Float64Var(&staggerSeconds, "stagger", 1.0, "Number of seconds to stagger client start and stop")
	flag.IntVar(&r.RunSeconds, "seconds", 60, "Number of seconds to run test for")
	flag.BoolVar(&r.Consume, "consume", false, "Run subscribe to all streams and consume data")
	flag.BoolVar(&r.Audio, "audio", false, "Publish Audio stream from webm file")
	flag.StringVar(&r.AccessToken, "token", "", "Access token")
	flag.StringVar(&r.ReportPath, "report", "", "test run report file path. if not provided report will be printed in stdout")
	flag.IntVar(&numRooms, "rooms", 1, "number of rooms (each room will have --clients number)")

	flag.Parse()

	r.Produce = r.ContainerPath != ""
	r.StaggerDuration = time.Duration(staggerSeconds*1000) * time.Millisecond

	// Validate type
	if r.Produce {
		ext, ok := producer.ValidateVPFile(r.ContainerPath)
		log.Println(ext)
		if !ok {
			panic("Only IVF and WEBM containers are supported.")
		}
		r.ContainerType = ext
	}

	rooms := make([]*ion.RoomRun, numRooms)
	waitGroup.Add(numRooms)
	roomName := r.RoomName

	for i := 0; i < numRooms; i++ {
		rData := r
		rData.RoomName = fmt.Sprintf("%v%v", roomName, getSuffix(numRooms, i))
		roomRun := &ion.RoomRun{}
		go roomRun.Run(&rData, &waitGroup)
		rooms[i] = roomRun
		time.Sleep(rData.StaggerDuration)
	}

	timer := time.NewTimer(time.Duration(r.RunSeconds) * time.Second)

	// Setup shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigs:
	case <-timer.C:
	}

	// make loop to stop all rooms
	for _, r := range rooms {
		r.Stop()
	}

	waitGroup.Wait()

	// log.Println("Wait for client shutdown")
	// waitGroup.Wait()
	log.Println("Done")
	// printReport(r.ReportPath, roomRun)
}
