package com.timeflod.worker

import com.timeflod.schooltimetabling.domain.DomainMapper
import com.timeflod.schooltimetabling.repository.TimetableRepository
import io.quarkus.runtime.QuarkusApplication
import io.quarkus.runtime.annotations.QuarkusMain
import jakarta.inject.Inject
import jakarta.ws.rs.Path
import org.jboss.logging.Logger
import kotlinx.coroutines.*
import kotlinx.coroutines.channels.Channel

@QuarkusMain
@Path("/")
class SolverWorker : QuarkusApplication {
    
    @Inject
    lateinit var solverService: com.timeflod.schooltimetabling.service.TimefoldSolverService

    @Inject
    lateinit var repository: TimetableRepository

    private val dbDir = (System.getenv("SHARED_DATA_DIR") ?: "../shared-data") + "/db"

    companion object {
        private val logger = Logger.getLogger(SolverWorker::class.java)
    }

    override fun run(vararg args: String?): Int {
        val taskId = System.getenv("TASK_ID")
        if (!taskId.isNullOrEmpty()) {
            processTask(taskId)
            return 0
        }
        io.quarkus.runtime.Quarkus.waitForExit()
        return 0
    }

    fun stopTask(taskId: String) {
        logger.info("[Kotlin] Recibido comando SQS para detener task: $taskId")
        try {
            solverService.solverManager.terminateEarly(taskId)
            logger.info("[Kotlin] terminateEarly enviado para task: $taskId")
        } catch (e: Exception) {
            logger.warn("[Kotlin] terminateEarly falló (puede que ya haya terminado): ${e.message}")
        }
    }

    fun processTask(taskId: String) {
        logger.info("[Kotlin] Iniciando Timefold para tarea $taskId")

        try {
            // Read the problem from SQLite via Repository
            val timetable = repository.readTimetableFromSqlite(taskId, dbDir)
            logger.info("[Kotlin] Timetable leído de SQLite. Iniciando solver.")

            DomainMapper.initializeProblemFacts(timetable)

            val currentBestSolution = java.util.concurrent.atomic.AtomicReference(timetable)
            val lastSentScoreRef = java.util.concurrent.atomic.AtomicReference<String?>(null)
            val firstOptimizandoSent = java.util.concurrent.atomic.AtomicBoolean(false)
            var lastNewBestSolutionSentTime = 0L
            val triggerChannel = Channel<Unit>(Channel.CONFLATED)

            val telemetryScope = CoroutineScope(Dispatchers.IO + SupervisorJob())
            val telemetryJob = telemetryScope.launch {
                try {
                    println("\u001B[38;5;240m[HEARTBEAT] Coroutine started for $taskId in SolverWorker\u001B[0m")
                    var retries = 0
                    while (solverService.solverManager.getSolverStatus(taskId) == ai.timefold.solver.core.api.solver.SolverStatus.NOT_SOLVING && retries < 15) {
                        delay(1000)
                        retries++
                    }
                    
                    while (solverService.solverManager.getSolverStatus(taskId) != ai.timefold.solver.core.api.solver.SolverStatus.NOT_SOLVING) {
                        val solution = currentBestSolution.get()
                        val currentScore = solution?.score?.toString()
                        val lastSentScore = lastSentScoreRef.get()
                        
                        val assigned = solution.lessons.count { it.room != null && it.timeslot != null }
                        
                        // Enviamos NEW_BEST_SOLUTION si el score cambió Y ya hay al menos una lección asignada (ignora el estado inicial vacío 0/N)
                        if (currentScore != null && currentScore != lastSentScore && assigned > 0) {
                            val timeSinceLastSent = System.currentTimeMillis() - lastNewBestSolutionSentTime
                            // Enviar solo si es la primera vez o pasaron más de 3 segundos
                            if (!firstOptimizandoSent.get() || timeSinceLastSent >= 3000) {
                                DomainMapper.inflateChildren(solution)
                                com.timeflod.schooltimetabling.rest.TelemetrySender.sendTelemetry(taskId, solution, "NEW_BEST_SOLUTION")
                                lastSentScoreRef.set(currentScore)
                                lastNewBestSolutionSentTime = System.currentTimeMillis()
                                firstOptimizandoSent.set(true)
                                
                                logger.info("[Kotlin] Throttled Progreso SQS (3s): $assigned/${solution.lessons.size} lecciones asignadas, score=$currentScore")
                            } else {
                                // Mejoró el score, pero enviamos solo un STATS_UPDATE para no saturar las tablas
                                DomainMapper.inflateChildren(solution!!)
                                com.timeflod.schooltimetabling.rest.TelemetrySender.sendTelemetry(taskId, solution, "STATS_UPDATE")
                            }
                        } else {
                            // The score hasn't changed (or it's the initial empty state) -> send as STATS_UPDATE to just update UI speed & progress
                            DomainMapper.inflateChildren(solution!!)
                            com.timeflod.schooltimetabling.rest.TelemetrySender.sendTelemetry(taskId, solution, "STATS_UPDATE")
                        }
                        
                        // Espera hasta 1000ms o hasta recibir el trigger inmediato
                        withTimeoutOrNull(1000) {
                            triggerChannel.receive()
                        }
                    }
                    println("\u001B[38;5;240m[HEARTBEAT] Coroutine exiting for $taskId\u001B[0m")
                } catch (e: Exception) {
                    if (e !is CancellationException) {
                        logger.error("Heartbeat coroutine error in SolverWorker", e)
                    }
                }
            }

            // Use solveBuilder so we get intermediate best solutions via the consumer
            val solverJob = solverService.solverManager.solveBuilder()
                .withProblemId(taskId)
                .withProblemFinder { _ -> timetable }
                .withBestSolutionConsumer { bestSolution ->
                    currentBestSolution.set(bestSolution)
                    
                    val assigned = bestSolution.lessons.count { it.timeslot != null && it.room != null }
                    // Si es la primera solución real (con al menos 1 asignación), despertamos la corrutina inmediatamente
                    if (assigned > 0 && !firstOptimizandoSent.get()) {
                        triggerChannel.trySend(Unit)
                    }
                }
                .run()

            // Wait for the solver to finish and get the final best solution
            val solution = solverJob.finalBestSolution
            telemetryJob.cancel()
            logger.info("[Kotlin] Solver finalizado. Score final: ${solution.score}")

            DomainMapper.inflateChildren(solution)
            
            // Enviar telemetría final al SQS (Go se encargará de persistir en SQLite)
            com.timeflod.schooltimetabling.rest.TelemetrySender.sendTelemetry(taskId, solution, "COMPLETED")
            logger.info("[Kotlin] Resultados finales enviados a SQS para $taskId")

        } catch (e: Exception) {
            logger.error("[Kotlin] Error en processTask para $taskId", e)
            com.timeflod.schooltimetabling.rest.TelemetrySender.sendTelemetry(taskId, null, "ERROR", e.message)
            if (System.getenv("APP_ENV") == "production") System.exit(1)
        }
    }
}
