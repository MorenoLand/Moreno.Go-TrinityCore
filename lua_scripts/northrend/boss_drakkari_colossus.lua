-- Drakkari Colossus (Gundrak) — Lua port of
-- src/server/scripts/Northrend/Gundrak/boss_drakkari_colossus.cpp
-- (boss_drakkari_colossus + boss_drakkari_elemental + npc_living_mojo).
-- Gundrak dungeon-script unit per northrend_script_loader.cpp order
-- (decl 22 / call 217, immediately after AddSC_boss_moorabi();
-- next: boss_gal_darah).
-- Entry: 29307 Drakkari Colossus (gundrak.h NPC_DRAKKARI_COLOSSUS —
-- the RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName binding
-- is instance-shimmed, the creature_template binding DB-side as
-- usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 9 OnDamageTaken, 19 OnSummoned, 23 OnReset, 38 OnDoAction.
-- Timers via CreateLuaEvent; melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast default is
-- triggered=false (vaelastrasz convention): creature:CastSpell(nil,
-- spell) = DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell[, triggered]) = DoCastSelf
-- (phase_hunter / apothecary_hanes precedent — the trailing true is
-- the triggered port).
-- Ported arms (C++-exact for all modeled arms):
-- boss_drakkari_colossus: EVENT_MIGHTY_BLOW — DoCastVictim(Mighty
-- Blow 54719); Reset-scheduled 10-30s init, rescheduled 5-15s
-- (scheduled on enterCombat in Lua — UpdateAI needs a victim either
-- way, so the first tick lands 10-30s after pull, not after spawn).
-- JustEngagedWith — me->RemoveAura(SPELL_FREEZE_ANIM 16245) (:119)
-- rides the creature:RemoveAura bridge (the saurfang/arlokk idiom);
-- the Freeze Anim aura is cast only by the DoAction freeze machine
-- below, so pre-freeze the removal is a faithful no-op.
-- DamageTaken phase machine (:166-181) — ev9 with the 2nd-return
-- damage rewrite (algalon/anubarak precedent): frozen (immune-to-PC
-- flag set) nulls all damage; phase NORMAL/FIRST + HealthBelowPct
-- (50/5, strict-< against the floored integer threshold — the
-- slad_ran precedent) advances the phase and fires DoAction FREEZE
-- + SUMMON_ELEMENTAL with damage 0. DamageTaken arms fire on lethal
-- hits (no post-damage guard — yazzai/nefarian convention), so the
-- 0-rewrite saves the boss exactly as C++ does. Phase lives in the
-- motion SetData store (type 1); Reset re-inits it to NORMAL, the
-- C++ Initialize() twin.
-- DoAction machine (:128-163) — ev38 (illidan/magtheridon precedent),
-- fired by the colossus's own DamageTaken above (SUMMON_ELEMENTAL /
-- FREEZE) and, once the mojo sender lands, by it (UNFREEZE) — plus the
-- elemental's SpellHitTarget MERGE leg, ported below (ev15).
-- SUMMON_ELEMENTAL self-casts Emerge 54850; FREEZE MoveIdle (the
-- Clear + MoveIdle collapse twin) + REACT_PASSIVE +
-- SetFlag(59, 0x100) UNIT_FLAG_IMMUNE_TO_PC (the
-- SetImmuneToPC(true) twin — algalon/BPC/saurfang precedent) +
-- self-cast Freeze Anim 16245; UNFREEZE returns early while
-- REACT_AGGRESSIVE, else clears the flag, sets REACT_AGGRESSIVE,
-- removes Freeze Anim, DoZoneInCombat.
-- Reset INTRO_DONE leg (:101-114) — GetData(2) gate + REACT_AGGRESSIVE
-- + flag clear + RemoveAura(16245); the introDone latch itself is
-- written only by the entry-blocked mojo MovementInform chain, so
-- the leg is dormant until mojos land.
-- JustSummoned (:218-231) — DoZoneInCombat(summon) unconditionally
-- plus the phase-SECOND half-health leg (summon:SetHealth(max/2)).
-- OnLeaveCombat(2) cancels the scheduler only: the colossus has NO
-- EnterEvadeMode override in C++ (BossAI evade resets in place) —
-- the old creature:Despawn() here cited the ELEMENTAL's evade
-- (:328-331), not the colossus's, and wrongly vanished the boss on
-- every evade. Fixed tick 505.
-- Unmodeled (no bridges — documented, not wired):
-- Constructor SetReactState(REACT_PASSIVE) (:90) — the AI
-- constructor has no Lua lifecycle equivalent (no ctor hook).
-- JustDied — _JustDied() DONE ports in colossusDied below
-- (SetBossState + instance flag); the _Reset half has no bridge.
-- boss_drakkari_elemental — ENTRY VERIFIED (29573, DB creature_template
-- ScriptName 'boss_drakkari_elemental' in both AC and TDB; the strip-15
-- "ENTRY UNVERIFIABLE" verdict is retracted — gundrak.h has no NPC
-- constant, but the DB row is binding evidence per the hook-coverage
-- rule). PORTED (follow-up strip 1, unit B): EVENT_SURGE (5-15s) —
-- DoCastSelf(Surge Visual 54827) + random-player DoCast(Surge 54801),
-- skipping the top-threat player (C++ SelectTarget(Random, 1) offset —
-- see surgeTick; Go GetAITarget(0, true, 1) would pick among the first
-- two instead, so the GetAITargets idiom is used); Reset's AddAura(Mojo
-- Volley 54849, self) on ev23 (fires at summon time — fireCreatureSpawned);
-- JustDied Talk(EMOTE_ACTIVATE_ALTAR 1) + Unit::Kill(colossus) via
-- GetInstanceCreature (colossus GUID latched by the new ev5 colossusSpawn
-- below — without it both cross-AI arms nil-guarded to no-ops);
-- DamageTaken HealthBelowPct(50) -> cross-AI GetData(DATA_COLOSSUS_PHASE)
-- + DoAction(ACTION_RETURN_TO_COLOSSUS) with the damage-0 rewrite before
-- the UNIT_STATE_CHARGING gate (the :313-321 MovePoint leg is commented
-- out in C++ — correctly not ported); EnterEvadeMode DespawnOrUnsummon
-- via ev2 + creature:Despawn (opera precedent). SpellHitTarget MERGE (ev15)
-- was already ported. Ctor Spawn Effect 54888 stays out (no ctor hook;
-- the old "ev5 respawn-only" rationale is stale — motionFor fires ev5 on
-- first spawn too — but the ctor arm itself was out of this unit's scope).
-- npc_living_mojo — ENTRY UNVERIFIABLE (same ground as the
-- elemental): zero registration. Port-pattern-ready but entry-blocked
-- arms: JustEngagedWith DoCastVictim(Mojo Wave 55626, 2s init, 15s
-- repeat) + DoCastVictim(Mojo Puddle 55627, 7s init, 18s repeat);
-- MoveMojos via GetCreaturesInRange + GetHomePosition + MoveTo(1,
-- home); MovementInform id-1 (ev28 POINT) -> colossus UNFREEZE +
-- SetData(DATA_INTRO_DONE) + DoZoneInCombat + self-despawn — the
-- intro chain the colossus INTRO_DONE leg depends on. STAY-OUT as a
-- hook: AttackStart home-proximity passive flip (no AttackStart
-- event in runtime.go; no GetCurrentMovementGeneratorType bridge).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent); BossAI _Reset/_JustDied instance
-- bookkeeping has no bridge.

