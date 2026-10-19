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
-- removal is ported via GetNPCFlags/SetNPCFlags on the Eluna gossip
-- creature (the old "no flag bridge" claim was stale —
-- engine/world/lua_objects.go:657/666), so the option is never
-- offered again after the first select, and C++ Reset does NOT restore
-- it, so the one-shot survives wipes), then the speech machine runs
-- while out of combat — 1s SAY_LINE1 (0), 12s SAY_LINE2 (1), 12s
-- SAY_LINE3 (2), 16s the speech ends. Engaging early cancels the
-- speech timers C++-exact (in C++ the combat branch of UpdateAI
-- silently consumes unmatched speech events). OnEnterCombat: self-cast
-- essence of the red 23513 + SetHealth(30% of max) (C++-exact via the
-- creature SetHealth bridge; ResetPlayerDamageReq has no bridge), then
-- arm cleave 19983
-- 10s then 15s (non-triggered DoCastVictim, C++-exact) / flame breath
-- 23461 15s then {8s,14s} (non-triggered DoCastVictim, C++-exact) /
-- fire nova 23462 20s then 15s (non-triggered DoCastVictim, C++-exact)
-- / tail swipe inert re-arm 15s (the DoCastVictim arm is commented
-- out upstream — only the re-arm is C++-exact) / burning adrenaline
-- caster 18173 15s then 15s (triggered cast on a random alive
-- mana-using player in the instance that is not the current victim —
-- C++ picks a non-pet mana user without the aura; players are never
-- pets, so the pet filter drops, and player:HasAura(18173) applies
-- the no-duplicate filter) /
-- burning adrenaline tank 18173 45s then 45s (triggered self-cast on
-- the victim, C++ me->CastSpell(victim, ..., true) — non-victim-safe).
-- Nil-target ticks cast nothing but keep the schedule (jeklik
-- convention). OnTargetDied(3): Talk(SAY_KILLTARGET=4) fires on a
-- 1-in-5 roll (rand32() % 5 == 0, C++-exact), any victim — no type
-- gate in C++. OnDamageTaken(9): first time the pre-damage health is
-- at or below 15%, Talk(SAY_HALFLIFE=3) once (C++ checks post-damage
-- HealthBelowPct(15) every UpdateAI tick; the Lua check is a timing
-- approximation). OnDied(4)/OnLeaveCombat(2)/OnReset(23): cancel
-- timers, clear combat state; the gossip-flag one-shot survives Reset
-- C++-exact (C++ never re-adds UNIT_NPC_FLAG_GOSSIP).
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
-- filter is bridged via player:HasAura(18173); the non-pet
-- SelectTarget filter drops (candidates are players only, never
-- pets); the
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

-- UNIT_NPC_FLAG_GOSSIP (UnitDefines.h:186) — the engine runs the gossip
-- hooks only while this flag is set (gossip.go helloCreatureNPC /
-- select flag gates), so clearing it in BeginSpeech is the C++-exact
-- one-shot (C++ Reset never re-adds it).
local UNIT_NPC_FLAG_GOSSIP = 0x00000001

local timers = {}
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

-- C++ EVENT_CLEAVE: non-triggered DoCastVictim(19983); re-arm 15s.
local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
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
        if p ~= victim and p:GetPowerType() == POWER_MANA
                and not p:HasAura(SPELL_BURNINGADRENALINE) then
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
    -- C++ adds the item unconditionally; the engine only fires the hook
    -- while UNIT_NPC_FLAG_GOSSIP is set (gossip.go helloCreatureNPC flag
    -- gate), so the option is inherently one-shot. C++ Reset never
    -- re-adds the flag (boss_vaelastrasz.cpp:88-94) — one-shot survives
    -- wipes C++-exact.
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_BEGIN_TEXT, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipSendMenu(0, creature)
end

local function onGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    player:GossipComplete()
    if sender == GOSSIP_SENDER_MAIN and action == GOSSIP_ACTION_INFO_DEF + 1 then
        -- C++ BeginSpeech (boss_vaelastrasz.cpp:113-116):
        -- me->RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP) — the
        -- one-shot; the player GUID record (ObjectGuid) has no bridge.
        -- Then the speech machine arms.
        creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(UNIT_NPC_FLAG_GOSSIP)))
        armSpeech(creature, creature:GetGUID())
    end
    -- C++ OnGossipSelect returns false: the AI/native select arms
    -- proceed (Eluna GOSSIP_EVENT_ON_SELECT: a Lua false lets them
    -- proceed; true/nothing would veto them).
    return false
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    -- Entering combat drops pending speech events C++-exact (the C++
    -- combat branch consumes unmatched speech events without acting).
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_ESSENCEOFTHERED)
    -- C++ JustEngagedWith also: SetHealth(CountPctFromMaxHealth(30))
    -- (bridged: floor(maxHealth*30/100)); ResetPlayerDamageReq has no
    -- bridge.
    creature:SetHealth(math.floor(creature:GetMaxHealth() * 30 / 100))
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
    -- once. Post-damage math (moroes/nefarian convention): C++ checks
    -- post-damage health every UpdateAI tick, and the killing blow never
    -- yells because UpdateAI stops on death, hence the > 0 floor guard.
    local guid = creature:GetGUID()
    if not hasYelled[guid] then
        local max = creature:GetMaxHealth()
        local postDamage = creature:GetHealth() - damage
        if max > 0 and postDamage > 0
            and postDamage < math.floor(max * 15 / 100) then
            hasYelled[guid] = true
            creature:Talk(SAY_HALFLIFE)
        end
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
