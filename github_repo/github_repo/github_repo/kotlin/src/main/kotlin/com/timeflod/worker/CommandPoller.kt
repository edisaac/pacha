package com.timeflod.worker

import com.fasterxml.jackson.databind.JsonNode
import com.fasterxml.jackson.databind.ObjectMapper
import io.quarkus.runtime.Startup
import jakarta.annotation.PostConstruct
import jakarta.enterprise.context.ApplicationScoped
import jakarta.inject.Inject
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration
import kotlin.concurrent.thread

@Startup
@ApplicationScoped
class CommandPoller {

    @Inject
    lateinit var solverWorker: SolverWorker

    private val httpClient = HttpClient.newBuilder()
        .connectTimeout(Duration.ofSeconds(5))
        .build()

    private val objectMapper = ObjectMapper()

    private val PURPLE = "\u001B[38;5;135m"
    private val RESET = "\u001B[0m"

    @PostConstruct
    fun init() {
        println("$PURPLE[COMMAND POLLER] Iniciando hilo de poling SQS para recibir comandos...$RESET")
        thread(isDaemon = true, name = "CommandPoller-Thread") {
            pollLoop()
        }
    }

    private fun pollLoop() {
        while (true) {
            try {
                val request = HttpRequest.newBuilder()
                    .uri(URI.create("http://localhost:8081/queue/receive?queue_name=commands"))
                    .timeout(Duration.ofSeconds(3))
                    .GET()
                    .build()

                val response = httpClient.send(request, HttpResponse.BodyHandlers.ofString())

                if (response.statusCode() == 200) {
                    val body = response.body()
                    if (body.isNotBlank()) {
                        val rootNode = objectMapper.readTree(body)
                        val messagesNode = rootNode.get("Messages")
                        
                        if (messagesNode != null && messagesNode.isArray && messagesNode.size() > 0) {
                            val msgNode = messagesNode.get(0)
                            val receiptHandle = msgNode.get("receipt_handle").asText()
                            val bodyStr = msgNode.get("body").asText()
                            
                            processMessage(bodyStr)
                            deleteMessage(receiptHandle)
                        }
                    }
                }
            } catch (e: InterruptedException) {
                println("$PURPLE[COMMAND POLLER] Poller interrumpido.$RESET")
                break
            } catch (e: Exception) {
                // Ignore connection errors if Go is down, just sleep a bit longer
                Thread.sleep(2000)
            }
            Thread.sleep(500)
        }
    }

    private fun processMessage(body: String) {
        try {
            val payload: JsonNode = objectMapper.readTree(body)
            val command = payload.get("command")?.asText()
            val taskId = payload.get("task_id")?.asText()
            
            if (command != null && taskId != null) {
                println("$PURPLE[COMMAND POLLER] Recibido comando $command para tarea $taskId$RESET")
                when (command.uppercase()) {
                    "SOLVE" -> {
                        thread { solverWorker.processTask(taskId) }
                    }
                    "STOP" -> {
                        solverWorker.stopTask(taskId)
                    }
                    else -> {
                        println("$PURPLE[COMMAND POLLER] Comando desconocido: $command$RESET")
                    }
                }
            }
        } catch (e: Exception) {
            println("$PURPLE[COMMAND POLLER] Error procesando mensaje: ${e.message}$RESET")
        }
    }

    private fun deleteMessage(receiptHandle: String) {
        try {
            val req = HttpRequest.newBuilder()
                .uri(URI.create("http://localhost:8081/queue/delete?receipt_handle=$receiptHandle&queue_name=commands"))
                .DELETE()
                .build()
            httpClient.send(req, HttpResponse.BodyHandlers.discarding())
        } catch (e: Exception) {
            println("$PURPLE[COMMAND POLLER] Error borrando mensaje de la cola: ${e.message}$RESET")
        }
    }
}
