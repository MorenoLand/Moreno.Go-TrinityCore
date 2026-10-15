-- Emerald Dragons (world bosses) — Lua port of
-- src/server/scripts/World/boss_emerald_dragons.cpp
-- (4 CreatureScripts: boss_ysondre / boss_lethon / boss_emeriss /
-- boss_taerar (WorldBossAI over the shared emerald_dragonAI base) +
-- npc_dream_fog (ScriptedAI, NPC_DREAM_FOG = 15224, zero Talk) +
-- npc_spirit_shade (PassiveAI, NPC_SPIRIT_SHADE = 15261, zero Talk);
-- 2 SpellScriptLoaders (spell_dream_fog_sleep, spell_mark_of_nature);
-- all registered from inside AddSC_emerald_dragons() (line 808);
-- loader decl 23 / call 44 per world_script_loader.cpp — the group
-- immediately after AddSC_areatrigger_scripts() (call 43) in
-- AddWorldScripts() — verified from the loader this run; the
-- checkpoint sequence (areatrigger_scripts -> emerald_dragons) is
-- followed).
-- Entries: 14887 Ysondre (Feralas) / 14888 Lethon (Duskwood) /
-- 14889 Emeriss (Hinterlands) / 14890 Taerar (Ashenvale) — all from
-- the file's own EmeraldDragonNPC enum; NPC_DREAM_FOG = 15224 from the
-- same enum; the ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_emerald_dragons" hits boss_emerald_dragons.cpp only (+ the
-- loader decl/call lines). No emerald-dragons lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent.
-- Ported arms (C++-exact for all modeled arms) — the four
-- JustEngagedWith arms (event 1 — the auriaya precedent; the
-- WorldBossAI::JustEngagedWith legs have no bridges):
-- boss_ysondre JustEngagedWith — Talk(SAY_YSONDRE_AGGRO 0) +
-- EVENT_LIGHTNING_WAVE (DoCastVictim 24819, 12s init,
-- randtime(10s,20s) repeat);
-- boss_lethon JustEngagedWith — Talk(SAY_LETHON_AGGRO 0) +
-- EVENT_SHADOW_BOLT_WHIRL (me->CastSpell(nullptr, 24834, false),
-- non-triggered, 10s init, randtime(15s,30s) repeat — lowered to
-- self-cast);
-- boss_emeriss JustEngagedWith — Talk(SAY_EMERISS_AGGRO 0) +
-- EVENT_VOLATILE_INFECTION (DoCastVictim 24928, 12s init, 120s
-- repeat);
-- boss_taerar JustEngagedWith — Talk(SAY_TAERAR_AGGRO 0) +
-- EVENT_ARCANE_BLAST (DoCast 24857 1-arg — AISpellInfo resolves to
-- AITARGET_VICTIM for the ranged nuke, so victim-cast; 12s init,
-- randtime(7s,12s) repeat) + EVENT_BELLOWING_ROAR (DoCast 22686 1-arg
-- — AISpellInfo stays AITARGET_SELF for the range-0 PBAOE, so
-- self-cast; 30s init, randtime(20s,30s) repeat).
-- (The Oct-12 "no-timer bridge" claim was wrong — CreateLuaEvent is
-- a real bridge, retracted this run.) All timers cancelled on
-- 2/4/23 (gargolmar precedent).
-- DOCUMENTED-ONLY (in this header; only the four dragon entries
-- registered):
-- emerald_dragonAI Reset — DoCast(me, SPELL_MARK_OF_NATURE_AURA 25041,
-- triggered) non-targeted self-cast + RemoveFlag NOT_SELECTABLE /
-- NON_ATTACKABLE + SetReactState(REACT_AGGRESSIVE) + the EVENT_TAIL_SWEEP
-- / EVENT_NOXIOUS_BREATH / EVENT_SEEPING_FOG ScheduleEvent legs (the
-- self-cast rides the vaelastrasz self-cast convention but is not a
-- Talk arm; timers / flags / react-state have no bridges).
-- emerald_dragonAI UpdateAI — the SPELL_SUMMON_PLAYER 24776 max-threat
-- pull + DoMeleeAttackIfReady legs (no bridges); the base event machine
-- (EVENT_SEEPING_FOG 120-150s DoCast pair, EVENT_NOXIOUS_BREATH 7.5-15s,
-- EVENT_TAIL_SWEEP 2s) — re-keyed 2026-10-15 (timer doctrine): the
-- timer and cast legs bridge (CreateLuaEvent / CastSpell are live);
-- the machine stays out because the payloads ride unbridged legs —
-- the SPELL_SUMMON_PLAYER 24776 max-threat pull (no threat /
-- random-target bridge — the_beast / doomwalker precedent) and the
-- RemoveFlag / SetReactState legs. The stale "no-timer / no-cast"
-- claim is corrected.
-- emerald_dragonAI KilledUnit — who->CastSpell(who, SPELL_MARK_OF_NATURE
-- 25040, triggered) gated on victim->GetTypeId() == TYPEID_PLAYER —
-- zero Talk, does not port: the C++ caster is the victim (a player
-- self-cast on a corpse); the Lua CastSpell path casts from the
-- creature with no caster override (blessed-banner precedent), and
-- spells 25040/24904 have no Go effect implementation (spell_
-- mark_of_nature SpellScript NOT registered, below) — a port would
-- broadcast a cast with no mechanical effect.
-- boss_emeriss KilledUnit — DoCast(who, SPELL_PUTRID_MUSHROOM 24904,
-- triggered) gated on player victim — same victim-self-cast / no-
-- effect grounds (rides on the base KilledUnit leg above).
-- boss_ysondre DamageTaken — Talk(SAY_YSONDRE_SUMMON_DRUIDS 1) rides
-- the 25%-per-stage DamageTaken machine (10x triggered
-- SPELL_SUMMON_DRUID_SPIRITS 24795 casts) — no-DamageTaken bridge.
-- boss_lethon DamageTaken — Talk(SAY_LETHON_DRAW_SPIRIT 1) rides the
-- 25%-per-stage DamageTaken machine (DoCast SPELL_DRAW_SPIRIT 24811) —
-- no-DamageTaken bridge.
-- boss_lethon SpellHitTarget — the DRAW_SPIRIT hit-target summon
-- machine (NPC_SPIRIT_SHADE 15261, 50s) — no-SpellHitTarget bridge.
-- boss_emeriss DamageTaken — Talk(SAY_EMERISS_CAST_CORRUPTION 1) rides
-- the 25%-per-stage DamageTaken machine (triggered
-- SPELL_CORRUPTION_OF_EARTH 24910 cast) — no-DamageTaken bridge.
-- boss_taerar DamageTaken — Talk(SAY_TAERAR_SUMMON_SHADES 1) rides the
-- 25%-per-stage banish machine (3x triggered victim-cast shade summons
-- 24841/24842/24843 + DoCast SPELL_SHADE 24313 + NOT_SELECTABLE /
-- NON_ATTACKABLE flags + REACT_PASSIVE + 60s _banishedTimer) —
-- no-DamageTaken bridge; the SummonedCreatureDies _shades-- leg rides
-- no-summon-death bridge; the UpdateAI banish/timeout machine —
-- re-keyed 2026-10-15 (timer doctrine): the timer and cast legs
-- bridge, but the machine stays out on the no-flag / no-react-state
-- / no-summon bridges. The stale "no-timer" claim is corrected.
-- The four dragon ExecuteEvent arms ported above (EVENT_LIGHTNING_WAVE
-- / EVENT_SHADOW_BOLT_WHIRL / EVENT_VOLATILE_INFECTION /
-- EVENT_ARCANE_BLAST / EVENT_BELLOWING_ROAR).
-- npc_dream_fog — zero Talk; NOT registered: the _roamTimer 15-30s
-- chase / 2.5s MoveRandom + MoveChase + SetWalk + walk-speed machine
-- — re-keyed 2026-10-15 (timer doctrine): no-movement bridge is the
-- true blocker (the timer leg bridges); with no registered Lua file
-- the class stays out. The stale "no-timer" claim is corrected.
-- npc_spirit_shade — zero Talk; NOT registered: the IsSummonedBy
-- MoveFollow + MovementInform dark-offering cast + 1s
-- DespawnOrUnsummon machine — no-movement / no-summon-death /
-- no-despawn bridges are the true blockers (the timer and cast legs
-- bridge); with no registered Lua file the class stays out. The
-- stale "no-timer / no-cast" claim is corrected.
-- spell_dream_fog_sleep + spell_mark_of_nature (SpellScriptLoaders) —
-- no-SpellScript bridge — NOT registered.
-- All Talk() calls in the file accounted for (4 ported above +
-- SAY_YSONDRE_SUMMON_DRUIDS 1 + SAY_LETHON_DRAW_SPIRIT 1 +
-- SAY_EMERISS_CAST_CORRUPTION 1 + SAY_TAERAR_SUMMON_SHADES 1
-- documented above).

