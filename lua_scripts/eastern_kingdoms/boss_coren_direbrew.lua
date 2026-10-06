-- Coren Direbrew (Blackrock Depths, Grim Guzzler) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/boss_coren_direbrew.cpp (boss_coren_direbrew —
-- BossAI combat scheduler via GetBlackrockDepthsAI, an instance-AI
-- retrieval wrapper).
-- Boss-script unit per eastern_kingdoms_script_loader.cpp order
-- (boss_coren_direbrew follows boss_tomb_of_seven; the BRD block
-- continues with the next boss group).
-- Entry (verifiable from the C++ sources): NPC_COREN = 23872 in
-- instance_blackrock_depths.cpp line 47 (the BRD instance creatures
-- enum). The creature_template ScriptName binding is DB-side (no TDB
-- in this workspace), but the entry itself is C++-verifiable so the
-- port is registered (npc_phalanx / boss_doomrel precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Eluna gossip events: 1 OnGossipHello,
-- 2 OnGossipSelect. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ DoAction(ACTION_START_FIGHT) event schedule +
-- UpdateAI event machine):
-- EVENT_SUMMON_MOLE_MACHINE: triggered self-cast Summon Mole Machine
-- Target Picker 47691, 15s init -> 15s loop (angerforge precedent —
-- self-cast timer, C++ casts with TRIGGERED_FULL_MASK + max-targets
-- mod; the picker/minion SpellScripts are unported but the self-cast
-- is a real engine spell).
-- EVENT_DIREBREW_DISARM: triggered self-cast Direbrew's Disarm
-- (precast) 47407, 20s init -> 20s loop (angerforge precedent —
-- self-cast timer; the disarm AuraScript itself is unported).
-- The timers schedule from OnEnterCombat(1) here; C++ schedules them
-- from DoAction(ACTION_START_FIGHT) which is only reachable through
-- the gossip fight option — see gossip notes below.
-- DamageTaken (C++-exact documented-only): 66% phase-two latch ->
-- SummonCreature(NPC_ILSA_DIREBREW 26764) once; 33% phase-three latch
-- -> SummonCreature(NPC_URSULA_DIREBREW 26822) once — no summon
-- bridge on the Lua surface (flamelash precedent), documented-only.
-- SummonedCreatureDies 1s sister-respawn re-arms are likewise
-- summon-machine arms (no bridge), documented-only.
-- Gossip (vael precedent — RegisterCreatureGossipEvent exists and
-- fires inline in gossip.go): OnGossipHello offers the fight and
-- apologize options; OnGossipSelect FIGHT closes the menu — the C++
-- arms (Talk(SAY_INSULT), DoAction(ACTION_START_FIGHT) ->
-- SetImmuneToPC(false) + SetFaction(FACTION_GOBLIN_DARK_IRON_BAR_PATRON)
-- + DoZoneInCombat) have no bridges, so the fight starts when a
-- player pulls. APOLOGIZE closes the menu C++-exact. The option/menu
-- texts are approximated — the real strings live in the DB
-- broadcast_text rows (vael precedent).
-- Unmodeled: intro phase (MoveInLineOfSight -> PHASE_INTRO event
-- machine, 3x NPC_ANTAGONIST 23795 summons at fixed positions,
-- SAY_INTRO/1/2 Talk arms, ACTION_ANTAGONIST_SAY_1/2/ HOSTILE relayed
-- via EntryCheckPredicate + DoAction) — no summon/DoAction bridges
-- (doomrel precedent for the Talk arms).
-- Unmodeled: npc_coren_direbrew_sisters (26764/26822 — the JustEngagedWith
-- arm is self-cast PORT_TO_COREN 52850 + control aura 47369/50278 then a
-- 2s->4s SelectTarget(Random) mug chuck 50276 — the chuck is the whole arm
-- and has no SelectTarget bridge, so no lua file; the control-aura ticks are
-- unported AuraScripts; documented-only).
-- Unmodeled: npc_direbrew_minion (faction latch + DoZoneInCombat + JustSummoned
-- instance-GUID latch — no faction/instance bridges); npc_direbrew_antagonist
-- (DoAction Talk/faction arms — no bridges).
-- Unmodeled: go_direbrew_mole_machine (GameObjectAI Reset -> UseDoorOrButton
-- + 50313 emerge + linked-trap GO activation — no GO-script bridge).
-- Unmodeled: the six SpellScripts/AuraScripts (47691 target picker, 47370 send
-- mug picker, 47344 request second mug, 47369 send mug control aura, 50278
-- barreled control aura, 47407 disarm) — no SpellScript/AuraScript bridges
-- (hellfire_peninsula precedent).
-- Unmodeled: JustDied LFG FinishDungeon(287) arm (sLFGMgr + group gates —
-- no LFG bridge).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and the
-- in-loop casting-state gate is dropped (maiden precedent). The C++
-- scheduler is driven from DoAction and drained by UpdateAI; the port
-- schedules from OnEnterCombat(1) and cancels on 2/4/23 (maiden convention).

