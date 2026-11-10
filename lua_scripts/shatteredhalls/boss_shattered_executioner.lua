-- Shattered Executioner (Shattered Halls, Hellfire Citadel) —
-- Lua port of src/server/scripts/Outland/HellfireCitadel/
-- ShatteredHalls/shattered_halls.cpp (boss_shattered_
-- executioner — the only CreatureScript AI class
-- AddSC_shattered_halls registers whose combat timer is
-- bridgeable; at_nethekurse_exit is an AreaTriggerScript —
-- no Lua area-trigger bridge, not registered (hakkar/
-- zulgurub precedent); its whole OnTrigger body is
-- instance-gated anyway (GetInstanceScript, IsHeroic,
-- GetGuidData(NPC_KARGATH_BLADEFIST)/GetGuidData(NPC_
-- SHATTERED_EXECUTIONER), SummonCreature, DoAction(ACTION_
-- EXECUTIONER_TAUNT = 1)); spell_kargath_executioner is an
-- AuraScript and spell_remove_kargath_executioner is a
-- SpellScript — no AuraScript/SpellScript bridges, both
-- documented only, not registered).
-- Zone .cpp in the Shattered Halls set per outland_script_
-- loader.cpp order (boss roster 3/3 closed with kargath;
-- instance_shattered_halls.lua ported 3fe088b3).
-- Entry: NPC_SHATTERED_EXECUTIONER = 17301 from shattered_
-- halls.h (SHCreatureIds); the creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace). The
-- executioner defines no Say enum — it never Talk()s.
-- SPELL_CLEAVE = 15284 is the .cpp's own enum; SPELL_
-- KARGATH_EXECUTIONER_1/2/3 (39288/39289/39290) and SPELL_
-- REMOVE_KARGATH_EXECUTIONER (39291) from shattered_halls.h
-- are data only here (their AuraScript/SpellScript scripts
-- have no bridge).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset, 38 OnDoAction (the SetData relay —
-- the instance file fires it after each SetData(4) write).
-- No event 3: the C++ has no KilledUnit override.
-- Fight shape (C++-exact for all modeled arms): Shattered
-- Executioner (17301): OnEnterCombat(1) — per-GUID reset to
-- the C++ Initialize() schedule (cleaveTimer = 500) + 500ms
-- scheduler pump (a port of UpdateAI — 500ms granularity, so
-- the 500ms-init cleave fires on time; the !UpdateVictim early
-- return collapses into the pump). Pump: cleave 15284 500ms init
-- then {5s,7s} — non-triggered DoCastVictim (felmyst
-- convention: C++ `DoCast(SPELL_CLEAVE)` single-arg casts
-- on the victim), re-arm {5s,7s} regardless (C++-exact).
-- OnReset(23) — the C++ Reset() arms: loot modes re-added per
-- prisonersExecuted (0 -> HM3, <=1 -> HM2, <=2 -> HM1; _Reset
-- first restores LOOT_MODE_DEFAULT via SetLootMode) read from
-- the shared instanceData table (gruul precedent) with the
-- creature's own GetData(4) store as fallback, and
-- SetImmuneToPC via the flag twin (RemoveFlag(59, 0x100) iff
-- GetBossState(DATA_KARGATH) == DONE, else SetFlag — opera
-- precedent, Unit.cpp:8440-8449). OnDied(4): the _JustDied arm
-- + the quest leg (prisonersExecuted > 0 -> return; else each
-- instance player with the quest incomplete completes 9524
-- Alliance / 9525 Horde). SetData(DATA_PRISONERS_EXECUTED)
-- arrives as a store write (lua_creature_events.go:792) plus
-- an ev38 relay from the instance file (broggok precedent):
-- victim kill via GetInstanceCreature(DATA_FIRST_PRISONER +
-- data - 1) (nil in practice — prisoner GUIDs are never
-- latched, areatrigger unported), FailQuest at data == 1, and
-- the RemoveLootMode fall-through (3 -> HM1, 2 -> HM2,
-- 1 -> HM3). OnLeaveCombat(2)/OnReset(23): cancel the pump,
-- drop per-GUID state (the Reset() observable remainder —
-- cleaveTimer = 500 — lands on OnEnterCombat per the
-- schedule precedent).
-- Melee is engine-driven.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (the BossAI ctor DATA_SHATTERED_EXECUTIONER = 3
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped —
-- DATA_SHATTERED_EXECUTIONER is an instance-side constant,
-- unbridgeable; GetShatteredHallsAI is an AI-factory helper,
-- not a script); no Lua area-trigger bridge — at_nethekurse_
-- exit unported (its whole OnTrigger body is instance-gated
-- anyway); spell_kargath_executioner is an AuraScript and
-- spell_remove_kargath_executioner is a SpellScript — no
-- AuraScript/SpellScript bridges, both documented only, not
-- registered; no Kargath-DONE Reset trigger — C++
-- instance_shattered_halls.cpp:155-156 calls executioner->
-- AI()->Reset() when Kargath dies and no creature:Reset()
-- bridge exists, so the immune-flag clear lands only on the
-- executioner's next own Reset (kargath-file relay pending);
-- the empty JustSummoned override (anti-despawn of
-- prisoners on death/reset) is skipped (ahune bunny /
-- nethekurse fissure empty-skeleton precedent).

