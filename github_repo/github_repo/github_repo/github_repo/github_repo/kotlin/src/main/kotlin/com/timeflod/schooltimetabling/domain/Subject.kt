package com.timeflod.schooltimetabling.domain

import ai.timefold.solver.core.api.domain.lookup.PlanningId

data class Subject(
    @PlanningId
    val id: Int,
    val cod: String,
    val name: String
) {
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (javaClass != other?.javaClass) return false
        other as Subject
        return id == other.id
    }

    override fun hashCode(): Int {
        return id.hashCode()
    }

    override fun toString(): String = name
}

