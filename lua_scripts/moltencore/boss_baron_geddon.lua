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
-- 19659 30s then 30s, non-triggered on a threat-list player alive
-- (the C++ -SPELL_IGNITE_MANA no-aura filter is unmodeled —
-- player HasAura is bridged (lua_player.go:203) but the engine's
-- creature-cast path never applies auras to targets (boss_ai.go:1481,
-- no applyAura arm; player-cast path only, spells.go:8922), so
-- HasAura(19659) would always read false) /
-- living bomb 20475 35s then 35s, non-triggered on a threat-list
-- player alive. Nil-target ticks cast nothing but keep the
-- schedule (jeklik convention). Per-tick UpdateAI armageddon check
-- (Lua 1s pump): health at-or-below 2% -> Talk(EMOTE_SERVICE=0) +
-- non-triggered self-cast armageddon 20478, C++-exact (no once-guard:
-- C++ re-casts and re-Talks every UpdateAI tick below 2%). While the
-- pump holds the armageddon-suppression flag the inferno/ignite-mana/
-- living-bomb callbacks skip their casts but keep re-arming (C++
-- events.Update keeps ticking while ExecuteEvent is skipped,
-- boss_baron_geddon.cpp:80-86); engine melee has no suppression
-- bridge (documented delta). OnDied(4)/
-- OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no live UNIT_STATE_CASTING read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::JustEngagedWith and
-- the BOSS_BARON_GEDDON bookkeeping arms skipped); the armageddon
-- InterruptNonMeleeSpells arm is bridged (no-op twin) and ported in onArmageddonPump below; the armageddon arm does
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

-- Armageddon suppression: C++ UpdateAI returns right after the armageddon
-- arm while !HealthAbovePct(2) (boss_baron_geddon.cpp:80-86): the C++
-- timers keep ticking via events.Update but no other event executes and
-- no melee happens. The
-- three timer callbacks skip their casts while the pump holds this flag
-- but keep re-arming (the C++ timers keep ticking via events.Update).
-- Engine-driven melee has no Lua suppression bridge (documented delta).
local armageddonSuppress = {}

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
    if not armageddonSuppress[guid] then
        creature:CastSpell(creature, SPELL_INFERNO)
    end
    schedule(guid, "inferno", 45000, function()
        onInferno(creature, guid)
    end)
end

-- Threat-list players alive (C++ SelectTarget(Random, 0, 0.0f, true)
-- for both arms — playerOnly, no dist cap).
local function randomThreatPlayer(creature)
    local targets = creature:GetAITargets()
    local players = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            players[#players + 1] = unit
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ EVENT_IGNITE_MANA: non-triggered cast 19659 on a threat-list
-- player alive (the -SPELL_IGNITE_MANA no-aura filter is
-- unmodeled — creature casts apply no auras engine-side, so the
-- bridged player HasAura(19659) would always read false); re-arm 30s.
local function onIgniteMana(creature, guid)
    if not armageddonSuppress[guid] then
        local pick = randomThreatPlayer(creature)
        if pick then
            creature:CastSpell(pick, SPELL_IGNITE_MANA)
        end
    end
    schedule(guid, "ignitemana", 30000, function()
        onIgniteMana(creature, guid)
    end)
end

-- C++ EVENT_LIVING_BOMB: non-triggered cast 20475 on a threat-list
-- player alive; re-arm 35s.
local function onLivingBomb(creature, guid)
    if not armageddonSuppress[guid] then
        local pick = randomThreatPlayer(creature)
        if pick then
            creature:CastSpell(pick, SPELL_LIVING_BOMB)
        end
    end
    schedule(guid, "livingbomb", 35000, function()
        onLivingBomb(creature, guid)
    end)
end

-- Forward declaration: baronGeddonEnterCombat's timer closures reference
-- onArmageddonPump textually before its local definition; without this
-- the name compiles to a global and the armageddon pump stream calls nil.
local onArmageddonPump

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

-- C++ UpdateAI: !HealthAbovePct(2) -> InterruptNonMeleeSpells(true),
-- non-triggered DoCast(me, 20478), Talk(EMOTE_SERVICE),
-- return — checked every tick (no once-guard, no DamageTaken hook in
-- C++). Modeled as a 1s gated pump (phalanx convention): !HealthAbovePct(2)
-- is health <= floor(2M/100), so it fires AT exactly 2%.
-- NOTE: plain assignment, NOT `local function` — the forward-declared
-- `local onArmageddonPump` above is a separate slot; re-declaring with
-- `local function` would shadow it and the pump would call nil.
onArmageddonPump = function(creature, guid)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth ~= 0
            and creature:GetHealth() <= math.floor(maxHealth * 2 / 100) then
        armageddonSuppress[guid] = true
        creature:InterruptNonMeleeSpells(true)
        creature:CastSpell(creature, SPELL_ARMAGEDDON)
        creature:Talk(EMOTE_SERVICE)
    else
        armageddonSuppress[guid] = nil
    end
    schedule(guid, "armageddon", 1000, function()
        onArmageddonPump(creature, guid)
    end)
end

local function baronGeddonResetState(guid)
    cancelTimers(guid)
    armageddonSuppress[guid] = nil
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
