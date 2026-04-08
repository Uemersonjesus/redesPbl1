
1. Visão Geral do Sistema

O RedesPBL12026 é um sistema distribuído de monitoramento de sensores IoT com controle automático e manual de atuadores. A arquitetura é composta por quatro componentes independentes que se comunicam em rede:

Integrador — hub central que gerencia sensores, atuadores e clientes
Sensor de Temperatura — envia leituras simuladas via UDP
Sensor de Umidade — envia leituras simuladas via UDP
Atuador — recebe comandos do integrador via WebSocket
Cliente — monitora sensores e envia comandos manuais via WebSocket

Todos os componentes exceto o Integrador podem ser executados em qualquer computador da mesma rede local — basta apontar para o IP da máquina que hospeda o Integrador,E isso é feito na hora de executar o arquivo executavel  colocando nomedoarquivo  ip_do_integrador.Lembrando que depedendo do sistema operacional a forma de executar o arquivo .exe  , sensores , clienets e atuadores muda. 


2. Estrutura de Diretórios
RedesPBL12026/
├── integrador/
│   ├── main.go
│   ├── integrador_controller.go
│   ├── sensor_manager.go
│   ├── atuadores_manager.go
│   ├── client_manager.go
│   ├── udpManageConections.go
│   ├── websocketConfiguracao_cliente.go
│   ├── web_socket_configuracao_atuador.go
│   ├── Dockerfile
│   └── docker-compose.yaml
├── sensor_temperatura/
│   ├── main.go
│   └── Dockerfile
├── sensor_umidade/
│   ├── main.go
│   
├── atuador/
│   ├── main.go
│   
└── cliente/
    ├── main.go
    
3. Arquitetura e Comunicação
Cada componente usa um protocolo diferente conforme sua função:

Componente → Integrador
Protocolo / Porta
Sensor Temperatura
UDP  porta 5000
Sensor Umidade
UDP  porta 5000
Atuador
WebSocket (TCP)  porta 9090
Cliente
WebSocket (TCP)  porta 8080


O Integrador é o único componente que precisa ter suas portas acessíveis na rede. Os sensores, atuadores e clientes são sempre os iniciadores da conexão — eles alcançam o Integrador, não o contrário.


4. Integrador
O Integrador é o componente de maior complexidade do sistema. Ele recebe dados de sensores via UDP, gerencia vínculos sensor-atuador via matchmaking um para um automático, transmite atualizações para clientes e executa controle automático por limites configuráveis.

4.1 Arquivos e responsabilidades
Arquivo
Responsabilidade
main.go
Inicializa mapas, Integrador, socket UDP e servidores HTTP nas portas 5000, 8080 e 9090
integrador_controller.go
Núcleo lógico: MatchTable O(1), broadcast, AutoControl, OnClientCommand
sensor_manager.go
Struct diagramaUdpInformation e mapOfSensors com sync.Mutex
atuadores_manager.go
Struct Actuator, MapOfActuators, writePump e readPump do atuador
client_manager.go
Struct Client, MapOfClients, writePump e readPump do cliente
udpManageConections.go
Loop de recepção UDP — desserializa JSON e delega ao Integrador
websocketConfiguracao_cliente.go
Handshake WebSocket RFC 6455 para clientes via http.Hijacker
web_socket_configuracao_atuador.go
Handshake WebSocket RFC 6455 para atuadores via http.Hijacker


4.2 Matchmaking sensor ↔ atuador
O sistema mantém um vínculo 1:1 entre sensores e atuadores gerenciado pela estrutura MatchTable. Toda operação é O(1) — sem iteração sobre coleções:

freeSensors []uint16 — fila de sensores aguardando atuador
freeActuators []uint16 — fila de atuadores aguardando sensor
bySensor map[uint16]uint16 — lookup direto sensorID → atuadorID
byActuator map[uint16]uint16 — lookup direto atuadorID → sensorID

Quando um novo sensor chega, RegisterSensor consulta o topo de freeActuators em O(1). Quando um atuador conecta, RegisterActuator consulta o topo de freeSensors em O(1). Ao desconectar, o componente que ficou sozinho volta automaticamente para a fila correspondente.

4.3 Controle automático por limites
A cada pacote UDP recebido, AutoControl avalia o valor e envia comando ao atuador vinculado se necessário:

Constante
Valor e Ação
TemperaturaMax
40 °C → comando on  (ligar resfriamento)
TemperaturaMin
15 °C → comando off
UmidadeMax
80 %  → comando on  (ligar desumidificador)
UmidadeMin
20 %  → comando off