local ENTRY_DRAKKARI_COLOSSUS = 29307
local ENTRY_DRAKKARI_ELEMENTAL = 29573 -- DB-verified: AC + TDB creature_template carry ScriptName 'boss_drakkari_elemental' on 'Drakkari Elemental' (strip-15 entry block lifted)
local DATA_DRAKKARI_COLOSSUS = 1 -- gundrak.h DATA_DRAKKARI_COLOSSUS
local BOSS_STATE_DONE = 3 -- EncounterState DONE (InstanceScript.h:68-76)

local SPELL_MIGHTY_BLOW = 54719
local SPELL_FREEZE_ANIM = 16245
local SPELL_EMERGE = 54850
local SPELL_MERGE = 54878 -- boss_drakkari_colossus.cpp:42
local SPELL_MOJO_VOLLEY = 54849
local SPELL_SURGE_VISUAL = 54827
local SPELL_SURGE = 54801

local DATA_COLOSSUS_PHASE = 1
local DATA_INTRO_DONE = 2

local COLOSSUS_PHASE_NORMAL = 1
local COLOSSUS_PHASE_FIRST_ELEMENTAL_SUMMON = 2
local COLOSSUS_PHASE_SECOND_ELEMENTAL_SUMMON = 3

local ACTION_SUMMON_ELEMENTAL = 1
local ACTION_FREEZE_COLOSSUS = 2
local ACTION_UNFREEZE_COLOSSUS = 3
local ACTION_RETURN_TO_COLOSSUS = 1 -- elemental's own enum (boss_drakkari_colossus.cpp:74-77)

