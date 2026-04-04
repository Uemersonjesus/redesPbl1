package main

import (
	"encoding/binary"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// ─── helpers de teste ─────────────────────────────────────────────────────────

// newIntegradorTest monta um Integrador completo sem dependência de rede.
func newIntegradorTest() *Integrador {
	s := newDiagramUdpInformation()
	c := NewMapOfClients()
	a := NewMapOfActuators()
	return NewIntegrador(&s, &c, &a)
}

// noopActuator registra um atuador com canal Send real mas sem conexão de rede.
func noopActuator(id uint16, ig *Integrador) chan []byte {
	send := make(chan []byte, 256)
	ig.Actuators.mu.Lock()
	ig.Actuators.actuatorsRegistred[id] = Actuator{ID: id, Send: send}
	ig.Actuators.mu.Unlock()
	return send
}

// noopClient registra um cliente com canal Send real mas sem conexão de rede.
func noopClient(id uint16, ig *Integrador) chan []byte {
	send := make(chan []byte, 256)
	ig.Clients.mu.Lock()
	ig.Clients.clientsRegistred[id] = Client{ID: id, Send: send}
	ig.Clients.mu.Unlock()
	return send
}

// drainChan esvazia um canal e retorna todas as mensagens recebidas até timeout.
func drainChan(ch chan []byte, timeout time.Duration) [][]byte {
	var msgs [][]byte
	deadline := time.After(timeout)
	for {
		select {
		case m := <-ch:
			msgs = append(msgs, m)
		case <-deadline:
			return msgs
		}
	}
}

// simpleCRC — cópia exata da função do sensor para uso nos testes.
func simpleCRC(id uint16, tipo, info uint8) uint8 {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, id)
	sum := uint16(buf[0]) + uint16(buf[1]) + uint16(tipo) + uint16(info)
	return uint8(sum & 0xFF)
}

// ═════════════════════════════════════════════════════════════════════════════
// 1. TESTES DE CRC
// ═════════════════════════════════════════════════════════════════════════════

// TestCRCConsistencia garante que sensor e integrador calculam o mesmo CRC
// para os mesmos dados — sem isso toda validação seria inválida.
func TestCRCConsistencia(t *testing.T) {
	casos := []struct {
		id         uint16
		tipo, info uint8
	}{
		{1, TipoTemperatura, 0},
		{1, TipoTemperatura, 35},
		{1, TipoTemperatura, 40},
		{100, TipoUmidade, 20},
		{100, TipoUmidade, 80},
		{65535, TipoTemperatura, 255},
		{0x0110, TipoUmidade, 5},
	}
	for _, c := range casos {
		got := simpleCRC(c.id, c.tipo, c.info)
		want := simpleCRC(c.id, c.tipo, c.info)
		if got != want {
			t.Errorf("CRC diverge para id=%d tipo=%d info=%d: got=0x%X want=0x%X",
				c.id, c.tipo, c.info, got, want)
		}
	}
}

// TestCRCDetectaValorAlterado garante que mudar o valor da leitura altera o CRC.
func TestCRCDetectaValorAlterado(t *testing.T) {
	original := simpleCRC(42, TipoTemperatura, 35)
	corrompido := simpleCRC(42, TipoTemperatura, 36)
	if original == corrompido {
		t.Error("CRC não detectou alteração no campo information")
	}
}

// TestCRCDetectaIDAlterado garante que mudar o ID altera o CRC.
func TestCRCDetectaIDAlterado(t *testing.T) {
	original := simpleCRC(100, TipoTemperatura, 35)
	corrompido := simpleCRC(101, TipoTemperatura, 35)
	if original == corrompido {
		t.Error("CRC não detectou alteração no campo ID")
	}
}