local SPELL_CLEAVE = 15284

local ENTRY_SHATTERED_EXECUTIONER = 17301

-- shattered_halls.h SHDataTypes / SHQuestIds / LootModes
-- (SharedDefines.h:43-46) / QuestDef.h:101-109.
local DATA_KARGATH = 2
local DATA_PRISONERS_EXECUTED = 4
local DATA_FIRST_PRISONER = 6
local QUEST_IMPRISONED_A = 9524
local QUEST_IMPRISONED_H = 9525
local QUEST_STATUS_INCOMPLETE = 3
local BOSS_STATE_DONE = 3
local LOOT_MODE_DEFAULT = 0x1
local LOOT_MODE_HARD_MODE_1 = 0x2
local LOOT_MODE_HARD_MODE_2 = 0x4
local LOOT_MODE_HARD_MODE_3 = 0x8
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100

-- Lua-side transport id (no C++ counterpart — the executioner
-- AI has no DoAction override): the instance file fires this
-- ev38 action right after each SetData(DATA_PRISONERS_EXECUTED)
-- write, so the SetData machine below runs once per write.
local ACTION_PRISONERS_EXECUTED = 1

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ Initialize() values: cleaveTimer = 500.
local function freshState()
    return {
        cleave = 500,
    }
end

-- C++ instance->GetData(DATA_PRISONERS_EXECUTED): the shared
-- instanceData table is the live source (the instance file
-- maintains prisonersExecuted there); the creature's own
-- GetData(4) store is the fallback (mirrors the last SetData
-- write; absent types read 0, the C++ member default).
local function prisonersExecutedOf(creature)
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data and data.prisonersExecuted ~= nil then
        return data.prisonersExecuted
    end
    return creature:GetData(DATA_PRISONERS_EXECUTED)
end

-- C++ Map::PlayerList (chromaggus precedent): every player in
-- this creature's map+instance, no alive gate (the C++
-- JustDied/SetData loops have none).
local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ Reset() (shattered_halls.cpp:85-108): _Reset() restores
-- the default loot mode, then the hard modes are re-added per
-- prisonersExecuted, and SetImmuneToPC follows Kargath's
-- state (false iff DATA_KARGATH is DONE).
local function applyResetArms(creature)
    local executed = prisonersExecutedOf(creature)
    creature:SetLootMode(LOOT_MODE_DEFAULT)
    if executed == 0 then
        creature:AddLootMode(LOOT_MODE_HARD_MODE_3)
    end
    if executed <= 1 then
        creature:AddLootMode(LOOT_MODE_HARD_MODE_2)
    end
    if executed <= 2 then
        creature:AddLootMode(LOOT_MODE_HARD_MODE_1)
    end
    if creature:GetBossState(DATA_KARGATH) == BOSS_STATE_DONE then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    else
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    end
end

