FROM maven:3.9.6-eclipse-temurin-17-alpine AS build

WORKDIR /app
COPY pom.xml .
# Descargar dependencias para acelerar futuras compilaciones
RUN mvn dependency:go-offline

COPY src ./src
RUN mvn clean package -DskipTests

FROM eclipse-temurin:17-jre-alpine
WORKDIR /app

# Copiar el fast-jar generado por Quarkus
COPY --from=build /app/target/quarkus-app/lib/ /app/lib/
COPY --from=build /app/target/quarkus-app/*.jar /app/
COPY --from=build /app/target/quarkus-app/app/ /app/app/
COPY --from=build /app/target/quarkus-app/quarkus/ /app/quarkus/

EXPOSE 8080
CMD ["java", "-jar", "/app/quarkus-run.jar"]