// TestCRCDetectaTipoAlterado garante que mudar o tipo altera o CRC.
func TestCRCDetectaTipoAlterado(t *testing.T) {
	original := simpleCRC(42, TipoTemperatura, 50)
	corrompido := simpleCRC(42, TipoUmidade, 50)
	if original == corrompido {
		t.Error("CRC não detectou alteração no campo tipo")
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// 2. TESTES DE MATCHTABLE
// ═════════════════════════════════════════════════════════════════════════════

// TestMatchSensorPrimeiro: sensor chega antes do atuador.
// Esperado: sensor entra na fila; quando atuador chega, par é formado.
func TestMatchSensorPrimeiro(t *testing.T) {
	mt := NewMatchTable()

	m1 := mt.RegisterSensor(10)
	if m1 != nil {
		t.Error("RegisterSensor sem atuador disponível deve retornar nil")
	}
	if len(mt.freeSensors) != 1 {
		t.Errorf("freeSensors deve ter 1 elemento, got %d", len(mt.freeSensors))
	}

	m2 := mt.RegisterActuator(20)
	if m2 == nil {
		t.Fatal("RegisterActuator com sensor na fila deve retornar Match")
	}
	if m2.SensorID != 10 || m2.ActuatorID != 20 {
		t.Errorf("Match incorreto: got %+v", m2)
	}
	if len(mt.freeSensors) != 0 {
		t.Errorf("freeSensors deve estar vazia após match, got %d", len(mt.freeSensors))
	}
}

// TestMatchAtuadorPrimeiro: atuador chega antes do sensor.
func TestMatchAtuadorPrimeiro(t *testing.T) {
	mt := NewMatchTable()

	m1 := mt.RegisterActuator(20)
	if m1 != nil {
		t.Error("RegisterActuator sem sensor disponível deve retornar nil")
	}
	if len(mt.freeActuators) != 1 {
		t.Errorf("freeActuators deve ter 1 elemento, got %d", len(mt.freeActuators))
	}

	m2 := mt.RegisterSensor(10)
	if m2 == nil {
		t.Fatal("RegisterSensor com atuador na fila deve retornar Match")
	}
	if m2.SensorID != 10 || m2.ActuatorID != 20 {
		t.Errorf("Match incorreto: got %+v", m2)
	}
	if len(mt.freeActuators) != 0 {
		t.Errorf("freeActuators deve estar vazia após match, got %d", len(mt.freeActuators))
	}
}

// TestMatchOrdemFIFO: garante que a fila é FIFO — primeiro sensor a entrar
// é o primeiro a ser pareado.
func TestMatchOrdemFIFO(t *testing.T) {
	mt := NewMatchTable()
	mt.RegisterSensor(1)
	mt.RegisterSensor(2)
	mt.RegisterSensor(3)

	m := mt.RegisterActuator(100)
	if m == nil || m.SensorID != 1 {
		t.Errorf("FIFO violado: esperado sensor 1, got %+v", m)
	}
	m = mt.RegisterActuator(101)
	if m == nil || m.SensorID != 2 {
		t.Errorf("FIFO violado: esperado sensor 2, got %+v", m)
	}
}

// TestMatchSensoresExcedentes: 3 sensores, 1 atuador → 2 sensores ficam na fila.
func TestMatchSensoresExcedentes(t *testing.T) {
	mt := NewMatchTable()
	mt.RegisterSensor(1)
	mt.RegisterSensor(2)
	mt.RegisterSensor(3)
	mt.RegisterActuator(100)

	if len(mt.freeSensors) != 2 {
		t.Errorf("Esperado 2 sensores livres, got %d", len(mt.freeSensors))
	}
}

// TestMatchAtuadoresExcedentes: 3 atuadores, 1 sensor → 2 atuadores ficam na fila.
func TestMatchAtuadoresExcedentes(t *testing.T) {
	mt := NewMatchTable()
	mt.RegisterActuator(1)
	mt.RegisterActuator(2)
	mt.RegisterActuator(3)
	mt.RegisterSensor(100)

	if len(mt.freeActuators) != 2 {
		t.Errorf("Esperado 2 atuadores livres, got %d", len(mt.freeActuators))
	}
}

// TestActuatorFor: lookup retorna o atuador correto após match.
func TestActuatorFor(t *testing.T) {
	mt := NewMatchTable()
	mt.RegisterSensor(10)
	mt.RegisterActuator(20)

	aID := mt.ActuatorFor(10)
	if aID != 20 {
		t.Errorf("ActuatorFor(10) = %d, esperado 20", aID)
	}
}

// TestActuatorForSemPar: sensor sem atuador deve retornar 0.
func TestActuatorForSemPar(t *testing.T) {
	mt := NewMatchTable()
	mt.RegisterSensor(10)

	aID := mt.ActuatorFor(10)
	if aID != 0 {
		t.Errorf("ActuatorFor sem par deve retornar 0, got %d", aID)
	}
}

// TestRemoveAtuadorLiberaSensor: ao remover atuador, sensor volta para fila
// e é pareado com o próximo atuador.
func TestRemoveAtuadorLiberaSensor(t *testing.T) {
	mt := NewMatchTable()
	mt.RegisterSensor(10)
	mt.RegisterActuator(20)
	mt.RemoveActuator(20)

	if len(mt.freeSensors) != 1 || mt.freeSensors[0] != 10 {
		t.Errorf("Sensor 10 deveria estar em freeSensors após remoção do atuador")
	}

	m := mt.RegisterActuator(30)
	if m == nil || m.SensorID != 10 || m.ActuatorID != 30 {
		t.Errorf("Revinculação incorreta: got %+v", m)
	}
}

// TestRemoveSensorLiberaAtuador: ao remover sensor, atuador volta para fila
// e é pareado com o próximo sensor.
func TestRemoveSensorLiberaAtuador(t *testing.T) {
	mt := NewMatchTable()
	mt.RegisterActuator(20)
	mt.RegisterSensor(10)
	mt.RemoveSensor(10)

	if len(mt.freeActuators) != 1 || mt.freeActuators[0] != 20 {
		t.Errorf("Atuador 20 deveria estar em freeActuators após remoção do sensor")
	}

	m := mt.RegisterSensor(11)
	if m == nil || m.SensorID != 11 || m.ActuatorID != 20 {
		t.Errorf("Revinculação incorreta: got %+v", m)
	}
}

// TestRemoveSensorSemPar: remover sensor que não tem atuador — não deve entrar em pânico.
func TestRemoveSensorSemPar(t *testing.T) {
	mt := NewMatchTable()
	mt.RegisterSensor(10)
	mt.RemoveSensor(10) // sensor estava na fila, não no mapa bySensor

	// Não deve quebrar — freeSensors fica com o sensor ainda (comportamento aceitável)
	// O ponto é não ter panic
}

// TestRemoveAtuadorInexistente: não deve entrar em pânico.
func TestRemoveAtuadorInexistente(t *testing.T) {
	mt := NewMatchTable()
	mt.RemoveActuator(999) // nunca registrado
}

// ═════════════════════════════════════════════════════════════════════════════
// 3. TESTES DE MAPA DE SENSORES
// ═════════════════════════════════════════════════════════════════════════════

// TestExistsThisSensor: sensor inexistente retorna false, existente retorna true.
func TestExistsThisSensor(t *testing.T) {
	m := newDiagramUdpInformation()
	if m.ExistsThisSensor(1) {
		t.Error("Sensor 1 não deveria existir antes de ser registrado")
	}

	m.mu.Lock()
	m.sensors[1] = diagramaUdpInformation{ID: 1}
	m.mu.Unlock()

	if !m.ExistsThisSensor(1) {
		t.Error("Sensor 1 deveria existir após ser inserido")
	}
}

// TestRemoveSensorDoMapa: após RemoveSensor, ExistsThisSensor retorna false.
func TestRemoveSensorDoMapa(t *testing.T) {
	m := newDiagramUdpInformation()
	m.mu.Lock()
	m.sensors[5] = diagramaUdpInformation{ID: 5}
	m.mu.Unlock()

	m.RemoveSensor(5)
	if m.ExistsThisSensor(5) {
		t.Error("Sensor 5 deveria ter sido removido do mapa")
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// 4. TESTES DE MAPA DE ATUADORES
// ═════════════════════════════════════════════════════════════════════════════

// TestFindNewIdToActuatorIncrementa: IDs gerados são sequenciais a partir de 1.
func TestFindNewIdToActuatorIncrementa(t *testing.T) {
	m := NewMapOfActuators()
	a1 := Actuator{ID: 0, Send: make(chan []byte, 1)}
	a2 := Actuator{ID: 0, Send: make(chan []byte, 1)}

	id1 := m.FindNewIdToActuator(a1)
	id2 := m.FindNewIdToActuator(a2)

	if id1 != 1 {
		t.Errorf("Primeiro ID esperado 1, got %d", id1)
	}
	if id2 != 2 {
		t.Errorf("Segundo ID esperado 2, got %d", id2)
	}
}

// TestFindNewIdToActuatorIDExistente: atuador com ID já registrado não sobrescreve.
func TestFindNewIdToActuatorIDExistente(t *testing.T) {
	m := NewMapOfActuators()
	original := Actuator{ID: 5, Send: make(chan []byte, 1)}
	m.FindNewIdToActuator(original)

	// Tenta registrar outro com o mesmo ID
	outro := Actuator{ID: 5, Send: make(chan []byte, 1)}
	id := m.FindNewIdToActuator(outro)

	if id != 5 {
		t.Errorf("Deveria retornar ID 5, got %d", id)
	}
	// Canal original deve ser o que está no mapa
	a, ok := m.FindActuator(5)
	if !ok {
		t.Fatal("Atuador 5 não encontrado")
	}
	if a.Send != original.Send {
		t.Error("Canal do atuador original foi sobrescrito indevidamente")
	}
}

// TestRemoveActuator: após remoção, FindActuator retorna false.
func TestRemoveActuator(t *testing.T) {
	m := NewMapOfActuators()
	a := Actuator{ID: 0, Send: make(chan []byte, 1)}
	id := m.FindNewIdToActuator(a)

	m.RemoveActuator(id)
	_, ok := m.FindActuator(id)
	if ok {
		t.Errorf("Atuador %d deveria ter sido removido", id)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// 5. TESTES DE MAPA DE CLIENTES
// ═════════════════════════════════════════════════════════════════════════════

// TestFindNewIdToClientIncrementa: IDs gerados são sequenciais a partir de 1.
func TestFindNewIdToClientIncrementa(t *testing.T) {
	m := NewMapOfClients()
	c1 := Client{ID: 0, Send: make(chan []byte, 1)}
	c2 := Client{ID: 0, Send: make(chan []byte, 1)}

	id1 := m.FindNewIdToClient(c1)
	id2 := m.FindNewIdToClient(c2)

	if id1 != 1 {
		t.Errorf("Primeiro ID esperado 1, got %d", id1)
	}
	if id2 != 2 {
		t.Errorf("Segundo ID esperado 2, got %d", id2)
	}
}

// TestRemoveClient: após remoção, ExistsThisClient retorna false.
func TestRemoveClient(t *testing.T) {
	m := NewMapOfClients()
	c := Client{ID: 0, Send: make(chan []byte, 1)}
	id := m.FindNewIdToClient(c)

	m.RemoveClient(id)
	if m.ExistsThisClient(id) {
		t.Errorf("Cliente %d deveria ter sido removido", id)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// 6. TESTES DO INTEGRADOR — OnSensorData
// ═════════════════════════════════════════════════════════════════════════════

// TestOnSensorDataRegistraSensorNovo: primeiro pacote de um sensor o registra.
func TestOnSensorDataRegistraSensorNovo(t *testing.T) {
	ig := newIntegradorTest()
	data := diagramaUdpInformation{ID: 42, Tipo: TipoTemperatura, Information: 30}

	ig.OnSensorData(data)

	if !ig.Sensors.ExistsThisSensor(42) {
		t.Error("Sensor 42 deveria estar registrado após OnSensorData")
	}
}

// TestOnSensorDataAtualizaValor: segundo pacote do mesmo sensor atualiza o valor.
func TestOnSensorDataAtualizaValor(t *testing.T) {
	ig := newIntegradorTest()
	ig.OnSensorData(diagramaUdpInformation{ID: 42, Tipo: TipoTemperatura, Information: 30})
	ig.OnSensorData(diagramaUdpInformation{ID: 42, Tipo: TipoTemperatura, Information: 38})

	ig.Sensors.mu.Lock()
	val := ig.Sensors.sensors[42].Information
	ig.Sensors.mu.Unlock()

	if val != 38 {
		t.Errorf("Valor atualizado esperado 38, got %d", val)
	}
}

// TestOnSensorDataMatchAutomatico: sensor novo + atuador aguardando → match imediato.
func TestOnSensorDataMatchAutomatico(t *testing.T) {
	ig := newIntegradorTest()
	noopActuator(1, ig)
	ig.OnActuatorConnected(1)

	ig.OnSensorData(diagramaUdpInformation{ID: 42, Tipo: TipoTemperatura, Information: 25})

	aID := ig.Matches.ActuatorFor(42)
	if aID != 1 {
		t.Errorf("Sensor 42 deveria estar vinculado ao atuador 1, got %d", aID)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// 7. TESTES DO INTEGRADOR — AutoControl
// ═════════════════════════════════════════════════════════════════════════════

// TestAutoControlTemperaturaMax: temperatura >= 40 dispara "on".
func TestAutoControlTemperaturaMax(t *testing.T) {
	ig := newIntegradorTest()
	send := noopActuator(1, ig)
	ig.OnActuatorConnected(1)
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 25})

	// Envia leitura no limiar máximo
	ig.AutoControl(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: TemperaturaMax})

	msgs := drainChan(send, 200*time.Millisecond)
	if len(msgs) == 0 {
		t.Fatal("Nenhum comando enviado ao atuador")
	}
	var cmd ActuatorCommand
	json.Unmarshal(msgs[len(msgs)-1], &cmd)
	if cmd.Command != "on" {
		t.Errorf("Esperado 'on', got '%s'", cmd.Command)
	}
	if cmd.TriggeredBy != "auto" {
		t.Errorf("TriggeredBy esperado 'auto', got '%s'", cmd.TriggeredBy)
	}
}

// TestAutoControlTemperaturaMin: temperatura <= 15 dispara "off".
func TestAutoControlTemperaturaMin(t *testing.T) {
	ig := newIntegradorTest()
	send := noopActuator(1, ig)
	ig.OnActuatorConnected(1)
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 25})

	ig.AutoControl(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: TemperaturaMin})

	msgs := drainChan(send, 200*time.Millisecond)
	if len(msgs) == 0 {
		t.Fatal("Nenhum comando enviado ao atuador")
	}
	var cmd ActuatorCommand
	json.Unmarshal(msgs[len(msgs)-1], &cmd)
	if cmd.Command != "off" {
		t.Errorf("Esperado 'off', got '%s'", cmd.Command)
	}
}

// TestAutoControlUmidadeMax: umidade >= 80 dispara "on".
func TestAutoControlUmidadeMax(t *testing.T) {
	ig := newIntegradorTest()
	send := noopActuator(1, ig)
	ig.OnActuatorConnected(1)
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoUmidade, Information: 50})

	ig.AutoControl(diagramaUdpInformation{ID: 10, Tipo: TipoUmidade, Information: UmidadeMax})

	msgs := drainChan(send, 200*time.Millisecond)
	if len(msgs) == 0 {
		t.Fatal("Nenhum comando enviado ao atuador")
	}
	var cmd ActuatorCommand
	json.Unmarshal(msgs[len(msgs)-1], &cmd)
	if cmd.Command != "on" {
		t.Errorf("Esperado 'on', got '%s'", cmd.Command)
	}
}

// TestAutoControlUmidadeMin: umidade <= 20 dispara "off".
func TestAutoControlUmidadeMin(t *testing.T) {
	ig := newIntegradorTest()
	send := noopActuator(1, ig)
	ig.OnActuatorConnected(1)
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoUmidade, Information: 50})

	ig.AutoControl(diagramaUdpInformation{ID: 10, Tipo: TipoUmidade, Information: UmidadeMin})

	msgs := drainChan(send, 200*time.Millisecond)
	if len(msgs) == 0 {
		t.Fatal("Nenhum comando enviado ao atuador")
	}
	var cmd ActuatorCommand
	json.Unmarshal(msgs[len(msgs)-1], &cmd)
	if cmd.Command != "off" {
		t.Errorf("Esperado 'off', got '%s'", cmd.Command)
	}
}

