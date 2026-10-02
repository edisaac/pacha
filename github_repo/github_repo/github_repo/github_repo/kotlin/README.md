# School Timetabling - Kotlin Version

This is the Kotlin version of the school timetabling application, migrated from the Python version. It uses Timefold Solver with Quarkus framework and AWS DynamoDB for data persistence.

## Features

- **Timefold Solver**: Advanced constraint optimization for timetabling
- **Quarkus Framework**: Fast, lightweight Java framework
- **AWS DynamoDB**: Cloud-based data storage
- **REST API**: Full RESTful API with OpenAPI documentation
- **CORS Support**: Cross-origin resource sharing enabled
- **Comprehensive Constraints**: Hard and soft constraints for realistic timetabling

## Prerequisites

- Java 17 or higher
- Maven 3.6 or higher
- AWS credentials configured (for DynamoDB access)

## Building the Project

```bash
# Navigate to the Kotlin project directory
cd horarios/kotlin

# Build the project
mvn clean compile

# Run tests
mvn test

# Build the executable JAR
mvn package
```

## Running the Application

### Development Mode

```bash
# Run in development mode with hot reload
mvn quarkus:dev
```

### Production Mode

```bash
# Build the native executable (requires GraalVM)
mvn package -Pnative

# Run the JAR file
java -jar target/quarkus-app/quarkus-run.jar
```

## API Endpoints

The application provides the following REST endpoints:

- `GET /timetables/demo-data` - Get demo data from DynamoDB
- `POST /timetables` - Submit a timetable for solving
- `GET /timetables/{jobId}` - Get solution for a job
- `GET /timetables/{jobId}/status` - Get status of a solving job
- `PUT /timetables/analyze` - Analyze a timetable's score
- `POST /timetables/test-initial` - Test initial solution without solving
- `DELETE /timetables/{jobId}` - Terminate solving for a job

## OpenAPI Documentation

Once the application is running, you can access the API documentation at:

- Swagger UI: http://localhost:8080/swagger-ui
- OpenAPI JSON: http://localhost:8080/openapi

## Configuration

The application configuration is in `src/main/resources/application.properties`:

- **Port**: 8080 (default)
- **Timefold Solver**: 60-second termination limit
- **AWS Region**: us-east-1 (configurable)
- **Logging**: DEBUG level for Timefold and application logs

## Domain Model

The application uses the following domain entities:

- **Subject**: Academic subjects with ID and name
- **Teacher**: Teachers with qualifications (subjects they can teach)
- **Timeslot**: Time periods with day, start time, and end time
- **Room**: Physical rooms for lessons
- **Lesson**: Planning entity that gets assigned teacher, timeslot, and room
- **Timetable**: Planning solution containing all entities

## Constraints

### Hard Constraints
- **Room Conflict**: No two lessons in the same room at the same time
- **Teacher Conflict**: No teacher can teach two lessons simultaneously
- **Student Group Conflict**: No student group can attend two lessons at the same time
- **Teacher Qualification**: Teachers must be qualified for the subjects they teach

### Soft Constraints
- **Teacher Room Stability**: Teachers prefer to teach in the same room
- **Teacher Time Efficiency**: Teachers prefer sequential lessons
- **Student Group Subject Variety**: Students prefer variety in sequential lessons
- **Student Time Efficiency**: Students prefer sequential lessons
- **Weekend Penalty**: Penalize lessons on weekends (especially Sundays)

## Data Source

The application loads data from AWS DynamoDB table `SchoolTimetable-dev` with the following structure:

- **PK**: `TIMETABLE#{problemId}`
- **SK**: Entity type with ID (e.g., `SUBJECT#1`, `TEACHER#1#SUBJECT#101`)

## Migration from Python

This Kotlin version maintains compatibility with the Python version:

- Same domain model structure
- Same DynamoDB data format
- Same REST API endpoints
- Enhanced constraints and performance

## Troubleshooting

### Common Issues

1. **AWS Credentials**: Ensure AWS credentials are properly configured
2. **DynamoDB Access**: Verify the table `SchoolTimetable-dev` exists and is accessible
3. **Memory**: Large datasets may require increased JVM heap size
4. **Port Conflicts**: Change the port in `application.properties` if 8080 is in use

### Logs

The application provides detailed logging:
- Timefold solver operations
- DynamoDB queries
- Constraint violations
- Performance metrics

## Development

### Adding New Constraints

1. Add constraint method to `TimeTableConstraintProvider.kt`
2. Register the constraint in `defineConstraints()`
3. Test with the analyze endpoint

### Modifying Domain Model

1. Update domain classes in `domain/` package
2. Ensure proper Timefold annotations
3. Update persistence layer if needed
4. Test data loading and serialization

## Performance

The Kotlin version offers improved performance over the Python version:

- **Native JVM execution**
- **Optimized Timefold solver**
- **Efficient AWS SDK v2**
- **Reduced memory footprint**

## License

This project is part of the Timeflod school timetabling system.

