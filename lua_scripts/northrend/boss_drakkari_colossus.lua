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
-- FREEZE) and, once the elemental/mojo senders land, by them
-- (UNFREEZE): SUMMON_ELEMENTAL self-casts Emerge 54850; FREEZE
-- MoveIdle (the Clear + MoveIdle collapse twin) + REACT_PASSIVE +
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
-- boss_drakkari_elemental — ENTRY UNVERIFIABLE (gundrak.h
-- GDCreatureIds has no NPC_DRAKKARI_ELEMENTAL; the .cpp enum names
-- only spells/texts; belnistrasz/willix precedent — no invented
-- identifiers): zero registration. Port-pattern-ready but
-- entry-blocked arms: EVENT_SURGE (5-15s) — DoCastSelf(Surge Visual
-- 54827) + randomPlayer-targeted DoCast(Surge 54801); Reset's
-- AddAura(Mojo Volley 54849, self); JustDied Talk(EMOTE_ACTIVATE_ALTAR 1)
-- + Unit::Kill(colossus) via GetInstanceCreature; DamageTaken
-- HealthBelowPct(50) -> cross-AI GetData(DATA_COLOSSUS_PHASE) +
-- DoAction(ACTION_RETURN_TO_COLOSSUS) (ev38 dispatch IS bridged;
-- the :313-321 MovePoint leg is commented out in C++ — correctly
-- not ported); EnterEvadeMode DespawnOrUnsummon via ev2 +
-- creature:Despawn; SpellHitTarget MERGE (ev15) -> colossus
-- UNFREEZE + self-despawn. Ctor Spawn Effect 54888 stays out on
-- first spawn (ev5 respawn-only). Joins the bridgeable-but-entry-
-- blocked queue.
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
local DATA_DRAKKARI_COLOSSUS = 1 -- gundrak.h DATA_DRAKKARI_COLOSSUS
local BOSS_STATE_DONE = 3 -- EncounterState DONE (InstanceScript.h:68-76)

local SPELL_MIGHTY_BLOW = 54719
local SPELL_FREEZE_ANIM = 16245
local SPELL_EMERGE = 54850

local DATA_COLOSSUS_PHASE = 1
local DATA_INTRO_DONE = 2

local COLOSSUS_PHASE_NORMAL = 1
local COLOSSUS_PHASE_FIRST_ELEMENTAL_SUMMON = 2
local COLOSSUS_PHASE_SECOND_ELEMENTAL_SUMMON = 3

local ACTION_SUMMON_ELEMENTAL = 1
local ACTION_FREEZE_COLOSSUS = 2
local ACTION_UNFREEZE_COLOSSUS = 3

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100

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
    local guid = creature:GetGUID()
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
    cancelTimers(creature:GetGUID())
end

local function colossusDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
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
    cancelTimers(creature:GetGUID())
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
-- the colossus's own DamageTaken above plus the entry-blocked
-- elemental SpellHitTarget / mojo MovementInform chains.
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

RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 1, colossusEnterCombat)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 2, colossusLeaveCombat)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 4, colossusDied)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 9, colossusDamageTaken)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 23, colossusReset)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 38, colossusDoAction)
RegisterCreatureEvent(ENTRY_DRAKKARI_COLOSSUS, 19, colossusJustSummoned)
