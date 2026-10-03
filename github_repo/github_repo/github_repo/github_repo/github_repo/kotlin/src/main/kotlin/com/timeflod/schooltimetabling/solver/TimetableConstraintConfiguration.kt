package com.timeflod.schooltimetabling.solver

import ai.timefold.solver.core.api.domain.constraintweight.ConstraintConfiguration
import ai.timefold.solver.core.api.domain.constraintweight.ConstraintWeight
import ai.timefold.solver.core.api.score.buildin.hardsoft.HardSoftScore

@ConstraintConfiguration
class TimetableConstraintConfiguration(
    // --- Hard Constraints ---
    @ConstraintWeight("Room conflict")
    var roomConflict: HardSoftScore = HardSoftScore.ONE_HARD,

    @ConstraintWeight("Teacher conflict")
    var teacherConflict: HardSoftScore = HardSoftScore.ONE_HARD,

    @ConstraintWeight("Student group hierarchy conflict (siblings allowed)")
    var studentGroupConflict: HardSoftScore = HardSoftScore.ONE_HARD,

    @ConstraintWeight("Teacher consistency per subject")
    var teacherConsistency: HardSoftScore = HardSoftScore.ONE_HARD,

    // --- Soft Constraints ---
    @ConstraintWeight("Student group subject variety")
    var studentGroupSubjectVariety: HardSoftScore = HardSoftScore.ofSoft(10),

    @ConstraintWeight("Student time efficiency")
    var studentTimeEfficiency: HardSoftScore = HardSoftScore.ofSoft(5),

    @ConstraintWeight("Teacher room stability")
    var teacherRoomStability: HardSoftScore = HardSoftScore.ZERO,

    @ConstraintWeight("Teacher time efficiency")
    var teacherTimeEfficiency: HardSoftScore = HardSoftScore.ZERO,

    @ConstraintWeight("Penalize weekends")
    var penalizeWeekends: HardSoftScore = HardSoftScore.ZERO,

    @ConstraintWeight("Penalize Sunday")
    var penalizeSunday: HardSoftScore = HardSoftScore.ZERO
) {
    // Default empty constructor required by Timefold
    constructor() : this(
        HardSoftScore.ONE_HARD,
        HardSoftScore.ONE_HARD,
        HardSoftScore.ONE_HARD,
        HardSoftScore.ONE_HARD,
        HardSoftScore.ofSoft(10),
        HardSoftScore.ofSoft(5),
        HardSoftScore.ZERO,
        HardSoftScore.ZERO,
        HardSoftScore.ZERO,
        HardSoftScore.ZERO
    )
}
