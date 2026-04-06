package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"time"
)

type diagramaUdpInformation struct {
	ID          uint16 `json:"id"`
	Tipo        uint8  `json:"tipo"`
	Information uint8  `json:"information"`
	Crc         uint8  `json:"crc"`
}

const (
	TipoUmidade uint8 = 2
	UmidadeMin  uint8 = 0
	UmidadeMax  uint8 = 100
	PortaUDP          = "5000"
)

func generateID() uint16 {
	n, err := rand.Int(rand.Reader, big.NewInt(65535))
	if err != nil {
		log.Fatalf("Erro ao gerar ID: %v", err)
	}
	return uint16(n.Int64()) + 1
}

// Este é o ckecksum , aposteriori será implementado o crc
func simpleCRC(id uint16, tipo, info uint8) uint8 {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, id)
	sum := uint16(buf[0]) + uint16(buf[1]) + uint16(tipo) + uint16(info)
	return uint8(sum & 0xFF)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Uso: ./sensor_umidade <ip_integrador>")
		fmt.Println("Ex:  ./sensor_umidade 192.168.1.10")
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%s", os.Args[1], PortaUDP)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatalf("Endereço inválido %s: %v", addr, err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		log.Fatalf("Erro ao abrir socket UDP: %v", err)
	}
	defer conn.Close()

	sensorID := generateID()
	fmt.Printf("Sensor Umidade | ID=%d | → %s\n", sensorID, addr)

	value := UmidadeMin
	ascending := true

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		crc := simpleCRC(sensorID, TipoUmidade, value)

		payload := diagramaUdpInformation{
			ID:          sensorID,
			Tipo:        TipoUmidade,
			Information: value,
			Crc:         crc,
		}

		raw, _ := json.Marshal(payload)

		if _, err := conn.Write(raw); err != nil {
			log.Printf("Erro ao enviar: %v", err)
			continue
		}

		fmt.Printf("📤 ID=%d  %d%%\n", sensorID, value)

		if ascending {
			if value >= UmidadeMax {
				ascending = false
				value--
			} else {
				value++
			}
		} else {
			if value <= UmidadeMin {
				ascending = true
				value++
			} else {
				value--
			}
		}
	}
}
