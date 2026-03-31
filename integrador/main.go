package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
)

func main() {
	fmt.Println("Integrador Iniciado")

	
	mapOfSensors := newDiagramUdpInformation()
	globalMapOfClients := NewMapOfClients()
	globalMapOfActuators := NewMapOfActuators()

	
	ig := NewIntegrador(&mapOfSensors, &globalMapOfClients, &globalMapOfActuators)

	
	addr, err := net.ResolveUDPAddr("udp", ":5000")
	if err != nil {
		log.Fatalf("Erro ao resolver endereço UDP: %v", err)
	}
	connUDP, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Erro ao abrir socket UDP: %v", err)
	}
	fmt.Println("UDP sensor listener: porta 5000")

	go managerUdpConnections(connUDP, &mapOfSensors, ig)

	
	muxClientes := http.NewServeMux()
	muxClientes.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleNativeWebsocketConection(w, r, &globalMapOfClients, ig)
	})

	go func() {
		fmt.Println("dashboard Server: porta 8080")
		log.Fatal(http.ListenAndServe(":8080", muxClientes))
	}()

	muxAtuadores := http.NewServeMux()
	muxAtuadores.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleActuatorWebsocketConnection(w, r, &globalMapOfActuators, ig)
	})

	fmt.Println("Actuator Server: porta 9090")
	log.Fatal(http.ListenAndServe(":9090", muxAtuadores))
}