local EMOTE_MOJO = 0
local EMOTE_ACTIVATE_ALTAR = 1
local UNIT_STATE_CHARGING = 0x00020000 -- Unit.h:228

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100

-- GUID-key rule: per-GUID tables are keyed on GetGUIDLow() (stable
-- number). GetGUID() returns a fresh UInt64 userdata per call and
-- Lua table keys compare userdata by identity, so a key stored
-- under one call never matches a lookup from another
-- (engine/scripting/object.go pushUInt64; instance_gundrak.lua
-- GUID-KEY FIX).
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

-- C++ EVENT_MIGHTY_BLOW: Reset-scheduled 10-30s init, reschedule
-- 5-15s; DoCastVictim. C++ UpdateAI pre-loop + in-loop CASTING gates
-- leave the event queued while casting — 500ms defer, no 5-15s
-- re-arm (mal_ganis idiom).
local function mightyBlowTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "mightyblow", 500, function()
            mightyBlowTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_MIGHTY_BLOW)
    schedule(guid, "mightyblow", math.random(5000, 15000), function()
        mightyBlowTick(creature, guid)
    end)
end

-- C++ :115-119 JustEngagedWith: BossAI base arms first, then
-- me->RemoveAura(SPELL_FREEZE_ANIM) via the RemoveAura bridge.
local function colossusEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:RemoveAura(SPELL_FREEZE_ANIM)
    schedule(guid, "mightyblow", math.random(10000, 30000), function()
        mightyBlowTick(creature, guid)
    end)
end

-- C++: the colossus has NO EnterEvadeMode override — BossAI evade
-- resets in place (Reset). Cancel timers only; do NOT despawn (the
-- old Despawn() cited the elemental's evade arm, :328-331).
local function colossusLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function colossusDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    -- C++ BossAI::_JustDied (ScriptedCreature.cpp:513-520):
    -- instance->SetBossState(DATA_DRAKKARI_COLOSSUS, DONE). The
    -- instance script's altar-clear leg reads the flag below (no
    -- boss-state-change event exists — instance_gundrak.lua header).
    creature:SetBossState(DATA_DRAKKARI_COLOSSUS, BOSS_STATE_DONE)
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data.drakkariDone = true
    end
end

-- C++ Reset (:98-114): _Reset(); INTRO_DONE leg (SetReactState
-- AGGRESSIVE + SetImmuneToPC(false) + RemoveAura Freeze Anim);
-- EVENT_MIGHTY_BLOW schedule (rides enterCombat in Lua, see above);
-- Initialize() resets the phase to NORMAL.
local function colossusReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:SetData(DATA_COLOSSUS_PHASE, COLOSSUS_PHASE_NORMAL)
    if creature:GetData(DATA_INTRO_DONE) ~= 0 then
        creature:SetReactState(REACT_AGGRESSIVE)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:RemoveAura(SPELL_FREEZE_ANIM)
    end
