package com.timeflod.schooltimetabling.solver

import ai.timefold.solver.core.api.score.buildin.hardsoft.HardSoftScore
import ai.timefold.solver.core.api.score.calculator.IncrementalScoreCalculator
import com.timeflod.schooltimetabling.domain.Lesson
import com.timeflod.schooltimetabling.domain.Timetable
import com.timeflod.schooltimetabling.domain.Timeslot
import it.unimi.dsi.fastutil.ints.Int2IntOpenHashMap
import it.unimi.dsi.fastutil.longs.Long2IntOpenHashMap
import it.unimi.dsi.fastutil.longs.Long2ObjectOpenHashMap
import jakarta.enterprise.context.Dependent

@Dependent
class TimeTableIncrementalScoreCalculator : IncrementalScoreCalculator<Timetable, HardSoftScore> {

    private var hardScore: Int = 0
    private var softScore: Int = 0
    private var config: TimetableConstraintConfiguration? = null

    // All keys are packed into Long primitives to avoid object creation.

    // 1. Room conflict: (roomId shl 32) or timeslotId
    private val roomTimeslotCount = Long2IntOpenHashMap()

    // 2. Teacher conflict: (teacherId shl 32) or timeslotId
    private val teacherTimeslotCount = Long2IntOpenHashMap()

    // 3. Student group hierarchy conflict: (timeslotId shl 32) or groupId
    private val timeslotGroupCount = Long2IntOpenHashMap()

    // 4. Teacher consistency per subject
    // (groupId shl 42) or (subjectId shl 21) or teacherId
    private val groupSubjectTeacherCount = Long2IntOpenHashMap()
    // (groupId shl 32) or subjectId
    private val groupSubjectDistinctCount = Long2IntOpenHashMap()

    // 5. Student group subject variety: (groupId shl 35) or (subjectId shl 3) or dayOfWeek
    private val groupSubjectDayCount = Long2IntOpenHashMap()

    // 6. Student time efficiency: (leafGroupId shl 32) or dayOfWeek
    private val leafDayBuckets = Long2ObjectOpenHashMap<LeafDayBucket>()

    // Fixed IntArray avoiding GC in the hot loop, dynamically resized if needed
    private class LeafDayBucket {
        var totalLessons = 0
        var minSeq = Int.MAX_VALUE
        var maxSeq = Int.MIN_VALUE
        var seqCounts = IntArray(128)

        fun add(seq: Int) {
            if (seq >= seqCounts.size) {
                var newSize = seqCounts.size * 2
                while (seq >= newSize) newSize *= 2
                seqCounts = seqCounts.copyOf(newSize)
            }
            seqCounts[seq]++
            totalLessons++
            if (seq < minSeq) minSeq = seq
            if (seq > maxSeq) maxSeq = seq
        }

        fun remove(seq: Int) {
            if (seq >= seqCounts.size) return
            seqCounts[seq]--
            totalLessons--
            if (totalLessons == 0) {
                minSeq = Int.MAX_VALUE
                maxSeq = Int.MIN_VALUE
            } else {
                if (seq == minSeq && seqCounts[seq] == 0) {
                    while (seqCounts[minSeq] == 0) minSeq++
                }
                if (seq == maxSeq && seqCounts[seq] == 0) {
                    while (seqCounts[maxSeq] == 0) maxSeq--
                }
            }
        }

        fun isEmpty(): Boolean = totalLessons == 0

        fun calculateQuadraticGaps(): Int {
            if (totalLessons < 2) return 0
            var gaps = 0
            var lastSeq = -1
            for (seq in minSeq..maxSeq) {
                if (seqCounts[seq] > 0) {
                    if (lastSeq != -1) {
                        val gap = seq - lastSeq - 1
                        if (gap > 0) {
                            gaps += gap * gap // Quadratic gap penalty
                        }
                    }
                    lastSeq = seq
                }
            }
            return gaps
        }
    }

    // 7. Teacher room stability: (teacherId shl 32) or roomId
    private val teacherRoomCount = Long2IntOpenHashMap()
    private val teacherDistinctRooms = Int2IntOpenHashMap() // teacherId -> distinct rooms count

