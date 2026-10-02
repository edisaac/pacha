package com.timeflod.schooltimetabling.domain

/**
 * Utility object for domain-specific mappings and transformations.
 */
object DomainMapper {

    /**
     * Injects the global Problem Facts (available teachers, rooms) and propagates
     * group conflict constraints down into each planning entity (Lesson).
     */
    fun initializeProblemFacts(timetable: Timetable) {
        timetable.lessons.forEach { lesson ->
            lesson.allTeachers = timetable.teachers
            lesson.allRooms = timetable.rooms
            lesson.conflictingGroupIds = lesson.studentGroup.conflictingNotSiblingGroupIds
        }
    }

    /**
     * Reconstructs the tree of child lessons for multi-duration lessons
     * by assigning them consecutive timeslots and matching rooms/teachers.
     */
    fun inflateChildren(solution: Timetable) {
        val timeslots = solution.timeslots
        for (lesson in solution.planningLessons) {
            if (lesson.duration > 1) {
                var currentTimeslot = lesson.timeslot
                for (child in lesson.childLessons) {
                    child.room = lesson.room
                    child.teacher = lesson.teacher
                    if (currentTimeslot != null) {
                        currentTimeslot = timeslots.find { 
                            it.dayOfWeek == currentTimeslot!!.dayOfWeek && 
                            it.sequenceIndex > currentTimeslot!!.sequenceIndex 
                        }
                    }
                    child.timeslot = currentTimeslot
                }
            }
        }
    }
}