end

-- C++ DamageTaken (:166-181): frozen nulls damage; phase NORMAL /
-- FIRST + HealthBelowPct(50 / 5) advances the phase and fires
-- FREEZE + SUMMON_ELEMENTAL with damage 0. ev9 2nd return rewrites
-- damage (algalon/anubarak precedent); no post-damage guard — this
-- arm IS DamageTaken, so it fires on lethal hits exactly as C++.
local function colossusDamageTaken(event, creature, attacker, damage)
    if creature:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC) then
        return false, 0
    end
    local phase = creature:GetData(DATA_COLOSSUS_PHASE)
    if phase == 0 then
        phase = COLOSSUS_PHASE_NORMAL
    end
    if phase == COLOSSUS_PHASE_NORMAL or phase == COLOSSUS_PHASE_FIRST_ELEMENTAL_SUMMON then
        local pct = 50
        if phase == COLOSSUS_PHASE_FIRST_ELEMENTAL_SUMMON then
            pct = 5
        end
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0 and creature:GetHealth() < math.floor(maxHealth * pct / 100) then
            local nextPhase = COLOSSUS_PHASE_FIRST_ELEMENTAL_SUMMON
            if phase == COLOSSUS_PHASE_FIRST_ELEMENTAL_SUMMON then
                nextPhase = COLOSSUS_PHASE_SECOND_ELEMENTAL_SUMMON
            end
            creature:SetData(DATA_COLOSSUS_PHASE, nextPhase)
            creature:DoAction(ACTION_FREEZE_COLOSSUS)
            creature:DoAction(ACTION_SUMMON_ELEMENTAL)
            return false, 0
        end
    end
end

-- C++ DoAction (:128-163): SUMMON_ELEMENTAL self-casts Emerge;
-- FREEZE clears motion + MoveIdle + REACT_PASSIVE + SetImmuneToPC +
-- self-cast Freeze Anim; UNFREEZE returns early while AGGRESSIVE,
-- else clears the flag, sets AGGRESSIVE, removes Freeze Anim,
-- DoZoneInCombat. ev38 (illidan/magtheridon precedent); senders are
-- the colossus's own DamageTaken above, the elemental's SpellHitTarget
-- MERGE leg (ev15, ported below), and the mojo MovementInform chain
-- (still entry-blocked).
local function colossusDoAction(event, creature, action)
    if action == ACTION_SUMMON_ELEMENTAL then
        creature:CastSpell(creature, SPELL_EMERGE)
    elseif action == ACTION_FREEZE_COLOSSUS then
        creature:MoveIdle()
        creature:SetReactState(REACT_PASSIVE)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:CastSpell(creature, SPELL_FREEZE_ANIM)
    elseif action == ACTION_UNFREEZE_COLOSSUS then
        if creature:GetReactState() == REACT_AGGRESSIVE then
            return
        end
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:SetReactState(REACT_AGGRESSIVE)
        creature:RemoveAura(SPELL_FREEZE_ANIM)
        creature:DoZoneInCombat()
    end
end

-- C++ JustSummoned (boss_drakkari_colossus.cpp:218-231): DoZoneInCombat(summon)
-- unconditionally; phase SECOND halves the summon's health.
local function colossusJustSummoned(_, creature, summoned)
    summoned:DoZoneInCombat()
    if creature:GetData(DATA_COLOSSUS_PHASE) == COLOSSUS_PHASE_SECOND_ELEMENTAL_SUMMON then
        summoned:SetHealth(math.floor(summoned:GetMaxHealth() / 2))
    end
end

