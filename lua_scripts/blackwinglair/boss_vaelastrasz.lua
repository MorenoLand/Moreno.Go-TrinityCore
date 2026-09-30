-- Vaelastrasz the Corrupt (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_vaelastrasz.cpp (boss_vaelAI + spell_vael_burning_adrenaline);
-- blackwing_lair.h:32 (DATA_VAELASTRAZ_THE_CORRUPT = 1, second boss),
-- :54 (NPC_VAELASTRAZ = 13020). Creature entry: 13020 Vaelastrasz the
-- Corrupt (C++ ScriptName "boss_vaelastrasz" per AddSC_boss_vaelastrasz).
-- The spell_vael_burning_adrenaline AuraScript from the same C++ file
-- has no Lua bridge (no AuraScript model) and is not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset. Eluna gossip
-- events: 1 OnGossipHello, 2 OnGossipSelect. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): the player talks to
-- Vael (one-shot gossip option — the C++ UNIT_NPC_FLAG_GOSSIP flag
-- removal is modeled with a per-GUID gate, so the option is never
-- offered again after the first select, and C++ Reset does NOT restore
-- it, so the one-shot survives wipes), then the speech machine runs
-- while out of combat — 1s SAY_LINE1 (0), 12s SAY_LINE2 (1), 12s
-- SAY_LINE3 (2), 16s the speech ends. Engaging early cancels the
-- speech timers C++-exact (in C++ the combat branch of UpdateAI
-- silently consumes unmatched speech events). OnEnterCombat: self-cast
-- essence of the red 23513 (the C++ SetHealth(30%) and
-- ResetPlayerDamageReq arms have no bridge), then arm cleave 19983
-- 10s then 15s (triggered DoCastVictim, C++-exact) / flame breath
-- 23461 15s then {8s,14s} (non-triggered DoCastVictim, C++-exact) /
-- fire nova 23462 20s then 15s (non-triggered DoCastVictim, C++-exact)
-- / tail swipe inert re-arm 15s (the DoCastVictim arm is commented
-- out upstream — only the re-arm is C++-exact) / burning adrenaline
-- caster 18173 15s then 15s (triggered cast on a random alive
-- mana-using player in the instance that is not the current victim —
-- C++ picks a non-pet mana user without the aura; players are never
-- pets and HasAura has no bridge, so those two filters drop) /
-- burning adrenaline tank 18173 45s then 45s (triggered self-cast on
-- the victim, C++ me->CastSpell(victim, ..., true) — non-victim-safe).
-- Nil-target ticks cast nothing but keep the schedule (jeklik
-- convention). OnTargetDied(3): Talk(SAY_KILLTARGET=4) fires on a
-- 1-in-5 roll (rand32() % 5 == 0, C++-exact), any victim — no type
-- gate in C++. OnDamageTaken(9): first time the pre-damage health is
-- at or below 15%, Talk(SAY_HALFLIFE=3) once (C++ checks post-damage
-- HealthBelowPct(15) every UpdateAI tick; the Lua check is a timing
-- approximation). OnDied(4)/OnLeaveCombat(2)/OnReset(23): cancel
-- timers, clear combat state; speechStarted survives Reset C++-exact.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (_Reset/BossAI::
-- JustEngagedWith and the DATA_VAELASTRAZ_THE_CORRUPT bookkeeping
-- arms skipped); the Reset SetStandState(UNIT_STAND_STATE_DEAD) and
-- the speech SetStandState(UNIT_STAND_STATE_STAND) arms, the
-- HandleEmoteCommand(EMOTE_ONESHOT_TALK) arms, and the
-- SetFaction(FACTION_FRIENDLY)/SetFaction(FACTION_DRAGONFLIGHT_BLACK)
-- arms have no bridges; speech_4's AttackStart on the gossip player
-- has no bearer (no movement/attack bridge on a non-combat target) —
-- the speech simply ends and the fight starts when a player pulls;
-- the DB gossip menu text (gossip_menu 6101) is approximated in the
-- hello handler — the real option text lives in the DB, not in C++;
-- PlayerGUID (ObjectGuid) has no bridge — the gossiping player is not
-- tracked across the speech; GetHealthPct has no pre/post-damage
-- distinction — the 15% yell gate is pre-damage; the burning
-- adrenaline caster-arm HasAura(SPELL_BURNINGADRENALINE) no-duplicate
-- filter and the non-pet SelectTarget filter have no bridge — the
-- debuff may re-land on a target that already has it; the
-- spell_vael_burning_adrenaline AuraScript (OnAuraRemove of EFFECT_2
-- -> self-cast SPELL_BURNINGADRENALINE_EXPLOSION 23478) is not modeled
-- (standing AuraScript gap).

