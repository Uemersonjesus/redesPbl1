package main

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
)

func handleActuatorWebsocketConnection(w http.ResponseWriter, r *http.Request, globalActuators *MapOfActuators, ig *Integrador) {
	// 1. Validação do Upgrade
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "Esperado upgrade para websocket", http.StatusBadRequest)
		return
	}

	// 2. Cálculo da Chave de Aceitação (RFC 6455)
	challengeKey := r.Header.Get("Sec-WebSocket-Key")
	guid := "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(challengeKey + guid))
	acceptKey := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// 3. Hijacking
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Servidor não suporta hijacking", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()

	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"

	bufrw.WriteString(response)
	bufrw.Flush()

	fmt.Println("Conexão de ATUADOR estabelecida na porta 9090!")

	a := Actuator{
		ID:    0,
		Conn:  conn,
		Bufrw: bufrw,
		Send:  make(chan []byte, 256),
	}

	oficialId := globalActuators.FindNewIdToActuator(a)
	a.ID = oficialId

	/*
		Bug de  race condition
		globalActuators.mu.Lock()
		globalActuators.actuatorsRegistred[oficialId] = a
		globalActuators.mu.Unlock()
	*/
	welcomeMsg := []byte(fmt.Sprintf(`{"type":"actuator_ack","id":%d}`, oficialId))
	a.Send <- welcomeMsg

	// Notifica o integrador → tenta matchmaking com sensor livre
	ig.OnActuatorConnected(oficialId)

	go a.writePump()
	a.readPump() // Loop de leitura — aguarda confirmações do hardware

	// Limpeza ao desconectar
	globalActuators.RemoveActuator(oficialId)
	ig.OnActuatorDisconnected(oficialId)
	fmt.Printf("Atuador %d desconectado da porta 9090.\n", oficialId)
}
