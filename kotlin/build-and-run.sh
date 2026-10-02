#!/bin/bash

# School Timetabling Kotlin - Build and Run Script

set -e

echo "🏗️  Building School Timetabling Kotlin Project..."

# Check if Maven is installed
if ! command -v mvn &> /dev/null; then
    echo "❌ Maven is not installed. Please install Maven first."
    exit 1
fi

# Check if Java 17+ is available
JAVA_VERSION=$(java -version 2>&1 | head -n 1 | cut -d'"' -f2 | cut -d'.' -f1)
if [ "$JAVA_VERSION" -lt 17 ]; then
    echo "❌ Java 17 or higher is required. Current version: $JAVA_VERSION"
    exit 1
fi

echo "✅ Java version: $(java -version 2>&1 | head -n 1)"

# Clean and compile
echo "📦 Cleaning and compiling..."
mvn clean compile

# Run tests
echo "🧪 Running tests..."
mvn test

echo "✅ Build completed successfully!"

mvn quarkus:dev
exit
# Ask user if they want to run the application
read -p "🚀 Do you want to run the application in development mode? (y/n): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "🌐 Starting application on http://localhost:8080"
    echo "📚 API Documentation will be available at:"
    echo "   - Swagger UI: http://localhost:8080/swagger-ui"
    echo "   - OpenAPI: http://localhost:8080/openapi"
    echo ""
    echo "Press Ctrl+C to stop the application"
    echo ""
    
    # Run in development mode
    mvn quarkus:dev
else
    echo "💡 To run the application later, use: mvn quarkus:dev"
    echo "💡 To build a JAR file, use: mvn package"
fi

