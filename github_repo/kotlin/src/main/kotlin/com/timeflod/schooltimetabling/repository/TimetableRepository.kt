package com.timeflod.schooltimetabling.repository

import ai.timefold.solver.core.api.score.buildin.hardsoft.HardSoftScore
import com.timeflod.schooltimetabling.domain.*
import jakarta.enterprise.context.ApplicationScoped
import org.jboss.logging.Logger
import java.sql.Connection
import java.sql.DriverManager
import java.time.LocalTime

@ApplicationScoped
class TimetableRepository {

    private val logger = Logger.getLogger(TimetableRepository::class.java)

    /**
     * Reads the entire Timetable problem from the SQLite database.
     */
    fun readTimetableFromSqlite(taskId: String, dbDir: String): Timetable {
        val jdbcUrl = "jdbc:sqlite:$dbDir/$taskId.db"
        
        return openConnection(jdbcUrl).use { connection ->
            val rooms = mutableListOf<Room>()
            val teachers = mutableListOf<Teacher>()
            val subjects = mutableListOf<Subject>()
            val studentGroups = mutableListOf<StudentGroup>()
            val timeslots = mutableListOf<Timeslot>()
            val lessons = mutableListOf<Lesson>()

            val roomMap = mutableMapOf<Int, Room>()
            val teacherMap = mutableMapOf<Int, Teacher>()
            val subjectMap = mutableMapOf<Int, Subject>()
            val groupMap = mutableMapOf<Int, StudentGroup>()
            val timeslotMap = mutableMapOf<Int, Timeslot>()

            connection.createStatement().use { stmt ->
                // Sort by day_of_week and start_time to assign sequenceIndex
                var rs = stmt.executeQuery("SELECT id, cod, day_of_week, name_of_day, start_time, end_time FROM timeslot ORDER BY day_of_week, start_time")
                var currentIndex = 0
                while (rs.next()) {
                    val t = Timeslot(
                        rs.getInt("id"),
                        rs.getString("cod"),
                        rs.getInt("day_of_week"),
                        rs.getString("name_of_day"),
                        LocalTime.parse(rs.getString("start_time")),
                        LocalTime.parse(rs.getString("end_time")),
                        currentIndex++
                    )
                    timeslots.add(t); timeslotMap[t.id] = t
                }

                rs = stmt.executeQuery("SELECT id, cod, name FROM room")
                while (rs.next()) {
                    val r = Room(rs.getInt("id"), rs.getString("cod"), rs.getString("name"))
                    rooms.add(r); roomMap[r.id] = r
                }

                val teacherSubjects = mutableMapOf<Int, MutableList<Int>>()
                rs = stmt.executeQuery("SELECT teacher_id, subject_id FROM teacher_subject")
                while (rs.next()) {
                    teacherSubjects.getOrPut(rs.getInt("teacher_id")) { mutableListOf() }.add(rs.getInt("subject_id"))
                }

                rs = stmt.executeQuery("SELECT id, cod, name FROM teacher")
                while (rs.next()) {
                    val tId = rs.getInt("id")
                    val t = Teacher(tId, rs.getString("cod"), rs.getString("name"), teacherSubjects[tId] ?: emptyList())
                    teachers.add(t); teacherMap[t.id] = t
                }

                rs = stmt.executeQuery("SELECT id, cod, name FROM subject")
                while (rs.next()) {
                    val s = Subject(rs.getInt("id"), rs.getString("cod"), rs.getString("name"))
                    subjects.add(s); subjectMap[s.id] = s
                }

                rs = stmt.executeQuery("SELECT id, cod, name, parent_id, parent_cod FROM student_group")
                while (rs.next()) {
                    val pId = rs.getObject("parent_id") as? Int
                    val sg = StudentGroup(rs.getInt("id"), rs.getString("cod"), rs.getString("name"), pId, rs.getString("parent_cod"))
                    studentGroups.add(sg); groupMap[sg.id] = sg
                }

                try {
                    rs = stmt.executeQuery("SELECT group_id, conflicting_group_id FROM student_group_conflict")
                    val groupConflicts = mutableMapOf<Int, MutableSet<Int>>()
                    while (rs.next()) {
                        val gId = rs.getInt("group_id")
                        val cId = rs.getInt("conflicting_group_id")
                        groupConflicts.getOrPut(gId) { mutableSetOf() }.add(cId)
                    }
                    studentGroups.forEach { group ->
                        group.conflictingNotSiblingGroupIds = groupConflicts[group.id] ?: emptySet()
                    }
                } catch (e: Exception) {
                    logger.warn("[Kotlin] Error al leer student_group_conflict (la tabla podría no existir si no hay data): ${e.message}")
                }

                try {
                    rs = stmt.executeQuery("SELECT group_id, leaf_group_id FROM student_group_leaf")
                    val groupLeafs = mutableMapOf<Int, MutableList<Int>>()
                    while (rs.next()) {
                        val gId = rs.getInt("group_id")
                        val lId = rs.getInt("leaf_group_id")
                        groupLeafs.getOrPut(gId) { mutableListOf() }.add(lId)
                    }
                    studentGroups.forEach { group ->
                        group.leafGroupIds = groupLeafs[group.id] ?: emptyList()
                    }
                } catch (e: Exception) {
                    logger.warn("[Kotlin] Error al leer student_group_leaf (la tabla podría no existir si no hay data): ${e.message}")
                }


                val lessonMap = mutableMapOf<Int, Lesson>()
                rs = stmt.executeQuery("SELECT id, subject_id, teacher_id, student_group_id, room_id, timeslot_id, is_pinned, parent_id FROM lesson ORDER BY id")
                while (rs.next()) {
                    val lessonId = rs.getInt("id")
                    val subjId = rs.getInt("subject_id")
                    val teachId = rs.getInt("teacher_id")
                    val groupId = rs.getInt("student_group_id")
                    val roomId = rs.getObject("room_id") as? Int
                    val timeslotId = rs.getObject("timeslot_id") as? Int
                    val parentId = rs.getObject("parent_id") as? Int

                    val l = Lesson(lessonId, subjectMap[subjId]!!, groupMap[groupId]!!)
                    if (teachId != 0) l.teacher = teacherMap[teachId]
                    if (roomId != null) l.room = roomMap[roomId]
                    if (timeslotId != null) l.timeslot = timeslotMap[timeslotId]
                    l.isPinned = rs.getBoolean("is_pinned")
                    if (parentId != null) {
                        val parent = lessonMap[parentId]
                        l.parentLesson = parent
                        l.isPinned = true
                        parent?.childLessons?.add(l)
                        parent?.duration = (parent?.duration ?: 1) + 1
                    }
                    
                    l.allTeachers = teachers
                    l.allRooms = rooms
                    lessons.add(l)
                    lessonMap[lessonId] = l
                }
            }

            val config = com.timeflod.schooltimetabling.solver.TimetableConstraintConfiguration()
            connection.createStatement().use { stmt ->
                try {
                    val rs = stmt.executeQuery("SELECT rule_name, weight FROM rule_configuration")
                    while (rs.next()) {
                        val name = rs.getString("rule_name")
                        val weight = rs.getInt("weight")
                        when (name) {
                            "studentGroupSubjectVariety" -> config.studentGroupSubjectVariety = HardSoftScore.ofSoft(weight)
                            "studentTimeEfficiency" -> config.studentTimeEfficiency = HardSoftScore.ofSoft(weight)
                            "teacherRoomStability" -> config.teacherRoomStability = HardSoftScore.ofSoft(weight)
                            "teacherTimeEfficiency" -> config.teacherTimeEfficiency = HardSoftScore.ofSoft(weight)
                            "penalizeWeekends" -> config.penalizeWeekends = HardSoftScore.ofSoft(weight)
                            "penalizeSunday" -> config.penalizeSunday = HardSoftScore.ofSoft(weight)
                        }
                    }
                    logger.info("[Kotlin] Constraint Config Loaded -> studentGroupSubjectVariety: ${config.studentGroupSubjectVariety.softScore}, studentTimeEfficiency: ${config.studentTimeEfficiency.softScore}, teacherRoomStability: ${config.teacherRoomStability.softScore}, teacherTimeEfficiency: ${config.teacherTimeEfficiency.softScore}, penalizeWeekends: ${config.penalizeWeekends.softScore}, penalizeSunday: ${config.penalizeSunday.softScore}")
                } catch (e: Exception) {
                    logger.warn("[Kotlin] Error al leer rule_configuration (la tabla podría no existir): ${e.message}")
                }
            }

            Timetable(taskId, subjects, studentGroups, teachers, timeslots, rooms, lessons, config)
        }
    }

    /** Opens a new SQLite connection with WAL mode enabled. */
    private fun openConnection(jdbcUrl: String): Connection {
        val conn = getSqliteConnection(jdbcUrl)
        conn.createStatement().use { stmt ->
            stmt.execute("PRAGMA journal_mode=WAL")
            stmt.execute("PRAGMA synchronous=NORMAL")
            // Reduce lock wait time for concurrent access with Go
            stmt.execute("PRAGMA busy_timeout=5000")
        }
        return conn
    }

    private fun getSqliteConnection(url: String): Connection {
        try {
            Class.forName("org.sqlite.JDBC")
        } catch (e: Throwable) {
            logger.debug("Class.forName('org.sqlite.JDBC') info: ${e.message}")
        }
        try {
            DriverManager.registerDriver(org.sqlite.JDBC())
        } catch (e: Throwable) {
            logger.debug("DriverManager.registerDriver info: ${e.message}")
        }
        return DriverManager.getConnection(url)
    }
}