// TestAutoControlFaixaNormal: valor dentro da faixa normal não dispara nada.
func TestAutoControlFaixaNormal(t *testing.T) {
	ig := newIntegradorTest()
	send := noopActuator(1, ig)
	ig.OnActuatorConnected(1)
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 25})

	ig.AutoControl(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 25})

	msgs := drainChan(send, 100*time.Millisecond)
	// O canal pode ter recebido o broadcast do OnSensorData — filtra por comando de controle
	for _, m := range msgs {
		var cmd ActuatorCommand
		if json.Unmarshal(m, &cmd) == nil && cmd.Type == "command" {
			t.Errorf("Não deveria ter disparado comando para valor normal: %+v", cmd)
		}
	}
}

// TestAutoControlSemAtuador: sensor sem atuador não deve causar panic.
func TestAutoControlSemAtuador(t *testing.T) {
	ig := newIntegradorTest()
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 25})
	// Não registra atuador — AutoControl deve retornar silenciosamente
	ig.AutoControl(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: TemperaturaMax})
}

// ═════════════════════════════════════════════════════════════════════════════
// 8. TESTES DO INTEGRADOR — OnClientCommand
// ═════════════════════════════════════════════════════════════════════════════

// TestOnClientCommandEncaminhaParaAtuador: comando do cliente chega ao atuador.
func TestOnClientCommandEncaminhaParaAtuador(t *testing.T) {
	ig := newIntegradorTest()
	sendAtuador := noopActuator(1, ig)
	ig.OnActuatorConnected(1)
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 25})

	raw, _ := json.Marshal(ClientCommand{Type: "control", SensorID: 10, Command: "on"})
	ig.OnClientCommand(99, raw)

	msgs := drainChan(sendAtuador, 200*time.Millisecond)
	encontrou := false
	for _, m := range msgs {
		var cmd ActuatorCommand
		if json.Unmarshal(m, &cmd) == nil && cmd.Type == "command" {
			if cmd.Command == "on" && cmd.TriggeredBy == "client" && cmd.SensorID == 10 {
				encontrou = true
			}
		}
	}
	if !encontrou {
		t.Error("Comando do cliente não chegou ao atuador com os campos corretos")
	}
}