local ENTRY_VAELASTRAZ = 13020

local SAY_LINE1 = 0
local SAY_LINE2 = 1
local SAY_LINE3 = 2
local SAY_HALFLIFE = 3
local SAY_KILLTARGET = 4

local SPELL_ESSENCEOFTHERED = 23513
local SPELL_FLAMEBREATH = 23461
local SPELL_FIRENOVA = 23462
local SPELL_TAILSWIPE = 15847 -- inert upstream: only the re-arm is C++-exact
local SPELL_BURNINGADRENALINE = 18173
local SPELL_CLEAVE = 19983

local POWER_MANA = 0

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_BEGIN_TEXT = "Begin the fight against Vaelastrasz."

local timers = {}
local speechStarted = {}
local hasYelled = {}

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

-- C++ EVENT_CLEAVE: triggered DoCastVictim(19983); re-arm 15s.
local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE, true)
    end
    schedule(guid, "cleave", 15000, function()
        onCleave(creature, guid)
    end)
end

-- C++ EVENT_FLAMEBREATH: non-triggered DoCastVictim(23461); re-arm {8s,14s}.
local function onFlameBreath(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FLAMEBREATH)
    end
    schedule(guid, "flamebreath", math.random(8000, 14000), function()
        onFlameBreath(creature, guid)
    end)
end

-- C++ EVENT_FIRENOVA: non-triggered DoCastVictim(23462); re-arm 15s.
local function onFireNova(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIRENOVA)
    end
    schedule(guid, "firenova", 15000, function()
        onFireNova(creature, guid)
    end)
end

-- C++ EVENT_TAILSWIPE: the cast is commented out upstream — only the
-- 15s re-arm is modeled (inert firing).
local function onTailSwipe(creature, guid)
    schedule(guid, "tailswipe", 15000, function()
        onTailSwipe(creature, guid)
    end)
end