local ENTRY_YSONDRE = 14887
local ENTRY_LETHON  = 14888
local ENTRY_EMERISS = 14889
local ENTRY_TAERAR  = 14890

local SAY_AGGRO = 0 -- SAY_YSONDRE_AGGRO / SAY_LETHON_AGGRO /
                    -- SAY_EMERISS_AGGRO / SAY_TAERAR_AGGRO (all 0)

local SPELL_LIGHTNING_WAVE = 24819 -- boss_emerald_dragons.cpp:248 (Ysondre)
local SPELL_SHADOW_BOLT_WHIRL = 24834 -- boss_emerald_dragons.cpp:337 (Lethon)
local SPELL_VOLATILE_INFECTION = 24928 -- boss_emerald_dragons.cpp:475 (Emeriss)
local SPELL_ARCANE_BLAST = 24857 -- boss_emerald_dragons.cpp:568 (Taerar)
local SPELL_BELLOWING_ROAR = 22686 -- boss_emerald_dragons.cpp:563 (Taerar)

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

-- C++ boss_ysondre::ExecuteEvent (:299-301): DoCastVictim(SPELL_
-- LIGHTNING_WAVE 24819) 2-arg (non-triggered); 12s init,
-- randtime(10s,20s) repeat.
local function lightningWaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_LIGHTNING_WAVE)
    schedule(guid, "lightning_wave", math.random(10, 20) * 1000, function()
        lightningWaveTick(creature, guid)
    end)