4.4 Dockerfile do Integrador
Build multi-stage sem dependências externas — apenas a stdlib do Go:

Stage 1 (golang:1.22-alpine): copia os .go, inicializa módulo com go mod init e compila com CGO_ENABLED=0 para binário estático
Stage 2 (scratch): imagem vazia absoluta — copia apenas o binário. Resultado: ~5 MB contra ~300 MB de uma imagem golang padrão
Sem bibliotecas externas para instalar — zero dependências além da stdlib

4.5 Pacotes utilizados
Pacote stdlib
Uso
net
Sockets UDP (UDPConn) e TCP (Conn) para WebSocket
net/http
Servidor HTTP para handshake WebSocket via http.Hijacker
encoding/json
Serialização e desserialização de todas as mensagens
crypto/sha1
Cálculo do Sec-WebSocket-Accept (RFC 6455)
encoding/base64
Codificação da chave de aceite do WebSocket
sync
sync.Mutex e sync.RWMutex nos mapas concorrentes
fmt / log
Logs de status e erros fatais na inicialização
bufio
Buffer de leitura/escrita nos sockets WebSocket



5. Sensores
Existem dois sensores independentes: sensor_temperatura e sensor_umidade. Cada um é um programa Go autônomo que simula leituras físicas e as transmite ao Integrador via UDP. A lógica de ambos é idêntica — diferem apenas no tipo e na faixa de valores. 

5.1 Funcionamento
Ao iniciar, gera um ID único via crypto/rand (uint16 entre 1 e 65535) — sem configuração manual
Recebe o IP do Integrador como argumento de linha de comando
Abre um socket UDP e envia uma leitura a cada 1 segundo
O valor oscila continuamente entre mínimo e máximo, subindo e descendo de 1 em 1 (sawtooth simulado)
Cada pacote deveria incluir um CRC 8 bits , no entanto a implementação disso será feita aposteriori e foi implementado o checksum que é mais simples no lugar apenas para efetivamente ultilizar o espaço em bits no protocolo criado.

5.2 Protocolo UDP — formato do pacote
Cada pacote é um JSON serializado enviado como payload UDP:
                                                
{ "id": 42301, "tipo": 1, "information": 35, "crc": 200 }

Campo
Descrição
id
uint16 — gerado por crypto/rand, único por execução
tipo
uint8  — 1 = temperatura, 2 = umidade
information
uint8  — valor da leitura (°C ou %)
crc
uint8  — XOR simples para verificação de integridade


5.3 Faixas de simulação
Sensor
Faixa / Intervalo
Temperatura
0 °C a 50 °C, 1 envio/segundo
Umidade
0 % a 100 %, 1 envio/segundo


5.4 Geração de ID automática
O ID do sensor é gerado via crypto/rand a cada inicialização do processo — sem nenhuma variável de ambiente ou argumento necessário. Isso garante:

Unicidade entre múltiplos containers no mesmo host (cada um gera seu próprio ID)
Unicidade entre hosts diferentes na mesma rede
Sem necessidade de coordenação manual de IDs ao escalar
No entanto há um limite ,dos 65 mil sensores possíveis devido ao paradoxo do aniversariante colisões começam a se tornar comum em torno de 4000 sensores matematicamente 
falando. 

5.5 Pacotes utilizados
Pacote stdlib
Uso
crypto/rand
Geração criptograficamente segura do ID único
math/big
Auxiliar de crypto/rand para gerar uint16
net
Socket UDP para envio dos pacotes
encoding/json
Serialização do pacote de dados
encoding/binary
Conversão do ID uint16 para bytes no cálculo do CRC
time
Ticker de 1 segundo entre envios
os / fmt / log
Argumentos CLI e saída de status

6. Atuador
O atuador é o componente que representa um dispositivo físico controlável. Ele conecta ao Integrador via WebSocket, aguarda ser vinculado a um sensor e recebe comandos de ligar/desligar — tanto automáticos (disparo por limite) quanto manuais (enviados por um cliente).

6.1 Funcionamento
Recebe o IP do Integrador como argumento de linha de comando
Estabelece conexão TCP e realiza o handshake WebSocket RFC 6455 manualmente (sem bibliotecas externas)
Gera a Sec-WebSocket-Key com crypto/rand e valida o Sec-WebSocket-Accept retornado
Após o handshake, aguarda a mensagem de registro do Integrador (actuator_ack com seu ID atribuído)
Entra em loop de leitura de frames WebSocket — cada frame é um comando JSON
Ao receber um comando, imprime no terminal simulando a ação física (LIGANDO / DESLIGANDO)

