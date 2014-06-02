package namedwebsockets

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func makeService(host string, port int) *NamedWebSocket_Service {
	service := &NamedWebSocket_Service{
		Host: host,
		Port: port,
	}
	return service
}

type WSClient struct {
	*websocket.Conn
}

func makeClient(t *testing.T, host, path string, peerId int) *WSClient {
	if peerId == 0 {
		// Generate unique id for connection
		rand.Seed(time.Now().UTC().UnixNano())
		peerId = rand.Int()
	}
	url := fmt.Sprintf("ws://%s%s/%d", host, path, peerId)
	ws, _, err := websocket.DefaultDialer.Dial(url, map[string][]string{
		"Origin": []string{"localhost"},
	})
	if err != nil {
		t.Fatalf("Websocket client connection failed: %s", err)
	}
	wsClient := &WSClient{ws}
	return wsClient
}

func (ws *WSClient) send(t *testing.T, message string) {
	if err := ws.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetWriteDeadline: %v", err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
}

// Make sure a broadcast message is sent to all peers
func (ws *WSClient) recv(t *testing.T, message string) {
	if err := ws.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	_, p, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(p) != message {
		t.Fatalf("message=%s, want %s", p, message)
	}
}

func (ws *WSClient) sendDirect(t *testing.T, action string, source, target int, payload string) {
	m := ControlWireMessage{
		Action:  action,
		Source:  source,
		Target:  target,
		Payload: payload,
	}
	messagePayload, err := json.Marshal(m)
	if err != nil {
		return
	}

	ws.send(t, string(messagePayload))
}

// Make sure a broadcast message is sent to all peers
func (ws *WSClient) recvDirect(t *testing.T, action string, source, target int, payload string) {
	if err := ws.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	_, p, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	var message ControlWireMessage
	if err := json.Unmarshal(p, &message); err != nil {
		t.Fatalf("ControlWireMessage JSON Unmarshaling: %s", err)
	}

	if message.Action != action {
		t.Fatalf("action=%s, want %s", message.Action, action)
	}
	if message.Source != source {
		t.Fatalf("source=%d, want %d", message.Source, source)
	}
	if message.Target != target {
		t.Fatalf("target=%d, want %d", message.Target, target)
	}
	if string(message.Payload) != payload {
		t.Fatalf("message=%s, want %s", message.Payload, payload)
	}
}

func TestLocalConnection_Broadcast(t *testing.T) {
	// Make named websocket test server
	s1 := makeService("localhost", 9021)
	go s1.StartHTTPServer()

	// Define connection identifiers
	const (
		c1_Id = 11111
		c2_Id = 22222
		c3_Id = 33333
		c4_Id = 44444
	)

	// Make named websocket test clients
	c1 := makeClient(t, "localhost:9021", "/local/testservice_A", c1_Id)
	c2 := makeClient(t, "localhost:9021", "/local/testservice_A", c2_Id)
	c3 := makeClient(t, "localhost:9021", "/local/testservice_A", c3_Id)

	// Make named websocket test client controllers
	c1_control := makeClient(t, "localhost:9021", "/control/local/testservice_A", c1_Id)
	c2_control := makeClient(t, "localhost:9021", "/control/local/testservice_A", c2_Id)
	c3_control := makeClient(t, "localhost:9021", "/control/local/testservice_A", c3_Id)

	// Test connect control messages
	c1_control.recvDirect(t, "connect", c1_Id, c2_Id, "")
	c1_control.recvDirect(t, "connect", c1_Id, c3_Id, "")
	c2_control.recvDirect(t, "connect", c2_Id, c1_Id, "")
	c2_control.recvDirect(t, "connect", c2_Id, c3_Id, "")
	c3_control.recvDirect(t, "connect", c3_Id, c1_Id, "")
	c3_control.recvDirect(t, "connect", c3_Id, c2_Id, "")

	// Test broadcast ( c1 -> [c2, c3] )
	c1.send(t, "A_HelloFrom1")
	c2.recv(t, "A_HelloFrom1")
	c3.recv(t, "A_HelloFrom1")

	// Test broadcast ( c2 -> [c1, c3] )
	c2.send(t, "A_HelloFrom2")
	c1.recv(t, "A_HelloFrom2")
	c3.recv(t, "A_HelloFrom2")

	// Test broadcast ( c3 -> [c1, c2] )
	c3.send(t, "A_HelloFrom3")
	c1.recv(t, "A_HelloFrom3")
	c2.recv(t, "A_HelloFrom3")

	// Close connection 1 and test disconnect control messages against not-yet-closed connections
	c1.Close()
	c2_control.recvDirect(t, "disconnect", c2_Id, c1_Id, "")
	c3_control.recvDirect(t, "disconnect", c3_Id, c1_Id, "")

	// Close connection 2 and test disconnect control messages against not-yet-closed connections
	c2.Close()
	c3_control.recvDirect(t, "disconnect", c3_Id, c2_Id, "")

	// Close connection 3
	c3.Close()
}

