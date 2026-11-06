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
-- (blackrock_spire.h). SetBossState legs (BossAI DATA_LORD_VALTHALAK
-- = 14, blackrock_spire.h:44; < EncounterCount 23): ev1 IN_PROGRESS
-- via base BossAI::JustEngagedWith (boss_lord_valthalak.cpp:69, base
-- call first; _JustEngagedWith writes IN_PROGRESS at ScriptedCreature.
-- cpp:537); ev23 NOT_STARTED via Reset's _Reset() (:61-65; _Reset
-- gates on alive + state != DONE, ScriptedCreature.cpp:499-511).
-- No ev4 leg — see Unmodeled below. Bridge live (lua_creature_events.
-- go:5917; gyth convention).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent timers; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine), all urand ranges as CreateLuaEvent {min,max} tables
-- (maiden convention):
-- EVENT_SUMMON_SPECTRAL_ASSASSIN: self-cast SPELL_SUMMON_SPECTRAL_ASSASSIN
-- 27249, 6s init (urand(6s,8s)) -> 30s loop (urand(30s,35s))
-- (DoCast(me) path — creature:CastSpell(creature, spell),
-- pyroguard_emberseer convention).
-- EVENT_SHADOW_WRATH: victim-cast SPELL_SHADOW_WRATH 27286, 9s init
-- (urand(9s,18s)) -> 19s loop (urand(19s,24s)); victim nil-guarded
-- (incarcerator convention).
-- Frenzy-40 latch: HealthBelowPct(40) = health < floor(max*40/100)
-- one-shot -> self-cast SPELL_FRENZY 8269 + CancelEvent(EVENT_SUMMON_
-- SPECTRAL_ASSASSIN) (C++-exact: the summon timer is dropped, not
-- re-armed; C++ emits no Talk here — EMOTE_FRENZY is enum-only).
-- Frenzy-15 latch: HealthBelowPct(15) = health < floor(max*15/100)
-- one-shot -> self-cast SPELL_FRENZY 8269 + ScheduleEvent(EVENT_SHADOW_
-- BOLT_VOLLEY, 7s, 14s). EVENT_SHADOW_BOLT_VOLLEY never fires before
-- this latch.
-- EVENT_SHADOW_BOLT_VOLLEY: victim-cast SPELL_SHADOW_BOLT_VOLLEY 27382,
-- 7s init (urand(7s,14s)) -> 4s loop (urand(4s,6s)).
-- The two frenzy latches ride OnDamageTaken(9) with per-guid one-shot
-- flags reset on OnEnterCombat(1) (gyth convention); C++ checks the
-- latches in UpdateAI even out of combat, but the only state transition
-- into sub-40%/sub-15% is damage, so the damage-event latch is faithful.
-- CASTING gates ported: C++ UpdateAI pre-loop gate (:86) + in-loop
-- gate (:109) are a 500ms IsCasting defer under each arm's own key
-- (live, lua_creature_events.go:1950; mal_ganis/rend idiom) -- a
-- gated arm fires nothing and consumes no re-arm. The post-loop
-- frenzy latches (:112/:122) stay ungated on OnDamageTaken(9): in
-- C++ they are skipped while casting but re-checked on later ticks,
-- not lost, and the damage hook is the only state transition into
-- sub-40%/sub-15%. The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (RemoveEventById convention — CreateLuaEvent returns an integer id,
-- not an object; the old :Cancel() method idiom errored at runtime and
-- never cancelled)
-- (maiden convention).
-- Unmodeled (documented-only): NO ev4 DONE leg — JustDied (:74-77)
-- overrides without _JustDied(), and its instance->SetData(DATA_LORD_
-- VALTHALAK, DONE) is a no-op in C++: instance SetData's switch
-- (instance_blackrock_spire.cpp:294-313) handles only AREATRIGGER and
-- DATA_BLACKHAND_INCARCERATOR, so the DONE write never happens
-- (C'Thun / bug-trio precedent). Reset's _Reset() bookkeeping is
-- covered by the cancel/re-arm on combat events (halycon precedent).

local ENTRY_LORD_VALTHALAK = 16042

local SPELL_FRENZY = 8269
local SPELL_SUMMON_SPECTRAL_ASSASSIN = 27249
local SPELL_SHADOW_BOLT_VOLLEY = 27382
local SPELL_SHADOW_WRATH = 27286

local timers = {}
local frenzy40 = {}
local frenzy15 = {}

local function schedule(guid, key, ms, fn)
    if timers[guid] then
        if timers[guid][key] then
            RemoveEventById(timers[guid][key])
        end
    else
        timers[guid] = {}
    end
    timers[guid][key] = CreateLuaEvent(fn, ms)
end

local function cancelTimers(guid)
    if timers[guid] then
        for _, id in pairs(timers[guid]) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function cancelKey(guid, key)
    if timers[guid] and timers[guid][key] then
        RemoveEventById(timers[guid][key])
        timers[guid][key] = nil
    end
end

local function onSummonSpectralAssassin(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "summon", 500, function() onSummonSpectralAssassin(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_SPECTRAL_ASSASSIN)
    schedule(guid, "summon", {30000, 35000}, function() onSummonSpectralAssassin(creature, guid) end)
end

local function onShadowWrath(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wrath", 500, function() onShadowWrath(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_WRATH)
    end
    schedule(guid, "wrath", {19000, 24000}, function() onShadowWrath(creature, guid) end)
end

local function onShadowBoltVolley(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "volley", 500, function() onShadowBoltVolley(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_BOLT_VOLLEY)
    end
    schedule(guid, "volley", {4000, 6000}, function() onShadowBoltVolley(creature, guid) end)
end

local function onEnterCombat(_, creature)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_lord_valthalak.cpp:69 base call first)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_lord_valthalak.cpp:69 base call first)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_LORD_VALTHALAK=14 (blackrock_spire.h:44).
    creature:SetBossState(14, 1)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    frenzy40[guid] = nil
    frenzy15[guid] = nil
    schedule(guid, "summon", {6000, 8000}, function() onSummonSpectralAssassin(creature, guid) end)
    schedule(guid, "wrath", {9000, 18000}, function() onShadowWrath(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset (boss_lord_valthalak.cpp:61-65): _Reset() first, then
-- Initialize() (frenzy-flag reset, already done on enter combat).
-- The _Reset() leg is alive-gated (Go fires ev23 on the death path
-- too) and skipped when already DONE (ScriptedCreature.cpp:499-511).
-- ev4 stays onCombatEnd (cancel only): JustDied writes no DONE — see
-- header Unmodeled note.
local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
    if creature:IsAlive() and creature:GetBossState(14) ~= 3 then
        creature:SetBossState(14, 0)
    end
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if not frenzy40[guid] then
        local postDamage = creature:GetHealth() - damage
        if postDamage > 0 and postDamage < math.floor(creature:GetMaxHealth() * 40 / 100) then
            frenzy40[guid] = true
            creature:CastSpell(creature, SPELL_FRENZY)
            cancelKey(guid, "summon")
        end
    end
    if not frenzy15[guid] then
        local postDamage = creature:GetHealth() - damage
        if postDamage > 0 and postDamage < math.floor(creature:GetMaxHealth() * 15 / 100) then
            frenzy15[guid] = true
            creature:CastSpell(creature, SPELL_FRENZY)
            schedule(guid, "volley", {7000, 14000}, function() onShadowBoltVolley(creature, guid) end)
        end
    end
end

RegisterCreatureEvent(ENTRY_LORD_VALTHALAK, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_LORD_VALTHALAK, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_LORD_VALTHALAK, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_LORD_VALTHALAK, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_LORD_VALTHALAK, 23, onReset)