end

-- C++ boss_lethon::ExecuteEvent (:399-401): me->CastSpell(nullptr,
-- SPELL_SHADOW_BOLT_WHIRL 24834, false) — explicit non-triggered,
-- no explicit target; 10s init, randtime(15s,30s) repeat. Lowered to
-- self-cast creature:CastSpell(creature, spell) (boss_ai.go DoCastAOE
-- self-cast convention — selin_fireheart / mr_smite precedent).
local function shadowBoltWhirlTick(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOW_BOLT_WHIRL)
    schedule(guid, "shadow_bolt_whirl", math.random(15, 30) * 1000, function()
        shadowBoltWhirlTick(creature, guid)
    end)
end

-- C++ boss_emeriss::ExecuteEvent (:529-531): DoCastVictim(SPELL_
-- VOLATILE_INFECTION 24928) 2-arg (non-triggered); 12s init, 120s
-- repeat.
local function volatileInfectionTick(creature, guid)
    creature:CastSpell(nil, SPELL_VOLATILE_INFECTION)
    schedule(guid, "volatile_infection", 120000, function()
        volatileInfectionTick(creature, guid)
    end)
end

-- C++ boss_taerar::ExecuteEvent (:649-651): DoCast(SPELL_ARCANE_BLAST
-- 24857) 1-arg (DoCastVictim); 12s init, randtime(7s,12s) repeat.
local function arcaneBlastTick(creature, guid)
    creature:CastSpell(nil, SPELL_ARCANE_BLAST)
    schedule(guid, "arcane_blast", math.random(7, 12) * 1000, function()
        arcaneBlastTick(creature, guid)
    end)
end

-- C++ boss_taerar::ExecuteEvent (:653-655): DoCast(SPELL_BELLOWING_
-- ROAR 22686) 1-arg. The 1-arg DoCast resolves via AISpellInfo
-- (UnitAI.cpp FillAISpellInfo): Bellowing Roar is a PBAOE with range
-- 0, so the target stays AITARGET_SELF — self-cast. 30s init,
-- randtime(20s,30s) repeat.
local function bellowingRoarTick(creature, guid)
    creature:CastSpell(creature, SPELL_BELLOWING_ROAR)
    schedule(guid, "bellowing_roar", math.random(20, 30) * 1000, function()
        bellowingRoarTick(creature, guid)
    end)
end

-- C++ boss_ysondre/boss_lethon/boss_emeriss/boss_taerar::JustEngagedWith:
-- Talk(SAY_*_AGGRO); WorldBossAI::JustEngagedWith(who) — the auriaya
-- precedent (the WorldBossAI legs have no bridges). Each dragon seeds
-- its own timer arm(s) below (the Oct-12 "no-timer bridge" claim was
-- wrong — CreateLuaEvent is a real bridge, retracted this run).
local function ysondreEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "lightning_wave", 12000, function()
        lightningWaveTick(creature, guid)
    end)
end

local function lethonEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadow_bolt_whirl", 10000, function()
        shadowBoltWhirlTick(creature, guid)
    end)
end

local function emerissEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "volatile_infection", 12000, function()
        volatileInfectionTick(creature, guid)
    end)
end

local function taerarEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arcane_blast", 12000, function()
        arcaneBlastTick(creature, guid)
    end)
    schedule(guid, "bellowing_roar", 30000, function()
        bellowingRoarTick(creature, guid)
    end)
end

local function dragonLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function dragonDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function dragonReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_YSONDRE, 1, ysondreEnterCombat)
RegisterCreatureEvent(ENTRY_YSONDRE, 2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_YSONDRE, 4, dragonDied)
RegisterCreatureEvent(ENTRY_YSONDRE, 23, dragonReset)
RegisterCreatureEvent(ENTRY_LETHON,  1, lethonEnterCombat)
RegisterCreatureEvent(ENTRY_LETHON,  2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_LETHON,  4, dragonDied)
RegisterCreatureEvent(ENTRY_LETHON,  23, dragonReset)
RegisterCreatureEvent(ENTRY_EMERISS, 1, emerissEnterCombat)
RegisterCreatureEvent(ENTRY_EMERISS, 2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_EMERISS, 4, dragonDied)
RegisterCreatureEvent(ENTRY_EMERISS, 23, dragonReset)
RegisterCreatureEvent(ENTRY_TAERAR,  1, taerarEnterCombat)
RegisterCreatureEvent(ENTRY_TAERAR,  2, dragonLeaveCombat)
RegisterCreatureEvent(ENTRY_TAERAR,  4, dragonDied)
RegisterCreatureEvent(ENTRY_TAERAR,  23, dragonReset)
