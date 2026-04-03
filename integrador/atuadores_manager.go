package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"
)

// Actuator representa a conexão física de um hardware atuador via WebSocket
type Actuator struct {
	ID    uint16 `json:"id"`
	Conn  net.Conn
	Bufrw *bufio.ReadWriter
	Send  chan []byte
}

type MapOfActuators struct {
	mu                 sync.Mutex
	actuatorsRegistred map[uint16]Actuator

	nextID uint16
}

func NewMapOfActuators() MapOfActuators {
	return MapOfActuators{
		actuatorsRegistred: make(map[uint16]Actuator),
		nextID:             0,
	}
}

func (m *MapOfActuators) ExistsThisActuator(id uint16) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.actuatorsRegistred[id]
	return ok
}

func (m *MapOfActuators) FindActuator(id uint16) (Actuator, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, ok := m.actuatorsRegistred[id]
	return a, ok
}

func findLargestActuatorId(m *MapOfActuators) uint16 {
	var max uint16 = 0
	for id := range m.actuatorsRegistred {
		if id > max {
			max = id
		}
	}
	return max
}

func (m *MapOfActuators) FindNewIdToActuator(a Actuator) uint16 {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Map lookup é O(1) — não precisa percorrer nada
	if a.ID != 0 {
		if _, exists := m.actuatorsRegistred[a.ID]; !exists {
			m.actuatorsRegistred[a.ID] = a
		}
		return a.ID
	}

	// ID 0 significa que o atuador não tem ID ainda — gera um novo
	// Também O(1): incrementa um contador em vez de buscar o maior
	m.nextID++
	m.actuatorsRegistred[m.nextID] = a
	return m.nextID
}

func (m *MapOfActuators) RemoveActuator(id uint16) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.actuatorsRegistred, id)
}

// writePump focado em enviar comandos do Integrador para o dispositivo físico
func (a *Actuator) writePump() {
	defer a.Conn.Close()

	for message := range a.Send {
		length := len(message)

		// Frame WebSocket: 0x81 (Texto)
		a.Bufrw.WriteByte(0x81)
		a.Bufrw.WriteByte(byte(length))
		a.Bufrw.Write(message)
		a.Bufrw.Flush()
	}
}

// readPump focado em receber confirmações de execução do atuador
func (a *Actuator) readPump() {
	defer a.Conn.Close()

	for {
		header, err := a.Bufrw.Peek(2)
		if err != nil {
			break
		}

		opcode := header[0] & 0x0F
		if opcode == 8 { // Close frame
			break
		}

		payloadLen := int(header[1] & 0x7F)
		a.Bufrw.Discard(2)

		mask, _ := a.Bufrw.Peek(4)
		a.Bufrw.Discard(4)

		payload, _ := a.Bufrw.Peek(payloadLen)
		a.Bufrw.Discard(payloadLen)

		// Unmask (XOR)
		for i := 0; i < payloadLen; i++ {
			payload[i] ^= mask[i%4]
		}

		fmt.Printf("Atuador %d confirmou ação: %s\n", a.ID, string(payload))
	}
}