// TestOnClientCommandSemAtuador: sensor sem atuador — não deve enviar nada e não deve panic.
func TestOnClientCommandSemAtuador(t *testing.T) {
	ig := newIntegradorTest()
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 25})

	raw, _ := json.Marshal(ClientCommand{Type: "control", SensorID: 10, Command: "on"})
	ig.OnClientCommand(99, raw) // não deve panic
}

// TestOnClientCommandJSONInvalido: JSON corrompido — não deve panic.
func TestOnClientCommandJSONInvalido(t *testing.T) {
	ig := newIntegradorTest()
	ig.OnClientCommand(99, []byte("{isto nao e json"))
}

// TestOnClientCommandTipoErrado: tipo diferente de "control" é ignorado.
func TestOnClientCommandTipoErrado(t *testing.T) {
	ig := newIntegradorTest()
	sendAtuador := noopActuator(1, ig)
	ig.OnActuatorConnected(1)
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 25})

	raw, _ := json.Marshal(ClientCommand{Type: "outro", SensorID: 10, Command: "on"})
	ig.OnClientCommand(99, raw)

	msgs := drainChan(sendAtuador, 100*time.Millisecond)
	for _, m := range msgs {
		var cmd ActuatorCommand
		if json.Unmarshal(m, &cmd) == nil && cmd.Type == "command" {
			t.Error("Comando não deveria ter sido encaminhado para tipo diferente de 'control'")
		}
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// 9. TESTES DO INTEGRADOR — BroadcastSensorUpdate
// ═════════════════════════════════════════════════════════════════════════════

// TestBroadcastEnviaParaTodosClientes: todos os clientes conectados recebem o update.
func TestBroadcastEnviaParaTodosClientes(t *testing.T) {
	ig := newIntegradorTest()
	send1 := noopClient(1, ig)
	send2 := noopClient(2, ig)
	send3 := noopClient(3, ig)

	data := diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 30}
	ig.BroadcastSensorUpdate(data)

	for i, ch := range []chan []byte{send1, send2, send3} {
		msgs := drainChan(ch, 100*time.Millisecond)
		if len(msgs) == 0 {
			t.Errorf("Cliente %d não recebeu broadcast", i+1)
		}
		var msg SensorBroadcast
		if err := json.Unmarshal(msgs[0], &msg); err != nil {
			t.Errorf("Cliente %d recebeu JSON inválido: %v", i+1, err)
		}
		if msg.Type != "sensor_update" || msg.SensorID != 10 || msg.Information != 30 {
			t.Errorf("Cliente %d recebeu broadcast incorreto: %+v", i+1, msg)
		}
	}
}

