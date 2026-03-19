// sensor/main.go
package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	// 1. Resolver o endereço do outro container pelo nome do serviço Docker
	serverAddr, _ := net.ResolveUDPAddr("udp", "receptor-service:5000")

	// 2. "Conectar" (No UDP, isso apenas define o destino padrão)
	conn, _ := net.DialUDP("udp", nil, serverAddr)
	defer conn.Close()

	for {
		mensagem := fmt.Sprintf("Leitura do sensor: %d", time.Now().Unix())

		// 3. Enviar o datagrama
		_, _ = conn.Write([]byte(mensagem))

		fmt.Println("Enviado:", mensagem)
		time.Sleep(2 * time.Second)
	}
}




/*

// sensor/main.go
package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	// 1. Resolver o endereço do outro container pelo nome do serviço Docker
	serverAddr, _ := net.ResolveUDPAddr("udp", "receptor-service:5000")

	// 2. "Conectar" (No UDP, isso apenas define o destino padrão)
	conn, _ := net.DialUDP("udp", nil, serverAddr)
	defer conn.Close()

	for {
		mensagem := fmt.Sprintf("Leitura do sensor: %d", time.Now().Unix())

		// 3. Enviar o datagrama
		_, _ = conn.Write([]byte(mensagem))

		fmt.Println("Enviado:", mensagem)
		time.Sleep(2 * time.Second)
	}
}

*/
