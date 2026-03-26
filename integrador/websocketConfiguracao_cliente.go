package main

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
)

func handleNativeWebsocketConection(w http.ResponseWriter, r *http.Request, globalClients *MapOfClients, ig *Integrador) {
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

	fmt.Println("🖥️  Conexão WebSocket de CLIENTE estabelecida!")

	c := Client{
		ID:    0,
		Conn:  conn,
		Bufrw: bufrw,
		Send:  make(chan []byte, 256),
	}

	oficialId := globalClients.FindNewIdToClient(c)
	c.ID = oficialId

	globalClients.mu.Lock()
	globalClients.clientsRegistred[oficialId] = c
	globalClients.mu.Unlock()

	// Boas-vindas com lista atual de sensores
	welcomeMsg := []byte(fmt.Sprintf(`{"type":"welcome","id":%d}`, oficialId))
	c.Send <- welcomeMsg

	go c.writePump()
	c.readPump(ig) // Loop de leitura — recebe comandos manuais

	globalClients.RemoveClient(oficialId)
	fmt.Printf("🖥️  Cliente %d desconectado.\n", oficialId)
}
