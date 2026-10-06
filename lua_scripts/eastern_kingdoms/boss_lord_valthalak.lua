-- Lord Valthalak (Blackrock Spire; the Furnace — summoned via Brazier of Beckoning) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_lord_valthalak.cpp
-- (boss_lord_valthalak — BossAI combat scheduler, the only CreatureScript class in
-- the file; via GetBlackrockSpireAI). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (AddSC_boss_lord_valthalak
-- follows AddSC_boss_urok_doomhowl in the Blackrock Spire block).
-- Entry: no NPC_LORD_VALTHALAK in blackrock_spire.h (the creature enum
-- stops at NPC_FINKLE_EINHORN = 10776); the ScriptName binding is DB-side
-- (no TDB in this workspace). Entry 16042 verified from the 3.3.5 data
-- source: wowhead npc=16042/lord-valthalak and the "Mea Culpa" quest
-- objective "NPC #16042 slain (1) — Lord Valthalak". The AI runs under
-- BossAI(DATA_LORD_VALTHALAK = 14, blackrock_spire.h:44).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h); the combat arms have no instance state.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent timers; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine), all urand ranges at their lower bound (halycon
-- recurring-timer convention):
-- EVENT_SUMMON_SPECTRAL_ASSASSIN: self-cast SPELL_SUMMON_SPECTRAL_ASSASSIN
-- 27249, 6s init (urand(6s,8s)) -> 30s loop (urand(30s,35s))
-- (DoCast(me) path — creature:CastSpell(creature, spell),
-- pyroguard_emberseer convention).
-- EVENT_SHADOW_WRATH: victim-cast SPELL_SHADOW_WRATH 27286, 9s init
-- (urand(9s,18s)) -> 19s loop (urand(19s,24s)); victim nil-guarded
-- (incarcerator convention).
-- Frenzy-40 latch: HealthBelowPct(40) one-shot -> self-cast SPELL_FRENZY
-- 8269 + Talk(EMOTE_FRENZY=0) + CancelEvent(EVENT_SUMMON_SPECTRAL_ASSASSIN)
-- (C++-exact: the summon timer is dropped, not re-armed).
-- Frenzy-15 latch: HealthBelowPct(15) one-shot -> self-cast SPELL_FRENZY
-- 8269 + Talk(EMOTE_FRENZY=0) + ScheduleEvent(EVENT_SHADOW_BOLT_VOLLEY,
-- 7s, 14s). EVENT_SHADOW_BOLT_VOLLEY never fires before this latch.
-- EVENT_SHADOW_BOLT_VOLLEY: victim-cast SPELL_SHADOW_BOLT_VOLLEY 27382,
-- 7s init (urand(7s,14s)) -> 4s loop (urand(4s,6s)).
-- The two frenzy latches ride OnDamageTaken(9) with per-guid one-shot
-- flags reset on OnEnterCombat(1) (gyth convention); C++ checks the
-- latches in UpdateAI even out of combat, but the only state transition
-- into sub-40%/sub-15% is damage, so the damage-event latch is faithful.
-- Talk(EMOTE_FRENZY) fires once per latch (maiden-of-virtue convention;
-- creature:Talk is bridged in lua_creature_events.go).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).
-- Unmodeled (documented-only): JustDied's instance->SetData(DATA_LORD_
-- VALTHALAK, DONE) — no instance bridge on the Lua surface (instance
-- precedent); Reset's _Reset() and JustDied's _JustDied() are internal
-- BossAI machinery covered by the cancel/re-arm on combat events
-- (halycon precedent).

local ENTRY_LORD_VALTHALAK = 16042

local SPELL_FRENZY = 8269
local SPELL_SUMMON_SPECTRAL_ASSASSIN = 27249
local SPELL_SHADOW_BOLT_VOLLEY = 27382
local SPELL_SHADOW_WRATH = 27286

local EMOTE_FRENZY = 0

local timers = {}
local frenzy40 = {}
local frenzy15 = {}

local function schedule(guid, key, ms, fn)
    if timers[guid] then
        if timers[guid][key] then
            timers[guid][key]:Cancel()
        end
    else
        timers[guid] = {}
    end
    timers[guid][key] = CreateLuaEvent(fn, ms)
end

local function cancelTimers(guid)
    if timers[guid] then
        for _, ev in pairs(timers[guid]) do
            ev:Cancel()
        end
        timers[guid] = nil
    end
end

local function cancelKey(guid, key)
    if timers[guid] and timers[guid][key] then
        timers[guid][key]:Cancel()
        timers[guid][key] = nil
    end
end

local function onSummonSpectralAssassin(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_SPECTRAL_ASSASSIN)
    schedule(guid, "summon", 30000, function() onSummonSpectralAssassin(creature, guid) end)
end

local function onShadowWrath(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_WRATH)
    end
    schedule(guid, "wrath", 19000, function() onShadowWrath(creature, guid) end)
end

local function onShadowBoltVolley(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_BOLT_VOLLEY)
    end
    schedule(guid, "volley", 4000, function() onShadowBoltVolley(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    frenzy40[guid] = nil
    frenzy15[guid] = nil
    schedule(guid, "summon", 6000, function() onSummonSpectralAssassin(creature, guid) end)
    schedule(guid, "wrath", 9000, function() onShadowWrath(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if not frenzy40[guid] then
        if creature:GetHealth() - damage < creature:GetMaxHealth() * 0.40 then
            frenzy40[guid] = true
            creature:CastSpell(creature, SPELL_FRENZY)
            creature:Talk(EMOTE_FRENZY)
            cancelKey(guid, "summon")
        end
    end
    if not frenzy15[guid] then
        if creature:GetHealth() - damage < creature:GetMaxHealth() * 0.15 then
            frenzy15[guid] = true
            creature:CastSpell(creature, SPELL_FRENZY)
            creature:Talk(EMOTE_FRENZY)
            schedule(guid, "volley", 7000, function() onShadowBoltVolley(creature, guid) end)
        end
    end
end

RegisterCreatureEvent(ENTRY_LORD_VALTHALAK, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_LORD_VALTHALAK, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_LORD_VALTHALAK, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_LORD_VALTHALAK, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_LORD_VALTHALAK, 23, onCombatEnd)
