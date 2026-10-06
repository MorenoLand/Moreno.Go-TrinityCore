-- Ruins of Ahn'Qiraj: Ossirian the Unscarred — Lua port of
-- src/server/scripts/Kalimdor/RuinsOfAhnQiraj/boss_ossirian.cpp
-- (boss_ossirianAI : public BossAI(creature, DATA_OSSIRIAN);
-- GetAI via GetAQ20AI<boss_ossirianAI>(creature) (AQ20ScriptName
-- "instance_ruins_of_ahnqiraj" gate); AddSC_boss_ossirian at end
-- registers boss_ossirian + go_ossirian_crystal; kalimdor loader
-- decl 84 / call 197 per kalimdor_script_loader.cpp — sixth
-- "// Ruins of ahn'qiraj" loader group, after ayamiss, before the
-- instance script).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_ossirian" (loader decl/call lines only otherwise).
-- Entry: ruins_of_ahnqiraj.h:46 names NPC_OSSIRIAN = 15339; the
-- creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 14 OnSpellHit, 23
-- OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage: schedule SILENCE (30s), CYCLONE (20s), STOMP (30s);
--   self-cast SUPREME 25176; Talk(SAY_AGGRO 2). (C++ also sets
--   weather, summons tornados, spawns crystals — no bridge,
--   documented below.)
-- - EVENT_SILENCE: Curse of Tongues 25195 on all players in the
--   instance (C++ DoCastAOE; no AoE bridge — iterate via
--   GetPlayersInWorld filtered by map/instance/alive, maiden
--   convention), reschedule 20-30s.
-- - EVENT_CYCLONE: DoCastVictim 25189 (jeklik GetVictim + CastSpell
--   convention), reschedule 20s.
-- - EVENT_STOMP: self-cast 25188, reschedule 30s.
-- - Supreme reapply (5s timer, C++ UpdateAI's per-tick HasAura
--   check): if not HasAura(SUPREME 25176) and not HasAura(any of
--   the 5 weaknesses) -> self-cast SUPREME + Talk(SAY_SUPREME 0)
--   (netherspite HasAura convention).
-- - No-kiting (5s timer, C++ per-tick distance check): victim
--   distance 60-120 (moroes GetDistance convention) ->
--   DoCastVictim(SPELL_SUMMON 20477).
-- - KilledUnit (event 3): Talk(SAY_SLAY 3), no target gate (C++
--   ignores the victim).
-- - SpellHit (event 14, midnight convention): weakness spell ID ->
--   RemoveAura(SUPREME 25176) (netherspite RemoveAura convention).
--   The crystal despawn + SpawnNextCrystal have no bridge.
-- Unmodeled (documented-only, no bridges):
-- - Weather (WEATHER_STATE_HEAVY_SANDSTORM): no bridge.
-- - Tornado summons (NPC_SAND_VORTEX): SummonCreature no bridge
--   (zero Lua usage).
-- - Crystal/trigger summons (SpawnNextCrystal): SummonCreature no
--   bridge.
-- - go_ossirian_crystal (GameObjectAI OnGossipHello): no bridge.
-- - MoveInLineOfSight SAY_INTRO: no Lua bridge for MoveInLineOfSight.
-- - DoAction(ACTION_TRIGGER_WEAKNESS): cross-AI via TriggerGUID,
--   no bridge.
-- - Cleanup()/EnterEvadeMode()/JustDied() instance legs: no bridge.
-- - BossAI ctor leg DATA_OSSIRIAN and _Reset() — instance bridge,
--   standing.
local ENTRY = 15339

local SPELL_CURSE_OF_TONGUES = 25195
local SPELL_CYCLONE = 25189
local SPELL_STOMP = 25188
local SPELL_SUPREME = 25176
local SPELL_SUMMON = 20477

local SPELL_WEAKNESS_FIRE = 25177
local SPELL_WEAKNESS_FROST = 25178
local SPELL_WEAKNESS_NATURE = 25180
local SPELL_WEAKNESS_ARCANE = 25181
local SPELL_WEAKNESS_SHADOW = 25183

local WEAKNESSES = {
    SPELL_WEAKNESS_FIRE,
    SPELL_WEAKNESS_FROST,
    SPELL_WEAKNESS_NATURE,
    SPELL_WEAKNESS_ARCANE,
    SPELL_WEAKNESS_SHADOW,
}

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

-- C++ EVENT_SILENCE: DoCastAOE(25195), init 30s -> 20-30s.
-- No AoE bridge; cast on each live player in the instance.
local function onSilence(creature, guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            creature:CastSpell(p, SPELL_CURSE_OF_TONGUES)
        end
    end
    schedule(guid, "silence", math.random(20000, 30000), function()
        onSilence(creature, guid)
    end)
end

-- C++ EVENT_CYCLONE: DoCastVictim(25189), init 20s -> 20s.
local function onCyclone(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CYCLONE)
    end
    schedule(guid, "cyclone", 20000, function()
        onCyclone(creature, guid)
    end)
end

-- C++ EVENT_STOMP: DoCastSelf(25188), init 30s -> 30s.
local function onStomp(creature, guid)
    creature:CastSpell(creature, SPELL_STOMP)
    schedule(guid, "stomp", 30000, function()
        onStomp(creature, guid)
    end)
end

-- C++ UpdateAI supreme leg: reapply SUPREME if neither it nor any
-- weakness is present; checked on a 5s timer (no per-tick UpdateAI
-- in Lua).
local function checkSupreme(creature, guid)
    if creature:HasAura(SPELL_SUPREME) then
        return
    end
    for _, w in ipairs(WEAKNESSES) do
        if creature:HasAura(w) then
            return
        end
    end
    creature:CastSpell(creature, SPELL_SUPREME)
    creature:Talk(0)
    schedule(guid, "supreme", 5000, function()
        checkSupreme(creature, guid)
    end)
end

-- C++ UpdateAI no-kiting leg: victim at 60-120yd -> SPELL_SUMMON.
-- Checked on a 5s timer.
local function checkKiting(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        local dist = creature:GetDistance(victim)
        if dist > 60.0 and dist < 120.0 then
            creature:CastSpell(victim, SPELL_SUMMON)
        end
    end
    schedule(guid, "kiting", 5000, function()
        checkKiting(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_SUPREME)
    creature:Talk(2)
    schedule(guid, "silence", 30000, function()
        onSilence(creature, guid)
    end)
    schedule(guid, "cyclone", 20000, function()
        onCyclone(creature, guid)
    end)
    schedule(guid, "stomp", 30000, function()
        onStomp(creature, guid)
    end)
    schedule(guid, "supreme", 5000, function()
        checkSupreme(creature, guid)
    end)
    schedule(guid, "kiting", 5000, function()
        checkKiting(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_SLAY 3), victim ignored.
local function onTargetDied(event, creature, victim)
    creature:Talk(3)
end

-- C++ SpellHit: weakness spell -> strip SUPREME (crystal legs
-- have no bridge).
local function onSpellHit(event, creature, caster, spellId)
    for _, w in ipairs(WEAKNESSES) do
        if spellId == w then
            creature:RemoveAura(SPELL_SUPREME)
            return
        end
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 14, onSpellHit)
RegisterCreatureEvent(ENTRY, 23, onReset)
