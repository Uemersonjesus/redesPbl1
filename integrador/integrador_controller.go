package main

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ─────────────────────────────────────────────
//
//	Tipos de sensor (campo "tipo" no datagrama)
//
// ─────────────────────────────────────────────
const (
	TipoTemperatura uint8 = 1
	TipoUmidade     uint8 = 2
)

// Limites que disparam comandos automáticos para o atuador
const (
	TemperaturaMax uint8 = 40 // °C  → desliga (ex.: AC ligado ao atingir máx)
	TemperaturaMin uint8 = 15 // °C  → liga
	UmidadeMax     uint8 = 80 // %   → desliga
	UmidadeMin     uint8 = 20 // %   → liga
)

// ─────────────────────────────────────────────
//  Comandos trocados por WebSocket (JSON)
// ─────────────────────────────────────────────

// Mensagem que o integrador envia a CLIENTES
type SensorBroadcast struct {
	Type        string `json:"type"` // "sensor_update"
	SensorID    uint16 `json:"sensor_id"`
	ActuatorID  uint16 `json:"actuator_id"` // 0 = sem atuador
	Tipo        uint8  `json:"tipo"`
	Information uint8  `json:"information"`
	Unit        string `json:"unit"`
}

// Mensagem que o integrador envia a ATUADORES
type ActuatorCommand struct {
	Type        string `json:"type"` // "command"
	SensorID    uint16 `json:"sensor_id"`
	Command     string `json:"command"`      // "on" | "off"
	TriggeredBy string `json:"triggered_by"` // "auto" | "client"
}

// Mensagem recebida de um CLIENTE (comando manual)
type ClientCommand struct {
	Type     string `json:"type"` // "control"
	SensorID uint16 `json:"sensor_id"`
	Command  string `json:"command"` // "on" | "off"
}

// ─────────────────────────────────────────────
//  Match: associa sensor ↔ atuador
// ─────────────────────────────────────────────

type Match struct {
	SensorID   uint16
	ActuatorID uint16
}

type MatchTable struct {
	mu         sync.RWMutex
	byActuator map[uint16]uint16 // actuatorID → sensorID
	bySensor   map[uint16]uint16 // sensorID   → actuatorID
}

func NewMatchTable() *MatchTable {
	return &MatchTable{
		byActuator: make(map[uint16]uint16),
		bySensor:   make(map[uint16]uint16),
	}
}

// TryMatch tenta parear sensor sem atuador com atuador sem sensor (ou vice-versa).
// Retorna o par formado, ou zero se não foi possível.
func (mt *MatchTable) TryMatch(sensors *mapOfSensors, actuators *MapOfActuators) *Match {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	actuators.mu.Lock()
	defer actuators.mu.Unlock()

	sensors.mu.Lock()
	defer sensors.mu.Unlock()

	// Atuador sem sensor?
	for aID := range actuators.actuatorsRegistred {
		if _, taken := mt.byActuator[aID]; !taken {
			// Sensor sem atuador?
			for sID := range sensors.sensors {
				if _, taken := mt.bySensor[sID]; !taken {
					mt.byActuator[aID] = sID
					mt.bySensor[sID] = aID
					fmt.Printf("🔗 Match: sensor %d ↔ atuador %d\n", sID, aID)
					return &Match{SensorID: sID, ActuatorID: aID}
				}
			}
		}
	}
	return nil
}

// ActuatorFor devolve o ID do atuador vinculado ao sensor (0 = nenhum).
func (mt *MatchTable) ActuatorFor(sensorID uint16) uint16 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.bySensor[sensorID]
}

// RemoveSensor desfaz o vínculo quando um sensor some.
func (mt *MatchTable) RemoveSensor(sensorID uint16) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	if aID, ok := mt.bySensor[sensorID]; ok {
		delete(mt.byActuator, aID)
		delete(mt.bySensor, sensorID)
		fmt.Printf("🔌 Vínculo removido: sensor %d ↔ atuador %d\n", sensorID, aID)
	}
}

// RemoveActuator desfaz o vínculo quando um atuador desconecta.
func (mt *MatchTable) RemoveActuator(actuatorID uint16) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	if sID, ok := mt.byActuator[actuatorID]; ok {
		delete(mt.bySensor, sID)
		delete(mt.byActuator, actuatorID)
		fmt.Printf("🔌 Vínculo removido: atuador %d ↔ sensor %d\n", actuatorID, sID)
	}
}

// ─────────────────────────────────────────────
//  Integrador — o cérebro central
// ─────────────────────────────────────────────

type Integrador struct {
	Sensors   *mapOfSensors
	Clients   *MapOfClients
	Actuators *MapOfActuators
	Matches   *MatchTable
}

func NewIntegrador(s *mapOfSensors, c *MapOfClients, a *MapOfActuators) *Integrador {
	return &Integrador{
		Sensors:   s,
		Clients:   c,
		Actuators: a,
		Matches:   NewMatchTable(),
	}
}

// ─── Chamado pelo UDP ao chegar dado de sensor ───────────────────────────────

