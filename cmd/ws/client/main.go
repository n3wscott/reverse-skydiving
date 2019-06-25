// Copyright 2015 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"
	"log"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", "localhost:8080", "http service address")

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: *addr, Path: "/ws"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	sr := beep.SampleRate(44100)
	speaker.Init(sr, sr.N(time.Second/10))

	format := beep.Format{
		SampleRate:  sr,
		NumChannels: 2,
		Precision:   4,
	}

	ring := AudioRing{
		format:   format,
		duration: time.Second * 12, // 12 second of shock protection.
	}
	ring.Init()

	speaker.Play(&ring)

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			//log.Printf("[%d] recv: %d\n", t, len(message))

			dec := json.NewDecoder(bytes.NewReader(message))

			for dec.More() {
				samples := make([][2]float64, 0, 512)
				if err := dec.Decode(&samples); err != nil {
					log.Printf("%s\nfailed to unmarshal: %v", string(message), err)
				} else {
					ring.Push(samples)
				}
			}
			//
			//samples := make([][2]float64, 0, 512)
			//if err := json.Unmarshal(message, &samples); err != nil {
			//	log.Printf("%s\nfailed to unmarshal: %v", string(message), err)
			//} else {
			//	ring.Push(samples)
			//}
		}
	}()

	//ticker := time.NewTicker(5 * time.Second)
	//defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		//case t := <-ticker.C:
		//	err := c.WriteMessage(websocket.TextMessage, []byte(t.String()))
		//	if err != nil {
		//		log.Println("write:", err)
		//		return
		//	}
		case <-interrupt:
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}

type AudioRing struct {
	head  uint64
	tail  uint64
	store uint64

	max      uint64
	format   beep.Format
	duration time.Duration

	buffer [][2]float64

	mux sync.Mutex
}

func (a *AudioRing) Init() {
	a.head = 0
	a.tail = 0
	a.store = 0

	a.max = uint64(a.duration.Seconds()) * uint64(a.format.SampleRate)

	log.Printf("making ring buffer %d long\n", a.max)

	a.buffer = make([][2]float64, a.max)

}

func (a *AudioRing) Push(samples [][2]float64) {
	a.mux.Lock()
	defer a.mux.Unlock()

	for _, v := range samples {
		a.buffer[a.head] = v
		a.head = (a.head + 1) % a.max
		if a.head == a.tail {
			a.tail = (a.tail + 1) % a.max
			//log.Println("head==tail")
		} else {
			a.store++
		}
	}
	//log.Printf("store[%d]\n", a.store)
}

func (a *AudioRing) Pop(samples [][2]float64) int {
	a.mux.Lock()
	defer a.mux.Unlock()

	if a.tail == a.head {
		//log.Println("zero tail==head")
		return 0
	}
	for i := 0; i < len(samples); i++ {
		samples[i] = a.buffer[a.tail]
		a.buffer[a.tail][0] = 0
		a.buffer[a.tail][1] = 0
		a.tail = (a.tail + 1) % a.max
		a.store--
		if a.tail == a.head {
			//log.Printf("tail==head [%d]\n", i)
			return i
		}
	}
	return len(samples)
}

func (a *AudioRing) Stream(samples [][2]float64) (n int, ok bool) {
	// We use the filled variable to track how many samples we've
	// successfully filled already. We loop until all samples are filled.
	filled := 0

	filled += a.Pop(samples)

	if filled == 0 {
		//log.Println("zeroing")
		// zero pad the buffer, this is going to be bad...
		for i := range samples[filled:] {
			samples[i][0] = 0
			samples[i][1] = 0
			filled++
		}
	}

	return len(samples), true
}

func (a *AudioRing) Err() error {
	return nil
}
