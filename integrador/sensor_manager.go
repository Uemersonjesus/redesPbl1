package main

import "sync"

type diagramaUdpInformation struct {
	ID          uint16 `json:"id"`
	Tipo        uint8  `json:"tipo"`
	Information uint8  `json:"information"` //ex  35 graus , 60 graus ou  40 % umidade , 80 % de umidade
	Crc         uint8  `josn:"crc"`
	send        int
}

type mapOfSensors struct {
	mu      sync.Mutex
	sensors map[uint16]diagramaUdpInformation
}

func newDiagramUdpInformation() mapOfSensors {
	return mapOfSensors{
		sensors: make(map[uint16]diagramaUdpInformation),
	}
}

func (m *mapOfSensors) ExistsThisSensor(u uint16) bool {

	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.sensors[u]

	if !ok {
		return false
	}

	return true
}

//Parei de usar estas funções depois que mudei a definição do id do sensor diretamente nele.
func (m *mapOfSensors) findUDatagrama(u uint16) (diagramaUdpInformation, bool) {
	datagrama, ok := m.sensors[u]
	return datagrama, ok
}

//Função auxiliar para encontrar o maior.
func findLargestId(m *mapOfSensors) uint16 {
	var max uint16 = 0
	// O range em map extrai as chaves (ids)
	for id := range m.sensors {
		if id > max {
			max = id
		}
	}
	return max
}
