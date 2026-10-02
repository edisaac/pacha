package com.timeflod.schooltimetabling.domain

import ai.timefold.solver.core.api.domain.lookup.PlanningId

data class Teacher(
    @PlanningId
    val id: Int,
    val cod: String,
    val name: String,
    val subjects: List<Int> = emptyList()
) {
    fun canTeachSubject(subject: Subject): Boolean {
        return subjects.contains(subject.id)
    }

    fun canTeachSubject(subjectId: Int): Boolean {
        return subjects.contains(subjectId)
    }

    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (javaClass != other?.javaClass) return false
        other as Teacher
        return id == other.id
    }

    override fun hashCode(): Int {
        return id.hashCode()
    }

    override fun toString(): String = name
}