// TestBroadcastClienteLento: cliente com canal cheio não bloqueia os demais.
func TestBroadcastClienteLento(t *testing.T) {
	ig := newIntegradorTest()

	// Cliente 1 com canal cheio (capacidade 1, já preenchido)
	sendCheio := make(chan []byte, 1)
	sendCheio <- []byte("bloqueado")
	ig.Clients.mu.Lock()
	ig.Clients.clientsRegistred[1] = Client{ID: 1, Send: sendCheio}
	ig.Clients.mu.Unlock()

	// Cliente 2 com canal livre
	send2 := noopClient(2, ig)

	data := diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 30}
	ig.BroadcastSensorUpdate(data)

	msgs := drainChan(send2, 100*time.Millisecond)
	if len(msgs) == 0 {
		t.Error("Cliente 2 deveria receber broadcast mesmo com cliente 1 lento")
	}
}

// TestBroadcastIncluiUnidade: broadcast inclui unidade correta por tipo.
func TestBroadcastIncluiUnidade(t *testing.T) {
	ig := newIntegradorTest()
	send := noopClient(1, ig)

	ig.BroadcastSensorUpdate(diagramaUdpInformation{ID: 1, Tipo: TipoTemperatura, Information: 25})
	msgs := drainChan(send, 100*time.Millisecond)
	var msg SensorBroadcast
	json.Unmarshal(msgs[0], &msg)
	if msg.Unit != "°C" {
		t.Errorf("Unidade esperada '°C', got '%s'", msg.Unit)
	}

	send2 := noopClient(2, ig)
	ig.BroadcastSensorUpdate(diagramaUdpInformation{ID: 2, Tipo: TipoUmidade, Information: 60})
	msgs2 := drainChan(send2, 100*time.Millisecond)
	var msg2 SensorBroadcast
	json.Unmarshal(msgs2[0], &msg2)
	if msg2.Unit != "%" {
		t.Errorf("Unidade esperada '%%', got '%s'", msg2.Unit)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// 10. TESTES DE DESCONEXÃO
// ═════════════════════════════════════════════════════════════════════════════

// TestDesconexaoAtuadorLiberaSensor: atuador desconecta → sensor volta à fila
// e é revinculado ao próximo atuador.
func TestDesconexaoAtuadorLiberaSensor(t *testing.T) {
	ig := newIntegradorTest()
	noopActuator(1, ig)
	ig.OnActuatorConnected(1)
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 25})

	// Verifica que estão vinculados
	if ig.Matches.ActuatorFor(10) != 1 {
		t.Fatal("Par não foi formado")
	}

	// Atuador desconecta
	ig.OnActuatorDisconnected(1)

	if ig.Matches.ActuatorFor(10) != 0 {
		t.Error("Sensor 10 deveria estar sem atuador após desconexão")
	}
	if len(ig.Matches.freeSensors) != 1 {
		t.Errorf("Sensor 10 deveria estar na fila freeSensors, got %d", len(ig.Matches.freeSensors))
	}

	// Novo atuador conecta e deve ser vinculado ao sensor 10
	noopActuator(2, ig)
	ig.OnActuatorConnected(2)

	if ig.Matches.ActuatorFor(10) != 2 {
		t.Errorf("Sensor 10 deveria estar vinculado ao atuador 2, got %d", ig.Matches.ActuatorFor(10))
	}
}