6.2 Mensagens recebidas do Integrador
Boas-vindas (imediatamente após o handshake):
{ "type": "actuator_ack", "id": 3 }

Comando de controle (automático ou manual):
{ "type": "command", "sensor_id": 42301, "command": "on", "triggered_by": "auto" }

Campo
Descrição
type
"command" para comandos de controle
sensor_id
ID do sensor que disparou o comando
command
"on" ou "off"
triggered_by
"auto" (limite automático) ou "client" (comando manual)


6.3 Pacotes utilizados
Pacote stdlib
Uso
net
Conexão TCP com o Integrador
bufio
Buffer de leitura dos frames WebSocket
crypto/rand
Geração da Sec-WebSocket-Key
crypto/sha1
Cálculo do Sec-WebSocket-Accept esperado
encoding/base64
Codificação da chave WS
encoding/json
Desserialização dos comandos recebidos
os / fmt / log
Argumentos CLI e saída de status



7. Cliente
O cliente é a interface de monitoramento e controle do sistema. Ele conecta ao Integrador via WebSocket, recebe atualizações em tempo real de todos os sensores e permite ao usuário enviar comandos manuais para atuadores específicos.

7.1 Funcionamento
Recebe o IP do Integrador como argumento de linha de comando
Conecta via WebSocket na porta 8080 do Integrador
Uma goroutine roda em background recebendo atualizações de sensores continuamente
O estado local (LocalStore) é atualizado a cada broadcast do Integrador com sync.RWMutex
A tela principal lista todos os sensores conhecidos com ID, tipo e status do atuador
O usuário seleciona um sensor pelo ID para entrar no modo de monitoramento individual
No modo de monitoramento, o valor do sensor atualiza na mesma linha do terminal em tempo real
O usuário pode digitar on, off ou sair para enviar comandos ou voltar ao menu

7.2 Mensagem enviada ao Integrador (comando manual)
{ "type": "control", "sensor_id": 42301, "command": "on" }

Campo
Descrição
type
sempre "control"
sensor_id
ID do sensor cujo atuador deve ser acionado
command
"on" ou "off"


7.3 Mensagem recebida do Integrador (broadcast)
{ "type": "sensor_update", "sensor_id": 42301, "actuator_id": 2,
  "tipo": 1, "information": 35, "unit": "°C" }

Campo
Descrição
sensor_id
ID do sensor atualizado
actuator_id
ID do atuador vinculado (0 = sem atuador)
tipo
1 = temperatura, 2 = umidade
information
Valor atual da leitura
unit
Unidade: °C ou %


7.4 Interface de uso
Tela principal — lista de sensores disponíveis:

Sensores disponíveis:
┌─────────┬──────────────┬──────────────────────┐
│   ID    │     Tipo     │      Atuador         │
├─────────┼──────────────┼──────────────────────┤
│  42301  │  temperatura │  vinculado (ID=2)    │
│  17843  │  umidade     │  sem atuador         │
└─────────┴──────────────┴──────────────────────┘
Digite o ID do sensor para monitorar: 

Modo de monitoramento individual (atualiza em tempo real):

Monitorando sensor 42301 (temperatura)
Atuador vinculado: ID=2
Comandos: on | off | sair
─────────────────────────────────────────────
 ID=42301   35°C  atuador=2
> 

7.5 Pacotes utilizados
Pacote stdlib
Uso
net
Conexão TCP com o Integrador
bufio
Buffer de leitura/escrita e leitura do stdin
crypto/rand
Geração da Sec-WebSocket-Key
encoding/base64
Codificação da chave WS
encoding/json
Serialização de comandos e desserialização de broadcasts
sync
sync.RWMutex no LocalStore para concorrência segura
os / fmt / strings / strconv
CLI, formatação e parsing de entrada do usuário



8. Como Executar
8.1 Pré-requisitos
Docker Engine 24+ ou Docker Desktop com Docker Compose v2
Portas 5000/UDP, 8080/TCP e 9090/TCP livres na máquina do Integrador
Todos os computadores na mesma rede local (WiFi doméstica, LAN, etc.)

8.2 Passo 1 — Subir o Integrador
Na pasta integrador/:

docker compose up --build 

Descobrir o IP da máquina do Integrador (necessário para os outros componentes):
ipconfig          # Windows
ip addr           # Linux