-- C++ boss_drakkari_elemental::SpellHitTarget (:333-342): if the
-- spell is MERGE 54878 and the target is a creature, that creature's
-- AI gets DoAction(ACTION_UNFREEZE_COLOSSUS) and the elemental
-- despawns. ev15 (boss_ai.go:4509 fires CreatureEventOnSpellHitTarget;
-- onyxia/nazan handler idiom (event, creature, target, spellId)).
-- The GetTypeId() == 3 check is the C++ target->ToCreature() null
-- test (vx_001 precedent); target:DoAction rides the bridged ev38
-- dispatch straight into colossusDoAction's UNFREEZE leg above, and
-- creature:Despawn is DespawnOrUnsummon (lua_creature_events.go:3305).
-- MERGE itself is cast only by the elemental's DoAction
-- RETURN_TO_COLOSSUS (:288-296), fired from its DamageTaken <50% arm
-- (:299-326) — both ported this tick (elementalDoAction /
-- elementalDamageTaken below), so this handler is live end-to-end.
local function elementalSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_MERGE then
        return
    end
    if not target or target:GetTypeId() ~= 3 then -- TYPEID_UNIT
        return
    end
    target:DoAction(ACTION_UNFREEZE_COLOSSUS)
    creature:Despawn()
end

-- C++ boss_drakkari_elemental::Reset (:239-245): events.Reset() (cancel
-- timers) + AddAura(Mojo Volley 54849, self). ev23 fires at summon time
-- (fireCreatureSpawned; temp summons ride motionFor via summonTempCreature),
-- so the aura applies on spawn, C++-exact (Creature.cpp:2198 ai->Reset()).
-- The EVENT_SURGE schedule from the same Reset rides enterCombat below
-- (mighty-blow precedent — UpdateAI needs a victim either way); evade
-- despawns the elemental, so no Reset re-schedule is needed.
local function elementalReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:AddAura(SPELL_MOJO_VOLLEY)
end

