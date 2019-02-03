package socket

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alpacahq/slait/cache"
	. "github.com/alpacahq/slait/utils/log"
	"github.com/eapache/channels"

	"github.com/gorilla/websocket"
)

// WebSocket server

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{}

type connection struct {
	sync.Mutex
	ws     *websocket.Conn
	send   *channels.InfiniteChannel
	done   uint32
	closed uint32
}

func (c *connection) WriteMessage(messageType int, data []byte) error {
	c.Lock()
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	defer c.Unlock()
	return c.ws.WriteMessage(messageType, data)
}

func (c *connection) WriteJSON(v interface{}) error {
	c.Lock()
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	defer c.Unlock()
	return c.ws.WriteJSON(v)
}

func (c *connection) ReadMessage() (messageType int, p []byte, err error) {
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	return c.ws.ReadMessage()
}

func (c *connection) ReadJSON(v interface{}) error {
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	return c.ws.ReadJSON(v)
}

func (c *connection) GetAddress() string {
	if c.ws != nil {
		return c.ws.RemoteAddr().String()
	} else {
		return "Unknown address"
	}
}

func (c *connection) Send(p *cache.Publication) {
	if atomic.LoadUint32(&c.done) > 0 {
		if atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
			c.send.Close()
		}
	} else {
		if atomic.LoadUint32(&c.closed) < 1 {
			c.send.In() <- p
		}
	}
}

type SocketMessage struct {
	Action     string
	Topic      string
	Partitions []string
	From       time.Time
	Data       cache.Entries
}

type subscription struct {
	conn *connection
	m    *sync.Map
	from time.Time
	done chan struct{}
}

func (s *subscription) buildPublicKey(topic, partition string) (key string) {
	return fmt.Sprintf("%v::%v", topic, partition)
}

func (s *subscription) parsePublicKey(key string) (topic, partition string) {
	keys := strings.Split(key, "::")
	return keys[0], keys[1]
}

func (s *subscription) shouldReceive(topic, partition string) (should bool) {
	s.m.Range(func(key interface{}, value interface{}) bool {
		t := key.(string)
		if topic == t {
			partitions := value.([]string)
			if len(partitions) == 0 {
				should = true
				return false
			}
			for _, p := range partitions {
				if p == partition {
					should = true
					return false
				}
			}
		}
		return true
	})
	return should
}

func (s *subscription) shouldPublic(topic, partition string) (should bool) {
	ht, hp := cache.Has(topic, partition)
	should = true

	if !ht || !hp {
		// No topic or partition exist in main cache, which means no publisher for
		// that topic/partition

	} else {

		// For each element in publications(publisher), the key of m sync.Map object
		// presented like <topic>::<partition> (see func buildPublicKey). So each time a new
		// publisher added to slait we need to check if other publishers contain the
		// topic/partition in the it's m already. If yes(contain the topic/partition), false
		// will be return, otherwise, return true
		hub.publications.Range(func(key interface{}, value interface{}) bool {
			pub := key.(*subscription)
			if _, ok := pub.m.Load(s.buildPublicKey(topic, partition)); ok {
				should = false
				return false
			}

			should = true
			return true
		})
	}

	return should
}

func (s *subscription) cleanup(closeMsg string) {
	if atomic.CompareAndSwapUint32(&s.conn.done, 0, 1) {
		s.conn.WriteMessage(websocket.CloseMessage, []byte(closeMsg))
		defer Log(INFO, "Unsubscribed %v", s.conn.GetAddress())
		hub.unsubscribe(s)
		s.done <- struct{}{}
		if s.conn.ws != nil {
			err := s.conn.ws.Close()
			if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
				Log(WARNING, "Error occurred closing websocket connection - Error: %v", err.Error())
			}
		}
	}
}

func (s *subscription) consume() {
	defer s.cleanup("")
	s.conn.ws.SetPongHandler(func(string) error {
		s.conn.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		m := SocketMessage{}
		err := s.conn.ReadJSON(&m)
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway) {
				Log(WARNING, "Unexpected WS closure - Error: %v", err)
			}
			return
		}

		switch strings.ToLower(m.Action) {
		// case "unsubscribe":
		// 	return
		case "subscribe":
			// update the subscription
			val, loaded := s.m.LoadOrStore(m.Topic, m.Partitions)
			if loaded {
				partitions := val.([]string)
				s.m.Store(m.Topic, append(partitions, m.Partitions...))
			}
			s.from = m.From
			hub.subscribe(s, false)

		case "publish":
			occupied := false
			closemsg := ""

			// Check if other publisher publishing topic/partition already
			for _, p := range m.Partitions {
				if !s.shouldPublic(m.Topic, p) {
					occupied = true
					closemsg = fmt.Sprintf("Topic %v and Partition %v occupied by other publisher", m.Topic, p)
					break
				}

				// Build the inner sync.Map to log <topic>::<partition> pair for
				// other publisher to check. This line is tricky
				s.m.LoadOrStore(s.buildPublicKey(m.Topic, p), true)
			}

			if occupied {
				// Notify the client that topic/partition pair already occupied
				// by other publishers. We close the connection for this client
				// once we find overlap on topic/partition due to we don't want
				// to double check the publish stream later for performance issue.

				s.cleanup(closemsg)
				Log(INFO, "Rejected new publisher due to %v", closemsg)
			} else {
				// Now we can allow this publisher to public it's topic/partitions
				s.from = m.From

				hub.subscribe(s, true)

				Log(INFO, "New publisher connected for Topic: %v and Partitions: %v", m.Topic, strings.Join(m.Partitions, "/"))

			}

		case "pub":

			if len(m.Topic) > 0 && len(m.Partitions) > 0 && len(m.Data) > 0 {
				cache.Append(m.Topic, m.Partitions[0], m.Data)
			}

		}
	}
}