-- C++ SetData(DATA_PRISONERS_EXECUTED, data) (shattered_halls.
-- cpp:125-176): data <= 3 only. Victim kill via the instance
-- GUID store (nil in practice — prisoner GUIDs are never
-- latched, areatrigger unported); at data == 1 every instance
-- player with the quest incomplete fails it; the loot-mode
-- fall-through removes HM1 at 3, HM2 at 2, HM3 at 1.
local function applyPrisonersExecuted(creature, data)
    if data > 3 then
        return
    end
    local victim = creature:GetInstanceCreature(DATA_FIRST_PRISONER + data - 1)
    if victim then
        victim:Kill(creature)
    end
    if data == 1 then
        for _, player in ipairs(playersInInstance(creature)) do
            local quest = QUEST_IMPRISONED_H
            if player:GetTeam() == 0 then
                quest = QUEST_IMPRISONED_A
            end
            if player:GetQuestStatus(quest) == QUEST_STATUS_INCOMPLETE then
                player:FailQuest(quest)
            end
        end
    end
    if data == 3 then
        creature:RemoveLootMode(LOOT_MODE_HARD_MODE_1)
    end
    if data >= 2 then
        creature:RemoveLootMode(LOOT_MODE_HARD_MODE_2)
    end
    if data >= 1 then
        creature:RemoveLootMode(LOOT_MODE_HARD_MODE_3)
    end
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Cleave 15284: 500ms init then {5s,7s} — non-triggered
    -- DoCastVictim (felmyst convention; C++-exact), re-arm
    -- {5s,7s} regardless (C++ `urand(5000, 7000)`, inclusive).
    -- 500ms pump: the 500ms init fires on the first tick, as C++.
    if st.cleave <= 500 then
        creature:CastSpell(nil, SPELL_CLEAVE)
        st.cleave = math.random(5000, 7000)
    else
        st.cleave = st.cleave - 500
    end
end

RegisterCreatureEvent(ENTRY_SHATTERED_EXECUTIONER, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    states[guid] = freshState()
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 500, 0)
end)

RegisterCreatureEvent(ENTRY_SHATTERED_EXECUTIONER, 2, function(_, creature)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_SHATTERED_EXECUTIONER, 4, function(_, creature)
    creature:SetBossState(3, 3) -- DATA_SHATTERED_EXECUTIONER, DONE (shattered_halls.cpp: _JustDied then quest leg)
    -- C++ JustDied (shattered_halls.cpp:110-123): prisonersExecuted
    -- > 0 -> return; else each instance player with the quest
    -- incomplete completes it (9524 Alliance / 9525 Horde).
    if prisonersExecutedOf(creature) == 0 then
        for _, player in ipairs(playersInInstance(creature)) do
            local quest = QUEST_IMPRISONED_H
            if player:GetTeam() == 0 then
                quest = QUEST_IMPRISONED_A
            end
            if player:GetQuestStatus(quest) == QUEST_STATUS_INCOMPLETE then
                player:CompleteQuest(quest)
            end
        end
    end
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_SHATTERED_EXECUTIONER, 23, function(_, creature)
    resetBoss(creature:GetGUIDLow())
    applyResetArms(creature)
end)

-- C++ SetData relay (broggok precedent): the instance file
-- fires ACTION_PRISONERS_EXECUTED after each SetData(4)
-- write; the machine reads the stored value.
RegisterCreatureEvent(ENTRY_SHATTERED_EXECUTIONER, 38, function(_, creature, action)
    if action == ACTION_PRISONERS_EXECUTED then
        applyPrisonersExecuted(creature, creature:GetData(DATA_PRISONERS_EXECUTED))
    end
end)