-- C++ EVENT_BURNINGADRENALINE_CASTER: triggered cast on a random
-- non-victim mana user; re-arm 15s unconditionally.
local function onBurningAdrenalineCaster(creature, guid)
    local victim = creature:GetVictim()
    local candidates = {}
    for _, p in ipairs(alivePlayersInInstance(creature)) do
        if p ~= victim and p:GetPowerType() == POWER_MANA then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        local pick = candidates[math.random(#candidates)]
        creature:CastSpell(pick, SPELL_BURNINGADRENALINE, true)
    end
    schedule(guid, "adcaster", 15000, function()
        onBurningAdrenalineCaster(creature, guid)
    end)
end

-- C++ EVENT_BURNINGADRENALINE_TANK: Vael casts it himself on the
-- victim, triggered; re-arm 45s unconditionally.
local function onBurningAdrenalineTank(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BURNINGADRENALINE, true)
    end
    schedule(guid, "adtank", 45000, function()
        onBurningAdrenalineTank(creature, guid)
    end)
end

-- Speech machine: runs out of combat after the gossip select.
-- C++ EVENT_SPEECH_1..4: 1s / 12s / 12s / 16s.
local function armSpeech(creature, guid)
    schedule(guid, "speech1", 1000, function()
        creature:Talk(SAY_LINE1)
        schedule(guid, "speech2", 12000, function()
            creature:Talk(SAY_LINE2)
            schedule(guid, "speech3", 12000, function()
                creature:Talk(SAY_LINE3)
                schedule(guid, "speech4", 16000, function()
                    -- C++ speech_4: SetFaction(FACTION_DRAGONFLIGHT_BLACK)
                    -- + AttackStart on the gossip player — both have no
                    -- bridge; the speech simply ends here and the fight
                    -- starts when a player pulls.
                end)
            end)
        end)
    end)
end

local function onGossipHello(event, player, creature)
    -- One-shot: stands in for the C++ UNIT_NPC_FLAG_GOSSIP flag
    -- removal (no flag bridge on the creature object). Survives Reset
    -- C++-exact — C++ never re-adds the flag.
    if not speechStarted[creature:GetGUID()] then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_BEGIN_TEXT, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    end
    player:GossipSendMenu(0, creature)
end

local function onGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    player:GossipComplete()
    if sender == GOSSIP_SENDER_MAIN and action == GOSSIP_ACTION_INFO_DEF + 1 then
        local guid = creature:GetGUID()
        if not speechStarted[guid] then
            speechStarted[guid] = true
            -- C++ BeginSpeech: clears the gossip flag (the gate above),
            -- records the player (ObjectGuid has no bridge), arms the
            -- speech machine.
            armSpeech(creature, guid)
        end
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    -- Entering combat drops pending speech events C++-exact (the C++
    -- combat branch consumes unmatched speech events without acting).
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_ESSENCEOFTHERED)
    -- C++ also: SetHealth(30% of max), ResetPlayerDamageReq — no bridge.
    schedule(guid, "cleave", 10000, function()
        onCleave(creature, guid)
    end)
    schedule(guid, "flamebreath", 15000, function()
        onFlameBreath(creature, guid)
    end)
    schedule(guid, "firenova", 20000, function()
        onFireNova(creature, guid)
    end)
    schedule(guid, "tailswipe", 11000, function()
        onTailSwipe(creature, guid)
    end)
    schedule(guid, "adcaster", 15000, function()
        onBurningAdrenalineCaster(creature, guid)
    end)
    schedule(guid, "adtank", 45000, function()
        onBurningAdrenalineTank(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(event, creature, victim)
    -- C++: if (rand32() % 5) return; — Talk fires on a 1-in-5 roll.
    if math.random(0, 4) == 0 then
        creature:Talk(SAY_KILLTARGET, victim)
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(event, creature, attacker, damage)
    -- C++ UpdateAI: HealthBelowPct(15) && !HasYelled -> SAY_HALFLIFE,
    -- once. Pre-damage approximation (no pre/post-damage distinction).
    local guid = creature:GetGUID()
    if not hasYelled[guid] and creature:GetHealthPct() <= 15 then
        hasYelled[guid] = true
        creature:Talk(SAY_HALFLIFE)
    end
    -- Damage is never modified (thekal convention returns nothing when
    -- the hook does not alter it).
end

local function onReset(event, creature)
    -- C++ Reset: SetStandState(DEAD) has no bridge — timer/state-only.
    -- Initialize() also clears HasYelled (C++-exact).
    local guid = creature:GetGUID()
    hasYelled[guid] = false
    cancelTimers(guid)
end

RegisterCreatureEvent(ENTRY_VAELASTRAZ, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_VAELASTRAZ, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_VAELASTRAZ, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_VAELASTRAZ, 4, onDied)
RegisterCreatureEvent(ENTRY_VAELASTRAZ, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_VAELASTRAZ, 23, onReset)
RegisterCreatureGossipEvent(ENTRY_VAELASTRAZ, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY_VAELASTRAZ, 2, onGossipSelect)