-- C++ EVENT_SURGE (:269-274): DoCastSelf(Surge Visual 54827), then a
-- random player target (SelectTarget(Random, 1, 0.0f, true) — offset 1
-- skips the first MaxThreat entry, uniform random among the remaining
-- players; nil when <=1 player) takes DoCast(Surge 54801); reschedule
-- 5-15s. The CASTING gate (:262, :277) defers 500ms (mal_ganis idiom).
-- GetAITargets is the threat-ordered list (lua_creature_events.go:4495);
-- Go's GetAITarget(0, true, 1, 0) would pick among the FIRST TWO
-- (creaturemotion.go:2505-2510), not skip-then-random, so the midnight
-- randomTargetAnywhere idiom is used instead (player + alive filter).
local function surgeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "surge", 500, function()
            surgeTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SURGE_VISUAL)
    local players = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead() then
            players[#players + 1] = t
        end
    end
    if #players > 1 then
        local target = players[math.random(2, #players)]
        creature:CastSpell(target, SPELL_SURGE)
    end
    schedule(guid, "surge", math.random(5000, 15000), function()
        surgeTick(creature, guid)
    end)
end

-- C++ Reset schedules EVENT_SURGE 5-15s; in Lua the pump starts on
-- enterCombat (the colossus JustSummoned already DoZoneInCombats the
-- elemental, so ev1 fires at summon time). No SetCombatPulseDelay:
-- that is a BossAI::_JustEngagedWith arm (ScriptedCreature.cpp:540)
-- and the elemental is ScriptedAI.
local function elementalEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "surge", math.random(5000, 15000), function()
        surgeTick(creature, guid)
    end)
end

-- C++ EnterEvadeMode (:328-331): DespawnOrUnsummon. ev2 is Go's
-- EnterEvadeMode/OnLeaveCombat hook (triggerCreatureEvade fires ev23
-- then ev2; opera precedent despawns summoned adds in ev2).
local function elementalLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:Despawn()
end

-- C++ JustDied (:247-253): Talk(EMOTE_ACTIVATE_ALTAR 1), then
-- Unit::Kill(killer, colossus) via GetInstanceCreature. colossus:Kill
-- fires the colossus's ev4, so colossusDied sets boss state DONE —
-- the C++-exact chain (kalecgos creature:Kill(caster) idiom).
local function elementalDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    creature:Talk(EMOTE_ACTIVATE_ALTAR)
    local colossus = creature:GetInstanceCreature(DATA_DRAKKARI_COLOSSUS)
    if colossus then
        if killer then
            colossus:Kill(killer)
        else
            colossus:Kill()
        end
    end
end

-- C++ DamageTaken (:298-326): HealthBelowPct(50) + colossus phase FIRST
-- -> damage 0, then (unless UNIT_STATE_CHARGING) DoAction
-- RETURN_TO_COLOSSUS. The damage-0 rewrite precedes the charging check
-- in C++, so both sub-branches return false, 0. NO post-damage guard:
-- DamageTaken arms fire on lethal hits (nefarian/yazzai convention).
-- CHARGING has no live model in Go (lua_unit_state.go — mask-only) but
-- the elemental never charges in C++ either (its only movement leg is
-- the commented-out MovePoint), so the gate reads false in both.
local function elementalDamageTaken(event, creature, attacker, damage)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0 and creature:GetHealth() < math.floor(maxHealth * 50 / 100) then
        local colossus = creature:GetInstanceCreature(DATA_DRAKKARI_COLOSSUS)
        if colossus and colossus:GetData(DATA_COLOSSUS_PHASE) == COLOSSUS_PHASE_FIRST_ELEMENTAL_SUMMON then
            if creature:HasUnitState(UNIT_STATE_CHARGING) then
                return false, 0
            end
            creature:DoAction(ACTION_RETURN_TO_COLOSSUS)
            return false, 0
        end
    end
end

-- C++ DoAction RETURN_TO_COLOSSUS (:284-296): Talk(EMOTE_MOJO 0),
-- DoCastSelf(Surge Visual 54827), DoCast(colossus, MERGE 54878, true).
-- The MERGE cast lands in elementalSpellHitTarget (ev15) above, which
-- unfreezes the colossus and despawns the elemental.
local function elementalDoAction(event, creature, action)
    if action ~= ACTION_RETURN_TO_COLOSSUS then
        return
    end
    creature:Talk(EMOTE_MOJO)
    creature:CastSpell(creature, SPELL_SURGE_VISUAL)
    local colossus = creature:GetInstanceCreature(DATA_DRAKKARI_COLOSSUS)
    if colossus then
        creature:CastSpell(colossus, SPELL_MERGE, true)
    end
end

-- Prerequisite latch for the elemental's cross-AI arms: the instance guid
-- store only resolves GetInstanceCreature(1) if the colossus latched its
-- GUID (instance_guid_store.go). ev5 ON_SPAWN fires on first spawn too
-- (motionFor, creaturemotion.go:1272-1279) — illidari-council /
-- magtheridon SetGuidData idiom.
local function colossusSpawn(event, creature)
    creature:SetGuidData(DATA_DRAKKARI_COLOSSUS)
end

RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 1, colossusEnterCombat)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 2, colossusLeaveCombat)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 4, colossusDied)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 9, colossusDamageTaken)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 23, colossusReset)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 38, colossusDoAction)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 19, colossusJustSummoned)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 5, colossusSpawn)
RegisterCreatureEvent(ENTRY_DRAKKARI_ELEMENTAL, 15, elementalSpellHitTarget)
RegisterCreatureEvent(ENTRY_DRAKKARI_ELEMENTAL, 1, elementalEnterCombat)
RegisterCreatureEvent(ENTRY_DRAKKARI_ELEMENTAL, 2, elementalLeaveCombat)
RegisterCreatureEvent(ENTRY_DRAKKARI_ELEMENTAL, 4, elementalDied)
RegisterCreatureEvent(ENTRY_DRAKKARI_ELEMENTAL, 9, elementalDamageTaken)
RegisterCreatureEvent(ENTRY_DRAKKARI_ELEMENTAL, 23, elementalReset)
RegisterCreatureEvent(ENTRY_DRAKKARI_ELEMENTAL, 38, elementalDoAction)
