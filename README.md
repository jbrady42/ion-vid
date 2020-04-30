## ION RTWatch Demo

### Setup

Follow gstreamer setup for [RTWatch](https://github.com/pion/rtwatch)

Have access to an [Ion](https://github.com/pion/ion) instance

### Run
`go run . -container-path <your video file> -room <room name>`

If your Ion server is not on localhost set the `-ion-url` param.

### Commands
The video can be controlled using the ion chat window via text commands.

```
@play
@pause
@seek <time in seconds>
```

### Status

Play and seek working.

#### Pause In progress
If the video is paused for more than 6s ion SFU will drop connection.