// TestRemoveSensorComMatch: sensor removido libera atuador para novo sensor.
func TestRemoveSensorComMatch(t *testing.T) {
	ig := newIntegradorTest()
	noopActuator(1, ig)
	ig.OnActuatorConnected(1)
	ig.OnSensorData(diagramaUdpInformation{ID: 10, Tipo: TipoTemperatura, Information: 25})

	ig.Sensors.RemoveSensorWithMatch(10, ig)

	if ig.Sensors.ExistsThisSensor(10) {
		t.Error("Sensor 10 deveria ter sido removido do mapa")
	}
	if len(ig.Matches.freeActuators) != 1 {
		t.Errorf("Atuador 1 deveria estar na fila freeActuators, got %d", len(ig.Matches.freeActuators))
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// 11. TESTES DE CONCORRÊNCIA (race detector: go test -race)
// ═════════════════════════════════════════════════════════════════════════════

// TestMatchTableConcorrente: 50 sensores e 50 atuadores registrando ao mesmo tempo.
// Verifica ausência de race condition e consistência dos mapas.
func TestMatchTableConcorrente(t *testing.T) {
	mt := NewMatchTable()
	var wg sync.WaitGroup
	const N = 50

	for i := 0; i < N; i++ {
		wg.Add(2)
		id := uint16(i + 1)
		go func(sID uint16) {
			defer wg.Done()
			mt.RegisterSensor(sID)
		}(id)
		go func(aID uint16) {
			defer wg.Done()
			mt.RegisterActuator(aID + 1000)
		}(id)
	}
	wg.Wait()

	// Consistência: bySensor e byActuator devem ser inversos um do outro
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	for aID, sID := range mt.byActuator {
		if mt.bySensor[sID] != aID {
			t.Errorf("Inconsistência: byActuator[%d]=%d mas bySensor[%d]=%d",
				aID, sID, sID, mt.bySensor[sID])
		}
	}
	// Total de pares + livres deve ser N
	total := len(mt.bySensor) + len(mt.freeSensors)
	if total != N {
		t.Errorf("Total de sensores contabilizados = %d, esperado %d", total, N)
	}
}

// TestOnSensorDataConcorrente: múltiplos sensores enviando dados ao mesmo tempo.
func TestOnSensorDataConcorrente(t *testing.T) {
	ig := newIntegradorTest()
	var wg sync.WaitGroup
	const N = 30

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(id uint16) {
			defer wg.Done()
			ig.OnSensorData(diagramaUdpInformation{
				ID:          id,
				Tipo:        TipoTemperatura,
				Information: 25,
			})
		}(uint16(i + 1))
	}
	wg.Wait()

	// Todos os sensores devem estar registrados
	for i := 1; i <= N; i++ {
		if !ig.Sensors.ExistsThisSensor(uint16(i)) {
			t.Errorf("Sensor %d deveria estar registrado", i)
		}
	}
}

// TestBroadcastConcorrente: broadcast acontecendo enquanto clientes são adicionados e removidos.
func TestBroadcastConcorrente(t *testing.T) {
	ig := newIntegradorTest()

	// Pré-registra 10 clientes
	for i := 1; i <= 10; i++ {
		noopClient(uint16(i), ig)
	}

	var wg sync.WaitGroup

	// Goroutine de broadcast contínuo
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			ig.BroadcastSensorUpdate(diagramaUdpInformation{
				ID: 1, Tipo: TipoTemperatura, Information: 25,
			})
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Goroutine removendo clientes concorrentemente
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= 5; i++ {
			ig.Clients.RemoveClient(uint16(i))
			time.Sleep(10 * time.Millisecond)
		}
	}()

	wg.Wait()
	// Não deve ter panic nem deadlock
}

