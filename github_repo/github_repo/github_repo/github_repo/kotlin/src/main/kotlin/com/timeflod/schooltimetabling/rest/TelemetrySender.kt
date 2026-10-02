package com.timeflod.schooltimetabling.rest

import com.fasterxml.jackson.databind.ObjectMapper
import com.timeflod.schooltimetabling.domain.Timetable
import org.slf4j.LoggerFactory
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration

object TelemetrySender {
    private val httpClient = HttpClient.newBuilder()
        .connectTimeout(Duration.ofSeconds(2))
        .build()
    private val objectMapper = ObjectMapper()

    private val lastCombinations = java.util.concurrent.ConcurrentHashMap<String, Long>()
    private val lastTime = java.util.concurrent.ConcurrentHashMap<String, Long>()
    private val lastSpeed = java.util.concurrent.ConcurrentHashMap<String, Long>()

    fun sendTelemetry(jobId: String, solution: Timetable?, status: String, errorMsg: String? = null) {
        val ORANGE = "\u001B[38;5;208m"
        val GRAY = "\u001B[38;5;240m"
        val RESET = "\u001B[0m"
        val COLOR = if (status == "STATS_UPDATE") GRAY else ORANGE
        
        try {
            println("$COLOR[TELEMETRY] 1. Iniciando preparación de datos SQS para $jobId (Status: $status)$RESET")
            
            val scoreStr = solution?.score?.toString() ?: ""
            var hardScore = 0
            var softScore = 0
            if (solution?.score != null) {
                hardScore = solution.score!!.hardScore
                softScore = solution.score!!.softScore
            }

            println("$COLOR[TELEMETRY] 2. Score parseado: $scoreStr$RESET")

            val cambios = solution?.lessons?.map { lesson ->
                mapOf(
                    "lesson_id" to (lesson.id ?: 0),
                    "room_id" to lesson.room?.id,
                    "timeslot_id" to lesson.timeslot?.id,
                    "teacher_id" to lesson.teacher?.id
                )
            } ?: emptyList()
            
            val unassignedCount = solution?.lessons?.count { it.timeslot == null || it.room == null || it.teacher == null } ?: 0
            val phase = if (unassignedCount > 0) "CONSTRUYENDO_BASE" else "OPTIMIZANDO"
            
            val now = System.currentTimeMillis()
            val currentCombinations = com.timeflod.schooltimetabling.solver.SolverMetrics.combinationsEvaluated.get()
            
            val prevTime = lastTime[jobId]
            val prevCombs = lastCombinations[jobId]
            
            val speed: Long
            if (prevTime == null || prevCombs == null) {
                lastTime[jobId] = now
                lastCombinations[jobId] = currentCombinations
                speed = 0L
            } else {
                val elapsedSecs = (now - prevTime) / 1000.0
                if (elapsedSecs >= 0.5) {
                    speed = ((currentCombinations - prevCombs) / elapsedSecs).toLong()
                    lastTime[jobId] = now
                    lastCombinations[jobId] = currentCombinations
                    lastSpeed[jobId] = speed
                } else {
                    speed = lastSpeed[jobId] ?: 0L
                }
            }

            println("$COLOR[TELEMETRY] 3. Cambios extraídos: ${cambios.size}$RESET")

            val payload = mutableMapOf(
                "task_id" to jobId,
                "score" to scoreStr,
                "time_millis" to now,
                "hard_score" to hardScore,
                "soft_score" to softScore,
                "status" to status,
                "phase" to phase,
                "unassigned_lessons" to unassignedCount,
                "combinations_evaluated" to currentCombinations,
                "evaluations_per_second" to speed,
                "cambios" to cambios
            )
            
            if (errorMsg != null) {
                payload["error_msg"] = errorMsg
            }

            val jsonBody = objectMapper.writeValueAsString(payload)
            println("$COLOR[TELEMETRY] 4. JSON generado correctamente. Enviando a http://localhost:8081/queue/send ...$RESET")

            val request = HttpRequest.newBuilder()
                .uri(URI.create("http://localhost:8081/queue/send"))
                .header("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(jsonBody))
                .build()

            httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString())
                .whenComplete { response, throwable ->
                    if (throwable != null) {
                        println("\u001B[31m[TELEMETRY-ERROR] ❌ Error enviando a SQS asíncronamente: ${throwable.message}$RESET")
                        throwable.printStackTrace()
                    } else {
                        println("$COLOR[TELEMETRY] ✅ Respuesta SQS Mock: ${response.statusCode()} ${response.body()}$RESET")
                    }
                }

            println("$COLOR[TELEMETRY] 5. Petición HTTP asíncrona lanzada.$RESET")

        } catch (e: Exception) {
            println("\u001B[31m[TELEMETRY-FATAL] ❌ Fallo crítico en sendTelemetry: ${e.message}$RESET")
            e.printStackTrace()
        }
    }
}