    // 8. Teacher time efficiency: (teacherId shl 32) or dayOfWeek -> TeacherDayBucket
    private val teacherDayBuckets = Long2ObjectOpenHashMap<TeacherDayBucket>()

    private class TeacherDayBucket {
        var totalLessons = 0
        var minSeq = Int.MAX_VALUE
        var maxSeq = Int.MIN_VALUE
        var seqCounts = IntArray(128)

        fun add(seq: Int) {
            if (seq >= seqCounts.size) {
                var newSize = seqCounts.size * 2
                while (seq >= newSize) newSize *= 2
                seqCounts = seqCounts.copyOf(newSize)
            }
            seqCounts[seq]++
            totalLessons++
            if (seq < minSeq) minSeq = seq
            if (seq > maxSeq) maxSeq = seq
        }

        fun remove(seq: Int) {
            if (seq >= seqCounts.size) return
            seqCounts[seq]--
            totalLessons--
            if (totalLessons == 0) {
                minSeq = Int.MAX_VALUE
                maxSeq = Int.MIN_VALUE
            } else {
                if (seq == minSeq && seqCounts[seq] == 0) {
                    while (seqCounts[minSeq] == 0) minSeq++
                }
                if (seq == maxSeq && seqCounts[seq] == 0) {
                    while (seqCounts[maxSeq] == 0) maxSeq--
                }
            }
        }

        fun isEmpty(): Boolean = totalLessons == 0

        fun calculateQuadraticGaps(): Int {
            if (totalLessons < 2) return 0
            var gaps = 0
            var lastSeq = -1
            for (seq in minSeq..maxSeq) {
                if (seqCounts[seq] > 0) {
                    if (lastSeq != -1) {
                        val gap = seq - lastSeq - 1
                        if (gap > 0) {
                            gaps += gap * gap // Quadratic gap penalty
                        }
                    }
                    lastSeq = seq
                }
            }
            return gaps
        }
    }

    private var unassignedPenalty: Int = 0

    private var nextTimeslots = emptyMap<Int, Timeslot>()

    private fun countUnassigned(lesson: Lesson): Int {
        var count = 0
        if (lesson.timeslot == null) count++
        if (lesson.room == null) count++
        if (lesson.teacher == null) count++
        return count * lesson.duration
    }

    override fun resetWorkingSolution(workingSolution: Timetable) {
        SolverMetrics.reset()
        hardScore = 0
        softScore = 0
        unassignedPenalty = 0
        config = workingSolution.constraintConfiguration
        
        roomTimeslotCount.clear()
        teacherTimeslotCount.clear()
        timeslotGroupCount.clear()
        groupSubjectTeacherCount.clear()
        groupSubjectDistinctCount.clear()
        groupSubjectDayCount.clear()
        leafDayBuckets.clear()
        teacherRoomCount.clear()
        teacherDistinctRooms.clear()
        teacherDayBuckets.clear()

        val map = mutableMapOf<Int, Timeslot>()
        val grouped = workingSolution.timeslots.groupBy { it.dayOfWeek }
        for ((_, list) in grouped) {
            val sorted = list.sortedBy { it.sequenceIndex }
            for (i in 0 until sorted.size - 1) {
                map[sorted[i].id] = sorted[i+1]
            }
        }
        nextTimeslots = map

        for (lesson in workingSolution.planningLessons) {
            unassignedPenalty -= countUnassigned(lesson)
            if (lesson.timeslot != null) {
                insert(lesson)
            }
        }
    }

    override fun beforeEntityAdded(entity: Any) {}
    
    override fun afterEntityAdded(entity: Any) {
        val lesson = entity as Lesson
        unassignedPenalty -= countUnassigned(lesson)
        if (lesson.timeslot != null) insert(lesson)
    }

    override fun beforeVariableChanged(entity: Any, variableName: String) {
        if (variableName == "gapToPreviousLesson") return
        val lesson = entity as Lesson
        unassignedPenalty += countUnassigned(lesson)
        if (lesson.timeslot != null) retract(lesson)
    }

    override fun afterVariableChanged(entity: Any, variableName: String) {
        if (variableName == "gapToPreviousLesson") return
        val lesson = entity as Lesson
        unassignedPenalty -= countUnassigned(lesson)
        if (lesson.timeslot != null) insert(lesson)
    }

