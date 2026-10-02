#!/bin/bash

# Start the Timetable API server
echo "Starting Timetable API server..."

# Check if .env file exists
if [ ! -f .env ]; then
    echo "Warning: .env file not found. Using default configuration."
    echo "Create a .env file with the following variables:"
    echo "API_PORT=8080"
    echo "KOTLIN_SERVICE_URL=http://localhost:8082"
    echo ""
fi

# Run the API server
go run cmd/api/main.go
