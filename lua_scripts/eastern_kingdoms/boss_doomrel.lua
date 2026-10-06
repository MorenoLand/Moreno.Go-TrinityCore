-- Doomrel (Blackrock Depths, Tomb of Seven) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/boss_tomb_of_seven.cpp (boss_doomrel /
-- boss_doomrelAI — ScriptedAI combat scheduler via
-- GetBlackrockDepthsAI, an instance-AI retrieval wrapper).
-- Boss-script unit per eastern_kingdoms_script_loader.cpp order
-- (boss_tomb_of_seven follows boss_moira_bronzebeard; the BRD block
-- continues with coren_direbrew).
-- Entry (verifiable from the C++ sources): NPC_DOOMREL = 9039 in
-- instance_blackrock_depths.cpp line 43 (the BRD instance creatures
-- enum). The creature_template ScriptName binding is DB-side (no TDB
-- in this workspace), but the entry itself is C++-verifiable so the
-- port is registered (npc_phalanx / boss_draganthaurissan precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Eluna gossip events: 1 OnGossipHello,
-- 2 OnGossipSelect. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith schedule + UpdateAI event machine):
-- EVENT_SHADOW_BOLT_VOLLEY: victim-cast Shadowbolt Volley 15245,
-- 10s init -> 12s loop (maiden convention — DoCastVictim path,
-- creature:GetVictim(); nil-victim ticks cast nothing but keep the
-- schedule, jeklik convention).
-- EVENT_CURSE_OF_WEAKNESS: victim-cast Curse of Weakness 12493, 5s
-- init -> 45s loop.
-- EVENT_DEMONARMOR: self-cast Demon Armor 13787, 16s init -> 5min
-- loop (creature:CastSpell(creature, ...) self-cast, vael precedent).
-- DamageTaken: first time health is at or below 50% -> triggered
-- victim-cast Summon Voidwalkers 15092 (C++ DoCastVictim(..., true));
-- the once-flag clears on OnReset(23) C++-exact (Reset calls
-- Initialize()). The health check is the pre-damage approximation
-- (vael precedent — no pre/post-damage distinction on the Lua
-- surface).
-- Gossip (vael precedent — RegisterCreatureGossipEvent exists and
-- fires inline in gossip.go): OnGossipHello offers the challenge
-- option; OnGossipSelect action 1001 shows the confirm option;
-- action 1002 closes the menu and the challenge resolves. The C++
-- challenge trigger legs — SetFaction(FACTION_DARK_IRON_DWARVES),
-- SetImmuneToPC(false), AI()->AttackStart(player),
-- instance->SetGuidData(DATA_EVENSTARTER, player GUID) — have no
-- bridges (vael precedent: SetFaction and AttackStart on the gossip
-- player have no bearer), so the fight starts when a player pulls.
-- The option texts are approximated — the real strings live in the
-- DB broadcast_text rows (vael precedent).
-- Unmodeled: EVENT_IMMOLATE (18s init -> 25s) -> SelectTarget(Random)
-- + DoCast(12742) — no SelectTarget bridge on the Lua surface
-- (doomwalker / kazzak / doomrel precedent — this file is the cited
-- unit); the timer chain is documented-only, not a no-op dead loop.
-- Unmodeled: Reset's SetFaction(FACTION_FRIENDLY) /
-- SetImmuneToPC(true) / UNIT_NPC_FLAG_GOSSIP flag machine (gated on
-- instance DATA_GHOSTKILL >= 7), EnterEvadeMode's instance
-- SetGuidData(DATA_EVENSTARTER), JustDied's instance
-- SetData(DATA_GHOSTKILL, 1) — no faction/immune/flag/instance-script
-- bridges (instance_blackrock_depths.cpp stays blocked on the
-- instance-script model; vael precedent for the SetFaction arms).
-- Unmodeled: boss_gloomrel (same C++ file, entry 9037 —
-- C++-verifiable at instance_blackrock_depths.cpp:42) is a pure
-- gossip teaching script: its hello gates need
-- GetQuestRewardStatus(4083) and GetSkillValue(SKILL_MINING >= 230) —
-- no bridges on the Lua surface — and its select arms need
-- instance->DoRespawnGameObject(DATA_GO_CHALICE) — no bridge;
-- documented-only (dustwallow precedent), no lua file.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and the
-- in-loop casting-state gate is dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).

