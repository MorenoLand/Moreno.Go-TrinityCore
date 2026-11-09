-- Halls of Stone: npc_brann_hos — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/halls_of_stone.cpp
-- (npc_brann_hos : public ScriptedAI).
-- Entry: NPC_BRANN_BRONZEBEARD 28070 (halls_of_stone.cpp:93).
-- Ported: OnGossipHello (halls_of_stone.cpp:703-710) -> event 1:
-- "Brann, it would be our honor!" (GOSSIP_ICON_CHAT, sender 1,
-- action 1001) + menu 13100 (TEXT_ID_START). The IsQuestGiver /
-- PrepareQuestMenu arm is documented-only (no quest-menu bridge —
-- kalecgos precedent).
-- OnGossipSelect (halls_of_stone.cpp:686-696) -> event 2: actions
-- 1001/1002 (GOSSIP_ACTION_INFO_DEF+1/+2) close the menu; the
-- StartWP() escort call is unbridged — documented-only.
-- Ported: UpdateAI low-HP arm (halls_of_stone.cpp:674-679) — Talk(SAY_LOW_HEALTH
-- 1) once when health drops strictly below 30%, latch re-armed when healed back
-- to >= 30%; rides the pre-damage event-9 hook with the nefarian postDamage > 0
-- guard and the maulgar/ironaya floored-integer threshold.
-- Ported: DamageTaken sparklin-news latch (halls_of_stone.cpp:405-408) —
-- if the tribunal is IN_PROGRESS (GetBossState(DATA_TRIBUNAL_OF_AGES 2) ==
-- 1), any damage taken clears the latch (SetData(DATA_BRANN_SPARKLIN_NEWS
-- 1, 0)); the latch is initialized true on spawn (ev5, the ctor Initialize
-- :289) and read by achievement_brann_spankin_new (ported in
-- engine/world/achievement_criteria_scripts.go, the abuse_the_ooze
-- precedent). Reset (halls_of_stone.cpp:304-313) re-inits the latch and
-- sets the tribunal NOT_STARTED when not escorting — PORTED (tick 515)
-- on ev23 (IsEscorted bridged lua_creature_events.go:2380); the
-- DespawnDwarf leg has no Lua twin yet (no dwarf GUID list — summons
-- unported), noted, not fabricated.
-- Documented-only: the full escort event machine (tribunal waypoints,
-- Sjonnir intro) — the escort primitives are all bridged (StartEscort
-- :2391, MoveWaypoint :4219, OnReachWP event 6, IsEscorted :2380; the
-- old-hillsbrad precedent), so this is unported work, not a stay-out;
-- the escort machine is the next unit. npc_tribuna_controller
-- (halls_of_stone.cpp:134-267) — PORTED (tick 515) in
-- lua_scripts/northrend/npc_tribuna_controller.lua; its Marnak/Abedneum
-- loops activate via the ev38 protocol (actions 2/3) driven by Brann's
-- escort steps 12/18; the Kaddrak glare arm is dead in C++
-- (KaddrakGUIDList never populated — UpdateFacesList commented out),
-- deliberately omitted. UpdateFacesList is commented out in C++.

local ENTRY_BRANN_HOS = 28070

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_ITEM_START = "Brann, it would be our honor!"
local TEXT_ID_START = 13100

local SAY_LOW_HEALTH = 1

-- DATA_TRIBUNAL_OF_AGES = 2 / DATA_BRANN_SPARKLIN_NEWS = 1
-- (halls_of_stone.h:33 / halls_of_stone.cpp:121).
local DATA_TRIBUNAL_OF_AGES = 2
local DATA_BRANN_SPARKLIN_NEWS = 1

local lowHpLatched = {}

-- C++ UpdateAI low-HP arm (halls_of_stone.cpp:674-679): !bIsLowHP &&
-- HealthBelowPct(30) -> Talk(SAY_LOW_HEALTH), latch; bIsLowHP &&
-- !HealthBelowPct(30) -> latch cleared (healed back above 30%). The arm
-- sits in UpdateAI (post-damage tick read, never fires on the killing
-- blow), so the nefarian convention applies: ported on the pre-damage
-- event-9 hook with a postDamage > 0 guard and the maulgar/ironaya
-- strict-< floored integer threshold. The same hook clears the latch
-- when post-damage health climbs back to >= the threshold (C++ re-arms
-- the yell every UpdateAI tick).
local function brannDamageTaken(_, creature, _, damage)
    -- C++ DamageTaken (halls_of_stone.cpp:405-408): while the tribunal
    -- is IN_PROGRESS, any damage taken clears the sparklin-news latch.
    -- No postDamage guard: C++ DamageTaken fires pre-damage, including
    -- on the killing blow, and the leg reads no damage value.
    if creature:GetBossState(DATA_TRIBUNAL_OF_AGES) == 1 then
        creature:SetData(DATA_BRANN_SPARKLIN_NEWS, 0)
    end
    local guid = creature:GetGUIDLow()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = creature:GetHealth() - damage
    if postDamage > 0 and postDamage < math.floor(maxHealth * 30 / 100) then
        if not lowHpLatched[guid] then
            lowHpLatched[guid] = true
            creature:Talk(SAY_LOW_HEALTH)
        end
    elseif postDamage >= math.floor(maxHealth * 30 / 100) then
        lowHpLatched[guid] = nil
    end
end

-- C++ ctor Initialize (halls_of_stone.cpp:289): brannSparklinNews
-- starts true; the ev5 spawn leg is the ctor twin (respawn rebuilds
-- the AI, same as the sjonnir ev5 SetGuidData precedent).
local function brannSpawned(event, creature)
    creature:SetData(DATA_BRANN_SPARKLIN_NEWS, 1)
end

local function brannGossipHello(event, player, creature)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_START,
        GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipSendMenu(TEXT_ID_START, creature)
end

local function brannGossipSelect(event, player, creature, sender, action)
    if action == GOSSIP_ACTION_INFO_DEF + 1
        or action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipComplete()
    end
end

-- C++ UpdateAI low-HP arm (halls_of_stone.cpp:674-679): !bIsLowHP &&
-- HealthBelowPct(30) -> Talk(SAY_LOW_HEALTH), latch; bIsLowHP &&
-- !HealthBelowPct(30) -> latch cleared (healed back above 30%).
-- The heal re-arm rides the pre-heal event-11 hook (valithria
-- precedent): a heal that lifts health back to >= the floored 30%
-- threshold clears the latch, so a second drop below 30% re-yells.
local function brannHealReceived(_, creature, _, heal)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() + heal >= math.floor(maxHealth * 30 / 100) then
        lowHpLatched[creature:GetGUIDLow()] = nil
    end
end

-- C++ Reset (halls_of_stone.cpp:304-313): while NOT escorting,
-- Initialize() (latch re-init) + DespawnDwarf() +
-- SetBossState(DATA_TRIBUNAL_OF_AGES, NOT_STARTED). The DespawnDwarf
-- leg has no Lua twin yet (no dwarf GUID list — summons unported).
local function brannReset(event, creature)
    if not creature:IsEscorted() then
        lowHpLatched[creature:GetGUIDLow()] = nil
        creature:SetBossState(DATA_TRIBUNAL_OF_AGES, 0)
    end
end

RegisterCreatureGossipEvent(ENTRY_BRANN_HOS, 1, brannGossipHello)
RegisterCreatureGossipEvent(ENTRY_BRANN_HOS, 2, brannGossipSelect)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 5, brannSpawned)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 9, brannDamageTaken)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 11, brannHealReceived)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 23, brannReset)
