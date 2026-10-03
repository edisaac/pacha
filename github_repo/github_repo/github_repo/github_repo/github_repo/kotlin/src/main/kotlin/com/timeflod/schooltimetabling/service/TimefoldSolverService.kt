package com.timeflod.schooltimetabling.service

import ai.timefold.solver.core.api.solver.SolverFactory
import ai.timefold.solver.core.api.solver.SolverManager
import ai.timefold.solver.core.config.score.director.ScoreDirectorFactoryConfig
import ai.timefold.solver.core.config.solver.EnvironmentMode
import ai.timefold.solver.core.config.solver.SolverConfig
import ai.timefold.solver.core.config.solver.termination.TerminationConfig
import com.timeflod.schooltimetabling.domain.Lesson
import com.timeflod.schooltimetabling.domain.Timetable
import com.timeflod.schooltimetabling.solver.TimeTableIncrementalScoreCalculator
import jakarta.annotation.PostConstruct
import jakarta.enterprise.context.ApplicationScoped
import org.eclipse.microprofile.config.inject.ConfigProperty
import org.slf4j.LoggerFactory
import java.time.Duration

@ApplicationScoped
class TimefoldSolverService {

    private val logger = LoggerFactory.getLogger(TimefoldSolverService::class.java)

    lateinit var solverManager: SolverManager<Timetable, String>

    @ConfigProperty(name = "quarkus.timefold.solver.termination.spent-limit", defaultValue = "120s")
    lateinit var spentLimit: String

    @ConfigProperty(name = "quarkus.timefold.solver.termination.unimproved-spent-limit", defaultValue = "60s")
    lateinit var unimprovedSpentLimit: String

    @ConfigProperty(name = "quarkus.timefold.solver.environment-mode", defaultValue = "NON_REPRODUCIBLE")
    lateinit var environmentMode: String

    @PostConstruct
    fun init() {
        logger.info("Inicializando TimefoldSolverService con TimeTableIncrementalScoreCalculator para máxima velocidad (Arquitectura Híbrida)...")

        val parsedSpentLimit = parseDuration(spentLimit)
        val parsedUnimprovedLimit = parseDuration(unimprovedSpentLimit)
        val envMode = try {
            EnvironmentMode.valueOf(environmentMode.uppercase())
        } catch (e: Exception) {
            EnvironmentMode.NON_REPRODUCIBLE
        }

        val solverConfig = SolverConfig()
            .withSolutionClass(Timetable::class.java)
            .withEntityClasses(Lesson::class.java)
            
        solverConfig.scoreDirectorFactoryConfig = ScoreDirectorFactoryConfig()
            .withIncrementalScoreCalculatorClass(TimeTableIncrementalScoreCalculator::class.java)
            
        solverConfig
            .withTerminationConfig(
                TerminationConfig()
                    .withSpentLimit(parsedSpentLimit)
                    .withUnimprovedSpentLimit(parsedUnimprovedLimit)
            )
            .withEnvironmentMode(envMode)
            .withPhases(
                ai.timefold.solver.core.config.constructionheuristic.ConstructionHeuristicPhaseConfig(),
                ai.timefold.solver.core.config.localsearch.LocalSearchPhaseConfig()
                    .withAcceptorConfig(ai.timefold.solver.core.config.localsearch.decider.acceptor.LocalSearchAcceptorConfig().withLateAcceptanceSize(400))
                    .withForagerConfig(ai.timefold.solver.core.config.localsearch.decider.forager.LocalSearchForagerConfig().withAcceptedCountLimit(1000))
                    .withMoveSelectorConfig(
                        ai.timefold.solver.core.config.heuristic.selector.move.composite.UnionMoveSelectorConfig(
                            listOf(
                                ai.timefold.solver.core.config.heuristic.selector.move.generic.ChangeMoveSelectorConfig(),
                                ai.timefold.solver.core.config.heuristic.selector.move.generic.SwapMoveSelectorConfig(),
                                ai.timefold.solver.core.config.heuristic.selector.move.generic.PillarSwapMoveSelectorConfig()
                            )
                        )
                    )
            )

        val solverFactory = SolverFactory.create<Timetable>(solverConfig)
        solverManager = SolverManager.create(solverFactory)
    }

    private fun parseDuration(durationStr: String): Duration {
        val clean = durationStr.trim().lowercase()
        return when {
            clean.endsWith("s") -> Duration.ofSeconds(clean.removeSuffix("s").toLongOrNull() ?: 120L)
            clean.endsWith("m") -> Duration.ofMinutes(clean.removeSuffix("m").toLongOrNull() ?: 2L)
            clean.endsWith("h") -> Duration.ofHours(clean.removeSuffix("h").toLongOrNull() ?: 1L)
            else -> Duration.ofSeconds(120L)
        }
    }
}
