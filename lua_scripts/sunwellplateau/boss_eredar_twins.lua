-- Eredar Twins (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_eredar_twins.cpp
-- (boss_sacrolash, boss_alythess, npc_shadow_image); sunwell_
-- plateau.h:34 (DATA_EREDAR_TWINS = 3, fourth boss), :42 (DATA_
-- ALYTHESS), :43 (DATA_SACROLASH), :76 (NPC_GRAND_WARLOCK_ALYTHESS
-- = 25166), :77 (NPC_SHADOW_IMAGE = 25214), :78 (NPC_LADY_
-- SACROLASH = 25165). Entries: 25165 Lady Sacrolash (C++ ScriptName
-- "boss_sacrolash"), 25166 Grand Warlock Alythess (C++ ScriptName
-- "boss_alythess"), 25214 Shadow Image (C++ ScriptName "npc_
-- shadow_image") — all entry+ScriptName verifiable from the C++
-- sources per RegisterSunwellPlateauCreatureAI in AddSC_boss_
-- eredar_twins; the creature_template ScriptName bindings are DB-
-- side — no TDB in this workspace, so only the C++-side naming is
-- verified. Talk lines used: Sacrolash YELL_SAC_KILL=8 (kill, 25%),
-- EMOTE_SHADOW_NOVA=5 + YELL_SHADOW_NOVA=9 (shadow nova);
-- Alythess EMOTE_CONFLAGRATION=4 + YELL_CANFLAGRATION=8
-- (conflagration), YELL_ALY_KILL=5 (kill, 25%), YELL_BERSERK=9
-- (berserk). YELL_INTRO_SAC_*=0/1/2/3 and YELL_INTRO_ALY_*=0/1/2/3
-- belong to the unmodeled intro chain; YELL_SISTER_ALYTHESS_DEAD=7
-- and YELL_SISTER_SACROLASH_DEAD=7 belong to the sister-death
-- detection arms (cross-creature blocked — below). YELL_SAC_DEAD=4
-- and YELL_ALY_DEAD=6 are the JustDied death yells, ported via the
-- twinDead latch (see below).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. The "event 15 never fires — no bridge"
-- claim is STALE — CREATURE_EVENT_ON_SPELL_HIT_TARGET is bridged
-- (boss_ai.go:2097-2115; the arugal precedent). The SpellHitTarget
-- touched-spell arms stay unmodeled on hit-target-enumeration grounds:
-- C++ fires SpellHitTarget per ACTUAL hit unit of each cast, while Go
-- reports the cast's single target — the shadow-blades / flame-sear /
-- shadow-fury arms are self-cast AoEs, so Go would report the boss
-- herself as the "hit" target, and the touched chain is an interacting
-- aura system (dark-touched + flame-touched -> dark flame) that cannot
-- be half-ported on primary-target-only data without fabricating
-- aura states C++ never produces. Corrected 2026-10-17.
-- Fight shape (C++-exact for the modeled arms): Lady Sacrolash
-- (25165). OnEnterCombat(1): per-GUID reset, arm shadow blades
-- 45248 10s then 10s, non-triggered self-cast (C++ DoCast(me, ...),
-- default triggered=false — vaelastrasz convention) / shadow nova
-- 45329 30s then {30s,35s}, non-triggered on a random alive player
-- threat-list unit alive (C++ SelectTarget(Random, 0),
-- playerOnly=false, UnitAI.h:170; nil pick casts nothing but
-- keeps the schedule),
-- Talk(EMOTE_SHADOW_NOVA, on target) + Talk(YELL_SHADOW_NOVA,
-- even on nil pick — C++-exact) (the
-- !SisterDeath gate is always true on this path; the emote's C++
-- target arm is bridged — Talk(id, target) via the whisperTarget arm
-- (engine/world/creature_text.go:62; C++-exact, non-whisper rows
-- ignore the target) — both emote calls pass the target
-- (boss_eredar_twins.cpp:254, 597) /
-- confounding blow 45256 25s then {20s,25s}, non-triggered on a
-- threat-list unit alive, nil pick keeps the schedule
-- / enrage 46587 6min one-shot: Talk(YELL_ENRAGE), non-triggered
-- self-cast (C++ DoCast default triggered=false). The shadow-image
-- summon arm (3 x DoSpawnCreature NPC_SHADOW_IMAGE) has no summon
-- bridge — skipped (felmyst convention); the sister-death
-- conflagration arm 45342 has no cross-creature death bridge —
-- skipped (below). OnTargetDied(3): 25% Talk(YELL_SAC_KILL), no
-- TYPEID gate (C++-exact — the C++ KilledUnit Talk has no victim
-- gate). OnDied(4): the SisterDeath-gated Talk(YELL_SAC_DEAD 4) is
-- ported via the twinDead latch (both twins' ev4 handlers live in
-- this file and fire in death order); the loot-flag arm ports as
-- RemoveFlag(79, LOOTABLE) on the sister-alive death (dynamic-flag
-- bridge live, twin_valkyr precedent). OnReset(23):
-- per-GUID reset (Enraged=false, SisterDeath=false) + cancel
-- timers; the instance sister respawn/threat relay and SetBossState
-- (NOT_STARTED) arms are blocked on the instance-script model.
-- OnLeaveCombat(2): cancel timers, drop per-GUID state. The
-- UpdateAI UNIT_STATE_CASTING queue gates and the InterruptSpell
-- arms have no UNIT_STATE/interrupt bridges — timers fire
-- unconditionally (jeklik convention). The SpellHitTarget
-- HandleTouchedSpells arm (45248/45329/45256/45270 -> dark touched
-- 45347; 45342 -> flame touched 45348) is unmodeled on
-- hit-target-enumeration grounds (see header top — the "no bridge"
-- label was stale). The melee-range HandleTouchedSpells(victim, 45347)
-- arm has no melee-hook bridge — unmodeled (melee is engine-
-- driven). Grand Warlock Alythess (25166). OnEnterCombat(1):
-- per-GUID reset, arm conflagration 45342 45s then {30s,35s}, non-
-- triggered on a threat-list unit alive (nil pick
-- casts nothing but keeps the schedule), Talk(EMOTE_CONFLAGRATION,
-- on target) + Talk(YELL_CANFLAGRATION, even on nil pick —
-- C++-exact), then the blaze timer is re-armed
-- once at 4s (C++-exact — the conflag arm sets BlazeTimer = 4000)
-- / flame sear 46771 15s then 15s, non-triggered self-cast /
-- pyrogenics 45230 15s then 15s, triggered self-cast (C++ DoCast
-- (me, SPELL_PYROGENICS, true), C++-exact) / blaze 45235 100ms then
-- 3.8s, non-triggered DoCastVictim (nil ticks cast nothing but keep
-- the schedule, jeklik convention) / enrage 46587 6min one-shot:
-- Talk(YELL_BERSERK), non-triggered self-cast. The sister-death
-- shadow-nova arm 45329 and the empower/sister-dead arms have no
-- cross-creature bridge — skipped. OnTargetDied(3): 25% Talk(YELL_
-- ALY_KILL), no TYPEID gate. OnDied(4): sister-death-gated Talk
-- (YELL_ALY_DEAD 6) is ported via the twinDead latch; the loot-
-- flag arm ports as RemoveFlag(79, LOOTABLE) on the sister-alive
-- death (dynamic-flag bridge live). OnReset(23): per-GUID reset + cancel
-- timers; the instance sister respawn/threat relay and SetBossState
-- arms blocked on the instance-script model. OnLeaveCombat(2):
-- cancel timers, drop per-GUID state. The SpellHitTarget arm (45235
-- -> blaze-summon 45236 self-cast on the hit target; 45342/46771 ->
-- flame touched 45348; 45329 -> dark touched 45347) — the
-- SpellHitTarget bridge is LIVE (event 15, see header top); the arm
-- stays unmodeled on hit-target-enumeration grounds. The 8-step intro
-- chain
-- (MoveInLineOfSight sighting + cross-creature Talk relays) has no
-- MoveInLineOfSight bridge — the fight starts on pull (ragnaros-
-- intro convention). The no-victim threat handoff from the sister
-- and the sister AttackStart relays have no cross-creature/threat
-- bridges — unmodeled. Shadow Image (25214). OnReset(23): the NOT_
-- SELECTABLE flag arm is PORTED 2026-10-20 via the live flag bridge
-- (the old "no flag bridge" claim is retracted; C++:677-679 sets only
-- the flag + Initialize(); the cast lives in the per-tick HasAura
-- arm, C++:712). OnEnterCombat(1): arm
-- shadow fury 45270 {5s,20s} then 10s, non-triggered self-cast /
-- dark strike 45271 3s then 3s, non-triggered DoCastVictim (nil
-- ticks cast nothing but keep the schedule); a 10s visual-
-- maintenance check re-applies 45263 non-triggered if missing (the
-- C++ per-tick HasAura arm — DoCast(me,45263) takes no triggered
-- arg — approximated). The 15s KillSelf arm is PORTED in
-- shadowImageEnterCombat (creature:Kill(), C++ KillTimer 15000);
-- the old "no kill bridge" claim was stale. The
-- sacrolash-side DoSpawnCreature summon (plain SummonCreature offset
-- wrapper — the summon bridge is live) stays out on the paired
-- AddThreat(target, 1000000.0f, temp) + AttackStart(target) legs
-- (:281-282, unbridged cross-creature combat relay; the image's sole
-- role is attacking the picked target) — images arrive only via
-- DB-side spawns — registered anyway for its own verifiable AI
-- arms (felmyst-trail convention).
-- Deliberate deviations (all await engine bridges): no instance-
-- script model — boss admission via the luaBossAI shim (BossAI::
-- JustEngagedWith/JustDied/Reset and the DATA_EREDAR_TWINS book-
-- keeping, sister AttackStart/respawn/threat relays skipped); no cross-creature bridge — the sister-death detection
-- (instance->GetCreature(DATA_*), Talk(YELL_SISTER_*_DEAD), empower
-- 45366 self-cast, the sister-phase spell arms) and the SetBossState
-- (DONE) gates plus the alythess intro cross-Talk are unmodeled (hit-target-enumeration grounds, see header top); all touched-spell chaining
-- and the alythess blaze-summon (GO 187366) arms unmodeled; no
-- summon bridge — the shadow-image spawns unmodeled; no flag/move/
-- threat/MoveInLineOfSight/kill bridges — the lootable-flag,
-- no-movement, AddThreat, intro-sighting and image KillSelf arms
-- unmodeled.

local SPELL_SHADOW_BLADES = 45248
local SPELL_SHADOW_NOVA = 45329
local SPELL_CONFOUNDING_BLOW = 45256
local SPELL_CONFLAGRATION = 45342
local SPELL_ENRAGE = 46587
local SPELL_PYROGENICS = 45230
local SPELL_FLAME_SEAR = 46771
local SPELL_BLAZE = 45235

-- UnitDefines.h:149; SetFlag/RemoveFlag on UNIT_FIELD_FLAGS (59) is
-- bridged (engine/world/lua_creature_events.go:968).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local SPELL_IMAGE_VISUAL = 45263
local SPELL_SHADOW_FURY = 45270
local SPELL_DARK_STRIKE = 45271

local YELL_SAC_KILL = 8
local YELL_SAC_DEAD = 4
local EMOTE_SHADOW_NOVA = 5
local YELL_SHADOW_NOVA = 9
local YELL_ENRAGE = 6
local EMOTE_CONFLAGRATION = 4
local YELL_CANFLAGRATION = 8
local YELL_ALY_KILL = 5
local YELL_ALY_DEAD = 6
local YELL_BERSERK = 9

local timers = {}
local sacrolashState = {}
local alythessState = {}

-- Sister-death latch for the JustDied YELL_SAC_DEAD / YELL_ALY_DEAD arms
-- (boss_eredar_twins.cpp:150-163 sacrolash, :431-441 alythess). The C++
-- SisterDeath bool is set on the SURVIVING twin's AI when its UpdateAI
-- poll observes the sister dead, and the DYING twin's JustDied reads its
-- own latch — so the yell fires exactly when the sister died before this
-- twin. Both twins' ev4 handlers live in this file and fire in death
-- order, so the latch is module state set in ev4: no cross-creature
-- lookup is needed (the UpdateAI empower / YELL_SISTER_*_DEAD detection
-- arms stay out — instance->GetCreature has no bridge). Go fires ev23
-- before ev4 on the death path (fireCreatureDied), so the reset handlers
-- below clear the latch only when the creature is alive — C++ Reset
-- never runs on the death path (the spark_overgrind IsDead-gate
-- precedent).
local twinDead = { sacrolash = false, alythess = false }

-- UNIT_DYNAMIC_FLAGS (UpdateFields.h:136: OBJECT_END+0x49 = 79);
-- UNIT_DYNFLAG_LOOTABLE (SharedDefines.h:3128) — the dynamic-flag
-- bridge is live (twin_valkyr precedent); the old "lootable-flag arm
-- has no flag bridge" claims are retracted 2026-10-21.
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_DYNFLAG_LOOTABLE = 0x0001

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end


-- C++ SelectTarget(Random, 0) with playerOnly=FALSE (UnitAI.h:170):
-- random over the threat list, pets/totems eligible. Bridged via
-- creature:GetAITargets() (threat-ordered; lua_creature_events.go:345).
-- Dead victims go offline via ShouldBeOffline / _IsTargetAcceptable
-- (Creature.cpp:2508, UNIT_STATE_DIED) and SelectTargetList skips
-- offline refs (UnitAI.h), so the pick filters dead entries.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    local alive = {}
    for _, t in ipairs(targets) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
end

-- ================= Lady Sacrolash (25165) =================

-- C++ EVENT shadow blades: DoCastSelf(45248); repeat 10s.
local function sacrolashShadowBlades(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOW_BLADES)
    schedule(guid, "shadowblades", 10000, function()
        sacrolashShadowBlades(creature, guid)
    end)
end

-- C++ EVENT shadow nova: DoCast(random target, 45329) +
-- Talk(EMOTE_SHADOW_NOVA) + Talk(YELL_SHADOW_NOVA); repeat
-- {30s,35s}. The !SisterDeath Talk gate is always true on this
-- path (the UpdateAI sister-death detection poll is cross-creature
-- blocked; only the JustDied latch is ported); C++ fires
-- YELL_SHADOW_NOVA even when the target pick is nil — Talk
-- placement matches.
local function sacrolashShadowNova(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_NOVA)
        creature:Talk(EMOTE_SHADOW_NOVA, target)
    end
    creature:Talk(YELL_SHADOW_NOVA)
    -- C++ 30000 + rand32()%5000 is exclusive-top [30000,34999].
    schedule(guid, "shadownova", math.random(30000, 34999), function()
        sacrolashShadowNova(creature, guid)
    end)
end

-- C++ EVENT confounding blow: DoCast(random target, 45256);
-- repeat {20s,25s}.
local function sacrolashConfoundingBlow(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CONFOUNDING_BLOW)
    end
    -- C++ 20000 + rand32()%5000 is exclusive-top [20000,24999].
    schedule(guid, "confoundingblow", math.random(20000, 24999), function()
        sacrolashConfoundingBlow(creature, guid)
    end)
end

-- C++ EVENT enrage: Talk(YELL_ENRAGE) + DoCastSelf(46587);
-- 6min one-shot, Enraged once-guard (C++-exact).
local function sacrolashEnrage(creature, guid)
    local st = sacrolashState[guid]
    if st and not st.enraged then
        st.enraged = true
        creature:Talk(YELL_ENRAGE)
        creature:CastSpell(creature, SPELL_ENRAGE)
    end
end

-- C++ JustEngagedWith + Initialize: per-GUID reset, arm the
-- five bridgeable timers.
local function sacrolashEnterCombat(event, creature)
    creature:DoZoneInCombat() -- boss_eredar_twins.cpp:136 (JustEngagedWith arm 1; bridge live 2026-10-23)
    creature:SetMeleeAttack(false) -- boss_eredar_twins.cpp:211: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    local guid = creature:GetGUID()
    sacrolashState[guid] = { enraged = false, sisterDeath = false }
    schedule(guid, "shadowblades", 10000, function()
        sacrolashShadowBlades(creature, guid)
    end)
    schedule(guid, "shadownova", 30000, function()
        sacrolashShadowNova(creature, guid)
    end)
    schedule(guid, "confoundingblow", 25000, function()
        sacrolashConfoundingBlow(creature, guid)
    end)
    schedule(guid, "enrage", 360000, function()
        sacrolashEnrage(creature, guid)
    end)
end

-- C++ KilledUnit: 25% Talk(YELL_SAC_KILL), no victim gate.
local function sacrolashTargetDied(event, creature)
    if math.random(1, 4) == 1 then
        creature:Talk(YELL_SAC_KILL)
    end
end

-- C++ JustDied (:150-163): if (SisterDeath) Talk(YELL_SAC_DEAD 4) —
-- the latch is twinDead.alythess (sister died first); the
-- SetBossState(DONE) arm has no instance bridge; the lootable-flag
-- arm ports as the else-branch RemoveFlag(79) (C++-verbatim:
-- sister-alive death clears LOOTABLE on me — Go's lethal path
-- auto-set it, twin_valkyr precedent).
local function sacrolashDied(event, creature)
    twinDead.sacrolash = true
    if twinDead.alythess then
        creature:Talk(YELL_SAC_DEAD)
    else
        creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
    end
end

local function sacrolashLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sacrolashState[guid] = nil
end

local function sacrolashReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sacrolashState[guid] = nil
    if not creature:IsDead() then
        twinDead.sacrolash = false
        twinDead.alythess = false
    end
end

-- ================= Grand Warlock Alythess (25166) =================

-- C++ EVENT blaze: DoCastVictim(45235); repeat 3.8s. The
-- conflagration arm re-arms this once at 4s (C++-exact).
local function alythessBlaze(creature, guid)
    creature:CastSpell(nil, SPELL_BLAZE)
    schedule(guid, "blaze", 3800, function()
        alythessBlaze(creature, guid)
    end)
end

-- C++ EVENT conflagration: DoCast(random target, 45342) +
-- Talk(EMOTE_CONFLAGRATION) + Talk(YELL_CANFLAGRATION), BlazeTimer
-- = 4000; repeat {30s,35s}. C++ fires YELL_CANFLAGRATION and the
-- blaze re-arm even when the target pick is nil — placement
-- matches.
local function alythessConflagration(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CONFLAGRATION)
        creature:Talk(EMOTE_CONFLAGRATION, target)
    end
    creature:Talk(YELL_CANFLAGRATION)
    schedule(guid, "blaze", 4000, function()
        alythessBlaze(creature, guid)
    end)
    -- C++ 30000 + rand32()%5000 is exclusive-top [30000,34999].
    schedule(guid, "conflagration", math.random(30000, 34999), function()
        alythessConflagration(creature, guid)
    end)
end

-- C++ EVENT flame sear: DoCastSelf(46771); repeat 15s.
local function alythessFlameSear(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_SEAR)
    schedule(guid, "flamesear", 15000, function()
        alythessFlameSear(creature, guid)
    end)
end

-- C++ EVENT pyrogenics: triggered DoCastSelf(45230); repeat 15s.
local function alythessPyrogenics(creature, guid)
    creature:CastSpell(creature, SPELL_PYROGENICS, true)
    schedule(guid, "pyrogenics", 15000, function()
        alythessPyrogenics(creature, guid)
    end)
end

-- C++ EVENT enrage: Talk(YELL_BERSERK) + DoCastSelf(46587);
-- 6min one-shot, Enraged once-guard (C++-exact).
local function alythessEnrage(creature, guid)
    local st = alythessState[guid]
    if st and not st.enraged then
        st.enraged = true
        creature:Talk(YELL_BERSERK)
        creature:CastSpell(creature, SPELL_ENRAGE)
    end
end

-- C++ JustEngagedWith + Initialize: per-GUID reset, arm the
-- five bridgeable timers.
local function alythessEnterCombat(event, creature)
    creature:DoZoneInCombat() -- boss_eredar_twins.cpp:392 (JustEngagedWith arm 1; bridge live 2026-10-23)
    creature:SetMeleeAttack(false) -- boss_eredar_twins.cpp:535: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    creature:SetCombatMovement(false) -- ScriptedAI ctor arm, boss_eredar_twins.cpp:335 (ranged AI stands and casts)
    local guid = creature:GetGUID()
    alythessState[guid] = { enraged = false, sisterDeath = false }
    schedule(guid, "conflagration", 45000, function()
        alythessConflagration(creature, guid)
    end)
    schedule(guid, "flamesear", 15000, function()
        alythessFlameSear(creature, guid)
    end)
    schedule(guid, "pyrogenics", 15000, function()
        alythessPyrogenics(creature, guid)
    end)
    schedule(guid, "blaze", 100, function()
        alythessBlaze(creature, guid)
    end)
    schedule(guid, "enrage", 360000, function()
        alythessEnrage(creature, guid)
    end)
end

-- C++ KilledUnit: 25% Talk(YELL_ALY_KILL), no victim gate.
local function alythessTargetDied(event, creature)
    if math.random(1, 4) == 1 then
        creature:Talk(YELL_ALY_KILL)
    end
end

-- C++ JustDied (:431-441): if (SisterDeath) Talk(YELL_ALY_DEAD 6) —
-- the latch is twinDead.sacrolash (sister died first); the
-- SetBossState(DONE) arm has no instance bridge; the lootable-flag
-- arm ports as the else-branch RemoveFlag(79) (C++-verbatim, see
-- sacrolashDied above).
local function alythessDied(event, creature)
    twinDead.alythess = true
    if twinDead.sacrolash then
        creature:Talk(YELL_ALY_DEAD)
    else
        creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
    end
end

local function alythessLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    alythessState[guid] = nil
end

local function alythessReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    alythessState[guid] = nil
    if not creature:IsDead() then
        twinDead.sacrolash = false
        twinDead.alythess = false
    end
end

-- ================= Shadow Image (25214) =================

-- C++ EVENT shadow fury: DoCastSelf(45270); first {5s,20s},
-- then 10s.
local function shadowImageFury(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOW_FURY)
    schedule(guid, "shadowfury", 10000, function()
        shadowImageFury(creature, guid)
    end)
end

-- C++ EVENT dark strike: DoCastVictim(45271) only when within
-- melee range of the victim (creature:IsWithinMeleeRange);
-- the 3s re-arm fires regardless.
local function shadowImageDarkStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim and creature:IsWithinMeleeRange(victim) then
        creature:CastSpell(nil, SPELL_DARK_STRIKE)
    end
    schedule(guid, "darkstrike", 3000, function()
        shadowImageDarkStrike(creature, guid)
    end)
end

-- C++ per-tick image-visual maintenance arm (approximated at
-- 10s): non-triggered self-cast 45263 if the aura is missing.
local function shadowImageVisualCheck(creature, guid)
    if not creature:HasAura(SPELL_IMAGE_VISUAL) then
        creature:CastSpell(creature, SPELL_IMAGE_VISUAL)
    end
    schedule(guid, "visualcheck", 10000, function()
        shadowImageVisualCheck(creature, guid)
    end)
end

-- C++ JustEngagedWith (empty in C++) + Initialize: arm the
-- combat timers, including the 15s KillSelf arm (ported via
-- creature:Kill(); Reset re-arms it, and the evade cycle re-arms
-- via LeaveCombat cancel + EnterCombat).
local function shadowImageEnterCombat(event, creature)
    local guid = creature:GetGUID()
    -- C++ 5000 + rand32()%15000 is exclusive-top [5000,19999].
    schedule(guid, "shadowfury", math.random(5000, 19999), function()
        shadowImageFury(creature, guid)
    end)
    schedule(guid, "darkstrike", 3000, function()
        shadowImageDarkStrike(creature, guid)
    end)
    schedule(guid, "visualcheck", 10000, function()
        shadowImageVisualCheck(creature, guid)
    end)
    schedule(guid, "kill", 15000, function()
        creature:Kill()
    end)
end

local function shadowImageLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_shadow_imageAI::Reset (boss_eredar_twins.cpp:678-680):
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE), then
-- Initialize(); the visual is the per-tick maintenance arm above.
-- No RemoveFlag arm exists in the C++ file, so Reset re-asserts on
-- evade (ported 2026-10-20); skip on the death path — C++ Reset
-- never runs there (Eluna On_Reset fires ahead of OnDied).
local function shadowImageReset(event, creature)
    cancelTimers(creature:GetGUID())
    if not creature:IsDead() then
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    end
end

RegisterCreatureEvent(25165, 1, sacrolashEnterCombat)
RegisterCreatureEvent(25165, 2, sacrolashLeaveCombat)
RegisterCreatureEvent(25165, 3, sacrolashTargetDied)
RegisterCreatureEvent(25165, 4, sacrolashDied)
RegisterCreatureEvent(25165, 23, sacrolashReset)

RegisterCreatureEvent(25166, 1, alythessEnterCombat)
RegisterCreatureEvent(25166, 2, alythessLeaveCombat)
RegisterCreatureEvent(25166, 3, alythessTargetDied)
RegisterCreatureEvent(25166, 4, alythessDied)
RegisterCreatureEvent(25166, 23, alythessReset)

RegisterCreatureEvent(25214, 1, shadowImageEnterCombat)
RegisterCreatureEvent(25214, 2, shadowImageLeaveCombat)
RegisterCreatureEvent(25214, 23, shadowImageReset)
