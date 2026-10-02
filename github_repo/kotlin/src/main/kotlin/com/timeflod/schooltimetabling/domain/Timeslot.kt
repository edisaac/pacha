package com.timeflod.schooltimetabling.domain

import ai.timefold.solver.core.api.domain.lookup.PlanningId
import java.time.LocalTime

data class Timeslot(
    @PlanningId
    val id: Int,
    val cod: String,
    val dayOfWeek: Int,
    val nameOfDay: String,
    val startTime: LocalTime,
    val endTime: LocalTime,
    val sequenceIndex: Int
) {
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (javaClass != other?.javaClass) return false
        other as Timeslot
        return id == other.id
    }

    override fun hashCode(): Int {
        return id.hashCode()
    }

    override fun toString(): String = "$nameOfDay($id)"
}