func (s *subscription) produce() {
	defer s.cleanup("")
	ticker := time.NewTicker(pingPeriod)
	for {
		select {
		case data := <-s.conn.send.Out():
			if err := s.conn.WriteJSON(data); err != nil {
				Log(ERROR, "Failed to write JSON to WS - Error: %v", err)
				return
			}
		case <-ticker.C:
			if err := s.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				if !strings.Contains(err.Error(), "websocket: close sent") {
					Log(ERROR, "Failed to write ping message to WS - Error: %v", err)
				}
				return
			}
		case <-s.done:
			return
		}
	}
}

type Hub struct {
	sync.RWMutex
	subscriptions sync.Map
	publications  sync.Map
}

func (h *Hub) unsubscribe(s *subscription) {
	if _, ok := h.subscriptions.Load(s); ok {

		h.subscriptions.Delete(s)

	} else if _, ok := h.publications.Load(s); ok {

		h.publications.Delete(s)

	}
}

func (h *Hub) subscribe(s *subscription, isPublisher bool) {
	if !isPublisher {
		h.subscriptions.Store(s, true)
		h.dump(s)

	} else {
		h.publications.Store(s, true)

		s.m.Range(func(key interface{}, value interface{}) bool {
			t, p := s.parsePublicKey(key.(string))
			ht, hp := cache.Has(t, p)

			if !ht {
				cache.Add(t)
			}

			if !hp {
				cache.Update(t, p, cache.AddPartition)
			}

			return true
		})

		s.conn.WriteJSON(SocketMessage{
			Action: "ready",
		})
	}
}

func (h *Hub) dump(s *subscription) {
	Log(INFO, "Dumping data to %v", s.conn.GetAddress())
	s.m.Range(func(key interface{}, value interface{}) bool {
		topic := key.(string)
		partitions := value.([]string)
		if len(partitions) == 0 {
			data := cache.GetAll(topic, &s.from, nil, 0)
			for partition, entries := range data {
				s.conn.Send(
					&cache.Publication{
						Topic:     topic,
						Partition: partition,
						Entries:   entries,
					})
			}
		} else {
			for _, partition := range partitions {
				entries := cache.Get(topic, partition, &s.from, nil, 0)
				s.conn.Send(
					&cache.Publication{
						Topic:     topic,
						Partition: partition,
						Entries:   entries,
					})
			}
		}
		return true
	})
	Log(INFO, "Finished data dump to %v", s.conn.GetAddress())
}

func (h *Hub) run() {
	for {
		select {
		case p := <-cache.Pull():
			pub := p.(*cache.Publication)
			h.subscriptions.Range(func(key interface{}, value interface{}) bool {
				sub := key.(*subscription)
				if sub.shouldReceive(pub.Topic, pub.Partition) {
					sub.conn.Send(pub)
				}
				return true
			})
		case a := <-cache.PullAdditions():
			h.subscriptions.Range(func(key interface{}, value interface{}) bool {
				sub := key.(*subscription)
				sub.conn.WriteJSON(SocketMessage{
					Topic:      a.Topic,
					Partitions: []string{a.Partition},
					Action:     "add",
				})
				return true
			})
		case r := <-cache.PullRemovals():
			h.subscriptions.Range(func(key interface{}, value interface{}) bool {
				sub := key.(*subscription)
				err := sub.conn.WriteJSON(SocketMessage{
					Topic:      r.Topic,
					Partitions: []string{r.Partition},
					Action:     "remove",
				})
				if err != nil {
					Log(ERROR, "Writing socket message failed! %v", err)
					return false
				}
				sub.m.Range(func(key interface{}, value interface{}) bool {
					t := key.(string)
					partitions := value.([]string)
					if len(partitions) == 0 {
						return false
					} else {
						for i, p := range partitions {
							if p == r.Partition {
								partitions = append(partitions[:i], partitions[i+1:]...)
								sub.m.Store(t, partitions)
								return false
							}
						}
					}
					return true
				})
				return true
			})
		}
	}
}

var hub = Hub{
	subscriptions: sync.Map{},
	publications:  sync.Map{},
}

type SocketHandler struct{}

func (sh *SocketHandler) Serve(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		Log(ERROR, "Failed to upgrade websocket - Error: %v", err)
		return
	}
	c := &connection{
		send: channels.NewInfiniteChannel(),
		ws:   ws,
	}

	s := subscription{
		conn: c,
		m:    &sync.Map{},
		done: make(chan struct{}),
	}

	if s.conn.ws != nil {
		Log(INFO, "New client: %v", s.conn.GetAddress())
	}
	go s.consume()
	go s.produce()
}

// call only once
func GetHandler() *SocketHandler {
	go hub.run()
	sh := &SocketHandler{}
	return sh
}
