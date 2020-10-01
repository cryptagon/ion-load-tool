## Ion load testing tool

### Test Data
Publishing of files in the following formats are supported.

|Container|Video Codecs|Audio|
|---|---|---|
IVF| VP8 | none
WEBM| VP8 and VP9 | ogg



If your data is not in this format you can transcode with:
```
ffmpeg -i $INPUT_FILE -g 30 output.(ivf|webm)
```

See the ffmpeg docs on [VP8](https://trac.ffmpeg.org/wiki/Encode/VP8) or [VP9](https://trac.ffmpeg.org/wiki/Encode/VP9) for encoding options

### Run

`ion-load-tool -clients <num clients>`


#### Producer

Pass `-produce <encoded video>`

Each client starts stream playback in an offset position from the last with a different track and SSRC ID to simulate independent content.


#### Consumers

Pass `-consume`

By default load test is only producing streams, use this flag to also subscribe to all remote streams and consume them

#### ION Url

Pass `-ion-url`

#### Room name

Pass `-room-name`

In case of testing only with one room, client will connect to room with provided name. In case of multi-room test, this name will be used as name prefix (followed by room number)

#### Duration

Pass `-seconds`

Number of seconds to run test (default is 60)

#### Audio

Pass `-audio`

By default audio is not sent from the stream

#### Access Token

Pass `-token`

If ION requires authentication with JWT, pass secret key here used to authenticate

#### Report

Pass `-report`

Filename for report output. If ommited, report will be printed in stdout

#### Multi rooms

Pass `-rooms`

NUmber of rooms to create. Each room will have number of clients provided in -clients



Each client subscribes to all published streams in the provided room. A basic consumer with simple out-of-order detection is implemented.


### Test Configurations

#### N to N fully connected

Run both produce and consume on the same command

`-produce <file> -consume -clients N`

This creates N clients publishing a stream, each of which will subscribe to the other N-1 client streams.

#### 1 to N fanout

Run separate instances of the load tool.

##### Producer

`-produce <file> -clients 1`

##### Consumer
`-consume -clients N`

#### Run 4 to 4 fully connected and streaming clients in 4 rooms (and log in report)
`-consume -produce ./big-buck-bunny_trailer.webm -token <secret_key> -rooms 4 -clients 4 -report ./report.json`
