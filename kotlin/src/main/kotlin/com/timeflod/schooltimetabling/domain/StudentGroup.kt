package com.timeflod.schooltimetabling.domain

import ai.timefold.solver.core.api.domain.lookup.PlanningId

data class StudentGroup(
    @PlanningId
    val id: Int,
    val cod: String,
    val name: String,
    val parentId: Int?,
    val parentCod: String?
) {
    // Matriz optimizada para O(1) lookup en el solver (TimeTableConstraintProvider)
    @Transient // No se persiste/serializa
    var conflictingNotSiblingGroupIds: Set<Int> = emptySet()

    // Pre-cálculo de todos los grupos hojas (sin hijos) que descienden de este grupo, o sí mismo si ya es hoja.
    // Typed as List<Int> (not Iterable) to allow size-aware iteration without iterator boxing in constraint streams.
    @Transient
    var leafGroupIds: List<Int> = emptyList()

    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (javaClass != other?.javaClass) return false
        other as StudentGroup
        return id == other.id
    }

    override fun hashCode(): Int {
        return id.hashCode()
    }

    override fun toString(): String = name
}
