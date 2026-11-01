# Build the notification service with Maven and ship the fat jar on a
# distroless Java 21 image. Render's free plan has 512MB, so the heap is sized
# from the container limit rather than the JVM default.
FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /src
COPY services/notification/pom.xml ./
RUN mvn -B -q dependency:go-offline
COPY services/notification/src ./src
RUN mvn -B -q package -DskipTests

FROM gcr.io/distroless/java21-debian12:nonroot
COPY --from=build /src/target/notification.jar /app/notification.jar
ENV JAVA_TOOL_OPTIONS="-XX:MaxRAMPercentage=75 -XX:+ExitOnOutOfMemoryError"
EXPOSE 8088
USER nonroot:nonroot
ENTRYPOINT ["java", "-jar", "/app/notification.jar"]
