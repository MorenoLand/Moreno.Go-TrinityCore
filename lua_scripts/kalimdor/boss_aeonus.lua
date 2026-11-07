-- The Black Morass: Aeonus — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/TheBlackMorass/
-- boss_aeonus.cpp (143 lines; boss_aeonusAI :
-- public BossAI(creature, TYPE_AEONUS); GetAI via
-- GetBlackMorassAI<boss_aeonusAI> (TBMScriptName
-- "instance_the_black_morass" gate); AddSC_boss_aeonus at end registers
-- the one script; kalimdor loader decl 41 / call 154
-- per kalimdor_script_loader.cpp — first "// CoT The Black Morass"
-- loader group, right after AddSC_old_hillsbrad()).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_aeonus" (loader decl/call lines only otherwise).
-- Entry: the_black_morass.h:62 names NPC_AEONUS = 17881 and
-- instance_the_black_morass.cpp:64 spawns it as the sixth RiftWaves
-- boss (SummonedPortalBoss) with the wave-18 :306 GetEntry() ==
-- NPC_AEONUS medivh-threat special-case — the name-to-entry tie is
-- C++-verified; the creature_template ScriptName binding stays DB-side.
-- Not GUID-bound in OnCreatureCreate (only NPC_MEDIVH is), so no
-- ramstein-strength GUID leg.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- UpdateAI CASTING gates PORTED (own UpdateAI pre-loop :111 + in-loop :135 returns, boss_aeonus.cpp): each arm defers 500ms while creature:IsCasting() (live CastingUntil window, lua_creature_events.go:1950), no re-arm consumed; defer precedes victim pick (sandbreath/timestop) and Talk (frenzy).
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (1) + arm each timer at
--   its C++ ScheduleEvent cooldown.
-- - Death (C++ JustDied, non-instance leg): Talk SAY_DEATH (4).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): C++ gates on
--   who->GetTypeId() == TYPEID_PLAYER before Talk SAY_SLAY (3)
--   (terestian_illhoof victim:GetObjectType() == "Player" convention).
-- - Sand Breath 31473 on the victim (C++ DoCastVictim — jeklik GetVictim
--   + CastSpell convention), non-triggered, init {15s,30s} -> re-arm
--   {15s,25s}.
-- - Time Stop 31422 on the victim (C++ DoCastVictim), non-triggered, init
--   {10s,15s} -> re-arm {20s,35s}.
-- - Frenzy: Talk EMOTE_FRENZY (5) + self-cast Enrage 37605 (C++
--   DoCast(me)), non-triggered, init {30s,45s} -> re-arm {20s,35s}.
-- Ported (2026-10-24): MoveInLineOfSight's Time Keeper leg — the old
-- "no nearby-creature enumeration" note is stale (FindNearestCreature
-- is bridged, lua_creature_events.go:2635, live-only, same map+instance).
-- Event 7 (onAIUpdate, hydross precedent: fires for every alive motion,
-- pre-pull included) polls the nearest live Time Keeper within 20 yd;
-- on a hit Talk SAY_BANISH (2) + keeper:Kill(creature) — the Kill bridge
-- (killer arg = aeonus) stands in for C++ Unit::DealDamage(me, who,
-- full health), same visible outcome (keeper dies). The leg is ungated
-- in C++ (no combat gate), so the poll is ungated too; a keeper dies on
-- first detection, so the Talk fires once per keeper, never spams.
-- Unmodeled (documented-only, no bridges):
-- - JustDied's instance legs: instance->SetData(TYPE_RIFT = 2, DONE) +
--   instance->SetData(TYPE_MEDIVH = 1, DONE) (the "FIXME: later should be
--   removed" comment is C++-verbatim) — PORTED via the shared
--   instanceData table (see instance_the_black_morass.lua), including
--   the DONE quest-credit branch (10297 + 9836, inlined below).
--   Medivh's check tick picks the DONE states up as its WIN leg.
-- - SPELL_CLEAVE 40504 is declared in the C++ enum but never cast in
--   C++ — no cast arm exists to port (the lieutenant_drake
--   ExplodingShout_Timer case).
-- - H_SPELL_SAND_BREATH 39049 is declared in the C++ enum but C++
--   UpdateAI casts only the non-heroic 31473 via DoCastVictim — no
--   heroic arm exists to port.
-- - SAY_ENTER 0 is declared in the C++ enum but never Talked in C++ (the
--   archimonde SAY_SOUL_CHARGE case) — no Talk arm exists to port.
-- - C++ Reset() is a no-op; BossAI::_Reset instance bookkeeping is
--   blocked (standing), so the engage re-arm collapses into onReset's
--   cancel (hyjal.lua convention).

local ENTRY = 17881
local NPC_TIME_KEEPER = 17918

local SAY_ENTER   = 0
local SAY_AGGRO   = 1
local SAY_BANISH  = 2
local SAY_SLAY    = 3
local SAY_DEATH   = 4
local EMOTE_FRENZY = 5

local SPELL_CLEAVE        = 40504
local SPELL_TIME_STOP     = 31422
local SPELL_ENRAGE        = 37605
local SPELL_SAND_BREATH   = 31473
local H_SPELL_SAND_BREATH = 39049

local QUEST_OPENING_PORTAL = 10297
local QUEST_MASTER_TOUCH = 9836
local QUEST_STATUS_INCOMPLETE = 3

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ EVENT_SANDBREATH: DoCastVictim(31473), non-triggered, init
-- {15s,30s} -> {15s,25s}; jeklik GetVictim + CastSpell convention.
local function onSandBreath(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "sandbreath", 500, function()
            onSandBreath(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_SAND_BREATH)
    end
    schedule(guid, "sandbreath", math.random(15000, 25000), function()
        onSandBreath(creature, guid)
    end)
end

-- C++ EVENT_TIMESTOP: DoCastVictim(31422), non-triggered, init
-- {10s,15s} -> {20s,35s}.
local function onTimeStop(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "timestop", 500, function()
            onTimeStop(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_TIME_STOP)
    end
    schedule(guid, "timestop", math.random(20000, 35000), function()
        onTimeStop(creature, guid)
    end)
end

-- C++ EVENT_FRENZY: Talk EMOTE_FRENZY (5) + DoCast(me, 37605),
-- non-triggered, init {30s,45s} -> {20s,35s}.
local function onFrenzy(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frenzy", 500, function()
            onFrenzy(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_FRENZY)
    creature:CastSpell(creature, SPELL_ENRAGE)
    schedule(guid, "frenzy", math.random(20000, 35000), function()
        onFrenzy(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "sandbreath", math.random(15000, 30000), function()
        onSandBreath(creature, guid)
    end)
    schedule(guid, "timestop", math.random(10000, 15000), function()
        onTimeStop(creature, guid)
    end)
    schedule(guid, "frenzy", math.random(30000, 45000), function()
        onFrenzy(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ SetData(TYPE_MEDIVH, DONE) quest-credit arm
-- (instance_the_black_morass.cpp:204-220): every player in the
-- instance with QUEST_OPENING_PORTAL 10297 / QUEST_MASTER_TOUCH 9836
-- incomplete gets AreaExploredOrEventHappens for it, 10297 first.
local function grantMedivhDoneCredit(mapObj)
    if not mapObj then
        return
    end
    local players = mapObj:GetPlayers()
    if not players then
        return
    end
    for _, player in ipairs(players) do
        if player:GetQuestStatus(QUEST_OPENING_PORTAL) == QUEST_STATUS_INCOMPLETE then
            player:AreaExploredOrEventHappens(QUEST_OPENING_PORTAL)
        end
        if player:GetQuestStatus(QUEST_MASTER_TOUCH) == QUEST_STATUS_INCOMPLETE then
            player:AreaExploredOrEventHappens(QUEST_MASTER_TOUCH)
        end
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    -- C++ JustDied instance legs (boss_aeonus.cpp:93-94):
    -- SetData(TYPE_RIFT, DONE) + SetData(TYPE_MEDIVH, DONE) (the
    -- "FIXME: later should be removed" comment is C++-verbatim).
    -- Written to the shared instanceData table (see
    -- instance_the_black_morass.lua); the DONE quest-credit branch
    -- of SetData runs inline below (bridges live: map:GetPlayers,
    -- player:GetQuestStatus, player:AreaExploredOrEventHappens).
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.riftState = 3
            data.medivhState = 3
        end
        grantMedivhDoneCredit(mapObj)
    end
end

local function onAIUpdate(event, creature)
    local keeper = creature:FindNearestCreature(NPC_TIME_KEEPER, 20)
    if keeper then
        creature:Talk(SAY_BANISH)
        keeper:Kill(creature)
    end
end

RegisterCreatureEvent(ENTRY, 7, onAIUpdate)
RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
