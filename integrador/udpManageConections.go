package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
)

func simpleChecksum(id uint16, tipo, info uint8) uint8 {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, id)
	sum := uint16(buf[0]) + uint16(buf[1]) + uint16(tipo) + uint16(info)
	return uint8(sum & 0xFF)
}

func managerUdpConnections(c *net.UDPConn, m *mapOfSensors, ig *Integrador) {
	buffer := make([]byte, 1024)

	for {
		n, addr, err := c.ReadFromUDP(buffer)
		if err != nil {
			fmt.Printf("Erro na leitura UDP: %v\n", err)
			continue
		}

		var sensorData diagramaUdpInformation
		if err = json.Unmarshal(buffer[:n], &sensorData); err != nil {
			fmt.Printf("Pacote UDP inválido de %s: %v\n", addr, err)
			continue
		}

		esperado := simpleChecksum(sensorData.ID, sensorData.Tipo, sensorData.Information)
		if sensorData.Crc != esperado {
			fmt.Printf("Check sum inválido de %s — sensor=%d recebido=0x%X esperado=0x%X — pacote descartado\n",
				addr, sensorData.ID, sensorData.Crc, esperado)
			continue
		}

		fmt.Printf("UDP recebido — sensor ID=%d tipo=%d valor=%d (de %s)\n",
			sensorData.ID, sensorData.Tipo, sensorData.Information, addr)

		// Delega toda a lógica ao Integrador
		ig.OnSensorData(sensorData)
	}
}

// RemoveSensor remove o sensor do mapa (assinatura original mantida).
func (m *mapOfSensors) RemoveSensor(id uint16) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sensors, id)
}

// RemoveSensorWithMatch remove o sensor e desfaz o vínculo no matchmaker.
func (m *mapOfSensors) RemoveSensorWithMatch(id uint16, ig *Integrador) {
	m.RemoveSensor(id)
	ig.Matches.RemoveSensor(id)
}