func TestNetworkConnection_Broadcast(t *testing.T) {
	// Make named websocket test servers
	s1 := makeService("localhost", 9022)
	go s1.StartHTTPServer()

	s2 := makeService("localhost", 9023)
	go s2.StartHTTPServer()

	s3 := makeService("localhost", 9024)
	go s3.StartHTTPServer()

	// Define connection identifiers
	const (
		c1_Id = 11111
		c2_Id = 22222
		c3_Id = 33333
		c4_Id = 44444
	)

	// Make named websocket test clients
	c1 := makeClient(t, "localhost:9022", "/broadcast/testservice_B", c1_Id)
	c2 := makeClient(t, "localhost:9022", "/broadcast/testservice_B", c2_Id)
	c3 := makeClient(t, "localhost:9023", "/broadcast/testservice_B", c3_Id)
	c4 := makeClient(t, "localhost:9024", "/broadcast/testservice_B", c4_Id)

	// Make named websocket test client controllers
	c1_control := makeClient(t, "localhost:9022", "/control/broadcast/testservice_B", c1_Id)
	c2_control := makeClient(t, "localhost:9022", "/control/broadcast/testservice_B", c2_Id)
	c3_control := makeClient(t, "localhost:9023", "/control/broadcast/testservice_B", c3_Id)
	c4_control := makeClient(t, "localhost:9024", "/control/broadcast/testservice_B", c4_Id)

	// Test connect control messages
	c1_control.recvDirect(t, "connect", c1_Id, c2_Id, "")
	c1_control.recvDirect(t, "connect", c1_Id, c3_Id, "")
	c1_control.recvDirect(t, "connect", c1_Id, c4_Id, "")
	c2_control.recvDirect(t, "connect", c2_Id, c1_Id, "")
	c2_control.recvDirect(t, "connect", c2_Id, c3_Id, "")
	c2_control.recvDirect(t, "connect", c2_Id, c4_Id, "")
	c3_control.recvDirect(t, "connect", c3_Id, c1_Id, "")
	c3_control.recvDirect(t, "connect", c3_Id, c2_Id, "")
	c3_control.recvDirect(t, "connect", c3_Id, c4_Id, "")
	c4_control.recvDirect(t, "connect", c4_Id, c1_Id, "")
	c4_control.recvDirect(t, "connect", c4_Id, c2_Id, "")
	c4_control.recvDirect(t, "connect", c4_Id, c3_Id, "")

	// Test broadcast -> receive ( c1 -> [c2, c3, c4] )
	c1.send(t, "B_HelloFrom1")
	c2.recv(t, "B_HelloFrom1")
	c3.recv(t, "B_HelloFrom1")
	c4.recv(t, "B_HelloFrom1")

	// Test broadcast -> receive ( c2 -> [c1, c3, c4] )
	c2.send(t, "B_HelloFrom2")
	c1.recv(t, "B_HelloFrom2")
	c3.recv(t, "B_HelloFrom2")
	c4.recv(t, "B_HelloFrom2")

	// Test broadcast -> receive ( c3 -> [c1, c2, c4] )
	c3.send(t, "B_HelloFrom3")
	c1.recv(t, "B_HelloFrom3")
	c2.recv(t, "B_HelloFrom3")
	c4.recv(t, "B_HelloFrom3")

	// Test broadcast -> receive ( c4 -> [c1, c2, c3] )
	c4.send(t, "B_HelloFrom4")
	c1.recv(t, "B_HelloFrom4")
	c2.recv(t, "B_HelloFrom4")
	c3.recv(t, "B_HelloFrom4")

	// Close connection 1 and test disconnect control messages against not-yet-closed connections
	c1.Close()
	c2_control.recvDirect(t, "disconnect", c2_Id, c1_Id, "")
	c3_control.recvDirect(t, "disconnect", c3_Id, c1_Id, "")
	c4_control.recvDirect(t, "disconnect", c4_Id, c1_Id, "")

	// Close connection 2 and test disconnect control messages against not-yet-closed connections
	c2.Close()
	c3_control.recvDirect(t, "disconnect", c3_Id, c2_Id, "")
	c4_control.recvDirect(t, "disconnect", c4_Id, c2_Id, "")

	// Close connection 3 and test disconnect control messages against not-yet-closed connections
	c3.Close()
	c4_control.recvDirect(t, "disconnect", c4_Id, c3_Id, "")

	// Close connection 4
	c4.Close()
}

