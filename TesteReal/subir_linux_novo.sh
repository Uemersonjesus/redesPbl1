#!/bin/bash

IP=$1
SENSORES=${2:-2}
ATUADORES=${3:-2}
CLIENTES=${4:-1}

if [ -z "$IP" ]; then
    echo "Uso: ./subir.sh <IP_DO_INTEGRADOR> <Qtd_Sensores> <Qtd_Atuadores> <Qtd_Clientes>"
    exit 1
fi

echo "Iniciando componentes para o integrador em $IP..."

# Sensores de Temperatura
for i in $(seq 1 $SENSORES); do
    echo "Iniciando Sensor Temp $i..."
    ./sensor_temperatura/sensor_linux "$IP" > log_temp_$i.txt 2>&1 &
done

# Sensores de Umidade
for i in $(seq 1 $SENSORES); do
    echo "Iniciando Sensor Umid $i..."
    ./sensor_umidade/sensor_linux "$IP" > log_umid_$i.txt 2>&1 &
done

# Atuadores
for i in $(seq 1 $ATUADORES); do
    echo "Iniciando Atuador $i..."
    ./atuador/atuador_linux "$IP" > log_atuador_$i.txt 2>&1 &
done

# Clientes
for i in $(seq 1 $CLIENTES); do
    echo "Iniciando Cliente $i..."
    ./cliente/cliente_linux "$IP" > log_cliente_$i.txt 2>&1 &
done

echo "Todos os processos foram iniciados em segundo plano."

echo "Para ver os logs, use: tail -f log_atuador_1.txt"
