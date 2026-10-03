package com.timeflod.schooltimetabling.domain

import ai.timefold.solver.core.api.domain.constraintweight.ConstraintConfigurationProvider
import ai.timefold.solver.core.api.domain.solution.PlanningEntityCollectionProperty
import ai.timefold.solver.core.api.domain.solution.PlanningScore
import ai.timefold.solver.core.api.domain.solution.PlanningSolution
import ai.timefold.solver.core.api.domain.solution.ProblemFactCollectionProperty
import ai.timefold.solver.core.api.domain.valuerange.ValueRangeProvider
import ai.timefold.solver.core.api.score.buildin.hardsoft.HardSoftScore
import ai.timefold.solver.core.api.solver.SolverStatus
import com.timeflod.schooltimetabling.solver.TimetableConstraintConfiguration

@PlanningSolution
data class Timetable(
    val id: String,
    @ProblemFactCollectionProperty
    val subjects: List<Subject>,
    @ProblemFactCollectionProperty
    val studentGroups: List<StudentGroup>,
    @ProblemFactCollectionProperty
    @ValueRangeProvider
    val teachers: List<Teacher>,
    @ProblemFactCollectionProperty
    @ValueRangeProvider
    val timeslots: List<Timeslot>,
    @ProblemFactCollectionProperty
    @ValueRangeProvider
    val rooms: List<Room>,
    val lessons: List<Lesson>,
    @PlanningEntityCollectionProperty
    val planningLessons: List<Lesson> = lessons.filter { it.parentLesson == null },
    @ConstraintConfigurationProvider
    val constraintConfiguration: TimetableConstraintConfiguration = TimetableConstraintConfiguration(),
    @PlanningScore
    var score: HardSoftScore? = null,
    var solverStatus: SolverStatus = SolverStatus.NOT_SOLVING
) {
    constructor(id: String, subjects: List<Subject>, studentGroups: List<StudentGroup>, teachers: List<Teacher>, timeslots: List<Timeslot>, rooms: List<Room>, lessons: List<Lesson>, constraintConfiguration: TimetableConstraintConfiguration = TimetableConstraintConfiguration()) : this(
        id, subjects, studentGroups, teachers, timeslots, rooms, lessons, lessons.filter { it.parentLesson == null }, constraintConfiguration, null, SolverStatus.NOT_SOLVING
    )

    override fun toString(): String = id
}
