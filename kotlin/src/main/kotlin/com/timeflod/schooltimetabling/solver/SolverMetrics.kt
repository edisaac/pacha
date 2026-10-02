package com.timeflod.schooltimetabling.solver

import java.util.concurrent.atomic.AtomicLong

object SolverMetrics {
    val combinationsEvaluated = AtomicLong(0)
    
    fun reset() {
        combinationsEvaluated.set(0)
    }
}