local ENTRY_COREN = 23872

local SPELL_MOLE_MACHINE_TARGET_PICKER = 47691
local SPELL_DIREBREW_DISARM_PRE_CAST = 47407

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_FIGHT_TEXT = "I'll take that challenge!"
local GOSSIP_APOLOGIZE_TEXT = "Sorry, I'm too busy to fight."

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

-- C++ EVENT_SUMMON_MOLE_MACHINE: triggered self-cast 47691; re-arm 15s.
local function onSummonMoleMachine(creature, guid)
    creature:CastSpell(creature, SPELL_MOLE_MACHINE_TARGET_PICKER, true)
    schedule(guid, "molemachine", 15000, function()
        onSummonMoleMachine(creature, guid)
    end)
end

-- C++ EVENT_DIREBREW_DISARM: DoCastSelf(47407, true); re-arm 20s.
local function onDirebrewDisarm(creature, guid)
    creature:CastSpell(creature, SPELL_DIREBREW_DISARM_PRE_CAST, true)
    schedule(guid, "disarm", 20000, function()
        onDirebrewDisarm(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    schedule(guid, "molemachine", 15000, function()
        onSummonMoleMachine(creature, guid)
    end)
    schedule(guid, "disarm", 20000, function()
        onDirebrewDisarm(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(event, creature, attacker, damage)
    -- C++ DamageTaken: 66% one-shot phase-two latch -> SummonCreature
    -- NPC_ILSA_DIREBREW 26764; 33% one-shot phase-three latch ->
    -- SummonCreature NPC_URSULA_DIREBREW 26822 — both behind the absent
    -- summon bridge (flamelash precedent); documented-only here.
    -- Damage is never modified (thekal convention returns nothing when
    -- the hook does not alter it).
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onGossipHello(event, player, creature)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_FIGHT_TEXT, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_APOLOGIZE_TEXT, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
    player:GossipSendMenu(0, creature)
end

local function onGossipSelect(event, player, creature, sender, action)
    if sender ~= GOSSIP_SENDER_MAIN then
        return
    end
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipClearMenu()
        player:GossipComplete()
        -- C++ GOSSIP_OPTION_FIGHT: Talk(SAY_INSULT) +
        -- DoAction(ACTION_START_FIGHT) -> SetImmuneToPC(false) +
        -- SetFaction(FACTION_GOBLIN_DARK_IRON_BAR_PATRON) +
        -- DoZoneInCombat + antagonist ACTION_ANTAGONIST_HOSTILE relay —
        -- no Talk/faction/immune/DoAction bridges (vael precedent); the
        -- fight starts when a player pulls.
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipClearMenu()
        player:GossipComplete()
        -- C++ GOSSIP_OPTION_APOLOGIZE: CloseGossipMenuFor — C++-exact.
    end
end

RegisterCreatureEvent(ENTRY_COREN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_COREN, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_COREN, 4, onDied)
RegisterCreatureEvent(ENTRY_COREN, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_COREN, 23, onReset)
RegisterCreatureGossipEvent(ENTRY_COREN, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY_COREN, 2, onGossipSelect)