8.3 Passo 2 — Subir os sensores
Em qualquer computador da rede, na pasta do sensor:


Sem Docker (binário compilado):
go build -o sensor_temperatura .
./sensor_temperatura 192.168.1.X


Para o sensor de umidade, o mesmo processo na pasta sensor_umidade/:
./sensor_umidade 192.168.1.X

ou use os executaveis que já foram compilados. 

8.4 Passo 3 — Subir os atuadores
Em qualquer computador da rede, na pasta atuador/:

go build -o atuador .
./atuador 192.168.1.X

Cada atuador que subir será automaticamente vinculado ao primeiro sensor livre disponível no Integrador. Se não houver sensor livre, o atuador aguarda na fila até um sensor se registrar.

8.5 Passo 4 — Conectar o cliente
Em qualquer computador da rede, na pasta cliente/:

go build -o cliente .
./cliente 192.168.1.X

Múltiplos clientes podem conectar simultaneamente — todos recebem o mesmo broadcast de atualizações.

8.6 Escalando — exemplo com 10 computadores
Supondo Integrador em 192.168.1.10, todos os outros computadores na mesma rede WiFi podem rodar qualquer combinação de sensores, atuadores e clientes apontando para o mesmo endereço. — o Integrador gerencia todos dinamicamente.


9. Limitações Conhecidas
Detecção de sensor offline
UDP não tem conexão — o Integrador não detecta quando um sensor para de enviar. O sensor permanece registrado mesmo após parar. A solução planejada é um watchdog com timeout: se nenhum pacote chegar em N segundos, o sensor é removido e o atuador vinculado volta para a fila.

Reuso de IDs de clientes e atuadores
IDs de clientes e atuadores são incrementais e não são reaproveitados após desconexão. O nextID continua crescendo. Para sessões muito longas com muitas reconexões isso pode eventualmente esgotar o espaço uint16. A solução é manter uma fila de IDs liberados, análoga ao freeSensors da MatchTable.

Frames WebSocket longos
As implementações de writePump usam payload length de 1 byte (máximo 125 bytes por frame). Mensagens maiores requerem implementação do extended payload length (opcodes 126/127 do RFC 6455).


Para executar os testes, navegue até a pasta integrador/ e rode:

    go test -v ./...                        (todos os testes)
    go test -v -race ./...                  (com detector de race condition)
    go test -v -run TestMatch ./...         (apenas testes de matchmaking)
    go test -v -run TestAutoControl ./...   (apenas testes de controle automático)

A flag -race é recomendada pois ativa o detector de condições de corrida do Go,
verificando se o acesso concorrente aos mapas e filas está corretamente sincronizado.


# Sobe 3 sensores de cada tipo, 3 atuadores e 2 clientes  no windowns.
subir.bat 192.168.1.10 3 3 2  

# Derruba tudo  no windowns.
derrubar.bat

# Sobe 3 sensores de cada tipo, 3 atuadores e 2 clientes  no linux.
subir_linux.sh 192.168.1.10 3 3 2  

# Derruba tudo  no linux.
derrubar_linux.sh

sendo necessário executar no mesmo diretório que se encontra os  scripts de execução

Esta parte foi removida porque a ferramenta de visualizar as informações internas do go não funcionou após algumas tentativas.

Para o teste de conocrrência real foi ultilizado o import no main do _ "net/http/pprof" e "runtime" e logo após a definição da  função main   go func() {
        log.Println("Análise pprof ativa em http://0.0.0.0:6060/debug/pprof/")
        if err := http.ListenAndServe("0.0.0.0:6060", nil); err != nil {
            log.Fatalf("Erro ao iniciar pprof: %v", err)
        }
    }()

    runtime .SetMutexProfileFraction(5) 
    runtime.SetBlockProfileRate(1)

no docker compose precisa adicionar - "6060:6060"
executando o comando abaixo conseguimos analisar os metricas reais do integrador.

go tool pprof -http=:8081 http://[IP_DO_INTEGRADOR]:6060/debug/pprof/profile?seconds=30  para analisar consumo de cpu 

go tool pprof -http=:8082 http://[IP_DO_INTEGRADOR]:6060/debug/pprof/heap  para analisar memory leaks

go tool pprof -http=:8083 http://[IP_DO_INTEGRADOR]:6060/debug/pprof/allocs toda a memória que já foi usada desde o começo.

no mesmo computador que está o integrador.

Para rodar as instâncias de testes 

