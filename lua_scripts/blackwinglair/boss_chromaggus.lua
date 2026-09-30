-- Chromaggus (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_chromaggus.cpp (boss_chromaggusAI only); blackwing_lair.h:37
-- (DATA_CHROMAGGUS = 6, seventh boss), :59 (NPC_CHROMAGGUS = 14020).
-- Creature entry: 14020 Chromaggus (C++ ScriptName "boss_chromaggus"
-- per AddSC_boss_chromaggus). One Talk line: EMOTE_SHIMMER = 1
-- (EMOTE_FRENZY = 0 is declared but unused by the AI).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (pre-damage hook, moroes convention), 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): the C++ constructor
-- rolls urand(0, 19) picking an ordered pair of distinct breaths from
-- the five (incinerate 23308, timelapse 23310, corrosive acid 23313,
-- ignite flesh 23315, frost burn 23187) and re-rolls nothing on Reset
-- — the Lua state keeps the pair per GUID across wipes and clears it
-- only on death, matching the C++ AI lifetime. OnEnterCombat: arm
-- shimmer 0s then every 45s — remove the old vulnerability aura,
-- non-triggered self-cast one of 22277/22278/22279/22280/22281,
-- Talk(EMOTE_SHIMMER=1) / breath1 30s then every 60s,
-- non-triggered DoCastVictim / breath2 60s then every 60s,
-- non-triggered DoCastVictim / affliction 10s then every 10s — each
-- alive player in the instance gets a triggered cast of one random
-- brood affliction (23153/23154/23155/23170/23169), and a player
-- carrying all five gets non-triggered 23174 chromatic mutation
-- (C++-exact; the bridge applies the aura casts, netherspite
-- convention) / frenzy 28371 15s then {10s,15s}, non-triggered
-- self-cast. Sub-20% enrage: C++ UpdateAI checks
-- !Enraged && HealthBelowPct(20) every tick — modeled on the
-- pre-damage hook as post-damage health strictly below 20% (moroes
-- convention), once, non-triggered self-cast 28747, damage untouched.
-- Nil-victim ticks cast nothing but keep the schedule (jeklik
-- convention). OnDied/OnLeaveCombat(2)/OnReset(23): cancel timers,
-- clear the enrage flag; the breath pair survives wipes (C++-exact)
-- and clears only on death.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::_Reset/
-- JustEngagedWith and the DATA_CHROMAGGUS bookkeeping arms skipped);
-- the go_chromaggus_lever GameObjectScript (door open + boss-state
-- set + JustEngagedWith on lever gossip) has no gameobject bridge and
-- is not registered (broodlord suppression-device convention) — the
-- fight starts on pull instead of on the lever.

local ENTRY_CHROMAGGUS = 14020

local SPELL_FIRE_VULNERABILITY = 22277
local SPELL_FROST_VULNERABILITY = 22278
local SPELL_SHADOW_VULNERABILITY = 22279
local SPELL_NATURE_VULNERABILITY = 22280
local SPELL_ARCANE_VULNERABILITY = 22281

local SPELL_INCINERATE = 23308
local SPELL_TIMELAPSE = 23310
local SPELL_CORROSIVEACID = 23313
local SPELL_IGNITEFLESH = 23315
local SPELL_FROSTBURN = 23187

local SPELL_BROODAF_BLUE = 23153
local SPELL_BROODAF_BLACK = 23154
local SPELL_BROODAF_RED = 23155
local SPELL_BROODAF_BRONZE = 23170
local SPELL_BROODAF_GREEN = 23169
local SPELL_CHROMATIC_MUT_1 = 23174

local SPELL_FRENZY = 28371
local SPELL_ENRAGE = 28747

local EMOTE_SHIMMER = 1

local VULNERABILITIES = {
    SPELL_FIRE_VULNERABILITY,
    SPELL_FROST_VULNERABILITY,
    SPELL_SHADOW_VULNERABILITY,
    SPELL_NATURE_VULNERABILITY,
    SPELL_ARCANE_VULNERABILITY,
}

local AFFLICTIONS = {
    SPELL_BROODAF_BLUE,
    SPELL_BROODAF_BLACK,
    SPELL_BROODAF_RED,
    SPELL_BROODAF_BRONZE,
    SPELL_BROODAF_GREEN,
}

-- C++ urand(0, 19) table, same (breath1, breath2) order as the C++
-- switch.
local BREATH_PAIRS = {
    { SPELL_INCINERATE, SPELL_TIMELAPSE },
    { SPELL_INCINERATE, SPELL_CORROSIVEACID },
    { SPELL_INCINERATE, SPELL_IGNITEFLESH },
    { SPELL_INCINERATE, SPELL_FROSTBURN },
    { SPELL_TIMELAPSE, SPELL_INCINERATE },
    { SPELL_TIMELAPSE, SPELL_CORROSIVEACID },
    { SPELL_TIMELAPSE, SPELL_IGNITEFLESH },
    { SPELL_TIMELAPSE, SPELL_FROSTBURN },
    { SPELL_CORROSIVEACID, SPELL_INCINERATE },
    { SPELL_CORROSIVEACID, SPELL_TIMELAPSE },
    { SPELL_CORROSIVEACID, SPELL_IGNITEFLESH },
    { SPELL_CORROSIVEACID, SPELL_FROSTBURN },
    { SPELL_IGNITEFLESH, SPELL_INCINERATE },
    { SPELL_IGNITEFLESH, SPELL_CORROSIVEACID },
    { SPELL_IGNITEFLESH, SPELL_TIMELAPSE },
    { SPELL_IGNITEFLESH, SPELL_FROSTBURN },
    { SPELL_FROSTBURN, SPELL_INCINERATE },
    { SPELL_FROSTBURN, SPELL_TIMELAPSE },
    { SPELL_FROSTBURN, SPELL_CORROSIVEACID },
    { SPELL_FROSTBURN, SPELL_IGNITEFLESH },
}

