package main

import (
	"fmt"
	"net"
)

func main() {
	// O servidor ouve em todas as interfaces (0.0.0.0) na porta 5000
	addr, _ := net.ResolveUDPAddr("udp", ":5000")
	conn, _ := net.ListenUDP("udp", addr)
	defer conn.Close()

	fmt.Println("Servidor UDP rodando na porta 5000...")

	buf := make([]byte, 1024)
	for {
		// n = tamanho dos dados, remoteAddr = quem enviou (o IP do container sensor)
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Println("Erro:", err)
			continue
		}

		fmt.Printf("Recebido de %s: %s\n", remoteAddr, string(buf[:n]))
	}
}
