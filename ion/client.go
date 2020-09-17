package ion

import (
	"encoding/json"
	"log"

	"github.com/cloudwebrtc/go-protoo/client"
	"github.com/cloudwebrtc/go-protoo/logger"
	"github.com/cloudwebrtc/go-protoo/peer"
	"github.com/cloudwebrtc/go-protoo/transport"
	"github.com/google/uuid"
	"github.com/pion/ion/pkg/proto"
	"github.com/pion/webrtc/v2"
)

var (
	IceServers = []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	}
)

type ClientChans struct {
	OnStreamAdd    chan proto.StreamAddMsg
	OnStreamRemove chan proto.StreamRemoveMsg
	OnBroadcast    chan json.RawMessage
}

type Consumer struct {
	Pc   *webrtc.PeerConnection
	Info proto.MediaInfo
}

type RoomClient struct {
	proto.MediaInfo
	ClientChans
	pubPeerCon *webrtc.PeerConnection
	WsPeer     *peer.Peer
	room       proto.RoomInfo
	name       string
	token      string
	AudioTrack *webrtc.Track
	VideoTrack *webrtc.Track
	paused     bool
	ionPath    string
	ReadyChan  chan bool
	client     *client.WebSocketClient
	consumers  []*Consumer
}

func newPeerCon() *webrtc.PeerConnection {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: IceServers,
	})
	if err != nil {
		log.Fatal(err)
	}
	return pc
}

func NewClient(name, room, path string, accessToken string) RoomClient {
	pc := newPeerCon()
	uidStr := name
	uuid, err := uuid.NewRandom()
	if err != nil {
		log.Println("Can't make new uuid??", err)
	} else {
		uidStr = uuid.String()
	}

	return RoomClient{
		ClientChans: ClientChans{
			OnStreamAdd:    make(chan proto.StreamAddMsg, 100),
			OnStreamRemove: make(chan proto.StreamRemoveMsg, 100),
			OnBroadcast:    make(chan json.RawMessage, 100),
		},
		pubPeerCon: pc,
		room: proto.RoomInfo{
			Uid: uidStr,
			Rid: room,
		},
		token:     accessToken,
		name:      name,
		ionPath:   path,
		ReadyChan: make(chan bool),
		consumers: make([]*Consumer, 0),
	}
}

func (t *RoomClient) Init() {
	t.client = client.NewClient(t.ionPath+"?peer="+t.room.Uid+"&access_token="+t.token, t.handleWebSocketOpen)
}

func (t *RoomClient) handleWebSocketOpen(transport *transport.WebSocketTransport) {
	logger.Infof("handleWebSocketOpen")

	t.WsPeer = peer.NewPeer(t.room.Uid, transport)

	go func() {
		for {
			select {
			case msg := <-t.WsPeer.OnNotification:
				t.handleNotification(msg)
			case msg := <-t.WsPeer.OnRequest:
				log.Println("Got request", msg)
			case msg := <-t.WsPeer.OnClose:
				log.Println("Peer close msg", msg)
			}
		}
	}()

}

func (t *RoomClient) Join() {
	joinMsg := proto.JoinMsg{RoomInfo: t.room, Info: proto.ClientUserInfo{Name: t.name}}
	res := <-t.WsPeer.Request(proto.ClientJoin, joinMsg, nil, nil)

	if res.Err != nil {
		logger.Infof("login reject: %d => %s", res.Err.Code, res.Err.Text)
	} else {
		logger.Infof("login success: =>  %s", res.Result)
	}
}

// TODO grab the codec from track
func (t *RoomClient) Publish(codec string) {
	if t.AudioTrack != nil {
		if _, err := t.pubPeerCon.AddTrack(t.AudioTrack); err != nil {
			log.Print(err)
			panic(err)
		}
	}
	if t.VideoTrack != nil {
		if _, err := t.pubPeerCon.AddTrack(t.VideoTrack); err != nil {
			log.Print(err)
			panic(err)
		}
	}

	t.pubPeerCon.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("Client %v producer State has changed %s \n", t.name, connectionState.String())
	})

	// Create an offer to send to the browser
	offer, err := t.pubPeerCon.CreateOffer(nil)
	if err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = t.pubPeerCon.SetLocalDescription(offer)
	if err != nil {
		panic(err)
	}

	pubMsg := proto.PublishMsg{
		RoomInfo: t.room,
		RTCInfo:  proto.RTCInfo{Jsep: offer},
		Options:  newPublishOptions(codec),
	}

	res := <-t.WsPeer.Request(proto.ClientPublish, pubMsg, nil, nil)
	if res.Err != nil {
		logger.Infof("publish reject: %d => %s", res.Err.Code, res.Err.Text)
		return
	}

	var msg proto.PublishResponseMsg
	err = json.Unmarshal(res.Result, &msg)
	if err != nil {
		log.Println(err)
		return
	}

	t.MediaInfo = msg.MediaInfo

	// Set the remote SessionDescription
	err = t.pubPeerCon.SetRemoteDescription(msg.Jsep)
	if err != nil {
		panic(err)
	}
}

