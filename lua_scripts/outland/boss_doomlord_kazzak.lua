-- Doom Lord Kazzak (Outland world boss) — Lua port of
-- src/server/scripts/Outland/boss_doomlord_kazzak.cpp
-- Creature entry 18728 (no NPC_ constant anywhere in the
-- C++ sources; TBC entry verified via the 2010 TrinityCore
-- changelog paste listing "NPC: Doom Lord Kazzak has now
-- fixed respawn rate (Entry ID: #18728)" — same paste also
-- grounds Doomwalker #17711; void_reaver precedent). The
-- creature_template ScriptName binding is DB-side, no TDB
-- in this workspace.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 5 OnSpawn, 23 OnReset.
-- Timers via CreateLuaEvent (per-GUID scheduler pump);
-- melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Deviations from C++ (documented-only legs):
-- * spell_mark_of_kazzak / spell_twisted_reflection are
--   SpellScriptLoader AuraScripts (mana-scaled Mark amount,
--   32961 explode-at-zero-mana, proc-redirected 21064 heal) —
--   no SpellScript/AuraScript bridge exists in Go; the boss
--   casts the base spells 32960/21063, the custom aura
--   mechanics are unmodeled.
-- * The UNIT_STATE_CASTING gate — creature:IsCasting is a
--   stub returning false (lua_objects.go:330) — timers fire
--   unconditionally (documented only).
-- * JustAppeared's SAY_INTRO is wired to hook 5 (OnSpawn):
--   no fire site exists for first grid activation, so the
--   intro fires on the respawn path (documented deviation).
-- * SAY_SURPREME (2) and SAY_RAND (6) are declared in the
--   C++ Texts enum but never used — no arms.
-- * SelectTarget(Random, 0, 0.0f, playerOnly) is approximated
--   with GetPlayersInWorld filtered by map/instance/alive
--   (maiden convention); 0.0f = no distance filter in C++,
--   so no yard-range gate here.
-- Fidelity notes (C++-exact):
-- * All casts are non-triggered DoCast/DoCastVictim (:101,
--   :127-:158) — plain CastSpell with no triggered flag.
-- * KilledUnit's TYPEID_PLAYER gate (cpp:98-100) — terestian
--   GetObjectType convention — before CAPTURE_SOUL 32966 +
--   SAY_KILL.
-- * Timer values from EventMap ScheduleEvent lines (:75-:82
--   inits, :128-:155 re-arms): volley {6s,10s}->{4s,6s},
--   cleave 7s->{8s,12s}, thunderclap {14s,18s}->{10s,14s},
--   void bolt 30s->{15s,18s}, mark 25s->20s, enrage 60s->30s,
--   twisted 33s->15s, berserk 180s one-shot.

local ENTRY = 18728

local SAY_INTRO, SAY_AGGRO, SAY_KILL, SAY_DEATH, EMOTE_FRENZY = 0, 1, 3, 4, 5

local SPELL_SHADOW_VOLLEY    = 32963
local SPELL_CLEAVE           = 31779
local SPELL_THUNDERCLAP      = 36706
local SPELL_VOID_BOLT        = 39329
local SPELL_MARK_OF_KAZZAK   = 32960
local SPELL_ENRAGE           = 32964
local SPELL_CAPTURE_SOUL     = 32966
local SPELL_TWISTED_REFLECTION = 21063
local SPELL_BERSERK          = 32965

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

-- C++ SelectTarget(Random, 0, 0.0f, playerOnly): random
-- threat-list player, no distance filter. Approximated with
-- alive players on the same map/instance (maiden convention).
local function randomPlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onShadowVolley(creature, guid)
    creature:CastSpell(nil, SPELL_SHADOW_VOLLEY)
    schedule(guid, "shadowvolley", {4000, 6000}, function() onShadowVolley(creature, guid) end)
end

local function onCleave(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {8000, 12000}, function() onCleave(creature, guid) end)
end

local function onThunderclap(creature, guid)
    creature:CastSpell(nil, SPELL_THUNDERCLAP)
    schedule(guid, "thunderclap", {10000, 14000}, function() onThunderclap(creature, guid) end)
end

local function onVoidBolt(creature, guid)
    creature:CastSpell(nil, SPELL_VOID_BOLT)
    schedule(guid, "voidbolt", {15000, 18000}, function() onVoidBolt(creature, guid) end)
end

local function onMarkOfKazzak(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_MARK_OF_KAZZAK)
    end
    schedule(guid, "markofkazzak", 20000, function() onMarkOfKazzak(creature, guid) end)
end

local function onEnrage(creature, guid)
    creature:Talk(EMOTE_FRENZY)
    creature:CastSpell(creature, SPELL_ENRAGE)
    schedule(guid, "enrage", 30000, function() onEnrage(creature, guid) end)
end

local function onTwistedReflection(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_TWISTED_REFLECTION)
    end
    schedule(guid, "twistedreflection", 15000, function() onTwistedReflection(creature, guid) end)
end

local function onEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "shadowvolley", {6000, 10000}, function() onShadowVolley(creature, guid) end)
    schedule(guid, "cleave", 7000, function() onCleave(creature, guid) end)
    schedule(guid, "thunderclap", {14000, 18000}, function() onThunderclap(creature, guid) end)
    schedule(guid, "voidbolt", 30000, function() onVoidBolt(creature, guid) end)
    schedule(guid, "markofkazzak", 25000, function() onMarkOfKazzak(creature, guid) end)
    schedule(guid, "enrage", 60000, function() onEnrage(creature, guid) end)
    schedule(guid, "twistedreflection", 33000, function() onTwistedReflection(creature, guid) end)
    schedule(guid, "berserk", 180000, function() creature:CastSpell(creature, SPELL_BERSERK) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onSpawn(event, creature)
    creature:Talk(SAY_INTRO)
end

local function onTargetDied(event, creature, victim)
    -- C++ KilledUnit: TYPEID_PLAYER gate (cpp:98-100,
    -- terestian GetObjectType convention).
    if victim:GetObjectType() == "Player" then
        creature:CastSpell(creature, SPELL_CAPTURE_SOUL)
        creature:Talk(SAY_KILL)
    end
end

local function onDied(event, creature)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 5, onSpawn)
RegisterCreatureEvent(ENTRY, 23, onReset)
