package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
)

// Comando recebido do integrador
type ActuatorCommand struct {
	Type        string `json:"type"`
	SensorID    uint16 `json:"sensor_id"`
	Command     string `json:"command"`      // "on" | "off"
	TriggeredBy string `json:"triggered_by"` // "auto" | "client"
}

const (
	PortaWS = "9090"
)

// generateKey gera a Sec-WebSocket-Key (16 bytes aleatórios em base64).
func generateKey() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// expectedAccept calcula o Sec-WebSocket-Accept esperado para validar o handshake.
func expectedAccept(key string) string {
	guid := "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(key + guid))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Uso: ./atuador <ip_integrador>")
		fmt.Println("Ex:  ./atuador 192.168.1.10")
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%s", os.Args[1], PortaWS)

	// ── Abre conexão TCP com o integrador ────────────────────────────────────
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatalf("Não foi possível conectar ao integrador em %s: %v", addr, err)
	}
	defer conn.Close()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	// ── Handshake WebSocket (RFC 6455 — lado cliente) ─────────────────────────
	key := generateKey()

	handshake := "GET /ws HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	rw.WriteString(handshake)
	rw.Flush()

	// Lê a resposta HTTP 101
	for {
		line, err := rw.ReadString('\n')
		if err != nil {
			log.Fatalf("Erro ao ler handshake: %v", err)
		}
		if line == "\r\n" {
			break // fim dos headers
		}
	}

	fmt.Printf("Atuador conectado ao integrador em %s\n", addr)
	fmt.Println("Aguardando comandos...")

	// ── Loop de leitura de frames WebSocket ──────────────────────────────────
	for {
		// Lê os 2 bytes do header do frame
		header := make([]byte, 2)
		if _, err := rw.Read(header); err != nil {
			fmt.Println("Conexão encerrada pelo integrador.")
			break
		}

		opcode := header[0] & 0x0F

		// Close frame
		if opcode == 8 {
			fmt.Println("Integrador enviou close frame.")
			break
		}

		payloadLen := int(header[1] & 0x7F)

		// Lê o payload (sem máscara — servidor não mascara)
		payload := make([]byte, payloadLen)
		if _, err := rw.Read(payload); err != nil {
			fmt.Printf("Erro ao ler payload: %v\n", err)
			break
		}

		// Tenta parsear como comando
		var cmd ActuatorCommand
		if err := json.Unmarshal(payload, &cmd); err != nil {
			fmt.Printf("Mensagem raw: %s\n", string(payload))
			continue
		}

		// É uma mensagem de boas-vindas do integrador
		if cmd.Type == "actuator_ack" {
			fmt.Printf("Registrado no integrador\n")
			continue
		}

		// Comando de ligar/desligar
		if cmd.Command == "on" {
			fmt.Printf("LIGANDO  — sensor=%d  origem=%s\n", cmd.SensorID, cmd.TriggeredBy)
		} else if cmd.Command == "off" {
			fmt.Printf("DESLIGANDO — sensor=%d  origem=%s\n", cmd.SensorID, cmd.TriggeredBy)
		} else {
			fmt.Printf("Comando desconhecido: %s\n", string(payload))
		}
	}
}
