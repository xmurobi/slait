package socket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/eapache/channels"

	"github.com/alpacahq/slait/cache"

	"github.com/gorilla/websocket"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type SocketTestSuite struct{}

var _ = Suite(&SocketTestSuite{})

var t1 = "bars"
var t2 = "quotes"
var p1 = "NVDA_composite"
var p2 = "AMD_bats"

func (s *SocketTestSuite) TestSocket(c *C) {
	cache.Build(c.MkDir())
	setup()
	handler := GetHandler()
	srv := httptest.NewServer(http.HandlerFunc(handler.Serve))
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		c.Fatalf("Cannot connect to websocket: %v", err)
	}
	done := make(chan struct{})
	pubsReceived := 0
	sockMsgsReceived := 0

	go func() {
		for {
			sm := SocketMessage{}
			pm := cache.Publication{}
			_, msg, err := conn.ReadMessage()
			if err != nil {
				c.Fatalf("Failed to read from websocket: %v", err)
			}
			err = json.Unmarshal(msg, &pm)
			if pm.Entries.Len() == 0 {
				err = json.Unmarshal(msg, &sm)
				if err != nil {
					c.Fatalf("Received an invalid message that couldn't be unmarshalled: %v", err)
				}
				// fmt.Println("Socket message:", sm)
				sockMsgsReceived++
			} else {
				// fmt.Println("Publication:", pm)
				pubsReceived++
			}
			if pubsReceived >= 6 && sockMsgsReceived >= 4 {
				done <- struct{}{}
			}
		}
	}()
	err = conn.WriteJSON(SocketMessage{
		Action: "subscribe",
		Topic:  t1, /*"bars"*/
	})
	if err != nil {
		c.Fatalf("Cannot write JSON message to websocket: %v", err)
	}
	push()

	// need to let the data flow out before removing
	err = cache.Update(t1, p1, cache.RemovePartition)
	if err != nil {
		c.Fatal(err.Error())
	}
	push()
	err = cache.Update(t1, p1, cache.RemovePartition)
	if err != nil {
		c.Fatal(err.Error())
	}
	err = cache.Update(t1, p1, cache.AddPartition)
	if err != nil {
		c.Fatal(err.Error())
	}
	push()

	t := time.NewTicker(3 * time.Second)
	select {
	case <-done:
		err = conn.WriteMessage(websocket.CloseMessage, []byte{})
		c.Assert(err, IsNil)
		return
	case <-t.C:
		c.Fatal(
			fmt.Sprintf(
				"Test didn't receive all expected data! Socket messages: %v - Pub messages: %v",
				sockMsgsReceived,
				pubsReceived,
			))
	}
}

func (s *SocketTestSuite) TestPubSub(c *C) {
	done := make(chan struct{})

	// server
	cache.Build(c.MkDir())
	handler := GetHandler()
	srv := httptest.NewServer(http.HandlerFunc(handler.Serve))
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"

	// publisher
	pubconn := connect(u)
	if pubconn == nil {
		c.Fatalf("Pubisher cannot connect to websocket")
	}

	// subscriber
	subconn := connect(u)
	if subconn == nil {
		c.Fatalf("Subscriber cannot connect to websocket")
	}

	sampleCounter := 0

	go func() {
		for {
			sm := SocketMessage{}
			pm := cache.Publication{}
			_, msg, err := subconn.ReadMessage()
			if err != nil {
				c.Fatalf("Failed to read from websocket: %v", err)
			}
			err = json.Unmarshal(msg, &pm)
			if pm.Entries.Len() == 0 {
				err = json.Unmarshal(msg, &sm)
				if err != nil {
					c.Fatalf("Received an invalid message that couldn't be unmarshalled: %v", err)
				}
				fmt.Println("Socket message:", sm)
			} else {
				fmt.Println("Subcriber got Publication:", pm)
				subconn.done = 1
			}

			if subconn.done > 0 && pubconn.done > 0 {
				done <- struct{}{}
			}
		}
	}()
	err := subconn.WriteJSON(SocketMessage{
		Action:     "subscribe",
		Topic:      "ABC",
		Partitions: []string{"A", "B", "C"},
	})
	if err != nil {
		c.Fatalf("Cannot write JSON message to websocket: %v", err)
	} else {
		fmt.Println("Subscriber connected to server")
	}

	// publisher

	go func() {
		for {
			sm := SocketMessage{}
			err := pubconn.ReadJSON(&sm)
			if err != nil {
				c.Fatalf("Failed to read from websocket: %v", err)
			}

			if sm.Action == "ready" {
				sm := SocketMessage{
					Action:     "pub",
					Topic:      "ABC",
					Partitions: []string{"A"},
					Data:       cache.GenData(),
				}
				err := pubconn.WriteJSON(sm)
				if err != nil {
					c.Fatalf("Failed to publish to websocket: %v", err)
				} else {
					fmt.Println("Pubisher sent data to server")
				}

				pubconn.done = 1
			}
		}
	}()

	err = pubconn.WriteJSON(SocketMessage{
		Action:     "publish",
		Topic:      "ABC",
		Partitions: []string{"A"},
	})

	if err != nil {
		c.Fatalf("Cannot write JSON message to websocket: %v", err)
	} else {
		fmt.Println("Pubisher connected to server")
	}

	// finally
	t := time.NewTicker(30 * time.Second)
	select {
	case <-done:
		err = subconn.WriteMessage(websocket.CloseMessage, []byte{})
		c.Assert(err, IsNil)
		fmt.Println("Subscriber disconnected")
		err = pubconn.WriteMessage(websocket.CloseMessage, []byte{})
		c.Assert(err, IsNil)
		fmt.Println("Publisher disconnected")
		return
	case <-t.C:
		c.Fatal(
			fmt.Sprintf(
				"Test didn't receive all expected data! messages: %v",
				sampleCounter,
			))
	}
}

func connect(u *url.URL) (c *connection) {
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)

	if err != nil {
		return nil
	}

	c = &connection{
		send: channels.NewInfiniteChannel(),
		ws:   conn,
	}

	return c
}

func setup() {
	cache.Add(t1)
	cache.Add(t2)
	cache.Update(t1, p1, cache.AddPartition)
	cache.Update(t1, p2, cache.AddPartition)
	cache.Update(t2, p1, cache.AddPartition)
	cache.Update(t2, p2, cache.AddPartition)
}

func push() {
	cache.Append(t1, p1, cache.GenData())
	cache.Append(t1, p2, cache.GenData())
	cache.Append(t2, p1, cache.GenData())
	cache.Append(t2, p2, cache.GenData())
}

