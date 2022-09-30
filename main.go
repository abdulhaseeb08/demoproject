package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tinyzimmer/go-glib/glib"
	"github.com/tinyzimmer/go-gst/examples"
	"github.com/tinyzimmer/go-gst/gst"
	"github.com/tinyzimmer/go-gst/gst/app"
)

func buildPipeline() (*gst.Pipeline, error) {

	//initialize gstreamer
	gst.Init(nil)

	//create a new pipeline
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return nil, err
	}

	elementsForVideo, err := gst.NewElementMany("v4l2src", "queue", "videoconvert", "videorate", "videoscale", "capsfilter", "queue", "x264enc", "queue")
	if err != nil {
		return nil, err
	}

	//Setting properties and caps
	elementsForVideo[3].Set("silent", false)
	if err := elementsForVideo[5].SetProperty("caps", gst.NewCapsFromString(
		"video/x-raw, width=1280, height=720, framerate=25/1",
	)); err != nil {
		return nil, err
	}

	elementsForVideo[7].Set("speed-preset", 3)
	elementsForVideo[7].Set("tune", "zerolatency")
	elementsForVideo[7].Set("bitrate", 3800)
	elementsForVideo[7].Set("key-int-max", 0)

	videoQueue := elementsForVideo[len(elementsForVideo)-1]
	pipeline.AddMany(elementsForVideo...)
	//linking video elements
	gst.ElementLinkMany(elementsForVideo...)

	elementsForAudio, err := gst.NewElementMany("openalsrc", "queue", "audioconvert", "audioresample", "audiorate", "capsfilter", "queue", "fdkaacenc", "queue")
	if err != nil {
		return nil, err
	}

	//Setting properties and caps
	if err := elementsForAudio[5].SetProperty("caps", gst.NewCapsFromString(
		"audio/x-raw, rate=48000, channels=2",
	)); err != nil {
		return nil, err
	}
	elementsForAudio[7].Set("bitrate", 128000)

	audioQueue := elementsForAudio[len(elementsForAudio)-1]
	pipeline.AddMany(elementsForAudio...)
	//linking audio elements
	gst.ElementLinkMany(elementsForAudio...)

	mux, err := gst.NewElement("mp4mux")
	if err != nil {
		return nil, err
	}
	mux.Set("streamable", true)
	pipeline.Add(mux)

	tee, err := gst.NewElement("tee")
	if err != nil {
		return nil, err
	}
	pipeline.Add(tee)

	teeQueues, err := gst.NewElementMany("queue", "queue")
	if err != nil {
		return nil, err
	}
	pipeline.AddMany(teeQueues...)

	filesinkOne, err := gst.NewElement("filesink")
	if err != nil {
		return nil, err
	}
	filesinkTwo, err := gst.NewElement("filesink")
	if err != nil {
		return nil, err
	}
	filesinkOne.Set("location", "fileOne.mp4")
	filesinkTwo.Set("location", "fileTwo.mp4")
	pipeline.AddMany(filesinkOne, filesinkTwo)
	//linking the mux and tee
	mux.Link(tee)

	//linking the audio and video branches to the mux sink
	muxAudioPad := mux.GetRequestPad("audio_%u")
	muxVideoPad := mux.GetRequestPad("video_%u")
	audioQueuePad := audioQueue.GetStaticPad("src")
	videoQueuePad := videoQueue.GetStaticPad("src")

	audioQueuePad.Link(muxAudioPad)
	videoQueuePad.Link(muxVideoPad)

	//linking the tee and the filesinks
	teePadOne := tee.GetRequestPad("src_%u")
	teeQueuePadOne := teeQueues[0].GetStaticPad("sink")
	teePadOne.Link(teeQueuePadOne)

	teePadTwo := tee.GetRequestPad("src_%u")
	teeQueuePadTwo := teeQueues[1].GetStaticPad("sink")
	teePadTwo.Link(teeQueuePadTwo)

	//now connecting queue src to the filesink
	teeQueues[0].Link(filesinkOne)
	teeQueues[1].Link(filesinkTwo)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		for sig := range ch {
			switch sig {
			case syscall.SIGINT:
				fmt.Println("Sending EOS")
				pipeline.SendEvent(gst.NewEOSEvent())
				return
			}
		}
	}()

	return pipeline, nil
}

func handleMessage(msg *gst.Message) error {
	switch msg.Type() {
	case gst.MessageEOS:
		return app.ErrEOS
	case gst.MessageError:
		gerr := msg.ParseError()
		if debug := gerr.DebugString(); debug != "" {
			fmt.Println(debug)
		}
		return gerr
	}
	return nil
}

func mainLoop(loop *glib.MainLoop, pipeline *gst.Pipeline) error {
	// Start the pipeline

	// Due to recent changes in the bindings - the finalizers might fire on the pipeline
	// prematurely when it's passed between scopes. So when you do this, it is safer to
	// take a reference that you dispose of when you are done. There is an alternative
	// to this method in other examples.
	pipeline.Ref()
	defer pipeline.Unref()

	pipeline.SetState(gst.StatePlaying)

	// Retrieve the bus from the pipeline and add a watch function
	pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		if err := handleMessage(msg); err != nil {
			fmt.Println(err)
			loop.Quit()
			return false
		}
		return true
	})

	loop.Run()

	return nil
}

func main() {
	examples.RunLoop(func(loop *glib.MainLoop) error {
		var pipeline *gst.Pipeline
		var err error
		if pipeline, err = buildPipeline(); err != nil {
			return err
		}
		return mainLoop(loop, pipeline)
	})
}
