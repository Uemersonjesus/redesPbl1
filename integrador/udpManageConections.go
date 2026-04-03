package main

import (
	"encoding/json"
	"fmt"
	"net"
)

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
