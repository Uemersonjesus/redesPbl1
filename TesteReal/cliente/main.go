package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

// ── Tipos espelhados do integrador ───────────────────────────────────────────

type SensorBroadcast struct {
	Type        string `json:"type"`
	SensorID    uint16 `json:"sensor_id"`
	ActuatorID  uint16 `json:"actuator_id"`
	Tipo        uint8  `json:"tipo"`
	Information uint8  `json:"information"`
	Unit        string `json:"unit"`
}

type ClientCommand struct {
	Type     string `json:"type"`
	SensorID uint16 `json:"sensor_id"`
	Command  string `json:"command"`
}

// ── Estado local do cliente ───────────────────────────────────────────────────

type SensorState struct {
	ID          uint16
	Tipo        uint8
	Information uint8
	Unit        string
	ActuatorID  uint16
}

type LocalStore struct {
	mu      sync.RWMutex
	sensors map[uint16]SensorState
}

func NewLocalStore() *LocalStore {
	return &LocalStore{sensors: make(map[uint16]SensorState)}
}

func (s *LocalStore) Update(b SensorBroadcast) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sensors[b.SensorID] = SensorState{
		ID:          b.SensorID,
		Tipo:        b.Tipo,
		Information: b.Information,
		Unit:        b.Unit,
		ActuatorID:  b.ActuatorID,
	}
}

func (s *LocalStore) Get(id uint16) (SensorState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.sensors[id]
	return st, ok
}

func (s *LocalStore) PrintList() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.sensors) == 0 {
		fmt.Println("  (nenhum sensor ainda)")
		return
	}

	fmt.Println("┌─────────┬──────────────┬──────────────────────┐")
	fmt.Println("│   ID    │     Tipo     │      Atuador         │")
	fmt.Println("├─────────┼──────────────┼──────────────────────┤")
	for _, st := range s.sensors {
		tipo := tipoName(st.Tipo)
		atuador := "sem atuador"
		if st.ActuatorID != 0 {
			atuador = fmt.Sprintf("vinculado (ID=%d)", st.ActuatorID)
		}
		fmt.Printf("│  %-6d │  %-11s │  %-19s │\n", st.ID, tipo, atuador)
	}
	fmt.Println("└─────────┴──────────────┴──────────────────────┘")
}

func tipoName(tipo uint8) string {
	switch tipo {
	case 1:
		return "temperatura"
	case 2:
		return "umidade"
	default:
		return "desconhecido"
	}
}

// ── WebSocket helpers ─────────────────────────────────────────────────────────

func generateKey() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// Envia um frame WebSocket texto (sem máscara — lado servidor não exige,
// mas o RFC 6455 diz que cliente DEVE mascarar).
func sendFrame(conn net.Conn, payload []byte) error {
	// Máscara obrigatória para frames de cliente (RFC 6455 §5.3)
	mask := make([]byte, 4)
	rand.Read(mask)

	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}

	frame := []byte{0x81, byte(0x80 | len(payload))}
	frame = append(frame, mask...)
	frame = append(frame, masked...)

	_, err := conn.Write(frame)
	return err
}

