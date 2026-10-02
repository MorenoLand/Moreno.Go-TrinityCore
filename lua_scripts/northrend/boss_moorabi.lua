-- Moorabi (Gundrak) — Lua port of
-- src/server/scripts/Northrend/Gundrak/boss_moorabi.cpp
-- (boss_moorabi + achievement_less_rabi + spell_moorabi_mojo_frenzy).
-- Gundrak dungeon-script unit per northrend_script_loader.cpp order
-- (decl 21 / call 216, immediately after AddSC_boss_slad_ran();
-- next: boss_drakkari_colossus).
-- Entry: 29305 Moorabi (gundrak.h NPC_MOORABI — the
-- RegisterCreatureAIWithFactory(GetGundrakAI) ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell[, triggered]) = DoCastSelf
-- (phase_hunter / apothecary_hanes precedent — the trailing true is
-- the triggered port).
-- Ported arms (C++-exact for all modeled arms):
-- boss_moorabi: JustEngagedWith Talk(SAY_AGGRO 0) + DoCastSelf(Mojo
-- Frenzy 55163, triggered); EVENT_TRANFORMATION (12s init): Talk
-- (EMOTE_BEGIN_TRANSFORM 5) + Talk(SAY_TRANSFORM 3) +
-- DoCastSelf(Transformation 55098) + DoCastSelf(Summon Phantom
-- Transform 55097, triggered) — single-fire, no reschedule: C++
-- SpellHit(55098) cancels EVENT_TRANFORMATION on the transformation
-- landing (event 15 never fires on the Lua surface — standing), so
-- one transform cast is the C++-exact observable behavior; the
-- _transformed flag is emulation-internal and has no other modeled
-- reader. KilledUnit Talk(SAY_SLAY 1) player-gated (event 3,
-- victim:GetObjectType() == "Player" — nalorakk precedent);
-- JustDied Talk(EMOTE_ACTIVATE_ALTAR 7) + Talk(SAY_DEATH 2)
-- (event 4).
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_GROUND_TREMOR / EVENT_NUMBLING_SHOUT / EVENT_DETERMINED_STAB
-- — all DoCastAOE (Ground Tremor 55142 / Quake 55101, Numbing Shout
-- 55106 / Numbing Roar 55100, Determined Stab 55104 / Determined
-- Gore 55102): no DoCastAOE bridge on the Lua surface (zero hits —
-- terestian / shazzrah precedent); the _transformed variant
-- selection is moot with the casts unbridged.
-- EVENT_PHANTOM (Reset-scheduled 21s -> 20-25s, PHASE_INTRO only) —
-- no phase bridge (SetPhase/IsInPhase zero hits on the Lua surface)
-- and no faithful pre-pull Reset trigger, so the pre-pull summon
-- cast is not emulated.
-- EnterEvadeMode _DespawnAtEvade — no despawn bridge (terestian
-- precedent).
-- SpellHit(55098) _transformed latch — event 15 never fires
-- (standing); GetData(DATA_LESS_RABI) has no cross-AI bridge (zero
-- hits).
-- achievement_less_rabi (AchievementCriteriaScript, CAST_AI cross-AI
-- OnCheck) — no achievement-criteria bridge on the Lua surface
-- (snakes precedent) — documented-only.
-- spell_moorabi_mojo_frenzy (AuraScript periodic dummy: cast-speed
-- bonus = (100 - healthPct) * 4 via SPELL_MOJO_FRENZY_CAST_SPEED
-- 55096) — no AuraScript binding bridge (standing) — documented-only.
-- OnLeaveCombat(2)/OnReset(23) cancel the scheduler (gargolmar
-- precedent); BossAI _Reset/_JustDied instance bookkeeping has no
-- bridge.

local ENTRY_MOORABI = 29305

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_TRANSFORM = 3
local SAY_QUAKE = 4
local EMOTE_BEGIN_TRANSFORM = 5
local EMOTE_TRANSFORMED = 6
local EMOTE_ACTIVATE_ALTAR = 7

local SPELL_MOJO_FRENZY = 55163
local SPELL_TRANSFORMATION = 55098
local SPELL_SUMMON_PHANTOM_TRANSFORM = 55097

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

-- C++ EVENT_TRANFORMATION: 12s init, Talk(EMOTE_BEGIN_TRANSFORM) +
-- Talk(SAY_TRANSFORM) + DoCastSelf(TRANSFORMATION) +
-- DoCastSelf(SUMMON_PHANTOM_TRANSFORM, triggered). Single-fire: the
-- C++ SpellHit(55098) leg cancels the event once the transformation
-- lands (see header), so no reschedule is the C++-exact behavior.
local function transformTick(creature, guid)
    timers[guid] = nil
    creature:Talk(EMOTE_BEGIN_TRANSFORM)
    creature:Talk(SAY_TRANSFORM)
    creature:CastSpell(creature, SPELL_TRANSFORMATION)
    creature:CastSpell(creature, SPELL_SUMMON_PHANTOM_TRANSFORM, true)
end

local function moorabiEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_MOJO_FRENZY, true)
    schedule(guid, "transform", 12000, function()
        transformTick(creature, guid)
    end)
end

local function moorabiLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function moorabiTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function moorabiDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(EMOTE_ACTIVATE_ALTAR)
    creature:Talk(SAY_DEATH)
end

local function moorabiReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MOORABI, 1, moorabiEnterCombat)
RegisterCreatureEvent(ENTRY_MOORABI, 2, moorabiLeaveCombat)
RegisterCreatureEvent(ENTRY_MOORABI, 3, moorabiTargetDied)
RegisterCreatureEvent(ENTRY_MOORABI, 4, moorabiDied)
RegisterCreatureEvent(ENTRY_MOORABI, 23, moorabiReset)
