#!/bin/bash

echo "Derrubando todos os componentes Go..."

pkill -f sensor_linux
pkill -f atuador_linux
pkill -f cliente_linux


echo "Todos os processos foram encerrados."
