-- Noth the Plaguebringer (Naxxramas, Plague Quarter) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_noth.cpp
-- (boss_noth (BossAI); AddSC_boss_noth registers only it).
-- Entry: 15954 Noth the Plaguebringer (wowhead classic
-- npc=15954 link + creature_template table cross-check:
-- 15956/15953/15952/15932/15936/15989/15990 all match
-- naxxramas.h NAXCreaturesIds; 25-man difficulty entry 29615
-- is NOT registered, base-entry-only convention like
-- anubrekhan/maexxna).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 19 OnJustSummoned,
-- 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go, like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(creature, spell) = DoCastSelf
-- (DoCastAOE is lowered to self-cast, selin/mr_smite
-- precedent); the trailing true is the triggered port.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith Talk(SAY_AGGRO 0) (event 1; BossAI::
-- JustEngagedWith instance-bookkeeping leg has no bridge —
-- tharon_ja precedent) + EnterPhaseGround; EnterPhaseGround
-- (DoZoneInCombat + EVENT_GROUND_ATTACKABLE 2s +
-- EVENT_BALCONY 90s/110s/180s by balconyCount +
-- EVENT_CURSE randtime(10s,25s) + EVENT_WARRIOR
-- randtime(20s,30s) + 25-man EVENT_BLINK randtime(20s,30s));
-- EVENT_CURSE DoCastAOE(CURSE 29213, non-triggered)
-- self-cast, randtime(50s,70s) repeat (the enum's 25-man
-- 54835 is never selected by the C++ arm — the arm casts
-- SPELL_CURSE unconditionally, so Lua does too);
-- EVENT_WARRIOR Talk(SAY_SUMMON 1) + Talk(EMOTE_SUMMON 4)
-- + CastSummon(RAID_MODE(2,3), 0, 0), 40s repeat;
-- EVENT_BLINK DoCastAOE(CRIPPLE 29212, triggered) self-cast
-- + DoCastAOE(RAND blink 29208/29209/29210/29211,
-- non-triggered) self-cast + ResetAllThreat
-- (ThreatManager.ResetThreat bridge, lua_creature_events.go)
-- + justBlinked latch, 40s repeat;
-- EVENT_BALCONY phase BALCONY + SetReactState(0) PASSIVE
-- + SetFlag NOT_SELECTABLE + AttackStop + StopMoving +
-- RemoveAllAuras + EVENT_BALCONY_TELEPORT 3s +
-- EVENT_WAVE randtime(5s,8s) + EVENT_GROUND 70s/97s/120s
-- by balconyCount; EVENT_BALCONY_TELEPORT Talk(EMOTE_
-- TELEPORT_1 6) + DoCastAOE(TELEPORT 29216) self-cast;
-- EVENT_WAVE Talk(EMOTE_SUMMON_WAVE 5) + CastSummon by
-- balconyCount (0: (0, RAID(2,4), 0); 1: (0, RAID(1,2),
-- RAID(1,2)); 2: (0, 0, RAID(2,4)); default: (0,
-- RAID(5,10), RAID(5,10))), randtime(30s,45s) repeat;
-- EVENT_GROUND balconyCount++ + DoCastAOE(TELEPORT_BACK
-- 29231) self-cast + Talk(EMOTE_TELEPORT_2 7) +
-- EnterPhaseGround; EVENT_GROUND_ATTACKABLE RemoveFlag
-- NOT_SELECTABLE + SetReactState(2) AGGRESSIVE.
-- CastSummon uses the C++ HandleSummon shuffle verbatim:
-- per-list copies (kept in per-boss state, mutated across
-- calls exactly like the C++ member arrays) walked in order,
-- each pick urand(it, nSpells-1) then swapped into the
-- walked prefix, skipping no spell twice per call.
-- Phase gating: ground-phase timers are cancelled on
-- balcony entry and balcony timers on ground entry (the
-- C++ EventMap phase masks, modeled explicitly — the
-- schedule helper overwrites same-key timers, cancelKey
-- removes the outgoing phase's keys).
-- JustSummoned (event 19): summoned:SetFarVisible(true) +
-- summoned:DoZoneInCombat() (C++ order); KilledUnit
-- Talk(SAY_SLAY 2) player-gated (event 3, commanders
-- precedent); JustDied Talk(SAY_DEATH 3) (event 4);
-- DamageTaken (event 9, balcony phase only): lethal hit
-- (damage >= health) -> SetHealth(1) + return false, 0
-- (thorim/eadric ev9 convention — C++ sets health 1 and
-- zeroes the damage, so the boss survives the balcony).
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired): Reset /
-- JustEngagedWith / JustDied instance bookkeeping (_Reset /
-- _JustDied via BossAI); EnterPhaseGround IsThreatened ->
-- EnterEvadeMode leg (engage implies threatened in Lua —
-- the check cannot fail on the engage path); EnterEvadeMode
-- balcony TELEPORT_BACK leg, modeled in Reset instead
-- (Go fires ev23 on the evade path before ev2, balnazzar
-- precedent — the cast is gated on phase BALCONY, so a
-- spawn Reset with phase NONE casts nothing); JustSummoned
-- setActive(true) (no bridge); justBlinked MoveChase
-- workaround (no motion bridge — the boss keeps its victim
-- via the engine instead); summons.Summon tracking /
-- DespawnAll (no bridge); 25-man detection is the
-- victim-group-size proxy (deathwhisper precedent;
-- Is25ManRaid / GetDifficulty unbridged).

