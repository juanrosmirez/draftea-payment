#!/bin/bash

# Payment System Setup Script
# This script sets up the development environment for the payment system

set -e

echo "🚀 Setting up Payment System Development Environment..."

# Check if required tools are installed
check_tool() {
    if ! command -v $1 &> /dev/null; then
        echo "❌ $1 is not installed. Please install it first."
        exit 1
    fi
}

echo "📋 Checking required tools..."
check_tool "go"
check_tool "docker"
check_tool "docker-compose"

# Create .env file from example if it doesn't exist
if [ ! -f .env ]; then
    echo "📝 Creating .env file from example..."
    cp .env.example .env
    echo "✅ .env file created. Please update it with your configuration."
else
    echo "✅ .env file already exists."
fi

# Build the application
echo "🔨 Building the application..."
go mod tidy
go build -o bin/payment-system ./cmd/main.go

# Start infrastructure services
echo "🐳 Starting infrastructure services..."
docker-compose up -d postgres redis kafka zookeeper

# Wait for services to be ready
echo "⏳ Waiting for services to be ready..."
sleep 10

# Run database migrations
echo "📊 Running database migrations..."
export $(cat .env | xargs)
go run migrations/migrate.go

echo "✅ Setup completed successfully!"
echo ""
echo "🎯 Next steps:"
echo "1. Update .env file with your configuration"
echo "2. Start the application: make run"
echo "3. Check API documentation: http://localhost:8080/docs"
echo "4. Monitor metrics: http://localhost:9091/metrics"
echo ""
echo "🔧 Useful commands:"
echo "- make test: Run tests"
echo "- make build: Build the application"
echo "- make docker-build: Build Docker image"
echo "- make clean: Clean build artifacts"