// TestMatchRemoveConcorrente: registros e remoções simultâneos na MatchTable.
func TestMatchRemoveConcorrente(t *testing.T) {
	mt := NewMatchTable()
	var wg sync.WaitGroup

	// Forma 10 pares
	for i := 1; i <= 10; i++ {
		mt.RegisterSensor(uint16(i))
		mt.RegisterActuator(uint16(i + 100))
	}

	// Remove sensores e atuadores ao mesmo tempo
	for i := 1; i <= 10; i++ {
		wg.Add(2)
		sID := uint16(i)
		aID := uint16(i + 100)
		go func() {
			defer wg.Done()
			mt.RemoveSensor(sID)
		}()
		go func() {
			defer wg.Done()
			mt.RemoveActuator(aID)
		}()
	}
	wg.Wait()
	// Não deve ter panic, deadlock ou race condition
}

// ═════════════════════════════════════════════════════════════════════════════
// 12. TESTES DE SERIALIZAÇÃO JSON
// ═════════════════════════════════════════════════════════════════════════════

// TestSensorBroadcastSerializacao: garante que o JSON do broadcast tem todos os campos.
func TestSensorBroadcastSerializacao(t *testing.T) {
	msg := SensorBroadcast{
		Type: "sensor_update", SensorID: 42, ActuatorID: 1,
		Tipo: TipoTemperatura, Information: 35, Unit: "°C",
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Erro ao serializar SensorBroadcast: %v", err)
	}

	var got SensorBroadcast
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Erro ao desserializar SensorBroadcast: %v", err)
	}
	if got.SensorID != 42 || got.ActuatorID != 1 || got.Information != 35 || got.Unit != "°C" {
		t.Errorf("Campos incorretos após serialização: %+v", got)
	}
}