local timers = {}
local chromaggusState = {}

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

-- C++ constructor arm: roll the breath pair once per AI lifetime.
local function state(guid)
    local st = chromaggusState[guid]
    if not st then
        local pair = BREATH_PAIRS[math.random(1, 20)]
        st = {
            breath1 = pair[1],
            breath2 = pair[2],
            currentVuln = 0,
            enraged = false,
        }
        chromaggusState[guid] = st
    end
    return st
end

local function alivePlayersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ EVENT_SHIMMER: remove old vulnerability, non-triggered
-- self-cast a new random one, Talk(EMOTE_SHIMMER); re-arm 45s.
local function onShimmer(creature, guid)
    local st = state(guid)
    if st.currentVuln ~= 0 then
        creature:RemoveAura(st.currentVuln)
    end
    local spell = VULNERABILITIES[math.random(#VULNERABILITIES)]
    creature:CastSpell(creature, spell)
    st.currentVuln = spell
    creature:Talk(EMOTE_SHIMMER)
    schedule(guid, "shimmer", 45000, function()
        onShimmer(creature, guid)
    end)
end

-- C++ EVENT_BREATH_1: non-triggered DoCastVictim; re-arm 60s.
local function onBreath1(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, state(guid).breath1)
    end
    schedule(guid, "breath1", 60000, function()
        onBreath1(creature, guid)
    end)
end

-- C++ EVENT_BREATH_2: non-triggered DoCastVictim; re-arm 60s.
local function onBreath2(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, state(guid).breath2)
    end
    schedule(guid, "breath2", 60000, function()
        onBreath2(creature, guid)
    end)
end

-- C++ EVENT_AFFLICTION: each alive player in the instance gets a
-- triggered cast of one random affliction; a player carrying all five
-- gets non-triggered 23174; re-arm 10s.
local function onAffliction(creature, guid)
    for _, p in ipairs(alivePlayersInInstance(creature)) do
        creature:CastSpell(p, AFFLICTIONS[math.random(#AFFLICTIONS)], true)
        if p:HasAura(SPELL_BROODAF_BLUE)
                and p:HasAura(SPELL_BROODAF_BLACK)
                and p:HasAura(SPELL_BROODAF_RED)
                and p:HasAura(SPELL_BROODAF_BRONZE)
                and p:HasAura(SPELL_BROODAF_GREEN) then
            creature:CastSpell(p, SPELL_CHROMATIC_MUT_1)
        end
    end
    schedule(guid, "affliction", 10000, function()
        onAffliction(creature, guid)
    end)
end

-- C++ EVENT_FRENZY: non-triggered self-cast 28371; re-arm {10s,15s}.
local function onFrenzy(creature, guid)
    creature:CastSpell(creature, SPELL_FRENZY)
    schedule(guid, "frenzy", math.random(10000, 15000), function()
        onFrenzy(creature, guid)
    end)
end

local function chromaggusEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state(guid).enraged = false
    state(guid).currentVuln = 0
    schedule(guid, "shimmer", 0, function()
        onShimmer(creature, guid)
    end)
    schedule(guid, "breath1", 30000, function()
        onBreath1(creature, guid)
    end)
    schedule(guid, "breath2", 60000, function()
        onBreath2(creature, guid)
    end)
    schedule(guid, "affliction", 10000, function()
        onAffliction(creature, guid)
    end)
    schedule(guid, "frenzy", 15000, function()
        onFrenzy(creature, guid)
    end)
end

-- C++ UpdateAI sub-20% enrage: !Enraged && HealthBelowPct(20). The
-- damage hook fires pre-application, so the post-damage health decides
-- (moroes convention); strictly below 20 like C++; damage untouched.
local function chromaggusDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = chromaggusState[guid]
    if st == nil or st.enraged then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 20 then
        creature:CastSpell(creature, SPELL_ENRAGE)
        st.enraged = true
    end
end

local function chromaggusClear(guid)
    cancelTimers(guid)
    chromaggusState[guid] = nil
end

local function chromaggusLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function chromaggusDied(event, creature, killer)
    chromaggusClear(creature:GetGUID())
end

local function chromaggusReset(event, creature)
    cancelTimers(creature:GetGUID())
    local st = chromaggusState[creature:GetGUID()]
    if st then
        st.enraged = false
        st.currentVuln = 0
    end
end

RegisterCreatureEvent(ENTRY_CHROMAGGUS, 1, chromaggusEnterCombat)
RegisterCreatureEvent(ENTRY_CHROMAGGUS, 2, chromaggusLeaveCombat)
RegisterCreatureEvent(ENTRY_CHROMAGGUS, 4, chromaggusDied)
RegisterCreatureEvent(ENTRY_CHROMAGGUS, 9, chromaggusDamageTaken)
RegisterCreatureEvent(ENTRY_CHROMAGGUS, 23, chromaggusReset)