    override fun beforeEntityRemoved(entity: Any) {
        val lesson = entity as Lesson
        unassignedPenalty += countUnassigned(lesson)
        if (lesson.timeslot != null) retract(lesson)
    }
    
    override fun afterEntityRemoved(entity: Any) {}

    override fun calculateScore(): HardSoftScore {
        SolverMetrics.combinationsEvaluated.incrementAndGet()
        return HardSoftScore.of(hardScore + unassignedPenalty, softScore)
    }

    private fun insert(lesson: Lesson) {
        var currentTs: Timeslot? = lesson.timeslot!!
        val room = lesson.room
        val teacher = lesson.teacher
        val group = lesson.studentGroup
        val gId = group.id.toLong()
        val subject = lesson.subject
        val sId = subject.id.toLong()

        for (i in 0 until lesson.duration) {
            if (currentTs == null) {
                hardScore -= 100 // Penalty for out of bounds block
                break
            }

            val tId = currentTs.id.toLong()
            val day = currentTs.dayOfWeek.toLong()
            val seq = currentTs.sequenceIndex

        // 1. Room conflict (Hard)
        if (room != null) {
            val key = (room.id.toLong() shl 32) or (tId and 0xFFFFFFFFL)
            val count = roomTimeslotCount.getOrDefault(key, 0)
            hardScore -= count
            roomTimeslotCount[key] = count + 1
        }

        // 2. Teacher conflict (Hard)
        if (teacher != null) {
            val key = (teacher.id.toLong() shl 32) or (tId and 0xFFFFFFFFL)
            val count = teacherTimeslotCount.getOrDefault(key, 0)
            hardScore -= count
            teacherTimeslotCount[key] = count + 1
        }

        // 3. Student group hierarchy conflict (Hard)
        for (conflictingId in group.conflictingNotSiblingGroupIds) {
            val cKey = (tId shl 32) or (conflictingId.toLong() and 0xFFFFFFFFL)
            val c = timeslotGroupCount.getOrDefault(cKey, 0)
            hardScore -= c
        }
        val tgKey = (tId shl 32) or (gId and 0xFFFFFFFFL)
        timeslotGroupCount[tgKey] = timeslotGroupCount.getOrDefault(tgKey, 0) + 1

        // 4. Teacher consistency per subject (Hard)
        if (teacher != null) {
            val tchrId = teacher.id.toLong()
            val gstKey = (gId shl 42) or ((sId and 0x1FFFFFL) shl 21) or (tchrId and 0x1FFFFFL)
            val gsKey = (gId shl 32) or (sId and 0xFFFFFFFFL)

            val teacherSubjectCount = groupSubjectTeacherCount.getOrDefault(gstKey, 0)
            if (teacherSubjectCount == 0) {
                val distinctCount = groupSubjectDistinctCount.getOrDefault(gsKey, 0)
                if (distinctCount > 0) {
                    hardScore -= 1 // Penalty increased by 1 for a new distinct teacher
                }
                groupSubjectDistinctCount[gsKey] = distinctCount + 1
            }
            groupSubjectTeacherCount[gstKey] = teacherSubjectCount + 1
        }

        // 5. Student group subject variety (Soft)
        val weight5 = config?.studentGroupSubjectVariety?.softScore ?: 0
        if (weight5 > 0) {
            val key = (gId shl 35) or ((sId and 0xFFFFFFFFL) shl 3) or (day and 0x7L)
            val count = groupSubjectDayCount.getOrDefault(key, 0)
            softScore -= count * weight5
            groupSubjectDayCount[key] = count + 1
        }

        // 6. Student time efficiency (Soft)
        val weight6 = config?.studentTimeEfficiency?.softScore ?: 0
        if (weight6 > 0) {
            for (i in 0 until group.leafGroupIds.size) {
                val leafId = group.leafGroupIds[i].toLong()
                val key = (leafId shl 32) or (day and 0xFFFFFFFFL)
                var bucket = leafDayBuckets[key]
                if (bucket == null) {
                    bucket = LeafDayBucket()
                    leafDayBuckets[key] = bucket
                }
                val oldGaps = bucket.calculateQuadraticGaps()
                bucket.add(seq)
                val newGaps = bucket.calculateQuadraticGaps()
                softScore += (oldGaps - newGaps) * weight6
            }
        }

        // 7. Teacher room stability (Soft)
        if (teacher != null && room != null) {
            val weight7 = config?.teacherRoomStability?.softScore ?: 0
            if (weight7 > 0) {
                val tchrId = teacher.id
                val key = (tchrId.toLong() shl 32) or (room.id.toLong() and 0xFFFFFFFFL)
                val thisRoomCount = teacherRoomCount.getOrDefault(key, 0)
                
                if (thisRoomCount == 0) { // First time in this room
                    val distinct = teacherDistinctRooms.getOrDefault(tchrId, 0)
                    teacherDistinctRooms[tchrId] = distinct + 1
                    if (distinct == 1) { // It's their 2nd room! Apply the single penalty.
                        softScore -= weight7
                    }
                }
                teacherRoomCount[key] = thisRoomCount + 1
            }
        }

        // 8. Teacher time efficiency (Soft)
        if (teacher != null) {
            val weight8 = config?.teacherTimeEfficiency?.softScore ?: 0
            if (weight8 > 0) {
                val key = (teacher.id.toLong() shl 32) or (day and 0xFFFFFFFFL)
                var bucket = teacherDayBuckets[key]
                if (bucket == null) {
                    bucket = TeacherDayBucket()
                    teacherDayBuckets[key] = bucket
                }
                val oldGaps = bucket.calculateQuadraticGaps()
                bucket.add(seq)
                val newGaps = bucket.calculateQuadraticGaps()
                softScore += (oldGaps - newGaps) * weight8
            }
        }

        // 9. Penalize weekends (Soft)
        val weight9 = config?.penalizeWeekends?.softScore ?: 0
        if (weight9 > 0 && currentTs.dayOfWeek >= 5) {
            softScore -= weight9
        }

        // 10. Penalize Sunday (Soft)
        val weight10 = config?.penalizeSunday?.softScore ?: 0
        if (weight10 > 0 && currentTs.dayOfWeek == 6) {
            softScore -= weight10
        }

            if (i < lesson.duration - 1) {
                currentTs = nextTimeslots[currentTs.id]
            }
        }
    }

