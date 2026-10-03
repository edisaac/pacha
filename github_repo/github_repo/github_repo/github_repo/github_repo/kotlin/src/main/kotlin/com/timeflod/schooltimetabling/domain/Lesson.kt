package com.timeflod.schooltimetabling.domain

import ai.timefold.solver.core.api.domain.entity.PlanningEntity
import ai.timefold.solver.core.api.domain.lookup.PlanningId
import ai.timefold.solver.core.api.domain.variable.PlanningVariable
import ai.timefold.solver.core.api.domain.variable.CustomShadowVariable
import ai.timefold.solver.core.api.domain.variable.PlanningVariableReference
import ai.timefold.solver.core.api.domain.entity.PlanningPin
import ai.timefold.solver.core.api.domain.valuerange.ValueRangeProvider
import com.fasterxml.jackson.annotation.JsonIdentityReference
import com.fasterxml.jackson.annotation.JsonIgnore
import kotlin.jvm.Transient

@PlanningEntity
data class Lesson(
    @PlanningId
    val id: Int,
    val subject: Subject,
    val studentGroup: StudentGroup
) {
    @Transient @JsonIgnore
    lateinit var allTeachers: List<Teacher>
    @Transient @JsonIgnore
    lateinit var allRooms: List<Room>

    @JsonIdentityReference
    @PlanningVariable(valueRangeProviderRefs = ["availableTeachers"], allowsUnassigned = true)
    var teacher: Teacher? = null

    @JsonIdentityReference
    @PlanningVariable(allowsUnassigned = true)
    var timeslot: Timeslot? = null

    @JsonIdentityReference
    @PlanningVariable(allowsUnassigned = true)
    var room: Room? = null

    @JsonIdentityReference
    var parentLesson: Lesson? = null

    @Transient @JsonIgnore
    var duration: Int = 1

    @Transient @JsonIgnore
    var childLessons: MutableList<Lesson> = mutableListOf()

    @Transient @JsonIgnore
    var conflictingGroupIds: Set<Int> = emptySet()



    @PlanningPin
    var isPinned: Boolean = false

    constructor(id: Int, subject: Subject, studentGroup: StudentGroup, teacher: Teacher?, timeslot: Timeslot?, room: Room?) : this(id, subject, studentGroup) {
        this.teacher = teacher
        this.timeslot = timeslot
        this.room = room
    }

    fun teacherHasRequiredSubject(): Boolean {
        return teacher?.canTeachSubject(subject) ?: false
    }

    @JsonIgnore
    @ValueRangeProvider(id = "availableTeachers")
    fun getAvailableTeachers(): List<Teacher> {
        return allTeachers.filter { it.canTeachSubject(subject) }
    }

    /* 
    @JsonIgnore
    @ValueRangeProvider(id = "availableRooms")
    fun getAvailableRooms(): List<Room> {
        val requiredType = subject.requiredRoomType
        return if (requiredType == null) {
            allRooms
        } else {
            allRooms.filter { it.type == requiredType }
        }
    }
    */
    override fun toString(): String = "${subject.name}($id)"
}