func TestNetworkConnection_DirectMessaging(t *testing.T) {
	// Make named websocket test servers
	s1 := makeService("localhost", 9025)
	go s1.StartHTTPServer()

	s2 := makeService("localhost", 9026)
	go s2.StartHTTPServer()

	s3 := makeService("localhost", 9027)
	go s3.StartHTTPServer()

	// Define connection identifiers
	const (
		c1_Id = 11111
		c2_Id = 22222
		c3_Id = 33333
		c4_Id = 44444
	)

	// Make named websocket test clients
	c1 := makeClient(t, "localhost:9025", "/broadcast/testservice_C", c1_Id)
	c2 := makeClient(t, "localhost:9026", "/broadcast/testservice_C", c2_Id)
	c3 := makeClient(t, "localhost:9026", "/broadcast/testservice_C", c3_Id)
	c4 := makeClient(t, "localhost:9027", "/broadcast/testservice_C", c4_Id)

	// Make named websocket test client controllers
	c1_control := makeClient(t, "localhost:9025", "/control/broadcast/testservice_C", c1_Id)
	c2_control := makeClient(t, "localhost:9026", "/control/broadcast/testservice_C", c2_Id)
	c3_control := makeClient(t, "localhost:9026", "/control/broadcast/testservice_C", c3_Id)
	c4_control := makeClient(t, "localhost:9027", "/control/broadcast/testservice_C", c4_Id)

	// Test connect control messages
	c1_control.recvDirect(t, "connect", c1_Id, c2_Id, "")
	c1_control.recvDirect(t, "connect", c1_Id, c3_Id, "")
	c1_control.recvDirect(t, "connect", c1_Id, c4_Id, "")
	c2_control.recvDirect(t, "connect", c2_Id, c1_Id, "")
	c2_control.recvDirect(t, "connect", c2_Id, c3_Id, "")
	c2_control.recvDirect(t, "connect", c2_Id, c4_Id, "")
	c3_control.recvDirect(t, "connect", c3_Id, c1_Id, "")
	c3_control.recvDirect(t, "connect", c3_Id, c2_Id, "")
	c3_control.recvDirect(t, "connect", c3_Id, c4_Id, "")
	c4_control.recvDirect(t, "connect", c4_Id, c1_Id, "")
	c4_control.recvDirect(t, "connect", c4_Id, c2_Id, "")
	c4_control.recvDirect(t, "connect", c4_Id, c3_Id, "")

	// Test direct message ( c1 -> c2 )
	c1_control.sendDirect(t, "message", c1_Id, c2_Id, "C_HelloFrom1To2")
	c2_control.recvDirect(t, "message", c1_Id, c2_Id, "C_HelloFrom1To2")

	// Test direct message ( c1 -> c3 )
	c1_control.sendDirect(t, "message", c1_Id, c3_Id, "C_HelloFrom1To3")
	c3_control.recvDirect(t, "message", c1_Id, c3_Id, "C_HelloFrom1To3")

	// Test direct message ( c1 -> c4 )
	c1_control.sendDirect(t, "message", c1_Id, c4_Id, "C_HelloFrom1To4")
	c4_control.recvDirect(t, "message", c1_Id, c4_Id, "C_HelloFrom1To4")

	// Test direct message ( c2 -> c1 )
	c2_control.sendDirect(t, "message", c2_Id, c1_Id, "C_HelloFrom2To1")
	c1_control.recvDirect(t, "message", c2_Id, c1_Id, "C_HelloFrom2To1")

	// Test direct message ( c2 -> c3 )
	c2_control.sendDirect(t, "message", c2_Id, c3_Id, "C_HelloFrom2To3")
	c3_control.recvDirect(t, "message", c2_Id, c3_Id, "C_HelloFrom2To3")

	// Test direct message ( c2 -> c4 )
	c2_control.sendDirect(t, "message", c2_Id, c4_Id, "C_HelloFrom2To4")
	c4_control.recvDirect(t, "message", c2_Id, c4_Id, "C_HelloFrom2To4")

	// Test direct message ( c3 -> c1 )
	c3_control.sendDirect(t, "message", c3_Id, c1_Id, "C_HelloFrom3To1")
	c1_control.recvDirect(t, "message", c3_Id, c1_Id, "C_HelloFrom3To1")

	// Test direct message ( c3 -> c2 )
	c3_control.sendDirect(t, "message", c3_Id, c2_Id, "C_HelloFrom3To2")
	c2_control.recvDirect(t, "message", c3_Id, c2_Id, "C_HelloFrom3To2")

	// Test direct message ( c3 -> c4 )
	c3_control.sendDirect(t, "message", c3_Id, c4_Id, "C_HelloFrom3To4")
	c4_control.recvDirect(t, "message", c3_Id, c4_Id, "C_HelloFrom3To4")

	// Test direct message ( c4 -> c1 )
	c4_control.sendDirect(t, "message", c4_Id, c1_Id, "C_HelloFrom4To1")
	c1_control.recvDirect(t, "message", c4_Id, c1_Id, "C_HelloFrom4To1")

	// Test direct message ( c4 -> c2 )
	c4_control.sendDirect(t, "message", c4_Id, c2_Id, "C_HelloFrom4To2")
	c2_control.recvDirect(t, "message", c4_Id, c2_Id, "C_HelloFrom4To2")

	// Test direct message ( c4 -> c3 )
	c4_control.sendDirect(t, "message", c4_Id, c3_Id, "C_HelloFrom4To3")
	c3_control.recvDirect(t, "message", c4_Id, c3_Id, "C_HelloFrom4To3")

	// Close connection 1 and test disconnect control messages against not-yet-closed connections
	c1.Close()
	c2_control.recvDirect(t, "disconnect", c2_Id, c1_Id, "")
	c3_control.recvDirect(t, "disconnect", c3_Id, c1_Id, "")
	c4_control.recvDirect(t, "disconnect", c4_Id, c1_Id, "")

	// Close connection 2 and test disconnect control messages against not-yet-closed connections
	c2.Close()
	c3_control.recvDirect(t, "disconnect", c3_Id, c2_Id, "")
	c4_control.recvDirect(t, "disconnect", c4_Id, c2_Id, "")

	// Close connection 3 and test disconnect control messages against not-yet-closed connections
	c3.Close()
	c4_control.recvDirect(t, "disconnect", c4_Id, c3_Id, "")

	// Close connection 4
	c4.Close()
}