local ENTRY_DOOMREL = 9039

local SPELL_SHADOWBOLTVOLLEY = 15245
local SPELL_CURSEOFWEAKNESS = 12493
local SPELL_DEMONARMOR = 13787
local SPELL_SUMMON_VOIDWALKERS = 15092

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_CHALLENGE_TEXT = "I challenge you, Doomrel, to battle."
local GOSSIP_CONFIRM_TEXT = "Are you certain? There is no turning back."

local timers = {}
local voidwalkersSummoned = {}

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

-- C++ EVENT_SHADOW_BOLT_VOLLEY: DoCastVictim(15245); re-arm 12s.
local function onShadowBoltVolley(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOWBOLTVOLLEY)
    end
    schedule(guid, "volley", 12000, function()
        onShadowBoltVolley(creature, guid)
    end)
end

-- C++ EVENT_CURSE_OF_WEAKNESS: DoCastVictim(12493); re-arm 45s.
local function onCurseOfWeakness(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CURSEOFWEAKNESS)
    end
    schedule(guid, "curse", 45000, function()
        onCurseOfWeakness(creature, guid)
    end)
end

-- C++ EVENT_DEMONARMOR: DoCast(me, 13787); re-arm 5min.
local function onDemonArmor(creature, guid)
    creature:CastSpell(creature, SPELL_DEMONARMOR)
    schedule(guid, "armor", 300000, function()
        onDemonArmor(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    schedule(guid, "volley", 10000, function()
        onShadowBoltVolley(creature, guid)
    end)
    schedule(guid, "curse", 5000, function()
        onCurseOfWeakness(creature, guid)
    end)
    schedule(guid, "armor", 16000, function()
        onDemonArmor(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(event, creature, attacker, damage)
    -- C++ DamageTaken: !_voidwalkers && !HealthAbovePct(50) ->
    -- triggered DoCastVictim(15092), once. Pre-damage health
    -- approximation (vael precedent). Flag clears on Reset(23)
    -- C++-exact (Reset calls Initialize()).
    local guid = creature:GetGUID()
    if not voidwalkersSummoned[guid] and creature:GetHealthPct() <= 50 then
        voidwalkersSummoned[guid] = true
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_SUMMON_VOIDWALKERS, true)
        end
    end
    -- Damage is never modified (thekal convention returns nothing when
    -- the hook does not alter it).
end

local function onReset(event, creature)
    -- C++ Reset: Initialize() clears _voidwalkers; timer/state-only
    -- here (the faction/immune/flag arms have no bridges).
    local guid = creature:GetGUID()
    voidwalkersSummoned[guid] = false
    cancelTimers(guid)
end

local function onGossipHello(event, player, creature)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_CHALLENGE_TEXT, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipSendMenu(0, creature)
end

local function onGossipSelect(event, player, creature, sender, action)
    if sender ~= GOSSIP_SENDER_MAIN then
        return
    end
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipClearMenu()
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_CONFIRM_TEXT, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
        player:GossipSendMenu(0, creature)
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipClearMenu()
        player:GossipComplete()
        -- C++: SetFaction(FACTION_DARK_IRON_DWARVES) +
        -- SetImmuneToPC(false) + AI()->AttackStart(player) +
        -- instance->SetGuidData(DATA_EVENSTARTER, player GUID) — all
        -- behind absent bridges (vael precedent); the fight starts
        -- when a player pulls.
    end
end

RegisterCreatureEvent(ENTRY_DOOMREL, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_DOOMREL, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_DOOMREL, 4, onDied)
RegisterCreatureEvent(ENTRY_DOOMREL, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_DOOMREL, 23, onReset)
RegisterCreatureGossipEvent(ENTRY_DOOMREL, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY_DOOMREL, 2, onGossipSelect)
