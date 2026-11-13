-- Ley-Guardian Eregos (The Oculus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/boss_eregos.cpp
-- (boss_eregos (CreatureScript) via GetOculusAI<boss_eregosAI>
-- (BossAI, DATA_EREGOS); spell_eregos_planar_shift
-- (SpellScriptLoader -> AuraScript); achievement_gen_eregos_void
-- x3 (AchievementCriteriaScript: achievement_ruby_void /
-- achievement_emerald_void / achievement_amber_void); all
-- registered from inside AddSC_boss_eregos(), no separate loader
-- lines; loader decl 87 / call 282 per
-- northrend_script_loader.cpp — the FOURTH group of the
-- "// The Nexus: The Oculus" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_varos() (decl 86 / call 281);
-- the calls after it are AddSC_instance_oculus() (decl 88 /
-- call 283), AddSC_oculus() (decl 89 / call 284) — loader order
-- confirmed this run; the checkpoint sequence (varos ->
-- eregos) is followed).
-- Entry: 27656 Ley-Guardian Eregos (oculus.h NPC_EREGOS, line 44;
-- DATA_EREGOS = 3, line 34; instance_oculus.cpp OnCreatureCreate
-- binds case NPC_EREGOS (line 72) and GetGuidData cases
-- DATA_EREGOS (lines 209 / 249) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_eregos"
-- hits boss_eregos.cpp + the loader decl/call lines only; grep
-- for "spell_eregos_planar_shift" hits boss_eregos.cpp only;
-- grep for "achievement_gen_eregos_void" /
-- "achievement_ruby_void" / "achievement_emerald_void" /
-- "achievement_amber_void" hits boss_eregos.cpp only; zero sql/
-- hits. No eregos lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 1) (event 1; the
-- BossAI::JustEngagedWith base arms are PORTED in eregosEnterCombat
-- (SetBossState(DATA_EREGOS, IN_PROGRESS) + pulse + me-form DoZone,
-- base-first per C++; instance/setActive/ScheduleTasks legs out).
-- SetBossState PORTED 2026-11-05 (creature:SetBossState bridged,
-- lua_creature_events.go:5917; SetBossState census)
-- + the drake-vehicle
-- FindNearestCreature achievement-void legs
-- (boss_eregos.cpp:124-128; NPC_RUBY_DRAKE_VEHICLE 27756 /
-- NPC_EMERALD_DRAKE_VEHICLE 27692 / NPC_AMBER_DRAKE_VEHICLE
-- 27755, 500.0f) — creature-search IS bridged
-- (lua_creature_events.go:1237); the latches are kept C++-exact
-- (reseeded true per engage, cleared to false where a vehicle is
-- found — the bridge returns nil when none), and mirrored into
-- the boss motion's SetData store (tick 417) for the Go
-- achievement gate.
-- KilledUnit — Talk(SAY_KILL 3) (event 3 — C++-GATED
-- who->GetTypeId() == TYPEID_PLAYER; the nalorakk / kelthuzad
-- player-gate variant — tenth ported variant overall, seventh
-- identical to nalorakk / kelthuzad / gothik / thaddius /
-- keristrasza / urom).
-- JustDied — Talk(SAY_DEATH 4) (event 4; the _JustDied
-- SetBossState(DONE) leg is PORTED in eregosDied (Talk-first,
-- 2026-11-05 — C++ calls Talk before _JustDied here,
-- boss_eregos.cpp:247-251).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — _Reset() SetBossState(NOT_STARTED, != DONE-gated)
-- PORTED 2026-11-05 in eregosReset (event 23; _Reset() is first in
-- C++ Reset) + Initialize() (_phase = PHASE_NORMAL, all three void
-- booleans = true) + DoAction(ACTION_SET_NORMAL_EVENTS) — the
-- timer legs seed on engage per the krystallus convention.
-- DoAction(ACTION_SET_NORMAL_EVENTS) — events.SetPhase(
-- PHASE_NORMAL) + ScheduleEvent x4 (EVENT_ARCANE_BARRAGE
-- 3s/10s, EVENT_ARCANE_VOLLEY 10s/25s, EVENT_ENRAGED_ASSAULT
-- 35s/50s, EVENT_SUMMON_LEY_WHELP 15s/30s, all PHASE_NORMAL)
-- (boss_eregos.cpp:148-157; seeded from Reset() at :107) —
-- the timer bridge is CreateLuaEvent (the stale "no
-- timer-event / phase bridge" claim is retracted); the ports
-- seed from OnEnterCombat(1) and cancel on 2/4/23 per the
-- krystallus/bjarngrim/halion convention. The planar-shift
-- machine that leaves NORMAL is PORTED 2026-11-08 (tick 427,
-- eregosDamageTaken below) and the AuraScript-driven re-arm of
-- this DoAction (spell_eregos_planar_shift AfterEffectRemove,
-- :274-282) is PORTED too (tick 427, planarShiftRemove, ev3);
-- both share seedNormalEvents below.
-- UpdateAI — the arcane event machine (boss_eregos.cpp:204-
-- 239), PORTED arms (all non-triggered):
-- EVENT_ARCANE_BARRAGE DoCastVictim(SPELL_ARCANE_BARRAGE 50804),
-- 3s/10s init -> 3s/10s loop — PORTED as CreateLuaEvent
-- {3000, 10000} + CastSpell(nil, 50804).
-- EVENT_ARCANE_VOLLEY DoCastAOE(SPELL_ARCANE_VOLLEY 51153),
-- 10s/25s init -> 10s/25s loop — PORTED as CreateLuaEvent
-- {10000, 25000} + CastSpell(creature, 51153) (the established
-- DoCastAOE -> self-cast lowering, AGENTS.md 2026-10-13).
-- EVENT_ENRAGED_ASSAULT Talk(SAY_ENRAGE 2) +
-- DoCast(SPELL_ENRAGED_ASSAULT 51170) self-cast, 35s/50s init
-- -> 35s/50s loop — PORTED as CreateLuaEvent {35000,
-- 50000} + Talk(2) + CastSpell(creature, 51170).
-- EVENT_SUMMON_LEY_WHELP 3x DoCast(SPELL_SUMMON_LEY_WHELP
-- 51175) self-cast, 15s/30s init -> 15s/30s loop — RESTORED
-- 2026-11-08 (tick 427, leyWhelpTick below): the creature-cast
-- path carries SPELL_EFFECT_SUMMON via creatureEffectSummonType
-- (boss_ai.go:3299, DBC-gated), so the arm is dormant-but-exact
-- until Spell.dbc ships — the Oct-14 removal's "no
-- SPELL_EFFECT_SUMMON leg" premise is stale.
-- The UNIT_STATE_CASTING gates are PORTED 2026-11-04 (500ms
-- IsCasting defer, mal_ganis idiom): own UpdateAI pre-loop :209 +
-- in-loop :238 gates cover all 3 ported ticks
-- (barrage/volley/enraged — defer precedes victim cast / self-cast /
-- Talk+cast; the event stays queued, no re-arm consumed). The stale
-- "gates have no bridge" claim is retracted — IsCasting is live
-- (lua_creature_events.go:1950). DoMeleeAttackIfReady is
-- engine-driven.
-- DamageTaken — heroic-only phase-shift machine: health
-- 60%..20% -> PHASE_FIRST_PLANAR (or < 20% ->
-- PHASE_SECOND_PLANAR), events.Reset() + Talk(SAY_SHIELD 5) +
-- DoCast(SPELL_PLANAR_SHIFT 51162) + summons.DespawnAll() +
-- DoCast(SPELL_PLANAR_ANOMALIES 57959) x6 — PORTED 2026-11-08
-- (tick 427, eregosDamageTaken below; dormant until the DBC
-- ships — see the handler note). creature:IsHeroic()
-- (lua_creature_events.go:3749) and creature:GetHealthPct()
-- (scripting/object.go:209) are bridged; the DespawnAll leg is
-- emulated with the Lua-tracked summons list (every summon, strip 7)
-- + DespawnOrUnsummon.
-- JustSummoned — BossAI::JustSummoned + NPC_PLANAR_ANOMALY
-- 30879: CombatStop + REACT_PASSIVE ARE bridged 2026-10-24
-- (lua_creature_events.go); the MoveRandom(100.0f) leg is PORTED
-- 2026-11-08 (tick 427) — creature:MoveRandom is bridged
-- (lua_creature_events.go:5467), retiring the "no bridge" note
-- (eregosJustSummoned, ev19, LATENT per the anubrekhan precedent:
-- anomalies are never delivered yet, so the arm fires only if a
-- future path summons via eregos). Strip 7: the BossAI::JustSummoned
-- leg is now FULLY ported — every summon is tracked in eregosSummons
-- (ScriptedCreature.cpp:559-561; the tick-427 table held anomalies
-- only, so C++ :196 DespawnAll would have missed ley whelps) and
-- non-anomaly summons are zoned into combat when the boss is engaged
-- (ScriptedCreature.cpp:562-563; IsInCombat + DoZoneInCombat bridged,
-- curator precedent). The spell-summon path DOES fire ev19
-- (creatureEffectSummonType routes through summonTempCreature,
-- boss_ai.go:4938/:5181), so the arm goes live with the DBC; the ev20
-- untrack now covers every summon (BossAI::SummonedCreatureDespawn,
-- ScriptedCreature.cpp:566-569). SummonedCreatureDespawn —
-- NPC_PLANAR_ANOMALY: CastSpell(summon, SPELL_PLANAR_BLAST
-- 57976, true) TRIGGERED self-cast (boss_eregos.cpp:177) — PORTED
-- 2026-11-08 (tick 427, eregosSummonedDespawn, ev20); event 20
-- (SummonedCreatureDespawn) IS bridged (runtime.go:230, fired on
-- timed-summon expiry in pet_combat.go and on TempSummon
-- DespawnOrUnsummon, the UnSummon leg). DORMANT: the anomaly
-- summons are DBC-side, so no hook fires until the DBC ships.
-- spell_eregos_planar_shift — AuraScript AfterEffectRemove
-- (EFFECT_0 SPELL_AURA_SCHOOL_IMMUNITY): target ToCreature
-- -> AI()->DoAction(ACTION_SET_NORMAL_EVENTS) (the
-- planar-shift end re-arms normal events) — PORTED 2026-11-08
-- (tick 427, planarShiftRemove, ev3).
-- achievement_gen_eregos_void x3 — OnCheck: target->GetAI()->
-- GetData(DATA_RUBY_VOID / DATA_EMERALD_VOID / DATA_AMBER_VOID)
-- (the drake-void achievements 2044 / 2045 / 2046) — PORTED tick 417:
-- the latches are mirrored into the boss motion's SetData store
-- (engage-time drake checks + Reset re-arm above) and read by the Go
-- gate via bossLatchData.

local ENTRY_EREGOS = 27656
local ENTRY_PLANAR_ANOMALY = 30879 -- boss_eregos.cpp:59

local SAY_AGGRO = 1
local SAY_KILL = 3
local SAY_DEATH = 4
local SAY_ENRAGE = 2
local SAY_SHIELD = 5 -- boss_eregos.cpp:44

local SPELL_ARCANE_BARRAGE = 50804
local SPELL_ARCANE_VOLLEY = 51153
local SPELL_ENRAGED_ASSAULT = 51170
local SPELL_PLANAR_SHIFT = 51162 -- boss_eregos.cpp:53
local SPELL_PLANAR_ANOMALIES = 57959 -- boss_eregos.cpp:52
local SPELL_PLANAR_BLAST = 57976 -- boss_eregos.cpp:56
local SPELL_SUMMON_LEY_WHELP = 51175 -- boss_eregos.cpp:54

local PHASE_NORMAL = 1 -- boss_eregos.cpp:62
local PHASE_FIRST_PLANAR = 2 -- boss_eregos.cpp:64
local PHASE_SECOND_PLANAR = 3 -- boss_eregos.cpp:65
local ACTION_SET_NORMAL_EVENTS = 1 -- boss_eregos.cpp:69

local NPC_RUBY_DRAKE_VEHICLE = 27756
local NPC_EMERALD_DRAKE_VEHICLE = 27692
local NPC_AMBER_DRAKE_VEHICLE = 27755

local DATA_EREGOS = 3 -- oculus.h:34 (OCDataTypes)
local DATA_RUBY_VOID = 0 -- boss_eregos.cpp:76
local DATA_EMERALD_VOID = 1 -- boss_eregos.cpp:77
local DATA_AMBER_VOID = 2 -- boss_eregos.cpp:78
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local timers = {}

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

-- C++ boss_eregosAI: the three achievement-void booleans are
-- latched true on Initialize() and only ever cleared on
-- JustEngagedWith; the Lua port re-seeds them true at the top of
-- every OnEnterCombat (Reset always precedes engage), kept
-- C++-exact in per-GUID tables.
local voidRuby = {}
local voidEmerald = {}
local voidAmber = {}
local eregosPhase = {} -- per-GUID _phase latch (Initialize PHASE_NORMAL)
local eregosSummons = {} -- per-GUID {summon GUID -> summon}: the BossAI
-- summons.Summon list twin (ScriptedCreature.cpp:559-561). EVERY summon
-- is tracked, not just planar anomalies: C++ DamageTaken
-- summons.DespawnAll() (boss_eregos.cpp:196) despawns the whole list,
-- including ley whelps from SPELL_SUMMON_LEY_WHELP — the old
-- anomaly-only table missed them (strip 7).

-- C++ EVENT_ARCANE_BARRAGE (:215-218): DoCastVictim(50804)
-- non-triggered, 3s/10s init -> 3s/10s loop.
local function arcaneBarrageTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "barrage", 500, function()
            arcaneBarrageTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_ARCANE_BARRAGE)
    schedule(guid, "barrage", {3000, 10000}, function()
        arcaneBarrageTick(creature, guid)
    end)
end

-- C++ EVENT_ARCANE_VOLLEY (:219-222): DoCastAOE(51153)
-- non-triggered, 10s/25s init -> 10s/25s loop.
local function arcaneVolleyTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "volley", 500, function()
            arcaneVolleyTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_ARCANE_VOLLEY)
    schedule(guid, "volley", {10000, 25000}, function()
        arcaneVolleyTick(creature, guid)
    end)
end

-- C++ EVENT_ENRAGED_ASSAULT (:223-227): Talk(SAY_ENRAGE) +
-- DoCast(51170) self-cast, non-triggered, 35s/50s init ->
-- 35s/50s loop.
local function enragedAssaultTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "enraged", 500, function()
            enragedAssaultTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_ENRAGE)
    creature:CastSpell(creature, SPELL_ENRAGED_ASSAULT)
    schedule(guid, "enraged", {35000, 50000}, function()
        enragedAssaultTick(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_LEY_WHELP (:228-231): 3x DoCast(SPELL_SUMMON_LEY_WHELP
-- 51175) self-cast, 15s/30s init -> 15s/30s loop. RESTORED 2026-11-08
-- (tick 427): the creature-cast path carries SPELL_EFFECT_SUMMON via
-- creatureEffectSummonType (boss_ai.go:3299, gated on the DBC Spell()
-- lookup), so the arm is dormant-but-exact until Spell.dbc ships —
-- the Oct-14 removal's "no SPELL_EFFECT_SUMMON leg" premise is
-- stale. The ley-whelp entry itself is DBC-side (no NPC_LEY_WHELP
-- in the C++ tree), so no explicit-summon twin is possible.
local function leyWhelpTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "leywhelp", 500, function()
            leyWhelpTick(creature, guid)
        end)
        return
    end
    for _ = 1, 3 do
        creature:CastSpell(creature, SPELL_SUMMON_LEY_WHELP)
    end
    schedule(guid, "leywhelp", {15000, 30000}, function()
        leyWhelpTick(creature, guid)
    end)
end

-- C++ DoAction(ACTION_SET_NORMAL_EVENTS) (boss_eregos.cpp:148-157):
-- events.SetPhase(PHASE_NORMAL) + the four normal-event schedules.
-- Shared by the engage seed and the planar-shift re-arm (below).
local function seedNormalEvents(creature, guid)
    schedule(guid, "barrage", {3000, 10000}, function()
        arcaneBarrageTick(creature, guid)
    end)
    schedule(guid, "volley", {10000, 25000}, function()
        arcaneVolleyTick(creature, guid)
    end)
    schedule(guid, "enraged", {35000, 50000}, function()
        enragedAssaultTick(creature, guid)
    end)
    schedule(guid, "leywhelp", {15000, 30000}, function()
        leyWhelpTick(creature, guid)
    end)
end

-- C++ boss_eregos.cpp:111-128 boss_eregosAI::JustEngagedWith:
-- Talk(SAY_AGGRO) unconditional + the drake-vehicle
-- FindNearestCreature achievement-void legs (:124-128; bridged —
-- lua_creature_events.go:1237), ported below.
local function eregosEnterCombat(event, creature, target)
    -- C++ JustEngagedWith base call first (boss_eregos.cpp:116-121):
    -- BossAI::JustEngagedWith arms (ScriptedCreature.cpp:537-542) --
    -- SetBossState(IN_PROGRESS) + SetCombatPulseDelay(5) + me-form
    -- DoZoneInCombat, before Talk (C++ arm order);
    -- instance/setActive/ScheduleTasks legs out.
    creature:SetBossState(DATA_EREGOS, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    voidRuby[guid] = true
    voidEmerald[guid] = true
    voidAmber[guid] = true
    eregosPhase[guid] = PHASE_NORMAL -- C++ Initialize() (:95-99)
    -- Achievement write leg (tick 417): mirror the void latches into
    -- the boss motion's SetData store for achievement_ruby_void /
    -- achievement_emerald_void / achievement_amber_void's GetData
    -- reads (Go gate via bossLatchData).
    creature:SetData(DATA_RUBY_VOID, 1)
    creature:SetData(DATA_EMERALD_VOID, 1)
    creature:SetData(DATA_AMBER_VOID, 1)
    if creature:FindNearestCreature(NPC_RUBY_DRAKE_VEHICLE, 500) then
        voidRuby[guid] = false
        creature:SetData(DATA_RUBY_VOID, 0)
    end
    if creature:FindNearestCreature(NPC_EMERALD_DRAKE_VEHICLE, 500) then
        voidEmerald[guid] = false
        creature:SetData(DATA_EMERALD_VOID, 0)
    end
    if creature:FindNearestCreature(NPC_AMBER_DRAKE_VEHICLE, 500) then
        voidAmber[guid] = false
        creature:SetData(DATA_AMBER_VOID, 0)
    end
    cancelTimers(guid)
    seedNormalEvents(creature, guid)
end

local function eregosCombatEnd(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset (boss_eregos.cpp:105-110): _Reset() FIRST —
-- SetBossState(NOT_STARTED), != DONE-gated (ScriptedCreature.cpp:510),
-- then Initialize() (phase/void booleans, reseeded on engage below)
-- + DoAction(ACTION_SET_NORMAL_EVENTS) (timer legs seed on engage
-- per the krystallus convention).
local function eregosReset(event, creature)
    if creature:GetBossState(DATA_EREGOS) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_EREGOS, BOSS_STATE_NOT_STARTED)
    end
    cancelTimers(creature:GetGUIDLow())
    eregosPhase[creature:GetGUIDLow()] = PHASE_NORMAL -- C++ Initialize() (:95-99)
    -- C++ Reset -> Initialize() (boss_eregos.cpp:95-99) re-arms all
    -- three void latches true. IsDead-guarded: ev23 pre-fires on death
    -- (kill.go:1091 -> fireCreatureDied) and an unguarded re-arm would
    -- overwrite the engage-time clears before the gate reads them.
    if not creature:IsDead() then
        creature:SetData(DATA_RUBY_VOID, 1)
        creature:SetData(DATA_EMERALD_VOID, 1)
        creature:SetData(DATA_AMBER_VOID, 1)
    end
end

-- C++ KilledUnit: gated on who->GetTypeId() == TYPEID_PLAYER —
-- the nalorakk / kelthuzad player-gate variant.
local function eregosTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) FIRST, then _JustDied()
-- (boss_eregos.cpp:247-251) — SetBossState(DONE)
-- (ScriptedCreature.cpp:519) follows the Talk here.
local function eregosDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_EREGOS, BOSS_STATE_DONE)
    cancelTimers(creature:GetGUIDLow())
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.eregosDone = true
        end
    end
end

-- C++ boss_eregosAI::DamageTaken (boss_eregos.cpp:184-202):
-- heroic-only; health 60%..20% -> PHASE_FIRST_PLANAR (or < 20% ->
-- PHASE_SECOND_PLANAR), events.Reset() + Talk(SAY_SHIELD 5) +
-- DoCast(SPELL_PLANAR_SHIFT 51162) + summons.DespawnAll() +
-- DoCast(SPELL_PLANAR_ANOMALIES 57959) x6. PORTED 2026-11-08
-- (tick 427). The phase latch is per-GUID (C++ _phase); health
-- reads are pre-damage on both sides (ev9 fires before damage is
-- applied), so no postDamage guard — the nefarian-form guard is
-- for UpdateAI HealthBelowPct arms, not DamageTaken.
-- summons.DespawnAll() is emulated with the Lua-tracked summons
-- (ev19, below; strip 7: every summon, not just anomalies) +
-- DespawnOrUnsummon, the vancleef precedent; each despawn fires ev20,
-- so each anomaly blasts exactly as C++ summons.DespawnAll() does. DORMANT: the 51162 self-cast and the
-- 57959 anomaly summons ride the DBC spell path (no Spell.dbc in
-- tree), so the shift aura never applies and no anomalies arrive
-- until the DBC ships; the re-arm below fires if the aura ever
-- expires.
local function eregosDamageTaken(event, creature, attacker, damage)
    if not creature:IsHeroic() then
        return
    end
    local guid = creature:GetGUIDLow()
    local phase = eregosPhase[guid] or PHASE_NORMAL
    local pct = creature:GetHealthPct()
    local first = pct < 60 and pct > 20 and phase < PHASE_FIRST_PLANAR
    local second = pct < 20 and phase < PHASE_SECOND_PLANAR
    if not first and not second then
        return
    end
    if first then
        eregosPhase[guid] = PHASE_FIRST_PLANAR
    else
        eregosPhase[guid] = PHASE_SECOND_PLANAR
    end
    cancelTimers(guid)
    creature:Talk(SAY_SHIELD)
    creature:CastSpell(creature, SPELL_PLANAR_SHIFT)
    local tracked = eregosSummons[guid]
    if tracked then
        for _, summon in pairs(tracked) do
            summon:DespawnOrUnsummon()
        end
        eregosSummons[guid] = nil
    end
    for _ = 1, 6 do
        creature:CastSpell(creature, SPELL_PLANAR_ANOMALIES)
    end
end

-- C++ spell_eregos_planar_shift::OnRemove (boss_eregos.cpp:274-282):
-- AfterEffectRemove, EFFECT_0 SPELL_AURA_SCHOOL_IMMUNITY. The
-- target (Eregos — the shift is a self-cast) re-arms the normal
-- events via AI()->DoAction(ACTION_SET_NORMAL_EVENTS). PORTED
-- 2026-11-08 (tick 427) via ev3; the DoAction re-seeds the four
-- normal timers on the target's GUID (seedNormalEvents, above).
-- DORMANT: the 51162 aura never applies until the DBC ships (see
-- eregosDamageTaken).
local function planarShiftRemove(event, aura, target)
    if target == nil then
        return
    end
    local guid = target:GetGUIDLow()
    cancelTimers(guid)
    seedNormalEvents(target, guid)
end

RegisterAuraEvent(SPELL_PLANAR_SHIFT, 3, planarShiftRemove)

-- C++ JustSummoned (boss_eregos.cpp:160-170): BossAI::JustSummoned
-- (ScriptedCreature.cpp:559-564) FIRST — summons.Summon(summon) +
-- DoZoneInCombat(summon) if the boss is engaged — then the anomaly
-- legs CombatStop(true) + SetReactState(REACT_PASSIVE) +
-- MoveRandom(100.0f) in C++ order. The anomaly legs are PORTED
-- 2026-11-08 (tick 427); strip 7 completes the BossAI leg: EVERY
-- summon is tracked in eregosSummons for the DespawnAll emulation in
-- eregosDamageTaken (above), and non-anomaly summons are zoned into
-- combat when the boss is engaged (IsInCombat + DoZoneInCombat are
-- bridged; curator precedent for summon:DoZoneInCombat()). The zone
-- leg is moot for anomalies (CombatStop clears it — the Go CombatStop
-- is RemoveMeFromThreatLists + ClearAllThreat + InCombat=false,
-- lua_creature_events.go:3168). creature:MoveRandom is bridged
-- (engine/world/lua_creature_events.go:5467). LATENT: summons arrive
-- only from DBC-side spell-summons (SPELL_SUMMON_LEY_WHELP 51175 /
-- SPELL_PLANAR_ANOMALIES 57959; anubrekhan latent precedent) — the
-- spell-summon path fires ev19 via summonTempCreature
-- (boss_ai.go:4938/:5181), so the arm goes live with the DBC.
local function eregosJustSummoned(_, creature, summoned)
    local guid = creature:GetGUIDLow()
    local tracked = eregosSummons[guid]
    if not tracked then
        tracked = {}
        eregosSummons[guid] = tracked
    end
    tracked[summoned:GetGUIDLow()] = summoned
    if summoned:GetEntry() == ENTRY_PLANAR_ANOMALY then
        summoned:CombatStop(true)
        summoned:SetReactState(0) -- REACT_PASSIVE
        summoned:MoveRandom(100)
    elseif creature:IsInCombat() then
        summoned:DoZoneInCombat()
    end
end

-- C++ SummonedCreatureDespawn (boss_eregos.cpp:173-179):
-- NPC_PLANAR_ANOMALY: triggered self-cast SPELL_PLANAR_BLAST
-- 57976 (the C++ @todo "See why the spell is not cast" notes the
-- cast itself; the hook is exact). PORTED 2026-11-08 (tick 427);
-- strip 7: untracks EVERY summon (BossAI::SummonedCreatureDespawn,
-- ScriptedCreature.cpp:566-569, summons.Despawn); the blast stays
-- anomaly-gated. DespawnOrUnsummon on a TempSummon fires ev20 (the
-- UnSummon leg), so the DamageTaken DespawnAll above blasts each
-- anomaly exactly as C++ summons.DespawnAll() does. DORMANT:
-- anomalies never arrive until the DBC ships.
local function eregosSummonedDespawn(_, creature, summoned)
    if summoned == nil then
        return
    end
    local guid = creature:GetGUIDLow()
    local tracked = eregosSummons[guid]
    if tracked then
        tracked[summoned:GetGUIDLow()] = nil
    end
    if summoned:GetEntry() == ENTRY_PLANAR_ANOMALY then
        summoned:CastSpell(summoned, SPELL_PLANAR_BLAST, true)
    end
end

RegisterCreatureEvent(ENTRY_EREGOS, 1, eregosEnterCombat)
RegisterCreatureEvent(ENTRY_EREGOS, 2, eregosCombatEnd)
RegisterCreatureEvent(ENTRY_EREGOS, 3, eregosTargetDied)
RegisterCreatureEvent(ENTRY_EREGOS, 4, eregosDied)
RegisterCreatureEvent(ENTRY_EREGOS, 9, eregosDamageTaken)
RegisterCreatureEvent(ENTRY_EREGOS, 23, eregosReset)
RegisterCreatureEvent(ENTRY_EREGOS, 19, eregosJustSummoned)
RegisterCreatureEvent(ENTRY_EREGOS, 20, eregosSummonedDespawn)