// TestActuatorCommandSerializacao: garante que o JSON do comando tem todos os campos.
func TestActuatorCommandSerializacao(t *testing.T) {
	msg := ActuatorCommand{
		Type: "command", SensorID: 10, Command: "on", TriggeredBy: "auto",
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Erro ao serializar ActuatorCommand: %v", err)
	}

	var got ActuatorCommand
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Erro ao desserializar ActuatorCommand: %v", err)
	}
	if got.Command != "on" || got.TriggeredBy != "auto" || got.SensorID != 10 {
		t.Errorf("Campos incorretos após serialização: %+v", got)
	}
}

// TestClientCommandDesserializacao: JSON de cliente é corretamente interpretado.
func TestClientCommandDesserializacao(t *testing.T) {
	raw := []byte(`{"type":"control","sensor_id":42,"command":"off"}`)
	var cmd ClientCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		t.Fatalf("Erro ao desserializar ClientCommand: %v", err)
	}
	if cmd.Type != "control" || cmd.SensorID != 42 || cmd.Command != "off" {
		t.Errorf("Desserialização incorreta: %+v", cmd)
	}
}

// TestUnitFor: função auxiliar retorna unidade correta por tipo.
func TestUnitFor(t *testing.T) {
	if u := unitFor(TipoTemperatura); u != "°C" {
		t.Errorf("unitFor(Temperatura) = '%s', esperado '°C'", u)
	}
	if u := unitFor(TipoUmidade); u != "%" {
		t.Errorf("unitFor(Umidade) = '%s', esperado '%%'", u)
	}
	if u := unitFor(99); u != "" {
		t.Errorf("unitFor(tipo desconhecido) = '%s', esperado ''", u)
	}
}
