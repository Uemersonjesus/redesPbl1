package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"
)

type Client struct {
	ID    uint16 `json:"id"`
	Conn  net.Conn
	Bufrw *bufio.ReadWriter
	Send  chan []byte
}

type MapOfClients struct {
	mu               sync.Mutex
	clientsRegistred map[uint16]Client
}

func NewMapOfClients() MapOfClients {
	return MapOfClients{
		clientsRegistred: make(map[uint16]Client),
	}
}

func (m *MapOfClients) ExistsThisClient(id uint16) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.clientsRegistred[id]
	return ok
}

func (m *MapOfClients) FindClient(id uint16) (Client, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.clientsRegistred[id]
	return c, ok
}

func findLargestClientId(m *MapOfClients) uint16 {
	var max uint16 = 0
	for id := range m.clientsRegistred {
		if id > max {
			max = id
		}
	}
	return max
}

func (m *MapOfClients) FindNewIdToClient(c Client, globalClient *MapOfClients) uint16 {

	globalClient.mu.Lock()
	defer globalClient.mu.Unlock()

	paternId := c.ID
	thisIdExists := false

	for id := range m.clientsRegistred {
		if paternId == id && paternId != 0 {
			thisIdExists = true
		}
	}

	largestId := findLargestClientId(m)

	if !thisIdExists {
		m.clientsRegistred[largestId+1] = c
		return largestId + 1
	}
	return paternId
}

func (m *MapOfClients) RemoveClient(id uint16) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clientsRegistred, id)
}

// writePump — envia mensagens do canal Send para o socket WebSocket.
func (c *Client) writePump() {
	defer c.Conn.Close()

	for message := range c.Send {
		length := len(message)
		// Frame WebSocket simples (sem máscara, texto)
		c.Bufrw.WriteByte(0x81)
		c.Bufrw.WriteByte(byte(length))
		c.Bufrw.Write(message)
		c.Bufrw.Flush()
	}
}

// readPump — lê frames WebSocket do cliente e encaminha comandos ao Integrador.
func (c *Client) readPump(ig *Integrador) {
	defer c.Conn.Close()

	for {
		header, err := c.Bufrw.Peek(2)
		if err != nil {
			break
		}

		opcode := header[0] & 0x0F
		if opcode == 8 { // Close frame
			break
		}

		payloadLen := int(header[1] & 0x7F)
		c.Bufrw.Discard(2)

		mask, _ := c.Bufrw.Peek(4)
		c.Bufrw.Discard(4)

		// Copia o payload para não ter problemas com o buffer circular do Peek
		rawPayload, _ := c.Bufrw.Peek(payloadLen)
		payload := make([]byte, payloadLen)
		copy(payload, rawPayload)
		c.Bufrw.Discard(payloadLen)

		// Unmask (XOR — obrigatório para frames vindos do cliente)
		for i := 0; i < payloadLen; i++ {
			payload[i] ^= mask[i%4]
		}

		fmt.Printf("📨 Cliente %d enviou: %s\n", c.ID, string(payload))

		// Delega ao Integrador para tratar comandos manuais
		ig.OnClientCommand(c.ID, payload)
	}
}
