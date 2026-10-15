-- Baron Geddon (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_baron_geddon.cpp (boss_baron_geddonAI only); molten_core.h:35
-- (BOSS_BARON_GEDDON = 5, sixth boss), :59 (NPC_BARON_GEDDON = 12056).
-- Creature entry: 12056 Baron Geddon (C++ ScriptName
-- "boss_baron_geddon" per AddSC_boss_baron_geddon). One Talk line:
-- EMOTE_SERVICE = 0, spoken on the armageddon arm.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat arms
-- inferno 19695 45s then 45s, non-triggered self-cast (C++ DoCast
-- default is triggered=false — vaelastrasz convention) / ignite mana
-- 19659 30s then 30s, non-triggered on a random alive player in the
-- instance (the C++ -SPELL_IGNITE_MANA no-aura filter has no HasAura
-- bridge — the pick may land on a target already carrying it) /
-- living bomb 20475 35s then 35s, non-triggered on a random alive
-- player in the instance. Nil-target ticks cast nothing but keep the
-- schedule (jeklik convention). Per-tick UpdateAI armageddon check
-- (Lua 1s pump): health at-or-below 2% -> Talk(EMOTE_SERVICE=0) +
-- non-triggered self-cast armageddon 20478, C++-exact (no once-guard:
-- C++ re-casts and re-Talks every UpdateAI tick below 2%). OnDied(4)/
-- OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::JustEngagedWith and
-- the BOSS_BARON_GEDDON bookkeeping arms skipped); the armageddon
-- InterruptNonMeleeSpells arm has no bridge; the armageddon arm does
-- not suppress pending timers in the port (there is no tick-driven
-- UpdateAI return to carry it — jeklik convention); the
-- spell_baron_geddon_inferno AuraScript (escalating 19698 tick damage
-- {500,500,1000,1000,2000,2000,3000,5000} via triggered
-- CastSpellExtraArgs) has no AuraScript bridge (standing AuraScript
-- gap) — the inferno aura's tick damage is never customized.

local ENTRY_BARON_GEDDON = 12056

local EMOTE_SERVICE = 0

local SPELL_INFERNO = 19695
local SPELL_IGNITE_MANA = 19659
local SPELL_LIVING_BOMB = 20475
local SPELL_ARMAGEDDON = 20478

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

-- C++ EVENT_INFERNO: non-triggered DoCast(me, 19695); re-arm 45s.
local function onInferno(creature, guid)
    creature:CastSpell(creature, SPELL_INFERNO)
    schedule(guid, "inferno", 45000, function()
        onInferno(creature, guid)
    end)
end

-- Random alive player in the instance (wushoolay helper).
local function randomAlivePlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local players = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ EVENT_IGNITE_MANA: non-triggered cast 19659 on a random alive
-- player in the instance (the -SPELL_IGNITE_MANA no-aura filter has no
-- HasAura bridge); re-arm 30s.
local function onIgniteMana(creature, guid)
    local pick = randomAlivePlayer(creature)
    if pick then
        creature:CastSpell(pick, SPELL_IGNITE_MANA)
    end
    schedule(guid, "ignitemana", 30000, function()
        onIgniteMana(creature, guid)
    end)
end

-- C++ EVENT_LIVING_BOMB: non-triggered cast 20475 on a random alive
-- player in the instance; re-arm 35s.
local function onLivingBomb(creature, guid)
    local pick = randomAlivePlayer(creature)
    if pick then
        creature:CastSpell(pick, SPELL_LIVING_BOMB)
    end
    schedule(guid, "livingbomb", 35000, function()
        onLivingBomb(creature, guid)
    end)
end

local function baronGeddonEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "inferno", 45000, function()
        onInferno(creature, guid)
    end)
    schedule(guid, "ignitemana", 30000, function()
        onIgniteMana(creature, guid)
    end)
    schedule(guid, "livingbomb", 35000, function()
        onLivingBomb(creature, guid)
    end)
    schedule(guid, "armageddon", 1000, function()
        onArmageddonPump(creature, guid)
    end)
end

-- C++ UpdateAI: !HealthAbovePct(2) -> InterruptNonMeleeSpells (no
-- bridge), non-triggered DoCast(me, 20478), Talk(EMOTE_SERVICE),
-- return — checked every tick (no once-guard, no DamageTaken hook in
-- C++). Modeled as a 1s gated pump (phalanx convention): !HealthAbovePct(2)
-- is health <= floor(2M/100), so it fires AT exactly 2%.
local function onArmageddonPump(creature, guid)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth ~= 0
            and creature:GetHealth() <= math.floor(maxHealth * 2 / 100) then
        creature:CastSpell(creature, SPELL_ARMAGEDDON)
        creature:Talk(EMOTE_SERVICE)
    end
    schedule(guid, "armageddon", 1000, function()
        onArmageddonPump(creature, guid)
    end)
end

local function baronGeddonResetState(guid)
    cancelTimers(guid)
end

local function baronGeddonLeaveCombat(event, creature)
    baronGeddonResetState(creature:GetGUID())
end

local function baronGeddonDied(event, creature, killer)
    baronGeddonResetState(creature:GetGUID())
end

local function baronGeddonReset(event, creature)
    baronGeddonResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_BARON_GEDDON, 1, baronGeddonEnterCombat)
RegisterCreatureEvent(ENTRY_BARON_GEDDON, 2, baronGeddonLeaveCombat)
RegisterCreatureEvent(ENTRY_BARON_GEDDON, 4, baronGeddonDied)
RegisterCreatureEvent(ENTRY_BARON_GEDDON, 23, baronGeddonReset)