func (t *RoomClient) handleNotification(msg peer.Notification) {
	switch msg.Method {
	case proto.ClientOnStreamAdd:
		t.handleStreamAdd(msg.Data)
	case proto.ClientOnStreamRemove:
		t.handleStreamRemove(msg.Data)
	case proto.ClientBroadcast:
		t.OnBroadcast <- msg.Data
	}
}

func (t *RoomClient) handleStreamAdd(msg json.RawMessage) {
	var msgData proto.StreamAddMsg
	if err := json.Unmarshal(msg, &msgData); err != nil {
		log.Println("Marshal error", err)
		return
	}
	log.Println("New stream", msgData)
	t.OnStreamAdd <- msgData
}

func (t *RoomClient) handleStreamRemove(msg json.RawMessage) {
	var msgData proto.StreamRemoveMsg
	if err := json.Unmarshal(msg, &msgData); err != nil {
		log.Println("Marshal error", err)
		return
	}
	log.Println("Remove stream", msgData)
	t.OnStreamRemove <- msgData
}

func (t *RoomClient) subcribe(mid string) {

}

func (t *RoomClient) UnPublish() {
	msg := proto.UnpublishMsg{MediaInfo: t.MediaInfo}
	res := <-t.WsPeer.Request(proto.ClientUnPublish, msg, nil, nil)
	if res.Err != nil {
		logger.Infof("unpublish reject: %d => %s", res.Err.Code, res.Err.Text)
		return
	}

	// Stop producer peer connection
	t.pubPeerCon.Close()
}

func (t *RoomClient) Subscribe(subData proto.StreamAddMsg) {
	info := subData.MediaInfo
	log.Println("Subscribing to ", info)
	id := len(t.consumers) // broken make better
	codec := ""
	// Find codec of first video track
	for _, trackList := range subData.Tracks {
		if len(trackList) == 0 {
			continue
		}
		track := trackList[0]
		if track.Type == "video" {
			codec = track.Codec
			break
		}
	}

	// Create peer connection
	pc := newConsumerPeerCon(t.name, id, codec)
	// Create an offer to send to the browser
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = pc.SetLocalDescription(offer)
	if err != nil {
		panic(err)
	}

	// Send subscribe requestv
	req := proto.SubscribeMsg{
		MediaInfo: info,
		RTCInfo:   proto.RTCInfo{Jsep: offer},
	}
	res := <-t.WsPeer.Request(proto.ClientSubscribe, req, nil, nil)
	if res.Err != nil {
		logger.Infof("unpublish reject: %d => %s", res.Err.Code, res.Err.Text)
		return
	}

	var msg proto.SubscribeResponseMsg
	err = json.Unmarshal(res.Result, &msg)
	if err != nil {
		log.Println(err)
		return
	}

	// Set the remote SessionDescription
	err = pc.SetRemoteDescription(msg.Jsep)
	if err != nil {
		panic(err)
	}

	// Create consumer
	consumer := &Consumer{pc, info}
	t.consumers = append(t.consumers, consumer)

	log.Println("Subscribe complete")
}

func (t *RoomClient) UnSubscribe(info proto.MediaInfo) {
	// Send upsubscribe request
	// Shut down peerConnection
	var sub *Consumer
	for _, a := range t.consumers {
		if a.Info.MID == info.MID {
			sub = a
			break
		}
	}
	if sub != nil && sub.Pc != nil {
		log.Println("Closing subscription peerConnection")
		sub.Pc.Close()
	}
}

func (t *RoomClient) Leave() {

}

// Shutdown client and websocket transport
func (t *RoomClient) Close() {
	t.client.Close()

	// Close any remaining consumers
	for _, sub := range t.consumers {
		sub.Pc.Close()
	}
}