const PortaWS = "8080"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Uso: ./cliente <ip_integrador>")
		fmt.Println("Ex:  ./cliente 192.168.1.10")
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%s", os.Args[1], PortaWS)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Printf("Não foi possível conectar ao integrador em %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer conn.Close()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	key := generateKey()
	handshake := "GET /ws HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	rw.WriteString(handshake)
	rw.Flush()

	// Descarta os headers da resposta 101
	for {
		line, err := rw.ReadString('\n')
		if err != nil {
			fmt.Printf("Erro no handshake: %v\n", err)
			os.Exit(1)
		}
		if line == "\r\n" {
			break
		}
	}

	fmt.Printf("Conectado ao integrador %s\n\n", addr)

	store := NewLocalStore()

	// Canal para notificar a goroutine de monitor que chegou dado novo
	// do sensor que está sendo monitorado
	monitorCh := make(chan SensorState, 10)
	var monitoringID uint16 = 0
	var monitorMu sync.Mutex

	// ── Goroutine: lê frames do integrador ───────────────────────────────────
	go func() {
		for {
			header := make([]byte, 2)
			if _, err := rw.Read(header); err != nil {
				fmt.Println("\n🔌 Conexão com o integrador encerrada.")
				os.Exit(0)
			}

			opcode := header[0] & 0x0F
			if opcode == 8 {
				fmt.Println("\nIntegrador encerrou a conexão.")
				os.Exit(0)
			}

			payloadLen := int(header[1] & 0x7F)
			payload := make([]byte, payloadLen)
			rw.Read(payload)

			var msg SensorBroadcast
			if err := json.Unmarshal(payload, &msg); err != nil {
				continue
			}

			if msg.Type != "sensor_update" {
				continue
			}

			store.Update(msg)

			// Se este sensor é o que está sendo monitorado, notifica
			monitorMu.Lock()
			watching := monitoringID
			monitorMu.Unlock()

			if watching != 0 && msg.SensorID == watching {
				st, _ := store.Get(msg.SensorID)
				select {
				case monitorCh <- st:
				default:
				}
			}
		}
	}()

	stdin := bufio.NewReader(os.Stdin)

	for {
		// ── Tela principal: lista de sensores ─────────────────────────────────
		fmt.Println("Sensores disponíveis:")
		store.PrintList()
		fmt.Print("\nDigite o ID do sensor para monitorar (ou Enter para atualizar): ")

		line, _ := stdin.ReadString('\n')
		line = strings.TrimSpace(line)

		if line == "" {
			fmt.Println()
			continue
		}

		id64, err := strconv.ParseUint(line, 10, 16)
		if err != nil {
			fmt.Println("ID inválido.\n")
			continue
		}

		sensorID := uint16(id64)
		st, ok := store.Get(sensorID)
		if !ok {
			fmt.Printf("Sensor %d não encontrado. Aguarde e tente novamente.\n\n", sensorID)
			continue
		}

		// ── Tela de monitoramento ─────────────────────────────────────────────
		monitorMu.Lock()
		monitoringID = sensorID
		monitorMu.Unlock()

		fmt.Printf("\nMonitorando sensor %d (%s)\n", sensorID, tipoName(st.Tipo))
		if st.ActuatorID != 0 {
			fmt.Printf("Atuador vinculado: ID=%d\n", st.ActuatorID)
		} else {
			fmt.Println("Sem atuador vinculado")
		}
		fmt.Println("Comandos: on | off | sair")
		fmt.Println(strings.Repeat("─", 45))

		// Goroutine que printa atualizações do sensor monitorado
		stopPrint := make(chan struct{})
		go func() {
			for {
				select {
				case update := <-monitorCh:
					atuador := "sem atuador"
					if update.ActuatorID != 0 {
						atuador = fmt.Sprintf("atuador=%d", update.ActuatorID)
					}
					fmt.Printf("\r ID=%-5d  %3d%-3s  %s          ",
						update.ID, update.Information, update.Unit, atuador)
				case <-stopPrint:
					return
				}
			}
		}()

		// Loop de comandos manuais
		for {
			fmt.Print("\n> ")
			cmd, _ := stdin.ReadString('\n')
			cmd = strings.TrimSpace(strings.ToLower(cmd))

			if cmd == "sair" {
				close(stopPrint)
				monitorMu.Lock()
				monitoringID = 0
				monitorMu.Unlock()
				fmt.Println("\n")
				break
			}

			if cmd != "on" && cmd != "off" {
				fmt.Println("Comando inválido. Use: on | off | sair")
				continue
			}

			// Verifica se tem atuador antes de enviar
			current, _ := store.Get(sensorID)
			if current.ActuatorID == 0 {
				fmt.Println("este sensor não possui atuador vinculado.")
				continue
			}

			msg := ClientCommand{
				Type:     "control",
				SensorID: sensorID,
				Command:  cmd,
			}
			raw, _ := json.Marshal(msg)

			if err := sendFrame(conn, raw); err != nil {
				fmt.Printf("Erro ao enviar comando: %v\n", err)
				close(stopPrint)
				break
			}

			fmt.Printf("Comando '%s' enviado para sensor %d\n", cmd, sensorID)
		}
	}
}