local ENTRY_NOTH = 15954

local SAY_AGGRO = 0
local SAY_SUMMON = 1
local SAY_SLAY = 2
local SAY_DEATH = 3
local EMOTE_SUMMON = 4
local EMOTE_SUMMON_WAVE = 5
local EMOTE_TELEPORT_1 = 6
local EMOTE_TELEPORT_2 = 7

local SPELL_CURSE = 29213
local SPELL_CRIPPLE = 29212
local SPELL_TELEPORT = 29216
local SPELL_TELEPORT_BACK = 29231
local SPELL_BLINK = { 29208, 29209, 29210, 29211 }

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local timers = {}
local state = {}

local nothCurse, nothWarrior, nothBlink, nothBalcony, nothWave, nothGround

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    state[guid] = nil
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

local function cancelKey(guid, key)
    local per = timers[guid]
    if per ~= nil and per[key] ~= nil then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

local function is25Man(victim)
    if victim ~= nil and victim.GetGroup ~= nil then
        local group = victim:GetGroup()
        if group ~= nil then
            local members = group:GetMembers()
            return members ~= nil and #members > 10
        end
    end
    return false
end

local function raidMode(victim, normal, heroic)
    if is25Man(victim) then
        return heroic
    end
    return normal
end

local function handleSummon(creature, list, num)
    local i = 1
    while num > 0 and i <= #list do
        num = num - 1
        local selected = math.random(i, #list)
        creature:CastSpell(creature, list[selected])
        if selected ~= i then
            list[selected], list[i] = list[i], list[selected]
        end
        i = i + 1
    end
end

local function castSummon(creature, st, nWarrior, nChampion, nGuardian)
    handleSummon(creature, st.warriorSpells, nWarrior)
    handleSummon(creature, st.championSpells, nChampion)
    handleSummon(creature, st.guardianSpells, nGuardian)
end

local function enterPhaseGround(creature, guid)
    local st = state[guid]
    if st == nil then
        return
    end
    cancelKey(guid, "wave")
    cancelKey(guid, "ground")
    cancelKey(guid, "balconyTeleport")
    st.phase = "ground"
    creature:DoZoneInCombat()
    local timeGround = 180
    if st.balconyCount == 0 then
        timeGround = 90
    elseif st.balconyCount == 1 then
        timeGround = 110
    end
    schedule(guid, "groundAttackable", 2000, function()
        local s = state[guid]
        if s ~= nil then
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:SetReactState(2)
        end
    end)
    schedule(guid, "balcony", timeGround * 1000, function()
        nothBalcony(creature, guid)
    end)
    schedule(guid, "curse", math.random(10000, 25000), function()
        nothCurse(creature, guid)
    end)
    schedule(guid, "warrior", math.random(20000, 30000), function()
        nothWarrior(creature, guid)
    end)
    if is25Man(creature:GetVictim()) then
        schedule(guid, "blink", math.random(20000, 30000), function()
            nothBlink(creature, guid)
        end)
    end
end

nothCurse = function(creature, guid)
    creature:CastSpell(creature, SPELL_CURSE)
    schedule(guid, "curse", math.random(50000, 70000), function()
        nothCurse(creature, guid)
    end)
end

nothWarrior = function(creature, guid)
    local st = state[guid]
    creature:Talk(SAY_SUMMON)
    creature:Talk(EMOTE_SUMMON)
    if st ~= nil then
        castSummon(creature, st, raidMode(creature:GetVictim(), 2, 3), 0, 0)
    end
    schedule(guid, "warrior", 40000, function()
        nothWarrior(creature, guid)
    end)
end

nothBlink = function(creature, guid)
    local st = state[guid]
    creature:CastSpell(creature, SPELL_CRIPPLE, true)
    creature:CastSpell(creature, SPELL_BLINK[math.random(#SPELL_BLINK)])
    creature:ResetAllThreat()
    if st ~= nil then
        st.justBlinked = true
    end
    schedule(guid, "blink", 40000, function()
        nothBlink(creature, guid)
    end)
end

nothBalcony = function(creature, guid)
    local st = state[guid]
    if st == nil then
        return
    end
    cancelKey(guid, "curse")
    cancelKey(guid, "warrior")
    cancelKey(guid, "blink")
    cancelKey(guid, "groundAttackable")
    st.phase = "balcony"
    creature:SetReactState(0)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:AttackStop()
    creature:StopMoving()
    creature:RemoveAllAuras()
    schedule(guid, "balconyTeleport", 3000, function()
        creature:Talk(EMOTE_TELEPORT_1)
        creature:CastSpell(creature, SPELL_TELEPORT)
    end)
    schedule(guid, "wave", math.random(5000, 8000), function()
        nothWave(creature, guid)
    end)
    local timeBalcony = 120
    if st.balconyCount == 0 then
        timeBalcony = 70
    elseif st.balconyCount == 1 then
        timeBalcony = 97
    end
    schedule(guid, "ground", timeBalcony * 1000, function()
        nothGround(creature, guid)
    end)
end

nothWave = function(creature, guid)
    local st = state[guid]
    creature:Talk(EMOTE_SUMMON_WAVE)
    if st ~= nil then
        local victim = creature:GetVictim()
        if st.balconyCount == 0 then
            castSummon(creature, st, 0, raidMode(victim, 2, 4), 0)
        elseif st.balconyCount == 1 then
            castSummon(creature, st, 0, raidMode(victim, 1, 2),
                raidMode(victim, 1, 2))
        elseif st.balconyCount == 2 then
            castSummon(creature, st, 0, 0, raidMode(victim, 2, 4))
        else
            castSummon(creature, st, 0, raidMode(victim, 5, 10),
                raidMode(victim, 5, 10))
        end
    end
    schedule(guid, "wave", math.random(30000, 45000), function()
        nothWave(creature, guid)
    end)
end

nothGround = function(creature, guid)
    local st = state[guid]
    if st ~= nil then
        st.balconyCount = st.balconyCount + 1
    end
    creature:CastSpell(creature, SPELL_TELEPORT_BACK)
    creature:Talk(EMOTE_TELEPORT_2)
    enterPhaseGround(creature, guid)
end

local function nothEnterCombat(event, creature)
    local guid = creature:GetGUID()
    state[guid] = {
        phase = "none",
        balconyCount = 0,
        justBlinked = false,
        warriorSpells = { 29247, 29248, 29249 },
        championSpells = { 29238, 29255, 29257, 29258, 29262, 29267 },
        guardianSpells = { 29239, 29256, 29268 },
    }
    creature:Talk(SAY_AGGRO)
    enterPhaseGround(creature, guid)
end

local function nothLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function nothTargetDied(event, creature, victim)
    if victim ~= nil and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function nothDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

local function nothDamageTaken(event, creature, attacker, damage)
    local st = state[creature:GetGUID()]
    if st ~= nil and st.phase == "balcony"
            and damage >= creature:GetHealth() then
        creature:SetHealth(1)
        return false, 0
    end
end

local function nothJustSummoned(event, creature, summon)
    if summon ~= nil then
        summon:SetFarVisible(true)
        summon:DoZoneInCombat()
    end
end

local function nothReset(event, creature)
    local guid = creature:GetGUID()
    local st = state[guid]
    if st ~= nil and st.phase == "balcony" then
        creature:CastSpell(creature, SPELL_TELEPORT_BACK)
    end
    cancelTimers(guid)
    creature:SetReactState(2)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
end

RegisterCreatureEvent(ENTRY_NOTH, 1, nothEnterCombat)
RegisterCreatureEvent(ENTRY_NOTH, 2, nothLeaveCombat)
RegisterCreatureEvent(ENTRY_NOTH, 3, nothTargetDied)
RegisterCreatureEvent(ENTRY_NOTH, 4, nothDied)
RegisterCreatureEvent(ENTRY_NOTH, 9, nothDamageTaken)
RegisterCreatureEvent(ENTRY_NOTH, 19, nothJustSummoned)
RegisterCreatureEvent(ENTRY_NOTH, 23, nothReset)
