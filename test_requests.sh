#!/bin/bash

# Skrypt do wykonania zadanej ilości zapytań do localhost:8009/test

if [ -z "$1" ]; then
    echo "Użycie: $0 <liczba_zapytań>"
    exit 1
fi

COUNT=$1
URL="http://localhost:8009/test200"


echo "Wykonuję $COUNT zapytań do $URL..."

for ((i=1; i<=COUNT; i++)); do
    echo "Zapytanie $i/$COUNT"
    echo "$URL?q=$i"
    curl -s -o /dev/null -w "Status: %{http_code}, Czas: %{time_total}s\n" "$URL?q=$i"
done



echo "Zakończono!"