func (ig *Integrador) OnSensorData(data diagramaUdpInformation) {
	isNew := !ig.Sensors.ExistsThisSensor(data.ID)

	// Salva/atualiza no mapa
	ig.Sensors.mu.Lock()
	ig.Sensors.sensors[data.ID] = data
	ig.Sensors.mu.Unlock()

	// Se sensor novo → tenta parear com atuador livre
	if isNew {
		fmt.Printf("Novo sensor detectado: ID=%d tipo=%d\n", data.ID, data.Tipo)
		ig.Matches.TryMatch(ig.Sensors, ig.Actuators)
	}

	// Broadcast para todos os clientes conectados
	ig.BroadcastSensorUpdate(data)

	// Lógica automática de controle
	ig.AutoControl(data)
}

// ─── Chamado pelo WebSocket ao conectar novo atuador ─────────────────────────

func (ig *Integrador) OnActuatorConnected(actuatorID uint16) {
	fmt.Printf("Novo atuador conectado: ID=%d\n", actuatorID)
	ig.Matches.TryMatch(ig.Sensors, ig.Actuators)
}

// ─── Chamado ao desconectar atuador ──────────────────────────────────────────

func (ig *Integrador) OnActuatorDisconnected(actuatorID uint16) {
	ig.Matches.RemoveActuator(actuatorID)
}

// ─── Broadcast de atualização de sensor para todos os clientes ───────────────

func (ig *Integrador) BroadcastSensorUpdate(data diagramaUdpInformation) {
	unit := unitFor(data.Tipo)
	actuatorID := ig.Matches.ActuatorFor(data.ID)

	msg := SensorBroadcast{
		Type:        "sensor_update",
		SensorID:    data.ID,
		ActuatorID:  actuatorID,
		Tipo:        data.Tipo,
		Information: data.Information,
		Unit:        unit,
	}

	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}

	ig.Clients.mu.Lock()
	defer ig.Clients.mu.Unlock()

	for _, c := range ig.Clients.clientsRegistred {
		select {
		case c.Send <- raw:
		default:
			fmt.Printf("Cliente %d lento, mensagem descartada\n", c.ID)
		}
	}
}

// ─── Controle automático por limites ─────────────────────────────────────────

func (ig *Integrador) AutoControl(data diagramaUdpInformation) {
	actuatorID := ig.Matches.ActuatorFor(data.ID)
	if actuatorID == 0 {
		return // sensor sem atuador — nada a fazer
	}

	var cmd string

	switch data.Tipo {
	case TipoTemperatura:
		if data.Information >= TemperaturaMax {
			cmd = "on" // liga o sistema de resfriamento
		} else if data.Information <= TemperaturaMin {
			cmd = "off"
		}
	case TipoUmidade:
		if data.Information >= UmidadeMax {
			cmd = "on" // liga o desumidificador
		} else if data.Information <= UmidadeMin {
			cmd = "off"
		}
	}

	if cmd != "" {
		ig.SendCommandToActuator(actuatorID, data.ID, cmd, "auto")
	}
}

// ─── Comando manual vindo de um cliente ──────────────────────────────────────

func (ig *Integrador) OnClientCommand(clientID uint16, raw []byte) {
	var cmd ClientCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		fmt.Printf("Cliente %d enviou JSON inválido: %s\n", clientID, raw)
		return
	}

	if cmd.Type != "control" {
		return
	}

	actuatorID := ig.Matches.ActuatorFor(cmd.SensorID)
	
	if actuatorID == 0 {
		fmt.Printf("Cliente %d pediu controle do sensor %d, mas sem atuador vinculado\n",
			clientID, cmd.SensorID)
		return
	}

	fmt.Printf("Cliente %d enviou comando '%s' para sensor %d → atuador %d\n",
		clientID, cmd.Command, cmd.SensorID, actuatorID)

	ig.SendCommandToActuator(actuatorID, cmd.SensorID, cmd.Command, "client")
}

// ─── Envia comando WebSocket para um atuador específico ──────────────────────

func (ig *Integrador) SendCommandToActuator(actuatorID, sensorID uint16, command, triggeredBy string) {
	a, ok := ig.Actuators.FindActuator(actuatorID)
	if !ok {
		fmt.Printf("⚠️  Atuador %d não encontrado no mapa\n", actuatorID)
		return
	}

	msg := ActuatorCommand{
		Type:        "command",
		SensorID:    sensorID,
		Command:     command,
		TriggeredBy: triggeredBy,
	}

	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}

	select {
	case a.Send <- raw:
		fmt.Printf("📤 Comando '%s' enviado → atuador %d (sensor %d) [%s]\n",
			command, actuatorID, sensorID, triggeredBy)
	default:
		fmt.Printf("⚠️  Canal do atuador %d cheio, comando descartado\n", actuatorID)
	}
}

// ─── Auxiliar ─────────────────────────────────────────────────────────────────

func unitFor(tipo uint8) string {
	switch tipo {
	case TipoTemperatura:
		return "°C"
	case TipoUmidade:
		return "%"
	default:
		return ""
	}
}
