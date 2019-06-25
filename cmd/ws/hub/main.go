// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"

	"log"
	"net/http"
	"os"
	"time"

	"github.com/n3wscott/reverse-skydiving/pkg/hub"
)

var kodata = "/Users/nicholss/src/github.com/reverse-skydiving/cmd/ws/hub/kodata"

var addr = flag.String("addr", ":8080", "http service address")

func serveHome(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "home.html")
}

func main() {
	flag.Parse()
	server := hub.NewHub()
	go server.Run()
	go main2(server)
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWs(server, w, r)
	})
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

type Queue struct {
	streamers []beep.Streamer

	Hub *hub.Hub
}

func (q *Queue) Add(streamers ...beep.Streamer) {
	q.streamers = append(q.streamers, streamers...)
}

func (q *Queue) Stream(outsamples [][2]float64) (n int, ok bool) {
	// We use the filled variable to track how many samples we've
	// successfully filled already. We loop until all samples are filled.

	samples := make([][2]float64, len(outsamples))
	for i := range outsamples {
		outsamples[i][0] = 0
		outsamples[i][1] = 0
	}

	filled := 0
	for filled < len(samples) {
		// There are no streamers in the queue, so we stream silence.
		if len(q.streamers) == 0 {
			break
		}

		// We stream from the first streamer in the queue.
		n, ok := q.streamers[0].Stream(samples[filled:])
		// If it's drained, we pop it from the queue, thus continuing with
		// the next streamer.
		if !ok {
			q.streamers = q.streamers[1:]
		}
		// We update the number of filled samples.
		filled += n
	}

	if filled > 0 {
		if b, err := json.Marshal(samples); err == nil {
			q.Hub.Broadcast <- b
		}
	}

	return len(outsamples), true
}

func (q *Queue) Err() error {
	return nil
}

func main2(h *hub.Hub) {
	sr := beep.SampleRate(44100)
	speaker.Init(sr, sr.N(time.Second/10))

	// A zero Queue is an empty Queue.
	var queue Queue

	queue.Hub = h
	speaker.Play(&queue)

	////url := "https://github.com/faiface/beep/blob/master/examples/tutorial/1-hello-beep/Lame_Drivers_-_01_-_Frozen_Egg.mp3?raw=true"
	// url := "https://sample-videos.com/audio/mp3/wave.mp3"
	//// Get the data
	//resp, err := http.Get(url)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//os.TempDir()
	//// Create the file
	//f, err := os.Create(os.TempDir() + "tmp.mp3")
	//if err != nil {
	//	log.Fatal(err)
	//}
	//defer f.Close()
	//
	//_, err = io.Copy(f, resp.Body)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//_ = resp.Body.Close()

	for {
		//var name string

		name := kodata + "/frozen-egg.mp3"

		//fmt.Print("Type an MP3 file name: ")
		//fmt.Scanln(&name)

		// Open the file on the disk.
		f, err := os.Open(name)
		if err != nil {
			fmt.Println(err)
			continue
		}

		// Decode it.
		streamer, format, err := mp3.Decode(f)
		if err != nil {
			fmt.Println(err)
			continue
		}

		// The speaker's sample rate is fixed at 44100. Therefore, we need to
		// resample the file in case it's in a different sample rate.
		resampled := beep.Resample(4, format.SampleRate, sr, streamer)

		// And finally, we add the song to the queue.
		speaker.Lock()
		queue.Add(resampled)
		speaker.Unlock()

		// wait until it is done.
		for {
			time.Sleep(1 * time.Second)
			if len(queue.streamers) == 0 {
				break
			}
		}
	}
}