    private fun retract(lesson: Lesson) {
        var currentTs: Timeslot? = lesson.timeslot!!
        val room = lesson.room
        val teacher = lesson.teacher
        val group = lesson.studentGroup
        val gId = group.id.toLong()
        val subject = lesson.subject
        val sId = subject.id.toLong()

        for (i in 0 until lesson.duration) {
            if (currentTs == null) {
                hardScore += 100
                break
            }

            val tId = currentTs.id.toLong()
            val day = currentTs.dayOfWeek.toLong()
            val seq = currentTs.sequenceIndex

        // 1. Room conflict (Hard)
        if (room != null) {
            val key = (room.id.toLong() shl 32) or (tId and 0xFFFFFFFFL)
            val count = roomTimeslotCount.getOrDefault(key, 0)
            hardScore += (count - 1)
            if (count <= 1) roomTimeslotCount.remove(key) else roomTimeslotCount[key] = count - 1
        }

        // 2. Teacher conflict (Hard)
        if (teacher != null) {
            val key = (teacher.id.toLong() shl 32) or (tId and 0xFFFFFFFFL)
            val count = teacherTimeslotCount.getOrDefault(key, 0)
            hardScore += (count - 1)
            if (count <= 1) teacherTimeslotCount.remove(key) else teacherTimeslotCount[key] = count - 1
        }

        // 3. Student group hierarchy conflict (Hard)
        val tgKey = (tId shl 32) or (gId and 0xFFFFFFFFL)
        val tgCount = timeslotGroupCount.getOrDefault(tgKey, 0) - 1
        if (tgCount <= 0) timeslotGroupCount.remove(tgKey) else timeslotGroupCount[tgKey] = tgCount
        
        for (conflictingId in group.conflictingNotSiblingGroupIds) {
            val cKey = (tId shl 32) or (conflictingId.toLong() and 0xFFFFFFFFL)
            val c = timeslotGroupCount.getOrDefault(cKey, 0)
            hardScore += c
        }

        // 4. Teacher consistency per subject (Hard)
        if (teacher != null) {
            val tchrId = teacher.id.toLong()
            val gstKey = (gId shl 42) or ((sId and 0x1FFFFFL) shl 21) or (tchrId and 0x1FFFFFL)
            val gsKey = (gId shl 32) or (sId and 0xFFFFFFFFL)

            val teacherSubjectCount = groupSubjectTeacherCount.getOrDefault(gstKey, 0) - 1
            if (teacherSubjectCount <= 0) {
                groupSubjectTeacherCount.remove(gstKey)
                val distinctCount = groupSubjectDistinctCount.getOrDefault(gsKey, 0) - 1
                if (distinctCount > 0) {
                    hardScore += 1
                }
                if (distinctCount <= 0) groupSubjectDistinctCount.remove(gsKey) else groupSubjectDistinctCount[gsKey] = distinctCount
            } else {
                groupSubjectTeacherCount[gstKey] = teacherSubjectCount
            }
        }

        // 5. Student group subject variety (Soft)
        val weight5 = config?.studentGroupSubjectVariety?.softScore ?: 0
        if (weight5 > 0) {
            val key = (gId shl 35) or ((sId and 0xFFFFFFFFL) shl 3) or (day and 0x7L)
            val count = groupSubjectDayCount.getOrDefault(key, 0)
            softScore += (count - 1) * weight5
            if (count <= 1) groupSubjectDayCount.remove(key) else groupSubjectDayCount[key] = count - 1
        }

        // 6. Student time efficiency (Soft)
        val weight6 = config?.studentTimeEfficiency?.softScore ?: 0
        if (weight6 > 0) {
            for (i in 0 until group.leafGroupIds.size) {
                val leafId = group.leafGroupIds[i].toLong()
                val key = (leafId shl 32) or (day and 0xFFFFFFFFL)
                val bucket = leafDayBuckets[key]
                if (bucket != null) {
                    val oldGaps = bucket.calculateQuadraticGaps()
                    bucket.remove(seq)
                    val newGaps = bucket.calculateQuadraticGaps()
                    softScore += (oldGaps - newGaps) * weight6
                    if (bucket.isEmpty()) leafDayBuckets.remove(key)
                }
            }
        }

        // 7. Teacher room stability (Soft)
        if (teacher != null && room != null) {
            val weight7 = config?.teacherRoomStability?.softScore ?: 0
            if (weight7 > 0) {
                val tchrId = teacher.id
                val key = (tchrId.toLong() shl 32) or (room.id.toLong() and 0xFFFFFFFFL)
                
                val thisRoomCount = teacherRoomCount.getOrDefault(key, 0) - 1
                if (thisRoomCount <= 0) { // No longer using this room at all
                    teacherRoomCount.remove(key)
                    val distinct = teacherDistinctRooms.getOrDefault(tchrId, 0)
                    teacherDistinctRooms[tchrId] = distinct - 1
                    if (distinct == 2) { // They were using 2 rooms, now they use 1
                        softScore += weight7
                    }
                } else {
                    teacherRoomCount[key] = thisRoomCount
                }
            }
        }

        // 8. Teacher time efficiency (Soft)
        if (teacher != null) {
            val weight8 = config?.teacherTimeEfficiency?.softScore ?: 0
            if (weight8 > 0) {
                val key = (teacher.id.toLong() shl 32) or (day and 0xFFFFFFFFL)
                val bucket = teacherDayBuckets[key]
                if (bucket != null) {
                    val oldGaps = bucket.calculateQuadraticGaps()
                    bucket.remove(seq)
                    val newGaps = bucket.calculateQuadraticGaps()
                    softScore += (oldGaps - newGaps) * weight8
                    if (bucket.isEmpty()) teacherDayBuckets.remove(key)
                }
            }
        }

        // 9. Penalize weekends (Soft)
        val weight9 = config?.penalizeWeekends?.softScore ?: 0
        if (weight9 > 0 && currentTs.dayOfWeek >= 5) {
            softScore += weight9
        }

        // 10. Penalize Sunday (Soft)
        val weight10 = config?.penalizeSunday?.softScore ?: 0
        if (weight10 > 0 && currentTs.dayOfWeek == 6) {
            softScore += weight10
        }

            if (i < lesson.duration - 1) {
                currentTs = nextTimeslots[currentTs.id]
            }
        }
    }
}
